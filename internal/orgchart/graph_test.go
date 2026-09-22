package orgchart

import (
	"errors"
	"sort"
	"sync"
	"testing"
)

func fixtureGraph(t *testing.T) *Graph {
	t.Helper()
	g := NewGraph()
	edges := []struct{ id, parent string }{
		{"director", ""},
		{"solo", ""},
		{"manager-a", "director"},
		{"manager-b", "director"},
		{"lead-a1", "manager-a"},
		{"lead-a2", "manager-a"},
		{"lead-b1", "manager-b"},
		{"ic-a1a", "lead-a1"},
		{"ic-a1b", "lead-a1"},
		{"ic-a2a", "lead-a2"},
	}
	for _, e := range edges {
		if err := g.AddWorker(Worker{ID: e.id, Role: "test", ReportsTo: e.parent}); err != nil {
			t.Fatalf("fixture AddWorker(%q -> %q): %v", e.id, e.parent, err)
		}
	}
	return g
}

func ids(ws []Worker) []string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.ID)
	}
	return out
}

func sortedIDs(ws []Worker) []string {
	out := ids(ws)
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAddWorker(t *testing.T) {
	tests := []struct {
		name    string
		pre     []Worker
		add     Worker
		wantErr error
	}{
		{
			name: "root parent accepted",
			add:  Worker{ID: "a", ReportsTo: ""},
		},
		{
			name: "existing parent accepted",
			pre:  []Worker{{ID: "a"}},
			add:  Worker{ID: "b", ReportsTo: "a"},
		},
		{
			name:    "duplicate id rejected",
			pre:     []Worker{{ID: "a"}},
			add:     Worker{ID: "a"},
			wantErr: ErrDuplicateWorker,
		},
		{
			name:    "dangling parent rejected",
			add:     Worker{ID: "b", ReportsTo: "nope"},
			wantErr: ErrParentNotFound,
		},
		{
			name:    "empty id rejected",
			add:     Worker{ID: ""},
			wantErr: ErrInvalidWorker,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGraph()
			for _, w := range tt.pre {
				if err := g.AddWorker(w); err != nil {
					t.Fatalf("pre AddWorker(%q): %v", w.ID, err)
				}
			}
			err := g.AddWorker(tt.add)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("AddWorker: unexpected error: %v", err)
				}
				got, err := g.Get(tt.add.ID)
				if err != nil {
					t.Fatalf("Get after add: %v", err)
				}
				if got.ReportsTo != tt.add.ReportsTo {
					t.Errorf("ReportsTo = %q, want %q", got.ReportsTo, tt.add.ReportsTo)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("AddWorker error = %v, want errors.Is(..., %v)", err, tt.wantErr)
			}
		})
	}
}

func TestAddWorkerCopiesMutableFields(t *testing.T) {
	g := NewGraph()
	meta := map[string]string{"k": "v"}
	key := []byte{1, 2, 3}
	if err := g.AddWorker(Worker{ID: "a", Metadata: meta, PublicKey: key}); err != nil {
		t.Fatalf("AddWorker: %v", err)
	}
	meta["k"] = "mutated"
	key[0] = 9

	got, err := g.Get("a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Metadata["k"] != "v" {
		t.Errorf("Metadata[k] = %q, want %q (caller mutation leaked into graph)", got.Metadata["k"], "v")
	}
	if got.PublicKey[0] != 1 {
		t.Errorf("PublicKey[0] = %d, want 1 (caller mutation leaked into graph)", got.PublicKey[0])
	}

	got.Metadata["k"] = "mutated-again"
	again, _ := g.Get("a")
	if again.Metadata["k"] != "v" {
		t.Errorf("Metadata[k] = %q, want %q (Get returned shared map)", again.Metadata["k"], "v")
	}
}

func TestReassign(t *testing.T) {
	tests := []struct {
		name      string
		worker    string
		newParent string
		wantErr   error
	}{
		{
			name:      "direct self cycle rejected",
			worker:    "manager-a",
			newParent: "manager-a",
			wantErr:   ErrCycle,
		},
		{
			name:      "one hop cycle rejected (parent under own child)",
			worker:    "manager-a",
			newParent: "lead-a1",
			wantErr:   ErrCycle,
		},
		{
			name:      "multi hop cycle rejected (director under a great-grandchild)",
			worker:    "director",
			newParent: "ic-a1a",
			wantErr:   ErrCycle,
		},
		{
			name:      "unknown worker rejected",
			worker:    "ghost",
			newParent: "director",
			wantErr:   ErrNotFound,
		},
		{
			name:      "unknown new parent rejected",
			worker:    "ic-a1a",
			newParent: "ghost",
			wantErr:   ErrParentNotFound,
		},
		{
			name:      "reassign to root allowed",
			worker:    "ic-a1a",
			newParent: "",
		},
		{
			name:      "reassign deep worker to root allowed",
			worker:    "manager-a",
			newParent: "",
		},
		{
			name:      "reassign to cousin allowed",
			worker:    "ic-a1a",
			newParent: "lead-b1",
		},
		{
			name:      "reassign to sibling allowed",
			worker:    "ic-a1a",
			newParent: "lead-a2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := fixtureGraph(t)
			before, _ := g.Get(tt.worker)

			err := g.Reassign(tt.worker, tt.newParent)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Reassign error = %v, want errors.Is(..., %v)", err, tt.wantErr)
				}
				if tt.worker != "ghost" {
					after, _ := g.Get(tt.worker)
					if after.ReportsTo != before.ReportsTo {
						t.Errorf("failed Reassign mutated ReportsTo: %q -> %q", before.ReportsTo, after.ReportsTo)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Reassign: unexpected error: %v", err)
			}

			got, err := g.Get(tt.worker)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got.ReportsTo != tt.newParent {
				t.Errorf("ReportsTo = %q, want %q", got.ReportsTo, tt.newParent)
			}
			target, err := g.EscalationTarget(tt.worker)
			if err != nil {
				t.Fatalf("EscalationTarget: %v", err)
			}
			if target != tt.newParent {
				t.Errorf("EscalationTarget = %q, want %q", target, tt.newParent)
			}
			if tt.newParent != "" {
				kids, err := g.Children(tt.newParent)
				if err != nil {
					t.Fatalf("Children(%q): %v", tt.newParent, err)
				}
				found := false
				for _, k := range kids {
					if k.ID == tt.worker {
						found = true
					}
				}
				if !found {
					t.Errorf("Children(%q) = %v, missing reassigned %q", tt.newParent, sortedIDs(kids), tt.worker)
				}
			}
			if before.ReportsTo != "" {
				kids, err := g.Children(before.ReportsTo)
				if err != nil {
					t.Fatalf("Children(%q): %v", before.ReportsTo, err)
				}
				for _, k := range kids {
					if k.ID == tt.worker {
						t.Errorf("old parent %q still lists %q as a child", before.ReportsTo, tt.worker)
					}
				}
			}
			anc, err := g.Ancestors(tt.worker)
			if err != nil {
				t.Fatalf("Ancestors: %v", err)
			}
			for _, a := range anc {
				if a.ID == tt.worker {
					t.Fatalf("Ancestors(%q) contains itself: %v", tt.worker, ids(anc))
				}
			}
		})
	}
}

func TestReassignThreeLevelCycleAttempt(t *testing.T) {
	g := NewGraph()
	for _, e := range []struct{ id, parent string }{{"A", ""}, {"B", "A"}, {"C", "B"}} {
		if err := g.AddWorker(Worker{ID: e.id, ReportsTo: e.parent}); err != nil {
			t.Fatalf("AddWorker(%q): %v", e.id, err)
		}
	}
	if err := g.Reassign("A", "C"); !errors.Is(err, ErrCycle) {
		t.Fatalf("Reassign(A, C) error = %v, want ErrCycle", err)
	}
	if err := g.Reassign("C", ""); err != nil {
		t.Fatalf("Reassign(C, root): %v", err)
	}
	if err := g.Reassign("A", "C"); err != nil {
		t.Fatalf("Reassign(A, C) after detach: %v", err)
	}
	anc, err := g.Ancestors("B")
	if err != nil {
		t.Fatalf("Ancestors(B): %v", err)
	}
	if want := []string{"A", "C"}; !equalStrings(ids(anc), want) {
		t.Errorf("Ancestors(B) = %v, want %v", ids(anc), want)
	}
}

func TestEscalationTargetOneHopPerLevel(t *testing.T) {
	g := fixtureGraph(t)
	tests := []struct {
		worker string
		want   string
	}{
		{"ic-a1a", "lead-a1"},
		{"lead-a1", "manager-a"},
		{"manager-a", "director"},
		{"director", ""},
		{"solo", ""},
	}
	for _, tt := range tests {
		t.Run(tt.worker, func(t *testing.T) {
			got, err := g.EscalationTarget(tt.worker)
			if err != nil {
				t.Fatalf("EscalationTarget(%q): %v", tt.worker, err)
			}
			if got != tt.want {
				t.Errorf("EscalationTarget(%q) = %q, want %q", tt.worker, got, tt.want)
			}
		})
	}
	hops := []string{}
	cur := "ic-a1a"
	for {
		next, err := g.EscalationTarget(cur)
		if err != nil {
			t.Fatalf("EscalationTarget(%q): %v", cur, err)
		}
		if next == "" {
			break
		}
		hops = append(hops, next)
		cur = next
	}
	if want := []string{"lead-a1", "manager-a", "director"}; !equalStrings(hops, want) {
		t.Errorf("escalation hops = %v, want %v", hops, want)
	}

	if _, err := g.EscalationTarget("ghost"); !errors.Is(err, ErrNotFound) {
		t.Errorf("EscalationTarget(ghost) error = %v, want ErrNotFound", err)
	}
}

func TestAncestors(t *testing.T) {
	g := fixtureGraph(t)
	tests := []struct {
		worker  string
		want    []string
		wantErr error
	}{
		{worker: "ic-a1a", want: []string{"lead-a1", "manager-a", "director"}},
		{worker: "ic-a2a", want: []string{"lead-a2", "manager-a", "director"}},
		{worker: "lead-b1", want: []string{"manager-b", "director"}},
		{worker: "director", want: []string{}},
		{worker: "solo", want: []string{}},
		{worker: "ghost", wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.worker, func(t *testing.T) {
			got, err := g.Ancestors(tt.worker)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Ancestors error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Ancestors: %v", err)
			}
			if !equalStrings(ids(got), tt.want) {
				t.Errorf("Ancestors(%q) = %v, want %v (nearest parent first)", tt.worker, ids(got), tt.want)
			}
		})
	}
}

func TestChildren(t *testing.T) {
	g := fixtureGraph(t)
	tests := []struct {
		worker  string
		want    []string
		wantErr error
	}{
		{worker: "", want: []string{"director", "solo"}},
		{worker: "director", want: []string{"manager-a", "manager-b"}},
		{worker: "manager-a", want: []string{"lead-a1", "lead-a2"}},
		{worker: "lead-a1", want: []string{"ic-a1a", "ic-a1b"}},
		{worker: "ic-a1a", want: []string{}},
		{worker: "ghost", wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		name := tt.worker
		if name == "" {
			name = "root"
		}
		t.Run(name, func(t *testing.T) {
			got, err := g.Children(tt.worker)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Children error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Children: %v", err)
			}
			if !equalStrings(sortedIDs(got), tt.want) {
				t.Errorf("Children(%q) = %v, want %v", tt.worker, sortedIDs(got), tt.want)
			}
		})
	}
}

func TestGetAndList(t *testing.T) {
	g := fixtureGraph(t)

	if _, err := g.Get("ghost"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(ghost) error = %v, want ErrNotFound", err)
	}
	w, err := g.Get("lead-a1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if w.ReportsTo != "manager-a" {
		t.Errorf("ReportsTo = %q, want manager-a", w.ReportsTo)
	}

	all := g.List()
	if len(all) != 10 {
		t.Fatalf("List() len = %d, want 10", len(all))
	}
	want := []string{"director", "ic-a1a", "ic-a1b", "ic-a2a", "lead-a1", "lead-a2", "lead-b1", "manager-a", "manager-b", "solo"}
	if !equalStrings(sortedIDs(all), want) {
		t.Errorf("List() = %v, want %v", sortedIDs(all), want)
	}

	if len(NewGraph().List()) != 0 {
		t.Error("List() on empty graph should be empty")
	}
}

func TestConcurrentAccess(t *testing.T) {
	g := NewGraph()
	if err := g.AddWorker(Worker{ID: "root-lead"}); err != nil {
		t.Fatalf("AddWorker: %v", err)
	}

	const goroutines = 8
	const iterations = 50

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				id := string(rune('a'+n)) + "-" + string(rune('0'+j%10))
				_ = g.AddWorker(Worker{
					ID:        id,
					Role:      "contributor",
					ReportsTo: "root-lead",
					Metadata:  map[string]string{"n": id},
					Status:    StatusConnected,
				})
				_ = g.Reassign(id, "")
				_ = g.Reassign(id, "root-lead")
				_, _ = g.Get(id)
				_, _ = g.Get("root-lead")
				_, _ = g.Children("root-lead")
				_, _ = g.Ancestors(id)
				_, _ = g.EscalationTarget(id)
				_ = g.List()
				_, _ = Rollup(g, "root-lead")
			}
		}(i)
	}
	wg.Wait()

	if _, err := g.Get("root-lead"); err != nil {
		t.Fatalf("graph unusable after concurrent access: %v", err)
	}
}

func TestSetRole(t *testing.T) {
	tests := []struct {
		name    string
		worker  string
		role    string
		wantErr error
	}{
		{name: "relabels a worker", worker: "lead-a1", role: "architect"},
		{name: "clears a role", worker: "lead-a1", role: ""},
		{name: "relabels a root", worker: "director", role: "principal"},
		{name: "unknown worker rejected", worker: "ghost", role: "architect", wantErr: ErrNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := fixtureGraph(t)

			err := g.SetRole(tt.worker, tt.role)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("SetRole error = %v, want errors.Is(..., %v)", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("SetRole: unexpected error: %v", err)
			}

			got, err := g.Get(tt.worker)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got.Role != tt.role {
				t.Errorf("Role = %q, want %q", got.Role, tt.role)
			}
			if got.ReportsTo != "" && got.ReportsTo != "director" && got.ReportsTo != "manager-a" {
				t.Errorf("SetRole moved the worker: ReportsTo = %q", got.ReportsTo)
			}
		})
	}
}

func TestSetRoleLeavesSiblingsAlone(t *testing.T) {
	g := fixtureGraph(t)
	if err := g.SetRole("lead-a1", "architect"); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	sibling, err := g.Get("lead-a2")
	if err != nil {
		t.Fatalf("Get(lead-a2): %v", err)
	}
	if sibling.Role != "test" {
		t.Errorf("sibling Role = %q, want the untouched fixture role", sibling.Role)
	}
}
