package simulation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestSimulationEndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sim, err := Start(ctx, Options{
		Addr:       "127.0.0.1:0",
		Seed:       1,
		History:    2 * time.Hour,
		ReportsDir: "../../reporting/library",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sim.Close()

	get := func(path string, out any) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, sim.BaseURL()+path, nil)
		if err != nil {
			t.Fatalf("build request %s: %v", path, err)
		}
		req.Header.Set("Authorization", "Bearer "+sim.AdminToken())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: http %d: %s", path, resp.StatusCode, body)
		}
		if err := json.Unmarshal(body, out); err != nil {
			t.Fatalf("GET %s: decode: %v", path, err)
		}
	}

	var scopes struct {
		Scopes []struct {
			Path      string `json:"path"`
			Depth     int    `json:"depth"`
			Instances int    `json:"instances"`
		} `json:"scopes"`
	}
	get("/api/reports/scopes", &scopes)
	if got, want := len(scopes.Scopes), sim.Company().ScopeCount(); got != want {
		t.Errorf("scopes: got %d, want %d (%+v)", got, want, scopes.Scopes)
	}
	for _, sc := range scopes.Scopes {
		if sc.Instances == 0 {
			t.Errorf("scope %s has no instances", sc.Path)
		}
	}

	var summary struct {
		Scope string `json:"scope"`
		Tiles []struct {
			Definition string `json:"definition"`
			Status     string `json:"status"`
		} `json:"tiles"`
	}
	get("/api/reports/summary?scope="+sim.Company().Name, &summary)
	if len(summary.Tiles) < 5 {
		t.Fatalf("summary at %s: got %d tiles, want >= 5 (%+v)", sim.Company().Name, len(summary.Tiles), summary.Tiles)
	}
	statuses := map[string]bool{}
	for _, tile := range summary.Tiles {
		statuses[tile.Status] = true
	}
	if len(statuses) < 2 {
		t.Errorf("summary statuses not mixed: %+v", summary.Tiles)
	}
	if !statuses["critical"] {
		t.Errorf("no critical tile at boot (forced critical team should guarantee one): %+v", summary.Tiles)
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, sim.DashboardURL(), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET dashboard: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("dashboard: http %d", resp.StatusCode)
	}
}
