package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/orgchart"
	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

type emitRequest struct {
	Type        string                    `json:"type"`
	To          string                    `json:"to,omitempty"`
	Subject     string                    `json:"subject,omitempty"`
	Body        json.RawMessage           `json:"body,omitempty"`
	StoragePtr  *messaging.StoragePointer `json:"storage_ptr,omitempty"`
	CausationID string                    `json:"causation_id,omitempty"`
}

type envelopeView struct {
	ID          string                    `json:"id"`
	Type        string                    `json:"type"`
	From        string                    `json:"from"`
	To          string                    `json:"to"`
	Subject     string                    `json:"subject,omitempty"`
	Body        json.RawMessage           `json:"body,omitempty"`
	StoragePtr  *messaging.StoragePointer `json:"storage_ptr,omitempty"`
	CreatedAt   time.Time                 `json:"created_at"`
	CausationID string                    `json:"causation_id,omitempty"`
	Delivered   bool                      `json:"delivered"`
}

func newEnvelopeView(env messaging.Envelope) envelopeView {
	return envelopeView{
		ID:          env.ID,
		Type:        string(env.Type),
		From:        env.From,
		To:          env.To,
		Subject:     env.Subject,
		Body:        env.Body,
		StoragePtr:  env.StoragePtr,
		CreatedAt:   env.CreatedAt,
		CausationID: env.CausationID,
		Delivered:   env.To != "",
	}
}

func (a *API) handleEmitMessage(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	if !requireMessageScope(w, claims, "message:send") {
		return
	}

	var req emitRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if messaging.MessageType(req.Type) == messaging.TypeScheduled {
		writeError(w, http.StatusBadRequest, "scheduled messages are sent by the control plane; register a schedule instead")
		return
	}

	env := messaging.Envelope{
		Type:        messaging.MessageType(req.Type),
		To:          req.To,
		Subject:     req.Subject,
		Body:        req.Body,
		StoragePtr:  req.StoragePtr,
		CausationID: req.CausationID,
	}

	sent, err := a.srv.EmitFromWorker(r.Context(), claims.WorkerID, env)
	switch {
	case err == nil:
	case errors.Is(err, server.ErrNoEscalationTarget):
		writeError(w, http.StatusConflict, "worker reports to root; there is no one to escalate to")
		return
	case errors.Is(err, server.ErrInvalidMessage):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, orgchart.ErrNotFound):
		writeError(w, http.StatusBadRequest, "sender or recipient is not a registered worker")
		return
	default:
		writeError(w, http.StatusInternalServerError, "could not emit message")
		return
	}

	writeJSON(w, http.StatusAccepted, newEnvelopeView(sent))
}

func requireMessageScope(w http.ResponseWriter, claims security.Claims, scope string) bool {
	if !claims.HasScope(scope) && !isAdmin(claims) {
		writeError(w, http.StatusForbidden, scope+" scope required")
		return false
	}
	return true
}

func (a *API) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	if !requireMessageScope(w, claims, "message:receive") {
		return
	}

	workerID := r.URL.Query().Get("worker_id")
	if workerID == "" {
		workerID = claims.WorkerID
	}
	if workerID != claims.WorkerID && !isAdmin(claims) {
		writeError(w, http.StatusForbidden, "cannot subscribe to another worker's channel without admin scope")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is not supported by this connection")
		return
	}

	ctx := r.Context()
	ch, err := a.srv.Messaging().Subscribe(ctx, workerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not subscribe")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case <-ctx.Done():
			return
		case env, ok := <-ch:
			if !ok {
				return
			}
			payload, err := json.Marshal(newEnvelopeView(env))
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

type ackRequest struct {
	Action string `json:"action"`
}

func (a *API) handleAck(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	id := r.PathValue("id")

	var req ackRequest
	if r.ContentLength != 0 {
		if !decodeJSON(w, r, &req) {
			return
		}
	}
	if req.Action == "" {
		req.Action = "read"
	}

	err := a.srv.Messaging().Ack(r.Context(), messaging.Ack{
		MessageID: id,
		ByWorker:  claims.WorkerID,
		Action:    req.Action,
		At:        a.clock.Now(),
	})
	switch {
	case err == nil:
	case errors.Is(err, messaging.ErrNotFound):
		writeError(w, http.StatusNotFound, "no such message")
		return
	default:
		writeError(w, http.StatusInternalServerError, "could not acknowledge message")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message_id": id,
		"by_worker":  claims.WorkerID,
		"action":     req.Action,
	})
}

func (a *API) handleTailMessages(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	if !requireMessageScope(w, claims, "message:receive") {
		return
	}

	q := r.URL.Query()
	workerID := q.Get("worker_id")
	if workerID == "" {
		workerID = claims.WorkerID
	}
	if workerID != claims.WorkerID && !isAdmin(claims) {
		writeError(w, http.StatusForbidden, "cannot read another worker's messages without admin scope")
		return
	}

	filter := messaging.TailFilter{WorkerID: workerID}
	if raw := q.Get("limit"); raw != "" {
		n, err := parsePositiveInt(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		filter.Limit = n
	}
	if raw := q.Get("since"); raw != "" {
		ts, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "since must be an RFC3339 timestamp")
			return
		}
		filter.Since = ts
	}

	envs, err := a.srv.Messaging().Tail(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read messages")
		return
	}

	out := make([]envelopeView, 0, len(envs))
	for _, env := range envs {
		out = append(out, newEnvelopeView(env))
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": out})
}
