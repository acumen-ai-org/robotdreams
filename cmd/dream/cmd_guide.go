package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/acumen-ai-org/robotdreams/internal/server"
)

const guideRule = "───────────────────────────────────────────"

func suggestedServerURL(boundAddr string) string {
	return suggestedServerURLWithScheme(boundAddr, false)
}

func suggestedServerURLWithScheme(boundAddr string, tls bool) string {
	scheme := "http://"
	if tls {
		scheme = "https://"
	}
	host, port, err := net.SplitHostPort(strings.TrimSpace(boundAddr))
	if err != nil {
		return baseURLFromAddr(boundAddr)
	}
	switch host {
	case "", "::", "0.0.0.0":
		host = "<this-host>"
	}
	return scheme + net.JoinHostPort(host, port)
}

func printAgentPunchline(out io.Writer, serverURL, tokenPath string, openEnrollment bool) {
	fmt.Fprintln(out, guideRule)
	fmt.Fprintln(out, "Connect an agent from anywhere:")
	fmt.Fprintln(out, "  1. On your runtime — any machine, any cloud — set two variables:")
	fmt.Fprintf(out, "       DREAM_URL=%s\n", serverURL)
	if openEnrollment {
		fmt.Fprintln(out, "       DREAM_TOKEN is not needed: this server runs --open-enrollment")
	} else {
		fmt.Fprintf(out, "       DREAM_TOKEN=$(cat %s)\n", tokenPath)
	}
	fmt.Fprintln(out, "  2. Then just tell your agent to create its node with:")
	fmt.Fprintln(out, "       dream node onboard")
	fmt.Fprintln(out, "       (or: npx robotdreams node onboard)")
	fmt.Fprintln(out, "     That command tells your agent everything it needs to know.")
	fmt.Fprintln(out, guideRule)
}

type serverGuideParams struct {
	DataDir string

	ServerURL string

	EnrollmentToken string

	OpenEnrollment bool
}

func enrollmentTokenPath(dataDir string) string {
	return filepath.Join(dataDir, server.EnrollmentTokenFileName)
}

func printServerGuide(out io.Writer, p serverGuideParams) {
	tokenPath := enrollmentTokenPath(p.DataDir)

	fmt.Fprintln(out, "Robot Dreams — server setup guide")
	fmt.Fprintln(out)

	fmt.Fprintln(out, "What `dream server init` creates")
	fmt.Fprintf(out, "  %s/\n", p.DataDir)
	fmt.Fprintln(out, "    control-plane DB (SQLite)  org chart, revocations, and the embedded message queue")
	fmt.Fprintln(out, "    signing key                signs the short-lived worker capability tokens (JWTs)")
	fmt.Fprintln(out, "    enrollment.token (0600)    the secret that authorizes new workers to enroll")
	fmt.Fprintln(out, "    storage/                   the default local-filesystem storage backend")
	fmt.Fprintln(out)

	fmt.Fprintln(out, "Server URLs")
	fmt.Fprintf(out, "  API        %s\n", p.ServerURL)
	fmt.Fprintf(out, "  Dashboard  %s/dashboard/  (Mission Control; opt out with --no-dashboard)\n", p.ServerURL)
	fmt.Fprintln(out)

	fmt.Fprintln(out, "Enrollment token")
	if p.OpenEnrollment {
		fmt.Fprintln(out, "  This server runs --open-enrollment: any caller may register a worker,")
		fmt.Fprintln(out, "  no token required. Dev/test only — never on a network-reachable server.")
	} else {
		fmt.Fprintf(out, "  Lives at %s\n", tokenPath)
		if p.EnrollmentToken != "" {
			fmt.Fprintf(out, "  Value:   %s\n", p.EnrollmentToken)
		} else {
			fmt.Fprintf(out, "  Read it with: cat %s   (or: dream server guide --show-token)\n", tokenPath)
		}
		fmt.Fprintln(out, "  `dream worker connect` on this machine auto-discovers it; anywhere else")
		fmt.Fprintln(out, "  it must be handed over out of band (DREAM_TOKEN or --admin-token).")
		fmt.Fprintln(out, "  Rotate it (the old value stops working at once, no restart) with:")
		fmt.Fprintf(out, "    dream server rotate-enrollment --data-dir %s\n", p.DataDir)
	}
	fmt.Fprintln(out)

	fmt.Fprintln(out, "Flags that matter (dream server init)")
	fmt.Fprintln(out, "  --addr             listen address (default 127.0.0.1:7420, this machine only;")
	fmt.Fprintln(out, "                     :7420 binds all interfaces and then needs TLS or --insecure)")
	fmt.Fprintln(out, "  --tls-cert/--tls-key")
	fmt.Fprintln(out, "                     serve HTTPS directly (PEM certificate and key; TLS 1.2+)")
	fmt.Fprintln(out, "  --insecure         allow a non-loopback --addr without TLS (trusted networks only)")
	fmt.Fprintln(out, "  --data-dir         state directory (default ~/.dream/_server)")
	fmt.Fprintln(out, "  --messaging        messaging backend URI (default: embedded SQLite queue in the data dir)")
	fmt.Fprintln(out, "  --storage          storage backend URI (default: local files in the data dir)")
	fmt.Fprintln(out, "  --orgchart         YAML org chart bulk-loaded on a first-ever startup")
	fmt.Fprintln(out, "  --no-dashboard     do not serve Mission Control at /dashboard")
	fmt.Fprintln(out, "  --open-enrollment  disable enrollment authorization (dev/test only)")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  Bearer tokens travel in every request, so a server reachable from")
	fmt.Fprintln(out, "  other machines must serve TLS — either --tls-cert/--tls-key here, or a")
	fmt.Fprintln(out, "  TLS-terminating reverse proxy in front (then pass --insecure, since the")
	fmt.Fprintln(out, "  hop to the proxy is plaintext by design). See docs/security-model.md.")
	fmt.Fprintln(out)

	fmt.Fprintln(out, "Reaching this server from other machines")
	fmt.Fprintln(out, "  Workers elsewhere need a URL that resolves to this machine —")
	fmt.Fprintln(out, "  https://<host-or-ip>:7420 with --tls-cert/--tls-key, or your TLS")
	fmt.Fprintln(out, "  proxy's https:// URL. That URL is what goes in DREAM_URL below; the")
	fmt.Fprintln(out, "  default loopback address only works for workers on this same machine.")
	fmt.Fprintln(out)

	fmt.Fprintln(out, "Environment variables (honored by every dream command)")
	fmt.Fprintln(out, "  DREAM_URL    control plane base URL; used whenever --server is not passed")
	fmt.Fprintln(out, "  DREAM_TOKEN  enrollment/admin token; used whenever --admin-token is not passed")
	fmt.Fprintln(out, "  Precedence: explicit flag > environment variable > local auto-discovery")
	fmt.Fprintf(out, "  (the enrollment-token file above, and the ~/.dream/<server-id>/config.json\n")
	fmt.Fprintf(out, "  a previous `dream worker connect` recorded).\n")
	fmt.Fprintln(out)

	printAgentPunchline(out, p.ServerURL, tokenPath, p.OpenEnrollment)
}

type serverGuideOptions struct {
	DataDir   string
	Server    string
	ShowToken bool
}

func newServerGuideCmd() *cobra.Command {
	var opts serverGuideOptions

	cmd := &cobra.Command{
		Use:   "guide",
		Short: "Print the full server setup guide (the long form of init's banner)",
		Long: "Print the full server setup guide: what `dream server init` creates and\n" +
			"where, the URLs it serves, where the enrollment token lives, the flags\n" +
			"that matter, and how to connect an agent from anywhere with just\n" +
			"DREAM_URL and DREAM_TOKEN.\n\n" +
			"Reads only the local data directory (to locate the enrollment token);\n" +
			"it does not need the server to be running. The token's value is\n" +
			"printed only with --show-token; by default the guide shows its path.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServerGuide(opts, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&opts.DataDir, "data-dir", "", "control plane state directory (default ~/.dream/_server)")
	cmd.Flags().StringVar(&opts.Server, "server", "",
		"server address to show in the guide (default: $"+envDreamURL+", else 127.0.0.1"+server.DefaultAddr+")")
	cmd.Flags().BoolVar(&opts.ShowToken, "show-token", false,
		"print the enrollment token's value, not just its path (it is a bearer secret)")

	return cmd
}

func runServerGuide(opts serverGuideOptions, out io.Writer) error {
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

	addr := serverAddrOrEnv(opts.Server)
	if addr == "" {
		addr = "127.0.0.1" + server.DefaultAddr
	}

	token := ""
	if opts.ShowToken {
		if raw, err := os.ReadFile(enrollmentTokenPath(dataDir)); err == nil {
			token = strings.TrimSpace(string(raw))
		}
	}

	printServerGuide(out, serverGuideParams{
		DataDir:         dataDir,
		ServerURL:       baseURLFromAddr(addr),
		EnrollmentToken: token,
	})
	return nil
}
