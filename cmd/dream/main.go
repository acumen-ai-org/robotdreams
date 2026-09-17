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

var version = "0.1.0-dev"

const devVersion = "0.1.0-dev"

var buildChannel = ""

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "dream",
		Short: "Robot Dreams: a control plane for fleets of autonomous agents",
		Long: "Robot Dreams is a lightweight control plane for fleets of autonomous\n" +
			"software agents. See https://github.com/acumen-ai-org/robotdreams for details.",

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
		newSimulateCmd(),
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
