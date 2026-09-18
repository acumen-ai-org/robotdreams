package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/spf13/cobra"
)

func TestMain(m *testing.M) {
	os.Unsetenv(envDreamURL)
	os.Unsetenv(envDreamToken)
	os.Exit(m.Run())
}

func findCommand(t *testing.T, root *cobra.Command, path ...string) *cobra.Command {
	t.Helper()
	cur := root
	for _, name := range path {
		var next *cobra.Command
		for _, c := range cur.Commands() {
			if c.Name() == name {
				next = c
				break
			}
		}
		if next == nil {
			t.Fatalf("command %q has no subcommand %q", cur.CommandPath(), name)
		}
		cur = next
	}
	return cur
}

func TestRootCommandTree(t *testing.T) {
	root := newRootCmd()

	want := [][]string{
		{"version"},
		{"server"},
		{"server", "init"},
		{"server", "revoke"},
		{"server", "guide"},
		{"worker"},
		{"worker", "connect"},
		{"worker", "reassign"},
		{"worker", "list"},
		{"worker", "onboard"},
		{"worker", "app"},
		{"worker", "app", "set"},
		{"worker", "app", "clear"},
		{"worker", "app", "list"},
		{"message"},
		{"message", "send"},
		{"message", "tail"},
		{"message", "ack"},
		{"storage"},
		{"storage", "put"},
		{"storage", "get"},
		{"storage", "ls"},
		{"self-update"},
		{"updates"},
		{"updates", "announce"},
		{"updates", "list"},
		{"updates", "pending"},
		{"updates", "report"},
		{"updates", "rollout"},
		{"updates", "contract"},
	}
	for _, path := range want {
		cmd := findCommand(t, root, path...)
		if cmd.Short == "" {
			t.Errorf("%s has no Short description", cmd.CommandPath())
		}
	}
}

func TestHelpDoesNotPanic(t *testing.T) {
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		if err := cmd.Help(); err != nil {
			t.Fatalf("%s: help: %v", cmd.CommandPath(), err)
		}
		if buf.Len() == 0 {
			t.Fatalf("%s: help produced no output", cmd.CommandPath())
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(newRootCmd())
}

func TestVersionCmd(t *testing.T) {
	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := buf.String(); got != "dream version "+version+"\n" {
		t.Fatalf("version output %q, want exactly %q", got, "dream version "+version+"\n")
	}
}

func TestFlagWiring(t *testing.T) {
	root := newRootCmd()

	cases := []struct {
		path  []string
		flags []string
	}{
		{[]string{"server", "init"}, []string{"data-dir", "messaging", "storage", "orgchart", "addr"}},
		{[]string{"server", "revoke"}, []string{"data-dir", "server", "reason"}},
		{[]string{"server", "guide"}, []string{"data-dir", "server"}},
		{[]string{"worker", "connect"}, []string{"server", "worker-id", "role", "reports-to", "metadata", "admin-token"}},
		{[]string{"worker", "onboard"}, []string{"server"}},
		{[]string{"worker", "reassign"}, []string{"reports-to", "server", "server-id", "worker-id"}},
		{[]string{"worker", "list"}, []string{"server", "server-id", "worker-id"}},
		{[]string{"message", "send"}, []string{"type", "to", "subject", "body", "storage-path", "storage-revision", "server", "worker-id"}},
		{[]string{"message", "tail"}, []string{"worker", "follow", "server", "worker-id"}},
		{[]string{"storage", "put"}, []string{"if-match-revision", "server", "worker-id"}},
		{[]string{"storage", "get"}, []string{"server", "server-id", "worker-id"}},
		{[]string{"storage", "ls"}, []string{"server", "server-id", "worker-id"}},
	}

	for _, tc := range cases {
		cmd := findCommand(t, root, tc.path...)
		for _, name := range tc.flags {
			if cmd.Flags().Lookup(name) == nil {
				t.Errorf("%s: missing --%s", cmd.CommandPath(), name)
			}
		}
	}
}

func TestRequiredFlags(t *testing.T) {
	t.Setenv(envDreamURL, "")
	t.Setenv(envDreamToken, "")
	cases := [][]string{
		{"worker", "connect"},
		{"message", "send"},
	}
	for _, args := range cases {
		root := newRootCmd()
		var buf bytes.Buffer
		root.SetOut(&buf)
		root.SetErr(&buf)
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Errorf("%v: expected an error for missing required flags", args)
		}
	}
}

func TestArgCounts(t *testing.T) {
	root := newRootCmd()

	if err := findCommand(t, root, "storage", "put").Args(nil, []string{"only-one"}); err == nil {
		t.Error("storage put: expected an error with one positional arg")
	}
	if err := findCommand(t, root, "storage", "get").Args(nil, []string{"a", "b"}); err != nil {
		t.Errorf("storage get: unexpected error with two args: %v", err)
	}
	if err := findCommand(t, root, "storage", "ls").Args(nil, nil); err != nil {
		t.Errorf("storage ls: unexpected error with no args: %v", err)
	}
	if err := findCommand(t, root, "server", "revoke").Args(nil, nil); err == nil {
		t.Error("server revoke: expected an error with no worker id")
	}
	if err := findCommand(t, root, "worker", "reassign").Args(nil, []string{"w1"}); err != nil {
		t.Errorf("worker reassign: unexpected error with one arg: %v", err)
	}
}
