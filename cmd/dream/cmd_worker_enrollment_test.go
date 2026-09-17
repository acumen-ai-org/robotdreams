package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/internal/server/api"
)

func startTestServerAt(t *testing.T, dataDir string) (addr string, srv *server.Server) {
	t.Helper()

	srv, err := server.New(server.Config{DataDir: dataDir})
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

func TestWorkerConnectRequiresEnrollmentAuth(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, _ := startTestServerAt(t, t.TempDir())

	_, err := runWorkerConnect(context.Background(), workerConnectOptions{
		Server: addr, WorkerID: "no-credential", Role: "contributor",
	})
	if err == nil {
		t.Fatal("worker connect succeeded with no admin/enrollment credential available")
	}
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %v, want an HTTP 401 *apiError", err)
	}
}

func TestWorkerConnectAdminTokenFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, srv := startTestServerAt(t, t.TempDir())

	admin, err := srv.MintAdminToken("test-operator", time.Minute)
	if err != nil {
		t.Fatalf("MintAdminToken: %v", err)
	}

	res, err := runWorkerConnect(context.Background(), workerConnectOptions{
		Server: addr, WorkerID: "explicit-admin-token", Role: "contributor",
		AdminToken: admin.Raw,
	})
	if err != nil {
		t.Fatalf("worker connect with --admin-token: %v", err)
	}
	if res.WorkerID != "explicit-admin-token" {
		t.Fatalf("connected as %+v", res)
	}

	res2, err := runWorkerConnect(context.Background(), workerConnectOptions{
		Server: addr, WorkerID: "explicit-enrollment-secret", Role: "contributor",
		AdminToken: srv.EnrollmentToken(),
	})
	if err != nil {
		t.Fatalf("worker connect with the raw enrollment secret: %v", err)
	}
	if res2.WorkerID != "explicit-enrollment-secret" {
		t.Fatalf("connected as %+v", res2)
	}
}

func TestWorkerConnectAutoDiscoversLocalEnrollmentToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dataDir := filepath.Join(home, ".dream", "_server")
	addr, _ := startTestServerAt(t, dataDir)

	res, err := runWorkerConnect(context.Background(), workerConnectOptions{
		Server: addr, WorkerID: "auto-discovered", Role: "contributor",
	})
	if err != nil {
		t.Fatalf("worker connect should auto-discover the local enrollment token: %v", err)
	}
	if res.WorkerID != "auto-discovered" {
		t.Fatalf("connected as %+v", res)
	}
}
