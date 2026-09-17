package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
	"github.com/acumen-ai-org/robotdreams/pkg/scheduling"
	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

type Scheduler struct {
	srv   *Server
	clock security.Clock
	log   *log.Logger

	onFired func(ScheduleFired)
}

type ScheduleFired struct {
	Schedule scheduling.Schedule
	Envelope messaging.Envelope
	FiredAt  time.Time
	NextAt   time.Time
}

func NewScheduler(srv *Server, clock security.Clock, onFired func(ScheduleFired)) *Scheduler {
	if clock == nil {
		clock = srv.clock
	}
	return &Scheduler{srv: srv, clock: clock, log: log.Default(), onFired: onFired}
}

func (sc *Scheduler) SetLogger(l *log.Logger) {
	if l != nil {
		sc.log = l
	}
}

func RunScheduler(ctx context.Context, srv *Server, clock security.Clock, interval time.Duration, onFired func(ScheduleFired)) {
	NewScheduler(srv, clock, onFired).Run(ctx, interval)
}

const defaultTickInterval = 30 * time.Second

func (sc *Scheduler) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultTickInterval
	}
	if err := sc.Start(ctx); err != nil {
		sc.log.Printf("scheduler: start: %v", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := sc.Tick(ctx); err != nil {
				sc.log.Printf("scheduler: tick: %v", err)
			}
		}
	}
}

func (sc *Scheduler) Start(ctx context.Context) error {
	now := sc.clock.Now()
	due, err := sc.srv.Schedules().ListDue(ctx, now)
	if err != nil {
		return err
	}
	for _, s := range due {
		next, err := nextAfter(s, now)
		if err != nil {
			sc.log.Printf("scheduler: schedule %s: %v (left as is)", s.ID, err)
			continue
		}
		if err := sc.srv.Schedules().MarkFired(ctx, s.ID, lastFiredAt(s), next); err != nil {
			sc.log.Printf("scheduler: schedule %s: advance: %v", s.ID, err)
			continue
		}
		sc.log.Printf("scheduler: schedule %s was due at %s while the server was down; not fired, next at %s",
			s.ID, s.NextAt.Format(time.RFC3339), next.Format(time.RFC3339))
	}
	return nil
}

func (sc *Scheduler) Tick(ctx context.Context) (int, error) {
	now := sc.clock.Now()
	due, err := sc.srv.Schedules().ListDue(ctx, now)
	if err != nil {
		return 0, err
	}
	fired := 0
	for _, s := range due {
		next, err := nextAfter(s, now)
		if err != nil {
			sc.log.Printf("scheduler: schedule %s: %v (skipped)", s.ID, err)
			continue
		}
		env, emitErr := sc.srv.FireSchedule(ctx, s)
		lastAt := now
		if emitErr != nil {
			sc.log.Printf("scheduler: schedule %s: fire: %v", s.ID, emitErr)
			lastAt = lastFiredAt(s)
		}
		if err := sc.srv.Schedules().MarkFired(ctx, s.ID, lastAt, next); err != nil {
			sc.log.Printf("scheduler: schedule %s: advance: %v", s.ID, err)
			continue
		}
		if emitErr != nil {
			continue
		}
		fired++
		if sc.onFired != nil {
			sc.onFired(ScheduleFired{Schedule: s, Envelope: env, FiredAt: now, NextAt: next})
		}
	}
	return fired, nil
}

func lastFiredAt(s scheduling.Schedule) time.Time {
	if s.LastAt == nil {
		return time.Time{}
	}
	return *s.LastAt
}

func nextAfter(s scheduling.Schedule, now time.Time) (time.Time, error) {
	next, err := scheduling.ParseCron(s.Cron)
	if err != nil {
		return time.Time{}, err
	}
	at := next(now)
	if at.IsZero() {
		return time.Time{}, fmt.Errorf("cron %q has no next occurrence after %s", s.Cron, now.Format(time.RFC3339))
	}
	return at, nil
}

func (s *Server) FireSchedule(ctx context.Context, sc scheduling.Schedule) (messaging.Envelope, error) {
	env := messaging.Envelope{
		Type:        messaging.TypeScheduled,
		To:          sc.To,
		Subject:     sc.Subject,
		Body:        sc.Body,
		CausationID: sc.ID,
	}
	return s.EmitFromWorker(ctx, sc.Owner, env)
}
