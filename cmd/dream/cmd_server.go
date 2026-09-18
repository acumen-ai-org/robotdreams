package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/acumen-ai-org/robotdreams/internal/dashboard"
	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/internal/server/api"
)

const defaultServerDataDirName = "_server"

func defaultServerDataDir() (string, error) {
	root, err := dreamDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, defaultServerDataDirName), nil
}

func newServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Run and administer a Robot Dreams control plane",
	}
	cmd.AddCommand(newServerInitCmd(), newServerRevokeCmd(), newServerRotateEnrollmentCmd(), newServerGuideCmd())
	return cmd
}

var defaultListenAddr = "127.0.0.1" + server.DefaultAddr

type serverInitOptions struct {
	DataDir           string
	MessagingURI      string
	StorageURI        string
	OrgChartFile      string
	ReportsDir        string
	Addr              string
	NoDashboard       bool
	OpenEnrollment    bool
	TLSCert           string
	TLSKey            string
	Insecure          bool
	TokenTTL          time.Duration
	DelegatedTokenTTL time.Duration
	onListen          func(boundAddr string)
}

func newServerInitCmd() *cobra.Command {
	var opts serverInitOptions

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize and serve a control plane (zero-config by default)",
		Long: "Initialize a control plane data directory and serve the HTTP API from it.\n\n" +
			"With no flags at all this is the zero-config path: state goes to\n" +
			"~/.dream/_server, messaging uses the embedded SQLite queue and storage\n" +
			"uses the local filesystem, both inside that directory. Backend URIs\n" +
			"passed here are persisted, so a later flagless run keeps using them.\n\n" +
			"init both configures AND serves: it blocks until SIGINT or SIGTERM,\n" +
			"then shuts the HTTP server down gracefully and closes the backends.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runServerInit(ctx, opts, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&opts.DataDir, "data-dir", "", "control plane state directory (default ~/.dream/_server)")
	cmd.Flags().StringVar(&opts.MessagingURI, "messaging", "", "messaging backend URI (default: embedded queue in the data directory)")
	cmd.Flags().StringVar(&opts.StorageURI, "storage", "", "storage backend URI (default: local files in the data directory)")
	cmd.Flags().StringVar(&opts.OrgChartFile, "orgchart", "", "YAML org chart to bulk-load on a first-ever startup")
	cmd.Flags().StringVar(&opts.ReportsDir, "reports", "", "report definition library directory to load (e.g. reporting/library)")
	cmd.Flags().StringVar(&opts.Addr, "addr", defaultListenAddr,
		"listen address; a non-loopback address needs --tls-cert/--tls-key or --insecure")
	cmd.Flags().StringVar(&opts.TLSCert, "tls-cert", "", "PEM certificate file: serve HTTPS directly (with --tls-key)")
	cmd.Flags().StringVar(&opts.TLSKey, "tls-key", "", "PEM private key file for --tls-cert")
	cmd.Flags().BoolVar(&opts.Insecure, "insecure", false,
		"serve plaintext HTTP on a non-loopback --addr (only behind a TLS proxy or on a trusted network)")
	cmd.Flags().BoolVar(&opts.NoDashboard, "no-dashboard", false, "do not serve Mission Control at /dashboard")
	cmd.Flags().BoolVar(&opts.OpenEnrollment, "open-enrollment", false,
		"disable worker-enrollment authorization (dev/test only; do not use on a network-reachable server)")
	cmd.Flags().DurationVar(&opts.TokenTTL, "token-ttl", 0,
		fmt.Sprintf("lifetime of minted worker tokens (default %s)", server.DefaultTokenTTL))
	cmd.Flags().DurationVar(&opts.DelegatedTokenTTL, "delegated-token-ttl", 0,
		fmt.Sprintf("default and maximum lifetime of a delegated child token; a child is still "+
			"capped at its parent's remaining lifetime (default %s)", server.DefaultDelegatedTokenTTL))

	return cmd
}

func runServerInit(ctx context.Context, opts serverInitOptions, out io.Writer) error {
	dataDir := opts.DataDir
	if dataDir == "" {
		d, err := defaultServerDataDir()
		if err != nil {
			return err
		}
		dataDir = d
	}
	dataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return fmt.Errorf("resolve --data-dir: %w", err)
	}

	for name, uri := range map[string]string{"--messaging": opts.MessagingURI, "--storage": opts.StorageURI} {
		if uri == "" {
			continue
		}
		if err := server.ValidateBackendURI(uri); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}

	addr := opts.Addr
	if addr == "" {
		addr = defaultListenAddr
	}

	if opts.TokenTTL < 0 || opts.DelegatedTokenTTL < 0 {
		return errors.New("--token-ttl and --delegated-token-ttl must not be negative")
	}

	useTLS, err := checkTransportSecurity(addr, opts.TLSCert, opts.TLSKey, opts.Insecure)
	if err != nil {
		return err
	}

	srv, err := server.New(server.Config{
		DataDir:           dataDir,
		MessagingURI:      opts.MessagingURI,
		StorageURI:        opts.StorageURI,
		Addr:              addr,
		OrgChartFile:      opts.OrgChartFile,
		ReportsDir:        opts.ReportsDir,
		OpenEnrollment:    opts.OpenEnrollment,
		TokenTTL:          opts.TokenTTL,
		DelegatedTokenTTL: opts.DelegatedTokenTTL,
	})
	if err != nil {
		return err
	}
	defer srv.Close()

	ln, err := net.Listen("tcp", srv.Addr())
	if err != nil {
		return fmt.Errorf("listen on %s: %w", srv.Addr(), err)
	}
	if opts.onListen != nil {
		opts.onListen(ln.Addr().String())
	}

	apiLayer := api.New(srv)
	handler := api.NewAPIRouter(apiLayer)
	if !opts.NoDashboard {
		handler = withDashboardMounted(handler)
	}

	printStartupBanner(out, srv, dataDir, ln.Addr().String(), !opts.NoDashboard, useTLS)

	reqCtx, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()

	httpSrv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		BaseContext:       func(net.Listener) context.Context { return reqCtx },
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}
	schedCtx, stopScheduler := context.WithCancel(ctx)
	schedDone := make(chan struct{})
	go func() {
		defer close(schedDone)
		apiLayer.RunScheduler(schedCtx, schedulerTick)
	}()
	defer func() {
		stopScheduler()
		<-schedDone
	}()

	errCh := make(chan error, 1)
	go func() {
		var err error
		if useTLS {
			err = httpSrv.ServeTLS(ln, opts.TLSCert, opts.TLSKey)
		} else {
			err = httpSrv.Serve(ln)
		}
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	fmt.Fprintln(out, "\nshutting down...")
	stopScheduler()
	if err := drainThenCancelRequests(httpSrv, cancelRequests); err != nil {
		return err
	}
	return <-errCh
}

func withDashboardMounted(apiHandler http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/dashboard/", dashboard.Mount("/dashboard", ""))
	mux.Handle("/", apiHandler)
	return mux
}

func drainThenCancelRequests(httpSrv *http.Server, cancelRequests context.CancelFunc) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	shutdownErr := make(chan error, 1)
	go func() { shutdownErr <- httpSrv.Shutdown(shutdownCtx) }()
	select {
	case err := <-shutdownErr:
		if err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return nil
	case <-time.After(shutdownDrain):
	}
	cancelRequests()
	if err := <-shutdownErr; err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

func checkTransportSecurity(addr, certFile, keyFile string, insecure bool) (useTLS bool, err error) {
	switch {
	case certFile != "" && keyFile != "":
		for name, f := range map[string]string{"--tls-cert": certFile, "--tls-key": keyFile} {
			if _, err := os.Stat(f); err != nil {
				return false, fmt.Errorf("%s: %w", name, err)
			}
		}
		if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
			return false, fmt.Errorf("--tls-cert/--tls-key: %w", err)
		}
		return true, nil
	case certFile != "" || keyFile != "":
		return false, errors.New("--tls-cert and --tls-key must be given together")
	}

	loopback, err := listenAddrIsLoopback(addr)
	if err != nil {
		return false, fmt.Errorf("--addr %q: %w", addr, err)
	}
	if loopback || insecure {
		return false, nil
	}
	return false, fmt.Errorf("refusing to serve plaintext HTTP on %s, which is reachable from other machines: "+
		"every request carries a bearer token. Either pass --tls-cert <cert.pem> --tls-key <key.pem> "+
		"to serve HTTPS, put a TLS-terminating proxy in front and pass --insecure, or bind loopback "+
		"(--addr %s) for this machine only", addr, defaultListenAddr)
}

func listenAddrIsLoopback(addr string) (bool, error) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return false, err
	}
	if host == "" {
		return false, nil
	}
	if strings.EqualFold(host, "localhost") {
		return true, nil
	}
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false, nil
	}
	return ip.IsLoopback(), nil
}

const (
	shutdownGrace     = 10 * time.Second
	shutdownDrain     = 2 * time.Second
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 2 * time.Minute
	schedulerTick     = 30 * time.Second
)

func printStartupBanner(out io.Writer, srv *server.Server, dataDir, boundAddr string, withDashboard, useTLS bool) {
	msgURI, stgURI := srv.BackendURIs()
	scheme := "http://"
	if useTLS {
		scheme = "https://"
	}

	fmt.Fprintf(out, "Robot Dreams control plane %s\n\n", version)
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  data dir\t%s\n", dataDir)
	fmt.Fprintf(tw, "  messaging\t%s\n", msgURI)
	fmt.Fprintf(tw, "  storage\t%s\n", stgURI)
	if dir := srv.ReportsDir(); dir != "" {
		fmt.Fprintf(tw, "  reports\t%s (%d definitions)\n", dir, len(srv.Reports().List()))
	}
	if useTLS {
		fmt.Fprintf(tw, "  listening\t%s (TLS)\n", boundAddr)
	} else {
		fmt.Fprintf(tw, "  listening\t%s (plaintext HTTP)\n", boundAddr)
	}
	if withDashboard {
		fmt.Fprintf(tw, "  dashboard\t%s%s/dashboard/\n", scheme, boundAddr)
	}
	fmt.Fprintf(tw, "  token TTL\t%s\n", srv.TokenTTL())
	fmt.Fprintf(tw, "  delegated token TTL\t%s\n", srv.DelegatedTokenTTL())
	if srv.OpenEnrollment() {
		fmt.Fprintf(tw, "  enrollment\topen (--open-enrollment: any caller may register a worker)\n")
	} else {
		fmt.Fprintf(tw, "  enrollment token\t%s\n", filepath.Join(dataDir, server.EnrollmentTokenFileName))
	}
	_ = tw.Flush()
	fmt.Fprintf(out, "\nConnect a worker on this machine with:\n  dream worker connect --server %s --role <label>\n", boundAddr)
	if !srv.OpenEnrollment() {
		fmt.Fprintf(out, "(worker connect on this machine auto-discovers the enrollment token above;\n"+
			"a remote worker needs it via DREAM_TOKEN or --admin-token)\n")
	}
	if loopback, _ := listenAddrIsLoopback(boundAddr); loopback && !useTLS {
		fmt.Fprintf(out, "This server is reachable from this machine only. To accept agents from\n"+
			"other machines, restart with --addr %s --tls-cert <cert.pem> --tls-key <key.pem>\n"+
			"(or --insecure behind a TLS proxy or on a trusted network).\n", server.DefaultAddr)
	}
	fmt.Fprintln(out)
	printAgentPunchline(out, suggestedServerURLWithScheme(boundAddr, useTLS), enrollmentTokenPath(dataDir), srv.OpenEnrollment())
	fmt.Fprintf(out, "\nFull setup guide: dream server guide\nPress Ctrl-C to stop.\n")
}

type serverRevokeOptions struct {
	WorkerID string
	DataDir  string
	Addr     string
	Reason   string
}

type revokeResult struct {
	WorkerID string `json:"worker_id"`
	Revoked  bool   `json:"revoked"`
	Reason   string `json:"reason"`
}

func newServerRevokeCmd() *cobra.Command {
	var opts serverRevokeOptions

	cmd := &cobra.Command{
		Use:   "revoke <worker-id>",
		Short: "Revoke a worker's credentials (local-admin bootstrap path)",
		Long: "Revoke a worker's credentials on a running control plane.\n\n" +
			"This mints an admin token locally from --data-dir (which requires\n" +
			"filesystem access to the server's state, and is therefore already\n" +
			"equivalent to admin control) and uses it against the running server\n" +
			"at --server. It is a bootstrap shortcut, not an administrator\n" +
			"identity model; see docs/security-model.md.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.WorkerID = args[0]
			res, err := runServerRevoke(cmd.Context(), opts)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "revoked %s (reason: %s)\n", res.WorkerID, res.Reason)
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.DataDir, "data-dir", "", "control plane state directory (default ~/.dream/_server)")
	cmd.Flags().StringVar(&opts.Addr, "server", "", "address of the running control plane "+
		"(default: $"+envDreamURL+", else 127.0.0.1"+server.DefaultAddr+")")
	cmd.Flags().StringVar(&opts.Reason, "reason", "", "reason recorded with the revocation")

	return cmd
}

const adminTokenTTL = 2 * time.Minute

func runServerRevoke(ctx context.Context, opts serverRevokeOptions) (revokeResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.WorkerID == "" {
		return revokeResult{}, fmt.Errorf("worker id is required")
	}

	dataDir := opts.DataDir
	if dataDir == "" {
		d, err := defaultServerDataDir()
		if err != nil {
			return revokeResult{}, err
		}
		dataDir = d
	}

	token, err := mintLocalAdminToken(dataDir)
	if err != nil {
		return revokeResult{}, err
	}

	addr := serverAddrOrEnv(opts.Addr)
	if addr == "" {
		addr = "127.0.0.1" + server.DefaultAddr
	}
	client := &apiClient{baseURL: baseURLFromAddr(addr), token: token, hc: defaultHTTPClient()}

	body := map[string]string{"reason": opts.Reason}
	if opts.Reason == "" {
		body["reason"] = "revoked via dream server revoke"
	}

	var res revokeResult
	if err := client.doJSON(ctx, http.MethodPost, "/api/workers/"+opts.WorkerID+"/revoke", body, &res); err != nil {
		return revokeResult{}, err
	}
	return res, nil
}

func mintLocalAdminToken(dataDir string) (string, error) {
	srv, err := server.New(server.Config{DataDir: dataDir})
	if err != nil {
		return "", fmt.Errorf("open control plane state at %s: %w", dataDir, err)
	}
	tok, mintErr := srv.MintAdminToken("cli-admin", adminTokenTTL)
	closeErr := srv.Close()
	if mintErr != nil {
		return "", fmt.Errorf("mint admin token: %w", mintErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close local control plane state: %w", closeErr)
	}
	return tok.Raw, nil
}

type serverRotateEnrollmentOptions struct {
	DataDir   string
	ShowToken bool
}

func newServerRotateEnrollmentCmd() *cobra.Command {
	var opts serverRotateEnrollmentOptions

	cmd := &cobra.Command{
		Use:   "rotate-enrollment",
		Short: "Replace the enrollment token; the old one stops working immediately",
		Long: "Generate a fresh enrollment token in --data-dir, replacing the current one.\n\n" +
			"A running control plane checks the file on every registration, so the\n" +
			"old token is refused from this moment on without a restart. Workers that\n" +
			"are already connected are unaffected — only new registrations need the\n" +
			"new token. Hand it to remote workers out of band (DREAM_TOKEN); workers\n" +
			"on this machine keep auto-discovering it from the file.\n\n" +
			"Prints the token file's path; pass --show-token to print the new value.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServerRotateEnrollment(opts, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&opts.DataDir, "data-dir", "", "control plane state directory (default ~/.dream/_server)")
	cmd.Flags().BoolVar(&opts.ShowToken, "show-token", false, "print the new token's value, not just its path (it is a bearer secret)")

	return cmd
}

func runServerRotateEnrollment(opts serverRotateEnrollmentOptions, out io.Writer) error {
	dataDir := opts.DataDir
	if dataDir == "" {
		d, err := defaultServerDataDir()
		if err != nil {
			return err
		}
		dataDir = d
	}
	dataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return fmt.Errorf("resolve --data-dir: %w", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, server.ServerKeyFileName)); err != nil {
		return fmt.Errorf("%s is not an initialized control plane data directory (run `dream server init` first): %w", dataDir, err)
	}

	path := server.EnrollmentTokenPath(dataDir)
	tok, err := server.RotateEnrollmentToken(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "rotated enrollment token at %s\n", path)
	fmt.Fprintln(out, "the previous token is no longer accepted; connected workers are unaffected")
	if opts.ShowToken {
		fmt.Fprintf(out, "new token: %s\n", tok)
	} else {
		fmt.Fprintf(out, "read it with: cat %s   (or re-run with --show-token)\n", path)
	}
	return nil
}
