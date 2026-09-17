package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/server"
)

type delegateRequest struct {
	Child      string   `json:"child,omitempty"`
	Scopes     []string `json:"scopes,omitempty"`
	TTLSeconds int      `json:"ttl_seconds,omitempty"`
}

type delegateResponse struct {
	WorkerID    string    `json:"worker_id"`
	DelegatedBy string    `json:"delegated_by"`
	Scopes      []string  `json:"scopes"`
	Token       string    `json:"token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (a *API) handleDelegate(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())

	var req delegateRequest
	if !decodeOptionalJSON(w, r, &req) {
		return
	}
	if req.TTLSeconds < 0 {
		writeError(w, http.StatusBadRequest, "ttl_seconds must not be negative")
		return
	}

	d, err := a.srv.MintDelegatedToken(claims, server.DelegationRequest{
		Child:  req.Child,
		Scopes: req.Scopes,
		TTL:    time.Duration(req.TTLSeconds) * time.Second,
	})
	switch {
	case errors.Is(err, server.ErrCannotDelegate):
		writeError(w, http.StatusForbidden, err.Error())
		return
	case errors.Is(err, server.ErrInvalidDelegation):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "mint delegated token")
		return
	}

	writeJSON(w, http.StatusCreated, delegateResponse{
		WorkerID:    d.WorkerID,
		DelegatedBy: d.DelegatedBy,
		Scopes:      d.Scopes,
		Token:       d.Token.Raw,
		ExpiresAt:   d.Token.ExpiresAt,
	})
}
