package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/acumen-ai-org/robotdreams/pkg/security"
	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

const followReconnectDelay = time.Second

func newMessageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "message",
		Aliases: []string{"msg"},
		Short:   "Send and read control-plane messages",
		Long: "Send and read control-plane messages.\n\n" +
			"These are deliberately thin debug/dev aids over the HTTP API: they do\n" +
			"not implement routing, retries or acknowledgment policy, they just let\n" +
			"a human poke at a running control plane.",
	}
	cmd.AddCommand(newMessageSendCmd(), newMessageTailCmd(), newMessageAckCmd())
	return cmd
}

type messageSendOptions struct {
	Type    string
	To      string
	Subject string

	Body string

	StoragePath     string
	StorageRevision string

	CausationID string

	Server   string
	ServerID string
	AsWorker string
}

func newMessageSendCmd() *cobra.Command {
	var opts messageSendOptions

	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a message from the local worker identity",
		Long: "Send a message from the local worker identity.\n\n" +
			"--to is advisory: the control plane honors it only for completed_work\n" +
			"and request_for_input. status_update and escalation always go exactly\n" +
			"one hop, to the sender's parent, and any --to is ignored server-side.\n" +
			"It is passed through rather than rejected here so the client never has\n" +
			"to duplicate the server's routing rules.\n\n" +
			"An escalation or request_for_input from a worker that already reports\n" +
			"to root fails with a conflict: there is nobody above it to answer.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := runMessageSend(cmd.Context(), opts)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if env.Delivered {
				fmt.Fprintf(out, "sent %s %s from %s to %s\n", env.Type, env.ID, env.From, env.To)
			} else {
				fmt.Fprintf(out, "accepted %s %s from %s but NOT delivered: sender reports to root, nothing above to aggregate into\n",
					env.Type, env.ID, env.From)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Type, "type", "", "status_update | completed_work | escalation | request_for_input (required)")
	cmd.Flags().StringVar(&opts.To, "to", "", "explicit recipient; honored only for completed_work and request_for_input")
	cmd.Flags().StringVar(&opts.Subject, "subject", "", "human-readable subject line")
	cmd.Flags().StringVar(&opts.Body, "body", "", "message body as a raw JSON document")
	cmd.Flags().StringVar(&opts.StoragePath, "storage-path", "",
		"storage object path this message points at (upload it with `dream storage put` first)")
	cmd.Flags().StringVar(&opts.StorageRevision, "storage-revision", "",
		"exact storage revision the pointer references (optional; use with --storage-path)")
	cmd.Flags().StringVar(&opts.CausationID, "causation-id", "", "id of the message this one responds to")
	cmd.Flags().StringVar(&opts.Server, "server", "", "control plane address (default: $"+envDreamURL+", else from local config)")
	cmd.Flags().StringVar(&opts.ServerID, "server-id", "", "local identity directory under ~/.dream to use")
	cmd.Flags().StringVar(&opts.AsWorker, "worker-id", "", "local worker identity to authenticate as (default: from local config)")
	_ = cmd.MarkFlagRequired("type")

	return cmd
}

func runMessageSend(ctx context.Context, opts messageSendOptions) (envelopeView, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Type == "" {
		return envelopeView{}, fmt.Errorf("--type is required")
	}

	req := map[string]any{"type": opts.Type}
	if opts.To != "" {
		req["to"] = opts.To
	}
	if opts.Subject != "" {
		req["subject"] = opts.Subject
	}
	if opts.CausationID != "" {
		req["causation_id"] = opts.CausationID
	}
	if opts.Body != "" {
		if !json.Valid([]byte(opts.Body)) {
			return envelopeView{}, fmt.Errorf("--body is not valid JSON (quote a string body as '\"text\"')")
		}
		req["body"] = json.RawMessage(opts.Body)
	}
	if opts.StoragePath != "" {
		req["storage_ptr"] = storagePointer{Path: opts.StoragePath, Revision: opts.StorageRevision}
	} else if opts.StorageRevision != "" {
		return envelopeView{}, fmt.Errorf("--storage-revision requires --storage-path")
	}

	target, err := resolveTarget(opts.Server, opts.ServerID, opts.AsWorker)
	if err != nil {
		return envelopeView{}, err
	}
	client, err := target.oneShotClient(ctx)
	if err != nil {
		return envelopeView{}, err
	}

	var env envelopeView
	if err := client.doJSON(ctx, http.MethodPost, "/api/messages", req, &env); err != nil {
		return envelopeView{}, err
	}
	return env, nil
}

type messageAckOptions struct {
	MessageID string
	Action    string

	Server   string
	ServerID string
	AsWorker string
}

func newMessageAckCmd() *cobra.Command {
	var opts messageAckOptions

	cmd := &cobra.Command{
		Use:   "ack <message-id>",
		Short: "Acknowledge a message you have handled",
		Long: "Acknowledge a message, recording that you have processed it.\n\n" +
			"This matters more than it looks. An unacknowledged message stays in\n" +
			"your inbox and is redelivered every time you reconnect, so a node that\n" +
			"never acks sees the same messages forever. Acking is how you say\n" +
			"\"handled\", so the next reconnect starts clean.\n\n" +
			"--action is free text describing what you did with it; it defaults to\n" +
			"\"read\". The acknowledgment is always attributed to the calling\n" +
			"identity, never to a worker named in the request.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.MessageID = args[0]
			if err := runMessageAck(cmd.Context(), opts); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "acked %s\n", opts.MessageID)
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Action, "action", "", "what you did with it (default: \"read\")")
	cmd.Flags().StringVar(&opts.Server, "server", "", "control plane address (default: $"+envDreamURL+", else from local config)")
	cmd.Flags().StringVar(&opts.ServerID, "server-id", "", "local identity directory under ~/.dream to use")
	cmd.Flags().StringVar(&opts.AsWorker, "worker-id", "", "local worker identity to authenticate as (default: from local config)")

	return cmd
}

func runMessageAck(ctx context.Context, opts messageAckOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.MessageID == "" {
		return fmt.Errorf("message id is required")
	}

	target, err := resolveTarget(opts.Server, opts.ServerID, opts.AsWorker)
	if err != nil {
		return err
	}
	client, err := target.oneShotClient(ctx)
	if err != nil {
		return err
	}

	body := map[string]string{}
	if opts.Action != "" {
		body["action"] = opts.Action
	}
	return client.doJSON(ctx, http.MethodPost, "/api/messages/"+opts.MessageID+"/ack", body, nil)
}

type messageTailOptions struct {
	Worker string

	Limit int
	Since string

	Server   string
	ServerID string
	AsWorker string

	SelfUpdate bool

	JSON bool
}

func newMessageTailCmd() *cobra.Command {
	var opts messageTailOptions
	var follow bool

	cmd := &cobra.Command{
		Use:   "tail",
		Short: "Print a worker's recent messages, optionally following live",
		Long: "Print a worker's recent messages.\n\n" +
			"Without --follow this is a single GET of recent history. With --follow\n" +
			"it attaches to the server-sent-events stream and prints envelopes as\n" +
			"they arrive until interrupted; because that session is long-lived it\n" +
			"is the one command that runs a security.WorkerClient background token\n" +
			"refresh, so a reconnect after a dropped stream uses a fresh token.\n\n" +
			"You may tail your own inbox; reading another worker's requires the\n" +
			"admin scope.\n\n" +
			"--json switches the output to one JSON envelope per line, which is what\n" +
			"a script should read: the human line does not carry the message id, and\n" +
			"without an id you cannot `dream message ack` what you just received —\n" +
			"so an unacked message is redelivered on every reconnect.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if follow {
				return runMessageFollow(cmd.Context(), opts, cmd.OutOrStdout(), cmd.ErrOrStderr())
			}
			envs, err := runMessageTail(cmd.Context(), opts)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(envs) == 0 {
				if !opts.JSON {
					fmt.Fprintln(out, "no messages")
				}
				return nil
			}
			for _, env := range envs {
				if err := emitEnvelope(out, env, opts.JSON); err != nil {
					return err
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Worker, "worker", "", "worker whose inbox to read (default: the calling identity)")
	cmd.Flags().IntVar(&opts.Limit, "limit", 0, "maximum number of messages to return")
	cmd.Flags().StringVar(&opts.Since, "since", "", "only messages at or after this RFC3339 timestamp")
	cmd.Flags().BoolVar(&follow, "follow", false, "stream new messages as they arrive")
	cmd.Flags().BoolVar(&opts.JSON, "json", false,
		"print one JSON envelope per line (NDJSON) instead of a human line; the JSON form carries the message id, which the human line does not")
	cmd.Flags().BoolVar(&opts.SelfUpdate, "self-update", selfUpdateDefault(),
		"when the control plane announces a "+updates.KindCLI+" update, apply it and re-exec "+
			"(default: false, or $"+envDreamSelfUpdate+"). This lets a message from the network "+
			"replace this binary, so enable it only where you trust the control plane accordingly; "+
			"with it off the follower just prints a line telling you to run `dream self-update`")
	cmd.Flags().StringVar(&opts.Server, "server", "", "control plane address (default: $"+envDreamURL+", else from local config)")
	cmd.Flags().StringVar(&opts.ServerID, "server-id", "", "local identity directory under ~/.dream to use")
	cmd.Flags().StringVar(&opts.AsWorker, "worker-id", "", "local worker identity to authenticate as (default: from local config)")

	return cmd
}

func tailQuery(opts messageTailOptions, includeHistoryFilters bool) (string, error) {
	q := url.Values{}
	if opts.Worker != "" {
		q.Set("worker_id", opts.Worker)
	}
	if includeHistoryFilters {
		if opts.Limit > 0 {
			q.Set("limit", strconv.Itoa(opts.Limit))
		}
		if opts.Since != "" {
			ts, err := time.Parse(time.RFC3339, opts.Since)
			if err != nil {
				return "", fmt.Errorf("--since must be an RFC3339 timestamp: %w", err)
			}
			q.Set("since", ts.Format(time.RFC3339))
		}
	}
	if len(q) == 0 {
		return "", nil
	}
	return "?" + q.Encode(), nil
}

func runMessageTail(ctx context.Context, opts messageTailOptions) ([]envelopeView, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	query, err := tailQuery(opts, true)
	if err != nil {
		return nil, err
	}

	target, err := resolveTarget(opts.Server, opts.ServerID, opts.AsWorker)
	if err != nil {
		return nil, err
	}
	client, err := target.oneShotClient(ctx)
	if err != nil {
		return nil, err
	}

	var out struct {
		Messages []envelopeView `json:"messages"`
	}
	if err := client.doJSON(ctx, http.MethodGet, "/api/messages"+query, nil, &out); err != nil {
		return nil, err
	}
	return out.Messages, nil
}

func runMessageFollow(ctx context.Context, opts messageTailOptions, out, errOut io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var inFlight sync.WaitGroup
	query, err := tailQuery(opts, false)
	if err != nil {
		return err
	}

	target, err := resolveTarget(opts.Server, opts.ServerID, opts.AsWorker)
	if err != nil {
		return err
	}

	streamingClient := &http.Client{}
	wc := security.NewWorkerClient(
		target.keyPair.PrivateKey,
		target.keyPair.PublicKey,
		tokenSourceFor(defaultHTTPClient(), target.baseURL, target.workerID, target.keyPair),
		nil,
	)
	if err := wc.Start(ctx); err != nil {
		return err
	}
	defer wc.Stop()

	for {
		tok, err := wc.Token()
		if err != nil {
			return err
		}
		client := &apiClient{baseURL: target.baseURL, token: tok.Raw, hc: streamingClient}

		handler := func(env envelopeView) error {
			inFlight.Add(1)
			defer inFlight.Done()

			if err := emitEnvelope(out, env, opts.JSON); err != nil {
				return err
			}

			req, ok := updateRequestFor(env)
			if !ok {
				return nil
			}
			if !opts.SelfUpdate {
				fmt.Fprintf(errOut, "  an update to %s %s is available; run `dream self-update`\n",
					req.Kind, req.Version)
				return nil
			}
			return req
		}

		streamErr := streamEvents(ctx, client, "/api/messages/subscribe"+query, handler)
		var req *updateRequest
		if errors.As(streamErr, &req) {
			inFlight.Wait()
			wc.Stop()
			return applyAnnouncedUpdate(ctx, errOut, req)
		}
		if streamErr != nil {
			return streamErr
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(followReconnectDelay):
		}
	}
}

type envelopeHandler func(env envelopeView) error

func streamEvents(ctx context.Context, client *apiClient, path string, h envelopeHandler) error {
	resp, err := client.do(ctx, http.MethodGet, path, "", nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for scanner.Scan() {
		line := scanner.Text()
		payload, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		var env envelopeView
		if err := json.Unmarshal([]byte(strings.TrimSpace(payload)), &env); err != nil {
			continue
		}
		if err := h(env); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

func emitEnvelope(out io.Writer, env envelopeView, asJSON bool) error {
	if !asJSON {
		_, err := fmt.Fprintln(out, formatEnvelopeLine(env))
		return err
	}
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("encode envelope %s: %w", env.ID, err)
	}
	_, err = fmt.Fprintln(out, string(b))
	return err
}

func formatEnvelopeLine(env envelopeView) string {
	subject := env.Subject
	if subject == "" {
		subject = "(no subject)"
	}
	line := fmt.Sprintf("%s  %-18s from %-16s %s", formatTime(env.CreatedAt), env.Type, env.From, subject)
	if len(env.Body) > 0 {
		line += "  " + string(env.Body)
	}
	if env.StoragePtr != nil && env.StoragePtr.Path != "" {
		line += "  [storage: " + env.StoragePtr.Path + "]"
	}
	return line
}
