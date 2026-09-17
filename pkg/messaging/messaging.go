// Package messaging defines the single-hop envelope transport Robot Dreams workers use, with backends selected by URI scheme.
package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// MessageType identifies the kind of Envelope being sent.
type MessageType string

const (
	// TypeStatusUpdate is a progress report that needs no response.
	TypeStatusUpdate MessageType = "status_update"

	// TypeCompletedWork reports a finished unit of work, by convention with StoragePtr set.
	TypeCompletedWork MessageType = "completed_work"

	// TypeEscalation reports a problem that needs the recipient's attention.
	TypeEscalation MessageType = "escalation"

	// TypeRequestForInput asks the recipient for information needed to continue.
	TypeRequestForInput MessageType = "request_for_input"

	// TypeScheduled is sent by the control plane on a schedule owner's behalf when the schedule comes due.
	TypeScheduled MessageType = "scheduled"
)

// StoragePointer references content held in a pkg/storage backend instead of inlined in the Body.
type StoragePointer struct {
	Backend  string
	Path     string
	Revision string
}

// Envelope is a single message addressed to one already-resolved recipient.
type Envelope struct {
	ID          string
	Type        MessageType
	From        string
	To          string
	Subject     string
	Body        json.RawMessage
	StoragePtr  *StoragePointer
	CreatedAt   time.Time
	CausationID string
}

// Ack records that a worker has processed a message.
type Ack struct {
	MessageID string
	ByWorker  string
	Action    string
	At        time.Time
}

// TailFilter narrows the results of a Tail call; zero values mean no restriction.
type TailFilter struct {
	WorkerID string
	Limit    int
	Since    time.Time
}

// MessagingBackend is the concurrency-safe transport workers use to send and receive envelopes.
type MessagingBackend interface {
	// Emit sends env to env.To.
	Emit(ctx context.Context, env Envelope) error

	// Subscribe returns a channel of envelopes addressed to workerID that closes, with its goroutine exiting, once ctx ends.
	Subscribe(ctx context.Context, workerID string) (<-chan Envelope, error)

	// Ack records that ack.ByWorker processed ack.MessageID, failing with ErrNotFound unless that envelope is addressed to ack.ByWorker.
	Ack(ctx context.Context, ack Ack) error

	// Tail returns previously emitted envelopes matching filter.
	Tail(ctx context.Context, filter TailFilter) ([]Envelope, error)

	// Health returns nil when the backend can serve requests.
	Health(ctx context.Context) error

	// Close releases the backend's resources and ends every live Subscribe without waiting for the subscriber's ctx.
	Close() error
}

// ErrNotFound is returned by Ack when no message with that ID is addressed to the acking worker.
var ErrNotFound = errors.New("messaging: not found")
