package localfs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/acumen-ai-org/robotdreams/pkg/storage"
	"github.com/acumen-ai-org/robotdreams/pkg/storage/testsuite"
)

func TestConformance(t *testing.T) {
	testsuite.RunConformance(t, func() storage.StorageBackend {
		b, err := New(t.TempDir())
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		return b
	})
}

func TestPathTraversalRejected(t *testing.T) {
	root := t.TempDir()
	b, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()
	ctx := context.Background()

	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret")
	if err := os.WriteFile(outsideFile, []byte("pre-existing"), 0o644); err != nil {
		t.Fatalf("seeding outside file: %v", err)
	}

	traversalPaths := []string{
		"../../etc/passwd",
		"../secret",
		"../../../../../../etc/passwd",
	}

	for _, p := range traversalPaths {
		t.Run("put_"+p, func(t *testing.T) {
			_, err := b.Put(ctx, p, bytes.NewReader([]byte("malicious")), storage.PutOptions{})
			if err == nil {
				t.Fatalf("Put(%q) unexpectedly succeeded", p)
			}
		})
		t.Run("get_"+p, func(t *testing.T) {
			_, _, err := b.Get(ctx, p)
			if err == nil {
				t.Fatalf("Get(%q) unexpectedly succeeded", p)
			}
		})
	}

	data, err := os.ReadFile(outsideFile)
	if err != nil {
		t.Fatalf("reading outside file: %v", err)
	}
	if string(data) != "pre-existing" {
		t.Fatalf("outside file was modified: got %q", data)
	}

	if _, err := os.Stat(filepath.Join(outsideDir, "..", "etc")); err == nil {
		t.Fatalf("traversal write escaped root: found unexpected path")
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("reading root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("root contains unexpected entries after rejected traversal attempts: %v", entries)
	}
}

func TestAbsolutePathRejected(t *testing.T) {
	root := t.TempDir()
	b, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()
	ctx := context.Background()

	_, err = b.Put(ctx, "/etc/passwd", bytes.NewReader([]byte("x")), storage.PutOptions{})
	if err == nil {
		t.Fatalf("Put with absolute path unexpectedly succeeded")
	}
}

func TestRevisionsHistory(t *testing.T) {
	root := t.TempDir()
	b, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()
	ctx := context.Background()

	const writes = maxRevisionHistory + 5
	for i := 0; i < writes; i++ {
		if _, err := b.Put(ctx, "obj", bytes.NewReader([]byte{byte(i)}), storage.PutOptions{}); err != nil {
			t.Fatalf("Put #%d: %v", i, err)
		}
	}

	revs, err := b.Revisions(ctx, "obj")
	if err != nil {
		t.Fatalf("Revisions: %v", err)
	}
	if len(revs) != maxRevisionHistory {
		t.Fatalf("got %d revisions, want %d (bounded history)", len(revs), maxRevisionHistory)
	}
}

func TestHealth(t *testing.T) {
	b, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	if err := b.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
}

func TestReservedMetaPathRejected(t *testing.T) {
	b, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()
	ctx := context.Background()

	for _, p := range []string{metaDirName, metaDirName + "/forged.json", "./" + metaDirName + "/x"} {
		if _, err := b.Put(ctx, p, bytes.NewReader([]byte("x")), storage.PutOptions{}); err == nil {
			t.Fatalf("Put %q: want rejection of the reserved %q namespace", p, metaDirName)
		}
		if _, _, err := b.Get(ctx, p); err == nil {
			t.Fatalf("Get %q: want rejection of the reserved %q namespace", p, metaDirName)
		}
	}
}

func TestPathSpellingsShareRevisionHistory(t *testing.T) {
	b, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()
	ctx := context.Background()

	first, err := b.Put(ctx, "victim.txt", bytes.NewReader([]byte("one")), storage.PutOptions{})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := b.Put(ctx, "./victim.txt", bytes.NewReader([]byte("two")), storage.PutOptions{}); err != nil {
		t.Fatalf("Put via alternate spelling: %v", err)
	}
	_, err = b.Put(ctx, "victim.txt", bytes.NewReader([]byte("three")), storage.PutOptions{IfMatchRevision: first.Revision})
	if !errors.Is(err, storage.ErrRevisionMismatch) {
		t.Fatalf("Put with stale IfMatchRevision after a write under another spelling: err = %v, want ErrRevisionMismatch", err)
	}
}
