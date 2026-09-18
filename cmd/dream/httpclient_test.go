package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acumen-ai-org/robotdreams/internal/identity"
)

func TestServerIDFromAddr(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{"host and port", "127.0.0.1:7420", "127.0.0.1_7420"},
		{"dns name", "control.example.com:7420", "control.example.com_7420"},
		{"http scheme stripped", "http://127.0.0.1:7420", "127.0.0.1_7420"},
		{"https scheme stripped", "https://control.example.com", "control.example.com"},
		{"trailing slash trimmed", "http://127.0.0.1:7420/", "127.0.0.1_7420"},
		{"path characters sanitized", "example.com:7420/base", "example.com_7420_base"},
		{"surrounding space trimmed", "  127.0.0.1:7420  ", "127.0.0.1_7420"},
		{"ipv6 brackets sanitized", "[::1]:7420", "___1__7420"},
		{"port only", ":7420", "_7420"},
		{"empty", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := serverIDFromAddr(tc.addr); got != tc.want {
				t.Fatalf("serverIDFromAddr(%q) = %q, want %q", tc.addr, got, tc.want)
			}
		})
	}
}

func TestServerIDFromAddrIsStable(t *testing.T) {
	const addr = "control.example.com:7420"
	first := serverIDFromAddr(addr)
	for i := 0; i < 5; i++ {
		if got := serverIDFromAddr(addr); got != first {
			t.Fatalf("serverIDFromAddr is not deterministic: %q then %q", first, got)
		}
	}
}

func TestBaseURLFromAddr(t *testing.T) {
	tests := []struct{ addr, want string }{
		{"127.0.0.1:7420", "http://127.0.0.1:7420"},
		{"http://127.0.0.1:7420", "http://127.0.0.1:7420"},
		{"https://control.example.com", "https://control.example.com"},
		{"http://127.0.0.1:7420/", "http://127.0.0.1:7420"},
	}
	for _, tc := range tests {
		if got := baseURLFromAddr(tc.addr); got != tc.want {
			t.Errorf("baseURLFromAddr(%q) = %q, want %q", tc.addr, got, tc.want)
		}
	}
}

func TestLocalConfigRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const serverID = "127.0.0.1_7420"
	want := localConfig{
		ServerAddr: "127.0.0.1:7420",
		WorkerID:   "lead",
		Role:       "team-lead",
		ReportsTo:  "",
	}

	if err := saveLocalConfig(serverID, want); err != nil {
		t.Fatalf("saveLocalConfig: %v", err)
	}

	got, err := loadLocalConfig(serverID)
	if err != nil {
		t.Fatalf("loadLocalConfig: %v", err)
	}
	if got != want {
		t.Fatalf("round trip mismatch: got %+v, want %+v", got, want)
	}
}

func TestLocalConfigLivesBesideIdentityKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	const serverID = "control.example.com_7420"
	if err := saveLocalConfig(serverID, localConfig{ServerAddr: "control.example.com:7420", WorkerID: "w1"}); err != nil {
		t.Fatalf("saveLocalConfig: %v", err)
	}

	keyPath, err := identity.DefaultKeyPath(serverID)
	if err != nil {
		t.Fatalf("DefaultKeyPath: %v", err)
	}
	cfgPath, err := configPathFor(serverID)
	if err != nil {
		t.Fatalf("configPathFor: %v", err)
	}

	if filepath.Dir(keyPath) != filepath.Dir(cfgPath) {
		t.Fatalf("identity key dir %q != config dir %q", filepath.Dir(keyPath), filepath.Dir(cfgPath))
	}
	if want := filepath.Join(home, ".dream", serverID); filepath.Dir(cfgPath) != want {
		t.Fatalf("config dir = %q, want %q", filepath.Dir(cfgPath), want)
	}

	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config mode = %04o, want 0600", perm)
	}
}

func TestListLocalServerIDs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if ids, err := listLocalServerIDs(); err != nil || len(ids) != 0 {
		t.Fatalf("empty home: got %v, %v; want no ids and no error", ids, err)
	}

	for _, id := range []string{"a_1", "b_2"} {
		if err := saveLocalConfig(id, localConfig{ServerAddr: id, WorkerID: "w"}); err != nil {
			t.Fatalf("saveLocalConfig(%s): %v", id, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(home, ".dream", "c_3"), 0o700); err != nil {
		t.Fatal(err)
	}

	ids, err := listLocalServerIDs()
	if err != nil {
		t.Fatalf("listLocalServerIDs: %v", err)
	}
	if len(ids) != 2 || ids[0] != "a_1" || ids[1] != "b_2" {
		t.Fatalf("got %v, want [a_1 b_2]", ids)
	}
}

func TestResolveTargetAmbiguity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, err := resolveTarget("", "", ""); err == nil {
		t.Fatal("expected an error with no local identities, got nil")
	}

	for _, id := range []string{"a_1", "b_2"} {
		if err := saveLocalConfig(id, localConfig{ServerAddr: id, WorkerID: "w"}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := resolveTarget("", "", "")
	if err == nil {
		t.Fatal("expected an error with two local identities, got nil")
	}
	if want := "several connected servers"; !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not mention %q", err, want)
	}
}

func TestAPIErrorMessage(t *testing.T) {
	e := &apiError{StatusCode: 409, Message: "worker reports to root; there is no one to escalate to"}
	if got, want := e.Error(), "worker reports to root; there is no one to escalate to (HTTP 409)"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got, want := (&apiError{StatusCode: 500}).Error(), "server returned Internal Server Error"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
