package simulation

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	subjectPing = "ping"
	subjectPong = "pong"

	pingMessageType = "request_for_input"
	pongMessageType = "completed_work"

	reconnectDelay       = time.Second
	pingPongCloseTimeout = 2 * time.Second
	subscribeLineBuffer  = 64 << 10
	subscribeLineMax     = 1 << 20
)

type pingPongApp struct {
	node        string
	peer        string
	label       string
	sendSubject string
	sendType    string
	worker      *simWorker
	base        string
	out         io.Writer

	ln  net.Listener
	srv *http.Server
	url string

	mu     sync.Mutex
	pings  int
	pongs  int
	lastAt time.Time
}

type envelope struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	From    string          `json:"from"`
	To      string          `json:"to"`
	Subject string          `json:"subject"`
	Body    json.RawMessage `json:"body"`
}

func newPingPongApp(node, peer, label, sendSubject, sendType string, w *simWorker, base string, out io.Writer) (*pingPongApp, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("ping-pong: listen for %s: %w", node, err)
	}

	a := &pingPongApp{
		node: node, peer: peer, label: label,
		sendSubject: sendSubject, sendType: sendType,
		worker: w, base: base, out: out,
		ln:  ln,
		url: "http://" + ln.Addr().String(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", a.servePage)
	mux.HandleFunc("GET /count", a.serveCount)
	mux.HandleFunc("POST /send", a.serveSend)
	a.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	go func() { _ = a.srv.Serve(ln) }()
	return a, nil
}

func (a *pingPongApp) URL() string { return a.url }

func (a *pingPongApp) close() error {
	ctx, cancel := context.WithTimeout(context.Background(), pingPongCloseTimeout)
	defer cancel()
	return a.srv.Shutdown(ctx)
}

func (a *pingPongApp) counts() (int, int, time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pings, a.pongs, a.lastAt
}

func (a *pingPongApp) listen(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		a.subscribeUntilStreamEnds(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectDelay):
		}
	}
}

func (a *pingPongApp) subscribeUntilStreamEnds(ctx context.Context) {
	tok, err := a.worker.token(ctx)
	if err != nil {
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		a.base+"/api/messages/subscribe?worker_id="+a.node, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, subscribeLineBuffer), subscribeLineMax)
	for sc.Scan() {
		payload, ok := strings.CutPrefix(sc.Text(), "data:")
		if !ok {
			continue
		}
		var env envelope
		if err := json.Unmarshal([]byte(strings.TrimSpace(payload)), &env); err != nil {
			continue
		}
		a.countAndAck(ctx, env)
	}
}

func (a *pingPongApp) countAndAck(ctx context.Context, env envelope) {
	switch env.Subject {
	case subjectPing:
		a.bump(&a.pings)
	case subjectPong:
		a.bump(&a.pongs)
	default:
		return
	}

	_ = a.worker.ackMessage(ctx, env.ID, "handled")
}

func (a *pingPongApp) bump(n *int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	*n++
	a.lastAt = time.Now()
}

func (a *pingPongApp) sendToPeer(ctx context.Context) (string, error) {
	return a.worker.emitMessage(ctx, map[string]any{
		"type":    a.sendType,
		"to":      a.peer,
		"subject": a.sendSubject,
		"body":    json.RawMessage(`{"from":"` + a.node + `"}`),
	})
}

func (a *pingPongApp) serveSend(w http.ResponseWriter, r *http.Request) {
	id, err := a.sendToPeer(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"sent": a.sendSubject, "to": a.peer, "id": id})
}

func (a *pingPongApp) serveCount(w http.ResponseWriter, r *http.Request) {
	pings, pongs, last := a.counts()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"node": a.node, "peer": a.peer, "label": a.label,
		"pings": pings, "pongs": pongs,
		"last": last.Format(time.RFC3339),
	})
}

func (a *pingPongApp) servePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintf(w, pingPongHTML,
		html.EscapeString(a.label), html.EscapeString(a.node),
		html.EscapeString(a.peer), html.EscapeString(a.sendSubject))
}

const pingPongHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>%[1]s — Robot Dreams demo app</title>
<style>
  :root { color-scheme: light dark; }
  body {
    margin: 0; padding: 20px;
    font: 14px/1.5 ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif;
    background: Canvas; color: CanvasText;
  }
  h1 { font-size: 1rem; margin: 0 0 2px; }
  .who { font-size: 0.75rem; opacity: 0.7; margin: 0 0 18px; font-family: ui-monospace, monospace; }
  .counts { display: flex; gap: 12px; flex-wrap: wrap; }
  .card {
    flex: 1 1 120px; padding: 14px 16px; border: 1px solid rgba(128,128,128,0.35);
    border-radius: 10px;
  }
  .n { font-size: 2rem; font-weight: 700; font-variant-numeric: tabular-nums; }
  .k { font-size: 0.6875rem; text-transform: uppercase; letter-spacing: 0.05em; opacity: 0.7; }
  .foot { margin-top: 4px; font-size: 0.6875rem; opacity: 0.6; }
  .send {
    margin-top: 18px; padding: 10px 18px; font: inherit; font-weight: 700;
    border: 1px solid rgba(128,128,128,0.4); border-radius: 8px;
    background: Canvas; color: CanvasText; cursor: pointer;
  }
  .send:hover { border-color: currentColor; }
  .send:disabled { opacity: 0.5; cursor: default; }
  .sent { font-size: 0.75rem; opacity: 0.7; margin-left: 8px; }
</style>
</head>
<body>
  <h1>%[1]s</h1>
  <p class="who">%[2]s &harr; %[3]s</p>
  <div class="counts">
    <div class="card"><div class="n" id="pings">0</div><div class="k">pings received</div></div>
    <div class="card"><div class="n" id="pongs">0</div><div class="k">pongs received</div></div>
  </div>
  <p><button id="send" class="send">Send %[4]s</button> <span id="sent" class="sent"></span></p>
  <p class="foot" id="foot">nothing is on a timer &mdash; press the button</p>
<script>
async function tick() {
  try {
    const r = await fetch("./count", { cache: "no-store" });
    const d = await r.json();
    document.getElementById("pings").textContent = d.pings;
    document.getElementById("pongs").textContent = d.pongs;
  } catch (e) {
    document.getElementById("foot").textContent = "cannot reach the app: " + e;
  }
}
tick();
setInterval(tick, 1000);

const btn = document.getElementById("send");
btn.addEventListener("click", async () => {
  btn.disabled = true;
  try {
    const r = await fetch("./send", { method: "POST" });
    const d = await r.json();
    document.getElementById("sent").textContent =
      r.ok ? "sent " + d.sent + " to " + d.to : "failed: " + (d.error || r.status);
  } catch (e) {
    document.getElementById("sent").textContent = "failed: " + e;
  } finally {
    btn.disabled = false;
  }
});
</script>
</body>
</html>
`

func (s *Sim) startPingPong(ctx context.Context) {
	a, b, ok := s.firstTwoLeads()
	if !ok {
		return
	}

	ping, err := newPingPongApp(a, b, "Ping", subjectPing, pingMessageType, s.workers[a], s.BaseURL(), s.opts.Out)
	if err != nil {
		fmt.Fprintf(s.opts.Out, "warning: %v\n", err)
		return
	}
	pong, err := newPingPongApp(b, a, "Pong", subjectPong, pongMessageType, s.workers[b], s.BaseURL(), s.opts.Out)
	if err != nil {
		fmt.Fprintf(s.opts.Out, "warning: %v\n", err)
		_ = ping.close()
		return
	}
	s.pingPong = []*pingPongApp{ping, pong}

	for _, app := range s.pingPong {
		if err := app.worker.postApp(ctx, map[string]string{
			"url": app.url,
			"description": "Demo app: press the button to message the other node, " +
				"and watch what this one receives.",
		}); err != nil {
			fmt.Fprintf(s.opts.Out, "warning: %s could not register its demo app: %v\n", app.node, err)
		}
		go app.listen(ctx)
	}
}

func (s *Sim) firstTwoLeads() (string, string, bool) {
	var leads []string
	for _, spec := range s.co.Workers() {
		if spec.Role == "lead" {
			leads = append(leads, spec.ID)
		}
		if len(leads) == 2 {
			return leads[0], leads[1], true
		}
	}
	return "", "", false
}

func (s *Sim) PingPongApps() []struct{ Node, Label, URL string } {
	out := make([]struct{ Node, Label, URL string }, 0, len(s.pingPong))
	for _, a := range s.pingPong {
		out = append(out, struct{ Node, Label, URL string }{a.node, a.label, a.url})
	}
	return out
}
