package localfs

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/acumen-ai-org/robotdreams/pkg/storage"
	"github.com/acumen-ai-org/robotdreams/pkg/storage/testsuite"
)

// TestConformance runs the shared storage.StorageBackend conformance
// suite against localfs.Backend, each subtest rooted at a fresh temp
// directory.
func TestConformance(t *testing.T) {
	testsuite.RunConformance(t, func() storage.StorageBackend {
		b, err := New(t.TempDir())
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		return b
	})
}

// TestPathTraversalRejected verifies that paths attempting to escape the
// backend's root (via ".." segments or absolute paths) are rejected and
// never cause reads or writes outside root.
func TestPathTraversalRejected(t *testing.T) {
	root := t.TempDir()
	b, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()
	ctx := context.Background()

	// A sibling directory to root, standing in for "outside the root".
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

	// The pre-existing outside file must be untouched.
	data, err := os.ReadFile(outsideFile)
	if err != nil {
		t.Fatalf("reading outside file: %v", err)
	}
	if string(data) != "pre-existing" {
		t.Fatalf("outside file was modified: got %q", data)
	}

	// Nothing should have been written outside root either.
	if _, err := os.Stat(filepath.Join(outsideDir, "..", "etc")); err == nil {
		t.Fatalf("traversal write escaped root: found unexpected path")
	}

	// root itself should contain nothing (all attempts were rejected
	// before any write occurred).
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("reading root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("root contains unexpected entries after rejected traversal attempts: %v", entries)
	}
}

// TestAbsolutePathRejected verifies that an absolute object path is
// rejected rather than silently reinterpreted relative to root.
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

// TestRevisionsHistory verifies that overwriting an object accumulates
// revision history, bounded by maxRevisionHistory.
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

// TestHealth verifies Health succeeds against a normal, writable root.
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
