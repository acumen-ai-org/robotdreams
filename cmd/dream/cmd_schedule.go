package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newScheduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Register and inspect schedules — messages the control plane sends on a cron cadence",
		Long: "Register and inspect schedules.\n\n" +
			"A schedule is a standing instruction: on a cron cadence, the control\n" +
			"plane sends one `scheduled` message from the schedule's owner to its\n" +
			"recipient, carrying the subject and body registered with it and the\n" +
			"schedule id as causation_id. What the recipient does with that message\n" +
			"is its own business; the control plane only keeps the appointment.\n\n" +
			"Like `dream message`, these are thin aids over the HTTP API.",
	}
	cmd.AddCommand(newScheduleCreateCmd(), newScheduleListCmd(), newScheduleDeleteCmd(), newScheduleFireCmd())
	return cmd
}

type scheduleView struct {
	ID        string          `json:"id"`
	Owner     string          `json:"owner"`
	To        string          `json:"to"`
	Cron      string          `json:"cron"`
	Subject   string          `json:"subject,omitempty"`
	Body      json.RawMessage `json:"body,omitempty"`
	Enabled   bool            `json:"enabled"`
	NextAt    time.Time       `json:"next_at"`
	LastAt    *time.Time      `json:"last_at,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type scheduleCreateOptions struct {
	ID string

	To      string
	Cron    string
	Subject string

	BodyFile string

	Owner string

	Disabled bool

	Server   string
	ServerID string
	AsWorker string
}

func newScheduleCreateCmd() *cobra.Command {
	var opts scheduleCreateOptions

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Register a schedule (PUT with --id, POST otherwise)",
		Long: "Register a schedule.\n\n" +
			"--cron is five fields (minute hour day-of-month month day-of-week), UTC\n" +
			"unless prefixed with TZ=<location>, e.g. \"TZ=Europe/Stockholm 0 9 * * 1-5\".\n" +
			"--to defaults to the calling identity — a worker scheduling its own\n" +
			"recurring work — and must be a registered worker.\n\n" +
			"With --id the registration is idempotent: the same command on every\n" +
			"startup keeps one schedule, updating its definition and leaving its\n" +
			"history (created, last fired) in place. Without --id the control plane\n" +
			"mints an id, and running it twice registers two schedules.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := runScheduleCreate(cmd.Context(), opts, cmd.InOrStdin())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "registered schedule %s: %s from %s to %s, next at %s\n",
				s.ID, s.Cron, s.Owner, s.To, formatTime(s.NextAt))
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.ID, "id", "", "client-chosen schedule id (re-registering the same id updates it)")
	cmd.Flags().StringVar(&opts.To, "to", "", "recipient worker (default: the calling identity)")
	cmd.Flags().StringVar(&opts.Cron, "cron", "", "five-field cron expression, optionally prefixed TZ=<location> (required)")
	cmd.Flags().StringVar(&opts.Subject, "subject", "", "subject of every message the schedule sends")
	cmd.Flags().StringVar(&opts.BodyFile, "body-file", "", "JSON file to send as the message body (\"-\" for stdin)")
	cmd.Flags().StringVar(&opts.Owner, "owner", "", "register on behalf of this worker (admin token only)")
	cmd.Flags().BoolVar(&opts.Disabled, "disabled", false, "register the schedule without enabling it")
	cmd.Flags().StringVar(&opts.Server, "server", "", "control plane address (default: $"+envDreamURL+", else from local config)")
	cmd.Flags().StringVar(&opts.ServerID, "server-id", "", "local identity directory under ~/.dream to use")
	cmd.Flags().StringVar(&opts.AsWorker, "worker-id", "", "local worker identity to authenticate as (default: from local config)")
	_ = cmd.MarkFlagRequired("cron")

	return cmd
}

func scheduleCreateRequest(opts scheduleCreateOptions, stdin io.Reader) (method, path string, body map[string]any, err error) {
	if strings.TrimSpace(opts.Cron) == "" {
		return "", "", nil, fmt.Errorf("--cron is required")
	}
	body = map[string]any{"cron": strings.TrimSpace(opts.Cron)}
	if opts.To != "" {
		body["to"] = opts.To
	}
	if opts.Subject != "" {
		body["subject"] = opts.Subject
	}
	if opts.Owner != "" {
		body["owner"] = opts.Owner
	}
	if opts.Disabled {
		body["enabled"] = false
	}
	if opts.BodyFile != "" {
		var raw []byte
		if opts.BodyFile == "-" {
			raw, err = io.ReadAll(stdin)
		} else {
			raw, err = os.ReadFile(opts.BodyFile)
		}
		if err != nil {
			return "", "", nil, fmt.Errorf("read --body-file: %w", err)
		}
		if !json.Valid(raw) {
			return "", "", nil, fmt.Errorf("--body-file %s is not valid JSON", opts.BodyFile)
		}
		body["body"] = json.RawMessage(raw)
	}
	if opts.ID != "" {
		if strings.ContainsAny(opts.ID, "/ \t") {
			return "", "", nil, fmt.Errorf("--id must not contain slashes or whitespace")
		}
		return http.MethodPut, "/api/schedules/" + url.PathEscape(opts.ID), body, nil
	}
	return http.MethodPost, "/api/schedules", body, nil
}

func runScheduleCreate(ctx context.Context, opts scheduleCreateOptions, stdin io.Reader) (scheduleView, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	method, path, body, err := scheduleCreateRequest(opts, stdin)
	if err != nil {
		return scheduleView{}, err
	}

	target, err := resolveTarget(opts.Server, opts.ServerID, opts.AsWorker)
	if err != nil {
		return scheduleView{}, err
	}
	client, err := target.oneShotClient(ctx)
	if err != nil {
		return scheduleView{}, err
	}

	var out scheduleView
	if err := client.doJSON(ctx, method, path, body, &out); err != nil {
		return scheduleView{}, err
	}
	return out, nil
}

type scheduleListOptions struct {
	Worker string

	JSON bool

	Server   string
	ServerID string
	AsWorker string
}

func newScheduleListCmd() *cobra.Command {
	var opts scheduleListOptions

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List a worker's schedules — those it owns or is sent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := runScheduleList(cmd.Context(), opts)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if opts.JSON {
				enc := json.NewEncoder(out)
				for _, s := range list {
					if err := enc.Encode(s); err != nil {
						return err
					}
				}
				return nil
			}
			if len(list) == 0 {
				fmt.Fprintln(out, "no schedules")
				return nil
			}
			for _, s := range list {
				fmt.Fprintln(out, formatScheduleLine(s))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Worker, "worker", "", "worker whose schedules to list (default: the calling identity; admin only for others)")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "print one JSON schedule per line (NDJSON)")
	cmd.Flags().StringVar(&opts.Server, "server", "", "control plane address (default: $"+envDreamURL+", else from local config)")
	cmd.Flags().StringVar(&opts.ServerID, "server-id", "", "local identity directory under ~/.dream to use")
	cmd.Flags().StringVar(&opts.AsWorker, "worker-id", "", "local worker identity to authenticate as (default: from local config)")

	return cmd
}

func scheduleListPath(worker string) string {
	if worker == "" {
		return "/api/schedules"
	}
	return "/api/schedules?" + url.Values{"worker_id": {worker}}.Encode()
}

func runScheduleList(ctx context.Context, opts scheduleListOptions) ([]scheduleView, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	target, err := resolveTarget(opts.Server, opts.ServerID, opts.AsWorker)
	if err != nil {
		return nil, err
	}
	client, err := target.oneShotClient(ctx)
	if err != nil {
		return nil, err
	}
	return fetchSchedules(ctx, client, opts.Worker)
}

func fetchSchedules(ctx context.Context, client *apiClient, worker string) ([]scheduleView, error) {
	var out struct {
		Schedules []scheduleView `json:"schedules"`
	}
	if err := client.doJSON(ctx, http.MethodGet, scheduleListPath(worker), nil, &out); err != nil {
		return nil, err
	}
	return out.Schedules, nil
}

func formatScheduleLine(s scheduleView) string {
	state := ""
	if !s.Enabled {
		state = "  (disabled)"
	}
	last := "never"
	if s.LastAt != nil {
		last = formatTime(*s.LastAt)
	}
	subject := s.Subject
	if subject == "" {
		subject = "(no subject)"
	}
	return fmt.Sprintf("%-28s %-24s to %-16s next %s  last %s  %s%s",
		s.ID, s.Cron, s.To, formatTime(s.NextAt), last, subject, state)
}

type scheduleIDOptions struct {
	ID string

	Server   string
	ServerID string
	AsWorker string
}

func addScheduleIDFlags(cmd *cobra.Command, opts *scheduleIDOptions) {
	cmd.Flags().StringVar(&opts.Server, "server", "", "control plane address (default: $"+envDreamURL+", else from local config)")
	cmd.Flags().StringVar(&opts.ServerID, "server-id", "", "local identity directory under ~/.dream to use")
	cmd.Flags().StringVar(&opts.AsWorker, "worker-id", "", "local worker identity to authenticate as (default: from local config)")
}

func newScheduleDeleteCmd() *cobra.Command {
	var opts scheduleIDOptions
	cmd := &cobra.Command{
		Use:   "delete <schedule-id>",
		Short: "Remove a schedule you own",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ID = args[0]
			if err := runScheduleDelete(cmd.Context(), opts); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted schedule %s\n", opts.ID)
			return nil
		},
	}
	addScheduleIDFlags(cmd, &opts)
	return cmd
}

func runScheduleDelete(ctx context.Context, opts scheduleIDOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.ID == "" {
		return fmt.Errorf("schedule id is required")
	}
	target, err := resolveTarget(opts.Server, opts.ServerID, opts.AsWorker)
	if err != nil {
		return err
	}
	client, err := target.oneShotClient(ctx)
	if err != nil {
		return err
	}
	return client.doJSON(ctx, http.MethodDelete, "/api/schedules/"+url.PathEscape(opts.ID), nil, nil)
}

type scheduleFireResult struct {
	Fired struct {
		ScheduleID string    `json:"schedule_id"`
		MessageID  string    `json:"message_id"`
		FiredAt    time.Time `json:"fired_at"`
		NextAt     time.Time `json:"next_at"`
	} `json:"fired"`
	Message envelopeView `json:"message"`
}

func newScheduleFireCmd() *cobra.Command {
	var opts scheduleIDOptions
	cmd := &cobra.Command{
		Use:   "fire <schedule-id>",
		Short: "Send a schedule's message now, without moving its next occurrence",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ID = args[0]
			res, err := runScheduleFire(cmd.Context(), opts)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "fired schedule %s: sent %s %s from %s to %s; next still at %s\n",
				res.Fired.ScheduleID, res.Message.Type, res.Message.ID, res.Message.From, res.Message.To,
				formatTime(res.Fired.NextAt))
			return nil
		},
	}
	addScheduleIDFlags(cmd, &opts)
	return cmd
}

func runScheduleFire(ctx context.Context, opts scheduleIDOptions) (scheduleFireResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.ID == "" {
		return scheduleFireResult{}, fmt.Errorf("schedule id is required")
	}
	target, err := resolveTarget(opts.Server, opts.ServerID, opts.AsWorker)
	if err != nil {
		return scheduleFireResult{}, err
	}
	client, err := target.oneShotClient(ctx)
	if err != nil {
		return scheduleFireResult{}, err
	}
	var res scheduleFireResult
	if err := client.doJSON(ctx, http.MethodPost, "/api/schedules/"+url.PathEscape(opts.ID)+"/fire", nil, &res); err != nil {
		return scheduleFireResult{}, err
	}
	return res, nil
}
