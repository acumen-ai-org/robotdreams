package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const maxArchiveBytes = 128 << 20

const githubTimeout = 30 * time.Second

type GitHub struct {
	Owner        string
	Repo         string
	HC           *http.Client
	Token        string
	APIBase      string
	DownloadBase string
}

func NewGitHub() *GitHub {
	tok := os.Getenv("GITHUB_TOKEN")
	if tok == "" {
		tok = os.Getenv("GH_TOKEN")
	}
	return &GitHub{Owner: DefaultOwner, Repo: DefaultRepo, Token: tok}
}

func (g *GitHub) client() *http.Client {
	if g.HC != nil {
		return g.HC
	}
	return http.DefaultClient
}

func (g *GitHub) apiBase() string {
	if g.APIBase != "" {
		return g.APIBase
	}
	return "https://api.github.com"
}

func (g *GitHub) downloadBase() string {
	if g.DownloadBase != "" {
		return g.DownloadBase
	}
	return "https://github.com"
}

func (g *GitHub) setHeaders(req *http.Request, accept string) {
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "dream-selfupdate")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
}

func (g *GitHub) Latest(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, githubTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", g.apiBase(), g.Owner, g.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("selfupdate: build request: %w", err)
	}
	g.setHeaders(req, "application/vnd.github+json")

	resp, err := g.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("selfupdate: query latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound && g.Token == "" {
		return "", fmt.Errorf("selfupdate: no latest release found for %s/%s "+
			"(if the repository is private, set GITHUB_TOKEN or GH_TOKEN)", g.Owner, g.Repo)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("selfupdate: query latest release: HTTP %d", resp.StatusCode)
	}

	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return "", fmt.Errorf("selfupdate: decode release: %w", err)
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("selfupdate: latest release has no tag name")
	}
	return TrimVersionPrefix(rel.TagName), nil
}

func (g *GitHub) DownloadAsset(ctx context.Context, version, name string, w io.Writer) (string, error) {
	url, accept, err := g.assetURL(ctx, version, name)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("selfupdate: build request: %w", err)
	}

	g.setHeaders(req, accept)

	resp, err := g.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("selfupdate: download %s: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("selfupdate: download %s: HTTP %d", name, resp.StatusCode)
	}

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(resp.Body, maxArchiveBytes+1))
	if err != nil {
		return "", fmt.Errorf("selfupdate: read %s: %w", name, err)
	}
	if n > maxArchiveBytes {
		return "", fmt.Errorf("selfupdate: %s exceeds %d bytes", name, maxArchiveBytes)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (g *GitHub) Checksums(ctx context.Context, version string) (string, error) {
	var sb strings.Builder
	if _, err := g.DownloadAsset(ctx, version, checksumsAsset, &sb); err != nil {
		return "", err
	}
	return sb.String(), nil
}

func (g *GitHub) assetURL(ctx context.Context, version, name string) (url, accept string, err error) {
	if g.Token == "" {
		return fmt.Sprintf("%s/%s/%s/releases/download/v%s/%s",
				g.downloadBase(), g.Owner, g.Repo, TrimVersionPrefix(version), name),
			"application/octet-stream", nil
	}

	id, err := g.assetID(ctx, version, name)
	if err != nil {
		return "", "", err
	}
	return fmt.Sprintf("%s/repos/%s/%s/releases/assets/%d", g.apiBase(), g.Owner, g.Repo, id),
		"application/octet-stream", nil
}

func (g *GitHub) assetID(ctx context.Context, version, name string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, githubTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/repos/%s/%s/releases/tags/v%s",
		g.apiBase(), g.Owner, g.Repo, TrimVersionPrefix(version))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("selfupdate: build request: %w", err)
	}
	g.setHeaders(req, "application/vnd.github+json")

	resp, err := g.client().Do(req)
	if err != nil {
		return 0, fmt.Errorf("selfupdate: look up release v%s: %w", version, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("selfupdate: look up release v%s: HTTP %d", version, resp.StatusCode)
	}

	var rel struct {
		Assets []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return 0, fmt.Errorf("selfupdate: decode release: %w", err)
	}
	for _, a := range rel.Assets {
		if a.Name == name {
			return a.ID, nil
		}
	}
	return 0, fmt.Errorf("selfupdate: release v%s has no asset named %q", version, name)
}
