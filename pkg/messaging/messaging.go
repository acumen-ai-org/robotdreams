// Package messaging defines the messaging abstraction used by Robot Dreams
// workers to exchange envelopes (status updates, completed-work reports,
// escalations, and requests for input) without coupling to a specific
// transport. Concrete backends (e.g. embedded SQLite, a future durable
// queue) implement MessagingBackend and register themselves with the
// package registry so callers can select one by URI scheme at runtime.
//
// MessagingBackend is deliberately a dumb transport: it delivers an
// Envelope to a single, already-resolved recipient (Envelope.To). It is
// NOT responsible for org-graph routing (e.g. "escalate to my manager" or
// "broadcast to my team") — that resolution logic belongs to a later
// phase's internal/orgchart or internal/server, which resolves a logical
// destination down to one or more concrete worker IDs before calling Emit.
package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// MessageType identifies the kind of Envelope being sent. Backends do not
// interpret MessageType beyond storing and returning it; enforcement of
// any type-specific convention (e.g. that TypeCompletedWork carries a
// StoragePtr) is the caller's responsibility.
type MessageType string

const (
	// TypeStatusUpdate is a lightweight progress report that does not
	// require a response.
	TypeStatusUpdate MessageType = "status_update"

	// TypeCompletedWork reports that a unit of work has finished. By
	// convention (not enforced by the type system) an Envelope of this
	// type should set StoragePtr to point at the durable result.
	TypeCompletedWork MessageType = "completed_work"

	// TypeEscalation reports a problem that needs attention from the
	// recipient, typically a manager or owning worker.
	TypeEscalation MessageType = "escalation"

	// TypeRequestForInput asks the recipient for information needed to
	// continue work.
	TypeRequestForInput MessageType = "request_for_input"

	// TypeScheduled is a message the control plane sends on behalf of a
	// schedule's owner when the schedule comes due (pkg/scheduling). It
	// is delivered to exactly the schedule's recipient, From is the
	// owner, and CausationID is the schedule's ID — so the recipient can
	// tell which standing instruction it came from. A worker cannot emit
	// one directly; the control plane is the only sender.
	TypeScheduled MessageType = "scheduled"
)

// StoragePointer references content held in a pkg/storage backend rather
// than inlined in the Envelope body, so large results don't have to be
// copied through the messaging transport.
type StoragePointer struct {
	// Backend identifies which storage backend holds the content (e.g.
	// a URI scheme like "file" or "s3").
	Backend string

	// Path is the object's key within the backend.
	Path string

	// Revision is the storage backend's revision identifier for the
	// exact content being referenced, if known.
	Revision string
}

// Envelope is a single message routed to one recipient. Backends treat To
// as an already-resolved single hop; multi-recipient or role-based
// routing must be resolved to concrete worker IDs, one Envelope per
// recipient, before calling Emit.
type Envelope struct {
	// ID uniquely identifies this message.
	ID string

	// Type is the kind of message being sent. See MessageType.
	Type MessageType

	// From identifies the sending worker.
	From string

	// To identifies the single recipient worker for this envelope. This
	// is a resolved hop, not a logical destination — see the package doc
	// comment.
	To string

	// Subject is a short, human-readable summary of the message.
	Subject string

	// Body is the message payload as raw JSON, shaped however the
	// sender and recipient agree.
	Body json.RawMessage

	// StoragePtr optionally references content held in a storage
	// backend instead of, or in addition to, Body. By convention it
	// should be set when Type is TypeCompletedWork, but this is not
	// enforced here.
	StoragePtr *StoragePointer

	// CreatedAt is when the message was created, per the backend's
	// configured clock.
	CreatedAt time.Time

	// CausationID optionally names the Envelope.ID this message is a
	// reply to or escalation of. Empty if this message is not caused by
	// another.
	CausationID string
}

// Ack records that a worker has processed a message.
type Ack struct {
	// MessageID is the Envelope.ID being acknowledged.
	MessageID string

	// ByWorker identifies the worker performing the acknowledgment.
	ByWorker string

	// Action describes what the worker did with the message (e.g.
	// "read", "handled"), free-form and backend-agnostic.
	Action string

	// At is when the acknowledgment was recorded, per the backend's
	// configured clock.
	At time.Time
}

// TailFilter narrows the results of a Tail call.
type TailFilter struct {
	// WorkerID restricts results to envelopes addressed to this worker.
	// Empty means no restriction by recipient.
	WorkerID string

	// Limit caps the number of envelopes returned. Zero or negative
	// means no limit.
	Limit int

	// Since restricts results to envelopes created at or after this
	// time. The zero value means no restriction.
	Since time.Time
}

// MessagingBackend is the interface Robot Dreams workers use to send and
// receive envelopes. Implementations must be safe for concurrent use by
// multiple goroutines.
type MessagingBackend interface {
	// Emit sends env to its recipient (env.To).
	Emit(ctx context.Context, env Envelope) error

	// Subscribe returns a channel of envelopes addressed to workerID.
	// Implementations MUST respect context cancellation: once ctx is
	// canceled (or its deadline passes), the returned channel must
	// eventually be closed and any goroutine backing the subscription
	// must exit — callers can rely on Subscribe never leaking a
	// goroutine or leaving the channel open past ctx's lifetime.
	Subscribe(ctx context.Context, workerID string) (<-chan Envelope, error)

	// Ack records that ack.ByWorker has processed ack.MessageID.
	//
	// An acknowledgment is bound to the recipient: it succeeds only when
	// an envelope with ID ack.MessageID is addressed (Envelope.To) to
	// ack.ByWorker, and otherwise fails with an error satisfying
	// errors.Is(err, ErrNotFound). Knowing a message ID is therefore not
	// enough to ack it — a sender cannot ack its own outgoing message,
	// and one worker cannot ack another worker's mail. Backends must not
	// distinguish "no such message" from "not yours" in the error they
	// return, so a caller cannot probe for the existence of other
	// workers' messages through Ack.
	Ack(ctx context.Context, ack Ack) error

	// Tail returns previously emitted envelopes matching filter.
	Tail(ctx context.Context, filter TailFilter) ([]Envelope, error)

	// Health reports whether the backend is currently able to serve
	// requests. It returns nil when healthy, or a descriptive error
	// otherwise.
	Health(ctx context.Context) error

	// Close releases any resources held by the backend. After Close,
	// the backend must not be used. Close also ends every live
	// Subscribe: each subscription's channel is closed and its backing
	// goroutine exits, without waiting for the subscriber's own ctx —
	// otherwise a poller would keep ticking against a closed store for
	// as long as the caller's ctx lives.
	Close() error
}

// ErrNotFound is returned by Ack when no message with the given
// MessageID is addressed to the acking worker (see MessagingBackend.Ack).
var ErrNotFound = errors.New("messaging: not found")
