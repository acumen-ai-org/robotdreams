package webhook

import (
	"context"
	"sync"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

// RetryPolicy is the exponential backoff schedule applied when a webhook POST fails, giving up after MaxAttempts.
type RetryPolicy struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	MaxAttempts  int
}

// DefaultRetryPolicy returns the backoff schedule 1s, 2s, 4s, 8s, 16s, capped at 30s, giving up after 5 attempts.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2,
		MaxAttempts:  5,
	}
}

func (p RetryPolicy) delayFor(attempt int) time.Duration {
	d := p.InitialDelay
	for i := 1; i < attempt; i++ {
		d = time.Duration(float64(d) * p.Multiplier)
		if d > p.MaxDelay {
			d = p.MaxDelay
			break
		}
	}
	if d > p.MaxDelay {
		d = p.MaxDelay
	}
	return d
}

const maxRetainedTerminal = 1000

type outboxItem struct {
	env         messaging.Envelope
	attempts    int
	nextAttempt time.Time
	delivered   bool
	failed      bool
	lastErr     error
}

type snapshot struct {
	Attempts    int
	NextAttempt time.Time
	Delivered   bool
	Failed      bool
	LastErr     error
}

type outbox struct {
	mu          sync.Mutex
	itemsByID   map[string]*outboxItem
	terminalIDs []string

	clock  security.Clock
	policy RetryPolicy
	poll   time.Duration

	deliver     func(ctx context.Context, env messaging.Envelope) error
	onDelivered func(env messaging.Envelope)

	closeCh chan struct{}
	closeOn sync.Once
	wg      sync.WaitGroup
}

func newOutbox(clock security.Clock, policy RetryPolicy, poll time.Duration, deliver func(ctx context.Context, env messaging.Envelope) error, onDelivered func(env messaging.Envelope)) *outbox {
	return &outbox{
		itemsByID:   make(map[string]*outboxItem),
		clock:       clock,
		policy:      policy,
		poll:        poll,
		deliver:     deliver,
		onDelivered: onDelivered,
		closeCh:     make(chan struct{}),
	}
}

func (o *outbox) start() {
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		ticker := time.NewTicker(o.poll)
		defer ticker.Stop()
		for {
			select {
			case <-o.closeCh:
				return
			case <-ticker.C:
				o.tick(context.Background())
			}
		}
	}()
}

func (o *outbox) close() {
	o.closeOn.Do(func() { close(o.closeCh) })
	o.wg.Wait()
}

func (o *outbox) enqueueFailed(env messaging.Envelope, attempts int, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	item := &outboxItem{
		env:      env,
		attempts: attempts,
		lastErr:  err,
	}
	if attempts >= o.policy.MaxAttempts {
		item.failed = true
	} else {
		item.nextAttempt = o.clock.Now().Add(o.policy.delayFor(attempts))
	}
	o.itemsByID[env.ID] = item
	if item.failed {
		o.retainTerminalLocked(env.ID)
	}
}

func (o *outbox) retainTerminalLocked(id string) {
	o.terminalIDs = append(o.terminalIDs, id)
	for len(o.terminalIDs) > maxRetainedTerminal {
		oldest := o.terminalIDs[0]
		o.terminalIDs = o.terminalIDs[1:]
		if item, ok := o.itemsByID[oldest]; ok && (item.delivered || item.failed) {
			delete(o.itemsByID, oldest)
		}
	}
}

func (o *outbox) tick(ctx context.Context) {
	now := o.clock.Now()

	var due []*outboxItem
	o.mu.Lock()
	for _, item := range o.itemsByID {
		if item.delivered || item.failed {
			continue
		}
		if !now.Before(item.nextAttempt) {
			due = append(due, item)
		}
	}
	o.mu.Unlock()

	for _, item := range due {
		err := o.deliver(ctx, item.env)

		o.mu.Lock()
		item.attempts++
		item.lastErr = err
		if err == nil {
			item.delivered = true
		} else if item.attempts >= o.policy.MaxAttempts {
			item.failed = true
		} else {
			item.nextAttempt = o.clock.Now().Add(o.policy.delayFor(item.attempts))
		}
		if item.delivered || item.failed {
			o.retainTerminalLocked(item.env.ID)
		}
		delivered := item.delivered
		o.mu.Unlock()

		if delivered && o.onDelivered != nil {
			o.onDelivered(item.env)
		}
	}
}

func (o *outbox) inspect(id string) (snapshot, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	item, ok := o.itemsByID[id]
	if !ok {
		return snapshot{}, false
	}
	return snapshot{
		Attempts:    item.attempts,
		NextAttempt: item.nextAttempt,
		Delivered:   item.delivered,
		Failed:      item.failed,
		LastErr:     item.lastErr,
	}, true
}

func (o *outbox) stats() (pending, failed int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, item := range o.itemsByID {
		switch {
		case item.failed:
			failed++
		case !item.delivered:
			pending++
		}
	}
	return pending, failed
}
