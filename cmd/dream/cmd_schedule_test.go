package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScheduleCreateRequest(t *testing.T) {
	dir := t.TempDir()
	bodyPath := filepath.Join(dir, "body.json")
	if err := os.WriteFile(bodyPath, []byte(`{"job":"idea-collector"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badPath, []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		opts       scheduleCreateOptions
		stdin      string
		wantMethod string
		wantPath   string
		wantBody   map[string]any
		wantErr    string
	}{
		{
			name:       "post mints an id",
			opts:       scheduleCreateOptions{Cron: "0 9 * * *"},
			wantMethod: http.MethodPost,
			wantPath:   "/api/schedules",
			wantBody:   map[string]any{"cron": "0 9 * * *"},
		},
		{
			name:       "put with an id and every field",
			opts:       scheduleCreateOptions{ID: "sched-1", Cron: " TZ=Europe/Stockholm 0 9 * * 1 ", To: "leaf-1", Subject: "go", Owner: "lead-1", Disabled: true, BodyFile: bodyPath},
			wantMethod: http.MethodPut,
			wantPath:   "/api/schedules/sched-1",
			wantBody: map[string]any{
				"cron": "TZ=Europe/Stockholm 0 9 * * 1", "to": "leaf-1", "subject": "go", "owner": "lead-1",
				"enabled": false, "body": map[string]any{"job": "idea-collector"},
			},
		},
		{
			name:       "body from stdin",
			opts:       scheduleCreateOptions{Cron: "* * * * *", BodyFile: "-"},
			stdin:      `["x"]`,
			wantMethod: http.MethodPost,
			wantPath:   "/api/schedules",
			wantBody:   map[string]any{"cron": "* * * * *", "body": []any{"x"}},
		},
		{name: "missing cron", opts: scheduleCreateOptions{To: "x"}, wantErr: "--cron is required"},
		{name: "invalid body file", opts: scheduleCreateOptions{Cron: "* * * * *", BodyFile: badPath}, wantErr: "not valid JSON"},
		{name: "missing body file", opts: scheduleCreateOptions{Cron: "* * * * *", BodyFile: filepath.Join(dir, "nope.json")}, wantErr: "read --body-file"},
		{name: "id with a slash", opts: scheduleCreateOptions{ID: "a/b", Cron: "* * * * *"}, wantErr: "--id must not"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			method, path, body, err := scheduleCreateRequest(tc.opts, strings.NewReader(tc.stdin))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if method != tc.wantMethod || path != tc.wantPath {
				t.Fatalf("%s %s, want %s %s", method, path, tc.wantMethod, tc.wantPath)
			}
			got, _ := json.Marshal(body)
			want, _ := json.Marshal(tc.wantBody)
			var g, w any
			_ = json.Unmarshal(got, &g)
			_ = json.Unmarshal(want, &w)
			if fmt.Sprint(g) != fmt.Sprint(w) {
				t.Fatalf("body = %s, want %s", got, want)
			}
		})
	}
}

func TestScheduleListPath(t *testing.T) {
	if got := scheduleListPath(""); got != "/api/schedules" {
		t.Fatalf("empty worker: %q", got)
	}
	if got := scheduleListPath("lead 1"); got != "/api/schedules?worker_id=lead+1" {
		t.Fatalf("worker: %q", got)
	}
}

func TestFetchSchedules(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		if r.URL.Path != "/api/schedules" || r.URL.Query().Get("worker_id") != "leaf-1" {
			t.Errorf("request = %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"schedules":[{"id":"s1","owner":"lead-1","to":"leaf-1","cron":"0 9 * * *","subject":"go","enabled":true,"next_at":"2026-09-14T09:00:00Z","created_at":"2026-09-13T09:00:00Z"}]}`)
	}))
	defer srv.Close()

	client := &apiClient{baseURL: srv.URL, token: "test-token", hc: srv.Client()}
	list, err := fetchSchedules(context.Background(), client, "leaf-1")
	if err != nil {
		t.Fatalf("fetchSchedules: %v", err)
	}
	if len(list) != 1 || list[0].ID != "s1" || list[0].To != "leaf-1" || !list[0].NextAt.Equal(time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("list = %+v", list)
	}
}

func TestFetchSchedulesSurfacesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":"cannot list another worker's schedules without admin scope"}`)
	}))
	defer srv.Close()

	client := &apiClient{baseURL: srv.URL, token: "t", hc: srv.Client()}
	_, err := fetchSchedules(context.Background(), client, "lead-1")
	if err == nil || !strings.Contains(err.Error(), "admin scope") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunScheduleCreateRejectsBadInputBeforeDialing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := runScheduleCreate(context.Background(), scheduleCreateOptions{
		Server: "127.0.0.1:1", AsWorker: "w", Cron: "",
	}, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "--cron is required") {
		t.Fatalf("got %v, want a --cron error", err)
	}
}

func TestFormatScheduleLine(t *testing.T) {
	last := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)
	s := scheduleView{
		ID: "sched-ideas", Owner: "lead-1", To: "leaf-1", Cron: "0 9 * * *", Subject: "collect ideas",
		Enabled: true, NextAt: time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC), LastAt: &last,
	}
	line := formatScheduleLine(s)
	for _, want := range []string{"sched-ideas", "0 9 * * *", "leaf-1", "collect ideas", "next ", "last "} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q missing %q", line, want)
		}
	}
	if strings.Contains(line, "disabled") || strings.Contains(line, "never") {
		t.Errorf("line %q", line)
	}

	s.Enabled, s.LastAt, s.Subject = false, nil, ""
	line = formatScheduleLine(s)
	for _, want := range []string{"(disabled)", "never", "(no subject)"} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q missing %q", line, want)
		}
	}
}
