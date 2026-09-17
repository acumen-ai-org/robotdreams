// Package temporal is a messaging.MessagingBackend that keeps one Temporal mailbox workflow per worker, registered as temporal://<host:port>/<namespace>.
package temporal

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

const pollInterval = 300 * time.Millisecond

// Backend is a messaging.MessagingBackend backed by Temporal Workflows.
type Backend struct {
	client        client.Client
	worker        worker.Worker
	namespace     string
	taskQueueName string
	mailboxPrefix string
	clock         security.Clock

	closed    chan struct{}
	closeOnce sync.Once
}

// Option configures a Backend constructed by New.
type Option func(*options)

type options struct {
	clientOptions client.Options
	taskQueue     string
	mailboxPrefix string
	clock         security.Clock
}

// WithMailboxPrefix scopes every mailbox Workflow ID this Backend derives, so Backends sharing a namespace do not collide.
func WithMailboxPrefix(prefix string) Option {
	return func(o *options) { o.mailboxPrefix = prefix }
}

// WithClock overrides the clock used to default Envelope.CreatedAt in Emit.
func WithClock(clock security.Clock) Option {
	return func(o *options) { o.clock = clock }
}

// WithHostPort overrides the Temporal server address.
func WithHostPort(hostPort string) Option {
	return func(o *options) { o.clientOptions.HostPort = hostPort }
}

// WithNamespace overrides the Temporal namespace.
func WithNamespace(namespace string) Option {
	return func(o *options) { o.clientOptions.Namespace = namespace }
}

// WithTaskQueue overrides the task queue the Worker polls and mailbox workflows start on.
func WithTaskQueue(taskQueue string) Option {
	return func(o *options) { o.taskQueue = taskQueue }
}

// New connects to a Temporal server and starts a Worker hosting the mailbox workflow.
func New(opts ...Option) (*Backend, error) {
	o := &options{
		clientOptions: client.Options{
			HostPort:  client.DefaultHostPort,
			Namespace: client.DefaultNamespace,
		},
		taskQueue: TaskQueue,
		clock:     security.RealClock{},
	}
	for _, opt := range opts {
		opt(o)
	}

	c, err := client.Dial(o.clientOptions)
	if err != nil {
		return nil, fmt.Errorf("messaging/temporal: connect to Temporal at %q namespace %q: %w", o.clientOptions.HostPort, o.clientOptions.Namespace, err)
	}

	w := worker.New(c, o.taskQueue, worker.Options{})
	w.RegisterWorkflow(mailboxWorkflow)

	if err := w.Start(); err != nil {
		c.Close()
		return nil, fmt.Errorf("messaging/temporal: start worker on task queue %q: %w", o.taskQueue, err)
	}

	return &Backend{
		client:        c,
		worker:        w,
		namespace:     o.clientOptions.Namespace,
		taskQueueName: o.taskQueue,
		mailboxPrefix: o.mailboxPrefix,
		clock:         o.clock,
		closed:        make(chan struct{}),
	}, nil
}

func (b *Backend) workflowID(workerID string) string {
	if b.mailboxPrefix == "" {
		return workflowIDForWorker(workerID)
	}
	return workflowIDForWorker(b.mailboxPrefix + "-" + workerID)
}

// Emit implements messaging.MessagingBackend.
func (b *Backend) Emit(ctx context.Context, env messaging.Envelope) error {
	if env.CreatedAt.IsZero() {
		env.CreatedAt = b.clock.Now()
	}

	workflowID := b.workflowID(env.To)
	_, err := b.client.SignalWithStartWorkflow(
		ctx,
		workflowID,
		SignalDeliver,
		env,
		client.StartWorkflowOptions{
			ID:                       workflowID,
			TaskQueue:                b.taskQueueName,
			WorkflowExecutionTimeout: workflowStartToCloseTimeout,
		},
		mailboxWorkflow,
		mailboxState{},
	)
	if err != nil {
		return fmt.Errorf("messaging/temporal: emit to worker %q: %w", env.To, err)
	}
	return nil
}

// Subscribe implements messaging.MessagingBackend.
func (b *Backend) Subscribe(ctx context.Context, workerID string) (<-chan messaging.Envelope, error) {
	ch := make(chan messaging.Envelope)

	go func() {
		defer close(ch)

		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		deliveredIDs := make(map[string]struct{})

		for {
			select {
			case <-ctx.Done():
				return
			case <-b.closed:
				return
			case <-ticker.C:
				envs, err := b.queryPending(ctx, workerID)
				if err != nil {
					continue
				}

				forgetIDsNoLongerPending(deliveredIDs, envs)

				for _, env := range envs {
					if _, seen := deliveredIDs[env.ID]; seen {
						continue
					}
					deliveredIDs[env.ID] = struct{}{}

					select {
					case ch <- env:
					case <-ctx.Done():
						return
					case <-b.closed:
						return
					}
				}
			}
		}
	}()

	return ch, nil
}

func forgetIDsNoLongerPending(deliveredIDs map[string]struct{}, pending []messaging.Envelope) {
	stillPending := make(map[string]struct{}, len(pending))
	for _, env := range pending {
		stillPending[env.ID] = struct{}{}
	}
	for id := range deliveredIDs {
		if _, ok := stillPending[id]; !ok {
			delete(deliveredIDs, id)
		}
	}
}

// Ack implements messaging.MessagingBackend.
func (b *Backend) Ack(ctx context.Context, ack messaging.Ack) error {
	pending, err := b.queryPending(ctx, ack.ByWorker)
	if err != nil {
		if isNotFound(err) {
			return fmt.Errorf("messaging/temporal: ack %q for worker %q: %w", ack.MessageID, ack.ByWorker, messaging.ErrNotFound)
		}
		return fmt.Errorf("messaging/temporal: ack %q for worker %q: %w", ack.MessageID, ack.ByWorker, err)
	}
	held := false
	for _, env := range pending {
		if env.ID == ack.MessageID {
			held = true
			break
		}
	}
	if !held {
		return fmt.Errorf("messaging/temporal: ack %q for worker %q: %w", ack.MessageID, ack.ByWorker, messaging.ErrNotFound)
	}

	workflowID := b.workflowID(ack.ByWorker)
	err = b.client.SignalWorkflow(ctx, workflowID, "", SignalAck, ackSignal{MessageID: ack.MessageID})
	if err != nil {
		return fmt.Errorf("messaging/temporal: ack %q for worker %q: %w", ack.MessageID, ack.ByWorker, err)
	}
	return nil
}

// Tail implements messaging.MessagingBackend.
func (b *Backend) Tail(ctx context.Context, filter messaging.TailFilter) ([]messaging.Envelope, error) {
	envs, err := b.queryPending(ctx, filter.WorkerID)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("messaging/temporal: tail worker %q: %w", filter.WorkerID, err)
	}

	out := make([]messaging.Envelope, 0, len(envs))
	for _, env := range envs {
		if !filter.Since.IsZero() && env.CreatedAt.Before(filter.Since) {
			continue
		}
		out = append(out, env)
	}

	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[len(out)-filter.Limit:]
	}
	return out, nil
}

func (b *Backend) queryPending(ctx context.Context, workerID string) ([]messaging.Envelope, error) {
	resp, err := b.client.QueryWorkflow(ctx, b.workflowID(workerID), "", QueryPending)
	if err != nil {
		return nil, err
	}
	var envs []messaging.Envelope
	if err := resp.Get(&envs); err != nil {
		return nil, fmt.Errorf("decode query result: %w", err)
	}
	return envs, nil
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "workflow not found")
}

// Health implements messaging.MessagingBackend.
func (b *Backend) Health(ctx context.Context) error {
	if _, err := b.client.CheckHealth(ctx, &client.CheckHealthRequest{}); err != nil {
		return fmt.Errorf("messaging/temporal: health check: %w", err)
	}
	return nil
}

// Close implements messaging.MessagingBackend.
func (b *Backend) Close() error {
	b.closeOnce.Do(func() {
		close(b.closed)
		b.worker.Stop()
		b.client.Close()
	})
	return nil
}

func init() {
	messaging.Register("temporal", dispatch)
}

func dispatch(u *url.URL) (messaging.MessagingBackend, error) {
	if u.Host == "" {
		return nil, fmt.Errorf("messaging/temporal: temporal:// URI missing host:port, got %q", u.String())
	}
	namespace := strings.TrimPrefix(u.Path, "/")
	if namespace == "" {
		namespace = client.DefaultNamespace
	}

	opts := []Option{WithHostPort(u.Host), WithNamespace(namespace)}
	if tq := u.Query().Get("taskqueue"); tq != "" {
		opts = append(opts, WithTaskQueue(tq))
	}
	return New(opts...)
}
