package server

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/acumen-ai-org/robotdreams/internal/server/store"
)

var ErrInvalidApp = errors.New("server: invalid app")

const maxAppDescription = 280

func (s *Server) SetWorkerApp(ctx context.Context, workerID, rawURL, description string) (store.WorkerApp, error) {
	if workerID == "" {
		return store.WorkerApp{}, fmt.Errorf("%w: empty worker ID", ErrInvalidApp)
	}
	if _, err := s.graph.Get(workerID); err != nil {
		return store.WorkerApp{}, err
	}

	clean, err := ValidateAppURL(rawURL)
	if err != nil {
		return store.WorkerApp{}, err
	}

	description = strings.TrimSpace(description)
	if len(description) > maxAppDescription {
		return store.WorkerApp{}, fmt.Errorf("%w: description is %d characters, limit is %d",
			ErrInvalidApp, len(description), maxAppDescription)
	}

	app := store.WorkerApp{
		WorkerID:    workerID,
		URL:         clean,
		Description: description,
		DeclaredAt:  s.clock.Now().UTC(),
	}
	if err := s.store.UpsertWorkerApp(ctx, app); err != nil {
		return store.WorkerApp{}, err
	}
	return app, nil
}

func (s *Server) ClearWorkerApp(ctx context.Context, workerID string) error {
	if _, err := s.graph.Get(workerID); err != nil {
		return err
	}
	return s.store.DeleteWorkerApp(ctx, workerID)
}

func (s *Server) WorkerApps(ctx context.Context) ([]store.WorkerApp, error) {
	return s.store.ListWorkerApps(ctx)
}

func (s *Server) WorkerApp(ctx context.Context, workerID string) (store.WorkerApp, error) {
	return s.store.GetWorkerApp(ctx, workerID)
}

func ValidateAppURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("%w: empty URL", ErrInvalidApp)
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%w: %q is not a URL: %s", ErrInvalidApp, raw, err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	case "":
		return "", fmt.Errorf("%w: %q has no scheme; use http:// or https://", ErrInvalidApp, raw)
	default:
		return "", fmt.Errorf("%w: scheme %q is not allowed; use http:// or https://", ErrInvalidApp, u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("%w: %q has no host", ErrInvalidApp, raw)
	}
	return u.String(), nil
}
