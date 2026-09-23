package api

import (
	"net/http"

	"github.com/acumen-ai-org/robotdreams/internal/server"
)

func NewRouter(srv *server.Server) http.Handler {
	return NewAPIRouter(New(srv))
}

func NewAPIRouter(a *API) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/workers/challenge", a.handleChallenge)
	mux.HandleFunc("POST /api/workers", a.handleConnect)
	mux.HandleFunc("POST /api/token", a.handleToken)
	mux.HandleFunc("GET /api/.well-known/jwks.json", a.handleJWKS)

	mux.HandleFunc("GET /api/workers", a.withAuth(a.handleListWorkers))
	mux.HandleFunc("GET /api/workers/{id}", a.withAuth(a.handleGetWorker))
	mux.HandleFunc("PATCH /api/workers/{id}", a.withAuth(a.handleEditWorker))
	mux.HandleFunc("POST /api/workers/{id}/reassign", a.withAuth(a.handleReassign))
	mux.HandleFunc("DELETE /api/workers/{id}", a.withAuth(a.handleDeleteWorker))
	mux.HandleFunc("POST /api/workers/{id}/revoke", a.withAuth(a.handleRevoke))
	mux.HandleFunc("POST /api/workers/delegate", a.withAuth(a.handleDelegate))

	mux.HandleFunc("POST /api/messages", a.withAuth(a.handleEmitMessage))
	mux.HandleFunc("GET /api/messages", a.withAuth(a.handleTailMessages))
	mux.HandleFunc("GET /api/messages/subscribe", a.withAuth(a.handleSubscribe))
	mux.HandleFunc("POST /api/messages/{id}/ack", a.withAuth(a.handleAck))

	mux.HandleFunc("PUT /api/schedules/{id}", a.withAuth(a.handlePutSchedule))
	mux.HandleFunc("POST /api/schedules", a.withAuth(a.handlePostSchedule))
	mux.HandleFunc("GET /api/schedules", a.withAuth(a.handleListSchedules))
	mux.HandleFunc("GET /api/schedules/{id}", a.withAuth(a.handleGetSchedule))
	mux.HandleFunc("DELETE /api/schedules/{id}", a.withAuth(a.handleDeleteSchedule))
	mux.HandleFunc("POST /api/schedules/{id}/fire", a.withAuth(a.handleFireSchedule))

	mux.HandleFunc("POST /api/updates", a.withAuth(a.handleAnnounceUpdate))
	mux.HandleFunc("GET /api/updates", a.withAuth(a.handleListUpdates))
	mux.HandleFunc("GET /api/updates/pending", a.withAuth(a.handlePendingUpdates))
	mux.HandleFunc("GET /api/updates/rollout", a.withAuth(a.handleUpdateRollout))
	mux.HandleFunc("POST /api/updates/reports", a.withAuth(a.handleUpdateReport))

	mux.HandleFunc("POST /api/apps", a.withAuth(a.handleSetApp))
	mux.HandleFunc("DELETE /api/apps", a.withAuth(a.handleClearApp))
	mux.HandleFunc("GET /api/apps", a.withAuth(a.handleListApps))

	mux.HandleFunc("POST /api/storage/objects", a.withAuth(a.handlePutObject))
	mux.HandleFunc("GET /api/storage/objects", a.withAuth(a.handleGetObject))
	mux.HandleFunc("DELETE /api/storage/objects", a.withAuth(a.handleDeleteObject))

	mux.HandleFunc("GET /api/reports/definitions", a.withAuth(a.handleReportDefinitions))
	mux.HandleFunc("GET /api/reports/scopes", a.withAuth(a.handleReportScopes))
	mux.HandleFunc("GET /api/reports/summary", a.withAuth(a.handleReportSummary))
	mux.HandleFunc("GET /api/reports/report", a.withAuth(a.handleReport))
	mux.HandleFunc("GET /api/reports/timeline", a.withAuth(a.handleReportTimeline))
	mux.HandleFunc("POST /api/reports/instances", a.withAuth(a.handleReportInstancePost))
	mux.HandleFunc("POST /api/reports/events", a.withAuth(a.handleReportEventsPost))

	mux.HandleFunc("GET /api/events", a.withAuth(a.handleEvents))

	mux.HandleFunc("GET /api/health", a.handleHealth)

	return mux
}

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := a.srv.Health(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "unhealthy"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
