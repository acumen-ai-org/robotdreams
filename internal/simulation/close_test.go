package simulation

import (
	"bufio"
	"context"
	"net/http"
	"testing"
	"time"
)

func TestCloseReleasesOpenEventStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sim, err := Start(ctx, Options{
		Addr: "127.0.0.1:0", Seed: 1, History: time.Hour,
		ReportsDir: "../../reporting/library", Universe: "spookify",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	closed := false
	defer func() {
		if !closed {
			sim.Close()
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sim.BaseURL()+"/api/events", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+sim.AdminToken())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/events: http %d", resp.StatusCode)
	}
	if _, err := bufio.NewReader(resp.Body).ReadString('\n'); err != nil {
		t.Fatalf("read first stream line: %v", err)
	}

	start := time.Now()
	err = sim.Close()
	closed = true
	took := time.Since(start)
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if took >= closeGrace {
		t.Fatalf("Close took %v with an open event stream; want under the %v grace", took, closeGrace)
	}
}
