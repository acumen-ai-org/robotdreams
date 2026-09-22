package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/acumen-ai-org/robotdreams/internal/identity"
	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/internal/server/store"
	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

func newWorkerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "worker",
		Aliases: []string{"node"},
		Short:   "Connect and manage workers (nodes) against a control plane",
	}
	cmd.AddCommand(newWorkerConnectCmd(), newWorkerDelegateCmd(), newWorkerEditCmd(), newWorkerDeleteCmd(), newWorkerReassignCmd(), newWorkerListCmd(), newWorkerOnboardCmd(), newWorkerAppCmd())
	return cmd
}

type workerConnectOptions struct {
	Server         string
	WorkerID       string
	Role           string
	ReportsTo      string
	Metadata       map[string]string
	Versions       map[string]string
	AppURL         string
	AppDescription string
	AdminToken     string
}

type workerConnectResult struct {
	ServerID  string
	ServerURL string
	WorkerID  string
	Role      string
	ReportsTo string
	KeyPath   string
	ExpiresAt time.Time
}

func newWorkerConnectCmd() *cobra.Command {
	var opts workerConnectOptions

	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Register this worker with a control plane",
		Long: "Register this worker with a control plane.\n\n" +
			"Generates (or reuses) an Ed25519 identity at\n" +
			"~/.dream/<server-id>/identity.key, proves possession of it against a\n" +
			"server-issued nonce, registers the worker, and records\n" +
			"~/.dream/<server-id>/config.json so later commands can default\n" +
			"--server and --worker-id.\n\n" +
			"<server-id> is derived from --server: the scheme is stripped and every\n" +
			"character outside [A-Za-z0-9._-] becomes '_', so 127.0.0.1:7420 maps to\n" +
			"127.0.0.1_7420.\n\n" +
			"Environment: $" + envDreamURL + " supplies the server address and $" + envDreamToken + " the\n" +
			"admin/enrollment credential when the corresponding flag is not passed\n" +
			"(precedence: flag > environment > local auto-discovery), so a runtime\n" +
			"with those two variables set needs no flags beyond --worker-id/--role.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := runWorkerConnect(cmd.Context(), opts)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "connected %s to %s\n", res.WorkerID, res.ServerURL)
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintf(tw, "  role\t%s\n", orDash(res.Role))
			fmt.Fprintf(tw, "  reports to\t%s\n", parentLabel(res.ReportsTo))
			fmt.Fprintf(tw, "  identity\t%s\n", res.KeyPath)
			fmt.Fprintf(tw, "  token expires\t%s\n", formatTime(res.ExpiresAt))
			return tw.Flush()
		},
	}

	cmd.Flags().StringVar(&opts.Server, "server", "", "control plane address, e.g. 127.0.0.1:7420 "+
		"(default: $"+envDreamURL+"; required if that is unset)")
	cmd.Flags().StringVar(&opts.WorkerID, "worker-id", "", "worker id (default: this machine's hostname)")
	cmd.Flags().StringVar(&opts.Role, "role", "", "role label recorded in the org chart")
	cmd.Flags().StringVar(&opts.ReportsTo, "reports-to", "", "parent worker id; empty means report directly to root")
	cmd.Flags().StringToStringVar(&opts.Metadata, "metadata", nil, "arbitrary metadata, e.g. --metadata team=infra,tier=1")
	cmd.Flags().StringVar(&opts.AppURL, "app-url", "",
		"URL of something this node serves for a human to open (http/https), shown in Mission Control")
	cmd.Flags().StringVar(&opts.AppDescription, "app-description", "",
		"one line describing what --app-url serves")
	cmd.Flags().StringToStringVar(&opts.Versions, "version-of", nil,
		"versions this node runs, as kind=version (e.g. --version-of acme/prompt-pack=3); "+
			updates.KindCLI+" is declared automatically")
	cmd.Flags().StringVar(&opts.AdminToken, "admin-token", "", "admin or enrollment token authorizing registration "+
		"(precedence: this flag, then $"+envDreamToken+", then auto-discovery from the local server's "+
		"data dir when --server is a loopback address)")

	return cmd
}

func runWorkerConnect(ctx context.Context, opts workerConnectOptions) (workerConnectResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	opts.Server = serverAddrOrEnv(opts.Server)
	if opts.Server == "" {
		return workerConnectResult{}, fmt.Errorf("--server is required (or set %s)", envDreamURL)
	}

	workerID := opts.WorkerID
	if workerID == "" {
		host, err := os.Hostname()
		if err != nil {
			return workerConnectResult{}, fmt.Errorf("no --worker-id given and hostname is unavailable: %w", err)
		}
		workerID = host
	}

	serverID := serverIDFromAddr(opts.Server)
	baseURL := baseURLFromAddr(opts.Server)

	keyPath, err := identity.DefaultKeyPath(serverID)
	if err != nil {
		return workerConnectResult{}, err
	}
	kp, err := identity.LoadOrGenerateKeyPair(keyPath)
	if err != nil {
		return workerConnectResult{}, err
	}

	hc := defaultHTTPClient()
	nonce, nonceB64, err := requestChallenge(ctx, hc, baseURL, workerID)
	if err != nil {
		return workerConnectResult{}, err
	}

	req := map[string]any{
		"worker_id":  workerID,
		"role":       opts.Role,
		"reports_to": opts.ReportsTo,
		"public_key": base64.StdEncoding.EncodeToString(kp.PublicKey),
		"nonce":      nonceB64,
		"signature":  base64.StdEncoding.EncodeToString(kp.SignNonce(nonce)),
	}
	if len(opts.Metadata) > 0 {
		req["metadata"] = opts.Metadata
	}

	adminToken := opts.AdminToken
	if adminToken == "" {
		adminToken = dreamTokenFromEnv()
	}
	if adminToken == "" {
		adminToken = discoverLocalEnrollmentToken(opts.Server)
	}

	var tok tokenResponse
	if err := postJSON(ctx, hc, baseURL+"/api/workers", adminToken, req, &tok); err != nil {
		return workerConnectResult{}, err
	}

	cfg := localConfig{
		ServerAddr: opts.Server,
		WorkerID:   workerID,
		Role:       opts.Role,
		ReportsTo:  opts.ReportsTo,
	}
	if err := saveLocalConfig(serverID, cfg); err != nil {
		return workerConnectResult{}, err
	}

	declareVersions(ctx, baseURL, workerID, kp, opts.Versions)
	declareApp(ctx, baseURL, workerID, kp, opts.AppURL, opts.AppDescription)

	return workerConnectResult{
		ServerID:  serverID,
		ServerURL: baseURL,
		WorkerID:  workerID,
		Role:      opts.Role,
		ReportsTo: opts.ReportsTo,
		KeyPath:   keyPath,
		ExpiresAt: tok.ExpiresAt,
	}, nil
}

func declareVersions(ctx context.Context, baseURL, workerID string, kp identity.KeyPair, extra map[string]string) {
	declared := map[string]string{updates.KindCLI: version}
	for kind, v := range extra {
		declared[kind] = v
	}

	hc := defaultHTTPClient()
	tok, err := fetchToken(ctx, hc, baseURL, workerID, kp)
	if err != nil {
		return
	}
	client := &apiClient{baseURL: baseURL, token: tok.Raw, hc: hc}

	for kind, v := range declared {
		if err := updates.ValidateKind(kind); err != nil {
			fmt.Fprintf(os.Stderr, "warning: not declaring version for %q: %v\n", kind, err)
			continue
		}
		body := map[string]string{
			"kind":            kind,
			"status":          updates.StatusCurrent,
			"current_version": v,
		}
		if err := client.doJSON(ctx, http.MethodPost, "/api/updates/reports", body, nil); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not declare %s version %s: %v\n", kind, v, err)
		}
	}
}

type workerEditOptions struct {
	Target    string
	Role      string
	RoleGiven bool
	Server    string
	ServerID  string
	AsWorker  string
}

func newWorkerEditCmd() *cobra.Command {
	var opts workerEditOptions

	cmd := &cobra.Command{
		Use:   "edit <worker-id>",
		Short: "Change what a worker is on the org chart",
		Long: "Change what a worker is on the org chart.\n\n" +
			"Today the one editable field is --role, the free-form label the control\n" +
			"plane records and Mission Control draws an icon from. A role is\n" +
			"otherwise uninterpreted: changing it relabels the node, it does not\n" +
			"move it or change what it may do.\n\n" +
			"Until now a role could only be set at `dream worker connect`, and a\n" +
			"second connect is refused while the worker is registered, so a\n" +
			"mislabelled node stayed mislabelled. This is the way to correct one.\n\n" +
			"The call is authenticated as the LOCAL identity selected by --server /\n" +
			"--server-id / --worker-id, and the control plane allows the edit only\n" +
			"when the caller is the worker itself, the worker's parent, or holds the\n" +
			"admin scope — the same rule as `dream worker reassign`.\n\n" +
			"An empty --role clears the label, which draws the default ⬡ icon.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Target = args[0]
			opts.RoleGiven = cmd.Flags().Changed("role")
			w, err := runWorkerEdit(cmd.Context(), opts)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s is now %s\n", w.ID, orDash(w.Role))
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Role, "role", "", "new role label; empty clears it")
	cmd.Flags().StringVar(&opts.Server, "server", "", "control plane address (default: $"+envDreamURL+", else from local config)")
	cmd.Flags().StringVar(&opts.ServerID, "server-id", "", "local identity directory under ~/.dream to use")
	cmd.Flags().StringVar(&opts.AsWorker, "worker-id", "", "local worker identity to authenticate as (default: from local config)")

	return cmd
}

func runWorkerEdit(ctx context.Context, opts workerEditOptions) (workerView, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !opts.RoleGiven {
		return workerView{}, fmt.Errorf("nothing to edit: pass --role")
	}
	target, err := resolveTarget(opts.Server, opts.ServerID, opts.AsWorker)
	if err != nil {
		return workerView{}, err
	}
	client, err := target.oneShotClient(ctx)
	if err != nil {
		return workerView{}, err
	}

	var out workerView
	err = client.doJSON(ctx, http.MethodPatch, "/api/workers/"+opts.Target,
		map[string]string{"role": opts.Role}, &out)
	if err != nil {
		return workerView{}, err
	}
	return out, nil
}

type workerDeleteOptions struct {
	Target     string
	Reason     string
	Cascade    bool
	Server     string
	DataDir    string
	AdminToken string
}

type workerDeleteResult struct {
	WorkerID string   `json:"worker_id"`
	Removed  []string `json:"removed"`
	Reason   string   `json:"reason"`
}

func newWorkerDeleteCmd() *cobra.Command {
	var opts workerDeleteOptions

	cmd := &cobra.Command{
		Use:   "delete <worker-id>",
		Short: "Remove a worker from the org chart and revoke its identity",
		Long: "Remove a worker from the org chart and revoke its identity.\n\n" +
			"Deleting is permanent and does two things at once: the worker leaves\n" +
			"the chart, and its identity is revoked, so it cannot reconnect until\n" +
			"an admin enrolls it again. To correct a label instead, use `dream\n" +
			"worker edit`; to move a worker, use `dream worker reassign`.\n\n" +
			"By default the worker's direct reports move up to its own parent, so\n" +
			"the chart stays connected. With --cascade every worker beneath it is\n" +
			"deleted and revoked as well. The command prints everything it removed.\n\n" +
			"Because a delete revokes, this call needs the admin scope, not a\n" +
			"worker identity. Precedence: --admin-token, then $" + envDreamToken + ",\n" +
			"then a token minted from the local server's data dir when --server is\n" +
			"a loopback address. Minting locally needs filesystem access to the\n" +
			"server's state, which is already equivalent to admin control; see\n" +
			"docs/security-model.md.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Target = args[0]
			res, err := runWorkerDelete(cmd.Context(), opts)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, id := range res.Removed {
				fmt.Fprintf(out, "removed %s\n", id)
			}
			fmt.Fprintf(out, "%d removed and revoked\n", len(res.Removed))
			return nil
		},
	}

	cmd.Flags().BoolVar(&opts.Cascade, "cascade", false, "delete every worker beneath this one as well")
	cmd.Flags().StringVar(&opts.Reason, "reason", "", "reason recorded against the revocation")
	cmd.Flags().StringVar(&opts.Server, "server", "", "control plane address (default: $"+envDreamURL+", else 127.0.0.1"+server.DefaultAddr+")")
	cmd.Flags().StringVar(&opts.DataDir, "data-dir", "", "control plane state directory used to mint a local admin token (default ~/.dream/_server)")
	cmd.Flags().StringVar(&opts.AdminToken, "admin-token", "", "admin token authorizing the delete "+
		"(precedence: this flag, then $"+envDreamToken+", then a token minted from --data-dir "+
		"when --server is a loopback address)")

	return cmd
}

func runWorkerDelete(ctx context.Context, opts workerDeleteOptions) (workerDeleteResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	addr := serverAddrOrEnv(opts.Server)
	if addr == "" {
		addr = "127.0.0.1" + server.DefaultAddr
	}

	token := opts.AdminToken
	if token == "" {
		token = dreamTokenFromEnv()
	}
	if token == "" {
		token = discoverLocalAdminToken(addr, opts.DataDir)
	}
	if token == "" {
		return workerDeleteResult{}, fmt.Errorf("no admin credential: pass --admin-token, set %s, or run against a local server", envDreamToken)
	}

	path := "/api/workers/" + opts.Target
	query := url.Values{}
	if opts.Cascade {
		query.Set("cascade", "true")
	}
	if opts.Reason != "" {
		query.Set("reason", opts.Reason)
	}
	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	client := &apiClient{baseURL: baseURLFromAddr(addr), token: token, hc: defaultHTTPClient()}

	var out workerDeleteResult
	if err := client.doJSON(ctx, http.MethodDelete, path, nil, &out); err != nil {
		return workerDeleteResult{}, err
	}
	return out, nil
}

func discoverLocalAdminToken(serverAddr, dataDir string) string {
	if dataDir == "" {
		if !isLoopbackServerAddr(serverAddr) {
			return ""
		}
		d, err := defaultServerDataDir()
		if err != nil {
			return ""
		}
		dataDir = d
	}
	if _, err := os.Stat(filepath.Join(dataDir, store.DBFileName)); err != nil {
		return ""
	}
	token, err := mintLocalAdminToken(dataDir)
	if err != nil {
		return ""
	}
	return token
}

type workerReassignOptions struct {
	Target    string
	ReportsTo string
	Server    string
	ServerID  string
	AsWorker  string
}

func newWorkerReassignCmd() *cobra.Command {
	var opts workerReassignOptions

	cmd := &cobra.Command{
		Use:   "reassign <worker-id>",
		Short: "Move a worker under a new parent",
		Long: "Move a worker under a new parent.\n\n" +
			"The call is authenticated as the LOCAL identity selected by --server /\n" +
			"--server-id / --worker-id. The control plane allows a reassignment only\n" +
			"when the caller is the worker itself, the worker's CURRENT parent, or\n" +
			"holds the admin scope — so run from an ordinary worker's identity this\n" +
			"command can reassign that worker or one of its own direct reports, and\n" +
			"nothing else. Anything broader needs an admin credential, which this\n" +
			"phase only mints locally (see `dream server revoke`).\n\n" +
			"An empty --reports-to moves the worker to the root of the chart.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Target = args[0]
			w, err := runWorkerReassign(cmd.Context(), opts)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s now reports to %s\n", w.ID, parentLabel(w.ReportsTo))
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.ReportsTo, "reports-to", "", "new parent worker id; empty means report directly to root")
	cmd.Flags().StringVar(&opts.Server, "server", "", "control plane address (default: $"+envDreamURL+", else from local config)")
	cmd.Flags().StringVar(&opts.ServerID, "server-id", "", "local identity directory under ~/.dream to use")
	cmd.Flags().StringVar(&opts.AsWorker, "worker-id", "", "local worker identity to authenticate as (default: from local config)")

	return cmd
}

func runWorkerReassign(ctx context.Context, opts workerReassignOptions) (workerView, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	target, err := resolveTarget(opts.Server, opts.ServerID, opts.AsWorker)
	if err != nil {
		return workerView{}, err
	}
	client, err := target.oneShotClient(ctx)
	if err != nil {
		return workerView{}, err
	}

	var out workerView
	err = client.doJSON(ctx, http.MethodPost, "/api/workers/"+opts.Target+"/reassign",
		map[string]string{"reports_to": opts.ReportsTo}, &out)
	if err != nil {
		return workerView{}, err
	}
	return out, nil
}

type workerListOptions struct {
	Server   string
	ServerID string
	AsWorker string
}

func newWorkerListCmd() *cobra.Command {
	var opts workerListOptions

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List every worker in the org chart",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			workers, err := runWorkerList(cmd.Context(), opts)
			if err != nil {
				return err
			}
			return printWorkerTable(cmd.OutOrStdout(), workers)
		},
	}

	cmd.Flags().StringVar(&opts.Server, "server", "", "control plane address (default: $"+envDreamURL+", else from local config)")
	cmd.Flags().StringVar(&opts.ServerID, "server-id", "", "local identity directory under ~/.dream to use")
	cmd.Flags().StringVar(&opts.AsWorker, "worker-id", "", "local worker identity to authenticate as (default: from local config)")

	return cmd
}

func runWorkerList(ctx context.Context, opts workerListOptions) ([]workerView, error) {
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

	var out struct {
		Workers []workerView `json:"workers"`
	}
	if err := client.doJSON(ctx, http.MethodGet, "/api/workers", nil, &out); err != nil {
		return nil, err
	}
	return out.Workers, nil
}

func printWorkerTable(out io.Writer, workers []workerView) error {
	if len(workers) == 0 {
		fmt.Fprintln(out, "no workers connected")
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "WORKER\tROLE\tREPORTS TO\tSTATUS\tLAST SEEN")
	for _, w := range workers {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			w.ID, orDash(w.Role), parentLabel(w.ReportsTo), orDash(w.Status), formatTime(w.LastSeenAt))
	}
	return tw.Flush()
}

func discoverLocalEnrollmentToken(serverAddr string) string {
	if !isLoopbackServerAddr(serverAddr) {
		return ""
	}
	dataDir, err := defaultServerDataDir()
	if err != nil {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(dataDir, server.EnrollmentTokenFileName))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func isLoopbackServerAddr(addr string) bool {
	s := strings.TrimSpace(addr)
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "https://")
	host := s
	if h, _, err := net.SplitHostPort(s); err == nil {
		host = h
	}
	switch host {
	case "127.0.0.1", "::1", "localhost", "[::1]":
		return true
	default:
		return false
	}
}

func parentLabel(reportsTo string) string {
	if reportsTo == "" {
		return "(root)"
	}
	return reportsTo
}

func declareApp(ctx context.Context, baseURL, workerID string, kp identity.KeyPair, appURL, description string) {
	if appURL == "" {
		return
	}
	if _, err := server.ValidateAppURL(appURL); err != nil {
		fmt.Fprintf(os.Stderr, "warning: not declaring an app: %v\n", err)
		return
	}

	hc := defaultHTTPClient()
	tok, err := fetchToken(ctx, hc, baseURL, workerID, kp)
	if err != nil {
		return
	}
	client := &apiClient{baseURL: baseURL, token: tok.Raw, hc: hc}

	body := map[string]string{"url": appURL}
	if description != "" {
		body["description"] = description
	}
	if err := client.doJSON(ctx, http.MethodPost, "/api/apps", body, nil); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not declare the app: %v\n", err)
	}
}

type appView struct {
	WorkerID    string `json:"worker_id"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	DeclaredAt  string `json:"declared_at"`
}

type workerAppOptions struct {
	URL         string
	Description string

	Server   string
	ServerID string
	AsWorker string
}

func newWorkerAppCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app",
		Short: "Declare what this node serves for a human to open",
		Long: "Declare what this node serves for a human to open.\n\n" +
			"A node may advertise one app: a URL and a short line saying what it\n" +
			"is. Mission Control shows it on the node and offers to open it.\n\n" +
			"The control plane stores the claim and does not evaluate it — it\n" +
			"never fetches the URL and never checks that what is there is what\n" +
			"you said. It does require an http or https URL, because the\n" +
			"dashboard turns this into something an operator clicks.\n\n" +
			"You can set this at registration with `dream worker connect\n" +
			"--app-url`, but only then: a second connect for a live worker is\n" +
			"refused. Use this command to set or change it afterwards.",
	}
	cmd.AddCommand(newWorkerAppSetCmd(), newWorkerAppClearCmd(), newWorkerAppListCmd())
	return cmd
}

func addWorkerAppClientFlags(cmd *cobra.Command, opts *workerAppOptions) {
	cmd.Flags().StringVar(&opts.Server, "server", "", "control plane address (default: $"+envDreamURL+", else from local config)")
	cmd.Flags().StringVar(&opts.ServerID, "server-id", "", "local identity directory under ~/.dream to use")
	cmd.Flags().StringVar(&opts.AsWorker, "worker-id", "", "local worker identity to authenticate as (default: from local config)")
}

func newWorkerAppSetCmd() *cobra.Command {
	var opts workerAppOptions

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set this node's app",
		Long: "Set what this node serves, replacing anything it said before.\n\n" +
			"The app is always this node's own: the record follows the identity\n" +
			"the call authenticates as, and there is no way to declare an app on\n" +
			"another node's behalf.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := runWorkerAppSet(cmd.Context(), opts)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s serves %s\n", app.WorkerID, app.URL)
			if app.Description != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", app.Description)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.URL, "url", "", "http or https URL this node serves")
	cmd.Flags().StringVar(&opts.Description, "description", "", "one line describing what it is")
	addWorkerAppClientFlags(cmd, &opts)
	_ = cmd.MarkFlagRequired("url")
	return cmd
}

func runWorkerAppSet(ctx context.Context, opts workerAppOptions) (appView, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := server.ValidateAppURL(opts.URL); err != nil {
		return appView{}, err
	}

	target, err := resolveTarget(opts.Server, opts.ServerID, opts.AsWorker)
	if err != nil {
		return appView{}, err
	}
	client, err := target.oneShotClient(ctx)
	if err != nil {
		return appView{}, err
	}

	body := map[string]string{"url": opts.URL}
	if opts.Description != "" {
		body["description"] = opts.Description
	}
	var out appView
	if err := client.doJSON(ctx, http.MethodPost, "/api/apps", body, &out); err != nil {
		return appView{}, err
	}
	return out, nil
}

func newWorkerAppClearCmd() *cobra.Command {
	var opts workerAppOptions

	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Remove this node's app",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := runWorkerAppClear(cmd.Context(), opts); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "cleared")
			return nil
		},
	}
	addWorkerAppClientFlags(cmd, &opts)
	return cmd
}

func runWorkerAppClear(ctx context.Context, opts workerAppOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	target, err := resolveTarget(opts.Server, opts.ServerID, opts.AsWorker)
	if err != nil {
		return err
	}
	client, err := target.oneShotClient(ctx)
	if err != nil {
		return err
	}
	return client.doJSON(ctx, http.MethodDelete, "/api/apps", nil, nil)
}

func newWorkerAppListCmd() *cobra.Command {
	var opts workerAppOptions

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List every app the fleet has declared",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			apps, err := runWorkerAppList(cmd.Context(), opts)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(apps) == 0 {
				fmt.Fprintln(out, "no apps declared")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NODE\tURL\tDESCRIPTION")
			for _, a := range apps {
				fmt.Fprintf(tw, "%s\t%s\t%s\n", a.WorkerID, a.URL, orDash(a.Description))
			}
			return tw.Flush()
		},
	}
	addWorkerAppClientFlags(cmd, &opts)
	return cmd
}

func runWorkerAppList(ctx context.Context, opts workerAppOptions) ([]appView, error) {
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
	var out struct {
		Apps []appView `json:"apps"`
	}
	if err := client.doJSON(ctx, http.MethodGet, "/api/apps", nil, &out); err != nil {
		return nil, err
	}
	return out.Apps, nil
}

type workerDelegateOptions struct {
	Server, ServerID, AsWorker string
	Child                      string
	Scopes                     []string
	TTL                        time.Duration
	JSON                       bool
}

type delegateView struct {
	WorkerID    string    `json:"worker_id"`
	DelegatedBy string    `json:"delegated_by"`
	Scopes      []string  `json:"scopes"`
	Token       string    `json:"token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func newWorkerDelegateCmd() *cobra.Command {
	var opts workerDelegateOptions

	cmd := &cobra.Command{
		Use:   "delegate",
		Short: "Mint a short-lived token for an ephemeral child of this worker",
		Long: "Mint a short-lived token for an ephemeral child of this worker.\n\n" +
			"The lightweight join: no keypair, no org-chart entry, no refresh. The\n" +
			"child is known as <this-worker>~<child>, carries delegated_by, holds a\n" +
			"subset of this worker's scopes (never admin), and its token lasts at\n" +
			"most the server's delegated-token limit and never past this worker's\n" +
			"own token. Revoking this worker revokes every child.\n\n" +
			"By default only the token is printed, so a spawned process can be\n" +
			"handed it directly:\n\n" +
			"  DREAM_CHILD_TOKEN=$(dream worker delegate --child fetch-1 --ttl 2m)\n\n" +
			"The child then calls the same /api/* endpoints with\n" +
			"`Authorization: Bearer <token>`. A delegated token cannot delegate\n" +
			"further; a child that needs more time asks its parent for a new one.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := runWorkerDelegate(cmd.Context(), opts)
			if err != nil {
				return err
			}
			if opts.JSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(d)
			}
			fmt.Fprintln(cmd.OutOrStdout(), d.Token)
			fmt.Fprintf(cmd.ErrOrStderr(), "delegated %s until %s (scopes: %s)\n", d.WorkerID, d.ExpiresAt.Local().Format(time.RFC3339), strings.Join(d.Scopes, " "))
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Child, "child", "", "child name, [A-Za-z0-9._-]+ (default: random)")
	cmd.Flags().StringArrayVar(&opts.Scopes, "scope", nil, "scope the child should hold; repeatable (default: every scope this worker holds except admin)")
	cmd.Flags().DurationVar(&opts.TTL, "ttl", 0, "token lifetime (default: the server's delegated-token TTL)")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "print the whole credential as JSON instead of the bare token")
	cmd.Flags().StringVar(&opts.Server, "server", "", "control plane address (default: $"+envDreamURL+", else from local config)")
	cmd.Flags().StringVar(&opts.ServerID, "server-id", "", "local identity directory under ~/.dream to use")
	cmd.Flags().StringVar(&opts.AsWorker, "worker-id", "", "local worker identity to delegate from (default: from local config)")

	return cmd
}

func runWorkerDelegate(ctx context.Context, opts workerDelegateOptions) (delegateView, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	target, err := resolveTarget(opts.Server, opts.ServerID, opts.AsWorker)
	if err != nil {
		return delegateView{}, err
	}
	client, err := target.oneShotClient(ctx)
	if err != nil {
		return delegateView{}, err
	}

	req := map[string]any{}
	if opts.Child != "" {
		req["child"] = opts.Child
	}
	if len(opts.Scopes) > 0 {
		req["scopes"] = opts.Scopes
	}
	if opts.TTL > 0 {
		req["ttl_seconds"] = int(opts.TTL / time.Second)
	}

	var out delegateView
	if err := client.doJSON(ctx, http.MethodPost, "/api/workers/delegate", req, &out); err != nil {
		return delegateView{}, err
	}
	return out, nil
}
