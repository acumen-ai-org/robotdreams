package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/orgchart"
	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock { return &fakeClock{now: start} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir(), security.RealClock{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
}

func TestWorkerRoundtrip(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	connectedAt := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	want := orgchart.Worker{
		ID:          "worker-a",
		Role:        "contributor",
		ReportsTo:   "lead-1",
		PublicKey:   []byte{1, 2, 3, 4},
		ConnectedAt: connectedAt,
		LastSeenAt:  connectedAt.Add(time.Minute),
		Status:      orgchart.StatusConnected,
		Metadata:    map[string]string{"region": "eu", "kind": "pod"},
	}

	if err := s.UpsertWorker(ctx, want); err != nil {
		t.Fatalf("UpsertWorker: %v", err)
	}

	got, err := s.GetWorker(ctx, "worker-a")
	if err != nil {
		t.Fatalf("GetWorker: %v", err)
	}
	assertWorkerEqual(t, want, got)
}

func TestWorkerUpsertReplaces(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	base := orgchart.Worker{ID: "w", Role: "contributor", Status: orgchart.StatusConnected}
	if err := s.UpsertWorker(ctx, base); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	base.Role = "lead"
	base.ReportsTo = ""
	base.Status = orgchart.StatusDegraded
	base.Metadata = map[string]string{"note": "promoted"}
	if err := s.UpsertWorker(ctx, base); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := s.GetWorker(ctx, "w")
	if err != nil {
		t.Fatalf("GetWorker: %v", err)
	}
	if got.Role != "lead" || got.Status != orgchart.StatusDegraded {
		t.Fatalf("upsert did not replace: got role=%q status=%q", got.Role, got.Status)
	}
	if got.Metadata["note"] != "promoted" {
		t.Fatalf("metadata not replaced: %v", got.Metadata)
	}

	all, err := s.ListWorkers(ctx)
	if err != nil {
		t.Fatalf("ListWorkers: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("upsert inserted a duplicate row: got %d workers", len(all))
	}
}

func TestWorkerZeroTimesAndNilFields(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	want := orgchart.Worker{ID: "declared", Role: "lead"}
	if err := s.UpsertWorker(ctx, want); err != nil {
		t.Fatalf("UpsertWorker: %v", err)
	}

	got, err := s.GetWorker(ctx, "declared")
	if err != nil {
		t.Fatalf("GetWorker: %v", err)
	}
	if !got.ConnectedAt.IsZero() || !got.LastSeenAt.IsZero() {
		t.Fatalf("zero times did not survive: %v / %v", got.ConnectedAt, got.LastSeenAt)
	}
	if got.Metadata != nil {
		t.Fatalf("nil metadata became %v", got.Metadata)
	}
	if len(got.PublicKey) != 0 {
		t.Fatalf("nil public key became %v", got.PublicKey)
	}
}

func TestListAndDeleteWorkers(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	for _, id := range []string{"c", "a", "b"} {
		if err := s.UpsertWorker(ctx, orgchart.Worker{ID: id, Status: orgchart.StatusConnected}); err != nil {
			t.Fatalf("UpsertWorker(%q): %v", id, err)
		}
	}

	list, err := s.ListWorkers(ctx)
	if err != nil {
		t.Fatalf("ListWorkers: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("want 3 workers, got %d", len(list))
	}
	for i, id := range []string{"a", "b", "c"} {
		if list[i].ID != id {
			t.Fatalf("ListWorkers not sorted by ID: got %q at %d, want %q", list[i].ID, i, id)
		}
	}

	if err := s.DeleteWorker(ctx, "b"); err != nil {
		t.Fatalf("DeleteWorker: %v", err)
	}
	if _, err := s.GetWorker(ctx, "b"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}

	if err := s.DeleteWorker(ctx, "nope"); err != nil {
		t.Fatalf("DeleteWorker(absent): %v", err)
	}

	list, err = s.ListWorkers(ctx)
	if err != nil {
		t.Fatalf("ListWorkers: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 workers after delete, got %d", len(list))
	}
}

func TestGetWorkerNotFound(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.GetWorker(context.Background(), "ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUpsertWorkerRejectsEmptyID(t *testing.T) {
	s := openTestStore(t)
	if err := s.UpsertWorker(context.Background(), orgchart.Worker{}); err == nil {
		t.Fatal("want an error for an empty worker ID, got nil")
	}
}

func TestRevocationDelegate(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	rev := s.Revocations()

	revoked, err := rev.IsRevoked(ctx, "w1")
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
	}
	if revoked {
		t.Fatal("fresh store reports w1 as revoked")
	}

	if err := rev.Revoke(ctx, "w1", "key leaked"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	revoked, err = rev.IsRevoked(ctx, "w1")
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
	}
	if !revoked {
		t.Fatal("w1 not revoked after Revoke")
	}

	records, err := rev.ListRevoked(ctx)
	if err != nil {
		t.Fatalf("ListRevoked: %v", err)
	}
	if len(records) != 1 || records[0].WorkerID != "w1" || records[0].Reason != "key leaked" {
		t.Fatalf("unexpected revocation records: %+v", records)
	}

	if err := rev.Unrevoke(ctx, "w1"); err != nil {
		t.Fatalf("Unrevoke: %v", err)
	}
	revoked, err = rev.IsRevoked(ctx, "w1")
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
	}
	if revoked {
		t.Fatal("w1 still revoked after Unrevoke")
	}
}

func TestRevocationSharesOneDatabase(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s, err := Open(dir, security.RealClock{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s.UpsertWorker(ctx, orgchart.Worker{ID: "w1"}); err != nil {
		t.Fatalf("UpsertWorker: %v", err)
	}
	if err := s.Revocations().Revoke(ctx, "w1", "test"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.db"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(files) != 1 || filepath.Base(files[0]) != DBFileName {
		t.Fatalf("want exactly %s in the data dir, got %v", DBFileName, files)
	}

	s2, err := Open(dir, security.RealClock{})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	if _, err := s2.GetWorker(ctx, "w1"); err != nil {
		t.Fatalf("worker did not persist: %v", err)
	}
	revoked, err := s2.Revocations().IsRevoked(ctx, "w1")
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
	}
	if !revoked {
		t.Fatal("revocation did not persist")
	}
}

func TestConfigRoundtrip(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC))
	s, err := Open(t.TempDir(), clock)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	if _, _, err := s.GetConfig(ctx); !errors.Is(err, ErrNoConfig) {
		t.Fatalf("want ErrNoConfig before SetConfig, got %v", err)
	}
	if _, err := s.ConfigUpdatedAt(ctx); !errors.Is(err, ErrNoConfig) {
		t.Fatalf("want ErrNoConfig from ConfigUpdatedAt, got %v", err)
	}

	if err := s.SetConfig(ctx, "queue://sqlite/tmp/m.db", "file:///tmp/objects"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	msg, stg, err := s.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if msg != "queue://sqlite/tmp/m.db" || stg != "file:///tmp/objects" {
		t.Fatalf("unexpected config: %q / %q", msg, stg)
	}

	updatedAt, err := s.ConfigUpdatedAt(ctx)
	if err != nil {
		t.Fatalf("ConfigUpdatedAt: %v", err)
	}
	if !updatedAt.Equal(clock.Now()) {
		t.Fatalf("ConfigUpdatedAt = %v, want %v", updatedAt, clock.Now())
	}

	clock.Advance(time.Hour)
	if err := s.SetConfig(ctx, "queue://sqlite/other.db", "file:///other"); err != nil {
		t.Fatalf("SetConfig(replace): %v", err)
	}
	msg, stg, err = s.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if msg != "queue://sqlite/other.db" || stg != "file:///other" {
		t.Fatalf("config not replaced: %q / %q", msg, stg)
	}
	updatedAt, err = s.ConfigUpdatedAt(ctx)
	if err != nil {
		t.Fatalf("ConfigUpdatedAt: %v", err)
	}
	if !updatedAt.Equal(clock.Now()) {
		t.Fatalf("updated_at not refreshed: %v", updatedAt)
	}
}

func TestOpenURI(t *testing.T) {
	dir := t.TempDir()

	t.Run("sqlite scheme", func(t *testing.T) {
		s, err := OpenURI("sqlite://"+filepath.Join(dir, "a", "ctl.db"), nil)
		if err != nil {
			t.Fatalf("OpenURI: %v", err)
		}
		defer s.Close()
		if err := s.Health(context.Background()); err != nil {
			t.Fatalf("Health: %v", err)
		}
	})

	t.Run("bare path", func(t *testing.T) {
		s, err := OpenURI(filepath.Join(dir, "b", "ctl.db"), nil)
		if err != nil {
			t.Fatalf("OpenURI: %v", err)
		}
		defer s.Close()
		if err := s.UpsertWorker(context.Background(), orgchart.Worker{ID: "x"}); err != nil {
			t.Fatalf("UpsertWorker: %v", err)
		}
	})

	t.Run("postgres stub", func(t *testing.T) {
		_, err := OpenURI("postgres://user@host/dream", nil)
		if !errors.Is(err, ErrUnsupportedBackend) {
			t.Fatalf("want ErrUnsupportedBackend, got %v", err)
		}
	})

	t.Run("unknown scheme", func(t *testing.T) {
		if _, err := OpenURI("mysql://host/db", nil); !errors.Is(err, ErrUnsupportedBackend) {
			t.Fatalf("want ErrUnsupportedBackend, got %v", err)
		}
	})
}

func TestOpenRejectsEmptyDataDir(t *testing.T) {
	if _, err := Open("", nil); err == nil {
		t.Fatal("want an error for an empty data dir, got nil")
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	const goroutines = 8
	const iterations = 25

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*4)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			id := string(rune('a' + g))
			for i := 0; i < iterations; i++ {
				w := orgchart.Worker{
					ID:          id,
					Role:        "contributor",
					Status:      orgchart.StatusConnected,
					LastSeenAt:  time.Unix(int64(i), 0).UTC(),
					PublicKey:   []byte{byte(g), byte(i)},
					Metadata:    map[string]string{"i": id},
					ConnectedAt: time.Unix(1, 0).UTC(),
				}
				if err := s.UpsertWorker(ctx, w); err != nil {
					errCh <- err
					return
				}
				if _, err := s.GetWorker(ctx, id); err != nil {
					errCh <- err
					return
				}
				if _, err := s.ListWorkers(ctx); err != nil {
					errCh <- err
					return
				}
				if err := s.Revocations().Revoke(ctx, id, "churn"); err != nil {
					errCh <- err
					return
				}
				if _, err := s.Revocations().IsRevoked(ctx, id); err != nil {
					errCh <- err
					return
				}
				if err := s.SetConfig(ctx, "queue://sqlite/m.db", "file:///s"); err != nil {
					errCh <- err
					return
				}
			}
		}(g)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent access: %v", err)
	}

	list, err := s.ListWorkers(ctx)
	if err != nil {
		t.Fatalf("ListWorkers: %v", err)
	}
	if len(list) != goroutines {
		t.Fatalf("want %d workers, got %d", goroutines, len(list))
	}
}

func assertWorkerEqual(t *testing.T, want, got orgchart.Worker) {
	t.Helper()
	if got.ID != want.ID || got.Role != want.Role || got.ReportsTo != want.ReportsTo {
		t.Fatalf("identity mismatch: got %+v, want %+v", got, want)
	}
	if got.Status != want.Status {
		t.Fatalf("status = %q, want %q", got.Status, want.Status)
	}
	if string(got.PublicKey) != string(want.PublicKey) {
		t.Fatalf("public key = %v, want %v", got.PublicKey, want.PublicKey)
	}
	if !got.ConnectedAt.Equal(want.ConnectedAt) {
		t.Fatalf("connected_at = %v, want %v", got.ConnectedAt, want.ConnectedAt)
	}
	if !got.LastSeenAt.Equal(want.LastSeenAt) {
		t.Fatalf("last_seen_at = %v, want %v", got.LastSeenAt, want.LastSeenAt)
	}
	if len(got.Metadata) != len(want.Metadata) {
		t.Fatalf("metadata = %v, want %v", got.Metadata, want.Metadata)
	}
	for k, v := range want.Metadata {
		if got.Metadata[k] != v {
			t.Fatalf("metadata[%q] = %q, want %q", k, got.Metadata[k], v)
		}
	}
}
