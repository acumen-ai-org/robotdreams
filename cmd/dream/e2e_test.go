package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/internal/server/api"
	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

func startTestServer(t *testing.T) (addr string, srv *server.Server) {
	t.Helper()

	srv, err := server.New(server.Config{DataDir: t.TempDir(), OpenEnrollment: true})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		srv.Close()
		t.Fatalf("listen: %v", err)
	}

	httpSrv := &http.Server{Handler: api.NewRouter(srv)}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("serve: %v", err)
		}
	}()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
		<-done
		_ = srv.Close()
	})

	return ln.Addr().String(), srv
}

func TestEndToEndConnectSendTail(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	addr, _ := startTestServer(t)
	ctx := context.Background()

	lead, err := runWorkerConnect(ctx, workerConnectOptions{
		Server:   addr,
		WorkerID: "lead",
		Role:     "team-lead",
	})
	if err != nil {
		t.Fatalf("connect lead: %v", err)
	}
	if lead.WorkerID != "lead" || lead.ReportsTo != "" {
		t.Fatalf("lead connected as %+v", lead)
	}
	if lead.ExpiresAt.IsZero() {
		t.Fatal("lead got no token expiry")
	}

	leaf, err := runWorkerConnect(ctx, workerConnectOptions{
		Server:    addr,
		WorkerID:  "leaf",
		Role:      "worker",
		ReportsTo: "lead",
		Metadata:  map[string]string{"team": "infra"},
	})
	if err != nil {
		t.Fatalf("connect leaf: %v", err)
	}
	if leaf.ReportsTo != "lead" {
		t.Fatalf("leaf reports to %q, want %q", leaf.ReportsTo, "lead")
	}

	if lead.ServerID != leaf.ServerID {
		t.Fatalf("server ids differ: %q vs %q", lead.ServerID, leaf.ServerID)
	}
	cfg, err := loadLocalConfig(leaf.ServerID)
	if err != nil {
		t.Fatalf("loadLocalConfig: %v", err)
	}
	if cfg.WorkerID != "leaf" || cfg.ServerAddr != addr {
		t.Fatalf("local config = %+v, want worker leaf at %s", cfg, addr)
	}

	workers, err := runWorkerList(ctx, workerListOptions{Server: addr, AsWorker: "leaf"})
	if err != nil {
		t.Fatalf("worker list: %v", err)
	}
	if len(workers) != 2 {
		t.Fatalf("got %d workers, want 2: %+v", len(workers), workers)
	}
	byID := map[string]workerView{}
	for _, w := range workers {
		byID[w.ID] = w
	}
	if byID["leaf"].ReportsTo != "lead" {
		t.Fatalf("leaf reports to %q in the chart", byID["leaf"].ReportsTo)
	}
	if byID["lead"].ReportsTo != "" {
		t.Fatalf("lead reports to %q in the chart, want root", byID["lead"].ReportsTo)
	}

	sent, err := runMessageSend(ctx, messageSendOptions{
		Server:   addr,
		AsWorker: "leaf",
		Type:     "escalation",
		Subject:  "disk is full",
		Body:     `{"free_bytes":0}`,
	})
	if err != nil {
		t.Fatalf("message send: %v", err)
	}
	if !sent.Delivered {
		t.Fatal("escalation was not delivered")
	}
	if sent.From != "leaf" || sent.To != "lead" {
		t.Fatalf("escalation routed %s -> %s, want leaf -> lead", sent.From, sent.To)
	}

	msgs, err := runMessageTail(ctx, messageTailOptions{
		Server:   addr,
		AsWorker: "lead",
		Worker:   "lead",
	})
	if err != nil {
		t.Fatalf("message tail: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d messages in lead's inbox, want exactly 1: %+v", len(msgs), msgs)
	}
	got := msgs[0]
	if got.Type != "escalation" {
		t.Errorf("message type = %q, want escalation", got.Type)
	}
	if got.From != "leaf" {
		t.Errorf("message from = %q, want leaf", got.From)
	}
	if got.ID != sent.ID {
		t.Errorf("message id = %q, want %q", got.ID, sent.ID)
	}
	if got.Subject != "disk is full" {
		t.Errorf("message subject = %q", got.Subject)
	}
	if string(got.Body) != `{"free_bytes":0}` {
		t.Errorf("message body = %q", got.Body)
	}
}

func TestEndToEndEscalationAtRoot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, _ := startTestServer(t)
	ctx := context.Background()

	if _, err := runWorkerConnect(ctx, workerConnectOptions{Server: addr, WorkerID: "solo", Role: "lead"}); err != nil {
		t.Fatalf("connect: %v", err)
	}

	_, err := runMessageSend(ctx, messageSendOptions{
		Server: addr, AsWorker: "solo", Type: "escalation", Subject: "help",
	})
	if err == nil {
		t.Fatal("expected an error escalating from a root worker")
	}
	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *apiError", err)
	}
	if apiErr.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Message, "no one to escalate to") {
		t.Fatalf("message = %q", apiErr.Message)
	}

	env, err := runMessageSend(ctx, messageSendOptions{
		Server: addr, AsWorker: "solo", Type: "status_update", Subject: "still working",
	})
	if err != nil {
		t.Fatalf("status_update: %v", err)
	}
	if env.Delivered {
		t.Fatal("status_update from a root worker should not be delivered")
	}
}

func TestEndToEndReassign(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, _ := startTestServer(t)
	ctx := context.Background()

	for _, w := range []workerConnectOptions{
		{Server: addr, WorkerID: "lead-a", Role: "lead"},
		{Server: addr, WorkerID: "lead-b", Role: "lead"},
		{Server: addr, WorkerID: "leaf", Role: "worker", ReportsTo: "lead-a"},
	} {
		if _, err := runWorkerConnect(ctx, w); err != nil {
			t.Fatalf("connect %s: %v", w.WorkerID, err)
		}
	}

	got, err := runWorkerReassign(ctx, workerReassignOptions{
		Server: addr, AsWorker: "leaf", Target: "leaf", ReportsTo: "lead-b",
	})
	if err != nil {
		t.Fatalf("self reassign: %v", err)
	}
	if got.ReportsTo != "lead-b" {
		t.Fatalf("leaf reports to %q, want lead-b", got.ReportsTo)
	}

	_, err = runWorkerReassign(ctx, workerReassignOptions{
		Server: addr, AsWorker: "lead-a", Target: "leaf", ReportsTo: "lead-a",
	})
	if err == nil {
		t.Fatal("expected a 403 reassigning a worker that is no longer a report")
	}
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("got %v, want HTTP 403", err)
	}
}

func TestEndToEndStorageRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, _ := startTestServer(t)
	ctx := context.Background()

	if _, err := runWorkerConnect(ctx, workerConnectOptions{Server: addr, WorkerID: "solo", Role: "lead"}); err != nil {
		t.Fatalf("connect: %v", err)
	}

	content := "hello from the CLI\n"
	meta, err := runStoragePut(ctx, storagePutOptions{
		storageTargetFlags: storageTargetFlags{Server: addr, AsWorker: "solo"},
		Path:               "workers/solo/reports/day1.txt",
		LocalFile:          "-",
	}, strings.NewReader(content))
	if err != nil {
		t.Fatalf("storage put: %v", err)
	}
	if meta.Size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", meta.Size, len(content))
	}
	if meta.Revision == "" {
		t.Fatal("put returned no revision")
	}

	objects, err := runStorageLs(ctx, storageLsOptions{
		storageTargetFlags: storageTargetFlags{Server: addr, AsWorker: "solo"},
		Prefix:             "workers/solo/reports/",
	})
	if err != nil {
		t.Fatalf("storage ls: %v", err)
	}
	if len(objects) != 1 || objects[0].Path != "workers/solo/reports/day1.txt" {
		t.Fatalf("ls = %+v", objects)
	}

	var stdout, meta2 strings.Builder
	err = runStorageGet(ctx, storageGetOptions{
		storageTargetFlags: storageTargetFlags{Server: addr, AsWorker: "solo"},
		Path:               "workers/solo/reports/day1.txt",
		LocalFile:          "-",
	}, &stdout, &meta2)
	if err != nil {
		t.Fatalf("storage get: %v", err)
	}
	if stdout.String() != content {
		t.Fatalf("got %q, want %q", stdout.String(), content)
	}
	if !strings.Contains(meta2.String(), "revision=") {
		t.Fatalf("metadata line = %q", meta2.String())
	}

	_, err = runStoragePut(ctx, storagePutOptions{
		storageTargetFlags: storageTargetFlags{Server: addr, AsWorker: "solo"},
		Path:               "workers/solo/reports/day1.txt",
		LocalFile:          "-",
		IfMatchRevision:    "not-the-current-revision",
	}, strings.NewReader("nope"))
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
		t.Fatalf("got %v, want HTTP 409", err)
	}
}

func TestServerRevokeMintsLocalAdminToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dataDir := t.TempDir()
	srv, err := server.New(server.Config{DataDir: dataDir, OpenEnrollment: true})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	httpSrv := &http.Server{Handler: api.NewRouter(srv)}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("serve: %v", err)
		}
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
		<-done
		_ = srv.Close()
	})

	addr := ln.Addr().String()
	ctx := context.Background()

	if _, err := runWorkerConnect(ctx, workerConnectOptions{Server: addr, WorkerID: "rogue", Role: "worker"}); err != nil {
		t.Fatalf("connect: %v", err)
	}

	res, err := runServerRevoke(ctx, serverRevokeOptions{
		WorkerID: "rogue",
		DataDir:  dataDir,
		Addr:     addr,
		Reason:   "compromised",
	})
	if err != nil {
		t.Fatalf("server revoke: %v", err)
	}
	if !res.Revoked || res.WorkerID != "rogue" || res.Reason != "compromised" {
		t.Fatalf("revoke result = %+v", res)
	}

	if _, err := runWorkerList(ctx, workerListOptions{Server: addr, AsWorker: "rogue"}); err == nil {
		t.Fatal("revoked worker still authenticated")
	}
}

func TestEndToEndUpdateAnnouncementAndRollout(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, srv := startTestServer(t)
	ctx := context.Background()

	if _, err := runWorkerConnect(ctx, workerConnectOptions{
		Server: addr, WorkerID: "leaf", Role: "worker",
		Versions: map[string]string{"acme/prompt-pack": "3"},
	}); err != nil {
		t.Fatalf("connect: %v", err)
	}

	rollout, err := srv.UpdateRollout(ctx, updates.KindCLI, "")
	if err != nil {
		t.Fatalf("rollout: %v", err)
	}
	if len(rollout.Nodes) != 1 || rollout.Nodes[0].CurrentVersion != version {
		t.Fatalf("connect did not declare the CLI version: %+v", rollout.Nodes)
	}
	packRollout, err := srv.UpdateRollout(ctx, "acme/prompt-pack", "")
	if err != nil {
		t.Fatalf("rollout for the deployment kind: %v", err)
	}
	if packRollout.Nodes[0].CurrentVersion != "3" {
		t.Fatalf("--version-of was not declared: %+v", packRollout.Nodes[0])
	}

	adminTok, err := srv.MintAdminToken("ops", time.Hour)
	if err != nil {
		t.Fatalf("MintAdminToken: %v", err)
	}
	res, err := runUpdatesAnnounce(ctx, updatesAnnounceOptions{
		updatesAdminOptions: updatesAdminOptions{Addr: addr, AdminToken: adminTok.Raw},
		Kind:                "acme/prompt-pack",
		Version:             "4",
		Severity:            updates.SeverityRecommended,
	})
	if err != nil {
		t.Fatalf("announce: %v", err)
	}
	if res.Recipients != 1 || res.Delivered != 1 {
		t.Fatalf("recipients/delivered = %d/%d, want 1/1", res.Recipients, res.Delivered)
	}

	pending, err := runUpdatesPending(ctx, updatesPendingOptions{
		updatesClientOptions: updatesClientOptions{Server: addr},
	})
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	if len(pending) != 1 || pending[0].Announcement.Kind != "acme/prompt-pack" {
		t.Fatalf("pending = %+v, want the prompt-pack announcement", pending)
	}

	envs, err := runMessageTail(ctx, messageTailOptions{Server: addr})
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	var found bool
	for _, env := range envs {
		if env.Subject == updates.SubjectUpdateAvailable {
			found = true
			if env.Type != string(messaging.TypeStatusUpdate) {
				t.Fatalf("announcement type = %q, want %q", env.Type, messaging.TypeStatusUpdate)
			}
			if env.From != server.ControlWorkerID {
				t.Fatalf("announcement from = %q, want %q", env.From, server.ControlWorkerID)
			}
		}
	}
	if !found {
		t.Fatalf("no announcement in the node's inbox: %+v", envs)
	}

	if err := runUpdatesReport(ctx, updatesReportOptions{
		updatesClientOptions: updatesClientOptions{Server: addr},
		Kind:                 "acme/prompt-pack",
		Status:               updates.StatusApplied,
		AnnouncementID:       res.Announcement.ID,
		CurrentVersion:       "4",
	}); err != nil {
		t.Fatalf("report: %v", err)
	}

	final, err := runUpdatesRollout(ctx, updatesRolloutOptions{
		updatesAdminOptions: updatesAdminOptions{Addr: addr, AdminToken: adminTok.Raw},
		Kind:                "acme/prompt-pack",
	})
	if err != nil {
		t.Fatalf("rollout: %v", err)
	}
	if final.Counts[updates.StatusApplied] != 1 {
		t.Fatalf("applied count = %d, want 1 (counts %+v)", final.Counts[updates.StatusApplied], final.Counts)
	}
	if final.Nodes[0].CurrentVersion != "4" {
		t.Fatalf("node version = %q, want 4", final.Nodes[0].CurrentVersion)
	}

	pending, err = runUpdatesPending(ctx, updatesPendingOptions{
		updatesClientOptions: updatesClientOptions{Server: addr},
	})
	if err != nil {
		t.Fatalf("pending after apply: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %+v, want empty", pending)
	}
}

func TestEndToEndMessageAck(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, _ := startTestServer(t)
	ctx := context.Background()

	for _, w := range []workerConnectOptions{
		{Server: addr, WorkerID: "lead", Role: "lead"},
		{Server: addr, WorkerID: "leaf", Role: "worker", ReportsTo: "lead"},
	} {
		if _, err := runWorkerConnect(ctx, w); err != nil {
			t.Fatalf("connect %s: %v", w.WorkerID, err)
		}
	}

	sent, err := runMessageSend(ctx, messageSendOptions{
		Server: addr, AsWorker: "leaf", Type: "escalation", Subject: "disk full",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	if err := runMessageAck(ctx, messageAckOptions{
		Server: addr, AsWorker: "lead", MessageID: sent.ID, Action: "handled",
	}); err != nil {
		t.Fatalf("ack: %v", err)
	}

	if err := runMessageAck(ctx, messageAckOptions{
		Server: addr, AsWorker: "lead", MessageID: "no-such-message",
	}); err == nil {
		t.Fatal("acking an unknown message succeeded, want an error")
	}
}

func TestEndToEndTailJSONEnablesTheAckLoop(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, _ := startTestServer(t)
	ctx := context.Background()

	for _, w := range []workerConnectOptions{
		{Server: addr, WorkerID: "lead", Role: "lead"},
		{Server: addr, WorkerID: "leaf", Role: "worker", ReportsTo: "lead"},
	} {
		if _, err := runWorkerConnect(ctx, w); err != nil {
			t.Fatalf("connect %s: %v", w.WorkerID, err)
		}
	}

	for _, subject := range []string{"disk full", "build broken"} {
		if _, err := runMessageSend(ctx, messageSendOptions{
			Server: addr, AsWorker: "leaf", Type: "escalation", Subject: subject,
		}); err != nil {
			t.Fatalf("send %q: %v", subject, err)
		}
	}

	envs, err := runMessageTail(ctx, messageTailOptions{Server: addr, AsWorker: "lead"})
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if len(envs) != 2 {
		t.Fatalf("got %d messages, want 2", len(envs))
	}

	var stream strings.Builder
	for _, env := range envs {
		if err := emitEnvelope(&stream, env, true); err != nil {
			t.Fatalf("emitEnvelope: %v", err)
		}
	}

	var acked []string
	for _, line := range strings.Split(strings.TrimSpace(stream.String()), "\n") {
		var env envelopeView
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			t.Fatalf("listener could not parse a stream line: %v (%q)", err, line)
		}
		if env.ID == "" {
			t.Fatal("stream line carries no message id; the listener cannot ack it")
		}
		if err := runMessageAck(ctx, messageAckOptions{
			Server: addr, AsWorker: "lead", MessageID: env.ID, Action: "handled",
		}); err != nil {
			t.Fatalf("ack %s: %v", env.ID, err)
		}
		acked = append(acked, env.ID)
	}
	if len(acked) != 2 {
		t.Fatalf("acked %d messages, want 2", len(acked))
	}

	again, err := runMessageTail(ctx, messageTailOptions{Server: addr, AsWorker: "lead"})
	if err != nil {
		t.Fatalf("tail after ack: %v", err)
	}
	if len(again) != 2 {
		t.Fatalf("history returned %d messages after acking, want both still there", len(again))
	}
}
