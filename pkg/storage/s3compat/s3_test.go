package s3compat

import (
	"strings"
	"testing"
	"time"
)

func newTestBackend(prefix string) *Backend {
	return &Backend{bucket: "test-bucket", prefix: prefix}
}

func TestResolveKey(t *testing.T) {
	cases := []struct {
		name    string
		prefix  string
		path    string
		want    string
		wantErr bool
	}{
		{name: "no prefix", prefix: "", path: "a/b", want: "a/b"},
		{name: "with prefix", prefix: "root", path: "a/b", want: "root/a/b"},
		{name: "nested prefix", prefix: "root/sub", path: "a/b", want: "root/sub/a/b"},
		{name: "empty path rejected", prefix: "", path: "", wantErr: true},
		{name: "absolute path rejected", prefix: "", path: "/etc/passwd", wantErr: true},
		{name: "dotdot rejected", prefix: "", path: "../secret", wantErr: true},
		{name: "embedded dotdot rejected", prefix: "root", path: "a/../../secret", wantErr: true},
		{name: "empty segment rejected", prefix: "", path: "a//b", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newTestBackend(tc.prefix)
			got, err := b.resolveKey(tc.path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveKey(%q) with prefix %q: expected error, got key %q", tc.path, tc.prefix, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveKey(%q) with prefix %q: unexpected error: %v", tc.path, tc.prefix, err)
			}
			if got != tc.want {
				t.Fatalf("resolveKey(%q) with prefix %q = %q, want %q", tc.path, tc.prefix, got, tc.want)
			}
		})
	}
}

func TestResolveKeyManyTraversalVariants(t *testing.T) {
	b := newTestBackend("root")
	paths := []string{
		"../../etc/passwd",
		"../secret",
		"../../../../../../etc/passwd",
		"a/b/../../../c",
	}
	for _, p := range paths {
		if _, err := b.resolveKey(p); err == nil {
			t.Fatalf("resolveKey(%q) unexpectedly succeeded", p)
		}
	}
}

func TestPathFromKeyRoundTrip(t *testing.T) {
	cases := []struct {
		prefix string
		path   string
	}{
		{prefix: "", path: "a/b"},
		{prefix: "root", path: "a/b"},
		{prefix: "root/sub", path: "a/b/c"},
	}
	for _, tc := range cases {
		b := newTestBackend(tc.prefix)
		key, err := b.resolveKey(tc.path)
		if err != nil {
			t.Fatalf("resolveKey(%q): %v", tc.path, err)
		}
		got := b.pathFromKey(key)
		if got != tc.path {
			t.Fatalf("pathFromKey(resolveKey(%q)) = %q, want %q", tc.path, got, tc.path)
		}
	}
}

func TestMetaDirAndSidecarKeyNamespaced(t *testing.T) {
	noPrefix := newTestBackend("")
	withPrefix := newTestBackend("root")

	if got := noPrefix.metaDir(); got != metaDirName {
		t.Fatalf("metaDir() with no prefix = %q, want %q", got, metaDirName)
	}
	if got := withPrefix.metaDir(); got != "root/"+metaDirName {
		t.Fatalf("metaDir() with prefix = %q, want %q", got, "root/"+metaDirName)
	}

	key1, _ := noPrefix.resolveKey("a/b")
	key2, _ := withPrefix.resolveKey("a/b")
	if noPrefix.sidecarKey(key1) == withPrefix.sidecarKey(key2) {
		t.Fatalf("sidecar keys for different prefixes unexpectedly collided")
	}
	if !strings.HasPrefix(withPrefix.sidecarKey(key2), "root/"+metaDirName+"/") {
		t.Fatalf("sidecarKey(%q) = %q, want prefix %q", key2, withPrefix.sidecarKey(key2), "root/"+metaDirName+"/")
	}
}

func TestIsUnderMetaDir(t *testing.T) {
	b := newTestBackend("root")
	if !b.isUnderMetaDir("root/.meta/abc.json") {
		t.Fatalf("isUnderMetaDir: expected true for a sidecar key")
	}
	if b.isUnderMetaDir("root/data/abc.json") {
		t.Fatalf("isUnderMetaDir: expected false for a normal object key")
	}
	if b.isUnderMetaDir("root/.metadata/abc.json") {
		t.Fatalf("isUnderMetaDir: false positive on a key that only shares a prefix string with .meta")
	}
}

func TestTrimAndQuoteETag(t *testing.T) {
	quoted := `"abc123"`
	if got := trimETag(&quoted); got != "abc123" {
		t.Fatalf("trimETag(%q) = %q, want %q", quoted, got, "abc123")
	}
	if got := trimETag(nil); got != "" {
		t.Fatalf("trimETag(nil) = %q, want empty string", got)
	}
	if got := *quoteETag("abc123"); got != `"abc123"` {
		t.Fatalf("quoteETag(%q) = %q, want %q", "abc123", got, `"abc123"`)
	}
}

func TestMetaFromUserMetadataUsesRecordedTimestamp(t *testing.T) {
	b := newTestBackend("")
	meta := b.metaFromUserMetadata("a/b", "rev1", 5, map[string]string{
		metaKeyUpdatedBy: "worker-1",
		metaKeyUpdatedAt: "2026-01-02T03:04:05.000000000Z",
	}, nil)

	if meta.UpdatedBy != "worker-1" {
		t.Errorf("UpdatedBy = %q, want %q", meta.UpdatedBy, "worker-1")
	}
	if meta.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt should have been parsed from user metadata, got zero value")
	}
	if meta.Path != "a/b" || meta.Revision != "rev1" || meta.Size != 5 {
		t.Errorf("unexpected base fields: %+v", meta)
	}
}

func TestMetaFromUserMetadataFallsBackToLastModified(t *testing.T) {
	b := newTestBackend("")
	lm := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	meta := b.metaFromUserMetadata("a/b", "rev1", 5, nil, &lm)

	if !meta.UpdatedAt.Equal(lm) {
		t.Fatalf("UpdatedAt = %v, want fallback to LastModified %v", meta.UpdatedAt, lm)
	}
	if meta.UpdatedBy != "" {
		t.Fatalf("expected empty UpdatedBy with nil user metadata, got %+v", meta)
	}
}
