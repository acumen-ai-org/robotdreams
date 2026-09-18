package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServerInitShutdownReleasesOpenStreams(t *testing.T) {
	dataDir := t.TempDir()

	token, err := mintLocalAdminToken(dataDir)
	if err != nil {
		t.Fatalf("mintLocalAdminToken: %v", err)
	}

	listening := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- runServerInit(ctx, serverInitOptions{
			DataDir:     dataDir,
			Addr:        "127.0.0.1:0",
			NoDashboard: true,
			onListen:    func(addr string) { listening <- addr },
		}, io.Discard)
	}()

	var addr string
	select {
	case addr = <-listening:
	case err := <-done:
		t.Fatalf("runServerInit returned before listening: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("server never started listening")
	}

	var bodies []io.ReadCloser
	for _, path := range []string{"/api/events", "/api/messages/subscribe"} {
		req, err := http.NewRequest(http.MethodGet, "http://"+addr+path, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: status %d", path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
			t.Fatalf("GET %s: Content-Type %q, want text/event-stream", path, ct)
		}
		bodies = append(bodies, resp.Body)
	}
	defer func() {
		for _, b := range bodies {
			b.Close()
		}
	}()

	started := time.Now()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runServerInit: %v", err)
		}
	case <-time.After(shutdownGrace):
		t.Fatalf("runServerInit did not return within the %s grace period", shutdownGrace)
	}
	if elapsed := time.Since(started); elapsed > shutdownGrace/2 {
		t.Fatalf("shutdown took %s with streams open; want well under the %s grace", elapsed, shutdownGrace)
	}

	for i, b := range bodies {
		readDone := make(chan struct{})
		go func() {
			defer close(readDone)
			_, _ = io.ReadAll(b)
		}()
		select {
		case <-readDone:
		case <-time.After(5 * time.Second):
			t.Fatalf("stream %d still open after shutdown", i)
		}
	}
}

func TestListenAddrIsLoopback(t *testing.T) {
	loopback := []string{
		"127.0.0.1:7420", "127.0.0.1:0", "127.1.2.3:7420", "[::1]:7420", "[::1%lo]:7420",
		"localhost:7420", "LOCALHOST:7420", " 127.0.0.1:7420 ",
	}
	for _, addr := range loopback {
		got, err := listenAddrIsLoopback(addr)
		if err != nil || !got {
			t.Errorf("listenAddrIsLoopback(%q) = %v, %v; want true, nil", addr, got, err)
		}
	}
	reachable := []string{
		":7420", "0.0.0.0:7420", "[::]:7420", "10.0.0.5:7420", "192.168.1.2:0",
		"[fe80::1]:7420", "[2001:db8::1]:7420", "example.com:7420", "myhost:7420",
	}
	for _, addr := range reachable {
		got, err := listenAddrIsLoopback(addr)
		if err != nil || got {
			t.Errorf("listenAddrIsLoopback(%q) = %v, %v; want false, nil", addr, got, err)
		}
	}
	for _, addr := range []string{"", "7420", "127.0.0.1", "http://127.0.0.1:7420"} {
		if _, err := listenAddrIsLoopback(addr); err == nil {
			t.Errorf("listenAddrIsLoopback(%q) = nil error, want a parse error", addr)
		}
	}
}

func TestCheckTransportSecurity(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)

	cases := []struct {
		name          string
		addr          string
		cert, key     string
		insecure      bool
		wantTLS       bool
		wantErr       bool
		wantErrSubstr string
	}{
		{name: "loopback plaintext", addr: "127.0.0.1:7420"},
		{name: "default addr plaintext", addr: defaultListenAddr},
		{name: "localhost plaintext", addr: "localhost:7420"},
		{name: "all interfaces plaintext refused", addr: ":7420", wantErr: true, wantErrSubstr: "--tls-cert"},
		{name: "lan ip plaintext refused", addr: "10.0.0.5:7420", wantErr: true, wantErrSubstr: "reachable from other machines"},
		{name: "all interfaces with --insecure", addr: ":7420", insecure: true},
		{name: "all interfaces with tls", addr: ":7420", cert: certFile, key: keyFile, wantTLS: true},
		{name: "loopback with tls", addr: "127.0.0.1:7420", cert: certFile, key: keyFile, wantTLS: true},
		{name: "cert without key", addr: ":7420", cert: certFile, wantErr: true, wantErrSubstr: "together"},
		{name: "key without cert", addr: ":7420", key: keyFile, wantErr: true, wantErrSubstr: "together"},
		{name: "missing cert file", addr: ":7420", cert: certFile + ".missing", key: keyFile, wantErr: true, wantErrSubstr: "--tls-cert"},
		{name: "mismatched pair", addr: ":7420", cert: certFile, key: certFile, wantErr: true, wantErrSubstr: "--tls-cert/--tls-key"},
		{name: "unparseable addr", addr: "nonsense", wantErr: true, wantErrSubstr: "--addr"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useTLS, err := checkTransportSecurity(tc.addr, tc.cert, tc.key, tc.insecure)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("got nil error, want one containing %q", tc.wantErrSubstr)
				}
				if !strings.Contains(err.Error(), tc.wantErrSubstr) {
					t.Fatalf("error %q does not contain %q", err, tc.wantErrSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if useTLS != tc.wantTLS {
				t.Fatalf("useTLS = %v, want %v", useTLS, tc.wantTLS)
			}
		})
	}
}

func TestServerInitRefusesPlaintextOnReachableAddr(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "never-created")
	err := runServerInit(context.Background(), serverInitOptions{
		DataDir:     dataDir,
		Addr:        "0.0.0.0:0",
		NoDashboard: true,
		onListen:    func(string) { t.Fatal("listener opened despite the refusal") },
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "--insecure") {
		t.Fatalf("runServerInit on 0.0.0.0 without TLS: err = %v, want the plaintext refusal", err)
	}
	if _, statErr := os.Stat(dataDir); !os.IsNotExist(statErr) {
		t.Fatalf("data dir %s was created before the refusal (stat err %v)", dataDir, statErr)
	}
}

func TestServerInitServesTLS(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	dataDir := t.TempDir()

	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("read cert: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certPEM) {
		t.Fatal("could not add the test certificate to a pool")
	}

	listening := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var banner bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- runServerInit(ctx, serverInitOptions{
			DataDir:     dataDir,
			Addr:        "127.0.0.1:0",
			NoDashboard: true,
			TLSCert:     certFile,
			TLSKey:      keyFile,
			onListen:    func(addr string) { listening <- addr },
		}, &banner)
	}()
	var addr string
	select {
	case addr = <-listening:
	case err := <-done:
		t.Fatalf("runServerInit returned before listening: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("server never started listening")
	}

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	resp, err := client.Get("https://" + addr + "/api/.well-known/jwks.json")
	if err != nil {
		t.Fatalf("GET over TLS: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET jwks over TLS: status %d", resp.StatusCode)
	}
	if resp.TLS == nil || resp.TLS.Version < tls.VersionTLS12 {
		t.Fatalf("response TLS state = %+v, want a TLS 1.2+ connection", resp.TLS)
	}

	plain := &http.Client{Timeout: 5 * time.Second}
	if resp, err := plain.Get("http://" + addr + "/api/.well-known/jwks.json"); err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Fatal("plaintext GET succeeded on a TLS listener")
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runServerInit: %v", err)
		}
	case <-time.After(shutdownGrace):
		t.Fatal("runServerInit did not shut down")
	}
	if out := banner.String(); !strings.Contains(out, "DREAM_URL=https://"+addr) || !strings.Contains(out, "(TLS)") {
		t.Fatalf("banner does not advertise the https:// URL:\n%s", out)
	}
}

func TestServerRotateEnrollmentCommand(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := mintLocalAdminToken(dataDir); err != nil {
		t.Fatalf("mintLocalAdminToken: %v", err)
	}
	tokenPath := filepath.Join(dataDir, "enrollment.token")
	before, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("read initial token: %v", err)
	}

	run := func(args ...string) (string, error) {
		root := newRootCmd()
		var buf bytes.Buffer
		root.SetOut(&buf)
		root.SetErr(&buf)
		root.SetArgs(append([]string{"server", "rotate-enrollment"}, args...))
		err := root.Execute()
		return buf.String(), err
	}

	out, err := run("--data-dir", dataDir)
	if err != nil {
		t.Fatalf("rotate-enrollment: %v\n%s", err, out)
	}
	after, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("read rotated token: %v", err)
	}
	if bytes.Equal(before, after) {
		t.Fatal("token file unchanged after rotate-enrollment")
	}
	if !strings.Contains(out, tokenPath) {
		t.Errorf("output does not name the token path:\n%s", out)
	}
	if strings.Contains(out, strings.TrimSpace(string(after))) {
		t.Errorf("output leaks the token value without --show-token:\n%s", out)
	}

	out, err = run("--data-dir", dataDir, "--show-token")
	if err != nil {
		t.Fatalf("rotate-enrollment --show-token: %v\n%s", err, out)
	}
	shown, _ := os.ReadFile(tokenPath)
	if bytes.Equal(shown, after) {
		t.Fatal("second rotation did not change the token")
	}
	if !strings.Contains(out, strings.TrimSpace(string(shown))) {
		t.Errorf("--show-token output does not contain the new value:\n%s", out)
	}

	if out, err := run("--data-dir", filepath.Join(t.TempDir(), "empty")); err == nil {
		t.Fatalf("rotate-enrollment on an uninitialized dir succeeded:\n%s", out)
	}
}

func TestServerGuideHidesTokenByDefault(t *testing.T) {
	dataDir := t.TempDir()
	tokenPath := filepath.Join(dataDir, "enrollment.token")
	if err := os.WriteFile(tokenPath, []byte("guide-secret-value\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"server", "guide", "--data-dir", dataDir, "--server", "127.0.0.1:7420"})
	if err := root.Execute(); err != nil {
		t.Fatalf("server guide: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "guide-secret-value") {
		t.Errorf("guide printed the enrollment token without --show-token:\n%s", out)
	}
	for _, want := range []string{tokenPath, "--show-token", "rotate-enrollment", "--tls-cert", "--insecure"} {
		if !strings.Contains(out, want) {
			t.Errorf("guide output missing %q", want)
		}
	}
}

func writeTestCertificate(t *testing.T) (certFile, keyFile string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "dream test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certFile, keyFile
}
