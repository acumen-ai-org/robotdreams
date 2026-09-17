package simulation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/identity"
	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

const (
	httpTimeout        = 15 * time.Second
	maxErrorBodyBytes  = 8 << 10
	tokenRefreshMargin = 2 * time.Minute
)

type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string { return fmt.Sprintf("http %d: %s", e.Status, e.Body) }

type httpAPI struct {
	baseURL string
	hc      *http.Client
}

func newHTTPAPI(baseURL string) *httpAPI {
	return &httpAPI{baseURL: baseURL, hc: &http.Client{Timeout: httpTimeout}}
}

func (a *httpAPI) postJSON(ctx context.Context, path, token string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return &apiError{Status: resp.StatusCode, Body: string(b)}
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

func (a *httpAPI) challenge(ctx context.Context, workerID string) (raw []byte, b64 string, err error) {
	var out challengeReply
	if err := a.postJSON(ctx, "/api/workers/challenge", "", map[string]string{"worker_id": workerID}, &out); err != nil {
		return nil, "", err
	}
	raw, err = base64.StdEncoding.DecodeString(out.Nonce)
	if err != nil {
		return nil, "", fmt.Errorf("malformed nonce: %w", err)
	}
	return raw, out.Nonce, nil
}

type simWorker struct {
	id  string
	api *httpAPI
	kp  identity.KeyPair

	mu  sync.Mutex
	tok security.Token
}

func connectWorker(ctx context.Context, api *httpAPI, enrollToken string, spec WorkerSpec) (*simWorker, error) {
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("generate keypair for %s: %w", spec.ID, err)
	}
	nonce, nonceB64, err := api.challenge(ctx, spec.ID)
	if err != nil {
		return nil, fmt.Errorf("challenge for %s: %w", spec.ID, err)
	}
	sig := kp.SignNonce(nonce)

	var out tokenReply
	err = api.postJSON(ctx, "/api/workers", enrollToken, map[string]any{
		"worker_id":  spec.ID,
		"role":       spec.Role,
		"reports_to": spec.ReportsTo,
		"public_key": base64.StdEncoding.EncodeToString(kp.PublicKey),
		"nonce":      nonceB64,
		"signature":  base64.StdEncoding.EncodeToString(sig),
		"metadata":   map[string]string{"scope": spec.Scope, "simulated": "true"},
	}, &out)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", spec.ID, err)
	}

	return &simWorker{
		id:  spec.ID,
		api: api,
		kp:  kp,
		tok: security.Token{Raw: out.Token, ExpiresAt: out.ExpiresAt, IssuedAt: time.Now()},
	}, nil
}

func (w *simWorker) token(ctx context.Context) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if time.Now().Add(tokenRefreshMargin).Before(w.tok.ExpiresAt) {
		return w.tok.Raw, nil
	}
	nonce, nonceB64, err := w.api.challenge(ctx, w.id)
	if err != nil {
		return "", fmt.Errorf("refresh challenge for %s: %w", w.id, err)
	}
	sig := w.kp.SignNonce(nonce)
	var out tokenReply
	err = w.api.postJSON(ctx, "/api/token", "", map[string]string{
		"worker_id": w.id,
		"nonce":     nonceB64,
		"signature": base64.StdEncoding.EncodeToString(sig),
	}, &out)
	if err != nil {
		return "", fmt.Errorf("refresh token for %s: %w", w.id, err)
	}
	w.tok = security.Token{Raw: out.Token, ExpiresAt: out.ExpiresAt, IssuedAt: time.Now()}
	return w.tok.Raw, nil
}

func (w *simWorker) postInstance(ctx context.Context, inst *reporting.Instance) error {
	tok, err := w.token(ctx)
	if err != nil {
		return err
	}
	return w.api.postJSON(ctx, "/api/reports/instances", tok, inst, nil)
}

func (w *simWorker) postUpdateReport(ctx context.Context, body map[string]string) error {
	tok, err := w.token(ctx)
	if err != nil {
		return err
	}
	return w.api.postJSON(ctx, "/api/updates/reports", tok, body, nil)
}

func (w *simWorker) postApp(ctx context.Context, body map[string]string) error {
	tok, err := w.token(ctx)
	if err != nil {
		return err
	}
	return w.api.postJSON(ctx, "/api/apps", tok, body, nil)
}

func (w *simWorker) emitMessage(ctx context.Context, body map[string]any) (string, error) {
	tok, err := w.token(ctx)
	if err != nil {
		return "", err
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := w.api.postJSON(ctx, "/api/messages", tok, body, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

func (w *simWorker) ackMessage(ctx context.Context, id, action string) error {
	tok, err := w.token(ctx)
	if err != nil {
		return err
	}
	return w.api.postJSON(ctx, "/api/messages/"+id+"/ack", tok, map[string]string{"action": action}, nil)
}

func (w *simWorker) postEvents(ctx context.Context, batch *EventBatch) error {
	tok, err := w.token(ctx)
	if err != nil {
		return err
	}
	return w.api.postJSON(ctx, "/api/reports/events", tok, batch, nil)
}
