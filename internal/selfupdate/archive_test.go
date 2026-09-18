package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func tarGzWith(t *testing.T, entries []tar.Header, payloads [][]byte) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for i, hdr := range entries {
		h := hdr
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(payloads[i]))
		}
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write(payloads[i]); err != nil {
				t.Fatalf("write payload: %v", err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}

	path := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	return path
}

func TestExtractBinaryFromTarGz(t *testing.T) {
	want := []byte("#!/bin/sh\necho dream\n")
	archive := tarGzWith(t,
		[]tar.Header{
			{Name: "README.md", Typeflag: tar.TypeReg, Mode: 0o644},
			{Name: "dream", Typeflag: tar.TypeReg, Mode: 0o755},
		},
		[][]byte{[]byte("docs"), want},
	)

	dest := filepath.Join(t.TempDir(), "dream")
	if err := ExtractBinary(archive, "dream", dest); err != nil {
		t.Fatalf("ExtractBinary: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read extracted: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("extracted %q, want %q", got, want)
	}

	if runtime.GOOS != "windows" {
		fi, err := os.Stat(dest)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if fi.Mode().Perm()&0o111 == 0 {
			t.Fatalf("extracted binary is not executable (mode %v)", fi.Mode().Perm())
		}
	}
}

func TestExtractBinaryFromZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("dream.exe")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	want := []byte("MZ fake windows binary")
	if _, err := w.Write(want); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	archive := filepath.Join(t.TempDir(), "archive.zip")
	if err := os.WriteFile(archive, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "dream.exe")
	if err := ExtractBinary(archive, "dream.exe", dest); err != nil {
		t.Fatalf("ExtractBinary: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read extracted: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("extracted %q, want %q", got, want)
	}
}

func TestExtractBinaryRejectsHostileEntries(t *testing.T) {
	tests := []struct {
		name    string
		entries []tar.Header
		payload []byte
	}{
		{
			name:    "binary absent",
			entries: []tar.Header{{Name: "README.md", Typeflag: tar.TypeReg, Mode: 0o644}},
			payload: []byte("docs"),
		},
		{
			name:    "nested path is not the root binary",
			entries: []tar.Header{{Name: "sub/dream", Typeflag: tar.TypeReg, Mode: 0o755}},
			payload: []byte("nope"),
		},
		{
			name:    "path traversal",
			entries: []tar.Header{{Name: "../dream", Typeflag: tar.TypeReg, Mode: 0o755}},
			payload: []byte("nope"),
		},
		{
			name:    "symlink named dream",
			entries: []tar.Header{{Name: "dream", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0o777}},
			payload: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			archive := tarGzWith(t, tc.entries, [][]byte{tc.payload})
			dest := filepath.Join(t.TempDir(), "dream")
			if err := ExtractBinary(archive, "dream", dest); err == nil {
				t.Fatal("extraction succeeded, want an error")
			}
			if _, err := os.Stat(dest); err == nil {
				t.Fatal("a destination file was written despite the failure")
			}
		})
	}
}

func TestExtractBinaryRejectsCorruptGzip(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := os.WriteFile(archive, []byte("this is not gzip"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := ExtractBinary(archive, "dream", filepath.Join(t.TempDir(), "dream")); err == nil {
		t.Fatal("extracting a corrupt archive succeeded, want an error")
	}
}
