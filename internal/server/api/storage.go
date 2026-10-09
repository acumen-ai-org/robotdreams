package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/storage"
)

const maxObjectUploadBytes = 256 << 20

type objectMetaView struct {
	Path      string    `json:"path"`
	Revision  string    `json:"revision"`
	Size      int64     `json:"size"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by,omitempty"`
}

func newObjectMetaView(m storage.ObjectMeta) objectMetaView {
	return objectMetaView{
		Path:      m.Path,
		Revision:  m.Revision,
		Size:      m.Size,
		UpdatedAt: m.UpdatedAt,
		UpdatedBy: m.UpdatedBy,
	}
}

func (a *API) handlePutObject(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())

	q := r.URL.Query()
	path := q.Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path query parameter is required")
		return
	}
	if !hasStorageScope(claims, "write", path) {
		writeError(w, http.StatusForbidden, "token is not scoped for write access to this path")
		return
	}

	opts := storage.PutOptions{
		IfMatchRevision: q.Get("if_match_revision"),
		UpdatedBy:       claims.WorkerID,
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxObjectUploadBytes)
	defer r.Body.Close()
	meta, err := a.srv.Storage().Put(r.Context(), path, r.Body, opts)
	switch {
	case err == nil:
	case errors.Is(err, storage.ErrRevisionMismatch):
		writeError(w, http.StatusConflict, fmt.Sprintf("revision mismatch: %q is not the current revision of %q", opts.IfMatchRevision, path))
		return
	default:
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("object exceeds %d bytes", maxObjectUploadBytes))
			return
		}
		writeError(w, http.StatusInternalServerError, "could not write object")
		return
	}

	writeJSON(w, http.StatusOK, newObjectMetaView(meta))
}

func (a *API) handleGetObject(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	path := q.Get("path")
	_, hasPrefix := q["prefix"]

	switch {
	case path != "" && hasPrefix:
		writeError(w, http.StatusBadRequest, "supply either path or prefix, not both")
		return
	case hasPrefix:
		a.listObjects(w, r, q.Get("prefix"))
		return
	case path != "":
		a.getObject(w, r, path)
		return
	default:
		writeError(w, http.StatusBadRequest, "one of path or prefix is required")
		return
	}
}

func (a *API) getObject(w http.ResponseWriter, r *http.Request, path string) {
	claims, _ := ClaimsFromContext(r.Context())
	if !hasStorageScope(claims, "read", path) {
		writeError(w, http.StatusForbidden, "token is not scoped for read access to this path")
		return
	}

	rc, meta, err := a.srv.Storage().Get(r.Context(), path)
	switch {
	case err == nil:
	case errors.Is(err, storage.ErrNotFound):
		writeError(w, http.StatusNotFound, "no such object")
		return
	default:
		writeError(w, http.StatusInternalServerError, "could not read object")
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))
	w.Header().Set("X-Path", meta.Path)
	w.Header().Set("X-Revision", meta.Revision)
	w.Header().Set("X-Updated-At", meta.UpdatedAt.UTC().Format(time.RFC3339Nano))
	if meta.UpdatedBy != "" {
		w.Header().Set("X-Updated-By", meta.UpdatedBy)
	}
	w.WriteHeader(http.StatusOK)

	_, _ = io.Copy(w, rc)
}

func (a *API) listObjects(w http.ResponseWriter, r *http.Request, prefix string) {
	claims, _ := ClaimsFromContext(r.Context())
	if !hasStorageScope(claims, "read", prefix) {
		writeError(w, http.StatusForbidden, "token is not scoped for read access to this prefix")
		return
	}

	metas, err := a.srv.Storage().List(r.Context(), prefix)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list objects")
		return
	}

	out := make([]objectMetaView, 0, len(metas))
	for _, m := range metas {
		out = append(out, newObjectMetaView(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"objects": out})
}

func (a *API) handleDeleteObject(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())

	q := r.URL.Query()
	path := q.Get("path")
	_, hasPrefix := q["prefix"]
	switch {
	case path != "" && hasPrefix:
		writeError(w, http.StatusBadRequest, "supply either path or prefix, not both")
		return
	case hasPrefix:
		a.deleteObjectsByPrefix(w, r, q.Get("prefix"), q.Get("dry_run") == "true")
		return
	case path == "":
		writeError(w, http.StatusBadRequest, "one of path or prefix is required")
		return
	}
	if !hasStorageScope(claims, "write", path) {
		writeError(w, http.StatusForbidden, "token is not scoped for write access to this path")
		return
	}

	err := a.srv.Storage().Delete(r.Context(), path)
	switch {
	case err == nil:
	case errors.Is(err, storage.ErrNotFound):
		writeError(w, http.StatusNotFound, "no such object")
		return
	default:
		writeError(w, http.StatusInternalServerError, "could not delete object")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// deleteObjectsByPrefix removes every object under prefix. The prefix must
// name a directory (end with "/"), so a delete can never match a sibling such
// as "workers/a" matching "workers/ab/", and the caller's write scope must
// cover the whole prefix. With dryRun it only counts. An empty prefix is
// "nothing to delete", not an error.
func (a *API) deleteObjectsByPrefix(w http.ResponseWriter, r *http.Request, prefix string, dryRun bool) {
	claims, _ := ClaimsFromContext(r.Context())
	if !strings.HasSuffix(prefix, "/") {
		writeError(w, http.StatusBadRequest, "prefix must end with /")
		return
	}
	if !hasStorageScope(claims, "write", prefix) {
		writeError(w, http.StatusForbidden, "token is not scoped for write access to this prefix")
		return
	}

	metas, err := a.srv.Storage().List(r.Context(), prefix)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list objects")
		return
	}

	deleted := 0
	for _, m := range metas {
		if !strings.HasPrefix(m.Path, prefix) {
			continue
		}
		if dryRun {
			deleted++
			continue
		}
		err := a.srv.Storage().Delete(r.Context(), m.Path)
		switch {
		case err == nil:
			deleted++
		case errors.Is(err, storage.ErrNotFound):
		default:
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("could not delete object %q after deleting %d", m.Path, deleted))
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"prefix":  prefix,
		"deleted": deleted,
		"dry_run": dryRun,
	})
}

func parsePositiveInt(raw string) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, fmt.Errorf("must be positive, got %d", n)
	}
	return n, nil
}
