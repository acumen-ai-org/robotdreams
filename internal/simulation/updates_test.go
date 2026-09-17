package simulation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

func simGet(ctx context.Context, t *testing.T, sim *Sim, path string, out any) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sim.BaseURL()+path, nil)
	if err != nil {
		t.Fatalf("build request %s: %v", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+sim.AdminToken())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: http %d: %s", path, resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("GET %s: decode: %v", path, err)
	}
}

func TestSimulationDeclaresFleetVersions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sim, err := Start(ctx, Options{
		Addr: "127.0.0.1:0", Seed: 1, History: time.Hour,
		ReportsDir: "../../reporting/library", Universe: "spookify",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sim.Close()

	var rollout updates.Rollout
	simGet(ctx, t, sim, "/api/updates/rollout?kind="+updates.KindCLI, &rollout)

	if len(rollout.Nodes) == 0 {
		t.Fatal("rollout lists no nodes")
	}
	var withVersion int
	for _, n := range rollout.Nodes {
		if n.CurrentVersion == simCLIVersion {
			withVersion++
		}
	}
	if withVersion != len(rollout.Nodes) {
		t.Fatalf("%d of %d nodes declared a version, want all of them", withVersion, len(rollout.Nodes))
	}
	if rollout.Announcement != nil {
		t.Fatalf("an announcement exists before the live run: %+v", rollout.Announcement)
	}
}

func TestSimulationAnnouncesAndAnswers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sim, err := Start(ctx, Options{
		Addr: "127.0.0.1:0", Seed: 1, History: time.Hour,
		ReportsDir: "../../reporting/library", Universe: "spookify",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sim.Close()

	if err := sim.announceFleetUpdate(ctx); err != nil {
		t.Fatalf("announceFleetUpdate: %v", err)
	}

	var rollout updates.Rollout
	simGet(ctx, t, sim, "/api/updates/rollout?kind="+updates.KindCLI, &rollout)

	if rollout.Announcement == nil || rollout.Announcement.Version != simCLINextVersion {
		t.Fatalf("announcement = %+v, want version %s", rollout.Announcement, simCLINextVersion)
	}

	total := len(rollout.Nodes)
	c := rollout.Counts
	for _, want := range []string{
		updates.StatusApplied, updates.StatusFailed, updates.StatusDeclined, updates.StatusUnknown,
	} {
		if c[want] == 0 {
			t.Errorf("no nodes are %q; the demo should show every phase (counts %+v)", want, c)
		}
	}
	if c[updates.StatusApplied] <= c[updates.StatusFailed] {
		t.Errorf("more failures than applications; the spread is not a plausible rollout: %+v", c)
	}
	var sum int
	for _, n := range c {
		sum += n
	}
	if sum != total {
		t.Fatalf("counts sum to %d but there are %d nodes", sum, total)
	}
}

func TestSimulationUpdateSpreadIsDeterministic(t *testing.T) {
	run := func() map[string]int {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		sim, err := Start(ctx, Options{
			Addr: "127.0.0.1:0", Seed: 7, History: time.Hour,
			ReportsDir: "../../reporting/library", Universe: "spookify",
		})
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		defer sim.Close()
		if err := sim.announceFleetUpdate(ctx); err != nil {
			t.Fatalf("announce: %v", err)
		}
		var rollout updates.Rollout
		simGet(ctx, t, sim, "/api/updates/rollout?kind="+updates.KindCLI, &rollout)
		return rollout.Counts
	}

	a, b := run(), run()
	for _, k := range []string{
		updates.StatusApplied, updates.StatusFailed, updates.StatusDeclined, updates.StatusUnknown,
	} {
		if a[k] != b[k] {
			t.Fatalf("two runs at the same seed disagree on %q: %d vs %d", k, a[k], b[k])
		}
	}
}

func TestSimulationAppsAreAllReachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sim, err := Start(ctx, Options{
		Addr: "127.0.0.1:0", Seed: 1, History: time.Hour,
		ReportsDir: "../../reporting/library", Universe: "spookify",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sim.Close()

	var out struct {
		Apps []struct {
			WorkerID    string `json:"worker_id"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"apps"`
	}
	simGet(ctx, t, sim, "/api/apps", &out)

	if len(out.Apps) == 0 {
		t.Fatal("no apps declared; the demo would show an empty affordance")
	}
	total := len(sim.Company().Workers())
	if len(out.Apps) >= total {
		t.Fatalf("%d of %d nodes declared an app; some nodes should have none", len(out.Apps), total)
	}

	served := map[string]bool{}
	for _, a := range sim.PingPongApps() {
		served[a.URL] = true
	}

	for _, a := range out.Apps {
		if a.Description == "" {
			t.Errorf("%s declared no description", a.WorkerID)
		}
		if !served[a.URL] {
			t.Errorf("%s advertises %q, which nothing in this simulation serves — "+
				"an app record that cannot be opened is worse than none", a.WorkerID, a.URL)
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("%s advertises %q, which does not answer: %v", a.WorkerID, a.URL, err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s advertises %q, which answered http %d", a.WorkerID, a.URL, resp.StatusCode)
		}
	}
}
