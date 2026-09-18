package webhook

import (
	"sync"

	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
)

type ringStore struct {
	mu       sync.Mutex
	cap      int
	byWorker map[string][]messaging.Envelope
}

func newRingStore(capacity int) *ringStore {
	return &ringStore{cap: capacity, byWorker: make(map[string][]messaging.Envelope)}
}

func (r *ringStore) add(env messaging.Envelope) {
	r.mu.Lock()
	defer r.mu.Unlock()

	list := append(r.byWorker[env.To], env)
	if len(list) > r.cap {
		list = list[len(list)-r.cap:]
	}
	r.byWorker[env.To] = list
}

func (r *ringStore) contains(workerID, id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, env := range r.byWorker[workerID] {
		if env.ID == id {
			return true
		}
	}
	return false
}

func (r *ringStore) tail(filter messaging.TailFilter) []messaging.Envelope {
	r.mu.Lock()
	defer r.mu.Unlock()

	var source []messaging.Envelope
	if filter.WorkerID != "" {
		source = r.byWorker[filter.WorkerID]
	} else {
		for _, list := range r.byWorker {
			source = append(source, list...)
		}
	}

	var out []messaging.Envelope
	for i := len(source) - 1; i >= 0; i-- {
		env := source[i]
		if !filter.Since.IsZero() && env.CreatedAt.Before(filter.Since) {
			continue
		}
		out = append(out, env)
		if filter.Limit > 0 && len(out) >= filter.Limit {
			break
		}
	}
	return out
}
