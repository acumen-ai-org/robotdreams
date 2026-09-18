package main

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/simulation"
)

func TestPrintUniverses(t *testing.T) {
	var sb strings.Builder
	if err := printUniverses(&sb); err != nil {
		t.Fatalf("printUniverses: %v", err)
	}
	out := sb.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	names := simulation.UniverseNames()
	if len(lines) != len(names)+1 {
		t.Fatalf("got %d lines, want header + %d universes:\n%s", len(lines), len(names), out)
	}
	for _, col := range []string{"NAME", "WORLDS", "REALMS", "SITES", "FTE", "DESCRIPTION"} {
		if !strings.Contains(lines[0], col) {
			t.Errorf("header %q lacks %q", lines[0], col)
		}
	}
	for i, name := range names {
		co, err := simulation.LoadCompany(name)
		if err != nil {
			t.Fatalf("LoadCompany(%s): %v", name, err)
		}
		row := lines[i+1]
		if !strings.HasPrefix(row, co.Name) {
			t.Errorf("row %d = %q, want it to start with %q", i+1, row, co.Name)
		}
		if !strings.Contains(row, co.Tagline) {
			t.Errorf("row %d lacks the tagline %q: %q", i+1, co.Tagline, row)
		}
		fields := strings.Fields(row)
		if len(fields) < 5 || fields[1] != strconv.Itoa(len(co.Divisions())) || fields[2] != strconv.Itoa(len(co.Departments())) ||
			fields[3] != strconv.Itoa(len(co.Teams)) || fields[4] != strconv.Itoa(co.Headcount()) {
			t.Errorf("row %d counts = %v, want %d %d %d %d", i+1, fields, len(co.Divisions()), len(co.Departments()), len(co.Teams), co.Headcount())
		}
	}
	if !strings.Contains(out, "spookify") {
		t.Errorf("the default universe is missing from the table:\n%s", out)
	}

	for _, opts := range []simulateOptions{{ListUnis: true}, {Universe: "list"}} {
		var buf strings.Builder
		if err := runSimulate(context.Background(), opts, &buf); err != nil {
			t.Fatalf("runSimulate(%+v): %v", opts, err)
		}
		if buf.String() != out {
			t.Errorf("runSimulate(%+v) printed something other than the universe table:\n%s", opts, buf.String())
		}
	}
}

func TestRunSimulateRejectsBadOptionsBeforeBooting(t *testing.T) {
	var buf strings.Builder
	err := runSimulate(context.Background(), simulateOptions{Universe: "spookify", Speed: 0}, &buf)
	if err == nil || !strings.Contains(err.Error(), "--speed must be > 0") {
		t.Fatalf("speed 0: %v", err)
	}
	err = runSimulate(context.Background(), simulateOptions{Universe: "narnia", Speed: 1}, &buf)
	if err == nil || !strings.Contains(err.Error(), `unknown universe "narnia"`) || !strings.Contains(err.Error(), "spookify") {
		t.Fatalf("unknown universe: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("validation failures printed output: %q", buf.String())
	}
}

func TestPrintSimulateBanner(t *testing.T) {
	if testing.Short() {
		t.Skip("boots a full simulated company; skipped under -short")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	opts := simulateOptions{Seed: 7, Speed: 2.5, History: time.Minute, Universe: "spookify", ReportsDir: "../../reporting/library"}
	sim, err := simulation.Start(ctx, simulation.Options{
		Addr: "127.0.0.1:0", Seed: opts.Seed, Speed: opts.Speed, History: opts.History,
		ReportsDir: opts.ReportsDir, Universe: opts.Universe,
	})
	if err != nil {
		t.Fatalf("simulation.Start: %v", err)
	}
	defer sim.Close()

	var sb strings.Builder
	printSimulateBanner(&sb, sim, opts)
	out := sb.String()

	co := sim.Company()
	st := sim.Stats()
	for _, want := range []string{
		"universe   " + co.Name + " — " + co.Title,
		"dashboard  " + sim.DashboardURL(),
		"api        " + sim.BaseURL(),
		"seed       7 (same seed, same history)",
		"speed      2.5x",
		"history    1m0s: " + strconv.Itoa(st.Instances) + " instances, " + strconv.Itoa(st.Events) + " events across " + strconv.Itoa(st.Scopes) + " scopes",
		"1. Open the dashboard:  " + sim.DashboardURL() + "#/outcomes/" + co.Name,
		"2. In the connect dialog, server URL:  " + sim.BaseURL(),
		"3. Token (admin, valid 24h):",
		sim.AdminToken(),
		"Live stream running. Press Ctrl-C to stop (everything is discarded).",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("banner missing %q:\n%s", want, out)
		}
	}
	apps := sim.PingPongApps()
	if len(apps) != 2 {
		t.Fatalf("expected two demo apps, got %+v", apps)
	}
	for _, a := range apps {
		if !strings.Contains(out, strings.ToLower(a.Label)+" app") || !strings.Contains(out, a.URL) || !strings.Contains(out, "("+a.Node+")") {
			t.Errorf("banner lacks the %s app line (%s at %s):\n%s", a.Label, a.Node, a.URL, out)
		}
	}
	if !strings.Contains(out, "Two demo apps are running: "+apps[0].Node+" and "+apps[1].Node) {
		t.Errorf("banner lacks the two-apps paragraph:\n%s", out)
	}
}
