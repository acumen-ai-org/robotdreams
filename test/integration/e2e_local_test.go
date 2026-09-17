package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/server"
	_ "github.com/acumen-ai-org/robotdreams/pkg/messaging/embedded"
	"github.com/acumen-ai-org/robotdreams/pkg/messaging/webhook"
	_ "github.com/acumen-ai-org/robotdreams/pkg/messaging/webhook"
)

type orgChart struct {
	director *worker
	leadA    *worker
	leadB    *worker
	icA1     *worker
	icA2     *worker
	icB1     *worker
}

func connectOrgChart(ctx context.Context, t *testing.T, ts *testServer) orgChart {
	t.Helper()

	director := connectWorker(ctx, t, ts, "director", "director", "", nil)
	leadA := connectWorker(ctx, t, ts, "lead-a", "lead", "director", nil)
	leadB := connectWorker(ctx, t, ts, "lead-b", "lead", "director", nil)
	icA1 := connectWorker(ctx, t, ts, "ic-a1", "contributor", "lead-a", map[string]string{"team": "a"})
	icA2 := connectWorker(ctx, t, ts, "ic-a2", "contributor", "lead-a", map[string]string{"team": "a"})
	icB1 := connectWorker(ctx, t, ts, "ic-b1", "contributor", "lead-b", map[string]string{"team": "b"})

	return orgChart{director: director, leadA: leadA, leadB: leadB, icA1: icA1, icA2: icA2, icB1: icB1}
}

func runStatusUpdates(ctx context.Context, t *testing.T, w *worker, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		body, _ := json.Marshal(map[string]any{"progress": i + 1, "of": n})
		if _, err := emit(ctx, w, "status_update", "", fmt.Sprintf("working (%d/%d)", i+1, n), body, nil, ""); err != nil {
			t.Fatalf("%s: status_update %d: %v", w.id, i+1, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func runSuccessPath(ctx context.Context, t *testing.T, w *worker, stgURI, path string) (envelopeView, []byte) {
	t.Helper()

	deliverable := []byte(fmt.Sprintf("deliverable from %s at %s\n", w.id, time.Now().UTC().Format(time.RFC3339Nano)))
	meta, err := putObject(ctx, w, path, deliverable, "outcome", "")
	if err != nil {
		t.Fatalf("%s: put deliverable: %v", w.id, err)
	}
	if meta.Revision == "" {
		t.Fatalf("%s: put deliverable returned no revision", w.id)
	}

	ptr := &storagePointerView{Backend: schemeOf(stgURI), Path: path, Revision: meta.Revision}
	body, _ := json.Marshal(map[string]string{"summary": "done"})
	env, err := emit(ctx, w, "completed_work", "", "work complete", body, ptr, "")
	if err != nil {
		t.Fatalf("%s: completed_work: %v", w.id, err)
	}
	return env, deliverable
}

func runEscalationPath(ctx context.Context, t *testing.T, w *worker, reason, causationID string) envelopeView {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"reason": reason})
	env, err := emit(ctx, w, "escalation", "", reason, body, nil, causationID)
	if err != nil {
		t.Fatalf("%s: escalation: %v", w.id, err)
	}
	return env
}

func schemeOf(uri string) string {
	if i := strings.Index(uri, "://"); i >= 0 {
		return uri[:i]
	}
	return uri
}

func mustContainEnvelope(t *testing.T, msgs []envelopeView, id, wantType, wantFrom string) {
	t.Helper()
	for _, m := range msgs {
		if m.ID == id {
			if m.Type != wantType {
				t.Errorf("envelope %s: type = %q, want %q", id, m.Type, wantType)
			}
			if m.From != wantFrom {
				t.Errorf("envelope %s: from = %q, want %q", id, m.From, wantFrom)
			}
			return
		}
	}
	t.Errorf("envelope %s (type %s, from %s) not found among %d messages", id, wantType, wantFrom, len(msgs))
}

func mustNotContainEnvelope(t *testing.T, msgs []envelopeView, id, whereDescription string) {
	t.Helper()
	for _, m := range msgs {
		if m.ID == id {
			t.Errorf("envelope %s unexpectedly present in %s (leak past the intended single hop)", id, whereDescription)
			return
		}
	}
}

type messagingVariant struct {
	name      string
	configure func(t *testing.T) (server.Config, func(ts *testServer))
}

func embeddedVariant() messagingVariant {
	return messagingVariant{
		name: "embedded",
		configure: func(t *testing.T) (server.Config, func(*testServer)) {
			return server.Config{}, nil
		},
	}
}

func webhookVariant() messagingVariant {
	return messagingVariant{
		name: "webhook",
		configure: func(t *testing.T) (server.Config, func(*testServer)) {
			callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(callback.Close)

			setup := func(ts *testServer) {
				wb, ok := ts.Srv.Messaging().(*webhook.Backend)
				if !ok {
					t.Fatalf("webhook variant: server messaging backend is %T, want *webhook.Backend", ts.Srv.Messaging())
				}
				ts.onConnect = func(workerID string) {
					wb.RegisterCallback(workerID, callback.URL)
				}
			}
			return server.Config{MessagingURI: "webhook://"}, setup
		},
	}
}

func TestLocalScenario(t *testing.T) {
	for _, v := range []messagingVariant{embeddedVariant(), webhookVariant()} {
		t.Run(v.name, func(t *testing.T) {
			runLocalScenario(t, v)
		})
	}
}

func runLocalScenario(t *testing.T, v messagingVariant) {
	ctx := context.Background()

	cfg, postSetup := v.configure(t)
	ts := startServer(t, cfg)
	if postSetup != nil {
		postSetup(ts)
	}
	_, stgURI := ts.Srv.BackendURIs()

	chart := connectOrgChart(ctx, t, ts)

	workers, err := listWorkers(ctx, chart.icA1)
	if err != nil {
		t.Fatalf("list workers: %v", err)
	}
	wantParents := map[string]string{
		"director": "",
		"lead-a":   "director",
		"lead-b":   "director",
		"ic-a1":    "lead-a",
		"ic-a2":    "lead-a",
		"ic-b1":    "lead-b",
	}
	gotParents := map[string]string{}
	for _, w := range workers {
		gotParents[w.ID] = w.ReportsTo
	}
	for id, want := range wantParents {
		got, ok := gotParents[id]
		if !ok {
			t.Errorf("worker %q missing from GET /api/workers", id)
			continue
		}
		if got != want {
			t.Errorf("worker %q reports_to = %q, want %q", id, got, want)
		}
	}
	if len(workers) != len(wantParents) {
		t.Errorf("GET /api/workers returned %d workers, want %d: %+v", len(workers), len(wantParents), workers)
	}

	runStatusUpdates(ctx, t, chart.icA1, 3)

	leadATail, err := tail(ctx, chart.leadA, "lead-a")
	if err != nil {
		t.Fatalf("tail lead-a: %v", err)
	}
	statusCount := 0
	for _, m := range leadATail {
		if m.Type == "status_update" && m.From == "ic-a1" {
			statusCount++
		}
	}
	if statusCount != 3 {
		t.Errorf("lead-a's tail has %d status_update(s) from ic-a1, want 3", statusCount)
	}

	for _, other := range []struct {
		id string
		w  *worker
	}{{"lead-b", chart.leadB}, {"ic-b1", chart.icB1}, {"ic-a2", chart.icA2}, {"director", chart.director}} {
		otherTail, err := tail(ctx, other.w, other.id)
		if err != nil {
			t.Fatalf("tail %s: %v", other.id, err)
		}
		for _, m := range otherTail {
			if m.Type == "status_update" && m.From == "ic-a1" {
				t.Errorf("ic-a1's status_update leaked into %s's tail (aggregation boundary violated)", other.id)
			}
		}
	}

	directorEnv, err := emit(ctx, chart.director, "status_update", "", "director rollup", []byte(`{"note":"weekly summary"}`), nil, "")
	if err != nil {
		t.Fatalf("director status_update: %v", err)
	}
	if directorEnv.Delivered {
		t.Errorf("director's status_update (director reports to root) was Delivered=true, want false: nothing above root to aggregate into")
	}

	escFromICA2 := runEscalationPath(ctx, t, chart.icA2, "cannot access required credentials", "")
	if !escFromICA2.Delivered || escFromICA2.To != "lead-a" {
		t.Fatalf("ic-a2 escalation: Delivered=%v To=%q, want Delivered=true To=lead-a", escFromICA2.Delivered, escFromICA2.To)
	}

	leadATail2, err := tail(ctx, chart.leadA, "lead-a")
	if err != nil {
		t.Fatalf("tail lead-a: %v", err)
	}
	mustContainEnvelope(t, leadATail2, escFromICA2.ID, "escalation", "ic-a2")

	for _, other := range []struct {
		id string
		w  *worker
	}{{"director", chart.director}, {"lead-b", chart.leadB}, {"ic-b1", chart.icB1}, {"ic-a1", chart.icA1}} {
		otherTail, err := tail(ctx, other.w, other.id)
		if err != nil {
			t.Fatalf("tail %s: %v", other.id, err)
		}
		mustNotContainEnvelope(t, otherTail, escFromICA2.ID, other.id+"'s tail")
	}

	escFromLeadA := runEscalationPath(ctx, t, chart.leadA, "escalating: credentials issue needs director sign-off", escFromICA2.ID)
	if !escFromLeadA.Delivered || escFromLeadA.To != "director" {
		t.Fatalf("lead-a re-escalation: Delivered=%v To=%q, want Delivered=true To=director", escFromLeadA.Delivered, escFromLeadA.To)
	}
	if escFromLeadA.CausationID != escFromICA2.ID {
		t.Errorf("lead-a re-escalation causation_id = %q, want %q", escFromLeadA.CausationID, escFromICA2.ID)
	}

	directorTail, err := tail(ctx, chart.director, "director")
	if err != nil {
		t.Fatalf("tail director: %v", err)
	}
	mustContainEnvelope(t, directorTail, escFromLeadA.ID, "escalation", "lead-a")
	mustNotContainEnvelope(t, directorTail, escFromICA2.ID, "director's tail (would be a leaf-to-grandparent shortcut)")

	completedEnv, deliverable := runSuccessPath(ctx, t, chart.icB1, stgURI, "shared/work/ic-b1/deliverable.txt")
	if !completedEnv.Delivered || completedEnv.To != "lead-b" {
		t.Fatalf("ic-b1 completed_work: Delivered=%v To=%q, want Delivered=true To=lead-b", completedEnv.Delivered, completedEnv.To)
	}
	if completedEnv.StoragePtr == nil {
		t.Fatal("ic-b1 completed_work carries no storage_ptr")
	}

	leadBTail, err := tail(ctx, chart.leadB, "lead-b")
	if err != nil {
		t.Fatalf("tail lead-b: %v", err)
	}
	mustContainEnvelope(t, leadBTail, completedEnv.ID, "completed_work", "ic-b1")

	gotBytes, meta, err := getObject(ctx, chart.leadB, completedEnv.StoragePtr.Path)
	if err != nil {
		t.Fatalf("get object %q: %v", completedEnv.StoragePtr.Path, err)
	}
	if string(gotBytes) != string(deliverable) {
		t.Fatalf("stored object bytes = %q, want %q", gotBytes, deliverable)
	}
	if meta.Revision != completedEnv.StoragePtr.Revision {
		t.Errorf("fetched revision = %q, want %q (from the completed_work envelope)", meta.Revision, completedEnv.StoragePtr.Revision)
	}

	admin := newAdminActor(t, ts)

	resp, err := authedRequest(ctx, httpClient(), ts.BaseURL, "not-a-real-token", http.MethodGet, "/api/workers", "", nil)
	if err != nil {
		t.Fatalf("request with bogus token: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("bogus token: status = %d, want 401", resp.StatusCode)
	}

	if err := revokeWorker(ctx, admin, "ic-a2", "compromised (test)"); err != nil {
		t.Fatalf("revoke ic-a2: %v", err)
	}

	if _, err := listWorkers(ctx, chart.icA2); err == nil {
		t.Fatal("ic-a2 (revoked) could still call the API")
	} else {
		var ae *apiErr
		if !asAPIErr(err, &ae) {
			t.Fatalf("ic-a2 call after revocation: got %v, want an *apiErr", err)
		}
		if ae.Status != http.StatusUnauthorized && ae.Status != http.StatusForbidden {
			t.Errorf("ic-a2 call after revocation: status = %d, want 401 or 403", ae.Status)
		}
	}

	if _, err := listWorkers(ctx, chart.icA1); err != nil {
		t.Fatalf("ic-a1 (sibling of revoked worker) should still work: %v", err)
	}
	if _, err := emit(ctx, chart.icA1, "status_update", "", "still fine", []byte(`{"ok":true}`), nil, ""); err != nil {
		t.Fatalf("ic-a1 (sibling of revoked worker) should still be able to emit: %v", err)
	}
}

func asAPIErr(err error, target **apiErr) bool {
	ae, ok := err.(*apiErr)
	if !ok {
		return false
	}
	*target = ae
	return true
}

func TestExpiredTokenRejected(t *testing.T) {
	ctx := context.Background()
	ts := startServer(t, server.Config{TokenTTL: 50 * time.Millisecond})

	w := connectWorker(ctx, t, ts, "short-lived", "contributor", "", nil)
	tok := w.token()
	w.client.Stop()

	time.Sleep(200 * time.Millisecond)

	resp, err := authedRequest(ctx, httpClient(), ts.BaseURL, tok, http.MethodGet, "/api/workers", "", nil)
	if err != nil {
		t.Fatalf("request with expired token: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expired token: status = %d, want 401", resp.StatusCode)
	}
}
