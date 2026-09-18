package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLatest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/proj/releases/latest" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != "2022-11-28" {
			t.Errorf("missing API version header, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v0.4.2"})
	}))
	defer ts.Close()

	g := &GitHub{Owner: "acme", Repo: "proj", APIBase: ts.URL}
	got, err := g.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if got != "0.4.2" {
		t.Fatalf("Latest = %q, want %q with the leading v stripped", got, "0.4.2")
	}
}

func TestLatestPrivateRepoHint(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	g := &GitHub{Owner: "acme", Repo: "proj", APIBase: ts.URL}
	_, err := g.Latest(context.Background())
	if err == nil {
		t.Fatal("Latest succeeded against a 404")
	}
	if !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Fatalf("error does not mention the token hint: %v", err)
	}
}

func TestDownloadAssetAnonymousUsesPublicPath(t *testing.T) {
	const payload = "archive-bytes"
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Header.Get("Authorization") != "" {
			t.Error("anonymous download sent an Authorization header")
		}
		_, _ = w.Write([]byte(payload))
	}))
	defer ts.Close()

	g := &GitHub{Owner: "acme", Repo: "proj", DownloadBase: ts.URL}
	var buf bytes.Buffer
	sum, err := g.DownloadAsset(context.Background(), "0.4.2", "dream_0.4.2_linux_amd64.tar.gz", &buf)
	if err != nil {
		t.Fatalf("DownloadAsset: %v", err)
	}
	if buf.String() != payload {
		t.Fatalf("downloaded %q, want %q", buf.String(), payload)
	}

	if err := VerifySHA256(sum, sha256Hex(payload)); err != nil {
		t.Fatalf("digest mismatch: %v", err)
	}
	want := "/acme/proj/releases/download/v0.4.2/dream_0.4.2_linux_amd64.tar.gz"
	if gotPath != want {
		t.Fatalf("path = %q, want %q", gotPath, want)
	}
}

func TestDownloadAssetWithTokenGoesThroughTheAPI(t *testing.T) {
	const payload = "archive-bytes"
	var assetFetched bool

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q, want a bearer token", got)
		}
		switch r.URL.Path {
		case "/repos/acme/proj/releases/tags/v0.4.2":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"assets": []map[string]any{
					{"id": 111, "name": "other.tar.gz"},
					{"id": 222, "name": "dream_0.4.2_linux_amd64.tar.gz"},
				},
			})
		case "/repos/acme/proj/releases/assets/222":
			assetFetched = true
			_, _ = w.Write([]byte(payload))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	g := &GitHub{Owner: "acme", Repo: "proj", APIBase: ts.URL, Token: "secret"}
	var buf bytes.Buffer
	if _, err := g.DownloadAsset(context.Background(), "0.4.2", "dream_0.4.2_linux_amd64.tar.gz", &buf); err != nil {
		t.Fatalf("DownloadAsset: %v", err)
	}
	if !assetFetched {
		t.Fatal("the asset was never fetched by ID")
	}
	if buf.String() != payload {
		t.Fatalf("downloaded %q, want %q", buf.String(), payload)
	}
}

func TestDownloadAssetMissingFromRelease(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"assets": []map[string]any{{"id": 1, "name": "something-else.tar.gz"}},
		})
	}))
	defer ts.Close()

	g := &GitHub{Owner: "acme", Repo: "proj", APIBase: ts.URL, Token: "secret"}
	var buf bytes.Buffer
	_, err := g.DownloadAsset(context.Background(), "0.4.2", "dream_0.4.2_linux_amd64.tar.gz", &buf)
	if err == nil {
		t.Fatal("DownloadAsset succeeded for an absent asset")
	}
}

func TestChecksums(t *testing.T) {
	const body = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08  dream_0.4.2_linux_amd64.tar.gz\n"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/checksums.txt") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	g := &GitHub{Owner: "acme", Repo: "proj", DownloadBase: ts.URL}
	got, err := g.Checksums(context.Background(), "0.4.2")
	if err != nil {
		t.Fatalf("Checksums: %v", err)
	}
	sum, err := ChecksumFor(got, "dream_0.4.2_linux_amd64.tar.gz")
	if err != nil {
		t.Fatalf("ChecksumFor: %v", err)
	}
	if sum != "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08" {
		t.Fatalf("checksum = %q", sum)
	}
}

func TestDownloadAssetPropagatesHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	g := &GitHub{Owner: "acme", Repo: "proj", DownloadBase: ts.URL}
	var buf bytes.Buffer
	if _, err := g.DownloadAsset(context.Background(), "0.4.2", "x.tar.gz", &buf); err == nil {
		t.Fatal("DownloadAsset succeeded against a 500")
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
