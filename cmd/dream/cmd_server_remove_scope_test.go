package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/internal/server/api"
	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
)

func TestServerRemoveScope(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(envDreamToken, "")
	t.Setenv(envDreamURL, "")

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

	for _, w := range []workerConnectOptions{
		{Server: addr, WorkerID: "censio-captario", Role: "node"},
		{Server: addr, WorkerID: "mgr-captario", Role: "manager", ReportsTo: "censio-captario"},
		{Server: addr, WorkerID: "executor-captario", Role: "worker", ReportsTo: "mgr-captario"},
		{Server: addr, WorkerID: "bystander", Role: "node"},
	} {
		if _, err := runWorkerConnect(ctx, w); err != nil {
			t.Fatalf("connect %s: %v", w.WorkerID, err)
		}
	}
	for _, id := range []string{"sched-captario-idea-collection", "sched-captarios-keep", "unrelated"} {
		mustRunDreamCmd(t, "schedule", "create", "--server", addr, "--worker-id", "bystander",
			"--id", id, "--cron", "0 9 * * *", "--to", "bystander")
	}
	for _, p := range []string{"workers/censio-captario/runs/1/out.txt", "workers/censio-captario/runs/2/out.txt"} {
		if _, err := runStoragePut(ctx, storagePutOptions{
			storageTargetFlags: storageTargetFlags{Server: addr, AsWorker: "censio-captario"},
			Path:               p, LocalFile: "-",
		}, strings.NewReader("x")); err != nil {
			t.Fatalf("storage put %s: %v", p, err)
		}
	}
	base := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	for _, scope := range []string{"captario", "captario/strategy/goals", "captarios"} {
		if err := srv.Store().InsertReportInstance(ctx, &reporting.Instance{
			Definition: "activity", Scope: scope, Producer: "censio-captario", ProducedAt: base,
		}); err != nil {
			t.Fatalf("insert report instance: %v", err)
		}
	}

	opts := serverRemoveScopeOptions{
		Scope:           "captario",
		Root:            "censio-captario",
		StoragePrefixes: []string{"workers/censio-captario/"},
		Server:          addr,
		DataDir:         dataDir,
		DryRun:          true,
	}

	var out strings.Builder
	plan, err := runServerRemoveScope(ctx, opts, &out)
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, out.String())
	}
	if !plan.RootFound || len(plan.Workers) != 3 || len(plan.Schedules) != 1 ||
		plan.Objects["workers/censio-captario/"] != 2 || plan.ReportInstances != 2 {
		t.Fatalf("dry run plan = %+v\n%s", plan, out.String())
	}
	for _, want := range []string{
		"dry run", "would remove executor-captario", "would remove sched-captario-idea-collection",
		"would remove 2 objects under workers/censio-captario/", "would remove 2 instances",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out.String())
		}
	}
	if _, err := srv.Graph().Get("executor-captario"); err != nil {
		t.Fatalf("dry run removed a worker: %v", err)
	}

	opts.DryRun = false
	out.Reset()
	res, err := runServerRemoveScope(ctx, opts, &out)
	if err != nil {
		t.Fatalf("remove-scope: %v\n%s", err, out.String())
	}
	if len(res.Workers) != 3 || len(res.Schedules) != 1 || res.Objects["workers/censio-captario/"] != 2 || res.ReportInstances != 2 {
		t.Fatalf("result = %+v\n%s", res, out.String())
	}
	if !strings.Contains(out.String(), "summary for captario: removed 3 workers, 1 schedules, 2 objects, 2 report instances") {
		t.Fatalf("summary missing:\n%s", out.String())
	}

	for _, id := range []string{"censio-captario", "mgr-captario", "executor-captario"} {
		if _, err := srv.Graph().Get(id); err == nil {
			t.Errorf("worker %s survived", id)
		}
	}
	if _, err := srv.Graph().Get("bystander"); err != nil {
		t.Errorf("bystander removed: %v", err)
	}
	scheds, err := runScheduleList(ctx, scheduleListOptions{Server: addr, AsWorker: "bystander"})
	if err != nil {
		t.Fatalf("schedule list: %v", err)
	}
	var ids []string
	for _, s := range scheds {
		ids = append(ids, s.ID)
	}
	if strings.Join(ids, ",") != "sched-captarios-keep,unrelated" {
		t.Fatalf("schedules left = %v", ids)
	}
	scopes, err := srv.Store().ListReportScopes(ctx, time.Time{})
	if err != nil {
		t.Fatalf("list report scopes: %v", err)
	}
	if len(scopes) != 1 || scopes[0].Path != "captarios" {
		t.Fatalf("report scopes left = %+v", scopes)
	}

	// A second run finds nothing and succeeds.
	out.Reset()
	again, err := runServerRemoveScope(ctx, opts, &out)
	if err != nil {
		t.Fatalf("second run: %v\n%s", err, out.String())
	}
	if again.RootFound || len(again.Workers) != 0 || len(again.Schedules) != 0 || again.ReportInstances != 0 {
		t.Fatalf("second run = %+v", again)
	}
	if !strings.Contains(out.String(), "censio-captario not on this plane, nothing to do") {
		t.Fatalf("second run output:\n%s", out.String())
	}

	// A client that publishes under its own prefix names the report scope
	// separately from the scope that keys its schedules.
	if err := srv.Store().InsertReportInstance(ctx, &reporting.Instance{
		Definition: "activity", Scope: "censio/captario/strategy", Producer: "censio-internal", ProducedAt: base,
	}); err != nil {
		t.Fatalf("insert report instance: %v", err)
	}
	opts.ReportScope = "censio/captario"
	out.Reset()
	prefixed, err := runServerRemoveScope(ctx, opts, &out)
	if err != nil {
		t.Fatalf("report-scope run: %v\n%s", err, out.String())
	}
	if prefixed.ReportInstances != 1 || !strings.Contains(out.String(), "under censio/captario") {
		t.Fatalf("report-scope run = %+v\n%s", prefixed, out.String())
	}
	scopes, err = srv.Store().ListReportScopes(ctx, time.Time{})
	if err != nil {
		t.Fatalf("list report scopes: %v", err)
	}
	if len(scopes) != 1 || scopes[0].Path != "captarios" {
		t.Fatalf("report scopes left after --report-scope = %+v", scopes)
	}
}

func TestServerRemoveScopeRejectsBadInput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := runServerRemoveScope(context.Background(), serverRemoveScopeOptions{Scope: " / "}, nil); err == nil {
		t.Fatal("empty scope accepted")
	}
	if _, err := runServerRemoveScope(context.Background(), serverRemoveScopeOptions{
		Scope: "captario", StoragePrefixes: []string{"workers/x"},
	}, nil); err == nil || !strings.Contains(err.Error(), "must end with /") {
		t.Fatalf("prefix without slash: %v", err)
	}
}
