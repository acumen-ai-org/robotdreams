package messaging

import (
	"fmt"
	"net/url"
	"sync"
)

// Constructor builds a MessagingBackend from a parsed URI. Backend
// packages implement one and register it against a URI scheme via
// Register, so that callers can select a backend at runtime by URI (e.g.
// "queue://sqlite/var/data/messages.db") without importing the backend
// package directly.
type Constructor func(uri *url.URL) (MessagingBackend, error)

var (
	registryMu sync.Mutex
	registry   = map[string]Constructor{}
)

// Register associates a URI scheme with a Constructor. Backend packages
// call Register from an init() function so that a blank import of the
// backend package (e.g.
// `import _ "github.com/acumen-ai-org/robotdreams/pkg/messaging/embedded"`)
// is enough to make the scheme available to New. This package must never
// import a concrete backend package itself — backends discover the
// registry, not the other way around.
//
// Register panics if scheme is already registered. Duplicate registration
// indicates a programming error (two backends claiming the same scheme)
// that should fail loudly at init time rather than silently shadow one
// backend with another.
func Register(scheme string, ctor Constructor) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if _, exists := registry[scheme]; exists {
		panic(fmt.Sprintf("messaging: backend already registered for scheme %q", scheme))
	}
	registry[scheme] = ctor
}

// New parses uri and dispatches to the Constructor registered for its
// scheme. It returns an error if uri cannot be parsed or if no backend is
// registered for its scheme. Callers must blank-import the desired
// backend package(s) so their init() functions run and register a
// Constructor before New is called.
func New(uri string) (MessagingBackend, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("messaging: invalid URI %q: %w", uri, err)
	}

	registryMu.Lock()
	ctor, ok := registry[u.Scheme]
	registryMu.Unlock()

	if !ok {
		return nil, fmt.Errorf("messaging: no backend registered for scheme %q", u.Scheme)
	}
	return ctor(u)
}
