// Package webhook is a messaging.MessagingBackend that POSTs each envelope to its recipient's registered callback URL, retrying from an in-memory outbox.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

const (
	defaultPollInterval     = 200 * time.Millisecond
	defaultHTTPTimeout      = 10 * time.Second
	defaultRingBufferSize   = 200
	defaultSubscriberBuffer = 16
)

// CallbackResolver maps a worker ID to its callback URL when RegisterCallback has not recorded one.
type CallbackResolver func(workerID string) (url string, ok bool)

// Backend is a messaging.MessagingBackend that delivers envelopes by HTTP POST to per-worker callback URLs.
type Backend struct {
	clock        security.Clock
	httpClient   *http.Client
	policy       RetryPolicy
	resolver     CallbackResolver
	pollInterval time.Duration

	callbacksMu      sync.RWMutex
	callbackByWorker map[string]string

	ring *ringStore

	subsMu       sync.Mutex
	subsByWorker map[string][]*subscription

	out *outbox

	closedSignal chan struct{}
	closeOnce    sync.Once
}

type subscription struct {
	mu      sync.Mutex
	ch      chan messaging.Envelope
	ctxDone <-chan struct{}
	closed  bool
}

func (s *subscription) deliver(env messaging.Envelope, backendClosed <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.ch <- env:
	case <-s.ctxDone:
	case <-backendClosed:
	}
}

func (s *subscription) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.ch)
}

// Option configures a Backend constructed by New.
type Option func(*Backend)

// WithClock overrides the clock used for timestamps and retry scheduling.
func WithClock(clock security.Clock) Option {
	return func(b *Backend) { b.clock = clock }
}

// WithHTTPClient overrides the http.Client used to deliver callbacks.
func WithHTTPClient(client *http.Client) Option {
	return func(b *Backend) { b.httpClient = client }
}

// WithRetryPolicy overrides the backoff schedule.
func WithRetryPolicy(policy RetryPolicy) Option {
	return func(b *Backend) { b.policy = policy }
}

// WithPollInterval overrides how often the outbox loop checks for due retries.
func WithPollInterval(d time.Duration) Option {
	return func(b *Backend) { b.pollInterval = d }
}

// WithCallbackResolver installs the fallback consulted when RegisterCallback has not recorded a URL for a recipient.
func WithCallbackResolver(resolver CallbackResolver) Option {
	return func(b *Backend) { b.resolver = resolver }
}

// WithDefaultCallbackURL resolves every worker without an explicit RegisterCallback entry to url.
func WithDefaultCallbackURL(url string) Option {
	return WithCallbackResolver(func(string) (string, bool) { return url, true })
}

// New builds a webhook Backend with no callback URLs registered.
func New(opts ...Option) (*Backend, error) {
	b := &Backend{
		clock:            security.RealClock{},
		httpClient:       &http.Client{Timeout: defaultHTTPTimeout},
		policy:           DefaultRetryPolicy(),
		pollInterval:     defaultPollInterval,
		callbackByWorker: make(map[string]string),
		ring:             newRingStore(defaultRingBufferSize),
		subsByWorker:     make(map[string][]*subscription),
		closedSignal:     make(chan struct{}),
	}

	for _, opt := range opts {
		opt(b)
	}

	b.out = newOutbox(b.clock, b.policy, b.pollInterval, b.attemptDeliver, b.onDelivered)
	b.out.start()

	return b, nil
}

// RegisterCallback records the callback URL for workerID, overriding any prior registration or resolver result.
func (b *Backend) RegisterCallback(workerID, url string) {
	b.callbacksMu.Lock()
	defer b.callbacksMu.Unlock()
	b.callbackByWorker[workerID] = url
}

func (b *Backend) resolveCallback(workerID string) (string, bool) {
	b.callbacksMu.RLock()
	url, ok := b.callbackByWorker[workerID]
	b.callbacksMu.RUnlock()
	if ok {
		return url, true
	}
	if b.resolver != nil {
		return b.resolver(workerID)
	}
	return "", false
}

// Emit implements messaging.MessagingBackend.
func (b *Backend) Emit(ctx context.Context, env messaging.Envelope) error {
	if env.CreatedAt.IsZero() {
		env.CreatedAt = b.clock.Now()
	}

	err := b.attemptDeliver(ctx, env)
	if err == nil {
		b.onDelivered(env)
		return nil
	}

	b.out.enqueueFailed(env, 1, err)
	return nil
}

func (b *Backend) attemptDeliver(ctx context.Context, env messaging.Envelope) error {
	callbackURL, ok := b.resolveCallback(env.To)
	if !ok {
		return fmt.Errorf("messaging/webhook: no callback registered for worker %q", env.To)
	}
	if _, err := url.Parse(callbackURL); err != nil {
		return fmt.Errorf("messaging/webhook: invalid callback URL for worker %q: %w", env.To, err)
	}

	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("messaging/webhook: marshal envelope %q: %w", env.ID, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("messaging/webhook: build request for %q: %w", env.ID, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("messaging/webhook: POST envelope %q to worker %q: %w", env.ID, env.To, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("messaging/webhook: worker %q callback returned status %d for envelope %q", env.To, resp.StatusCode, env.ID)
	}
	return nil
}

func (b *Backend) onDelivered(env messaging.Envelope) {
	b.ring.add(env)

	b.subsMu.Lock()
	subs := append([]*subscription(nil), b.subsByWorker[env.To]...)
	b.subsMu.Unlock()

	for _, s := range subs {
		s.deliver(env, b.closedSignal)
	}
}

// Subscribe implements messaging.MessagingBackend.
func (b *Backend) Subscribe(ctx context.Context, workerID string) (<-chan messaging.Envelope, error) {
	sub := &subscription{
		ch:      make(chan messaging.Envelope, defaultSubscriberBuffer),
		ctxDone: ctx.Done(),
	}

	b.subsMu.Lock()
	b.subsByWorker[workerID] = append(b.subsByWorker[workerID], sub)
	b.subsMu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
		case <-b.closedSignal:
		}

		b.subsMu.Lock()
		list := b.subsByWorker[workerID]
		for i, s := range list {
			if s == sub {
				b.subsByWorker[workerID] = append(list[:i], list[i+1:]...)
				break
			}
		}
		b.subsMu.Unlock()

		sub.close()
	}()

	return sub.ch, nil
}

// Ack implements messaging.MessagingBackend.
func (b *Backend) Ack(ctx context.Context, ack messaging.Ack) error {
	if !b.ring.contains(ack.ByWorker, ack.MessageID) {
		return fmt.Errorf("messaging/webhook: ack %q: %w", ack.MessageID, messaging.ErrNotFound)
	}
	return nil
}

// Tail implements messaging.MessagingBackend.
func (b *Backend) Tail(ctx context.Context, filter messaging.TailFilter) ([]messaging.Envelope, error) {
	return b.ring.tail(filter), nil
}

// Health implements messaging.MessagingBackend.
func (b *Backend) Health(ctx context.Context) error {
	select {
	case <-b.closedSignal:
		return fmt.Errorf("messaging/webhook: backend is closed")
	default:
	}

	_, failed := b.out.stats()
	if failed > 0 {
		return fmt.Errorf("messaging/webhook: %d envelope(s) exhausted retry attempts and were not delivered", failed)
	}
	return nil
}

// Stats reports how many envelopes await a retry and how many exhausted MaxAttempts.
func (b *Backend) Stats() (pending, failed int) {
	return b.out.stats()
}

// Inspect returns the outbox retry bookkeeping for envelopeID.
func (b *Backend) Inspect(envelopeID string) (attempts int, nextAttempt time.Time, delivered, failed bool, ok bool) {
	snap, ok := b.out.inspect(envelopeID)
	if !ok {
		return 0, time.Time{}, false, false, false
	}
	return snap.Attempts, snap.NextAttempt, snap.Delivered, snap.Failed, true
}

// Tick runs one pass of the outbox retry check against the Backend's clock.
func (b *Backend) Tick(ctx context.Context) {
	b.out.tick(ctx)
}

// Close implements messaging.MessagingBackend.
func (b *Backend) Close() error {
	b.closeOnce.Do(func() {
		close(b.closedSignal)
		b.out.close()
	})
	return nil
}

func init() {
	messaging.Register("webhook", dispatch)
}

func dispatch(u *url.URL) (messaging.MessagingBackend, error) {
	return New()
}
