// Package testsuite is the conformance suite every storage.StorageBackend implementation runs.
package testsuite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/acumen-ai-org/robotdreams/pkg/storage"
)

// RunConformance runs the conformance subtests, each against a fresh backend from factory.
func RunConformance(t *testing.T, factory func() storage.StorageBackend) {
	t.Helper()

	t.Run("put-then-get-roundtrip", func(t *testing.T) {
		backend := factory()
		defer backend.Close()
		ctx := context.Background()

		content := []byte("hello, robot dreams")
		if _, err := backend.Put(ctx, "a/b", bytes.NewReader(content), storage.PutOptions{}); err != nil {
			t.Fatalf("Put: %v", err)
		}

		rc, _, err := backend.Get(ctx, "a/b")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		defer rc.Close()

		got, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("reading body: %v", err)
		}
		if !bytes.Equal(got, content) {
			t.Fatalf("got %q, want %q", got, content)
		}
	})

	t.Run("get-missing-returns-not-found", func(t *testing.T) {
		backend := factory()
		defer backend.Close()
		ctx := context.Background()

		_, _, err := backend.Get(ctx, "does/not/exist")
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("Get on missing path: got err=%v, want ErrNotFound", err)
		}
	})

	t.Run("stat-matches-put", func(t *testing.T) {
		backend := factory()
		defer backend.Close()
		ctx := context.Background()

		content := []byte("stat me")
		putMeta, err := backend.Put(ctx, "stat/me", bytes.NewReader(content), storage.PutOptions{})
		if err != nil {
			t.Fatalf("Put: %v", err)
		}

		statMeta, err := backend.Stat(ctx, "stat/me")
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}

		if statMeta.Path != putMeta.Path {
			t.Errorf("Path: got %q, want %q", statMeta.Path, putMeta.Path)
		}
		if statMeta.Revision != putMeta.Revision {
			t.Errorf("Revision: got %q, want %q", statMeta.Revision, putMeta.Revision)
		}
		if statMeta.Size != putMeta.Size {
			t.Errorf("Size: got %d, want %d", statMeta.Size, putMeta.Size)
		}
		if statMeta.Size != int64(len(content)) {
			t.Errorf("Size: got %d, want %d (len of content)", statMeta.Size, len(content))
		}
	})

	t.Run("put-records-updated-by", func(t *testing.T) {
		backend := factory()
		defer backend.Close()
		ctx := context.Background()

		putMeta, err := backend.Put(ctx, "audit/who.txt", bytes.NewReader([]byte("who wrote this")), storage.PutOptions{UpdatedBy: "worker-a"})
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		if putMeta.UpdatedBy != "worker-a" {
			t.Errorf("Put: UpdatedBy = %q, want %q", putMeta.UpdatedBy, "worker-a")
		}

		statMeta, err := backend.Stat(ctx, "audit/who.txt")
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if statMeta.UpdatedBy != "worker-a" {
			t.Errorf("Stat: UpdatedBy = %q, want %q", statMeta.UpdatedBy, "worker-a")
		}

		rc, getMeta, err := backend.Get(ctx, "audit/who.txt")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		rc.Close()
		if getMeta.UpdatedBy != "worker-a" {
			t.Errorf("Get: UpdatedBy = %q, want %q", getMeta.UpdatedBy, "worker-a")
		}

		revs, err := backend.Revisions(ctx, "audit/who.txt")
		if err != nil {
			t.Fatalf("Revisions: %v", err)
		}
		if len(revs) == 0 {
			t.Fatal("Revisions: no entries for a path just written")
		}
		if latest := revs[len(revs)-1]; latest.UpdatedBy != "worker-a" {
			t.Errorf("Revisions: latest entry UpdatedBy = %q, want %q", latest.UpdatedBy, "worker-a")
		}
	})

	t.Run("list-with-prefix", func(t *testing.T) {
		backend := factory()
		defer backend.Close()
		ctx := context.Background()

		paths := []string{"foo/1", "foo/2", "bar/1"}
		for _, p := range paths {
			if _, err := backend.Put(ctx, p, bytes.NewReader([]byte(p)), storage.PutOptions{}); err != nil {
				t.Fatalf("Put(%q): %v", p, err)
			}
		}

		got, err := backend.List(ctx, "foo/")
		if err != nil {
			t.Fatalf("List: %v", err)
		}

		gotPaths := map[string]bool{}
		for _, m := range got {
			gotPaths[m.Path] = true
		}
		if len(gotPaths) != 2 || !gotPaths["foo/1"] || !gotPaths["foo/2"] {
			t.Fatalf("List(%q) = %v, want exactly foo/1 and foo/2", "foo/", got)
		}
		if gotPaths["bar/1"] {
			t.Fatalf("List(%q) unexpectedly included bar/1", "foo/")
		}
	})

	t.Run("unconditional-put-overwrites", func(t *testing.T) {
		backend := factory()
		defer backend.Close()
		ctx := context.Background()

		first, err := backend.Put(ctx, "over/write", bytes.NewReader([]byte("v1")), storage.PutOptions{})
		if err != nil {
			t.Fatalf("Put v1: %v", err)
		}

		second, err := backend.Put(ctx, "over/write", bytes.NewReader([]byte("v2 longer")), storage.PutOptions{})
		if err != nil {
			t.Fatalf("Put v2: %v", err)
		}

		if second.Revision == first.Revision {
			t.Fatalf("revision did not change across overwrite: %q", second.Revision)
		}

		rc, _, err := backend.Get(ctx, "over/write")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		defer rc.Close()
		got, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("reading body: %v", err)
		}
		if string(got) != "v2 longer" {
			t.Fatalf("got %q, want %q", got, "v2 longer")
		}
	})

	t.Run("if-match-correct-revision-succeeds", func(t *testing.T) {
		backend := factory()
		defer backend.Close()
		ctx := context.Background()

		first, err := backend.Put(ctx, "cond/ok", bytes.NewReader([]byte("v1")), storage.PutOptions{})
		if err != nil {
			t.Fatalf("Put v1: %v", err)
		}

		_, err = backend.Put(ctx, "cond/ok", bytes.NewReader([]byte("v2")), storage.PutOptions{
			IfMatchRevision: first.Revision,
		})
		if err != nil {
			t.Fatalf("Put with correct IfMatchRevision: %v", err)
		}
	})

	t.Run("if-match-stale-revision-fails-without-mutating", func(t *testing.T) {
		backend := factory()
		defer backend.Close()
		ctx := context.Background()

		if _, err := backend.Put(ctx, "cond/stale", bytes.NewReader([]byte("v1")), storage.PutOptions{}); err != nil {
			t.Fatalf("Put v1: %v", err)
		}

		_, err := backend.Put(ctx, "cond/stale", bytes.NewReader([]byte("v2")), storage.PutOptions{
			IfMatchRevision: "definitely-not-the-current-revision",
		})
		if !errors.Is(err, storage.ErrRevisionMismatch) {
			t.Fatalf("Put with stale IfMatchRevision: got err=%v, want ErrRevisionMismatch", err)
		}

		rc, meta, err := backend.Get(ctx, "cond/stale")
		if err != nil {
			t.Fatalf("Get after failed conditional put: %v", err)
		}
		defer rc.Close()
		got, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("reading body: %v", err)
		}
		if string(got) != "v1" {
			t.Fatalf("object was mutated by failed conditional put: got %q, want %q", got, "v1")
		}

		statMeta, err := backend.Stat(ctx, "cond/stale")
		if err != nil {
			t.Fatalf("Stat after failed conditional put: %v", err)
		}
		if statMeta.Revision != meta.Revision {
			t.Fatalf("Stat revision %q != Get revision %q after failed conditional put", statMeta.Revision, meta.Revision)
		}
	})

	t.Run("concurrent-conditional-put-exactly-one-wins", func(t *testing.T) {
		backend := factory()
		defer backend.Close()
		ctx := context.Background()

		seed, err := backend.Put(ctx, "race/obj", bytes.NewReader([]byte("seed")), storage.PutOptions{})
		if err != nil {
			t.Fatalf("seed Put: %v", err)
		}

		const n = 8
		var wg sync.WaitGroup
		type result struct {
			idx     int
			content string
			err     error
		}
		results := make([]result, n)

		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				content := fmt.Sprintf("writer-%d", i)
				_, err := backend.Put(ctx, "race/obj", bytes.NewReader([]byte(content)), storage.PutOptions{
					IfMatchRevision: seed.Revision,
				})
				results[i] = result{idx: i, content: content, err: err}
			}(i)
		}
		wg.Wait()

		var winners []result
		for _, r := range results {
			if r.err == nil {
				winners = append(winners, r)
			} else if !errors.Is(r.err, storage.ErrRevisionMismatch) {
				t.Fatalf("writer %d: unexpected error %v", r.idx, r.err)
			}
		}

		if len(winners) != 1 {
			t.Fatalf("expected exactly 1 winner, got %d: %+v", len(winners), winners)
		}

		rc, _, err := backend.Get(ctx, "race/obj")
		if err != nil {
			t.Fatalf("Get after race: %v", err)
		}
		defer rc.Close()
		got, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("reading body: %v", err)
		}
		if string(got) != winners[0].content {
			t.Fatalf("stored content %q does not match winner's content %q", got, winners[0].content)
		}
	})

	t.Run("delete-then-get-not-found", func(t *testing.T) {
		backend := factory()
		defer backend.Close()
		ctx := context.Background()

		if _, err := backend.Put(ctx, "del/me", bytes.NewReader([]byte("bye")), storage.PutOptions{}); err != nil {
			t.Fatalf("Put: %v", err)
		}
		if err := backend.Delete(ctx, "del/me"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, _, err := backend.Get(ctx, "del/me"); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("Get after Delete: got err=%v, want ErrNotFound", err)
		}
	})

	t.Run("health-succeeds", func(t *testing.T) {
		backend := factory()
		defer backend.Close()
		ctx := context.Background()

		if err := backend.Health(ctx); err != nil {
			t.Fatalf("Health: %v", err)
		}
	})
}
