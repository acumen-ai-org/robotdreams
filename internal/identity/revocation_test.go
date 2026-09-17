package identity

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryRevocationStore(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	store := NewInMemoryRevocationStore(clock)
	ctx := context.Background()

	revoked, err := store.IsRevoked(ctx, "worker-1")
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
	}
	if revoked {
		t.Error("unrevoked worker reported as revoked")
	}

	if err := store.Revoke(ctx, "worker-1", "compromised key"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	revoked, err = store.IsRevoked(ctx, "worker-1")
	if err != nil {
		t.Fatalf("IsRevoked after revoke: %v", err)
	}
	if !revoked {
		t.Error("revoked worker reported as not revoked")
	}
	revoked, err = store.IsRevoked(ctx, "worker-2")
	if err != nil {
		t.Fatalf("IsRevoked (other worker): %v", err)
	}
	if revoked {
		t.Error("unrelated worker reported as revoked")
	}
	if err := store.Revoke(ctx, "worker-1", "revoked again"); err != nil {
		t.Fatalf("Revoke (second time): %v", err)
	}

	store.Unrevoke("worker-1")
	revoked, err = store.IsRevoked(ctx, "worker-1")
	if err != nil {
		t.Fatalf("IsRevoked after unrevoke: %v", err)
	}
	if revoked {
		t.Error("unrevoked worker still reported as revoked")
	}
}
