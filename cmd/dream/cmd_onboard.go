package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/internal/templates"
	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

const onboardHealthTimeout = 5 * time.Second

type workerOnboardOptions struct {
	Server        string
	Template      string
	ListTemplates bool
}

func newWorkerOnboardCmd() *cobra.Command {
	var opts workerOnboardOptions

	cmd := &cobra.Command{
		Use:   "onboard",
		Short: "Print the onboarding brief an AI agent needs to join this fleet",
		Long: "Print a self-contained onboarding brief, written for an AI agent as the\n" +
			"reader. It explains what Robot Dreams is, checks that the control plane\n" +
			"at $" + envDreamURL + " is reachable, and lists the exact commands to register a\n" +
			"node, send every message type, read the inbox, and use storage.\n\n" +
			"Intended use: on the agent's runtime, set " + envDreamURL + " and " + envDreamToken + " (the\n" +
			"server operator gets both from `dream server init` / `dream server\n" +
			"guide`), then tell the agent to run `dream node onboard`.\n\n" +
			"--template prints a named recipe for one particular kind of node instead\n" +
			"of the generic brief (see --list-templates). A template is a document:\n" +
			"printing one installs nothing, writes nothing and starts nothing.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if opts.ListTemplates {
				printTemplateList(out)
				return nil
			}
			if opts.Template != "" {
				return printTemplateBrief(out, opts.Template)
			}
			return runWorkerOnboard(cmd.Context(), opts, out)
		},
	}

	cmd.Flags().StringVar(&opts.Server, "server", "", "control plane address (default: $"+envDreamURL+")")
	cmd.Flags().StringVar(&opts.Template, "template", "",
		"print this template's recipe instead of the generic brief (see --list-templates)")
	cmd.Flags().BoolVar(&opts.ListTemplates, "list-templates", false,
		"list the available onboarding templates and exit")

	return cmd
}

func printTemplateList(out io.Writer) {
	all := templates.List()
	if len(all) == 0 {
		fmt.Fprintln(out, "no templates")
		return
	}
	fmt.Fprintln(out, "Onboarding templates. Each prints a recipe; none of them install anything.")
	fmt.Fprintln(out)
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, t := range all {
		fmt.Fprintf(tw, "  %s\t%s\n", t.Name, t.Summary)
	}
	_ = tw.Flush()
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Print one with:\n  dream node onboard --template %s\n", all[0].Name)
}

func printTemplateBrief(out io.Writer, name string) error {
	t, err := templates.Get(name)
	if err != nil {
		return err
	}
	if len(t.Assumes) > 0 {
		fmt.Fprintln(out, "This template assumes:")
		for _, a := range t.Assumes {
			fmt.Fprintf(out, "  - %s\n", a)
		}
		fmt.Fprintln(out)
	}
	fmt.Fprint(out, t.Brief)
	if !strings.HasSuffix(t.Brief, "\n") {
		fmt.Fprintln(out)
	}
	return nil
}

func checkServerHealth(ctx context.Context, baseURL string) error {
	ctx, cancel := context.WithTimeout(ctx, onboardHealthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/health", nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: onboardHealthTimeout}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errorFromResponse(resp)
	}
	return nil
}

func runWorkerOnboard(ctx context.Context, opts workerOnboardOptions, out io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}

	addr := serverAddrOrEnv(opts.Server)
	if addr == "" {
		return fmt.Errorf("%s is not set and no --server was given.\n"+
			"Set %s to the control plane's base URL (the operator who ran\n"+
			"`dream server init` has it — see `dream server guide`), then re-run:\n"+
			"  %s=http://<server-host>:7420 dream node onboard", envDreamURL, envDreamURL, envDreamURL)
	}
	baseURL := baseURLFromAddr(addr)

	healthErr := checkServerHealth(ctx, baseURL)
	printOnboardBrief(out, baseURL, dreamTokenFromEnv() != "", healthErr)
	return nil
}

func printOnboardBrief(out io.Writer, baseURL string, tokenSet bool, healthErr error) {
	p := func(format string, args ...any) { fmt.Fprintf(out, format+"\n", args...) }

	p("# Robot Dreams — node onboarding brief")
	p("")
	p("You are an agent joining a Robot Dreams fleet. Robot Dreams is a control")
	p("plane for fleets of autonomous agents, built on two primitives: messaging")
	p("(the substrate that carries messages between nodes) and storage (the")
	p("library where nodes deposit outcomes and retrieve inputs). You will")
	p("register yourself as a node in an org chart, report upward to a parent")
	p("node, and exchange work through messages and the library.")
	p("")
	p("Note on words: what this brief calls a \"node\", flags and docs sometimes")
	p("call a \"worker\" — same thing. `dream node ...` and `dream worker ...`")
	p("are interchangeable.")
	p("")

	p("## Connectivity")
	p("")
	if healthErr == nil {
		p("Control plane %s is REACHABLE (health check passed).", baseURL)
	} else {
		p("WARNING: control plane %s is UNREACHABLE right now:", baseURL)
		p("  %v", healthErr)
		p("The commands below are still correct. Before running them, make sure")
		p("%s points at a running, reachable `dream server init` process.", envDreamURL)
	}
	p("")

	p("## Your environment")
	p("")
	p("Every command below relies on two environment variables so that no")
	p("--server or --admin-token flag is ever needed:")
	p("")
	p("  %s    the control plane's base URL   (in effect: %s)", envDreamURL, baseURL)
	if tokenSet {
		p("  %s  the enrollment token authorizing your registration (set)", envDreamToken)
	} else {
		p("  %s  the enrollment token authorizing your registration (NOT set —", envDreamToken)
		p("               registration will fail with 401 unless the server runs")
		p("               --open-enrollment or you are on the server's own machine;")
		p("               ask the operator for the token)")
	}
	p("")

	p("## 1. Register your node (do this once)")
	p("")
	p("Choose a stable, unique node id (task name, hostname, ...), a short role")
	p("label, and — if you were told one — the id of the parent node you report")
	p("to. Then run:")
	p("")
	p("  dream node connect --worker-id <your-node-id> --role <your-role> --reports-to <parent-node-id>")
	p("")
	p("- Omit --reports-to to report directly to the root of the org chart.")
	p("- This generates an Ed25519 identity under ~/.dream/ and records local")
	p("  config, so every later command knows who you are without extra flags.")
	p("- Re-running with the same id reuses the same identity; it is safe.")
	p("")
	p("Verify your node appears:")
	p("")
	p("  dream node list")
	p("")

	p("## 2. Send messages")
	p("")
	p("There are exactly four message types. status_update and escalation always")
	p("go one hop up to your parent (--to is ignored for them); completed_work")
	p("and request_for_input honor --to and default to your parent.")
	p("")
	p("Progress report (send these regularly while working):")
	p("")
	p("  dream message send --type status_update --subject \"halfway there\" --body '{\"progress\":\"50%%\"}'")
	p("")
	p("Completed work — ALWAYS put the deliverable into storage FIRST, then send")
	p("a message that points at it (messages carry pointers, never payloads):")
	p("")
	p("  dream storage put shared/<your-node-id>/report.md ./report.md")
	p("  dream message send --type completed_work --subject \"report ready\" \\")
	p("    --storage-path shared/<your-node-id>/report.md")
	p("")
	p("Escalation (you are blocked and need your parent to act):")
	p("")
	p("  dream message send --type escalation --subject \"blocked\" --body '{\"reason\":\"missing credentials\"}'")
	p("")
	p("Request for input (you need an answer to continue):")
	p("")
	p("  dream message send --type request_for_input --subject \"need a decision\" \\")
	p("    --body '{\"question\":\"deploy to staging or prod?\"}'")
	p("")
	p("Note: escalation and request_for_input fail with a conflict if your node")
	p("reports directly to root — there is nobody above you to answer.")
	p("")

	p("## 3. Read your inbox")
	p("")
	p("  dream message tail --limit 20      # recent messages, one line each")
	p("  dream message tail --follow        # stream new messages as they arrive")
	p("")
	p("When a message you receive carries a storage pointer, fetch the object it")
	p("names with `dream storage get` (next section).")
	p("")

	p("## 4. Storage — the library")
	p("")
	p("Put outcomes in, get inputs out:")
	p("")
	p("  dream storage put <path> <local-file>    # \"-\" as <local-file> reads stdin")
	p("  dream storage get <path> <local-file>    # \"-\" as <local-file> writes stdout")
	p("  dream storage ls [prefix]                # list objects under a prefix")
	p("")
	p("You can read and write exactly two areas:")
	p("")
	p("  workers/<your-node-id>/*   your private prefix (scratch, working state)")
	p("  shared/*                   the shared namespace — anything a parent or")
	p("                             peer must be able to read goes here")
	p("")
	p("Ordering rule: put before send. Upload the object, then send the message")
	p("that points at it — never the other way around.")
	p("")

	p("## 5. Updates")
	p("")
	p("The control plane may tell you a new version of something is available. It")
	p("will never install anything for you and will never restart you. What you do")
	p("about it is your call, including nothing.")
	p("")
	p("Announcements arrive in your inbox as an ordinary status_update from")
	p("%q with subject %q and a JSON body naming a", server.ControlWorkerID, updates.SubjectUpdateAvailable)
	p("\"kind\", a \"version\", and an \"announcement_id\". Filter on the kind:")
	p("%q is the dream tool itself; anything else is defined by", updates.KindCLI)
	p("this deployment. If you do not recognize a kind, ignore it.")
	p("")
	p("  dream updates pending            # announced, and not yet settled by you")
	p("  dream updates report --kind K --status S --announcement-id ID")
	p("  dream updates contract           # the full contract, written for you")
	p("")
	p("Statuses: %s, %s, %s, %s,", updates.StatusAcknowledged, updates.StatusInProgress, updates.StatusApplied, updates.StatusDeclined)
	p("%s — and %q, which just declares what version you are on", updates.StatusFailed, updates.StatusCurrent)
	p("with no announcement involved.")
	p("")
	p("Recommended procedure, NOT enforced by anything — you may deviate:")
	p("  1. Report \"%s\".", updates.StatusAcknowledged)
	p("  2. Stop accepting NEW work.")
	p("  3. Let work already in flight finish; report \"%s\" if slow.", updates.StatusInProgress)
	p("  4. Apply it — for %s that is `dream self-update`.", updates.KindCLI)
	p("  5. Restart yourself if it needs it.")
	p("  6. Resume, and report \"%s\" with your new --current-version.", updates.StatusApplied)
	p("")
	p("If it goes wrong, report \"%s\" with --detail and keep working on the", updates.StatusFailed)
	p("version you have. If you are deliberately not applying it, report")
	p("\"%s\" with --detail. Both are legitimate answers.", updates.StatusDeclined)
	p("")
	p("Ack messages you have handled (`dream message ack <id>`). An unacked")
	p("message is redelivered every time you reconnect.")
	p("")

	p("## 6. Credentials (nothing to do)")
	p("")
	p("Token refresh is automatic. Each command proves possession of your local")
	p("Ed25519 key and receives a short-lived, capability-scoped token (15 min")
	p("TTL); long-lived sessions like `dream message tail --follow` refresh")
	p("themselves in the background. You never handle tokens directly — just")
	p("keep your ~/.dream/ directory intact.")
}
