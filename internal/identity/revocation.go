package identity

import (
	"context"
	"sync"
	"time"
)

type RevocationStore interface {
	IsRevoked(ctx context.Context, workerID string) (bool, error)
	Revoke(ctx context.Context, workerID string, reason string) error
}

type revocationEntry struct {
	Reason    string
	RevokedAt time.Time
}

type InMemoryRevocationStore struct {
	clock Clock

	mu      sync.RWMutex
	revoked map[string]revocationEntry
}

func NewInMemoryRevocationStore(clock Clock) *InMemoryRevocationStore {
	if clock == nil {
		clock = RealClock{}
	}
	return &InMemoryRevocationStore{
		clock:   clock,
		revoked: make(map[string]revocationEntry),
	}
}

func (s *InMemoryRevocationStore) IsRevoked(_ context.Context, workerID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.revoked[workerID]
	return ok, nil
}

func (s *InMemoryRevocationStore) Revoke(_ context.Context, workerID string, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revoked[workerID] = revocationEntry{
		Reason:    reason,
		RevokedAt: s.clock.Now(),
	}
	return nil
}

func (s *InMemoryRevocationStore) Unrevoke(workerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.revoked, workerID)
}
