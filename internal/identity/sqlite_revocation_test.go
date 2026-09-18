package identity

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestSQLiteStore(t *testing.T) (*SQLiteRevocationStore, *fakeClock) {
	t.Helper()

	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	store, err := NewSQLiteRevocationStore(filepath.Join(t.TempDir(), "revocations.db"), clock)
	if err != nil {
		t.Fatalf("NewSQLiteRevocationStore: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return store, clock
}

func TestSQLiteRevocationStoreFreshIsNotRevoked(t *testing.T) {
	store, _ := newTestSQLiteStore(t)

	revoked, err := store.IsRevoked(context.Background(), "worker-unknown")
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
	}
	if revoked {
		t.Error("unknown worker reported as revoked")
	}
}

func TestSQLiteRevocationStoreRevokeThenIsRevoked(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()

	if err := store.Revoke(ctx, "worker-1", "compromised key"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	revoked, err := store.IsRevoked(ctx, "worker-1")
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
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
}

func TestSQLiteRevocationStoreRevokeTwiceUpdatesReasonAndTime(t *testing.T) {
	store, clock := newTestSQLiteStore(t)
	ctx := context.Background()

	first := clock.Now()
	if err := store.Revoke(ctx, "worker-1", "first reason"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	clock.Advance(90 * time.Second)
	second := clock.Now()
	if err := store.Revoke(ctx, "worker-1", "second reason"); err != nil {
		t.Fatalf("Revoke (second time): %v", err)
	}

	records, err := store.ListRevoked(ctx)
	if err != nil {
		t.Fatalf("ListRevoked: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("ListRevoked returned %d records, want 1 (upsert, not insert)", len(records))
	}
	if records[0].Reason != "second reason" {
		t.Errorf("Reason = %q, want %q", records[0].Reason, "second reason")
	}
	if !records[0].RevokedAt.Equal(second) {
		t.Errorf("RevokedAt = %v, want %v (latest revocation)", records[0].RevokedAt, second)
	}
	if records[0].RevokedAt.Equal(first) {
		t.Error("RevokedAt still reflects the first revocation")
	}
}

func TestSQLiteRevocationStoreUnrevoke(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()

	if err := store.Revoke(ctx, "worker-1", "compromised key"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if err := store.Unrevoke(ctx, "worker-1"); err != nil {
		t.Fatalf("Unrevoke: %v", err)
	}

	revoked, err := store.IsRevoked(ctx, "worker-1")
	if err != nil {
		t.Fatalf("IsRevoked after unrevoke: %v", err)
	}
	if revoked {
		t.Error("unrevoked worker still reported as revoked")
	}
	if err := store.Unrevoke(ctx, "worker-never-revoked"); err != nil {
		t.Fatalf("Unrevoke (unknown worker): %v", err)
	}
}

func TestSQLiteRevocationStoreListRevoked(t *testing.T) {
	store, clock := newTestSQLiteStore(t)
	ctx := context.Background()

	if empty, err := store.ListRevoked(ctx); err != nil {
		t.Fatalf("ListRevoked (empty): %v", err)
	} else if len(empty) != 0 {
		t.Fatalf("ListRevoked on fresh store returned %d records, want 0", len(empty))
	}

	want := map[string]struct {
		reason    string
		revokedAt time.Time
	}{}
	for _, w := range []struct{ id, reason string }{
		{"worker-1", "compromised key"},
		{"worker-2", "decommissioned"},
		{"worker-3", "policy violation"},
	} {
		if err := store.Revoke(ctx, w.id, w.reason); err != nil {
			t.Fatalf("Revoke %s: %v", w.id, err)
		}
		want[w.id] = struct {
			reason    string
			revokedAt time.Time
		}{w.reason, clock.Now()}
		clock.Advance(time.Minute)
	}

	records, err := store.ListRevoked(ctx)
	if err != nil {
		t.Fatalf("ListRevoked: %v", err)
	}
	if len(records) != len(want) {
		t.Fatalf("ListRevoked returned %d records, want %d", len(records), len(want))
	}
	for _, rec := range records {
		exp, ok := want[rec.WorkerID]
		if !ok {
			t.Errorf("unexpected worker %q in revocation list", rec.WorkerID)
			continue
		}
		if rec.Reason != exp.reason {
			t.Errorf("%s: Reason = %q, want %q", rec.WorkerID, rec.Reason, exp.reason)
		}
		if !rec.RevokedAt.Equal(exp.revokedAt) {
			t.Errorf("%s: RevokedAt = %v, want %v", rec.WorkerID, rec.RevokedAt, exp.revokedAt)
		}
	}
	for i := 1; i < len(records); i++ {
		if records[i-1].RevokedAt.Before(records[i].RevokedAt) {
			t.Errorf("ListRevoked not ordered newest-first at index %d", i)
		}
	}
}

func TestSQLiteRevocationStorePersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "revocations.db")
	ctx := context.Background()
	clock := newFakeClock(time.Unix(1_700_000_000, 0))

	store, err := NewSQLiteRevocationStore(path, clock)
	if err != nil {
		t.Fatalf("NewSQLiteRevocationStore: %v", err)
	}
	if err := store.Revoke(ctx, "worker-1", "compromised key"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := NewSQLiteRevocationStore(path, clock)
	if err != nil {
		t.Fatalf("NewSQLiteRevocationStore (reopen): %v", err)
	}
	defer reopened.Close()

	revoked, err := reopened.IsRevoked(ctx, "worker-1")
	if err != nil {
		t.Fatalf("IsRevoked after reopen: %v", err)
	}
	if !revoked {
		t.Error("revocation did not survive reopening the database")
	}
}

func TestSQLiteRevocationStoreOwningCloseClosesDB(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	if !store.ownsDB {
		t.Fatal("store opened by NewSQLiteRevocationStore should own its db")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := store.db.PingContext(context.Background()); err == nil {
		t.Error("owning Close left the underlying database usable")
	}
	store.ownsDB = false
}

func TestSQLiteRevocationStoreFromDB(t *testing.T) {
	ctx := context.Background()
	clock := newFakeClock(time.Unix(1_700_000_000, 0))

	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "shared.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	store, err := NewSQLiteRevocationStoreFromDB(db, clock)
	if err != nil {
		t.Fatalf("NewSQLiteRevocationStoreFromDB: %v", err)
	}
	if store.ownsDB {
		t.Error("FromDB-constructed store must not claim ownership of the db")
	}

	if err := store.Revoke(ctx, "worker-1", "compromised key"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	revoked, err := store.IsRevoked(ctx, "worker-1")
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
	}
	if !revoked {
		t.Error("revoked worker reported as not revoked")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("FromDB Close closed the caller-owned database: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workers (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("caller db unusable after store.Close: %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM revocations`).Scan(&count); err != nil {
		t.Fatalf("query revocations via caller db: %v", err)
	}
	if count != 1 {
		t.Errorf("revocations row count = %d, want 1", count)
	}
}

func TestSQLiteRevocationStoreNilClockAndNilDB(t *testing.T) {
	store, err := NewSQLiteRevocationStore(filepath.Join(t.TempDir(), "revocations.db"), nil)
	if err != nil {
		t.Fatalf("NewSQLiteRevocationStore with nil clock: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	before := time.Now()
	if err := store.Revoke(ctx, "worker-1", "compromised key"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	after := time.Now()

	records, err := store.ListRevoked(ctx)
	if err != nil {
		t.Fatalf("ListRevoked: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("ListRevoked returned %d records, want 1", len(records))
	}
	if records[0].RevokedAt.Before(before) || records[0].RevokedAt.After(after) {
		t.Errorf("RevokedAt %v outside [%v, %v] — nil clock should default to RealClock",
			records[0].RevokedAt, before, after)
	}

	if _, err := NewSQLiteRevocationStoreFromDB(nil, nil); err == nil {
		t.Error("NewSQLiteRevocationStoreFromDB(nil, nil) should return an error")
	}
}

func TestSQLiteRevocationStoreConcurrentUse(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()

	const workers = 8
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := string(rune('a' + i))
			for range 20 {
				if err := store.Revoke(ctx, id, "churn"); err != nil {
					t.Errorf("Revoke: %v", err)
					return
				}
				if _, err := store.IsRevoked(ctx, id); err != nil {
					t.Errorf("IsRevoked: %v", err)
					return
				}
				if _, err := store.ListRevoked(ctx); err != nil {
					t.Errorf("ListRevoked: %v", err)
					return
				}
			}
		}(i)
	}
	wg.Wait()

	records, err := store.ListRevoked(ctx)
	if err != nil {
		t.Fatalf("ListRevoked: %v", err)
	}
	if len(records) != workers {
		t.Errorf("ListRevoked returned %d records, want %d", len(records), workers)
	}
}

func TestSQLiteRevocationStoreSatisfiesInterface(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	var rs RevocationStore = store
	ctx := context.Background()

	if err := rs.Revoke(ctx, "worker-1", "compromised key"); err != nil {
		t.Fatalf("Revoke via interface: %v", err)
	}
	revoked, err := rs.IsRevoked(ctx, "worker-1")
	if err != nil {
		t.Fatalf("IsRevoked via interface: %v", err)
	}
	if !revoked {
		t.Error("revoked worker reported as not revoked via interface")
	}
}
