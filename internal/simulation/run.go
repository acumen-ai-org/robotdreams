package simulation

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/dashboard"
	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/internal/server/api"
)

type Options struct {
	Addr       string
	Seed       int64
	Speed      float64
	History    time.Duration
	ReportsDir string
	Universe   string
	Out        io.Writer
	Now        func() time.Time
}

type HistoryStats struct {
	Instances int
	Events    int
	Scopes    int
}

type Sim struct {
	opts Options
	co   *Company

	srv            *server.Server
	httpSrv        *http.Server
	ln             net.Listener
	served         chan error
	dataDir        string
	cancelRequests context.CancelFunc

	api        *httpAPI
	workers    map[string]*simWorker
	gen        *Generator
	rng        *rand.Rand
	adminToken string
	stats      HistoryStats

	pingPong []*pingPongApp

	closeOnce sync.Once
	closeErr  error
}

const (
	defaultAddr       = "127.0.0.1:0"
	defaultSeed       = 1
	defaultSpeed      = 1.0
	defaultHistory    = 48 * time.Hour
	defaultReportsDir = "reporting/library"
	defaultUniverse   = "spookify"
	adminTokenTTL     = 24 * time.Hour
)

func Start(ctx context.Context, opts Options) (*Sim, error) {
	if opts.Addr == "" {
		opts.Addr = defaultAddr
	}
	if opts.Seed == 0 {
		opts.Seed = defaultSeed
	}
	if opts.Speed <= 0 {
		opts.Speed = defaultSpeed
	}
	if opts.History <= 0 {
		opts.History = defaultHistory
	}
	if opts.ReportsDir == "" {
		opts.ReportsDir = defaultReportsDir
	}
	if opts.Universe == "" {
		opts.Universe = defaultUniverse
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}

	co, err := LoadCompany(opts.Universe)
	if err != nil {
		return nil, err
	}

	if fi, err := os.Stat(opts.ReportsDir); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("report library %q not found: run `dream simulate` from the repository root, or point --reports at a report definition library directory (e.g. reporting/library)", opts.ReportsDir)
	}

	dataDir, err := os.MkdirTemp("", "dream-simulate-")
	if err != nil {
		return nil, fmt.Errorf("create temp data dir: %w", err)
	}

	s := &Sim{
		opts:    opts,
		co:      co,
		dataDir: dataDir,
		workers: make(map[string]*simWorker),
		rng:     rand.New(rand.NewSource(opts.Seed + 2)),
	}

	srv, err := server.New(server.Config{
		DataDir:    dataDir,
		Addr:       opts.Addr,
		ReportsDir: opts.ReportsDir,
	})
	if err != nil {
		os.RemoveAll(dataDir)
		return nil, err
	}
	s.srv = srv

	ln, err := net.Listen("tcp", srv.Addr())
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("listen on %s: %w", srv.Addr(), err)
	}
	s.ln = ln

	mux := http.NewServeMux()
	mux.Handle("/dashboard/", dashboard.Mount("/dashboard", ""))
	mux.Handle("/", api.NewRouter(srv))
	reqCtx, cancelRequests := context.WithCancel(context.Background())
	s.cancelRequests = cancelRequests
	s.httpSrv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		BaseContext:       func(net.Listener) context.Context { return reqCtx },
	}
	s.served = make(chan error, 1)
	go func() {
		err := s.httpSrv.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		s.served <- err
	}()

	admin, err := srv.MintAdminToken("simulate-dashboard", adminTokenTTL)
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("mint dashboard admin token: %w", err)
	}
	s.adminToken = admin.Raw

	s.api = newHTTPAPI("http://" + ln.Addr().String())

	if err := s.connectCompany(ctx); err != nil {
		s.Close()
		return nil, err
	}
	s.declareFleetVersions(ctx)
	s.startPingPong(ctx)

	s.gen = NewGenerator(co, opts.Seed, opts.Now(), opts.History)
	if err := s.replayHistory(ctx); err != nil {
		s.Close()
		return nil, err
	}

	return s, nil
}

func (s *Sim) Company() *Company { return s.co }

func (s *Sim) BaseURL() string { return s.api.baseURL }

func (s *Sim) DashboardURL() string { return s.BaseURL() + "/dashboard/" }

func (s *Sim) AdminToken() string { return s.adminToken }

func (s *Sim) Stats() HistoryStats { return s.stats }

func (s *Sim) Close() error {
	s.closeOnce.Do(func() { s.closeErr = s.close() })
	return s.closeErr
}

func (s *Sim) close() error {
	var errs []error
	for _, a := range s.pingPong {
		errs = append(errs, a.close())
	}
	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), closeGrace)
		shutdownErr := make(chan error, 1)
		go func() { shutdownErr <- s.httpSrv.Shutdown(ctx) }()
		select {
		case err := <-shutdownErr:
			errs = append(errs, err)
		case <-time.After(closeDrain):
			s.cancelRequests()
			errs = append(errs, <-shutdownErr)
		}
		cancel()
		errs = append(errs, <-s.served)
	} else if s.ln != nil {
		errs = append(errs, s.ln.Close())
	}
	if s.cancelRequests != nil {
		s.cancelRequests()
	}
	if s.srv != nil {
		errs = append(errs, s.srv.Close())
	}
	if s.dataDir != "" {
		errs = append(errs, os.RemoveAll(s.dataDir))
	}
	return errors.Join(errs...)
}

const (
	closeGrace = 5 * time.Second
	closeDrain = 500 * time.Millisecond
)

func (s *Sim) connectCompany(ctx context.Context) error {
	specs := s.co.Workers()
	for _, spec := range specs {
		w, err := connectWorker(ctx, s.api, s.srv.EnrollmentToken(), spec)
		if err != nil {
			return err
		}
		s.workers[spec.ID] = w
	}
	fmt.Fprintf(s.opts.Out, "connected %d simulated workers (%d divisions, %d departments, %d teams, %d FTE modelled)\n",
		len(specs), len(s.co.Divisions()), len(s.co.Departments()), len(s.co.Teams), s.co.Headcount())
	return nil
}

func (s *Sim) workerFor(producer string) (*simWorker, error) {
	w, ok := s.workers[producer]
	if !ok {
		return nil, fmt.Errorf("no connected worker %q", producer)
	}
	return w, nil
}

const (
	replayShards         = 16
	replayShardQueue     = 64
	replayProgressStride = 1000
)

func (s *Sim) replayHistory(ctx context.Context) error {
	subs := s.gen.History()
	scopes := map[string]bool{}
	total := 0
	for _, sub := range subs {
		if sub.Instance != nil {
			total++
			scopes[sub.Instance.Scope] = true
			s.stats.Instances++
			s.stats.Events += len(sub.Instance.Events)
		} else if sub.Events != nil {
			scopes[sub.Events.Scope] = true
			s.stats.Events += len(sub.Events.Events)
		}
	}
	s.stats.Scopes = len(scopes)

	chans := make([]chan Submission, replayShards)
	errCh := make(chan error, replayShards)
	var wg sync.WaitGroup
	var done atomic.Int64
	for i := range chans {
		chans[i] = make(chan Submission, replayShardQueue)
		wg.Add(1)
		go func(ch <-chan Submission) {
			defer wg.Done()
			for sub := range ch {
				if err := ctx.Err(); err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}
				if err := s.submit(ctx, sub); err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}
				if sub.Instance != nil {
					if n := done.Add(1); n%replayProgressStride == 0 {
						fmt.Fprintf(s.opts.Out, "history: %d/%d instances submitted...\n", n, total)
					}
				}
			}
		}(chans[i])
	}

	shardFor := func(scope string) int {
		h := fnv.New32a()
		h.Write([]byte(scope))
		return int(h.Sum32() % replayShards)
	}
dispatch:
	for _, sub := range subs {
		scope := ""
		if sub.Instance != nil {
			scope = sub.Instance.Scope
		} else if sub.Events != nil {
			scope = sub.Events.Scope
		}
		select {
		case chans[shardFor(scope)] <- sub:
		case err := <-errCh:
			for _, ch := range chans {
				close(ch)
			}
			wg.Wait()
			return err
		case <-ctx.Done():
			break dispatch
		}
	}
	for _, ch := range chans {
		close(ch)
	}
	wg.Wait()
	select {
	case err := <-errCh:
		return err
	default:
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	fmt.Fprintf(s.opts.Out, "history: %d instances, %d events across %d scopes\n",
		s.stats.Instances, s.stats.Events, s.stats.Scopes)
	return nil
}

func (s *Sim) submit(ctx context.Context, sub Submission) error {
	w, err := s.workerFor(sub.Producer)
	if err != nil {
		return err
	}
	if sub.Instance != nil {
		err = w.postInstance(ctx, sub.Instance)
	} else if sub.Events != nil {
		err = w.postEvents(ctx, sub.Events)
	}
	if errorIsGeneratorBug(err) {
		return fmt.Errorf("GENERATOR BUG: server rejected a generated payload as invalid (fix the generator, do not ignore this): %w", err)
	}
	return err
}

func (s *Sim) submitLive(ctx context.Context, sub Submission) error {
	err := s.submit(ctx, sub)
	if err == nil || errors.Is(err, context.Canceled) {
		return nil
	}
	if errorIsGeneratorBug(err) {
		return err
	}
	fmt.Fprintf(s.opts.Out, "live: submission failed (continuing): %v\n", err)
	return nil
}

func errorIsGeneratorBug(err error) bool {
	var ae *apiError
	return errors.As(err, &ae) && ae.Status == http.StatusBadRequest
}

type liveIncidentStage int

const (
	liveIncidentOpened liveIncidentStage = iota
	liveIncidentMitigated
)

type liveIncident struct {
	inc    *incident
	stage  liveIncidentStage
	nextAt time.Time
}

const (
	liveTickMinSeconds       = 3
	liveTickMaxSeconds       = 8
	liveInstanceEvery        = 45 * time.Second
	liveAnnounceAfter        = 30 * time.Second
	liveFirstIncidentMinSecs = 120
	liveFirstIncidentMaxSecs = 240
	liveIncidentGapMinSecs   = 150
	liveIncidentGapMaxSecs   = 300
	liveIncidentStageMinSecs = 20
	liveIncidentStageMaxSecs = 40
	liveAboveTeamRefreshRate = 0.15
)

var livePulseWeights = []weighted{
	{"activity", 40}, {"logs", 30}, {"delivery", 20}, {"performance", 10},
}

func (s *Sim) RunLive(ctx context.Context) error {
	speed := s.opts.Speed
	scale := func(d time.Duration) time.Duration { return time.Duration(float64(d) / speed) }
	randomSeconds := func(lo, hi float64) time.Duration {
		return time.Duration(s.gen.between(lo, hi)) * time.Second
	}

	now := time.Now().UTC()
	nextInstance := now.Add(scale(liveInstanceEvery))
	nextIncident := now.Add(scale(randomSeconds(liveFirstIncidentMinSecs, liveFirstIncidentMaxSecs)))
	announceAt := now.Add(scale(liveAnnounceAfter))
	announced := false
	var open []*liveIncident

	for {
		tick := scale(time.Duration(s.gen.between(liveTickMinSeconds, liveTickMaxSeconds) * float64(time.Second)))
		timer := time.NewTimer(tick)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		now = time.Now().UTC()

		tm := s.co.Teams[s.rng.Intn(len(s.co.Teams))]
		def := s.pickProducedDefinition(tm, livePulseWeights)
		batch := s.gen.pulseBatch(def, tm, now)
		if err := s.submitLive(ctx, Submission{Producer: tm.ProducerID(), Events: &batch}); err != nil {
			return err
		}

		if !announced && now.After(announceAt) {
			announced = true
			if err := s.announceFleetUpdate(ctx); err != nil {
				fmt.Fprintf(s.opts.Out, "warning: %v\n", err)
			}
		}

		if now.After(nextInstance) {
			nextInstance = now.Add(scale(liveInstanceEvery))
			if err := s.submitLive(ctx, s.freshInstance(now)); err != nil {
				return err
			}
		}

		remaining := open[:0]
		for _, li := range open {
			if now.Before(li.nextAt) {
				remaining = append(remaining, li)
				continue
			}
			var sub Submission
			if li.stage == liveIncidentOpened {
				li.inc.mitigated = now
				sub = s.gen.incidentInstance(li.inc, "mitigated", now)
				li.stage = liveIncidentMitigated
				li.nextAt = now.Add(scale(randomSeconds(liveIncidentStageMinSecs, liveIncidentStageMaxSecs)))
				remaining = append(remaining, li)
			} else {
				li.inc.resolved = now
				sub = s.gen.incidentInstance(li.inc, "resolved", now)
			}
			if err := s.submitLive(ctx, sub); err != nil {
				return err
			}
		}
		open = remaining

		if now.After(nextIncident) {
			nextIncident = now.Add(scale(randomSeconds(liveIncidentGapMinSecs, liveIncidentGapMaxSecs)))
			inc := s.gen.newLiveIncident(now)
			if err := s.submitLive(ctx, s.gen.incidentInstance(inc, "opened", now)); err != nil {
				return err
			}
			open = append(open, &liveIncident{
				inc:    inc,
				nextAt: now.Add(scale(randomSeconds(liveIncidentStageMinSecs, liveIncidentStageMaxSecs))),
			})
		}
	}
}

func (s *Sim) freshInstance(now time.Time) Submission {
	if s.rng.Float64() < liveAboveTeamRefreshRate {
		ds := s.co.Departments()
		d := ds[s.rng.Intn(len(ds))]
		def := DepartmentReports[s.rng.Intn(len(DepartmentReports))]
		if build, ok := scopeBuilders[def]; ok {
			return build(s.gen, d.Scope(), d.LeadID(), now)
		}
	}

	ti := s.rng.Intn(len(s.co.Teams))
	tm := s.co.Teams[ti]
	defs := tm.ReportDefinitions()
	for i := 0; i < len(defs); i++ {
		def := defs[s.rng.Intn(len(defs))]
		if build, ok := teamBuilders[def]; ok {
			return build(s.gen, ti, tm, now)
		}
	}
	return buildTeamActivity(s.gen, ti, tm, now)
}

func (s *Sim) pickProducedDefinition(tm Team, choices []weighted) string {
	var eligible []weighted
	for _, c := range choices {
		if tm.Produces(c.name) {
			eligible = append(eligible, c)
		}
	}
	if len(eligible) == 0 {
		return "activity"
	}
	return pickWeighted(s.rng, eligible)
}

type weighted struct {
	name   string
	weight int
}

func pickWeighted(rng *rand.Rand, choices []weighted) string {
	total := 0
	for _, c := range choices {
		total += c.weight
	}
	n := rng.Intn(total)
	for _, c := range choices {
		if n < c.weight {
			return c.name
		}
		n -= c.weight
	}
	return choices[len(choices)-1].name
}
