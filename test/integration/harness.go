package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/identity"
	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/internal/server/api"
	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

type testServer struct {
	Srv     *server.Server
	BaseURL string

	onConnect func(workerID string)
}

func startServer(t *testing.T, cfg server.Config) *testServer {
	t.Helper()

	if cfg.DataDir == "" {
		cfg.DataDir = t.TempDir()
	}
	srv, err := server.New(cfg)
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

	return &testServer{Srv: srv, BaseURL: "http://" + ln.Addr().String()}
}

type apiErr struct {
	Status int
	Body   string
}

func (e *apiErr) Error() string { return fmt.Sprintf("http %d: %s", e.Status, e.Body) }

func httpClient() *http.Client { return &http.Client{Timeout: 10 * time.Second} }

func postJSON(ctx context.Context, hc *http.Client, url string, in, out any) error {
	return postJSONWithToken(ctx, hc, url, "", in, out)
}

func postJSONWithToken(ctx context.Context, hc *http.Client, url, token string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return &apiErr{Status: resp.StatusCode, Body: string(b)}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func authedRequest(ctx context.Context, hc *http.Client, baseURL, token, method, path, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return hc.Do(req)
}

func authedJSON(ctx context.Context, hc *http.Client, baseURL, token, method, path string, in, out any) error {
	var body io.Reader
	contentType := ""
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
		contentType = "application/json"
	}
	resp, err := authedRequest(ctx, hc, baseURL, token, method, path, contentType, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return &apiErr{Status: resp.StatusCode, Body: string(b)}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type challengeReply struct {
	Nonce string `json:"nonce"`
}

type tokenReply struct {
	WorkerID  string    `json:"worker_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func requestChallenge(ctx context.Context, hc *http.Client, baseURL, workerID string) (raw []byte, b64 string, err error) {
	var out challengeReply
	if err := postJSON(ctx, hc, baseURL+"/api/workers/challenge", map[string]string{"worker_id": workerID}, &out); err != nil {
		return nil, "", err
	}
	raw, err = base64.StdEncoding.DecodeString(out.Nonce)
	if err != nil {
		return nil, "", fmt.Errorf("malformed nonce: %w", err)
	}
	return raw, out.Nonce, nil
}

func refreshToken(ctx context.Context, hc *http.Client, baseURL, workerID string, kp identity.KeyPair) (security.Token, error) {
	nonce, nonceB64, err := requestChallenge(ctx, hc, baseURL, workerID)
	if err != nil {
		return security.Token{}, err
	}
	sig := kp.SignNonce(nonce)

	var out tokenReply
	err = postJSON(ctx, hc, baseURL+"/api/token", map[string]string{
		"worker_id": workerID,
		"nonce":     nonceB64,
		"signature": base64.StdEncoding.EncodeToString(sig),
	}, &out)
	if err != nil {
		return security.Token{}, err
	}
	return security.Token{Raw: out.Token, ExpiresAt: out.ExpiresAt, IssuedAt: time.Now()}, nil
}

type worker struct {
	t       *testing.T
	id      string
	baseURL string
	hc      *http.Client
	kp      identity.KeyPair
	client  *security.WorkerClient
}

func connectWorker(ctx context.Context, t *testing.T, ts *testServer, id, role, reportsTo string, metadata map[string]string) *worker {
	t.Helper()

	hc := httpClient()
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate keypair for %s: %v", id, err)
	}

	nonce, nonceB64, err := requestChallenge(ctx, hc, ts.BaseURL, id)
	if err != nil {
		t.Fatalf("challenge for %s: %v", id, err)
	}
	sig := kp.SignNonce(nonce)

	connectBody := map[string]any{
		"worker_id":  id,
		"role":       role,
		"reports_to": reportsTo,
		"public_key": base64.StdEncoding.EncodeToString(kp.PublicKey),
		"nonce":      nonceB64,
		"signature":  base64.StdEncoding.EncodeToString(sig),
	}
	if len(metadata) > 0 {
		connectBody["metadata"] = metadata
	}

	enrollTok, err := ts.Srv.MintAdminToken("test-harness-enroll", time.Minute)
	if err != nil {
		t.Fatalf("mint enrollment credential for connecting %s: %v", id, err)
	}

	var out tokenReply
	if err := postJSONWithToken(ctx, hc, ts.BaseURL+"/api/workers", enrollTok.Raw, connectBody, &out); err != nil {
		t.Fatalf("connect %s: %v", id, err)
	}

	if ts.onConnect != nil {
		ts.onConnect(id)
	}

	source := security.TokenSourceFunc(func(ctx context.Context) (security.Token, error) {
		return refreshToken(ctx, hc, ts.BaseURL, id, kp)
	})

	wc := security.NewWorkerClient(kp.PrivateKey, kp.PublicKey, source, nil)
	if err := wc.Start(ctx); err != nil {
		t.Fatalf("start worker client for %s: %v", id, err)
	}
	t.Cleanup(wc.Stop)

	return &worker{t: t, id: id, baseURL: ts.BaseURL, hc: hc, kp: kp, client: wc}
}

func (w *worker) token() string {
	tok, err := w.client.Token()
	if err != nil {
		w.t.Fatalf("worker %s: no token available: %v", w.id, err)
	}
	return tok.Raw
}

type workerView struct {
	ID          string            `json:"id"`
	Role        string            `json:"role"`
	ReportsTo   string            `json:"reports_to"`
	PublicKey   string            `json:"public_key,omitempty"`
	ConnectedAt time.Time         `json:"connected_at"`
	LastSeenAt  time.Time         `json:"last_seen_at"`
	Status      string            `json:"status"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type storagePointerView struct {
	Backend  string
	Path     string
	Revision string
}

type envelopeView struct {
	ID          string              `json:"id"`
	Type        string              `json:"type"`
	From        string              `json:"from"`
	To          string              `json:"to"`
	Subject     string              `json:"subject,omitempty"`
	Body        json.RawMessage     `json:"body,omitempty"`
	StoragePtr  *storagePointerView `json:"storage_ptr,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
	CausationID string              `json:"causation_id,omitempty"`
	Delivered   bool                `json:"delivered"`
}

type objectMetaView struct {
	Path      string    `json:"path"`
	Revision  string    `json:"revision"`
	Size      int64     `json:"size"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by,omitempty"`
	Category  string    `json:"category,omitempty"`
}

type bearer interface {
	authToken() string
	base() string
	httpClientOf() *http.Client
}

func (w *worker) authToken() string          { return w.token() }
func (w *worker) base() string               { return w.baseURL }
func (w *worker) httpClientOf() *http.Client { return w.hc }

type adminActor struct {
	baseURL string
	hc      *http.Client
	tok     string
}

func newAdminActor(t *testing.T, ts *testServer) *adminActor {
	t.Helper()
	tok, err := ts.Srv.MintAdminToken("test-admin", time.Minute)
	if err != nil {
		t.Fatalf("mint admin token: %v", err)
	}
	return &adminActor{baseURL: ts.BaseURL, hc: httpClient(), tok: tok.Raw}
}

func (a *adminActor) authToken() string          { return a.tok }
func (a *adminActor) base() string               { return a.baseURL }
func (a *adminActor) httpClientOf() *http.Client { return a.hc }

func listWorkers(ctx context.Context, b bearer) ([]workerView, error) {
	var out struct {
		Workers []workerView `json:"workers"`
	}
	err := authedJSON(ctx, b.httpClientOf(), b.base(), b.authToken(), http.MethodGet, "/api/workers", nil, &out)
	return out.Workers, err
}

type emitRequestBody struct {
	Type        string              `json:"type"`
	To          string              `json:"to,omitempty"`
	Subject     string              `json:"subject,omitempty"`
	Body        json.RawMessage     `json:"body,omitempty"`
	StoragePtr  *storagePointerView `json:"storage_ptr,omitempty"`
	CausationID string              `json:"causation_id,omitempty"`
}

func emit(ctx context.Context, w *worker, msgType, to, subject string, body json.RawMessage, ptr *storagePointerView, causationID string) (envelopeView, error) {
	var out envelopeView
	req := emitRequestBody{Type: msgType, To: to, Subject: subject, Body: body, StoragePtr: ptr, CausationID: causationID}
	err := authedJSON(ctx, w.hc, w.baseURL, w.token(), http.MethodPost, "/api/messages", req, &out)
	return out, err
}

func tail(ctx context.Context, b bearer, workerID string) ([]envelopeView, error) {
	var out struct {
		Messages []envelopeView `json:"messages"`
	}
	path := "/api/messages?worker_id=" + workerID
	err := authedJSON(ctx, b.httpClientOf(), b.base(), b.authToken(), http.MethodGet, path, nil, &out)
	return out.Messages, err
}

func putObject(ctx context.Context, w *worker, path string, data []byte, category, ifMatchRevision string) (objectMetaView, error) {
	q := "path=" + urlQueryEscape(path)
	if category != "" {
		q += "&category=" + urlQueryEscape(category)
	}
	if ifMatchRevision != "" {
		q += "&if_match_revision=" + urlQueryEscape(ifMatchRevision)
	}
	resp, err := authedRequest(ctx, w.hc, w.baseURL, w.token(), http.MethodPost, "/api/storage/objects?"+q, "application/octet-stream", bytes.NewReader(data))
	if err != nil {
		return objectMetaView{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return objectMetaView{}, &apiErr{Status: resp.StatusCode, Body: string(b)}
	}
	var out objectMetaView
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return objectMetaView{}, err
	}
	return out, nil
}

func getObject(ctx context.Context, b bearer, path string) ([]byte, objectMetaView, error) {
	q := "path=" + urlQueryEscape(path)
	resp, err := authedRequest(ctx, b.httpClientOf(), b.base(), b.authToken(), http.MethodGet, "/api/storage/objects?"+q, "", nil)
	if err != nil {
		return nil, objectMetaView{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return nil, objectMetaView{}, &apiErr{Status: resp.StatusCode, Body: string(body)}
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, objectMetaView{}, err
	}
	meta := objectMetaView{
		Path:      resp.Header.Get("X-Path"),
		Revision:  resp.Header.Get("X-Revision"),
		UpdatedBy: resp.Header.Get("X-Updated-By"),
		Category:  resp.Header.Get("X-Category"),
		Size:      int64(len(data)),
	}
	return data, meta, nil
}

func revokeWorker(ctx context.Context, admin *adminActor, workerID, reason string) error {
	return authedJSON(ctx, admin.hc, admin.baseURL, admin.tok, http.MethodPost, "/api/workers/"+workerID+"/revoke", map[string]string{"reason": reason}, nil)
}

func urlQueryEscape(s string) string { return url.QueryEscape(s) }
