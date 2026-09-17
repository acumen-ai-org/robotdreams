package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/acumen-ai-org/robotdreams/internal/dashboard"
	"github.com/acumen-ai-org/robotdreams/internal/server"
)

type dashboardOptions struct {
	Server string
	Addr   string
}

func newDashboardCmd() *cobra.Command {
	var opts dashboardOptions

	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Open or serve Mission Control, the org-chart/activity dashboard",
		Long: "Serve Mission Control (the browser dashboard) as a standalone process\n" +
			"pointed at a remote control plane.\n\n" +
			"If the control plane at --server already serves the dashboard itself\n" +
			"(the default for `dream server init`), you can just open\n" +
			"http://<server>/dashboard/ directly — this command's standalone mode is\n" +
			"only needed when the dashboard was disabled there (--no-dashboard) or\n" +
			"you want it served from a separate address.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runDashboard(ctx, opts, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&opts.Server, "server", "", "address of the control plane to point the dashboard at "+
		"(default: $"+envDreamURL+", else 127.0.0.1"+server.DefaultAddr+")")
	cmd.Flags().StringVar(&opts.Addr, "addr", "127.0.0.1:7421", "address to serve the standalone dashboard on")

	return cmd
}

func runDashboard(ctx context.Context, opts dashboardOptions, out io.Writer) error {
	serverAddr := serverAddrOrEnv(opts.Server)
	if serverAddr == "" {
		serverAddr = "127.0.0.1" + server.DefaultAddr
	}
	apiBase := baseURLFromAddr(serverAddr)

	ln, err := net.Listen("tcp", opts.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", opts.Addr, err)
	}

	fmt.Fprintf(out, "Mission Control (standalone client of %s)\n", apiBase)
	fmt.Fprintf(out, "  serving at http://%s/\n\nPress Ctrl-C to stop.\n", ln.Addr().String())

	httpSrv := &http.Server{
		Handler:           dashboard.New(apiBase).Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.Serve(ln) }()

	select {
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	fmt.Fprintln(out, "\nshutting down...")
	return httpSrv.Close()
}
