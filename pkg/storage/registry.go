package storage

import (
	"fmt"
	"net/url"
	"sync"
)

// Constructor builds a StorageBackend from a parsed URI.
type Constructor func(uri *url.URL) (StorageBackend, error)

var (
	registryMu sync.Mutex
	registry   = map[string]Constructor{}
)

// Register associates a URI scheme with a Constructor and panics if the scheme is already registered.
func Register(scheme string, ctor Constructor) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if _, exists := registry[scheme]; exists {
		panic(fmt.Sprintf("storage: backend already registered for scheme %q", scheme))
	}
	registry[scheme] = ctor
}

// New parses uri and dispatches to the Constructor registered for its scheme.
func New(uri string) (StorageBackend, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("storage: invalid URI %q: %w", uri, err)
	}

	registryMu.Lock()
	ctor, ok := registry[u.Scheme]
	registryMu.Unlock()

	if !ok {
		return nil, fmt.Errorf("storage: no backend registered for scheme %q", u.Scheme)
	}
	return ctor(u)
}
