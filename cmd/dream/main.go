// Command dream is the Robot Dreams CLI entrypoint.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/acumen-ai-org/robotdreams/internal/selfupdate"
)

// version is the CLI's reported version. Release builds override it via
// -ldflags "-X main.version=<tag>" (see .goreleaser.yaml); dev builds
// report the fallback below.
var version = "0.1.0-dev"

// devVersion is version's compile-time default, kept as a separate
// constant so `dream self-update` can tell "goreleaser stamped this" from
// "this is a local build" by comparing the two.
const devVersion = "0.1.0-dev"

// buildChannel is set to "release" by goreleaser's ldflags and is empty
// everywhere else.
//
// This is an explicit marker rather than an inference. Deducing "release"
// from "version differs from devVersion" happens to work today, but it
// would break silently the moment anything else sets main.version — and
// the failure mode is a binary that decides it may overwrite itself when
// it may not. One word of YAML buys certainty.
var buildChannel = ""

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "dream",
		Short: "Robot Dreams: a control plane for fleets of autonomous agents",
		Long: "Robot Dreams is a lightweight control plane for fleets of autonomous\n" +
			"software agents. See https://github.com/acumen-ai-org/robotdreams for details.",

		// A command that fails at RUNTIME (server refused the call, no
		// local identity, network down) should print the error alone.
		// Dumping the whole usage block there implies the user typed
		// something wrong, which is misleading and buries the message.
		// Cobra still prints usage for genuine flag/argument errors,
		// because those are reported before RunE is reached.
		SilenceUsage: true,
	}

	root.AddCommand(
		newVersionCmd(),
		newSelfUpdateCmd(),
		newUpdatesCmd(),
		newServerCmd(),
		newWorkerCmd(),
		newMessageCmd(),
		newScheduleCmd(),
		newStorageCmd(),
		newDashboardCmd(),
	)

	return root
}

func newVersionCmd() *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the dream CLI version",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			// The plain form is deliberately byte-stable: scripts parse
			// it, and `dream self-update` runs it against a freshly
			// downloaded binary as a smoke test before swapping it in.
			fmt.Fprintf(out, "dream version %s\n", version)
			if !verbose {
				return nil
			}
			fmt.Fprintf(out, "  platform  %s/%s\n", runtime.GOOS, runtime.GOARCH)
			inst, err := selfupdate.Detect(version, devVersion, buildChannel)
			if err != nil {
				fmt.Fprintf(out, "  install   unknown (%v)\n", err)
				return nil
			}
			fmt.Fprintf(out, "  install   %s — %s\n", inst.Method, inst.Reason)
			fmt.Fprintf(out, "  path      %s\n", inst.ExePath)
			return nil
		},
	}

	cmd.Flags().BoolVar(&verbose, "verbose", false,
		"also print the platform, how this binary was installed, and where it lives")
	return cmd
}

func main() {
	// Signal handling is wired once, here, rather than per command.
	//
	// Note this changes cancellation semantics for EVERY command at once:
	// until now cmd.Context() was always context.Background(), so Ctrl-C
	// killed the process outright mid-write. Now an in-flight HTTP call is
	// cancelled and unwound instead. The three commands that already build
	// their own signal context (server, dashboard) are left
	// alone and keep working unchanged.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
