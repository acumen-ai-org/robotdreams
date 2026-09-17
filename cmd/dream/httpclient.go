package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/identity"
	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

const (
	configFileName = "config.json"
	requestTimeout = 30 * time.Second
)

type localConfig struct {
	ServerAddr string `json:"server_addr"`
	WorkerID   string `json:"worker_id"`
	Role       string `json:"role,omitempty"`
	ReportsTo  string `json:"reports_to,omitempty"`
}

var unsafeServerIDChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func serverIDFromAddr(addr string) string {
	s := strings.TrimSpace(addr)
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimRight(s, "/")
	s = unsafeServerIDChars.ReplaceAllString(s, "_")
	return s
}

func baseURLFromAddr(addr string) string {
	s := strings.TrimRight(strings.TrimSpace(addr), "/")
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s
	}
	return "http://" + s
}

func dreamDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".dream"), nil
}

func configPathFor(serverID string) (string, error) {
	root, err := dreamDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, serverID, configFileName), nil
}

func saveLocalConfig(serverID string, cfg localConfig) error {
	path, err := configPathFor(serverID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func loadLocalConfig(serverID string) (localConfig, error) {
	path, err := configPathFor(serverID)
	if err != nil {
		return localConfig{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return localConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg localConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return localConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

func listLocalServerIDs() ([]string, error) {
	root, err := dreamDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", root, err)
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, e.Name(), configFileName)); err == nil {
			ids = append(ids, e.Name())
		}
	}
	return ids, nil
}

type clientTarget struct {
	serverID   string
	serverAddr string
	baseURL    string
	workerID   string
	keyPath    string
	keyPair    identity.KeyPair
}

func resolveTarget(serverFlag, serverIDFlag, workerIDFlag string) (*clientTarget, error) {
	if serverFlag == "" && serverIDFlag == "" {
		serverFlag = dreamURLFromEnv()
	}
	serverID := serverIDFlag
	if serverID == "" && serverFlag != "" {
		serverID = serverIDFromAddr(serverFlag)
	}
	if serverID == "" {
		ids, err := listLocalServerIDs()
		if err != nil {
			return nil, err
		}
		switch len(ids) {
		case 1:
			serverID = ids[0]
		case 0:
			return nil, fmt.Errorf("no connected server found under ~/.dream; run `dream worker connect --server <host:port> ...` first, or pass --server")
		default:
			return nil, fmt.Errorf("several connected servers found under ~/.dream (%s); pass --server <host:port> or --server-id <id> to choose one", strings.Join(ids, ", "))
		}
	}

	cfg, cfgErr := loadLocalConfig(serverID)

	addr := serverFlag
	if addr == "" {
		addr = cfg.ServerAddr
	}
	if addr == "" {
		return nil, fmt.Errorf("no server address known for %q: %w", serverID, cfgErr)
	}

	workerID := workerIDFlag
	if workerID == "" {
		workerID = cfg.WorkerID
	}
	if workerID == "" {
		return nil, fmt.Errorf("no worker id known for %q; pass --worker-id (config: %v)", serverID, cfgErr)
	}

	keyPath, err := identity.DefaultKeyPath(serverID)
	if err != nil {
		return nil, err
	}
	kp, err := identity.LoadKeyPair(keyPath)
	if err != nil {
		return nil, fmt.Errorf("load local identity for %q: %w", serverID, err)
	}

	return &clientTarget{
		serverID:   serverID,
		serverAddr: addr,
		baseURL:    baseURLFromAddr(addr),
		workerID:   workerID,
		keyPath:    keyPath,
		keyPair:    kp,
	}, nil
}

type challengeResponse struct {
	Nonce string `json:"nonce"`
}

type tokenResponse struct {
	WorkerID  string    `json:"worker_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func requestChallenge(ctx context.Context, hc *http.Client, baseURL, workerID string) (raw []byte, b64 string, err error) {
	var out challengeResponse
	if err := postJSON(ctx, hc, baseURL+"/api/workers/challenge", "", map[string]string{"worker_id": workerID}, &out); err != nil {
		return nil, "", err
	}
	raw, err = base64.StdEncoding.DecodeString(out.Nonce)
	if err != nil {
		return nil, "", fmt.Errorf("server returned a malformed nonce: %w", err)
	}
	return raw, out.Nonce, nil
}

func fetchToken(ctx context.Context, hc *http.Client, baseURL, workerID string, kp identity.KeyPair) (security.Token, error) {
	nonce, nonceB64, err := requestChallenge(ctx, hc, baseURL, workerID)
	if err != nil {
		return security.Token{}, err
	}
	sig := kp.SignNonce(nonce)

	var out tokenResponse
	err = postJSON(ctx, hc, baseURL+"/api/token", "", map[string]string{
		"worker_id": workerID,
		"nonce":     nonceB64,
		"signature": base64.StdEncoding.EncodeToString(sig),
	}, &out)
	if err != nil {
		return security.Token{}, err
	}
	return security.Token{Raw: out.Token, IssuedAt: time.Now(), ExpiresAt: out.ExpiresAt}, nil
}

func tokenSourceFor(hc *http.Client, baseURL, workerID string, kp identity.KeyPair) security.TokenSource {
	return security.TokenSourceFunc(func(ctx context.Context) (security.Token, error) {
		return fetchToken(ctx, hc, baseURL, workerID, kp)
	})
}

func (t *clientTarget) oneShotClient(ctx context.Context) (*apiClient, error) {
	hc := defaultHTTPClient()
	tok, err := fetchToken(ctx, hc, t.baseURL, t.workerID, t.keyPair)
	if err != nil {
		return nil, err
	}
	return &apiClient{baseURL: t.baseURL, token: tok.Raw, hc: hc}, nil
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: requestTimeout}
}

type apiClient struct {
	baseURL string
	token   string
	hc      *http.Client
}

type apiError struct {
	StatusCode int
	Message    string
}

func (e *apiError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("server returned %s", http.StatusText(e.StatusCode))
	}
	return fmt.Sprintf("%s (HTTP %d)", e.Message, e.StatusCode)
}

func errorFromResponse(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	var payload struct {
		Error string `json:"error"`
	}
	msg := ""
	if err := json.Unmarshal(body, &payload); err == nil && payload.Error != "" {
		msg = payload.Error
	} else {
		msg = strings.TrimSpace(string(body))
	}
	return &apiError{StatusCode: resp.StatusCode, Message: msg}
}

func (c *apiClient) do(ctx context.Context, method, path, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()
		return nil, errorFromResponse(resp)
	}
	return resp, nil
}

func (c *apiClient) doJSON(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	contentType := ""
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(b)
		contentType = "application/json"
	}
	resp, err := c.do(ctx, method, path, contentType, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func decodeJSONBody(r io.Reader, out any) error {
	if err := json.NewDecoder(r).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func postJSON(ctx context.Context, hc *http.Client, url, token string, in, out any) error {
	c := &apiClient{baseURL: "", token: token, hc: hc}
	return c.doJSON(ctx, http.MethodPost, url, in, out)
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

type storagePointer struct {
	Backend  string
	Path     string
	Revision string
}

type envelopeView struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	From        string          `json:"from"`
	To          string          `json:"to"`
	Subject     string          `json:"subject,omitempty"`
	Body        json.RawMessage `json:"body,omitempty"`
	StoragePtr  *storagePointer `json:"storage_ptr,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	CausationID string          `json:"causation_id,omitempty"`
	Delivered   bool            `json:"delivered"`
}

type objectMetaView struct {
	Path      string    `json:"path"`
	Revision  string    `json:"revision"`
	Size      int64     `json:"size"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by,omitempty"`
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format(time.RFC3339)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
