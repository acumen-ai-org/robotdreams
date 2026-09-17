package temporal

import (
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
)

// TaskQueue is the Temporal task queue the Worker polls and mailbox workflows start on.
const TaskQueue = "robotdreams-mailbox"

// SignalDeliver is the Signal that delivers an Envelope into a worker's mailbox workflow.
const SignalDeliver = "deliver"

// SignalAck is the Signal that removes a delivered Envelope from the mailbox's pending buffer.
const SignalAck = "ack"

// QueryPending is the read-only Query that returns the mailbox's delivered-but-unacked envelopes.
const QueryPending = "pending"

const maxBufferedEnvelopes = 1000

const continueAsNewAfterSignals = 2000

type mailboxState struct {
	Pending []messaging.Envelope
}

type ackSignal struct {
	MessageID string
}

func mailboxWorkflow(ctx workflow.Context, initial mailboxState) error {
	state := initial
	if state.Pending == nil {
		state.Pending = []messaging.Envelope{}
	}

	deliverCh := workflow.GetSignalChannel(ctx, SignalDeliver)
	ackCh := workflow.GetSignalChannel(ctx, SignalAck)

	err := workflow.SetQueryHandler(ctx, QueryPending, func() ([]messaging.Envelope, error) {
		return state.Pending, nil
	})
	if err != nil {
		return err
	}

	signalsProcessed := 0

	for {
		selector := workflow.NewSelector(ctx)

		selector.AddReceive(deliverCh, func(c workflow.ReceiveChannel, more bool) {
			var env messaging.Envelope
			c.Receive(ctx, &env)
			state.Pending = appendBounded(state.Pending, env)
			signalsProcessed++
		})

		selector.AddReceive(ackCh, func(c workflow.ReceiveChannel, more bool) {
			var ack ackSignal
			c.Receive(ctx, &ack)
			state.Pending = removeEnvelope(state.Pending, ack.MessageID)
			signalsProcessed++
		})

		selector.Select(ctx)

		if signalsProcessed >= continueAsNewAfterSignals {
			state.Pending = absorbAlreadyQueuedSignals(state.Pending, deliverCh, ackCh)
			return workflow.NewContinueAsNewError(ctx, mailboxWorkflow, state)
		}
	}
}

func appendBounded(pending []messaging.Envelope, env messaging.Envelope) []messaging.Envelope {
	pending = append(pending, env)
	if len(pending) > maxBufferedEnvelopes {
		pending = pending[len(pending)-maxBufferedEnvelopes:]
	}
	return pending
}

func absorbAlreadyQueuedSignals(pending []messaging.Envelope, deliverCh, ackCh workflow.ReceiveChannel) []messaging.Envelope {
	for {
		var env messaging.Envelope
		if !deliverCh.ReceiveAsync(&env) {
			break
		}
		pending = appendBounded(pending, env)
	}
	for {
		var ack ackSignal
		if !ackCh.ReceiveAsync(&ack) {
			break
		}
		pending = removeEnvelope(pending, ack.MessageID)
	}
	return pending
}

func removeEnvelope(pending []messaging.Envelope, id string) []messaging.Envelope {
	for i, env := range pending {
		if env.ID == id {
			out := make([]messaging.Envelope, 0, len(pending)-1)
			out = append(out, pending[:i]...)
			out = append(out, pending[i+1:]...)
			return out
		}
	}
	return pending
}

func workflowIDForWorker(workerID string) string {
	return "mailbox-" + workerID
}

const workflowStartToCloseTimeout = 24 * time.Hour * 365 * 10
