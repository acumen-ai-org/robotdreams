package simulation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func getJSON(ctx context.Context, t *testing.T, url string, out any) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request %s: %v", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: http %d: %s", url, resp.StatusCode, body)
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			t.Fatalf("GET %s: decode: %v", url, err)
		}
	}
}

type ppCount struct {
	Node  string `json:"node"`
	Peer  string `json:"peer"`
	Label string `json:"label"`
	Pings int    `json:"pings"`
	Pongs int    `json:"pongs"`
}

func TestPingPongExchangesRealMessages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	sim, err := Start(ctx, Options{
		Addr: "127.0.0.1:0", Seed: 1, History: time.Hour,
		ReportsDir: "../../reporting/library", Universe: "spookify",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sim.Close()

	apps := sim.PingPongApps()
	if len(apps) != 2 {
		t.Fatalf("got %d demo apps, want 2", len(apps))
	}

	var ping, pong ppCount
	getJSON(ctx, t, apps[0].URL+"/count", &ping)
	getJSON(ctx, t, apps[1].URL+"/count", &pong)
	if ping.Pings+ping.Pongs+pong.Pings+pong.Pongs != 0 {
		t.Fatalf("counters moved before any button was pressed: %+v %+v", ping, pong)
	}

	const presses = 3
	for i := 0; i < presses; i++ {
		press(ctx, t, apps[0].URL)
		press(ctx, t, apps[1].URL)
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		getJSON(ctx, t, apps[0].URL+"/count", &ping)
		getJSON(ctx, t, apps[1].URL+"/count", &pong)
		if ping.Pongs >= presses && pong.Pings >= presses {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	if pong.Pings != presses {
		t.Errorf("%s received %d pings, want the %d that were sent", pong.Node, pong.Pings, presses)
	}
	if ping.Pongs != presses {
		t.Errorf("%s received %d pongs, want the %d that were sent", ping.Node, ping.Pongs, presses)
	}

	if ping.Pings != 0 {
		t.Errorf("%s counted %d pings; only the other side should receive those", ping.Node, ping.Pings)
	}
	if pong.Pongs != 0 {
		t.Errorf("%s counted %d pongs; only the other side should receive those", pong.Node, pong.Pongs)
	}

	if ping.Peer != pong.Node || pong.Peer != ping.Node {
		t.Fatalf("the pair is not mutual: %s→%s and %s→%s", ping.Node, ping.Peer, pong.Node, pong.Peer)
	}
}

func press(ctx context.Context, t *testing.T, appURL string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, appURL+"/send", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s/send: %v", appURL, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s/send: http %d: %s", appURL, resp.StatusCode, body)
	}
}

func TestPingPongRegistersItsApps(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sim, err := Start(ctx, Options{
		Addr: "127.0.0.1:0", Seed: 1, History: time.Hour,
		ReportsDir: "../../reporting/library", Universe: "spookify",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sim.Close()

	var out struct {
		Apps []struct {
			WorkerID string `json:"worker_id"`
			URL      string `json:"url"`
		} `json:"apps"`
	}
	simGet(ctx, t, sim, "/api/apps", &out)

	byWorker := map[string]string{}
	for _, a := range out.Apps {
		byWorker[a.WorkerID] = a.URL
	}
	for _, a := range sim.PingPongApps() {
		got := byWorker[a.Node]
		if got != a.URL {
			t.Errorf("%s advertises %q, want the app actually being served at %q", a.Node, got, a.URL)
		}
	}
}

func TestPingPongPagesRender(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sim, err := Start(ctx, Options{
		Addr: "127.0.0.1:0", Seed: 1, History: time.Hour,
		ReportsDir: "../../reporting/library", Universe: "spookify",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sim.Close()

	for _, a := range sim.PingPongApps() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", a.URL, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		page := string(body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: http %d", a.URL, resp.StatusCode)
		}
		if !contains(page, a.Label) || !contains(page, a.Node) {
			t.Errorf("%s page does not name itself or its node", a.Node)
		}
		if !contains(page, "./send") {
			t.Errorf("%s page has no send button; the exchange is driven by pressing one", a.Node)
		}
		for _, bad := range []string{"http://", "https://", "//cdn", "googleapis"} {
			if contains(page, bad) {
				t.Errorf("%s page references %q — it must be fully self-contained", a.Node, bad)
			}
		}
		if h := resp.Header.Get("X-Frame-Options"); h != "" {
			t.Errorf("%s page sets X-Frame-Options=%q, which would refuse the preview", a.Node, h)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
