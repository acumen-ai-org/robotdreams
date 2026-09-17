package simulation

import (
	"fmt"
	"math"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
)

func init() {
	registerTeam("subscriber-growth", everyWeek, buildSubscriberGrowth)
	registerTeam("sales-pipeline", everyWeek, buildSalesPipeline)
	registerTeam("campaign", everyDay, buildCampaign)
	registerTeam("brand-reach", everyDay, buildBrandReach)
}

const (
	weeksPerMonth         = 4.345
	voluntaryChurnShare   = 0.70
	involuntaryChurnShare = 0.30

	pipelineCoverageFloor     = 3.0
	pipelineCoverageQuotaRisk = 2.4

	negativeNewsCycleChance = 0.07
)

type marketProfile struct {
	mau              float64
	premium          float64
	arpu             float64
	monthlyChurnPct  float64
	weeklyGrowthRate float64
}

var marketProfiles = map[string]marketProfile{
	"north-america":  {mau: 100e6, premium: 51e6, arpu: 7.60, monthlyChurnPct: 3.6, weeklyGrowthRate: 0.0008},
	"latin-america":  {mau: 150e6, premium: 42e6, arpu: 2.90, monthlyChurnPct: 4.6, weeklyGrowthRate: 0.0018},
	"nordics":        {mau: 32e6, premium: 21e6, arpu: 6.80, monthlyChurnPct: 2.8, weeklyGrowthRate: 0.0006},
	"growth-markets": {mau: 188e6, premium: 92e6, arpu: 5.40, monthlyChurnPct: 3.4, weeklyGrowthRate: 0.0016},
	"india":          {mau: 130e6, premium: 40e6, arpu: 1.30, monthlyChurnPct: 5.4, weeklyGrowthRate: 0.0035},
	"southeast-asia": {mau: 96e6, premium: 30e6, arpu: 1.80, monthlyChurnPct: 5.0, weeklyGrowthRate: 0.0035},
}

func fallbackMarketProfile(headcount int) marketProfile {
	return marketProfile{
		mau:     float64(headcount) * 400_000,
		premium: float64(headcount) * 150_000,
		arpu:    4.20, monthlyChurnPct: 4.2, weeklyGrowthRate: 0.0015,
	}
}

var planTiers = []struct {
	name            string
	subscriberShare float64
	arpuFactor      float64
	churnMultiplier float64
}{
	{"individual", 0.42, 1.30, 1.25},
	{"duo", 0.14, 1.05, 0.85},
	{"family", 0.32, 0.72, 0.62},
	{"student", 0.12, 0.55, 1.45},
}

func buildSubscriberGrowth(g *Generator, ti int, tm Team, t time.Time) Submission {
	p, ok := marketProfiles[tm.Name]
	if !ok {
		p = fallbackMarketProfile(tm.Headcount)
	}

	mau := p.mau * (1 + g.between(-0.012, 0.012))
	premium := p.premium * (1 + g.between(-0.010, 0.010))
	churnPct := p.monthlyChurnPct * (1 + g.between(-0.10, 0.14))
	arpu := p.arpu * (1 + g.between(-0.03, 0.03))

	weeklyChurn := churnPct / 100 / weeksPerMonth
	voluntary := premium * weeklyChurn * voluntaryChurnShare
	involuntary := premium * weeklyChurn * involuntaryChurnShare
	netAdds := premium * p.weeklyGrowthRate * g.between(0.65, 1.35)
	grossAdds := netAdds + voluntary + involuntary

	freeToPaid := g.between(52, 62)
	trials := grossAdds / (freeToPaid / 100)
	offersSeen := trials / g.between(0.28, 0.38)
	retained90 := grossAdds * g.between(0.78, 0.88)

	blend := 0.0
	for _, tier := range planTiers {
		blend += tier.subscriberShare * tier.arpuFactor
	}
	planRows := make([][]string, 0, len(planTiers))
	for _, tier := range planTiers {
		subs := premium * tier.subscriberShare
		planRows = append(planRows, []string{
			tier.name,
			money(subs),
			fmt.Sprintf("%.2f", arpu*tier.arpuFactor/blend),
			fmt.Sprintf("%.1f%%", churnPct*tier.churnMultiplier),
		})
	}

	planNetAdds := p.premium * p.weeklyGrowthRate
	var events []reporting.Event
	if netAdds < planNetAdds*0.85 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(30, 2400)), Type: "growth.target_missed", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("%s net adds %s against a plan of %s", tm.Name, money(netAdds), money(planNetAdds)),
		})
	}
	if g.rng.Float64() < 0.12 {
		churnPct *= g.between(1.15, 1.4)
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(60, 3000)), Type: "growth.churn_spike", Severity: reporting.SeverityCritical,
			Label: "churn spike in " + tm.Name + ": " + g.llm.Cause(ArchRevenue),
		})
	}
	if g.rng.Float64() < 0.08 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(60, 4000)), Type: "growth.milestone", Severity: reporting.SeverityInfo,
			Label: fmt.Sprintf("%s passed %s premium subscribers", tm.Name, money(math.Floor(premium/5e6)*5e6)),
		})
	}

	in := tm.instance("subscriber-growth", t,
		map[string]float64{
			"mau":              math.Round(mau),
			"premium_subs":     math.Round(premium),
			"net_adds":         math.Round(netAdds),
			"churn_pct":        round1(churnPct),
			"arpu_eur":         math.Round(arpu*100) / 100,
			"free_to_paid_pct": round1(freeToPaid),
		},
		map[string]float64{
			"net_adds":  math.Round(planNetAdds),
			"churn_pct": round1(p.monthlyChurnPct),
			"arpu_eur":  math.Round(p.arpu*100) / 100,
		})
	in.Series = map[string][]reporting.SeriesPoint{
		"mau_trend":     g.series(t, 8, everyDay, mau, 0.008),
		"net_adds_rate": g.series(t, 7, everyDay, netAdds/7, 0.3),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"subscriber_bridge": bridge("opening subs", "closing subs", math.Round(premium), [][2]interface{}{
			{"gross adds", math.Round(grossAdds)},
			{"voluntary churn", -math.Round(voluntary)},
			{"involuntary churn", -math.Round(involuntary)},
		}),
		"growth_funnel": funnel([][2]interface{}{
			{"free base reached", math.Round(offersSeen)},
			{"trial started", math.Round(trials)},
			{"converted to paid", math.Round(grossAdds)},
			{"retained 90d", math.Round(retained90)},
		}),
		"plan_mix":        {Columns: []string{"plan", "subscribers", "arpu_eur", "churn_pct"}, Rows: planRows},
		"premium_by_unit": byUnit("premium_subs", tm, math.Round(premium)),
	}
	return tm.submit(in)
}

type salesDesk struct {
	weeklyBookingsEUR float64
	avgDealEUR        float64
	winRatePct        float64
	pipelineCoverage  float64
	ecpmEUR           float64
	sellThroughPct    float64
	categories        []adCategory
	advertisers       []string
}

type adCategory struct {
	name       string
	weight     float64
	ecpmFactor float64
}

var brandCategories = []adCategory{
	{"automotive", 0.18, 1.22},
	{"consumer packaged goods", 0.16, 1.05},
	{"entertainment", 0.15, 1.12},
	{"telco", 0.13, 1.08},
	{"retail", 0.12, 0.92},
	{"financial services", 0.11, 1.28},
	{"travel", 0.09, 0.95},
	{"gaming", 0.06, 0.86},
}

var selfServeCategories = []adCategory{
	{"local services", 0.22, 0.78},
	{"e-commerce", 0.20, 0.96},
	{"gaming", 0.14, 1.04},
	{"education", 0.12, 0.88},
	{"health and fitness", 0.11, 0.92},
	{"events and ticketing", 0.10, 1.10},
	{"real estate", 0.06, 0.84},
	{"professional services", 0.05, 0.90},
}

var (
	brandAdvertisers = []string{
		"Northbound Motors", "Kellerman Foods", "Helix Telecom", "Meridian Pictures",
		"Vantage Retail Group", "Aurora Financial", "Trailhead Travel", "Pixelforge Games",
		"Orbit Beverages", "Sundial Apparel",
	}
	selfServeAdvertisers = []string{
		"Beacon Dental", "Trailmark Outfitters", "Fika Coffee Roasters", "Lumen Yoga",
		"Kestrel Bookshop", "Nordvik Fitness", "Palermo Pizza Co", "Studio Ren",
		"Harbourline Removals", "Vesper Skincare",
	}
	dealStages = []string{"qualified", "proposal", "negotiation", "verbal commit"}
)

var desks = map[string]salesDesk{
	"brand-sales": {
		weeklyBookingsEUR: 27e6, avgDealEUR: 300_000, winRatePct: 26, pipelineCoverage: 3.6,
		ecpmEUR: 15.0, sellThroughPct: 78, categories: brandCategories, advertisers: brandAdvertisers,
	},
	"self-serve-ads": {
		weeklyBookingsEUR: 6e6, avgDealEUR: 1_600, winRatePct: 52, pipelineCoverage: 2.7,
		ecpmEUR: 6.0, sellThroughPct: 62, categories: selfServeCategories, advertisers: selfServeAdvertisers,
	},
}

func fallbackSalesDesk(headcount int) salesDesk {
	return salesDesk{
		weeklyBookingsEUR: float64(headcount) * 120_000, avgDealEUR: 40_000, winRatePct: 34,
		pipelineCoverage: 3.2, ecpmEUR: 9.0, sellThroughPct: 70,
		categories: brandCategories, advertisers: brandAdvertisers,
	}
}

func buildSalesPipeline(g *Generator, ti int, tm Team, t time.Time) Submission {
	d, ok := desks[tm.Name]
	if !ok {
		d = fallbackSalesDesk(tm.Headcount)
	}

	bookings := d.weeklyBookingsEUR * g.between(0.82, 1.18) * g.season(t, tm, 0.08)
	avgDeal := d.avgDealEUR * g.between(0.88, 1.14)
	winRate := d.winRatePct * g.between(0.88, 1.12)
	coverage := d.pipelineCoverage + g.between(-0.45, 0.45)
	ecpm := d.ecpmEUR * g.between(0.92, 1.10)
	sellThrough := d.sellThroughPct * g.between(0.94, 1.06)

	won := bookings / avgDeal
	proposals := won / (winRate / 100)
	qualified := proposals / g.between(0.50, 0.62)
	leads := qualified / g.between(0.38, 0.50)

	raw := make([]float64, len(d.categories))
	total := 0.0
	for i, c := range d.categories {
		raw[i] = c.weight * g.between(0.75, 1.3)
		total += raw[i]
	}
	catRows := make([][]string, 0, len(d.categories))
	for i, c := range d.categories {
		catRows = append(catRows, []string{
			c.name,
			money(bookings * raw[i] / total),
			fmt.Sprintf("%.2f", ecpm*c.ecpmFactor),
		})
	}

	dealRows := make([][]string, 0, 6)
	for i := 0; i < 6; i++ {
		adv := d.advertisers[g.rng.Intn(len(d.advertisers))]
		cat := d.categories[g.rng.Intn(len(d.categories))]
		dealRows = append(dealRows, []string{
			adv,
			cat.name,
			dealStages[g.rng.Intn(len(dealStages))],
			money(avgDeal * g.between(0.4, 3.2)),
			g.llm.Owner(),
		})
	}

	var events []reporting.Event
	for i := 0; i < g.intBetween(1, 3); i++ {
		adv := d.advertisers[g.rng.Intn(len(d.advertisers))]
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(60, 9000)), Type: "pipeline.deal_won", Severity: reporting.SeverityInfo,
			Label: fmt.Sprintf("closed %s at %s EUR", adv, money(avgDeal*g.between(0.6, 2.4))),
		})
	}
	if g.rng.Float64() < 0.55 {
		adv := d.advertisers[g.rng.Intn(len(d.advertisers))]
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(60, 9000)), Type: "pipeline.deal_lost", Severity: reporting.SeverityWarn,
			Label: "lost " + adv + ": " + g.llm.Cause(ArchSales),
		})
	}
	if coverage < pipelineCoverageFloor {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(30, 600)), Type: "pipeline.coverage_low", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("coverage at %.1fx, below the 3x floor", coverage),
		})
	}
	if coverage < pipelineCoverageQuotaRisk {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(10, 400)), Type: "pipeline.quota_at_risk", Severity: reporting.SeverityCritical,
			Label: tm.Name + " will not cover quota on current pipeline",
		})
	}

	in := tm.instance("sales-pipeline", t,
		map[string]float64{
			"bookings_eur":      math.Round(bookings),
			"pipeline_coverage": round1(coverage),
			"win_rate_pct":      round1(winRate),
			"avg_deal_size_eur": math.Round(avgDeal),
			"sell_through_pct":  round1(sellThrough),
			"ecpm_eur":          math.Round(ecpm*100) / 100,
		},
		map[string]float64{
			"bookings_eur":      math.Round(d.weeklyBookingsEUR),
			"pipeline_coverage": d.pipelineCoverage,
			"win_rate_pct":      d.winRatePct,
			"sell_through_pct":  d.sellThroughPct,
			"ecpm_eur":          d.ecpmEUR,
		})
	in.Series = map[string][]reporting.SeriesPoint{
		"bookings_rate":  g.series(t, 7, everyDay, bookings/7, 0.35),
		"pipeline_value": g.series(t, 7, everyDay, bookings*13*coverage, 0.05),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"pipeline_funnel": funnel([][2]interface{}{
			{"lead", math.Round(leads)},
			{"qualified", math.Round(qualified)},
			{"proposal", math.Round(proposals)},
			{"closed won", math.Round(won)},
		}),
		"bookings_by_category": {Columns: []string{"category", "bookings_eur", "ecpm_eur"}, Rows: catRows},
		"open_deals":           {Columns: []string{"advertiser", "category", "stage", "value_eur", "owner"}, Rows: dealRows},
		"bookings_by_unit":     byUnit("bookings_eur", tm, math.Round(bookings)),
	}
	return tm.submit(in)
}

type marketingDesk struct {
	dailySpendEUR          float64
	dailyImpressions       float64
	ctrPct                 float64
	signupsPerClick        float64
	paidPerSignup          float64
	netRevPerConversionEUR float64
	channels               []marketingChannel
}

type marketingChannel struct {
	name                 string
	weight               float64
	conversionEfficiency float64
}

var brandChannels = []marketingChannel{
	{"broadcast video", 0.26, 0.55},
	{"out-of-home", 0.20, 0.35},
	{"social video", 0.18, 1.35},
	{"podcast sponsorship", 0.14, 0.95},
	{"audio partners", 0.12, 1.10},
	{"creator and influencer", 0.10, 1.60},
}

var performanceChannels = []marketingChannel{
	{"paid social", 0.30, 1.05},
	{"paid search", 0.24, 1.45},
	{"app install network", 0.18, 0.80},
	{"email lifecycle", 0.12, 2.30},
	{"push and in-app", 0.09, 2.60},
	{"affiliate", 0.07, 0.90},
}

var marketingDesks = map[string]marketingDesk{
	"brand-campaigns": {
		dailySpendEUR: 620_000, dailyImpressions: 77.5e6, ctrPct: 0.50, signupsPerClick: 0.050,
		paidPerSignup: 0.66, netRevPerConversionEUR: 40, channels: brandChannels,
	},
	"creative-studio": {
		dailySpendEUR: 340_000, dailyImpressions: 42e6, ctrPct: 0.45, signupsPerClick: 0.045,
		paidPerSignup: 0.65, netRevPerConversionEUR: 42, channels: brandChannels,
	},
	"lifecycle-marketing": {
		dailySpendEUR: 260_000, dailyImpressions: 90e6, ctrPct: 3.5, signupsPerClick: 0.030,
		paidPerSignup: 0.50, netRevPerConversionEUR: 26, channels: performanceChannels,
	},
	"performance-marketing": {
		dailySpendEUR: 680_000, dailyImpressions: 62e6, ctrPct: 1.8, signupsPerClick: 0.090,
		paidPerSignup: 0.70, netRevPerConversionEUR: 32, channels: performanceChannels,
	},
}

func fallbackMarketingDesk(headcount int) marketingDesk {
	return marketingDesk{
		dailySpendEUR: float64(headcount) * 9_000, dailyImpressions: float64(headcount) * 1.4e6,
		ctrPct: 1.1, signupsPerClick: 0.05, paidPerSignup: 0.6, netRevPerConversionEUR: 32, channels: performanceChannels,
	}
}

func buildCampaign(g *Generator, ti int, tm Team, t time.Time) Submission {
	d, ok := marketingDesks[tm.Name]
	if !ok {
		d = fallbackMarketingDesk(tm.Headcount)
	}

	pace := g.season(t, tm, 0.12) * g.between(0.85, 1.15)
	spend := d.dailySpendEUR * pace
	impressions := d.dailyImpressions * pace * g.between(0.92, 1.08)
	ctr := d.ctrPct * g.between(0.85, 1.15)
	clicks := impressions * ctr / 100
	signups := clicks * d.signupsPerClick * g.between(0.88, 1.12)
	conversions := signups * d.paidPerSignup * g.between(0.92, 1.08)
	cac := spend / conversions
	roas := conversions * d.netRevPerConversionEUR * g.between(0.92, 1.08) / spend

	planConversions := d.dailyImpressions * d.ctrPct / 100 * d.signupsPerClick * d.paidPerSignup
	planCAC := d.dailySpendEUR / planConversions
	planROAS := planConversions * d.netRevPerConversionEUR / d.dailySpendEUR

	rawSpend := make([]float64, len(d.channels))
	rawConv := make([]float64, len(d.channels))
	spendTotal, convTotal := 0.0, 0.0
	for i, c := range d.channels {
		rawSpend[i] = c.weight * g.between(0.8, 1.25)
		rawConv[i] = rawSpend[i] * c.conversionEfficiency
		spendTotal += rawSpend[i]
		convTotal += rawConv[i]
	}
	chanRows := make([][]string, 0, len(d.channels))
	for i, c := range d.channels {
		cs := spend * rawSpend[i] / spendTotal
		cc := conversions * rawConv[i] / convTotal
		chanRows = append(chanRows, []string{
			c.name,
			money(cs),
			money(cc),
			fmt.Sprintf("%.2f", cs/cc),
			fmt.Sprintf("%.2f", cc*d.netRevPerConversionEUR/cs),
		})
	}

	var events []reporting.Event
	if g.rng.Float64() < 0.35 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(30, 1400)), Type: "campaign.launched", Severity: reporting.SeverityInfo,
			Label: "live: " + g.llm.Service(ArchMarketing),
		})
	}
	if cac > planCAC*1.15 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(15, 900)), Type: "campaign.cac_breach", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("CAC %.2f EUR against a %.2f EUR target: %s", cac, planCAC, g.llm.Cause(ArchMarketing)),
		})
	}
	if g.rng.Float64() < 0.15 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(15, 1200)), Type: "campaign.creative_rejected", Severity: reporting.SeverityWarn,
			Label: "creative rejected on " + d.channels[g.rng.Intn(len(d.channels))].name,
		})
	}
	if g.rng.Float64() < 0.06 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(5, 600)), Type: "campaign.paused", Severity: reporting.SeverityCritical,
			Label: "paused " + g.llm.Service(ArchMarketing) + ": " + g.llm.Cause(ArchMarketing),
		})
	}

	in := tm.instance("campaign", t,
		map[string]float64{
			"spend_eur":   math.Round(spend),
			"impressions": math.Round(impressions),
			"ctr_pct":     math.Round(ctr*100) / 100,
			"cac_eur":     math.Round(cac*100) / 100,
			"roas":        math.Round(roas*100) / 100,
			"conversions": math.Round(conversions),
		},
		map[string]float64{
			"cac_eur":     math.Round(planCAC*100) / 100,
			"roas":        math.Round(planROAS*100) / 100,
			"conversions": math.Round(planConversions),
		})
	in.Series = map[string][]reporting.SeriesPoint{
		"spend_rate": g.series(t, 8, 3*time.Hour, spend/24, 0.35),
		"cac_trend":  g.series(t, 7, everyDay, cac, 0.12),
	}
	in.Events = events
	cacCents := math.Round(cac * 100)
	in.Tables = map[string]reporting.Table{
		"cac_bridge": bridge("yesterday", "today", cacCents, [][2]interface{}{
			{"auction price", math.Round(cac * 100 * g.between(0.01, 0.07))},
			{"creative performance", math.Round(cac * 100 * g.between(-0.06, 0.02))},
			{"channel mix", math.Round(cac * 100 * g.between(-0.04, 0.04))},
			{"landing conversion", math.Round(cac * 100 * g.between(-0.05, 0.02))},
		}),
		"acquisition_funnel": funnel([][2]interface{}{
			{"impression", math.Round(impressions)},
			{"click", math.Round(clicks)},
			{"signup", math.Round(signups)},
			{"paid", math.Round(conversions)},
		}),
		"spend_by_channel": {Columns: []string{"channel", "spend_eur", "conversions", "cac_eur", "roas"}, Rows: chanRows},
		"spend_by_unit":    byUnit("spend_eur", tm, math.Round(spend)),
	}
	return tm.submit(in)
}

type commsDesk struct {
	mentionsLo, mentionsHi float64
	sovLo, sovHi           float64
	sentimentLo            float64
	sentimentHi            float64
	releasesLo, releasesHi int
	pickupLo, pickupHi     float64
	sentimentTarget        float64
	pickupTarget           float64
	channels               []string
	spikeThreshold         float64
}

var (
	pressChannels = []string{
		"national press", "trade press", "broadcast", "newswire",
		"music and audio blogs", "regional press", "podcast press",
	}
	internalChannels = []string{
		"all-hands", "intranet", "email digest", "team channels",
		"manager cascade", "regional newsletter",
	}
)

var commsDesks = map[string]commsDesk{
	"press-office": {
		mentionsLo: 800, mentionsHi: 2400, sovLo: 28, sovHi: 42,
		sentimentLo: 58, sentimentHi: 72, releasesLo: 0, releasesHi: 4,
		pickupLo: 34, pickupHi: 52, sentimentTarget: 62, pickupTarget: 45,
		channels: pressChannels, spikeThreshold: 2000,
	},
	"internal-communications": {
		mentionsLo: 120, mentionsHi: 420, sovLo: 12, sovHi: 24,
		sentimentLo: 68, sentimentHi: 82, releasesLo: 1, releasesHi: 3,
		pickupLo: 58, pickupHi: 78, sentimentTarget: 72, pickupTarget: 65,
		channels: internalChannels, spikeThreshold: 360,
	},
}

var fallbackCommsDesk = commsDesk{
	mentionsLo: 200, mentionsHi: 900, sovLo: 15, sovHi: 30,
	sentimentLo: 60, sentimentHi: 76, releasesLo: 0, releasesHi: 3,
	pickupLo: 40, pickupHi: 62, sentimentTarget: 65, pickupTarget: 50,
	channels: pressChannels, spikeThreshold: 800,
}

func buildBrandReach(g *Generator, ti int, tm Team, t time.Time) Submission {
	d, ok := commsDesks[tm.Name]
	if !ok {
		d = fallbackCommsDesk
	}

	mentions := g.between(d.mentionsLo, d.mentionsHi) * g.season(t, tm, 0.3)
	sov := g.between(d.sovLo, d.sovHi)
	sentiment := g.between(d.sentimentLo, d.sentimentHi)
	releases := float64(g.intBetween(d.releasesLo, d.releasesHi))
	pickup := g.between(d.pickupLo, d.pickupHi)

	negativeCycle := g.rng.Float64() < negativeNewsCycleChance
	if negativeCycle {
		mentions *= g.between(1.6, 3.0)
		sentiment -= g.between(12, 26)
	}

	raw := make([]float64, len(d.channels))
	total := 0.0
	for i := range d.channels {
		raw[i] = g.between(0.4, 1.6)
		total += raw[i]
	}
	chanRows := make([][]string, 0, len(d.channels))
	for i, c := range d.channels {
		chanRows = append(chanRows, []string{
			c,
			money(mentions * raw[i] / total),
			fmt.Sprintf("%.0f", sentiment*g.between(0.88, 1.10)),
			pct(pickup * g.between(0.75, 1.25)),
		})
	}

	itemRows := make([][]string, 0, 4)
	for i := 0; i < g.intBetween(2, 4); i++ {
		pickedUp := "no"
		if g.rng.Float64() < pickup/100 {
			pickedUp = "yes"
		}
		itemRows = append(itemRows, []string{
			g.llm.Service(ArchComms),
			d.channels[g.rng.Intn(len(d.channels))],
			money(mentions * g.between(0.05, 0.35)),
			fmt.Sprintf("%.0f", sentiment*g.between(0.9, 1.08)),
			pickedUp,
		})
	}

	var events []reporting.Event
	for i := 0; i < int(releases); i++ {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(30, 900)), Type: "comms.release_published", Severity: reporting.SeverityInfo,
			Label: "published " + g.llm.Service(ArchComms),
		})
	}
	if mentions > d.spikeThreshold {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(15, 700)), Type: "comms.coverage_spike", Severity: reporting.SeverityInfo,
			Label: fmt.Sprintf("%s mentions today, above the %s baseline", money(mentions), money(d.spikeThreshold)),
		})
	}
	if sentiment < d.sentimentTarget {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(20, 800)), Type: "comms.sentiment_drop", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("sentiment %.0f against a %.0f floor: %s", sentiment, d.sentimentTarget, g.llm.Cause(ArchComms)),
		})
	}
	if negativeCycle {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(5, 400)), Type: "comms.negative_cycle", Severity: reporting.SeverityCritical,
			Label: "negative cycle running: " + g.llm.Cause(ArchComms),
		})
	}

	in := tm.instance("brand-reach", t,
		map[string]float64{
			"mentions":           math.Round(mentions),
			"share_of_voice_pct": round1(sov),
			"sentiment_score":    round1(sentiment),
			"releases_published": releases,
			"pickup_rate_pct":    round1(pickup),
		},
		map[string]float64{
			"sentiment_score": d.sentimentTarget,
			"pickup_rate_pct": d.pickupTarget,
		})
	in.Series = map[string][]reporting.SeriesPoint{
		"mention_rate":    g.series(t, 8, 3*time.Hour, mentions/24, 0.4),
		"sentiment_trend": g.series(t, 7, everyDay, sentiment, 0.05),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"reach_by_channel": {Columns: []string{"channel", "mentions", "sentiment_score", "pickup_rate_pct"}, Rows: chanRows},
		"top_items":        {Columns: []string{"item", "channel", "mentions", "sentiment_score", "picked_up"}, Rows: itemRows},
		"mentions_by_unit": byUnit("mentions", tm, math.Round(mentions)),
	}
	return tm.submit(in)
}
