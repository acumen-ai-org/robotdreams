package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/orgchart"
	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
)

const pollInterval = 2 * time.Second

type sseEvent struct {
	Type string
	Data any
}

type eventHub struct {
	mu          sync.Mutex
	subs        map[int]chan sseEvent
	nextID      int
	cancelPoll  context.CancelFunc
	pollStopped chan struct{}
	poll        pollFunc
}

type pollFunc func(ctx context.Context) []sseEvent

func newEventHub(poll pollFunc) *eventHub {
	return &eventHub{
		subs: make(map[int]chan sseEvent),
		poll: poll,
	}
}

func (h *eventHub) subscribe() (<-chan sseEvent, func()) {
	h.mu.Lock()
	id := h.nextID
	h.nextID++
	ch := make(chan sseEvent, 64)
	h.subs[id] = ch
	starting := len(h.subs) == 1
	h.mu.Unlock()

	if starting {
		h.startPoll()
	}

	unsubscribe := func() {
		h.mu.Lock()
		delete(h.subs, id)
		empty := len(h.subs) == 0
		var stop context.CancelFunc
		if empty && h.cancelPoll != nil {
			stop = h.cancelPoll
			h.cancelPoll = nil
		}
		h.mu.Unlock()
		if stop != nil {
			stop()
			<-h.pollStopped
		}
	}
	return ch, unsubscribe
}

func (h *eventHub) startPoll() {
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})

	h.mu.Lock()
	h.cancelPoll = cancel
	h.pollStopped = stopped
	h.mu.Unlock()

	go func() {
		defer close(stopped)
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, ev := range h.poll(ctx) {
					h.broadcast(ev)
				}
			}
		}
	}()
}

func (h *eventHub) broadcast(ev sseEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (h *eventHub) subscriberCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

type orgSnapshot struct {
	role      string
	reportsTo string
	status    orgchart.Status
}

type serverPollState struct {
	srv             *serverDeps
	mu              sync.Mutex
	workers         map[string]orgSnapshot
	objectRevisions map[string]string
	sinceMessages   time.Time
	initialized     bool
}

type serverDeps struct {
	Graph    func() *orgchart.Graph
	ListObjs func(ctx context.Context, prefix string) ([]objectMetaView, error)
	TailMsgs func(ctx context.Context, filter messaging.TailFilter) ([]messaging.Envelope, error)
}

func newServerPollState(deps *serverDeps) *serverPollState {
	return &serverPollState{
		srv:             deps,
		workers:         make(map[string]orgSnapshot),
		objectRevisions: make(map[string]string),
	}
}

func (s *serverPollState) poll(ctx context.Context) []sseEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	var events []sseEvent
	events = append(events, s.pollWorkersLocked()...)
	events = append(events, s.pollStorageLocked(ctx)...)
	events = append(events, s.pollMessagesLocked(ctx)...)
	s.initialized = true
	return events
}

func (s *serverPollState) pollWorkersLocked() []sseEvent {
	var events []sseEvent
	current := s.srv.Graph().List()
	seen := make(map[string]bool, len(current))

	for _, w := range current {
		seen[w.ID] = true
		prev, existed := s.workers[w.ID]
		snap := orgSnapshot{role: w.Role, reportsTo: w.ReportsTo, status: w.Status}
		if existed && prev.role != snap.role {
			events = append(events, sseEvent{Type: "worker_role_changed", Data: map[string]any{
				"worker_id": w.ID,
				"old_role":  prev.role,
				"new_role":  snap.role,
			}})
		}
		switch {
		case !existed:
			if s.initialized {
				events = append(events, sseEvent{Type: "worker_connected", Data: map[string]any{
					"worker_id":  w.ID,
					"role":       w.Role,
					"reports_to": w.ReportsTo,
					"status":     string(w.Status),
				}})
			}
		case prev.reportsTo != snap.reportsTo:
			events = append(events, sseEvent{Type: "worker_reassigned", Data: map[string]any{
				"worker_id":      w.ID,
				"old_reports_to": prev.reportsTo,
				"new_reports_to": snap.reportsTo,
			}})
			fallthrough
		case prev.status != snap.status:
			events = append(events, sseEvent{Type: "worker_status_changed", Data: map[string]any{
				"worker_id": w.ID,
				"status":    string(w.Status),
			}})
		}
		s.workers[w.ID] = snap
	}

	for id := range s.workers {
		if !seen[id] {
			delete(s.workers, id)
			if s.initialized {
				events = append(events, sseEvent{Type: "worker_disconnected", Data: map[string]any{
					"worker_id": id,
				}})
			}
		}
	}
	return events
}

func (s *serverPollState) pollStorageLocked(ctx context.Context) []sseEvent {
	metas, err := s.srv.ListObjs(ctx, "")
	if err != nil {
		return nil
	}
	var events []sseEvent
	seen := make(map[string]bool, len(metas))
	for _, m := range metas {
		seen[m.Path] = true
		if prevRev, existed := s.objectRevisions[m.Path]; !existed || prevRev != m.Revision {
			if s.initialized {
				events = append(events, sseEvent{Type: "storage_write", Data: m})
			}
		}
		s.objectRevisions[m.Path] = m.Revision
	}
	for path := range s.objectRevisions {
		if !seen[path] {
			delete(s.objectRevisions, path)
			if s.initialized {
				events = append(events, sseEvent{Type: "storage_delete", Data: map[string]any{"path": path}})
			}
		}
	}
	return events
}

type volumeKey struct{ from, to string }

func (s *serverPollState) pollMessagesLocked(ctx context.Context) []sseEvent {
	envs, err := s.srv.TailMsgs(ctx, messaging.TailFilter{Since: s.sinceMessages})
	if err != nil {
		return nil
	}

	var events []sseEvent
	volumes := make(map[volumeKey]int)
	newWatermark := s.sinceMessages

	for _, env := range envs {
		if !env.CreatedAt.After(s.sinceMessages) {
			continue
		}
		if env.CreatedAt.After(newWatermark) {
			newWatermark = env.CreatedAt
		}

		if env.From == server.ControlWorkerID && env.Subject == server.SubjectUpdateAvailable {
			continue
		}

		switch env.Type {
		case messaging.TypeStatusUpdate:
			volumes[volumeKey{from: env.From, to: env.To}]++
		default:
			events = append(events, sseEvent{Type: "message", Data: newEnvelopeView(env)})
		}
	}
	s.sinceMessages = newWatermark

	keys := make([]volumeKey, 0, len(volumes))
	for k := range volumes {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].from != keys[j].from {
			return keys[i].from < keys[j].from
		}
		return keys[i].to < keys[j].to
	})
	for _, k := range keys {
		events = append(events, sseEvent{Type: "message_volume", Data: map[string]any{
			"from":           k.from,
			"to":             k.to,
			"message_type":   string(messaging.TypeStatusUpdate),
			"count":          volumes[k],
			"window_seconds": int(pollInterval / time.Second),
		}})
	}
	return events
}

func (a *API) handleEvents(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	if !isAdmin(claims) {
		writeError(w, http.StatusForbidden, "admin scope required to stream org-wide events")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is not supported by this connection")
		return
	}

	ch, unsubscribe := a.events.subscribe()
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			payload, err := json.Marshal(ev.Data)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, payload); err != nil {
				return
			}
			flusher.Flush()
		case <-keepalive.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
