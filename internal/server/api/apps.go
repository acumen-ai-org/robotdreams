package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/orgchart"
	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/internal/server/store"
)

type appView struct {
	WorkerID    string `json:"worker_id"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	DeclaredAt  string `json:"declared_at"`
}

func newAppView(a store.WorkerApp) appView {
	return appView{
		WorkerID:    a.WorkerID,
		URL:         a.URL,
		Description: a.Description,
		DeclaredAt:  a.DeclaredAt.UTC().Format(time.RFC3339),
	}
}

type setAppRequest struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

func (a *API) handleSetApp(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())

	var req setAppRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	app, err := a.srv.SetWorkerApp(r.Context(), claims.WorkerID, req.URL, req.Description)
	switch {
	case errors.Is(err, server.ErrInvalidApp):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, orgchart.ErrNotFound):
		writeError(w, http.StatusBadRequest, "not a registered worker")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not record the app")
		return
	}

	writeJSON(w, http.StatusOK, newAppView(app))
}

func (a *API) handleClearApp(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())

	err := a.srv.ClearWorkerApp(r.Context(), claims.WorkerID)
	switch {
	case errors.Is(err, orgchart.ErrNotFound):
		writeError(w, http.StatusBadRequest, "not a registered worker")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not clear the app")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"cleared": true})
}

func (a *API) handleListApps(w http.ResponseWriter, r *http.Request) {
	apps, err := a.srv.WorkerApps(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list apps")
		return
	}

	out := make([]appView, 0, len(apps))
	for _, app := range apps {
		out = append(out, newAppView(app))
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": out})
}
