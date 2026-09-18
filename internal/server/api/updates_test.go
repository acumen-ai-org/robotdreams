package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

func (e *testEnv) announce(req announceRequest) announceResponse {
	e.t.Helper()
	status, raw := e.do(http.MethodPost, "/api/updates", e.adminToken("ops"), req)
	if status != http.StatusAccepted {
		e.t.Fatalf("announce: status %d, body %s", status, raw)
	}
	var resp announceResponse
	decodeInto(e.t, raw, &resp)
	return resp
}

func TestAnnounceRequiresAdminScope(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("w1", "contributor", "")

	tests := []struct {
		name  string
		token string
		want  int
	}{
		{"no token", "", http.StatusUnauthorized},
		{"worker token", w.Token, http.StatusForbidden},
		{"admin token", e.adminToken("ops"), http.StatusAccepted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, raw := e.do(http.MethodPost, "/api/updates", tc.token, announceRequest{
				Kind: updates.KindCLI, Version: "0.4.2",
			})
			if status != tc.want {
				t.Fatalf("status = %d, want %d; body %s", status, tc.want, raw)
			}
		})
	}
}

func TestRolloutRequiresAdminScope(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("w1", "contributor", "")
	e.announce(announceRequest{Kind: updates.KindCLI, Version: "0.4.2"})

	path := "/api/updates/rollout?kind=" + updates.KindCLI
	tests := []struct {
		name  string
		token string
		want  int
	}{
		{"no token", "", http.StatusUnauthorized},
		{"worker token", w.Token, http.StatusForbidden},
		{"admin token", e.adminToken("ops"), http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, raw := e.do(http.MethodGet, path, tc.token, nil)
			if status != tc.want {
				t.Fatalf("status = %d, want %d; body %s", status, tc.want, raw)
			}
		})
	}
}

func TestListUpdatesOpenToAnyWorker(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("w1", "contributor", "")
	e.announce(announceRequest{Kind: updates.KindCLI, Version: "0.4.2"})

	status, raw := e.do(http.MethodGet, "/api/updates", w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", status, raw)
	}
	var resp struct {
		Announcements []updates.Announcement `json:"announcements"`
	}
	decodeInto(t, raw, &resp)
	if len(resp.Announcements) != 1 {
		t.Fatalf("got %d announcements, want 1", len(resp.Announcements))
	}
}

func TestAnnouncementBodyMatchesListedAnnouncement(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("w1", "contributor", "")

	e.announce(announceRequest{
		Kind:       updates.KindCLI,
		Version:    "0.4.2",
		Source:     "https://example.test/v0.4.2",
		MinVersion: "0.3.0",
		Severity:   updates.SeverityRecommended,
		Notes:      "finish in-flight work first",
	})

	status, raw := e.do(http.MethodGet, "/api/updates", w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d, body %s", status, raw)
	}
	var listed struct {
		Announcements []json.RawMessage `json:"announcements"`
	}
	decodeInto(t, raw, &listed)
	if len(listed.Announcements) != 1 {
		t.Fatalf("got %d announcements, want 1", len(listed.Announcements))
	}

	status, raw = e.do(http.MethodGet, "/api/messages?worker_id=w1", w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("tail: status %d, body %s", status, raw)
	}
	var inbox struct {
		Messages []envelopeView `json:"messages"`
	}
	decodeInto(t, raw, &inbox)

	var body json.RawMessage
	for _, m := range inbox.Messages {
		if m.Subject == updates.SubjectUpdateAvailable {
			body = m.Body
			break
		}
	}
	if body == nil {
		t.Fatalf("no update announcement in w1's inbox; got %+v", inbox.Messages)
	}

	if string(body) != string(listed.Announcements[0]) {
		t.Fatalf("push and pull disagree about the announcement\n inbox: %s\n   api: %s",
			body, listed.Announcements[0])
	}
}

func TestUpdateReportSubjectIsAuthenticatedCaller(t *testing.T) {
	e := newTestEnv(t, nil)
	w1 := e.connect("w1", "contributor", "")
	e.connect("w2", "contributor", "")

	status, raw := e.do(http.MethodPost, "/api/updates/reports", w1.Token, map[string]any{
		"worker_id": "w2",
		"kind":      updates.KindCLI,
		"status":    updates.StatusCurrent,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an unknown worker_id field; body %s", status, raw)
	}

	status, raw = e.do(http.MethodPost, "/api/updates/reports", w1.Token, updateReportRequest{
		Kind: updates.KindCLI, Status: updates.StatusCurrent, CurrentVersion: "0.4.1",
	})
	if status != http.StatusOK {
		t.Fatalf("report: status %d, body %s", status, raw)
	}

	e.announce(announceRequest{Kind: updates.KindCLI, Version: "0.4.2"})
	status, raw = e.do(http.MethodGet, "/api/updates/rollout?kind="+updates.KindCLI, e.adminToken("ops"), nil)
	if status != http.StatusOK {
		t.Fatalf("rollout: status %d, body %s", status, raw)
	}
	var rollout updates.Rollout
	decodeInto(t, raw, &rollout)

	for _, n := range rollout.Nodes {
		switch n.WorkerID {
		case "w1":
			if n.CurrentVersion != "0.4.1" {
				t.Fatalf("w1 current version = %q, want 0.4.1", n.CurrentVersion)
			}
		case "w2":
			if n.CurrentVersion != "" {
				t.Fatalf("w2 has version %q; w1's report leaked onto it", n.CurrentVersion)
			}
		}
	}
}

func TestUpdateReportRejectsBadInput(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("w1", "contributor", "")

	tests := []struct {
		name string
		req  updateReportRequest
	}{
		{"empty kind", updateReportRequest{Status: updates.StatusCurrent}},
		{"malformed kind", updateReportRequest{Kind: "nope", Status: updates.StatusCurrent}},
		{"empty status", updateReportRequest{Kind: updates.KindCLI}},
		{"unknown status", updateReportRequest{Kind: updates.KindCLI, Status: "done"}},
		{"synthesized status", updateReportRequest{Kind: updates.KindCLI, Status: updates.StatusUnknown}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, raw := e.do(http.MethodPost, "/api/updates/reports", w.Token, tc.req)
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body %s", status, raw)
			}
		})
	}
}

func TestAnnounceRejectsInvalidKind(t *testing.T) {
	e := newTestEnv(t, nil)
	admin := e.adminToken("ops")

	tests := []struct {
		name string
		req  announceRequest
	}{
		{"empty kind", announceRequest{Version: "1"}},
		{"unnamespaced kind", announceRequest{Kind: "cli", Version: "1"}},
		{"reserved namespace", announceRequest{Kind: "robotdreams/other", Version: "1"}},
		{"empty version", announceRequest{Kind: updates.KindCLI}},
		{"unknown severity", announceRequest{Kind: updates.KindCLI, Version: "1", Severity: "urgent"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, raw := e.do(http.MethodPost, "/api/updates", admin, tc.req)
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body %s", status, raw)
			}
		})
	}
}

func TestPendingUpdates(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("w1", "contributor", "")

	resp := e.announce(announceRequest{Kind: updates.KindCLI, Version: "0.4.2"})

	status, raw := e.do(http.MethodGet, "/api/updates/pending", w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("pending: status %d, body %s", status, raw)
	}
	var got struct {
		Pending []updates.Pending `json:"pending"`
	}
	decodeInto(t, raw, &got)
	if len(got.Pending) != 1 {
		t.Fatalf("got %d pending, want 1", len(got.Pending))
	}

	status, raw = e.do(http.MethodPost, "/api/updates/reports", w.Token, updateReportRequest{
		Kind: updates.KindCLI, AnnouncementID: resp.Announcement.ID,
		Status: updates.StatusApplied, CurrentVersion: "0.4.2",
	})
	if status != http.StatusOK {
		t.Fatalf("report: status %d, body %s", status, raw)
	}

	status, raw = e.do(http.MethodGet, "/api/updates/pending", w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("pending after apply: status %d, body %s", status, raw)
	}
	decodeInto(t, raw, &got)
	if len(got.Pending) != 0 {
		t.Fatalf("got %d pending after applying, want 0", len(got.Pending))
	}
}

func TestPendingUpdatesCrossWorkerNeedsAdmin(t *testing.T) {
	e := newTestEnv(t, nil)
	w1 := e.connect("w1", "contributor", "")
	e.connect("w2", "contributor", "")

	status, raw := e.do(http.MethodGet, "/api/updates/pending?worker_id=w2", w1.Token, nil)
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body %s", status, raw)
	}

	status, raw = e.do(http.MethodGet, "/api/updates/pending?worker_id=w2", e.adminToken("ops"), nil)
	if status != http.StatusOK {
		t.Fatalf("admin status = %d, want 200; body %s", status, raw)
	}
}

func TestRolloutRequiresKindAndKnownAnnouncement(t *testing.T) {
	e := newTestEnv(t, nil)
	admin := e.adminToken("ops")

	status, _ := e.do(http.MethodGet, "/api/updates/rollout", admin, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("missing kind: status = %d, want 400", status)
	}

	status, _ = e.do(http.MethodGet,
		"/api/updates/rollout?kind="+updates.KindCLI+"&announcement_id=nope", admin, nil)
	if status != http.StatusNotFound {
		t.Fatalf("unknown announcement: status = %d, want 404", status)
	}
}

func TestAnnounceReportsRecipientCount(t *testing.T) {
	e := newTestEnv(t, nil)
	e.connect("w1", "contributor", "")
	e.connect("w2", "contributor", "")

	resp := e.announce(announceRequest{Kind: updates.KindCLI, Version: "0.4.2"})
	if resp.Recipients != 2 || resp.Delivered != 2 {
		t.Fatalf("recipients/delivered = %d/%d, want 2/2", resp.Recipients, resp.Delivered)
	}
	if resp.Warning != "" {
		t.Fatalf("unexpected warning: %s", resp.Warning)
	}
	if resp.Announcement.ID == "" {
		t.Fatal("response carries no announcement ID")
	}
}

func TestAnnounceBroadcastsOneLiveEvent(t *testing.T) {
	e := newTestEnv(t, nil)
	e.connect("w1", "contributor", "")
	e.connect("w2", "contributor", "")

	ch, unsubscribe := e.api.events.subscribe()
	defer unsubscribe()

	resp := e.announce(announceRequest{
		Kind: updates.KindCLI, Version: "0.4.2", Severity: updates.SeverityRecommended,
	})

	var got sseEvent
	select {
	case got = <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("no event was broadcast for an announcement")
	}

	if got.Type != "update_announced" {
		t.Fatalf("event type = %q, want update_announced", got.Type)
	}
	data, ok := got.Data.(map[string]any)
	if !ok {
		t.Fatalf("event data is %T, want a map", got.Data)
	}
	if data["announcement_id"] != resp.Announcement.ID {
		t.Fatalf("announcement_id = %v, want %s", data["announcement_id"], resp.Announcement.ID)
	}
	if data["kind"] != updates.KindCLI || data["version"] != "0.4.2" {
		t.Fatalf("event payload = %+v", data)
	}
	if data["recipients"] != 2 {
		t.Fatalf("recipients = %v, want 2", data["recipients"])
	}

	select {
	case extra := <-ch:
		t.Fatalf("a second event was broadcast for one announcement: %+v", extra)
	case <-time.After(300 * time.Millisecond):
	}
}
