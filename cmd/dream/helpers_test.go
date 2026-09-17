package main

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func runDreamCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func mustRunDreamCmd(t *testing.T, args ...string) string {
	t.Helper()
	out, err := runDreamCmd(t, args...)
	if err != nil {
		t.Fatalf("dream %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func waitForOutput(t *testing.T, b *syncBuffer, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(b.String(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("output never contained %q within %s; got:\n%s", want, timeout, b.String())
}

func waitForErr(t *testing.T, ch <-chan error, timeout time.Duration) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(timeout):
		t.Fatalf("command did not return within %s", timeout)
		return nil
	}
}

func connectPair(t *testing.T, addr string) {
	t.Helper()
	ctx := context.Background()
	for _, w := range []workerConnectOptions{
		{Server: addr, WorkerID: "lead", Role: "lead"},
		{Server: addr, WorkerID: "leaf", Role: "worker", ReportsTo: "lead"},
	} {
		if _, err := runWorkerConnect(ctx, w); err != nil {
			t.Fatalf("connect %s: %v", w.WorkerID, err)
		}
	}
}
