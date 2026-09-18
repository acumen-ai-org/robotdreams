package orgchart

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validChart = `workers:
  - id: lead-1
    role: lead
    reports_to: ""
  - id: lead-2
    role: lead
    reports_to: ""
  - id: worker-a
    role: contributor
    reports_to: lead-1
  - id: worker-b
    role: contributor
    reports_to: lead-1
  - id: worker-c
    role: contributor
    reports_to: lead-2
`

func TestParseChartFileValid(t *testing.T) {
	got, err := ParseChartFile([]byte(validChart))
	if err != nil {
		t.Fatalf("ParseChartFile: %v", err)
	}
	want := []Worker{
		{ID: "lead-1", Role: "lead"},
		{ID: "lead-2", Role: "lead"},
		{ID: "worker-a", Role: "contributor", ReportsTo: "lead-1"},
		{ID: "worker-b", Role: "contributor", ReportsTo: "lead-1"},
		{ID: "worker-c", Role: "contributor", ReportsTo: "lead-2"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d workers, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i].ID || got[i].Role != want[i].Role || got[i].ReportsTo != want[i].ReportsTo {
			t.Errorf("workers[%d] = %+v, want %+v", i, got[i], want[i])
		}
		if got[i].PublicKey != nil || got[i].Metadata != nil {
			t.Errorf("workers[%d]: PublicKey/Metadata should be nil, got %v/%v", i, got[i].PublicKey, got[i].Metadata)
		}
		if !got[i].ConnectedAt.IsZero() || !got[i].LastSeenAt.IsZero() {
			t.Errorf("workers[%d]: timestamps should be zero", i)
		}
		if got[i].Status != "" {
			t.Errorf("workers[%d]: Status = %q, want empty", i, got[i].Status)
		}
	}
}

func TestParseChartFileLoadsIntoGraph(t *testing.T) {
	workers, err := ParseChartFile([]byte(validChart))
	if err != nil {
		t.Fatalf("ParseChartFile: %v", err)
	}
	g := NewGraph()
	for _, w := range workers {
		if err := g.AddWorker(w); err != nil {
			t.Fatalf("AddWorker(%q): %v", w.ID, err)
		}
	}
	kids, err := g.Children("lead-1")
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if len(kids) != 2 {
		t.Errorf("lead-1 children = %v, want 2", sortedIDs(kids))
	}
	target, err := g.EscalationTarget("worker-c")
	if err != nil {
		t.Fatalf("EscalationTarget: %v", err)
	}
	if target != "lead-2" {
		t.Errorf("EscalationTarget(worker-c) = %q, want lead-2", target)
	}
}

func TestParseChartFileValidation(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		wantContains []string
	}{
		{
			name: "dangling reports_to",
			yaml: `workers:
  - id: a
    reports_to: ghost
`,
			wantContains: []string{"dangling reports_to", `"ghost"`},
		},
		{
			name: "duplicate id",
			yaml: `workers:
  - id: a
  - id: a
`,
			wantContains: []string{"duplicate id"},
		},
		{
			name: "empty id",
			yaml: `workers:
  - id: ""
    role: lead
`,
			wantContains: []string{"empty id"},
		},
		{
			name: "self reference",
			yaml: `workers:
  - id: a
    reports_to: a
`,
			wantContains: []string{"reports to itself"},
		},
		{
			name: "two node cycle",
			yaml: `workers:
  - id: a
    reports_to: b
  - id: b
    reports_to: a
`,
			wantContains: []string{"cycle", `"a"`, `"b"`},
		},
		{
			name: "three node cycle",
			yaml: `workers:
  - id: a
    reports_to: c
  - id: b
    reports_to: a
  - id: c
    reports_to: b
`,
			wantContains: []string{"cycle", `"a"`, `"b"`, `"c"`},
		},
		{
			name: "cycle alongside a valid tree",
			yaml: `workers:
  - id: lead
    reports_to: ""
  - id: ok
    reports_to: lead
  - id: x
    reports_to: y
  - id: y
    reports_to: x
`,
			wantContains: []string{`"x"`, `"y"`},
		},
		{
			name: "multiple problems all reported",
			yaml: `workers:
  - id: a
  - id: a
  - id: b
    reports_to: ghost
  - id: ""
  - id: c
    reports_to: d
  - id: d
    reports_to: c
`,
			wantContains: []string{"duplicate id", "dangling reports_to", "empty id", "cycle"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseChartFile([]byte(tt.yaml))
			if err == nil {
				t.Fatalf("ParseChartFile succeeded, want error (got %+v)", got)
			}
			if got != nil {
				t.Errorf("workers = %+v, want nil on error", got)
			}
			if !errors.Is(err, ErrInvalidChartFile) {
				t.Errorf("error = %v, want errors.Is(..., ErrInvalidChartFile)", err)
			}
			msg := err.Error()
			for _, want := range tt.wantContains {
				if !strings.Contains(msg, want) {
					t.Errorf("error %q does not contain %q", msg, want)
				}
			}
		})
	}
}

func TestParseChartFileCollectsEveryProblem(t *testing.T) {
	const bad = `workers:
  - id: a
  - id: a
  - id: b
    reports_to: ghost
  - id: c
    reports_to: nowhere
`
	_, err := ParseChartFile([]byte(bad))
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, want := range []string{`duplicate id "a"`, `"ghost"`, `"nowhere"`} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	if n := strings.Count(msg, "\n"); n < 2 {
		t.Errorf("expected at least 3 joined problems, got %d newlines in %q", n, msg)
	}
}

func TestParseChartFileMalformedYAML(t *testing.T) {
	_, err := ParseChartFile([]byte("workers: [ unclosed"))
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
	if !strings.Contains(err.Error(), "orgchart:") {
		t.Errorf("error %q should be package-prefixed", err.Error())
	}
}

func TestParseChartFileEmpty(t *testing.T) {
	got, err := ParseChartFile([]byte(""))
	if err != nil {
		t.Fatalf("ParseChartFile(empty): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d workers, want 0", len(got))
	}
}

func TestLoadChartFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chart.yaml")
	if err := os.WriteFile(path, []byte(validChart), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := LoadChartFile(path)
	if err != nil {
		t.Fatalf("LoadChartFile: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("got %d workers, want 5", len(got))
	}
	if got[0].ID != "lead-1" {
		t.Errorf("workers[0].ID = %q, want lead-1", got[0].ID)
	}
}

func TestLoadChartFileErrors(t *testing.T) {
	dir := t.TempDir()

	t.Run("missing file", func(t *testing.T) {
		_, err := LoadChartFile(filepath.Join(dir, "nope.yaml"))
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("error = %v, want os.ErrNotExist", err)
		}
	})

	t.Run("invalid contents keep path in message", func(t *testing.T) {
		path := filepath.Join(dir, "bad.yaml")
		if err := os.WriteFile(path, []byte("workers:\n  - id: a\n    reports_to: ghost\n"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		_, err := LoadChartFile(path)
		if !errors.Is(err, ErrInvalidChartFile) {
			t.Fatalf("error = %v, want ErrInvalidChartFile", err)
		}
		if !strings.Contains(err.Error(), "bad.yaml") {
			t.Errorf("error %q should name the file", err.Error())
		}
	})
}

func TestLoadSampleOrgChart(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "sample-orgchart.yaml")
	workers, err := LoadChartFile(path)
	if err != nil {
		t.Fatalf("LoadChartFile(%s): %v", path, err)
	}
	if len(workers) == 0 {
		t.Fatal("sample org chart declares no workers")
	}

	g := NewGraph()
	for _, w := range workers {
		if err := g.AddWorker(w); err != nil {
			t.Fatalf("AddWorker(%q) from sample chart: %v", w.ID, err)
		}
	}
	tops, err := g.Children("")
	if err != nil {
		t.Fatalf("Children(root): %v", err)
	}
	if len(tops) < 2 {
		t.Errorf("sample chart has %d top-level workers, want at least 2 leads", len(tops))
	}
	for _, lead := range tops {
		sum, err := Rollup(g, lead.ID)
		if err != nil {
			t.Fatalf("Rollup(%q): %v", lead.ID, err)
		}
		if sum.DirectReports < 2 {
			t.Errorf("lead %q has %d direct reports, want at least 2", lead.ID, sum.DirectReports)
		}
	}
}
