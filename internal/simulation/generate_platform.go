package simulation

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
)

const (
	catalogDailyDeliveries = 120_000
	catalogSizeTracks      = 105_000_000
	licensingCompanyMGEUR  = 250_000_000
	companyMarkets         = 184

	mlReadoutRowCap = 10

	secDetectionTeamBacklogShrink = 0.18
	secAgeBucketDays              = 30
)

func init() {
	registerTeam("catalog", everyDay, buildCatalog)
	registerTeam("licensing", everyWeek, buildLicensing)
	registerTeam("editorial", everyDay, buildEditorial)
	registerTeam("data-pipeline", hourly, buildDataPipeline)
	registerTeam("ml-experiment", everyDay, buildMLExperiment)
	registerTeam("security-posture", everyDay, buildSecurityPosture)
}

func (g *Generator) platformShare(tm Team, def string) float64 {
	total := 0
	for _, t := range g.co.Teams {
		if t.Produces(def) {
			total += t.Headcount
		}
	}
	if total == 0 {
		return 1
	}
	return float64(tm.Headcount) / float64(total)
}

func platformLocal(t time.Time, tm Team) time.Time {
	return t.Add(time.Duration(regionOffsetHours[tm.Region] * float64(time.Hour)))
}

type dataset struct {
	name string
	lag  float64
	sla  float64
}

func hasDataset(sets []dataset, name string) bool {
	for _, d := range sets {
		if d.name == name {
			return true
		}
	}
	return false
}

var (
	catalogMusicSources = []string{
		"Meridian Music Group", "Halcyon Records", "Northstar Music",
		"Coalition", "Ampersand", "DropTrack", "Indiewire Distro", "Tapehead",
	}
	catalogBookSources = []string{
		"Lanternhouse Audio", "Quill & Ear", "Marginalia Audio",
		"Readaloud", "Foxglove Voices", "Brightwater Audio",
	}
	catalogTakedownReasons = []string{
		"rights withdrawn by owner", "artist impersonation",
		"unlicensed sample", "duplicate release", "territory not cleared",
		"metadata dispute",
	}
	catalogTerritories = []string{"global", "US", "DE", "BR", "IN", "JP", "SE", "UK", "MX"}

	licensingCounterparties = []string{
		"major label", "indie aggregator", "publisher", "CMO/PRO", "audiobook publisher",
	}
	licensingDealStatus = []string{
		"unsigned", "in redline", "at signature", "signed",
	}
	licensingMixLabelRelations = []float64{0.44, 0.31, 0.12, 0.13, 0}
	licensingMixAudiobooks     = []float64{0.04, 0.08, 0.16, 0.04, 0.68}
	licensingMixPublishing     = []float64{0.09, 0.11, 0.44, 0.31, 0.05}

	editorialFlagships = []string{
		"Fresh Cuts Friday", "Bassline", "Now Playing", "Undergrowth",
		"Heartland Country", "Voltage", "First Listen",
	}
	editorialHubs = []string{
		"Nordic Pop", "Anime Now", "Baila Reggaeton", "Indie Grime",
		"Jazz Classics", "Lo-Fi Beats", "Afrobeats Hits",
	}
	editorialTrackTitles = []string{
		"Halvljus", "Paper Cranes", "Midnight Ferry", "Cold Rooms",
		"Blue Hour", "Sundial", "Kite Season", "Static Bloom",
		"Northbound", "Salt & Neon",
	}
	editorialArtists = []string{
		"Vera Lindh", "KOJO", "Marisol Reyes", "The Tenth Floor",
		"Ayo Bankole", "Nils Berg", "Hana Ito", "Fever Choir",
	}
	editorialSources = []string{"pitch", "editor pick", "algotorial", "label priority"}

	mlPrimaryMetrics = []string{
		"day-7 retention", "stream starts per DAU", "save rate",
		"podcast completion", "premium conversion", "ad completion rate",
		"search success rate",
	}
	mlSignificance = []string{"n.s.", "n.s.", "n.s.", "p=0.04", "p=0.02", "p<0.01"}
	mlDecisions    = []string{"ship", "iterate", "extend", "no decision"}
	mlGuardrails   = []string{
		"skip rate", "unsubscribes", "playback errors", "support contacts",
		"library saves", "ad load complaints",
	}

	secDetectionSources = []string{
		"EDR", "cloud audit log", "identity provider", "WAF",
		"SIEM correlation", "honeytoken", "third-party intel",
	}
	secSeverities  = []string{"critical", "high", "medium", "low"}
	secAgeBuckets  = []string{"0-30d", "31-60d", "61-90d", "90d+"}
	secPatchSLADay = map[string]int{"critical": 30, "high": 60, "medium": 90}
)

func buildCatalog(g *Generator, ti int, tm Team, t time.Time) Submission {
	share := g.platformShare(tm, "catalog")
	delivered := math.Round(catalogDailyDeliveries * share * g.between(0.82, 1.18) * g.season(t, tm, 0.12))
	failures := math.Round(delivered * g.between(0.004, 0.021))
	held := math.Round((delivered - failures) * g.between(0.001, 0.008))
	published := delivered - failures - held
	takedowns := math.Round(delivered * g.between(0.0004, 0.0026))
	completeness := round1(g.between(93.5, 99.4))
	rights := round1(g.between(97.4, 99.95))
	size := math.Round(catalogSizeTracks * share)

	sources := catalogMusicSources
	if tm.Division == "audiobooks" {
		sources = catalogBookSources
	}
	weights := make([]float64, len(sources))
	sum := 0.0
	for i := range sources {
		weights[i] = math.Pow(0.62, float64(i)) * g.between(0.75, 1.3)
		sum += weights[i]
	}
	srcRows := make([][]string, 0, len(sources))
	for i, s := range sources {
		srcRows = append(srcRows, []string{s, money(math.Round(delivered * weights[i] / sum))})
	}

	var events []reporting.Event
	for i := 0; i < g.intBetween(1, 3); i++ {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(30, 900)), Type: "delivery.received", Severity: reporting.SeverityInfo,
			Label: fmt.Sprintf("delivery from %s accepted", sources[g.intBetween(0, len(sources)-1)]),
		})
	}
	if failures > delivered*0.012 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(20, 600)), Type: "ingest.failed", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("%s rejected: %s", sources[g.intBetween(0, len(sources)-1)], g.llm.Cause(tm.Archetype)),
		})
	}
	tdRows := make([][]string, 0, 3)
	for i := 0; i < g.intBetween(0, 3); i++ {
		when := t.Add(-g.minutes(30, 1200))
		title := editorialTrackTitles[g.intBetween(0, len(editorialTrackTitles)-1)]
		holder := sources[g.intBetween(0, len(sources)-1)]
		terr := catalogTerritories[g.intBetween(0, len(catalogTerritories)-1)]
		reason := catalogTakedownReasons[g.intBetween(0, len(catalogTakedownReasons)-1)]
		tdRows = append(tdRows, []string{when.Format("15:04"), title, holder, terr, reason})
		events = append(events, reporting.Event{
			T: when, Type: "takedown.received", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("takedown for %q (%s): %s", title, terr, reason),
		})
	}
	if rights < 98.5 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(10, 400)), Type: "rights.gap", Severity: reporting.SeverityCritical,
			Label: fmt.Sprintf("unmatched recordings published without clear rights: %s", g.llm.Cause(ArchLicensing)),
		})
	}

	in := tm.instance("catalog", t,
		map[string]float64{
			"tracks_ingested":           delivered,
			"ingestion_failures":        failures,
			"metadata_completeness_pct": completeness,
			"takedowns":                 takedowns,
			"rights_coverage_pct":       rights,
			"catalog_size":              size,
		},
		map[string]float64{
			"metadata_completeness_pct": 98,
			"rights_coverage_pct":       99.5,
		})
	in.Series = map[string][]reporting.SeriesPoint{
		"ingest_rate": g.series(t, 8, 3*time.Hour, delivered/24, 0.35),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"ingest_stages": funnel([][2]interface{}{
			{"delivered", delivered},
			{"validated", delivered - failures},
			{"published", published},
		}),
		"tracks_by_source": {Columns: []string{"source", "tracks"}, Rows: srcRows},
		"recent_takedowns": {
			Columns: []string{"when", "title", "rights_holder", "territory", "reason"},
			Rows:    tdRows,
		},
		"tracks_by_unit": byUnit("tracks_ingested", tm, delivered),
	}
	return tm.submit(in)
}

func buildLicensing(g *Generator, ti int, tm Team, t time.Time) Submission {
	scale := float64(tm.Headcount) / 30
	deals := g.intBetween(3, 6+int(math.Round(scale*6)))
	renewals := g.intBetween(1, 3+int(math.Round(math.Min(4, scale))))
	disputes := g.intBetween(0, 2+int(math.Round(math.Min(5, scale))))
	mg := math.Round(licensingCompanyMGEUR * g.platformShare(tm, "licensing") * g.between(0.7, 1.35))
	territories := float64(g.intBetween(158, companyMarkets))

	mix := licensingMixLabelRelations
	switch {
	case tm.Division == "audiobooks":
		mix = licensingMixAudiobooks
	case tm.Name == "publishing-rights":
		mix = licensingMixPublishing
	}
	counts := make([]int, len(licensingCounterparties))
	remaining, top := deals, 0
	for i := range licensingCounterparties {
		n := int(math.Round(float64(deals) * mix[i]))
		if n > remaining {
			n = remaining
		}
		counts[i] = n
		remaining -= n
		if mix[i] > mix[top] {
			top = i
		}
	}
	counts[top] += remaining
	cpRows := make([][]string, 0, len(licensingCounterparties))
	for i, cp := range licensingCounterparties {
		cpRows = append(cpRows, []string{cp, fmt.Sprintf("%d", counts[i])})
	}

	var events []reporting.Event
	rnRows := make([][]string, 0, renewals)
	for i := 0; i < renewals; i++ {
		opened := t.Add(-time.Duration(g.intBetween(10, 80)) * 24 * time.Hour)
		cp := licensingCounterparties[0]
		roll, acc := g.rng.Float64(), 0.0
		for j, w := range mix {
			if acc += w; roll <= acc {
				cp = licensingCounterparties[j]
				break
			}
		}
		terr := catalogTerritories[g.intBetween(0, len(catalogTerritories)-1)]
		status := licensingDealStatus[g.intBetween(0, len(licensingDealStatus)-1)]
		expires := t.Add(time.Duration(g.intBetween(3, 89)) * 24 * time.Hour)
		rnRows = append(rnRows, []string{
			expires.Format("2006-01-02"), cp, terr,
			money(mg * g.between(0.02, 0.2)), status,
		})
		events = append(events, reporting.Event{
			T: opened, Type: "renewal.opened", Severity: reporting.SeverityInfo,
			Label: fmt.Sprintf("renewal notice served on a %s (%s)", cp, terr),
		})
		if status == "signed" {
			events = append(events, reporting.Event{
				T:    opened.Add(time.Duration(g.intBetween(4, 40)) * 24 * time.Hour),
				Type: "deal.signed", Severity: reporting.SeverityInfo,
				Label: fmt.Sprintf("signed %s with a %s", g.llm.Service(ArchLicensing), cp),
			})
		} else {
			events = append(events, reporting.Event{
				T: t.Add(-g.minutes(60, 2400)), Type: "renewal.due", Severity: reporting.SeverityWarn,
				Label: fmt.Sprintf("%s renewal expires %s with no signed paper", cp, expires.Format("2 Jan")),
			})
		}
	}
	for i := 0; i < disputes; i++ {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(120, 6000)), Type: "dispute.raised", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("dispute opened: %s", g.llm.Cause(ArchLicensing)),
		})
	}
	if g.rng.Float64() < 0.4 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(60, 3000)), Type: "dispute.resolved", Severity: reporting.SeverityInfo,
			Label: "dispute settled without amendment",
		})
	}
	if g.rng.Float64() < 0.25 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(30, 2000)), Type: "deal.escalated", Severity: reporting.SeverityCritical,
			Label: fmt.Sprintf("%s escalated to the licensing committee: %s",
				g.llm.Service(ArchLicensing), g.llm.Cause(ArchLicensing)),
		})
	}

	in := tm.instance("licensing", t,
		map[string]float64{
			"deals_in_negotiation":   float64(deals),
			"renewals_due_90d":       float64(renewals),
			"minimum_guarantees_eur": mg,
			"disputes_open":          float64(disputes),
			"territories_cleared":    territories,
		},
		map[string]float64{"territories_cleared": companyMarkets})
	in.Series = map[string][]reporting.SeriesPoint{
		"negotiation_pipeline": g.series(t, 8, 7*24*time.Hour, float64(deals), 0.25),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"deals_by_counterparty": {Columns: []string{"counterparty_type", "deals"}, Rows: cpRows},
		"renewals_due": {
			Columns: []string{"expires", "counterparty", "territory", "mg_eur", "status"},
			Rows:    rnRows,
		},
		"mg_bridge": bridge("last quarter", "committed", mg, [][2]interface{}{
			{"new deals", math.Round(mg * g.between(0.05, 0.25))},
			{"expiries", -math.Round(mg * g.between(0.02, 0.16))},
			{"renegotiated rates", math.Round(mg * g.between(-0.09, 0.09))},
		}),
		"deals_by_unit": byUnit("deals_in_negotiation", tm, float64(deals)),
	}
	return tm.submit(in)
}

func buildEditorial(g *Generator, ti int, tm Team, t time.Time) Submission {
	scale := float64(tm.Headcount) / 50
	lift := 1.0
	switch platformLocal(t, tm).Weekday() {
	case time.Monday:
		lift = 0.85
	case time.Wednesday:
		lift = 1.2
	case time.Thursday:
		lift = 1.5
	case time.Friday:
		lift = 1.35
	}

	refreshed := math.Round(g.between(6, 15) * scale * lift)
	filled := round1(g.between(92.5, 100))
	reviewed := math.Round(g.between(60, 150) * scale * lift)
	accepted := math.Round(reviewed * g.between(0.05, 0.14))
	shortlisted := math.Round(reviewed * g.between(0.18, 0.34))
	if shortlisted < accepted {
		shortlisted = accepted
	}
	adds := math.Round(accepted * g.between(1.3, 2.6))
	charted := math.Round(accepted * g.between(0.03, 0.18))
	share := round1(g.between(17.5, 31))

	playlists := editorialFlagships
	if tm.Name == "genre-curation" {
		playlists = editorialHubs
	}
	addRows := make([][]string, 0, len(playlists))
	weights := make([]float64, len(playlists))
	sum := 0.0
	for i := range playlists {
		weights[i] = math.Pow(0.7, float64(i)) * g.between(0.7, 1.35)
		sum += weights[i]
	}
	for i, p := range playlists {
		addRows = append(addRows, []string{p, money(math.Round(adds * weights[i] / sum))})
	}

	var events []reporting.Event
	for i := 0; i < g.intBetween(1, 3); i++ {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(30, 700)), Type: "playlist.refreshed", Severity: reporting.SeverityInfo,
			Label: fmt.Sprintf("%s refreshed", playlists[g.intBetween(0, len(playlists)-1)]),
		})
	}
	events = append(events, reporting.Event{
		T: t.Add(-g.minutes(20, 600)), Type: "pitch.accepted", Severity: reporting.SeverityInfo,
		Label: fmt.Sprintf("%s taken for %s",
			editorialArtists[g.intBetween(0, len(editorialArtists)-1)],
			playlists[g.intBetween(0, len(playlists)-1)]),
	})
	if g.rng.Float64() < 0.6 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(20, 600)), Type: "pitch.rejected", Severity: reporting.SeverityInfo,
			Label: "pitch declined: no slot in the window",
		})
	}

	openRows := make([][]string, 0, 3)
	if filled < 100 {
		for i := 0; i < g.intBetween(1, 3); i++ {
			p := playlists[g.intBetween(0, len(playlists)-1)]
			slot := t.Add(time.Duration(g.intBetween(1, 5)) * 24 * time.Hour)
			openRows = append(openRows, []string{
				p, slot.Format("2006-01-02"),
				editorialHubs[g.intBetween(0, len(editorialHubs)-1)],
				g.llm.Cause(ArchContent),
			})
			events = append(events, reporting.Event{
				T: t.Add(-g.minutes(10, 300)), Type: "slot.unfilled", Severity: reporting.SeverityWarn,
				Label: fmt.Sprintf("%s still short for %s", p, slot.Format("Mon 2 Jan")),
			})
		}
	}
	if g.rng.Float64() < 0.35 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(15, 500)), Type: "release.late", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("master not delivered in time: %s", g.llm.Cause(ArchContent)),
		})
	}

	progRows := make([][]string, 0, 5)
	for i := 0; i < g.intBetween(3, 5); i++ {
		when := t.Add(-g.minutes(30, 800))
		progRows = append(progRows, []string{
			when.Format("15:04"),
			playlists[g.intBetween(0, len(playlists)-1)],
			editorialTrackTitles[g.intBetween(0, len(editorialTrackTitles)-1)],
			editorialArtists[g.intBetween(0, len(editorialArtists)-1)],
			editorialSources[g.intBetween(0, len(editorialSources)-1)],
		})
	}

	in := tm.instance("editorial", t,
		map[string]float64{
			"playlists_refreshed": refreshed,
			"slots_filled_pct":    filled,
			"pitches_reviewed":    reviewed,
			"pitches_accepted":    accepted,
			"first_listen_adds":   adds,
			"editorial_share_pct": share,
		},
		map[string]float64{"slots_filled_pct": 100})
	in.Series = map[string][]reporting.SeriesPoint{
		"pitch_volume": g.series(t, 8, 3*time.Hour, reviewed/8, 0.4),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"pitch_stages": funnel([][2]interface{}{
			{"pitched", reviewed},
			{"shortlisted", shortlisted},
			{"programmed", accepted},
			{"charted", charted},
		}),
		"adds_by_playlist": {Columns: []string{"playlist", "first_listen_adds"}, Rows: addRows},
		"open_slots": {
			Columns: []string{"playlist", "slot_date", "genre", "blocker"},
			Rows:    openRows,
		},
		"recent_programming": {
			Columns: []string{"when", "playlist", "track", "artist", "source"},
			Rows:    progRows,
		},
		"playlists_by_unit": byUnit("playlists_refreshed", tm, refreshed),
	}
	return tm.submit(in)
}

func buildDataPipeline(g *Generator, ti int, tm Team, t time.Time) Submission {
	scale := float64(tm.Headcount) / 25
	jobs := math.Round(g.between(45, 130) * scale * g.season(t, tm, 0.2))
	failed := 0.0
	if g.rng.Float64() < 0.35 {
		failed = math.Round(g.between(1, 4))
	}
	lag := g.between(4, 48)
	dq := round1(g.between(98.2, 100))
	rows := math.Round(g.between(8e7, 6e8) * scale)

	if inc := g.incidentAt(ti, t); inc != nil {
		lag += g.between(60, 240)
		failed += math.Round(g.between(2, 7))
		dq = round1(math.Max(90, dq-g.between(1, 6)))
	}
	slaHit := round1(math.Max(84, 100-failed*g.between(0.8, 2.4)-math.Max(0, lag-60)/30))
	lag = round1(lag)

	sets := make([]dataset, 0, 6)
	want := g.intBetween(4, 6)
	for i := 0; i < want*4 && len(sets) < want; i++ {
		name := g.llm.Service(ArchData)
		if hasDataset(sets, name) {
			continue
		}
		sets = append(sets, dataset{
			name: name,
			lag:  round1(lag * g.between(0.15, 1.0)),
			sla:  float64(g.intBetween(30, 240)),
		})
	}
	sets[0].lag = lag
	sort.SliceStable(sets, func(i, j int) bool { return sets[i].lag > sets[j].lag })

	rankRows := make([][]string, 0, len(sets))
	lateRows := make([][]string, 0, len(sets))
	var events []reporting.Event
	for _, d := range sets {
		rankRows = append(rankRows, []string{d.name, money(d.lag)})
		if d.lag > d.sla {
			status := "past SLA"
			lateRows = append(lateRows, []string{
				d.name, g.llm.Owner(), money(d.lag), money(d.sla), status,
			})
			events = append(events, reporting.Event{
				T: t.Add(-g.minutes(1, 55)), Type: "freshness.breached", Severity: reporting.SeverityWarn,
				Label: fmt.Sprintf("%s is %.0fm stale against a %.0fm SLA", d.name, d.lag, d.sla),
			})
		}
	}
	if failed > 0 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(1, 55)), Type: "job.failed", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("%s failed: %s", g.llm.Component(ArchData), g.llm.Cause(ArchData)),
		})
	}
	if slaHit < 99 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(1, 50)), Type: "sla.missed", Severity: reporting.SeverityCritical,
			Label: fmt.Sprintf("%s missed its delivery window: %s", sets[0].name, g.llm.Cause(ArchData)),
		})
	}
	if g.rng.Float64() < 0.3 {
		start := t.Add(-g.minutes(90, 300))
		events = append(events, reporting.Event{
			T: start, Type: "backfill.started", Severity: reporting.SeverityInfo,
			Label: fmt.Sprintf("backfilling %s", sets[0].name),
		})
		if end := start.Add(g.minutes(40, 200)); end.Before(t) {
			events = append(events, reporting.Event{
				T: end, Type: "backfill.finished", Severity: reporting.SeverityInfo,
				Label: fmt.Sprintf("%s backfill complete", sets[0].name),
			})
		}
	}

	in := tm.instance("data-pipeline", t,
		map[string]float64{
			"jobs_run":              jobs,
			"jobs_failed":           failed,
			"freshness_lag_minutes": lag,
			"sla_hit_pct":           slaHit,
			"rows_processed":        rows,
			"dq_pass_pct":           dq,
		},
		map[string]float64{
			"sla_hit_pct":           99,
			"freshness_lag_minutes": 60,
			"dq_pass_pct":           99,
		})
	in.Series = map[string][]reporting.SeriesPoint{
		"freshness_lag": g.series(t, 8, 10*time.Minute, lag, 0.3),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"lateness_by_dataset": {Columns: []string{"dataset", "lag_minutes"}, Rows: rankRows},
		"late_datasets": {
			Columns: []string{"dataset", "owner", "lag_minutes", "sla_minutes", "status"},
			Rows:    lateRows,
		},
		"lag_by_unit": byUnit("freshness_lag_minutes", tm, lag),
	}
	return tm.submit(in)
}

func buildMLExperiment(g *Generator, ti int, tm Team, t time.Time) Submission {
	scale := float64(tm.Headcount) / 25
	running := g.intBetween(2, 3+int(math.Round(scale*4)))
	exposure := round1(g.between(3.5, 22))
	conclusive := round1(g.between(16, 42))
	shipped := math.Round(float64(running) * g.between(0.05, 0.3))
	breaches := 0.0
	if g.rng.Float64() < 0.18 {
		breaches = math.Round(g.between(1, 2))
	}

	started := math.Round(float64(running) * g.between(2.4, 4.2))
	enrolled := math.Round(started * g.between(0.6, 0.85))
	concluded := math.Round(enrolled * conclusive / 100)
	shippedWindow := math.Round(math.Min(concluded, shipped+g.between(0, 2)))

	shown := running
	if shown > mlReadoutRowCap {
		shown = mlReadoutRowCap
	}
	rows := make([][]string, 0, shown)
	best := 0.0
	for i := 0; i < shown; i++ {
		name := fmt.Sprintf("%s %s", g.llm.Service(ArchResearch), g.llm.Version())
		metric := mlPrimaryMetrics[g.intBetween(0, len(mlPrimaryMetrics)-1)]
		sig := mlSignificance[g.intBetween(0, len(mlSignificance)-1)]
		lift := round1(g.between(-1.4, 2.2))
		decision := "no decision"
		if sig != "n.s." {
			lift = round1(g.between(-2.6, 4.6))
			if lift < 0 {
				decision = "roll back"
			} else {
				decision = mlDecisions[g.intBetween(0, len(mlDecisions)-1)]
				if lift > best {
					best = lift
				}
			}
		}
		rows = append(rows, []string{name, metric, fmt.Sprintf("%+.1f%%", lift), sig, decision})
	}
	if best == 0 {
		best = round1(g.between(0.3, 1.4))
	}

	var events []reporting.Event
	for i := 0; i < g.intBetween(1, 3); i++ {
		start := t.Add(-time.Duration(g.intBetween(3, 24)) * 24 * time.Hour)
		events = append(events, reporting.Event{
			T: start, Type: "experiment.started", Severity: reporting.SeverityInfo,
			Label: fmt.Sprintf("started %s on %s",
				g.llm.Service(ArchResearch),
				mlPrimaryMetrics[g.intBetween(0, len(mlPrimaryMetrics)-1)]),
		})
		if end := start.Add(time.Duration(g.intBetween(7, 21)) * 24 * time.Hour); end.Before(t) {
			events = append(events, reporting.Event{
				T: end, Type: "experiment.concluded", Severity: reporting.SeverityInfo,
				Label: "readout: no significant movement on the primary metric",
			})
		}
	}
	if shipped > 0 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(60, 900)), Type: "experiment.shipped", Severity: reporting.SeverityInfo,
			Label: fmt.Sprintf("shipped %s to 100%%", g.llm.Service(ArchResearch)),
		})
	}
	if g.rng.Float64() < 0.25 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(60, 1200)), Type: "experiment.stopped", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("stopped early: %s", g.llm.Cause(ArchResearch)),
		})
	}
	guardRows := make([][]string, 0, 2)
	for i := 0; i < int(breaches); i++ {
		gm := mlGuardrails[g.intBetween(0, len(mlGuardrails)-1)]
		exp := g.llm.Service(ArchResearch)
		move := round1(g.between(0.6, 3.8))
		guardRows = append(guardRows, []string{exp, gm, fmt.Sprintf("+%.1f%%", move), "rolled back"})
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(30, 800)), Type: "guardrail.breached", Severity: reporting.SeverityCritical,
			Label: fmt.Sprintf("%s moved %s by +%.1f%%", exp, gm, move),
		})
	}

	in := tm.instance("ml-experiment", t,
		map[string]float64{
			"experiments_running": float64(running),
			"exposure_pct":        exposure,
			"conclusive_pct":      conclusive,
			"best_lift_pct":       best,
			"experiments_shipped": shipped,
			"guardrail_breaches":  breaches,
		},
		map[string]float64{"guardrail_breaches": 0})
	in.Series = map[string][]reporting.SeriesPoint{
		"exposure_trend": g.series(t, 8, 3*time.Hour, exposure, 0.15),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"experiment_stages": funnel([][2]interface{}{
			{"exposed", started},
			{"enrolled", enrolled},
			{"conclusive", concluded},
			{"shipped", shippedWindow},
		}),
		"experiments": {
			Columns: []string{"experiment", "primary_metric", "lift_pct", "significance", "decision"},
			Rows:    rows,
		},
		"guardrails": {
			Columns: []string{"experiment", "guardrail", "movement", "action"},
			Rows:    guardRows,
		},
		"experiments_by_unit": byUnit("experiments_running", tm, float64(running)),
	}
	return tm.submit(in)
}

func buildSecurityPosture(g *Generator, ti int, tm Team, t time.Time) Submission {
	appsec := tm.Name == "application-security"
	scale := float64(tm.Headcount) / 60

	cells := map[string][]float64{}
	shrink := 1.0
	if !appsec {
		shrink = secDetectionTeamBacklogShrink
	}
	for _, sev := range secSeverities {
		var base float64
		switch sev {
		case "critical":
			base = g.between(1, 6)
		case "high":
			base = g.between(8, 26)
		case "medium":
			base = g.between(25, 70)
		default:
			base = g.between(40, 110)
		}
		base *= scale * shrink
		row := make([]float64, len(secAgeBuckets))
		for i := range secAgeBuckets {
			row[i] = math.Round(base * math.Pow(0.55, float64(i)) * g.between(0.6, 1.4))
		}
		sla, hasSLA := secPatchSLADay[sev]
		if hasSLA {
			firstBreach := sla / secAgeBucketDays
			for i := firstBreach; i < len(row); i++ {
				if g.rng.Float64() < 0.85 {
					row[i] = 0
				} else {
					row[i] = math.Round(g.between(1, 3))
				}
			}
		}
		cells[sev] = row
	}

	heatRows := make([][]string, 0, len(secSeverities)*len(secAgeBuckets))
	open, breached := 0.0, 0.0
	for _, sev := range secSeverities {
		row := cells[sev]
		sla, hasSLA := secPatchSLADay[sev]
		for i, bucket := range secAgeBuckets {
			heatRows = append(heatRows, []string{sev, bucket, money(row[i])})
			open += row[i]
			if hasSLA && i >= sla/secAgeBucketDays {
				breached += row[i]
			}
		}
	}

	var mttr, patch, detections, falsePos float64
	if appsec {
		mttr = round1(g.between(11, 33))
		patch = round1(g.between(91.5, 99.2))
		detections = math.Round(g.between(2, 14))
		falsePos = round1(g.between(6, 18))
	} else {
		mttr = round1(g.between(1.5, 8))
		patch = round1(g.between(94.5, 99.6))
		detections = math.Round(g.between(420, 1500) * scale * g.season(t, tm, 0.15))
		falsePos = round1(g.between(34, 66))
	}

	var events []reporting.Event
	for i := 0; i < g.intBetween(1, 3); i++ {
		opened := t.Add(-time.Duration(g.intBetween(1, 20)) * 24 * time.Hour)
		events = append(events, reporting.Event{
			T: opened, Type: "finding.opened", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("%s: %s", g.llm.Service(ArchSecurity), g.llm.Cause(ArchSecurity)),
		})
		if closed := opened.Add(time.Duration(g.intBetween(2, 25)) * 24 * time.Hour); closed.Before(t) {
			events = append(events, reporting.Event{
				T: closed, Type: "finding.closed", Severity: reporting.SeverityInfo,
				Label: "patched and verified in production",
			})
		}
	}

	openRows := make([][]string, 0, 4)
	for _, sev := range secSeverities {
		sla, hasSLA := secPatchSLADay[sev]
		if !hasSLA {
			continue
		}
		row := cells[sev]
		for i := sla / secAgeBucketDays; i < len(row); i++ {
			if row[i] == 0 {
				continue
			}
			age := g.intBetween(sla+1, sla+90)
			openRows = append(openRows, []string{
				sev, g.llm.Service(ArchSecurity), fmt.Sprintf("%d", age),
				g.llm.Owner(), fmt.Sprintf("%d days overdue", age-sla),
			})
			events = append(events, reporting.Event{
				T: t.Add(-g.minutes(20, 700)), Type: "sla.breached", Severity: reporting.SeverityCritical,
				Label: fmt.Sprintf("%s finding %d days old against a %d-day SLA", sev, age, sla),
			})
		}
	}

	srcRows := make([][]string, 0, len(secDetectionSources))
	if !appsec {
		weights := make([]float64, len(secDetectionSources))
		sum := 0.0
		for i := range secDetectionSources {
			weights[i] = math.Pow(0.66, float64(i)) * g.between(0.7, 1.3)
			sum += weights[i]
		}
		for i, s := range secDetectionSources {
			srcRows = append(srcRows, []string{s, money(math.Round(detections * weights[i] / sum))})
		}
	}
	if detections > 20 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(10, 600)), Type: "detection.escalated", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("%s escalated from %s", g.llm.Service(ArchSecurity),
				secDetectionSources[g.intBetween(0, len(secDetectionSources)-1)]),
		})
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(10, 600)), Type: "detection.dismissed", Severity: reporting.SeverityInfo,
			Label: "alert closed as expected administrative activity",
		})
	}

	in := tm.instance("security-posture", t,
		map[string]float64{
			"findings_open":         open,
			"findings_sla_breached": breached,
			"mttr_days":             mttr,
			"patch_coverage_pct":    patch,
			"detections_triaged":    detections,
			"false_positive_pct":    falsePos,
		},
		map[string]float64{
			"findings_sla_breached": 0,
			"patch_coverage_pct":    95,
		})
	in.Series = map[string][]reporting.SeriesPoint{
		"findings_trend": g.series(t, 8, 3*time.Hour, open, 0.08),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"findings_heat":     {Columns: []string{"row", "column", "value"}, Rows: heatRows},
		"open_findings":     {Columns: []string{"severity", "component", "age_days", "owner", "due"}, Rows: openRows},
		"detection_sources": {Columns: []string{"source", "detections"}, Rows: srcRows},
		"findings_by_unit":  byUnit("findings_open", tm, open),
	}
	return tm.submit(in)
}
