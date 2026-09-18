package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTailQuery(t *testing.T) {
	tests := []struct {
		name    string
		opts    messageTailOptions
		history bool
		want    string
		wantErr bool
	}{
		{name: "empty", want: ""},
		{name: "worker only", opts: messageTailOptions{Worker: "lead"}, want: "?worker_id=lead"},
		{
			name:    "history filters applied",
			opts:    messageTailOptions{Worker: "lead", Limit: 10},
			history: true,
			want:    "?limit=10&worker_id=lead",
		},
		{
			name:    "history filters dropped for subscribe",
			opts:    messageTailOptions{Worker: "lead", Limit: 10, Since: "2026-08-19T00:00:00Z"},
			history: false,
			want:    "?worker_id=lead",
		},
		{
			name:    "bad since",
			opts:    messageTailOptions{Since: "yesterday"},
			history: true,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tailQuery(tc.opts, tc.history)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStreamEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, ": a comment line\n\n")
		fmt.Fprint(w, "event: message\n")
		fmt.Fprintf(w, "data: {\"id\":\"m1\",\"type\":\"escalation\",\"from\":\"leaf\",\"subject\":\"disk full\",\"created_at\":%q}\n\n",
			time.Now().UTC().Format(time.RFC3339))
		fmt.Fprint(w, "data: not json at all\n\n")
		fmt.Fprintf(w, "data: {\"id\":\"m2\",\"type\":\"status_update\",\"from\":\"leaf\",\"subject\":\"ok\",\"created_at\":%q}\n\n",
			time.Now().UTC().Format(time.RFC3339))
	}))
	defer srv.Close()

	client := &apiClient{baseURL: srv.URL, token: "test-token", hc: srv.Client()}
	var out strings.Builder
	handler := func(env envelopeView) error {
		fmt.Fprintln(&out, formatEnvelopeLine(env))
		return nil
	}
	if err := streamEvents(context.Background(), client, "/stream", handler); err != nil {
		t.Fatalf("streamEvents: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), out.String())
	}
	if !strings.Contains(lines[0], "escalation") || !strings.Contains(lines[0], "disk full") {
		t.Errorf("line 0 = %q", lines[0])
	}
	if !strings.Contains(lines[1], "status_update") {
		t.Errorf("line 1 = %q", lines[1])
	}
}

func TestStreamEventsSurfacesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":"cannot subscribe to another worker's channel without admin scope"}`)
	}))
	defer srv.Close()

	client := &apiClient{baseURL: srv.URL, token: "t", hc: srv.Client()}
	err := streamEvents(context.Background(), client, "/stream", func(envelopeView) error { return nil })
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "admin scope") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunMessageSendRejectsInvalidJSONBody(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := runMessageSend(context.Background(), messageSendOptions{
		Server: "127.0.0.1:1", AsWorker: "w", Type: "status_update", Body: "{not json",
	})
	if err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("got %v, want a JSON validation error", err)
	}
}

func TestFormatEnvelopeLine(t *testing.T) {
	env := envelopeView{
		Type:      "escalation",
		From:      "leaf",
		Subject:   "disk full",
		CreatedAt: time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC),
	}
	line := formatEnvelopeLine(env)
	for _, want := range []string{"escalation", "leaf", "disk full"} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q missing %q", line, want)
		}
	}

	env.Subject = ""
	if !strings.Contains(formatEnvelopeLine(env), "(no subject)") {
		t.Errorf("empty subject not rendered: %q", formatEnvelopeLine(env))
	}
}

func TestEmitEnvelope(t *testing.T) {
	env := envelopeView{
		ID:        "m1",
		Type:      "request_for_input",
		From:      "lead",
		To:        "leaf",
		Subject:   "please look at the build",
		Body:      json.RawMessage(`{"repo":"robotdreams"}`),
		CreatedAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
	}

	t.Run("human line carries no id", func(t *testing.T) {
		var out strings.Builder
		if err := emitEnvelope(&out, env, false); err != nil {
			t.Fatalf("emitEnvelope: %v", err)
		}
		got := out.String()
		if !strings.Contains(got, "request_for_input") || !strings.Contains(got, "please look at the build") {
			t.Fatalf("human line = %q", got)
		}
		if strings.Contains(got, "m1") {
			t.Fatalf("human line now carries the id (%q) — --json's rationale needs revisiting", got)
		}
	})

	t.Run("json line round-trips", func(t *testing.T) {
		var out strings.Builder
		if err := emitEnvelope(&out, env, true); err != nil {
			t.Fatalf("emitEnvelope: %v", err)
		}
		line := out.String()
		if strings.Count(line, "\n") != 1 || !strings.HasSuffix(line, "\n") {
			t.Fatalf("want exactly one trailing newline (NDJSON), got %q", line)
		}
		var back envelopeView
		if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &back); err != nil {
			t.Fatalf("emitted line is not valid JSON: %v (%q)", err, line)
		}
		if back.ID != "m1" || back.Type != "request_for_input" || back.From != "lead" {
			t.Fatalf("round-tripped envelope = %+v", back)
		}
		if string(back.Body) != `{"repo":"robotdreams"}` {
			t.Fatalf("body = %s, want it preserved verbatim", back.Body)
		}
	})
}

func TestEmitEnvelopeJSONIsOneLinePerMessage(t *testing.T) {
	var out strings.Builder
	for _, body := range []string{`{"text":"line one\nline two"}`, `{"a":1}`} {
		err := emitEnvelope(&out, envelopeView{ID: "m", Body: json.RawMessage(body)}, true)
		if err != nil {
			t.Fatalf("emitEnvelope: %v", err)
		}
	}
	if got := strings.Count(out.String(), "\n"); got != 2 {
		t.Fatalf("got %d lines for 2 envelopes: %q", got, out.String())
	}
}
