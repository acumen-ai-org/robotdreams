// Package storage defines the object store Robot Dreams workers read and write through, with backends selected by URI scheme.
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// ObjectMeta describes an object stored under a path, without its content.
type ObjectMeta struct {
	Path      string
	Revision  string
	Size      int64
	UpdatedAt time.Time
	UpdatedBy string
}

// PutOptions configures a Put call.
type PutOptions struct {
	IfMatchRevision string
	UpdatedBy       string
}

// StorageBackend is the concurrency-safe object store workers read and write through.
type StorageBackend interface {
	// Get returns the content (which the caller must Close) and metadata at path, or ErrNotFound.
	Get(ctx context.Context, path string) (io.ReadCloser, ObjectMeta, error)

	// Put writes r to path, failing with ErrRevisionMismatch and leaving the object unchanged when opts.IfMatchRevision is set and does not match the stored revision.
	Put(ctx context.Context, path string, r io.Reader, opts PutOptions) (ObjectMeta, error)

	// Delete removes the object at path, or returns ErrNotFound.
	Delete(ctx context.Context, path string) error

	// List returns metadata for every object whose path starts with prefix.
	List(ctx context.Context, prefix string) ([]ObjectMeta, error)

	// Stat returns metadata for the object at path without reading it, or ErrNotFound.
	Stat(ctx context.Context, path string) (ObjectMeta, error)

	// Revisions returns the backend's retained past revisions of path in implementation-defined order, or ErrNotFound.
	Revisions(ctx context.Context, path string) ([]ObjectMeta, error)

	// Health returns nil when the backend can serve requests.
	Health(ctx context.Context) error

	// Close releases the backend's resources; the backend must not be used afterwards.
	Close() error
}

// ErrRevisionMismatch is returned by Put when PutOptions.IfMatchRevision does not match the stored revision.
var ErrRevisionMismatch = errors.New("storage: revision mismatch")

// ErrNotFound is returned by Get, Delete, Stat, and Revisions when no object exists at the path.
var ErrNotFound = errors.New("storage: not found")
