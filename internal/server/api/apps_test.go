package api

import (
	"net/http"
	"testing"
)

func TestSetAppIsAlwaysAboutTheCaller(t *testing.T) {
	e := newTestEnv(t, nil)
	w1 := e.connect("w1", "contributor", "")
	e.connect("w2", "contributor", "")

	status, raw := e.do(http.MethodPost, "/api/apps", w1.Token, map[string]any{
		"worker_id": "w2",
		"url":       "https://evil.example.test",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an unknown worker_id field; body %s", status, raw)
	}

	status, raw = e.do(http.MethodPost, "/api/apps", w1.Token, setAppRequest{
		URL: "https://build.example.test", Description: "Build dashboard",
	})
	if status != http.StatusOK {
		t.Fatalf("set: status %d, body %s", status, raw)
	}
	var got appView
	decodeInto(t, raw, &got)
	if got.WorkerID != "w1" {
		t.Fatalf("app recorded against %q, want the authenticated caller w1", got.WorkerID)
	}

	status, raw = e.do(http.MethodGet, "/api/apps", w1.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d, body %s", status, raw)
	}
	var list struct {
		Apps []appView `json:"apps"`
	}
	decodeInto(t, raw, &list)
	if len(list.Apps) != 1 || list.Apps[0].WorkerID != "w1" {
		t.Fatalf("apps = %+v, want only w1's", list.Apps)
	}
}

func TestSetAppRejectsUnsafeURLs(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("w1", "contributor", "")

	for _, bad := range []string{
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"file:///etc/passwd",
		"example.test",
		"",
	} {
		t.Run(bad, func(t *testing.T) {
			status, raw := e.do(http.MethodPost, "/api/apps", w.Token, setAppRequest{URL: bad})
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d for %q, want 400; body %s", status, bad, raw)
			}
		})
	}
}

func TestListAppsOpenToAnyWorker(t *testing.T) {
	e := newTestEnv(t, nil)
	w1 := e.connect("w1", "contributor", "")
	w2 := e.connect("w2", "contributor", "")

	if status, raw := e.do(http.MethodPost, "/api/apps", w1.Token, setAppRequest{
		URL: "https://build.example.test",
	}); status != http.StatusOK {
		t.Fatalf("set: status %d, body %s", status, raw)
	}

	status, raw := e.do(http.MethodGet, "/api/apps", w2.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("another worker reading apps: status %d, body %s", status, raw)
	}
}

func TestClearAppRemovesOnlyTheCallers(t *testing.T) {
	e := newTestEnv(t, nil)
	w1 := e.connect("w1", "contributor", "")
	w2 := e.connect("w2", "contributor", "")

	for _, w := range []testWorker{w1, w2} {
		if status, _ := e.do(http.MethodPost, "/api/apps", w.Token, setAppRequest{
			URL: "https://" + w.ID + ".example.test",
		}); status != http.StatusOK {
			t.Fatalf("set for %s failed", w.ID)
		}
	}

	if status, raw := e.do(http.MethodDelete, "/api/apps", w1.Token, nil); status != http.StatusOK {
		t.Fatalf("clear: status %d, body %s", status, raw)
	}

	_, raw := e.do(http.MethodGet, "/api/apps", w2.Token, nil)
	var list struct {
		Apps []appView `json:"apps"`
	}
	decodeInto(t, raw, &list)
	if len(list.Apps) != 1 || list.Apps[0].WorkerID != "w2" {
		t.Fatalf("apps = %+v, want only w2's to survive", list.Apps)
	}
}

func TestAppRequiresAuth(t *testing.T) {
	e := newTestEnv(t, nil)
	e.connect("w1", "contributor", "")

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodPost, "/api/apps"},
		{http.MethodGet, "/api/apps"},
		{http.MethodDelete, "/api/apps"},
	} {
		status, _ := e.do(tc.method, tc.path, "", setAppRequest{URL: "https://example.test"})
		if status != http.StatusUnauthorized {
			t.Errorf("%s %s without a token: status %d, want 401", tc.method, tc.path, status)
		}
	}
}
