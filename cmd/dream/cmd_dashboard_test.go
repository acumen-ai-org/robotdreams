package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRunDashboardServesUntilCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := &syncBuffer{}
	done := make(chan error, 1)
	go func() {
		done <- runDashboard(ctx, dashboardOptions{Server: "control.example.test:7420", Addr: "127.0.0.1:0"}, out)
	}()

	waitForOutput(t, out, "serving at http://", followTimeout)
	if !strings.Contains(out.String(), "Mission Control (standalone client of http://control.example.test:7420)") {
		t.Fatalf("banner does not name the control plane:\n%s", out.String())
	}
	line := out.String()[strings.Index(out.String(), "serving at "):]
	url := strings.TrimSpace(strings.SplitN(strings.TrimPrefix(line, "serving at "), "\n", 2)[0])
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatalf("served URL = %q", url)
	}

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("index content type = %q", ct)
	}
	if !strings.Contains(string(body), `window.__DREAM_API_BASE__="http://control.example.test:7420"`) {
		t.Fatalf("index does not point the browser at the given control plane:\n%s", body)
	}

	cancel()
	if err := waitForErr(t, done, followTimeout); err != nil {
		t.Fatalf("runDashboard returned %v after cancel, want nil", err)
	}
	if !strings.Contains(out.String(), "shutting down...") {
		t.Fatalf("no shutdown line:\n%s", out.String())
	}
	if _, err := (&http.Client{Timeout: time.Second}).Get(url); err == nil {
		t.Fatalf("dashboard still answering at %s after shutdown", url)
	}
}

func TestRunDashboardDefaultsAndEnv(t *testing.T) {
	cases := []struct {
		name   string
		env    string
		server string
		want   string
	}{
		{name: "local default", want: "http://127.0.0.1:7420"},
		{name: "from DREAM_URL", env: "https://cp.example.test", want: "https://cp.example.test"},
		{name: "flag beats env", env: "https://cp.example.test", server: "10.0.0.5:7420", want: "http://10.0.0.5:7420"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envDreamURL, tc.env)
			ctx, cancel := context.WithCancel(context.Background())
			out := &syncBuffer{}
			done := make(chan error, 1)
			go func() {
				done <- runDashboard(ctx, dashboardOptions{Server: tc.server, Addr: "127.0.0.1:0"}, out)
			}()
			waitForOutput(t, out, "serving at http://", followTimeout)
			cancel()
			if err := waitForErr(t, done, followTimeout); err != nil {
				t.Fatalf("runDashboard: %v", err)
			}
			if !strings.Contains(out.String(), "standalone client of "+tc.want+")") {
				t.Fatalf("api base: want %q in\n%s", tc.want, out.String())
			}
		})
	}

	err := runDashboard(context.Background(), dashboardOptions{Addr: "256.256.256.256:1"}, &syncBuffer{})
	if err == nil || !strings.Contains(err.Error(), "listen on 256.256.256.256:1") {
		t.Fatalf("bad --addr: %v", err)
	}
}
