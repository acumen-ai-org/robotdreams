package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/identity"
)

func TestTokenSourceForRunsTheHandshake(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, _ := startTestServer(t)
	ctx := context.Background()

	res, err := runWorkerConnect(ctx, workerConnectOptions{Server: addr, WorkerID: "follower", Role: "worker"})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	kp, err := identity.LoadKeyPair(res.KeyPath)
	if err != nil {
		t.Fatalf("load keypair: %v", err)
	}

	src := tokenSourceFor(defaultHTTPClient(), res.ServerURL, "follower", kp)

	before := time.Now()
	tok, err := src.FetchToken(ctx)
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.Raw == "" {
		t.Fatal("empty token")
	}
	if tok.IssuedAt.Before(before) || tok.IssuedAt.After(time.Now()) {
		t.Fatalf("IssuedAt %s is not stamped at receipt (test started %s)", tok.IssuedAt, before)
	}
	if !tok.ExpiresAt.After(tok.IssuedAt) {
		t.Fatalf("ExpiresAt %s is not after IssuedAt %s", tok.ExpiresAt, tok.IssuedAt)
	}

	client := &apiClient{baseURL: res.ServerURL, token: tok.Raw, hc: defaultHTTPClient()}
	var out struct {
		Workers []workerView `json:"workers"`
	}
	if err := client.doJSON(ctx, http.MethodGet, "/api/workers", nil, &out); err != nil {
		t.Fatalf("token from the source was rejected: %v", err)
	}
	if len(out.Workers) != 1 || out.Workers[0].ID != "follower" {
		t.Fatalf("workers = %+v", out.Workers)
	}

	again, err := src.FetchToken(ctx)
	if err != nil {
		t.Fatalf("second Token: %v", err)
	}
	if again.Raw == tok.Raw {
		t.Fatal("the source returned the same token twice; each call must run a fresh handshake")
	}
}

func TestTokenSourceForRejectsTheWrongKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, _ := startTestServer(t)
	ctx := context.Background()

	if _, err := runWorkerConnect(ctx, workerConnectOptions{Server: addr, WorkerID: "real", Role: "worker"}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	impostor, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	src := tokenSourceFor(defaultHTTPClient(), baseURLFromAddr(addr), "real", impostor)
	_, err = src.FetchToken(ctx)
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %v, want an HTTP 401 *apiError", err)
	}
}

func TestTokenSourceForSurfacesTransportErrors(t *testing.T) {
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"nonce":"not base64!"}`)
	}))
	defer bad.Close()
	_, err = tokenSourceFor(bad.Client(), bad.URL, "w", kp).FetchToken(context.Background())
	if err == nil || !strings.Contains(err.Error(), "malformed nonce") {
		t.Fatalf("malformed nonce: got %v", err)
	}

	gone := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := gone.URL
	gone.Close()
	_, err = tokenSourceFor(&http.Client{Timeout: time.Second}, url, "w", kp).FetchToken(context.Background())
	if err == nil {
		t.Fatal("a closed server should fail the handshake")
	}
}
