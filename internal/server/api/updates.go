package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/acumen-ai-org/robotdreams/internal/orgchart"
	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/internal/server/store"
	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

const defaultUpdateListLimit = 50

type announceRequest struct {
	Kind       string `json:"kind"`
	Version    string `json:"version"`
	Source     string `json:"source,omitempty"`
	MinVersion string `json:"min_version,omitempty"`
	Severity   string `json:"severity,omitempty"`
	Notes      string `json:"notes,omitempty"`
}

type announceResponse struct {
	Announcement updates.Announcement `json:"announcement"`
	Recipients   int                  `json:"recipients"`
	Delivered    int                  `json:"delivered"`
	Warning      string               `json:"warning,omitempty"`
}

type updateReportRequest struct {
	Kind           string `json:"kind"`
	AnnouncementID string `json:"announcement_id,omitempty"`
	Status         string `json:"status"`
	CurrentVersion string `json:"current_version,omitempty"`
	TargetVersion  string `json:"target_version,omitempty"`
	Detail         string `json:"detail,omitempty"`
}

func (a *API) handleAnnounceUpdate(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	if !isAdmin(claims) {
		writeError(w, http.StatusForbidden, "admin scope required to announce an update")
		return
	}

	var req announceRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	ann, delivered, err := a.srv.AnnounceUpdate(r.Context(), updates.Announcement{
		Kind:       req.Kind,
		Version:    req.Version,
		Source:     req.Source,
		MinVersion: req.MinVersion,
		Severity:   req.Severity,
		Notes:      req.Notes,
	}, claims.WorkerID)

	resp := announceResponse{Announcement: ann, Delivered: delivered}
	switch {
	case errors.Is(err, server.ErrInvalidUpdate):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case err != nil && ann.ID == "":
		writeError(w, http.StatusInternalServerError, "could not announce update")
		return
	case err != nil:
		resp.Warning = err.Error()
	}

	rollout, rErr := a.srv.UpdateRollout(r.Context(), ann.Kind, ann.ID)
	if rErr == nil {
		resp.Recipients = len(rollout.Nodes)
	} else {
		resp.Recipients = delivered
	}

	a.events.broadcast(sseEvent{Type: "update_announced", Data: map[string]any{
		"announcement_id": ann.ID,
		"kind":            ann.Kind,
		"version":         ann.Version,
		"severity":        ann.Severity,
		"announced_by":    ann.AnnouncedBy,
		"announced_at":    ann.AnnouncedAt,
		"recipients":      resp.Recipients,
	}})

	writeJSON(w, http.StatusAccepted, resp)
}

func (a *API) handleListUpdates(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	limit := defaultUpdateListLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "limit must be a non-negative integer")
			return
		}
		limit = n
	}

	anns, err := a.srv.LatestUpdateAnnouncements(r.Context(), kind, limit)
	if errors.Is(err, server.ErrInvalidUpdate) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list updates")
		return
	}
	if anns == nil {
		anns = []updates.Announcement{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"announcements": anns})
}

func (a *API) handlePendingUpdates(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())

	workerID := r.URL.Query().Get("worker_id")
	if workerID == "" {
		workerID = claims.WorkerID
	}
	if workerID != claims.WorkerID && !isAdmin(claims) {
		writeError(w, http.StatusForbidden, "cannot read another worker's pending updates without admin scope")
		return
	}

	pending, err := a.srv.PendingUpdatesFor(r.Context(), workerID)
	if errors.Is(err, orgchart.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "not a registered worker")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read pending updates")
		return
	}
	if pending == nil {
		pending = []updates.Pending{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"pending": pending})
}

func (a *API) handleUpdateReport(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())

	var req updateReportRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	err := a.srv.RecordUpdateState(r.Context(), claims.WorkerID, updates.Report{
		Kind:           req.Kind,
		AnnouncementID: req.AnnouncementID,
		Status:         req.Status,
		CurrentVersion: req.CurrentVersion,
		TargetVersion:  req.TargetVersion,
		Detail:         req.Detail,
	})
	switch {
	case errors.Is(err, server.ErrInvalidUpdate):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, orgchart.ErrNotFound):
		writeError(w, http.StatusBadRequest, "not a registered worker")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not record update state")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"recorded": true})
}

func (a *API) handleUpdateRollout(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	if !isAdmin(claims) {
		writeError(w, http.StatusForbidden, "admin scope required to read a rollout")
		return
	}

	kind := r.URL.Query().Get("kind")
	if kind == "" {
		writeError(w, http.StatusBadRequest, "kind is required")
		return
	}

	rollout, err := a.srv.UpdateRollout(r.Context(), kind, r.URL.Query().Get("announcement_id"))
	switch {
	case errors.Is(err, server.ErrInvalidUpdate):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "no such announcement")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not assemble rollout")
		return
	}

	writeJSON(w, http.StatusOK, rollout)
}
