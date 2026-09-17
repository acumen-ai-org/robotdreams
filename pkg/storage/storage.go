// Package storage defines the storage abstraction used by Robot Dreams
// workers to read and write objects without coupling to a specific
// backend. Concrete backends (e.g. local filesystem, cloud object stores)
// implement StorageBackend and register themselves with the package
// registry so callers can select one by URI scheme at runtime.
//
// Concurrency control here is optimistic: callers that need to avoid
// clobbering a concurrent writer pass PutOptions.IfMatchRevision instead of
// acquiring a lease. This is a deliberate simplification over a heavier
// pessimistic locking model — see PutOptions for details.
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// ObjectMeta describes an object stored under a path, without its content.
type ObjectMeta struct {
	// Path is the object's key within the backend.
	Path string

	// Revision is an opaque, monotonic-per-path identifier for the exact
	// content stored at Path. Backends are free to choose any scheme
	// (a content hash is a valid implementation) as long as two writes
	// with different content never produce the same revision for the
	// same path.
	Revision string

	// Size is the content length in bytes.
	Size int64

	// UpdatedAt is when this revision was written, per the backend's
	// configured clock.
	UpdatedAt time.Time

	// UpdatedBy identifies the writer of this revision, for audit
	// purposes: whatever the caller passed as PutOptions.UpdatedBy. The
	// control plane always supplies it, so it is empty only for objects
	// written by a direct library caller that left the option unset.
	UpdatedBy string
}

// PutOptions configures a Put call.
type PutOptions struct {
	// IfMatchRevision enables optimistic concurrency control. When empty,
	// Put unconditionally creates or overwrites the object. When
	// non-empty, Put succeeds only if the object's current stored
	// revision equals IfMatchRevision; otherwise it fails with
	// ErrRevisionMismatch and leaves the stored object unchanged. This
	// replaces a heavier pessimistic lease model by design: callers
	// detect conflicting concurrent writers by retrying against the
	// latest revision rather than holding a lock.
	IfMatchRevision string

	// UpdatedBy is the authenticated identity of the writer, recorded as
	// ObjectMeta.UpdatedBy. The control plane sets it from the claims on
	// the request that triggered the write; a backend stores it verbatim
	// and never infers it from ambient state such as the process user or
	// the connection's credentials.
	UpdatedBy string
}

// StorageBackend is the interface Robot Dreams workers use to read and
// write objects. Implementations must be safe for concurrent use by
// multiple goroutines.
type StorageBackend interface {
	// Get returns the content and metadata for the object at path. The
	// caller must Close the returned ReadCloser. Returns an error
	// satisfying errors.Is(err, ErrNotFound) if no object exists at path.
	Get(ctx context.Context, path string) (io.ReadCloser, ObjectMeta, error)

	// Put writes content to path, subject to opts. It returns the
	// metadata for the newly written object. See PutOptions for
	// conditional-write semantics.
	Put(ctx context.Context, path string, r io.Reader, opts PutOptions) (ObjectMeta, error)

	// Delete removes the object at path. Returns an error satisfying
	// errors.Is(err, ErrNotFound) if no object exists at path.
	Delete(ctx context.Context, path string) error

	// List returns metadata for all objects whose path starts with
	// prefix. An empty prefix lists all objects.
	List(ctx context.Context, prefix string) ([]ObjectMeta, error)

	// Stat returns metadata for the object at path without reading its
	// content. Returns an error satisfying errors.Is(err, ErrNotFound) if
	// no object exists at path.
	Stat(ctx context.Context, path string) (ObjectMeta, error)

	// Revisions returns metadata for known past revisions of the object
	// at path, in an implementation-defined order. Implementations are
	// free to retain a bounded history rather than every revision ever
	// written; see individual backend documentation for their retention
	// policy. Returns an error satisfying errors.Is(err, ErrNotFound) if
	// no object exists at path.
	Revisions(ctx context.Context, path string) ([]ObjectMeta, error)

	// Health reports whether the backend is currently able to serve
	// requests. It returns nil when healthy, or a descriptive error
	// otherwise.
	Health(ctx context.Context) error

	// Close releases any resources held by the backend. After Close,
	// the backend must not be used.
	Close() error
}

// ErrRevisionMismatch is returned by Put when PutOptions.IfMatchRevision
// does not match the object's current stored revision.
var ErrRevisionMismatch = errors.New("storage: revision mismatch")

// ErrNotFound is returned by Get, Delete, Stat, and Revisions when no
// object exists at the requested path.
var ErrNotFound = errors.New("storage: not found")
