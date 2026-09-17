package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
)

const boardDefYAML = `
version: v1alpha1
kind: ReportDefinition
name: board
description: Work in flight
categories: [roadmap]
modalities: [board]
stances: [operational]
scope:
  attach: [site]
  aggregation:
    status: worst
    kpis:
      open: sum
    items: merge
data:
  kpis:
    - name: open
      unit: count
facets:
  summary:
    kpis: [open]
  plan:
    states: [todo, doing, done]
    lanes: [feature, defect]
    wip_limits: { doing: 2 }
    size_unit: points
  detail:
    panels:
      - { title: The board, primitive: board, data: items, section: next }
      - { title: Open, primitive: kpi_card, data: kpi/open, section: overview }
`

func (e *testEnv) planReport(token, def, scope string) planReportResponse {
	e.t.Helper()
	status, raw := e.do(http.MethodGet,
		"/api/reports/report?definition="+def+"&scope="+scope, token, nil)
	if status != http.StatusOK {
		e.t.Fatalf("GET report: status %d, body %s", status, raw)
	}
	var out planReportResponse
	decodeInto(e.t, raw, &out)
	return out
}

func newPlanTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"board.yaml":  boardDefYAML,
		"errors.yaml": errorsDefYAML,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return newTestEnv(t, func(cfg *server.Config) { cfg.ReportsDir = dir })
}

type planColumn struct {
	State string  `json:"state"`
	Items int     `json:"items"`
	Shown int     `json:"shown"`
	Limit int     `json:"limit"`
	Size  float64 `json:"size"`
}

type planBlock struct {
	Definition string                  `json:"definition"`
	Policy     string                  `json:"policy"`
	States     []planColumn            `json:"states"`
	Lanes      []struct{ Name string } `json:"lanes"`
	Items      []reporting.Item        `json:"items"`
	Total      int                     `json:"total"`
	Blocked    int                     `json:"blocked"`
	SizeUnit   string                  `json:"size_unit"`
}

type planReportResponse struct {
	Plan   *planBlock `json:"plan"`
	Panels []struct {
		Title    string     `json:"title"`
		DataKind string     `json:"data_kind"`
		Section  string     `json:"section"`
		Stances  []string   `json:"stances"`
		Plan     *planBlock `json:"plan"`
	} `json:"panels"`
}

func TestReportEndpointPlanBlock(t *testing.T) {
	e := newPlanTestEnv(t)
	w := e.connect("w1", "node", "")
	base := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	e.postInstance(w.Token, reporting.Instance{
		Definition: "board", Scope: "acme/a", ProducedAt: base,
		KPIs: map[string]float64{"open": 2},
		Items: []reporting.Item{
			{ID: "a1", Title: "First", State: "doing", Lane: "feature", Size: 3},
			{ID: "a2", Title: "Second", State: "todo", Lane: "defect", BlockedBy: []string{"a1"}},
		},
	})
	e.postInstance(w.Token, reporting.Instance{
		Definition: "board", Scope: "acme/b", ProducedAt: base.Add(time.Minute),
		KPIs: map[string]float64{"open": 1},
		Items: []reporting.Item{
			{ID: "b1", Title: "Theirs", State: "done", Lane: "feature", Size: 5},
		},
	})

	got := e.planReport(w.Token, "board", "acme")

	if got.Plan == nil {
		t.Fatal("no plan block in the report response")
	}
	if got.Plan.Total != 3 || got.Plan.Blocked != 1 {
		t.Fatalf("Total=%d Blocked=%d, want 3 and 1", got.Plan.Total, got.Plan.Blocked)
	}
	if got.Plan.SizeUnit != "points" {
		t.Errorf("SizeUnit = %q, want points", got.Plan.SizeUnit)
	}

	want := []string{"todo", "doing", "done"}
	if len(got.Plan.States) != len(want) {
		t.Fatalf("got %d columns, want %d", len(got.Plan.States), len(want))
	}
	for i, name := range want {
		if got.Plan.States[i].State != name {
			t.Fatalf("column %d = %q, want %q", i, got.Plan.States[i].State, name)
		}
	}

	for _, c := range got.Plan.States {
		if c.State == "doing" && c.Limit != 4 {
			t.Errorf("doing limit = %d, want 4 (2 x 2 contributors)", c.Limit)
		}
	}

	for _, it := range got.Plan.Items {
		if it.Scope == "" || it.Definition != "board" {
			t.Fatalf("item %q not origin-stamped: %+v", it.ID, it)
		}
	}

	var boardPanel bool
	for _, p := range got.Panels {
		if p.Title != "The board" {
			continue
		}
		boardPanel = true
		if p.DataKind != "items" {
			t.Errorf("board panel data_kind = %q, want items", p.DataKind)
		}
		if p.Plan == nil || len(p.Plan.States) != 3 {
			t.Error("board panel carries no plan, so it cannot order its columns")
		}
		if len(p.Stances) != 1 || p.Stances[0] != "strategic" {
			t.Errorf("next-section panel stances = %v, want [strategic]", p.Stances)
		}
	}
	if !boardPanel {
		t.Fatal("no items panel in the response")
	}
}

func TestReportEndpointNoPlanFacet(t *testing.T) {
	e := newPlanTestEnv(t)
	w := e.connect("w1", "node", "")
	e.postInstance(w.Token, reporting.Instance{
		Definition: "errors", Scope: "acme/a", ProducedAt: time.Now().UTC(),
		KPIs: map[string]float64{"error_count": 1},
	})

	got := e.planReport(w.Token, "errors", "acme")
	if got.Plan != nil {
		t.Fatalf("a definition with no plan facet must report a null plan, got %+v", got.Plan)
	}
}

func TestReportInstancePostRejectsBadItems(t *testing.T) {
	e := newPlanTestEnv(t)
	w := e.connect("w1", "node", "")

	status, raw := e.do(http.MethodPost, "/api/reports/instances", w.Token, reporting.Instance{
		Definition: "board", Scope: "acme/a", ProducedAt: time.Now().UTC(),
		Items: []reporting.Item{
			{ID: "x", Title: "Bad state", State: "shipping"},
			{ID: "y", Title: "Dangling", State: "todo", BlockedBy: []string{"nope"}},
		},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status %d, want 400; body %s", status, raw)
	}
	for _, want := range []string{"shipping", "cross-instance dependencies are not modelled"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %q in problem list: %s", want, raw)
		}
	}
}

func TestPlanItemsSurviveTheStore(t *testing.T) {
	e := newPlanTestEnv(t)
	w := e.connect("w1", "node", "")
	due := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	e.postInstance(w.Token, reporting.Instance{
		Definition: "board", Scope: "acme/a", ProducedAt: time.Now().UTC(),
		Items: []reporting.Item{
			{
				ID: "a1", Title: "Keeps its fields", State: "doing", Lane: "feature",
				Owner: "ana", Due: &due, Size: 3, Level: "task",
			},
		},
	})

	got := e.planReport(w.Token, "board", "acme")
	if got.Plan == nil || len(got.Plan.Items) != 1 {
		t.Fatal("item did not survive the round trip")
	}
	it := got.Plan.Items[0]
	if it.Owner != "ana" || it.Size != 3 || it.Level != "task" || it.Due == nil || !it.Due.Equal(due) {
		t.Fatalf("fields lost in the round trip: %+v", it)
	}
}
