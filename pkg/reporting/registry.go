package reporting

import (
	"sort"
	"sync"
)

// Registry is a concurrency-safe, name-keyed set of report definitions.
type Registry struct {
	mu     sync.RWMutex
	byName map[string]*Definition
}

// NewRegistry builds a registry from a slice of definitions, typically the result of LoadLibraryDir.
func NewRegistry(defs []*Definition) *Registry {
	r := &Registry{byName: make(map[string]*Definition, len(defs))}
	for _, d := range defs {
		if d == nil {
			continue
		}
		r.byName[d.Name] = d
	}
	return r
}

// Get returns the definition with the given name, if registered.
func (r *Registry) Get(name string) (*Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.byName[name]
	return d, ok
}

// List returns all registered definitions sorted by name.
func (r *Registry) List() []*Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Definition, 0, len(r.byName))
	for _, d := range r.byName {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
