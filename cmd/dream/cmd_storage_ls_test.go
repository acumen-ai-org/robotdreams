package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPrintObjectTable(t *testing.T) {
	at := time.Date(2026, 9, 17, 8, 30, 0, 0, time.UTC)
	objects := []objectMetaView{
		{Path: "workers/solo/reports/day1.txt", Revision: "rev-aaaa", Size: 19, UpdatedAt: at, UpdatedBy: "solo"},
		{Path: "workers/solo/reports/empty.bin", Revision: "", Size: 0, UpdatedBy: ""},
		{Path: "workers/solo/reports/big.tar", Revision: "rev-bbbb", Size: 1 << 30, UpdatedAt: at.Add(time.Hour), UpdatedBy: "solo"},
	}

	var sb strings.Builder
	if err := printObjectTable(&sb, objects); err != nil {
		t.Fatalf("printObjectTable: %v", err)
	}
	out := sb.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want header + 3:\n%s", len(lines), out)
	}
	for _, col := range []string{"PATH", "SIZE", "REVISION", "UPDATED BY", "UPDATED AT"} {
		if !strings.Contains(lines[0], col) {
			t.Errorf("header %q lacks %q", lines[0], col)
		}
	}
	if !strings.HasPrefix(lines[1], "workers/solo/reports/day1.txt") || !strings.Contains(lines[1], "  19  ") ||
		!strings.Contains(lines[1], "rev-aaaa") || !strings.Contains(lines[1], formatTime(at)) {
		t.Errorf("row 1 = %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "workers/solo/reports/empty.bin") || !strings.Contains(lines[2], "  0  ") {
		t.Errorf("row 2 = %q", lines[2])
	}
	if fields := strings.Fields(lines[2]); len(fields) != 5 || fields[2] != "-" || fields[3] != "-" || fields[4] != "-" {
		t.Errorf("row 2 should show dashes for revision, updater and time: %q", lines[2])
	}
	if !strings.Contains(lines[3], "1073741824") || !strings.Contains(lines[3], "rev-bbbb") {
		t.Errorf("row 3 = %q", lines[3])
	}
	sizeCol := strings.Index(lines[0], "SIZE")
	for i, line := range lines[1:] {
		if len(line) <= sizeCol || line[sizeCol-1] != ' ' || line[sizeCol] == ' ' {
			t.Errorf("row %d is not aligned to the SIZE column at %d: %q", i+1, sizeCol, line)
		}
	}

	sb.Reset()
	if err := printObjectTable(&sb, nil); err != nil {
		t.Fatalf("printObjectTable(nil): %v", err)
	}
	if strings.TrimSpace(sb.String()) != "no objects" {
		t.Fatalf("empty table = %q", sb.String())
	}
}

func TestStorageLsCommandRendersRevisions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, _ := startTestServer(t)
	ctx := context.Background()

	if _, err := runWorkerConnect(ctx, workerConnectOptions{Server: addr, WorkerID: "solo", Role: "lead"}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	flags := storageTargetFlags{Server: addr, AsWorker: "solo"}
	first, err := runStoragePut(ctx, storagePutOptions{storageTargetFlags: flags, Path: "workers/solo/a.txt", LocalFile: "-"}, strings.NewReader("12345"))
	if err != nil {
		t.Fatalf("put a: %v", err)
	}
	second, err := runStoragePut(ctx, storagePutOptions{storageTargetFlags: flags, Path: "workers/solo/a.txt", LocalFile: "-"}, strings.NewReader("1234567"))
	if err != nil {
		t.Fatalf("put a again: %v", err)
	}
	if first.Revision == second.Revision {
		t.Fatalf("two puts minted the same revision %q", first.Revision)
	}
	other, err := runStoragePut(ctx, storagePutOptions{storageTargetFlags: flags, Path: "workers/solo/b/c.txt", LocalFile: "-"}, strings.NewReader("xy"))
	if err != nil {
		t.Fatalf("put c: %v", err)
	}

	out := mustRunDreamCmd(t, "storage", "ls", "workers/solo/", "--server", addr, "--worker-id", "solo")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("ls printed %d lines, want header + 2:\n%s", len(lines), out)
	}
	if strings.Contains(out, first.Revision) {
		t.Errorf("ls shows the superseded revision %s:\n%s", first.Revision, out)
	}
	for _, want := range []string{"workers/solo/a.txt", second.Revision, "  7  ", "workers/solo/b/c.txt", other.Revision, "  2  ", "solo"} {
		if !strings.Contains(out, want) {
			t.Errorf("ls missing %q:\n%s", want, out)
		}
	}

	out = mustRunDreamCmd(t, "storage", "ls", "workers/solo/nothing/", "--server", addr, "--worker-id", "solo")
	if strings.TrimSpace(out) != "no objects" {
		t.Fatalf("ls of an empty prefix = %q", out)
	}

	out, err = runDreamCmd(t, "storage", "ls", "workers/nobody/", "--server", addr, "--worker-id", "solo")
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("ls of another worker's prefix: err = %v, out = %q", err, out)
	}
}

func TestStorageLsPathAlwaysCarriesPrefix(t *testing.T) {
	if got := storageLsPath(""); got != "/api/storage/objects?prefix=" {
		t.Fatalf("empty prefix path = %q; the server needs prefix present to list everything", got)
	}
	if got := storageLsPath("workers/solo/"); got != "/api/storage/objects?prefix=workers%2Fsolo%2F" {
		t.Fatalf("prefix path = %q", got)
	}
}
