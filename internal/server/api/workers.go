package api

import (
	"encoding/base64"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/orgchart"
	"github.com/acumen-ai-org/robotdreams/internal/server"
)

type workerView struct {
	ID          string            `json:"id"`
	Role        string            `json:"role"`
	ReportsTo   string            `json:"reports_to"`
	PublicKey   string            `json:"public_key,omitempty"`
	ConnectedAt time.Time         `json:"connected_at"`
	LastSeenAt  time.Time         `json:"last_seen_at"`
	Status      string            `json:"status"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func newWorkerView(w orgchart.Worker) workerView {
	v := workerView{
		ID:          w.ID,
		Role:        w.Role,
		ReportsTo:   w.ReportsTo,
		ConnectedAt: w.ConnectedAt,
		LastSeenAt:  w.LastSeenAt,
		Status:      string(w.Status),
		Metadata:    w.Metadata,
	}
	if len(w.PublicKey) > 0 {
		v.PublicKey = base64.StdEncoding.EncodeToString(w.PublicKey)
	}
	return v
}

func (a *API) handleListWorkers(w http.ResponseWriter, r *http.Request) {
	workers := a.srv.Graph().List()
	sort.Slice(workers, func(i, j int) bool { return workers[i].ID < workers[j].ID })

	out := make([]workerView, 0, len(workers))
	for _, wk := range workers {
		out = append(out, newWorkerView(wk))
	}
	writeJSON(w, http.StatusOK, map[string]any{"workers": out})
}

func (a *API) handleGetWorker(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	worker, err := a.srv.Graph().Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "no such worker")
		return
	}
	writeJSON(w, http.StatusOK, newWorkerView(worker))
}

type editWorkerRequest struct {
	Role *string `json:"role"`
}

func (a *API) handleEditWorker(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	id := r.PathValue("id")

	var req editWorkerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Role == nil {
		writeError(w, http.StatusBadRequest, "no editable field given")
		return
	}

	err := a.srv.SetWorkerRole(r.Context(), id, *req.Role, claims.WorkerID, isAdmin(claims))
	switch {
	case err == nil:
	case errors.Is(err, server.ErrNotAuthorized):
		writeError(w, http.StatusForbidden, "not authorized to edit this worker")
		return
	case errors.Is(err, orgchart.ErrNotFound):
		writeError(w, http.StatusNotFound, "no such worker")
		return
	default:
		writeError(w, http.StatusInternalServerError, "could not edit worker")
		return
	}

	worker, err := a.srv.Graph().Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "worker disappeared during edit")
		return
	}
	writeJSON(w, http.StatusOK, newWorkerView(worker))
}

type reassignRequest struct {
	ReportsTo string `json:"reports_to"`
}

func (a *API) handleReassign(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	id := r.PathValue("id")

	var req reassignRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	err := a.srv.ReassignWorker(r.Context(), id, req.ReportsTo, claims.WorkerID, isAdmin(claims))
	switch {
	case err == nil:
	case errors.Is(err, server.ErrNotAuthorized):
		writeError(w, http.StatusForbidden, "not authorized to reassign this worker")
		return
	case errors.Is(err, orgchart.ErrNotFound):
		writeError(w, http.StatusNotFound, "no such worker")
		return
	case errors.Is(err, orgchart.ErrParentNotFound):
		writeError(w, http.StatusBadRequest, "reports_to is not a registered worker")
		return
	case errors.Is(err, orgchart.ErrCycle):
		writeError(w, http.StatusConflict, "reassignment would create a cycle")
		return
	default:
		writeError(w, http.StatusInternalServerError, "could not reassign worker")
		return
	}

	worker, err := a.srv.Graph().Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "worker disappeared during reassignment")
		return
	}
	writeJSON(w, http.StatusOK, newWorkerView(worker))
}

type revokeRequest struct {
	Reason string `json:"reason"`
}

func (a *API) handleRevoke(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	if !isAdmin(claims) {
		writeError(w, http.StatusForbidden, "admin scope required")
		return
	}

	id := r.PathValue("id")

	var req revokeRequest
	if !decodeOptionalJSON(w, r, &req) {
		return
	}
	if req.Reason == "" {
		req.Reason = "revoked by " + claims.WorkerID
	}

	if err := a.srv.Revocations().Revoke(r.Context(), id, req.Reason); err != nil {
		writeError(w, http.StatusInternalServerError, "could not revoke worker")
		return
	}
	a.revCache.invalidate(id)

	writeJSON(w, http.StatusOK, map[string]any{
		"worker_id": id,
		"revoked":   true,
		"reason":    req.Reason,
	})
}
