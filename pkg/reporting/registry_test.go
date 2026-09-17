package reporting

import (
	"sync"
	"testing"
)

func TestRegistry(t *testing.T) {
	defs := []*Definition{
		{Name: "roadmap"},
		{Name: "activity"},
		{Name: "delivery"},
	}
	r := NewRegistry(defs)

	if d, ok := r.Get("activity"); !ok || d.Name != "activity" {
		t.Errorf("Get(activity) = %v, %v", d, ok)
	}
	if _, ok := r.Get("ghost"); ok {
		t.Error("Get(ghost) should miss")
	}

	list := r.List()
	if len(list) != 3 {
		t.Fatalf("List() has %d entries, want 3", len(list))
	}
	for i, want := range []string{"activity", "delivery", "roadmap"} {
		if list[i].Name != want {
			t.Errorf("List()[%d].Name = %q, want %q (sorted by name)", i, list[i].Name, want)
		}
	}
}

func TestRegistryConcurrent(t *testing.T) {
	r := NewRegistry([]*Definition{{Name: "a"}, {Name: "b"}})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if _, ok := r.Get("a"); !ok {
					t.Error("Get(a) missed")
					return
				}
				if got := r.List(); len(got) != 2 {
					t.Errorf("List() = %d entries, want 2", len(got))
					return
				}
			}
		}()
	}
	wg.Wait()
}
