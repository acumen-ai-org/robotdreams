package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/acumen-ai-org/robotdreams/internal/selfupdate"
	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

type selfUpdateOptions struct {
	Check          bool
	Version        string
	AllowDowngrade bool
	NoRestart      bool
}

func newSelfUpdateCmd() *cobra.Command {
	var opts selfUpdateOptions

	cmd := &cobra.Command{
		Use:   "self-update",
		Short: "Update the dream CLI itself",
		Long: "Update the dream CLI to the latest release.\n\n" +
			"What this does depends on how dream was installed. A binary from a\n" +
			"GitHub release archive is replaced in place: the new archive is\n" +
			"downloaded, verified against the release's checksums.txt, run once to\n" +
			"confirm it works, swapped atomically, and — unless --no-restart — the\n" +
			"running process re-execs itself, keeping its PID and arguments.\n\n" +
			"A binary a package manager owns is never overwritten. If dream was\n" +
			"installed with npm or `go install`, this prints the exact command to\n" +
			"run instead and exits non-zero. Writing behind a package manager\n" +
			"leaves its records claiming something that is no longer true.\n\n" +
			"When the install method cannot be determined, dream refuses to replace\n" +
			"itself and prints instructions. That is the deliberate default.\n\n" +
			"Note what the checksum does and does not buy you: it travels the same\n" +
			"channel as the archive, so it catches a corrupt or truncated download,\n" +
			"not a compromised release. It is not a signature.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSelfUpdate(cmd, opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Check, "check", false,
		"report the latest available version and exit without changing anything")
	cmd.Flags().StringVar(&opts.Version, "version", "",
		"install this exact version instead of the latest")
	cmd.Flags().BoolVar(&opts.AllowDowngrade, "allow-downgrade", false,
		"permit installing a version older than the one running")
	cmd.Flags().BoolVar(&opts.NoRestart, "no-restart", false,
		"replace the binary but do not re-exec the running process")

	return cmd
}

func runSelfUpdate(cmd *cobra.Command, opts selfUpdateOptions) error {
	out := cmd.OutOrStdout()

	if hops := selfupdate.Hops(); hops > 0 && !opts.Check {
		return fmt.Errorf("refusing to update again: this process has already re-exec'd %d time(s) "+
			"for an update.\nThe installed binary may not be reporting the announced version", hops)
	}

	res, err := selfupdate.Run(cmd.Context(), selfupdate.Options{
		CurrentVersion: version,
		DevVersion:     devVersion,
		BuildChannel:   buildChannel,
		TargetVersion:  opts.Version,
		AllowDowngrade: opts.AllowDowngrade,
		CheckOnly:      opts.Check,
	})

	fmt.Fprintf(out, "  current  %s   (installed via %s)\n", res.CurrentVersion, res.Install.Method)
	if res.LatestVersion != "" {
		label := "latest"
		if opts.Version != "" {
			label = "target"
		}
		fmt.Fprintf(out, "  %-7s  %s\n", label, res.LatestVersion)
	}

	if err != nil {
		if res.Refused && res.Install.Fix != "" {
			fmt.Fprintf(out, "\n  refusing to replace a %s-managed binary.\n  run:  %s\n",
				res.Install.Method, res.Install.Fix)
		}
		return err
	}

	switch {
	case res.AlreadyCurrent:
		fmt.Fprintf(out, "\ndream is already up to date.\n")
		return nil
	case opts.Check:
		fmt.Fprintf(out, "\nan update is available. run `dream self-update` to install it.\n")
		return nil
	}

	fmt.Fprintf(out, "\nupdated to %s.\n", res.LatestVersion)

	if opts.NoRestart {
		return nil
	}
	if !selfupdate.SupportsInPlace() {
		return nil
	}

	selfupdate.CleanupBackup(res.Install.ExePath)

	if err := selfupdate.Restart(res.Install.ExePath, os.Args, selfupdate.Hops()); err != nil {
		return fmt.Errorf("updated to %s, but re-exec failed: %w\nrestart dream to run the new version",
			res.LatestVersion, err)
	}
	return nil
}

func applyAnnouncedUpdate(ctx context.Context, out io.Writer, req *updateRequest) error {
	fmt.Fprintf(out, "\napplying %s %s (announced by the control plane)\n", req.Kind, req.Version)

	reportUpdateProgress(ctx, req, updates.StatusInProgress, "", "draining and applying")

	res, err := selfupdate.Run(ctx, selfupdate.Options{
		CurrentVersion: version,
		DevVersion:     devVersion,
		BuildChannel:   buildChannel,
		TargetVersion:  req.Version,
	})

	switch {
	case err != nil && res.Refused:
		fmt.Fprintf(out, "  not applying it: %v\n", err)
		if res.Install.Fix != "" {
			fmt.Fprintf(out, "  run:  %s\n", res.Install.Fix)
		}
		fmt.Fprintln(out, "  still following; this node keeps working on the version it has.")
		reportUpdateProgress(ctx, req, updates.StatusDeclined, "", declineReason(res))
		return nil

	case err != nil:
		fmt.Fprintf(out, "  update failed: %v\n", err)
		fmt.Fprintln(out, "  still following; this node keeps working on the version it has.")
		reportUpdateProgress(ctx, req, updates.StatusFailed, "", err.Error())
		return nil

	case res.AlreadyCurrent:

		reportUpdateProgress(ctx, req, updates.StatusApplied, res.CurrentVersion, "already running this version")
		fmt.Fprintln(out, "  already on that version; nothing to do.")
		return nil
	}

	reportUpdateProgress(ctx, req, updates.StatusApplied, res.LatestVersion, "restarting")
	ackUpdateMessage(ctx, req)

	fmt.Fprintf(out, "  updated to %s; restarting.\n", res.LatestVersion)

	if !selfupdate.SupportsInPlace() {
		return nil
	}
	selfupdate.CleanupBackup(res.Install.ExePath)
	if err := selfupdate.Restart(res.Install.ExePath, os.Args, selfupdate.Hops()); err != nil {
		return fmt.Errorf("updated to %s, but re-exec failed: %w\nrestart dream to run the new version",
			res.LatestVersion, err)
	}
	return nil
}

func declineReason(res selfupdate.Result) string {
	if res.Install.Fix != "" {
		return fmt.Sprintf("managed by %s; run: %s", res.Install.Method, res.Install.Fix)
	}
	return fmt.Sprintf("managed by %s", res.Install.Method)
}

func reportUpdateProgress(ctx context.Context, req *updateRequest, status, currentVersion, detail string) {
	_ = runUpdatesReport(ctx, updatesReportOptions{
		Kind:           req.Kind,
		Status:         status,
		AnnouncementID: req.AnnouncementID,
		CurrentVersion: currentVersion,
		TargetVersion:  req.Version,
		Detail:         detail,
	})
}

func ackUpdateMessage(ctx context.Context, req *updateRequest) {
	if req.MessageID == "" {
		return
	}
	_ = runMessageAck(ctx, messageAckOptions{MessageID: req.MessageID, Action: "applied"})
}
