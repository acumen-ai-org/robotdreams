package dashboard

import (
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIndexIsEmbeddedAndServed(t *testing.T) {
	srv := New("")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html prefix", ct)
	}

	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])
	if !strings.Contains(body, "Mission Control") {
		t.Fatalf("served index.html does not contain expected title text, got: %s", body)
	}
	if strings.Contains(body, apiBasePlaceholder) {
		t.Fatalf("apiBasePlaceholder was not replaced in served index.html")
	}
}

func TestIndexInjectsAPIBase(t *testing.T) {
	srv := New("http://example.invalid:9999")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])
	if !strings.Contains(body, "http://example.invalid:9999") {
		t.Fatalf("served index.html does not carry injected apiBase, got: %s", body)
	}
}

func discoverAsset(t *testing.T, suffix string) string {
	t.Helper()
	var found string
	err := fs.WalkDir(webFS, "web/dist", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if found == "" && !d.IsDir() && strings.HasSuffix(path, suffix) {
			found = strings.TrimPrefix(path, "web/dist")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded FS: %v", err)
	}
	if found == "" {
		t.Fatalf("no embedded asset ending in %q", suffix)
	}
	return found
}

func TestStaticAssetsAreServedWithCorrectContentType(t *testing.T) {
	if !Built() {
		t.Skip("only the placeholder index.html is embedded; run `make ui-build` to test the real assets")
	}
	srv := New("")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	cases := []struct {
		path       string
		wantPrefix string
	}{
		{discoverAsset(t, ".css"), "text/css"},
		{discoverAsset(t, ".js"), "text/javascript"},
		{discoverAsset(t, ".webp"), "image/webp"},
		{"/favicon.png", "image/png"},
	}
	for _, c := range cases {
		resp, err := ts.Client().Get(ts.URL + c.path)
		if err != nil {
			t.Fatalf("GET %s: %v", c.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("GET %s: status %d, want 200", c.path, resp.StatusCode)
		}
		ct := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, c.wantPrefix) && !strings.Contains(ct, "javascript") && !strings.Contains(ct, "css") {
			t.Fatalf("GET %s: Content-Type = %q, want prefix %q", c.path, ct, c.wantPrefix)
		}
	}
}

func TestMountStripsPrefix(t *testing.T) {
	h := Mount("/dashboard", "")
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/dashboard/")
	if err != nil {
		t.Fatalf("GET /dashboard/: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestPlaceholderOrBuild(t *testing.T) {
	for _, root := range []string{"web/placeholder", siteRoot()} {
		raw, err := fs.ReadFile(webFS, root+"/index.html")
		if err != nil {
			t.Fatalf("read embedded %s/index.html: %v", root, err)
		}
		if !strings.Contains(string(raw), apiBasePlaceholder) {
			t.Fatalf("embedded %s/index.html lacks the %s marker", root, apiBasePlaceholder)
		}
	}

	hasAssets := false
	_ = fs.WalkDir(webFS, "web", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(path, "web/dist/assets/") {
			hasAssets = true
		}
		return nil
	})
	if Built() != hasAssets {
		t.Fatalf("Built() = %v but embedded dist/assets/ present = %v", Built(), hasAssets)
	}
	if !Built() {
		t.Logf("placeholder dashboard embedded (run `make ui-build` for the real one)")
	}
}
