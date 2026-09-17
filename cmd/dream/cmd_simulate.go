package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/acumen-ai-org/robotdreams/internal/simulation"
)

type simulateOptions struct {
	Addr       string
	Seed       int64
	Speed      float64
	History    time.Duration
	ReportsDir string
	Universe   string
	ListUnis   bool
	UIDev      bool
	UIDir      string
}

func newSimulateCmd() *cobra.Command {
	var opts simulateOptions

	cmd := &cobra.Command{
		Use:   "simulate",
		Short: "Boot a simulated company on a throwaway control plane (demo/review)",
		Long: "Boot an in-process control plane on a temp data dir (embedded queue,\n" +
			"local storage), load the report library, mount Mission Control, and\n" +
			"populate a fictional company — simulated workers connected through the\n" +
			"real HTTP handshake, a deterministic mock LLM writing all narrative\n" +
			"text, 48 hours of generated report history, and a live stream of\n" +
			"instances and pulse events until interrupted.\n\n" +
			"Which company is --universe; `--universe list` prints the choices.\n" +
			"Whichever it is, each team produces only the reports its own work\n" +
			"generates: a finance team a month-end close and a budget bridge, a\n" +
			"support team a queue — and deploys, latency and log lines only from\n" +
			"the teams that actually run software.\n\n" +
			"Same seed, same history. Everything is thrown away on exit.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runSimulate(ctx, opts, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&opts.Addr, "addr", "127.0.0.1:0", "HTTP listen address (port 0 picks a free port)")
	cmd.Flags().Int64Var(&opts.Seed, "seed", 1, "rng seed; same seed, same history")
	cmd.Flags().Float64Var(&opts.Speed, "speed", 1.0, "live-stream speed multiplier (2.0 = twice as fast)")
	cmd.Flags().DurationVar(&opts.History, "history", 48*time.Hour, "how much backdated report history to generate")
	cmd.Flags().StringVar(&opts.ReportsDir, "reports", "reporting/library", "report definition library directory to load")
	cmd.Flags().StringVar(&opts.Universe, "universe", "spookify", "which fixture company to simulate (\"list\" prints them)")
	cmd.Flags().BoolVar(&opts.ListUnis, "list-universes", false, "print the available fixture companies and exit")
	cmd.Flags().BoolVar(&opts.UIDev, "ui-dev", false, "also start the Vite dev UI wired to this simulation (hot reload, auto-connected; needs Node in internal/dashboard/ui)")
	cmd.Flags().StringVar(&opts.UIDir, "ui-dir", "internal/dashboard/ui", "dashboard UI source directory for --ui-dev")

	return cmd
}

func (o simulateOptions) listsUniverses() bool {
	return o.ListUnis || o.Universe == "list"
}

func runSimulate(ctx context.Context, opts simulateOptions, out io.Writer) error {
	if opts.listsUniverses() {
		return printUniverses(out)
	}
	if opts.Speed <= 0 {
		return fmt.Errorf("--speed must be > 0")
	}
	if !simulation.HasUniverse(opts.Universe) {
		return fmt.Errorf("unknown universe %q; available: %s (or --universe list)",
			opts.Universe, strings.Join(simulation.UniverseNames(), ", "))
	}

	fmt.Fprintf(out, "Robot Dreams simulation %s\n\n", version)
	sim, err := simulation.Start(ctx, simulation.Options{
		Addr:       opts.Addr,
		Seed:       opts.Seed,
		Speed:      opts.Speed,
		History:    opts.History,
		ReportsDir: opts.ReportsDir,
		Universe:   opts.Universe,
		Out:        out,
	})
	if err != nil {
		return err
	}
	defer sim.Close()

	printSimulateBanner(out, sim, opts)

	if opts.UIDev {
		stopUI, uiErr := startUIDev(ctx, sim, opts.UIDir, out)
		if uiErr != nil {
			return uiErr
		}
		defer stopUI()
	}

	err = sim.RunLive(ctx)
	fmt.Fprintln(out, "\nshutting down...")
	if err != nil {
		return err
	}
	return sim.Close()
}

func startUIDev(ctx context.Context, sim *simulation.Sim, uiDir string, out io.Writer) (func(), error) {
	if _, err := os.Stat(filepath.Join(uiDir, "package.json")); err != nil {
		return nil, fmt.Errorf("--ui-dev: %s/package.json not found — run from the repository root (or point --ui-dir at the dashboard UI)", uiDir)
	}
	if _, err := os.Stat(filepath.Join(uiDir, "node_modules")); err != nil {
		return nil, fmt.Errorf("--ui-dev: %s/node_modules missing — run `npm ci` there once (Node is only needed for UI development)", uiDir)
	}

	cmd := exec.Command("npm", "run", "dev")
	cmd.Dir = uiDir
	cmd.Env = append(os.Environ(),
		"DREAM_API="+sim.BaseURL(),
		"VITE_DREAM_DEV_TOKEN="+sim.AdminToken(),
	)
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("--ui-dev: start `npm run dev`: %w (is Node installed?)", err)
	}

	fmt.Fprint(out, "\nVite dev UI starting — open the URL it prints below; it is already\n"+
		"wired to this simulation (proxy + token), no connect dialog needed.\n"+
		"Edits under "+uiDir+"/src hot-reload instantly.\n\n")

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	go func() {
		select {
		case <-ctx.Done():
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		case <-done:
		}
	}()
	return func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}, nil
}

func printUniverses(out io.Writer) error {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tWORLDS\tREALMS\tSITES\tFTE\tDESCRIPTION")
	for _, name := range simulation.UniverseNames() {
		co, err := simulation.LoadCompany(name)
		if err != nil {
			return err
		}
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t%s\n",
			co.Name, len(co.Divisions()), len(co.Departments()), len(co.Teams), co.Headcount(), co.Tagline)
	}
	return tw.Flush()
}

func printSimulateBanner(out io.Writer, sim *simulation.Sim, opts simulateOptions) {
	st := sim.Stats()

	co := sim.Company()

	fmt.Fprintln(out)
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  universe\t%s — %s (%d worlds, %d realms, %d sites, %d FTE)\n",
		co.Name, co.Title, len(co.Divisions()), len(co.Departments()), len(co.Teams), co.Headcount())
	fmt.Fprintf(tw, "  dashboard\t%s\n", sim.DashboardURL())
	fmt.Fprintf(tw, "  api\t%s\n", sim.BaseURL())
	fmt.Fprintf(tw, "  seed\t%d (same seed, same history)\n", opts.Seed)
	fmt.Fprintf(tw, "  speed\t%.1fx\n", opts.Speed)
	fmt.Fprintf(tw, "  history\t%s: %d instances, %d events across %d scopes\n", opts.History, st.Instances, st.Events, st.Scopes)
	for _, a := range sim.PingPongApps() {
		fmt.Fprintf(tw, "  %s app\t%s  (%s)\n", strings.ToLower(a.Label), a.URL, a.Node)
	}
	_ = tw.Flush()

	fmt.Fprint(out, "\nTo use the running instance:\n\n")
	fmt.Fprintf(out, "  1. Open the dashboard:  %s#/outcomes/%s\n", sim.DashboardURL(), co.Name)
	fmt.Fprintf(out, "  2. In the connect dialog, server URL:  %s\n", sim.BaseURL())
	fmt.Fprintf(out, "  3. Token (admin, valid 24h):\n\n     %s\n\n", sim.AdminToken())
	fmt.Fprint(out, "The token carries the admin scope, so one paste covers everything:\n"+
		"all reporting reads AND the live pulse rail — GET /api/events (SSE)\n"+
		"is admin-only; with a non-admin token the dashboard falls back to\n"+
		"15s polling instead of live updates.\n")
	if apps := sim.PingPongApps(); len(apps) == 2 {
		fmt.Fprintf(out, "\nTwo demo apps are running: %s and %s pass messages to each other\n"+
			"through the control plane, and each page counts what it received. Both are\n"+
			"registered as apps on their nodes, so Mission Control can open or embed them —\n"+
			"select either node in Control Plane.\n", apps[0].Node, apps[1].Node)
	}
	fmt.Fprintf(out, "\nLive stream running. Press Ctrl-C to stop (everything is discarded).\n")
}
