package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/orgchart"
	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/pkg/scheduling"
	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

type scheduleRequest struct {
	Owner   string          `json:"owner,omitempty"`
	To      string          `json:"to,omitempty"`
	Cron    string          `json:"cron"`
	Subject string          `json:"subject,omitempty"`
	Body    json.RawMessage `json:"body,omitempty"`
	Enabled *bool           `json:"enabled,omitempty"`
}

type scheduleView struct {
	ID        string          `json:"id"`
	Owner     string          `json:"owner"`
	To        string          `json:"to"`
	Cron      string          `json:"cron"`
	Subject   string          `json:"subject,omitempty"`
	Body      json.RawMessage `json:"body,omitempty"`
	Enabled   bool            `json:"enabled"`
	NextAt    time.Time       `json:"next_at"`
	LastAt    *time.Time      `json:"last_at,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

func newScheduleView(s scheduling.Schedule) scheduleView {
	return scheduleView{
		ID: s.ID, Owner: s.Owner, To: s.To, Cron: s.Cron, Subject: s.Subject, Body: s.Body,
		Enabled: s.Enabled, NextAt: s.NextAt, LastAt: s.LastAt, CreatedAt: s.CreatedAt,
	}
}

type scheduleFiredView struct {
	ScheduleID string    `json:"schedule_id"`
	Owner      string    `json:"owner"`
	To         string    `json:"to"`
	Subject    string    `json:"subject,omitempty"`
	MessageID  string    `json:"message_id"`
	FiredAt    time.Time `json:"fired_at"`
	NextAt     time.Time `json:"next_at"`
}

func newScheduleFiredView(f server.ScheduleFired) scheduleFiredView {
	return scheduleFiredView{
		ScheduleID: f.Schedule.ID, Owner: f.Schedule.Owner, To: f.Schedule.To, Subject: f.Schedule.Subject,
		MessageID: f.Envelope.ID, FiredAt: f.FiredAt, NextAt: f.NextAt,
	}
}

func (a *API) broadcastScheduleFired(f server.ScheduleFired) {
	a.events.broadcast(sseEvent{Type: "schedule_fired", Data: newScheduleFiredView(f)})
}

func (a *API) RunScheduler(ctx context.Context, interval time.Duration) {
	server.RunScheduler(ctx, a.srv, a.clock, interval, a.broadcastScheduleFired)
}

func (a *API) handlePutSchedule(w http.ResponseWriter, r *http.Request) {
	a.upsertSchedule(w, r, r.PathValue("id"))
}

func (a *API) handlePostSchedule(w http.ResponseWriter, r *http.Request) {
	a.upsertSchedule(w, r, server.NewID())
}

func (a *API) upsertSchedule(w http.ResponseWriter, r *http.Request, id string) {
	claims, _ := ClaimsFromContext(r.Context())
	if !a.authorizeScheduleWrite(w, claims) {
		return
	}

	var req scheduleRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	owner := claims.WorkerID
	if req.Owner != "" && req.Owner != claims.WorkerID {
		if !isAdmin(claims) {
			writeError(w, http.StatusForbidden, "only an admin may register a schedule for another worker")
			return
		}
		owner = req.Owner
	}
	if _, err := a.srv.Graph().Get(owner); err != nil {
		if isAdmin(claims) {
			writeError(w, http.StatusBadRequest, "owner is not a registered worker")
		} else {
			writeError(w, http.StatusForbidden, "only a registered worker may register a schedule")
		}
		return
	}
	to := req.To
	if to == "" {
		to = owner
	}
	if _, err := a.srv.Graph().Get(to); err != nil {
		writeError(w, http.StatusBadRequest, "to is not a registered worker")
		return
	}

	if existing, err := a.srv.Schedules().Get(r.Context(), id); err == nil {
		if existing.Owner != claims.WorkerID && !isAdmin(claims) {
			writeError(w, http.StatusForbidden, "schedule is owned by another worker")
			return
		}
	} else if !errors.Is(err, scheduling.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "could not read schedule")
		return
	}

	next, err := scheduling.ParseCron(req.Cron)
	if err != nil {
		writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), "scheduling: invalid schedule: "))
		return
	}
	now := a.clock.Now()
	nextAt := next(now)
	if nextAt.IsZero() {
		writeError(w, http.StatusBadRequest, "cron expression never matches")
		return
	}

	s := scheduling.Schedule{
		ID: id, Owner: owner, To: to, Cron: strings.TrimSpace(req.Cron), Subject: req.Subject, Body: req.Body,
		Enabled: req.Enabled == nil || *req.Enabled, NextAt: nextAt, CreatedAt: now,
	}
	if err := s.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), "scheduling: invalid schedule: "))
		return
	}
	if err := a.srv.Schedules().Upsert(r.Context(), s); err != nil {
		writeError(w, http.StatusInternalServerError, "could not store schedule")
		return
	}
	stored, err := a.srv.Schedules().Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read schedule back")
		return
	}
	writeJSON(w, http.StatusOK, newScheduleView(stored))
}

func (a *API) authorizeScheduleWrite(w http.ResponseWriter, claims security.Claims) bool {
	if claims.Delegated() {
		writeError(w, http.StatusForbidden, "a delegated child cannot register schedules")
		return false
	}
	if !claims.HasScope("message:send") && !isAdmin(claims) {
		writeError(w, http.StatusForbidden, "message:send scope required")
		return false
	}
	return true
}

func (a *API) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	workerID := r.URL.Query().Get("worker_id")

	list, err := a.srv.Schedules().List(r.Context(), scheduling.Filter{Worker: workerID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list schedules")
		return
	}
	out := make([]scheduleView, 0, len(list))
	for _, s := range list {
		out = append(out, newScheduleView(s))
	}
	writeJSON(w, http.StatusOK, map[string]any{"schedules": out})
}

func (a *API) handleGetSchedule(w http.ResponseWriter, r *http.Request) {
	s, err := a.srv.Schedules().Get(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, scheduling.ErrNotFound) {
			writeError(w, http.StatusNotFound, "no such schedule")
		} else {
			writeError(w, http.StatusInternalServerError, "could not read schedule")
		}
		return
	}
	writeJSON(w, http.StatusOK, newScheduleView(s))
}

func (a *API) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	s, ok := a.loadOwnedSchedule(w, r, claims)
	if !ok {
		return
	}
	if err := a.srv.Schedules().Delete(r.Context(), s.ID); err != nil {
		if errors.Is(err, scheduling.ErrNotFound) {
			writeError(w, http.StatusNotFound, "no such schedule")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not delete schedule")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": s.ID, "deleted": true})
}

func (a *API) handleFireSchedule(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	s, ok := a.loadOwnedSchedule(w, r, claims)
	if !ok {
		return
	}

	env, err := a.srv.FireSchedule(r.Context(), s)
	switch {
	case err == nil:
	case errors.Is(err, orgchart.ErrNotFound):
		writeError(w, http.StatusConflict, "owner or recipient is no longer a registered worker")
		return
	case errors.Is(err, server.ErrInvalidMessage):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	default:
		writeError(w, http.StatusInternalServerError, "could not send message")
		return
	}

	now := a.clock.Now()
	if err := a.srv.Schedules().MarkFired(r.Context(), s.ID, now, s.NextAt); err != nil {
		writeError(w, http.StatusInternalServerError, "sent, but could not record the fire")
		return
	}
	fired := server.ScheduleFired{Schedule: s, Envelope: env, FiredAt: now, NextAt: s.NextAt}
	a.broadcastScheduleFired(fired)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"fired":   newScheduleFiredView(fired),
		"message": newEnvelopeView(env),
	})
}

func (a *API) loadOwnedSchedule(w http.ResponseWriter, r *http.Request, claims security.Claims) (scheduling.Schedule, bool) {
	s, err := a.srv.Schedules().Get(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, scheduling.ErrNotFound) {
			writeError(w, http.StatusNotFound, "no such schedule")
		} else {
			writeError(w, http.StatusInternalServerError, "could not read schedule")
		}
		return scheduling.Schedule{}, false
	}
	if !isAdmin(claims) && s.Owner != claims.WorkerID {
		writeError(w, http.StatusForbidden, "not the owner of this schedule")
		return scheduling.Schedule{}, false
	}
	return s, true
}
