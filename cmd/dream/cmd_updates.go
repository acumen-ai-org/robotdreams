package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

func newUpdatesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "updates",
		Short: "Announce, inspect and report on fleet updates",
		Long: "Announce, inspect and report on fleet updates.\n\n" +
			"Robot Dreams ships an update CONTRACT, not an update runtime. The\n" +
			"control plane broadcasts \"a version of <kind> is available\" to every\n" +
			"node and records what each node says back. It never downloads,\n" +
			"installs, restarts or supervises anything, and it never compares two\n" +
			"version strings — your node's runtime is yours.\n\n" +
			"Every node receives every announcement and decides for itself whether\n" +
			"a kind applies to it. `robotdreams/cli` is the one reserved kind: the\n" +
			"dream binary itself. Your deployment defines its own (for example\n" +
			"acme/prompt-pack) and gets the same rollout tracking for free.\n\n" +
			"Run `dream updates contract` for the full base contract.",
	}
	cmd.AddCommand(
		newUpdatesAnnounceCmd(),
		newUpdatesListCmd(),
		newUpdatesPendingCmd(),
		newUpdatesReportCmd(),
		newUpdatesRolloutCmd(),
		newUpdatesContractCmd(),
	)
	return cmd
}

type updatesClientOptions struct {
	Server   string
	ServerID string
	AsWorker string
}

func addUpdatesClientFlags(cmd *cobra.Command, opts *updatesClientOptions) {
	cmd.Flags().StringVar(&opts.Server, "server", "", "control plane address (default: $"+envDreamURL+", else from local config)")
	cmd.Flags().StringVar(&opts.ServerID, "server-id", "", "local identity directory under ~/.dream to use")
	cmd.Flags().StringVar(&opts.AsWorker, "worker-id", "", "local worker identity to authenticate as (default: from local config)")
}

func (o updatesClientOptions) nodeClient(ctx context.Context) (*apiClient, error) {
	target, err := resolveTarget(o.Server, o.ServerID, o.AsWorker)
	if err != nil {
		return nil, err
	}
	return target.oneShotClient(ctx)
}

type updatesAdminOptions struct {
	Addr       string
	DataDir    string
	AdminToken string
}

func addUpdatesAdminFlags(cmd *cobra.Command, opts *updatesAdminOptions) {
	cmd.Flags().StringVar(&opts.Addr, "server", "", "control plane address (default: $"+envDreamURL+", else 127.0.0.1"+server.DefaultAddr+")")
	cmd.Flags().StringVar(&opts.DataDir, "data-dir", "", "server data directory, used to mint a local admin token")
	cmd.Flags().StringVar(&opts.AdminToken, "admin-token", "", "admin token (precedence: this flag, then $"+envDreamToken+", then minted from --data-dir)")
}

func (o updatesAdminOptions) adminClient() (*apiClient, error) {
	token := o.AdminToken
	if token == "" {
		token = dreamTokenFromEnv()
	}
	if token == "" {
		dataDir := o.DataDir
		if dataDir == "" {
			d, err := defaultServerDataDir()
			if err != nil {
				return nil, err
			}
			dataDir = d
		}
		t, err := mintLocalAdminToken(dataDir)
		if err != nil {
			return nil, err
		}
		token = t
	}

	addr := serverAddrOrEnv(o.Addr)
	if addr == "" {
		addr = "127.0.0.1" + server.DefaultAddr
	}
	return &apiClient{baseURL: baseURLFromAddr(addr), token: token, hc: defaultHTTPClient()}, nil
}

type updatesAnnounceOptions struct {
	updatesAdminOptions

	Kind       string
	Version    string
	Source     string
	MinVersion string
	Severity   string
	Notes      string
}

type announceResult struct {
	Announcement updates.Announcement `json:"announcement"`
	Recipients   int                  `json:"recipients"`
	Delivered    int                  `json:"delivered"`
	Warning      string               `json:"warning,omitempty"`
}

func newUpdatesAnnounceCmd() *cobra.Command {
	var opts updatesAnnounceOptions

	cmd := &cobra.Command{
		Use:   "announce",
		Short: "Broadcast that a version of some kind is available (admin)",
		Long: "Broadcast to every registered node that a version of some kind is\n" +
			"available.\n\n" +
			"This is a broadcast, not a targeted rollout: every node receives the\n" +
			"announcement and filters on --kind itself. There is no selector and no\n" +
			"canary staging, because the control plane announces rather than\n" +
			"orchestrates.\n\n" +
			"The announcement is durable. A node that is offline right now finds it\n" +
			"waiting in its inbox when it reconnects.\n\n" +
			"Nothing is installed by this command, on any node. It states that a\n" +
			"version exists; what each node does about it is that node's business.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := runUpdatesAnnounce(cmd.Context(), opts)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "announced %s %s (announcement %s)\n",
				res.Announcement.Kind, res.Announcement.Version, res.Announcement.ID)
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintf(tw, "  severity\t%s\n", orDash(res.Announcement.Severity))
			fmt.Fprintf(tw, "  source\t%s\n", orDash(res.Announcement.Source))
			fmt.Fprintf(tw, "  recipients\t%d\n", res.Recipients)
			fmt.Fprintf(tw, "  delivered\t%d\n", res.Delivered)
			if err := tw.Flush(); err != nil {
				return err
			}
			if res.Warning != "" {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"\nwarning: %d of %d recipients could not be reached:\n  %s\n",
					res.Recipients-res.Delivered, res.Recipients, res.Warning)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Kind, "kind", "", "what is being updated, as <namespace>/<name> (e.g. "+updates.KindCLI+")")
	cmd.Flags().StringVar(&opts.Version, "version", "", "the available version (opaque to Robot Dreams)")
	cmd.Flags().StringVar(&opts.Source, "source", "", "where the artifact lives; never fetched by Robot Dreams")
	cmd.Flags().StringVar(&opts.MinVersion, "min-version", "", "advisory: nodes below this version should update")
	cmd.Flags().StringVar(&opts.Severity, "severity", "", "optional|recommended|required — advisory, interpreted by the node")
	cmd.Flags().StringVar(&opts.Notes, "notes", "", "free text for the node: your own drain-and-restart guidance")
	addUpdatesAdminFlags(cmd, &opts.updatesAdminOptions)

	_ = cmd.MarkFlagRequired("kind")
	_ = cmd.MarkFlagRequired("version")
	return cmd
}

func runUpdatesAnnounce(ctx context.Context, opts updatesAnnounceOptions) (announceResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := opts.adminClient()
	if err != nil {
		return announceResult{}, err
	}

	body := map[string]string{"kind": opts.Kind, "version": opts.Version}
	for k, v := range map[string]string{
		"source": opts.Source, "min_version": opts.MinVersion,
		"severity": opts.Severity, "notes": opts.Notes,
	} {
		if v != "" {
			body[k] = v
		}
	}

	var res announceResult
	if err := client.doJSON(ctx, http.MethodPost, "/api/updates", body, &res); err != nil {
		return announceResult{}, err
	}
	return res, nil
}

type updatesListOptions struct {
	updatesClientOptions
	Kind  string
	Limit int
}

func newUpdatesListCmd() *cobra.Command {
	var opts updatesListOptions

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent announcements",
		Long: "List recent update announcements, newest first.\n\n" +
			"Readable by any authenticated node, not just an admin: the same\n" +
			"announcements were broadcast into every node's inbox anyway, so a\n" +
			"stricter pull endpoint would protect nothing while breaking a node's\n" +
			"catch-up path after downtime.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			anns, err := runUpdatesList(cmd.Context(), opts)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(anns) == 0 {
				fmt.Fprintln(out, "no announcements")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "KIND\tVERSION\tSEVERITY\tANNOUNCED\tID")
			for _, a := range anns {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					a.Kind, a.Version, orDash(a.Severity), formatTime(a.AnnouncedAt), a.ID)
			}
			return tw.Flush()
		},
	}

	cmd.Flags().StringVar(&opts.Kind, "kind", "", "only this kind")
	cmd.Flags().IntVar(&opts.Limit, "limit", 0, "maximum number of announcements to return")
	addUpdatesClientFlags(cmd, &opts.updatesClientOptions)
	return cmd
}

func runUpdatesList(ctx context.Context, opts updatesListOptions) ([]updates.Announcement, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := opts.nodeClient(ctx)
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	if opts.Kind != "" {
		q.Set("kind", opts.Kind)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}

	var out struct {
		Announcements []updates.Announcement `json:"announcements"`
	}
	if err := client.doJSON(ctx, http.MethodGet, "/api/updates"+encodeQuery(q), nil, &out); err != nil {
		return nil, err
	}
	return out.Announcements, nil
}

type updatesPendingOptions struct {
	updatesClientOptions
	Kind   string
	Worker string
	Quiet  bool
}

func newUpdatesPendingCmd() *cobra.Command {
	var opts updatesPendingOptions

	cmd := &cobra.Command{
		Use:   "pending",
		Short: "Show announcements this node has not resolved",
		Long: "Show the announcements this node has neither applied nor declined.\n\n" +
			"An announcement stays pending until this node reports `applied` or\n" +
			"`declined` against it. Acknowledging or failing leaves it listed,\n" +
			"since neither says the matter is settled.\n\n" +
			"This is a convenience, not an authority: the announcements are already\n" +
			"in this node's inbox, and a node is free never to call this.\n\n" +
			"You may read your own; reading another node's requires the admin\n" +
			"scope.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pending, err := runUpdatesPending(cmd.Context(), opts)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			if opts.Quiet {
				for _, p := range pending {
					fmt.Fprintln(out, p.Announcement.Kind)
				}
				return nil
			}
			if len(pending) == 0 {
				fmt.Fprintln(out, "nothing pending")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "KIND\tVERSION\tSEVERITY\tMY VERSION\tMY STATUS\tANNOUNCEMENT")
			for _, p := range pending {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
					p.Announcement.Kind, p.Announcement.Version, orDash(p.Announcement.Severity),
					orDash(p.CurrentVersion), p.Status, p.Announcement.ID)
			}
			return tw.Flush()
		},
	}

	cmd.Flags().StringVar(&opts.Kind, "kind", "", "only this kind")
	cmd.Flags().StringVar(&opts.Worker, "worker", "", "node to inspect (default: the calling identity; others need admin)")
	cmd.Flags().BoolVar(&opts.Quiet, "quiet", false, "print only the pending kinds, one per line")
	addUpdatesClientFlags(cmd, &opts.updatesClientOptions)
	return cmd
}

func runUpdatesPending(ctx context.Context, opts updatesPendingOptions) ([]updates.Pending, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := opts.nodeClient(ctx)
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	if opts.Worker != "" {
		q.Set("worker_id", opts.Worker)
	}

	var out struct {
		Pending []updates.Pending `json:"pending"`
	}
	if err := client.doJSON(ctx, http.MethodGet, "/api/updates/pending"+encodeQuery(q), nil, &out); err != nil {
		return nil, err
	}

	if opts.Kind == "" {
		return out.Pending, nil
	}
	var filtered []updates.Pending
	for _, p := range out.Pending {
		if p.Announcement.Kind == opts.Kind {
			filtered = append(filtered, p)
		}
	}
	return filtered, nil
}

type updatesReportOptions struct {
	updatesClientOptions

	Kind           string
	Status         string
	AnnouncementID string
	CurrentVersion string
	TargetVersion  string
	Detail         string
}

func newUpdatesReportCmd() *cobra.Command {
	var opts updatesReportOptions

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Report this node's version or update progress",
		Long: "Report what this node is running, or how an update is going.\n\n" +
			"Statuses:\n" +
			"  current       just declaring a version; no announcement involved\n" +
			"  acknowledged  seen it, it applies, not started\n" +
			"  in_progress   applying now\n" +
			"  applied       done — pass the new --current-version\n" +
			"  declined      seen it, deliberately not applying (say why in --detail)\n" +
			"  failed        tried, did not work (say why in --detail)\n\n" +
			"`declined` is a legitimate answer, not a failure. Robot Dreams records\n" +
			"what you say about your own runtime and does not second-guess it: it\n" +
			"never checks that a version is plausible or that `applied` matches\n" +
			"what was announced.\n\n" +
			"The report is always about the calling identity. There is no flag to\n" +
			"report on another node's behalf.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := runUpdatesReport(cmd.Context(), opts); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "reported %s %s\n", opts.Kind, opts.Status)
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Kind, "kind", "", "the kind this report is about")
	cmd.Flags().StringVar(&opts.Status, "status", "", "current|acknowledged|in_progress|applied|declined|failed")
	cmd.Flags().StringVar(&opts.AnnouncementID, "announcement-id", "", "the announcement being reported against")
	cmd.Flags().StringVar(&opts.CurrentVersion, "current-version", "", "the version now running (defaults to this CLI's own version for "+updates.KindCLI+")")
	cmd.Flags().StringVar(&opts.TargetVersion, "target-version", "", "the version being moved toward")
	cmd.Flags().StringVar(&opts.Detail, "detail", "", "free text explaining the status")
	addUpdatesClientFlags(cmd, &opts.updatesClientOptions)

	_ = cmd.MarkFlagRequired("kind")
	_ = cmd.MarkFlagRequired("status")
	return cmd
}

func runUpdatesReport(ctx context.Context, opts updatesReportOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}

	if opts.Kind == updates.KindCLI && opts.CurrentVersion == "" {
		opts.CurrentVersion = version
	}

	client, err := opts.nodeClient(ctx)
	if err != nil {
		return err
	}

	body := map[string]string{"kind": opts.Kind, "status": opts.Status}
	for k, v := range map[string]string{
		"announcement_id": opts.AnnouncementID,
		"current_version": opts.CurrentVersion,
		"target_version":  opts.TargetVersion,
		"detail":          opts.Detail,
	} {
		if v != "" {
			body[k] = v
		}
	}
	return client.doJSON(ctx, http.MethodPost, "/api/updates/reports", body, nil)
}

type updatesRolloutOptions struct {
	updatesAdminOptions
	Kind           string
	AnnouncementID string
}

func newUpdatesRolloutCmd() *cobra.Command {
	var opts updatesRolloutOptions

	cmd := &cobra.Command{
		Use:   "rollout",
		Short: "Show how a rollout is going across the fleet (admin)",
		Long: "Show every node's state for one kind.\n\n" +
			"Nodes that have said nothing appear as `unknown` rather than being\n" +
			"omitted — they are usually the ones worth looking at. Each row also\n" +
			"carries the node's connectivity, so a node that is not answering\n" +
			"because it is down can be told from one that is ignoring the\n" +
			"announcement.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rollout, err := runUpdatesRollout(cmd.Context(), opts)
			if err != nil {
				return err
			}
			printRollout(cmd.OutOrStdout(), rollout)
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Kind, "kind", updates.KindCLI, "the kind to report on")
	cmd.Flags().StringVar(&opts.AnnouncementID, "announcement-id", "", "a specific announcement (default: the most recent for this kind)")
	addUpdatesAdminFlags(cmd, &opts.updatesAdminOptions)
	return cmd
}

func runUpdatesRollout(ctx context.Context, opts updatesRolloutOptions) (updates.Rollout, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := opts.adminClient()
	if err != nil {
		return updates.Rollout{}, err
	}

	q := url.Values{}
	q.Set("kind", opts.Kind)
	if opts.AnnouncementID != "" {
		q.Set("announcement_id", opts.AnnouncementID)
	}

	var rollout updates.Rollout
	if err := client.doJSON(ctx, http.MethodGet, "/api/updates/rollout"+encodeQuery(q), nil, &rollout); err != nil {
		return updates.Rollout{}, err
	}
	return rollout, nil
}

var rolloutStatusOrder = []string{
	updates.StatusApplied, updates.StatusInProgress, updates.StatusAcknowledged,
	updates.StatusDeclined, updates.StatusFailed, updates.StatusCurrent, updates.StatusUnknown,
}

func printRollout(out io.Writer, rollout updates.Rollout) {
	if rollout.Announcement != nil {
		a := rollout.Announcement
		fmt.Fprintf(out, "kind %s, announcement %s (%s, announced %s)\n",
			a.Kind, a.ID, a.Version, formatTime(a.AnnouncedAt))
	} else {
		fmt.Fprintf(out, "kind %s — nothing announced yet\n", rollout.Kind)
	}

	var counts []string
	for _, s := range rolloutStatusOrder {
		if n := rollout.Counts[s]; n > 0 {
			counts = append(counts, fmt.Sprintf("%s %d", s, n))
		}
	}
	if len(counts) > 0 {
		fmt.Fprintf(out, "  %s\n", strings.Join(counts, "  "))
	}
	if len(rollout.Nodes) == 0 {
		fmt.Fprintln(out, "\nno nodes registered")
		return
	}

	fmt.Fprintln(out)
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NODE\tCONNECTIVITY\tMY VERSION\tUPDATE STATUS\tREPORTED\tDETAIL")
	for _, n := range rollout.Nodes {
		reported := "-"
		if !n.ReportedAt.IsZero() {
			reported = formatTime(n.ReportedAt)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			n.WorkerID, orDash(n.WorkerStatus), orDash(n.CurrentVersion),
			n.Status, reported, orDash(n.Detail))
	}
	_ = tw.Flush()
}

func newUpdatesContractCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "contract",
		Short: "Print the update base contract, written for an agent to read",
		Long: "Print the Robot Dreams update base contract: the message a node can\n" +
			"receive, what it may say back, and the procedure it is invited (not\n" +
			"required) to follow.\n\n" +
			"Written for an AI agent as the reader, in the same spirit as\n" +
			"`dream node onboard`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			printUpdateContract(cmd.OutOrStdout())
			return nil
		},
	}
}

func printUpdateContract(out io.Writer) {
	p := func(format string, args ...any) { fmt.Fprintf(out, format+"\n", args...) }

	p("# Robot Dreams — update base contract")
	p("")
	p("Robot Dreams may tell you that a new version of something is available.")
	p("It will never install anything for you and will never restart you. What")
	p("you do about an announcement is entirely your call, including nothing.")
	p("")

	p("## What arrives")
	p("")
	p("An ordinary status_update message in your inbox, from %q, with subject", server.ControlWorkerID)
	p("%q and a JSON body:", updates.SubjectUpdateAvailable)
	p("")
	p("  {")
	p("    \"announcement_id\": \"...\",   // quote this when you report back")
	p("    \"kind\":            \"...\",   // WHAT is being updated — filter on this")
	p("    \"version\":         \"...\",   // opaque; Robot Dreams never parses it")
	p("    \"source\":          \"...\",   // where it lives; never fetched for you")
	p("    \"min_version\":     \"...\",   // advisory, optional")
	p("    \"severity\":        \"...\",   // optional|recommended|required, advisory")
	p("    \"notes\":           \"...\"    // this deployment's own guidance to you")
	p("  }")
	p("")
	p("Kinds are namespaced as <namespace>/<name>. %q is the dream", updates.KindCLI)
	p("binary itself. Every other kind is defined by this deployment — if you do")
	p("not recognize a kind, ignore it. That is a normal outcome, not an error.")
	p("")

	p("## What you can say back")
	p("")
	p("  dream updates pending    # what is announced that you have not resolved")
	p("  dream updates report --kind K --status S [--announcement-id ID]")
	p("")
	p("Statuses:")
	p("  %-14s just declaring what version you run; no announcement needed", updates.StatusCurrent)
	p("  %-14s seen it, it applies to you, not started", updates.StatusAcknowledged)
	p("  %-14s applying now", updates.StatusInProgress)
	p("  %-14s done — pass your new --current-version", updates.StatusApplied)
	p("  %-14s seen it, deliberately not applying (say why in --detail)", updates.StatusDeclined)
	p("  %-14s tried, did not work (say why in --detail)", updates.StatusFailed)
	p("")
	p("Reporting is how a human sees the fleet's state. Declining is a")
	p("legitimate answer and is recorded as such.")
	p("")

	p("## The procedure — recommended, NOT enforced")
	p("")
	p("Nothing checks that you do any of this, and you may deviate whenever your")
	p("own judgment says otherwise:")
	p("")
	p("  1. Report \"%s\".", updates.StatusAcknowledged)
	p("  2. Stop accepting NEW work.")
	p("  3. Let work already in flight finish. If that will take a while, report")
	p("     \"%s\" so nobody thinks you are stuck.", updates.StatusInProgress)
	p("  4. Apply the update. For %s, that is `dream self-update`;", updates.KindCLI)
	p("     for any other kind, whatever applying it means in your runtime.")
	p("  5. Restart yourself if the update needs it.")
	p("  6. Resume accepting work, and report \"%s\" with your new version.", updates.StatusApplied)
	p("")
	p("If it goes wrong, report \"%s\" with --detail and carry on doing your", updates.StatusFailed)
	p("job on the old version. A node that cannot update is still a working node.")
	p("")

	p("## Announcements wait for you")
	p("")
	p("Delivery is durable. If you were offline when something was announced, it")
	p("is in your inbox when you reconnect — so you will not miss one by being")
	p("away, and you may see one you have already dealt with. Reporting")
	p("\"%s\" or \"%s\" is what settles it.", updates.StatusApplied, updates.StatusDeclined)
}

func encodeQuery(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

type updateRequest struct {
	Kind           string
	Version        string
	AnnouncementID string
	MessageID      string
}

func (r *updateRequest) Error() string {
	return fmt.Sprintf("update announced: %s %s", r.Kind, r.Version)
}

func updateRequestFor(env envelopeView) (*updateRequest, bool) {
	if env.From != server.ControlWorkerID || env.Subject != updates.SubjectUpdateAvailable {
		return nil, false
	}
	var ann updates.Announcement
	if err := json.Unmarshal(env.Body, &ann); err != nil {
		return nil, false
	}
	if ann.Kind != updates.KindCLI || ann.Version == "" {
		return nil, false
	}
	return &updateRequest{
		Kind:           ann.Kind,
		Version:        ann.Version,
		AnnouncementID: ann.ID,
		MessageID:      env.ID,
	}, true
}
