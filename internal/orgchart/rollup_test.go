package orgchart

import (
	"errors"
	"testing"
	"time"
)

var (
	t1 = time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	t2 = time.Date(2026, 8, 19, 11, 0, 0, 0, time.UTC)
	t3 = time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	t4 = time.Date(2026, 8, 19, 13, 0, 0, 0, time.UTC)
)

func rollupFixture(t *testing.T) *Graph {
	t.Helper()
	g := NewGraph()
	add := func(w Worker) {
		t.Helper()
		if err := g.AddWorker(w); err != nil {
			t.Fatalf("AddWorker(%q): %v", w.ID, err)
		}
	}
	add(Worker{ID: "lead", Status: StatusConnected, LastSeenAt: t4})
	add(Worker{ID: "c1", ReportsTo: "lead", Status: StatusConnected, LastSeenAt: t1})
	add(Worker{ID: "c2", ReportsTo: "lead", Status: StatusConnected, LastSeenAt: t3})
	add(Worker{ID: "c3", ReportsTo: "lead", Status: StatusDegraded, LastSeenAt: t2})
	add(Worker{ID: "c4", ReportsTo: "lead", Status: StatusDisconnected, LastSeenAt: t1})
	add(Worker{ID: "g1", ReportsTo: "c1", Status: StatusConnected, LastSeenAt: t4})
	return g
}

func TestRollup(t *testing.T) {
	g := rollupFixture(t)

	tests := []struct {
		name    string
		worker  string
		want    RollupSummary
		wantErr error
	}{
		{
			name:   "lead with four mixed children",
			worker: "lead",
			want:   RollupSummary{DirectReports: 4, ActiveCount: 2, LastActivity: t3},
		},
		{
			name:   "single connected child",
			worker: "c1",
			want:   RollupSummary{DirectReports: 1, ActiveCount: 1, LastActivity: t4},
		},
		{
			name:   "leaf has zero-value summary",
			worker: "c2",
			want:   RollupSummary{},
		},
		{
			name:   "root sees top-level workers",
			worker: "",
			want:   RollupSummary{DirectReports: 1, ActiveCount: 1, LastActivity: t4},
		},
		{
			name:    "unknown worker",
			worker:  "ghost",
			wantErr: ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Rollup(g, tt.worker)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Rollup error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Rollup: %v", err)
			}
			if got != tt.want {
				t.Errorf("Rollup(%q) = %+v, want %+v", tt.worker, got, tt.want)
			}
		})
	}
}

func TestRollupLeafLastActivityIsZero(t *testing.T) {
	g := rollupFixture(t)
	got, err := Rollup(g, "c3")
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if !got.LastActivity.IsZero() {
		t.Errorf("LastActivity = %v, want zero for a worker with no reports", got.LastActivity)
	}
}

func TestRollupWithActivity(t *testing.T) {
	t5 := time.Date(2026, 8, 19, 14, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		worker   string
		activity map[string]ActivityInfo
		want     RollupSummary
	}{
		{
			name:   "nil activity matches Rollup",
			worker: "lead",
			want:   RollupSummary{DirectReports: 4, ActiveCount: 2, LastActivity: t3},
		},
		{
			name:   "empty activity matches Rollup",
			worker: "lead", activity: map[string]ActivityInfo{},
			want: RollupSummary{DirectReports: 4, ActiveCount: 2, LastActivity: t3},
		},
		{
			name:   "override subset promotes a disconnected child",
			worker: "lead",
			activity: map[string]ActivityInfo{
				"c4": {Status: StatusConnected, LastSeenAt: t5},
			},
			want: RollupSummary{DirectReports: 4, ActiveCount: 3, LastActivity: t5},
		},
		{
			name:   "override subset demotes a connected child",
			worker: "lead",
			activity: map[string]ActivityInfo{
				"c2": {Status: StatusDisconnected, LastSeenAt: t1},
			},
			want: RollupSummary{DirectReports: 4, ActiveCount: 1, LastActivity: t2},
		},
		{
			name:   "override every child",
			worker: "lead",
			activity: map[string]ActivityInfo{
				"c1": {Status: StatusConnected, LastSeenAt: t1},
				"c2": {Status: StatusConnected, LastSeenAt: t1},
				"c3": {Status: StatusConnected, LastSeenAt: t1},
				"c4": {Status: StatusConnected, LastSeenAt: t1},
			},
			want: RollupSummary{DirectReports: 4, ActiveCount: 4, LastActivity: t1},
		},
		{
			name:   "entries for non-children are ignored",
			worker: "lead",
			activity: map[string]ActivityInfo{
				"g1":    {Status: StatusConnected, LastSeenAt: t5},
				"ghost": {Status: StatusConnected, LastSeenAt: t5},
			},
			want: RollupSummary{DirectReports: 4, ActiveCount: 2, LastActivity: t3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := rollupFixture(t)
			got, err := RollupWithActivity(g, tt.worker, tt.activity)
			if err != nil {
				t.Fatalf("RollupWithActivity: %v", err)
			}
			if got != tt.want {
				t.Errorf("RollupWithActivity(%q) = %+v, want %+v", tt.worker, got, tt.want)
			}
		})
	}
}

func TestRollupWithActivityUnknownWorker(t *testing.T) {
	g := rollupFixture(t)
	if _, err := RollupWithActivity(g, "ghost", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestRollupReflectsReassignment(t *testing.T) {
	g := rollupFixture(t)
	if err := g.Reassign("c4", ""); err != nil {
		t.Fatalf("Reassign: %v", err)
	}
	got, err := Rollup(g, "lead")
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	want := RollupSummary{DirectReports: 3, ActiveCount: 2, LastActivity: t3}
	if got != want {
		t.Errorf("Rollup after reassign = %+v, want %+v", got, want)
	}
}
