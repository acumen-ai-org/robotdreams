package orgchart

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type Status string

const (
	StatusConnected    Status = "connected"
	StatusDegraded     Status = "degraded"
	StatusDisconnected Status = "disconnected"
)

type Worker struct {
	ID          string
	Role        string
	ReportsTo   string
	PublicKey   []byte
	ConnectedAt time.Time
	LastSeenAt  time.Time
	Status      Status
	Metadata    map[string]string
}

func (w Worker) clone() Worker {
	out := w
	if w.PublicKey != nil {
		out.PublicKey = append([]byte(nil), w.PublicKey...)
	}
	if w.Metadata != nil {
		out.Metadata = make(map[string]string, len(w.Metadata))
		for k, v := range w.Metadata {
			out.Metadata[k] = v
		}
	}
	return out
}

var ErrNotFound = errors.New("orgchart: worker not found")

var ErrDuplicateWorker = errors.New("orgchart: worker already exists")

var ErrParentNotFound = errors.New("orgchart: parent worker not found")

var ErrCycle = errors.New("orgchart: reassignment would create a cycle")

var ErrInvalidWorker = errors.New("orgchart: invalid worker")

type Graph struct {
	mu      sync.RWMutex
	workers map[string]*Worker
}

func NewGraph() *Graph {
	return &Graph{workers: make(map[string]*Worker)}
}

func (g *Graph) AddWorker(w Worker) error {
	if w.ID == "" {
		return fmt.Errorf("orgchart: add worker: empty worker ID: %w", ErrInvalidWorker)
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := g.workers[w.ID]; ok {
		return fmt.Errorf("orgchart: add worker %q: %w", w.ID, ErrDuplicateWorker)
	}
	if w.ReportsTo != "" {
		if _, ok := g.workers[w.ReportsTo]; !ok {
			return fmt.Errorf("orgchart: add worker %q: reports_to %q: %w", w.ID, w.ReportsTo, ErrParentNotFound)
		}
	}

	stored := w.clone()
	g.workers[w.ID] = &stored
	return nil
}

func (g *Graph) Reassign(workerID, newParent string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	w, ok := g.workers[workerID]
	if !ok {
		return fmt.Errorf("orgchart: reassign %q: %w", workerID, ErrNotFound)
	}
	if newParent == "" {
		w.ReportsTo = ""
		return nil
	}
	if newParent == workerID {
		return fmt.Errorf("orgchart: reassign %q to itself: %w", workerID, ErrCycle)
	}
	parent, ok := g.workers[newParent]
	if !ok {
		return fmt.Errorf("orgchart: reassign %q: new parent %q: %w", workerID, newParent, ErrParentNotFound)
	}

	if g.hasAncestorLocked(parent, workerID) {
		return fmt.Errorf("orgchart: reassign %q under %q: %w", workerID, newParent, ErrCycle)
	}

	w.ReportsTo = newParent
	return nil
}

func (g *Graph) SetRole(workerID, role string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	w, ok := g.workers[workerID]
	if !ok {
		return fmt.Errorf("orgchart: set role of %q: %w", workerID, ErrNotFound)
	}
	w.Role = role
	return nil
}

func (g *Graph) hasAncestorLocked(start *Worker, ancestorID string) bool {
	for cur := start; cur != nil && cur.ReportsTo != ""; {
		if cur.ReportsTo == ancestorID {
			return true
		}
		next, ok := g.workers[cur.ReportsTo]
		if !ok {
			return false
		}
		cur = next
	}
	return false
}

func (g *Graph) Get(workerID string) (Worker, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	w, ok := g.workers[workerID]
	if !ok {
		return Worker{}, fmt.Errorf("orgchart: get %q: %w", workerID, ErrNotFound)
	}
	return w.clone(), nil
}

func (g *Graph) Children(workerID string) ([]Worker, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if workerID != "" {
		if _, ok := g.workers[workerID]; !ok {
			return nil, fmt.Errorf("orgchart: children of %q: %w", workerID, ErrNotFound)
		}
	}

	out := make([]Worker, 0)
	for _, w := range g.workers {
		if w.ReportsTo == workerID {
			out = append(out, w.clone())
		}
	}
	return out, nil
}

func (g *Graph) Ancestors(workerID string) ([]Worker, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	w, ok := g.workers[workerID]
	if !ok {
		return nil, fmt.Errorf("orgchart: ancestors of %q: %w", workerID, ErrNotFound)
	}

	out := make([]Worker, 0)
	seen := map[string]bool{workerID: true}
	for cur := w; cur.ReportsTo != ""; {
		parent, ok := g.workers[cur.ReportsTo]
		if !ok || seen[parent.ID] {
			break
		}
		seen[parent.ID] = true
		out = append(out, parent.clone())
		cur = parent
	}
	return out, nil
}

func (g *Graph) EscalationTarget(workerID string) (string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	w, ok := g.workers[workerID]
	if !ok {
		return "", fmt.Errorf("orgchart: escalation target of %q: %w", workerID, ErrNotFound)
	}
	return w.ReportsTo, nil
}

func (g *Graph) List() []Worker {
	g.mu.RLock()
	defer g.mu.RUnlock()

	out := make([]Worker, 0, len(g.workers))
	for _, w := range g.workers {
		out = append(out, w.clone())
	}
	return out
}
