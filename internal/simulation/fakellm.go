package simulation

import (
	"fmt"
	"math/rand"
)

type FakeLLM struct {
	rng *rand.Rand
	v   *Vocabulary
}

type Vocabulary struct {
	Services    map[Archetype][]string
	Causes      map[Archetype][]string
	Components  map[Archetype][]string
	Milestones  []string
	TaskNouns   []string
	Initiatives []string
	Objectives  []string
}

func NewFakeLLM(seed int64, v *Vocabulary) *FakeLLM {
	if v == nil {
		v = &Vocabulary{}
	}
	return &FakeLLM{rng: rand.New(rand.NewSource(seed)), v: v}
}

func (l *FakeLLM) services() map[Archetype][]string {
	if l.v.Services != nil {
		return l.v.Services
	}
	return llmServices
}

func (l *FakeLLM) causes() map[Archetype][]string {
	if l.v.Causes != nil {
		return l.v.Causes
	}
	return llmCauses
}

func (l *FakeLLM) components() map[Archetype][]string {
	if l.v.Components != nil {
		return l.v.Components
	}
	return llmComponents
}

func (l *FakeLLM) milestones() []string {
	if len(l.v.Milestones) > 0 {
		return l.v.Milestones
	}
	return llmMilestoneThemes
}

func (l *FakeLLM) objectives() []string {
	if len(l.v.Objectives) > 0 {
		return l.v.Objectives
	}
	return llmObjectives
}

func (l *FakeLLM) initiatives() []string {
	if len(l.v.Initiatives) > 0 {
		return l.v.Initiatives
	}
	return financeInitiatives
}

func (l *FakeLLM) taskNouns() []string {
	if len(l.v.TaskNouns) > 0 {
		return l.v.TaskNouns
	}
	return llmTaskNouns
}

var llmServices = map[Archetype][]string{
	ArchService: {
		"playback-gateway", "track-metadata", "session-store", "audio-cdn-edge",
		"recs-ranker", "search-indexer", "episode-transcoder", "ad-decision",
		"royalty-ledger", "lyrics-sync", "offline-cache", "social-graph",
	},
	ArchData: {
		"listens-firehose", "royalty-facts", "user-dim", "session-rollup",
		"catalog-snapshot", "ad-impressions", "churn-features", "market-daily",
	},
	ArchResearch: {
		"ranker-v7", "cold-start-embeddings", "shuffle-policy", "podcast-affinity",
		"churn-propensity", "ad-relevance", "query-understanding", "taste-profile",
	},
	ArchSecurity: {
		"token-service", "enrollment-flow", "partner-webhooks", "admin-console",
		"payment-callback", "third-party-sdk", "artifact-registry", "vpn-gateway",
	},
	ArchFinance: {
		"the operating plan", "the rolling forecast", "the cloud budget",
		"the royalty accrual model", "the headcount plan", "the FX exposure",
	},
	ArchAccounting: {
		"the revenue ledger", "the royalty accrual", "the payroll run",
		"intercompany reconciliation", "the deferred revenue schedule", "the VAT return",
	},
	ArchRevenue: {
		"the trial funnel", "the family plan", "the student tier",
		"the winback campaign", "the price change", "the carrier bundle",
	},
	ArchSales: {
		"the upfront commitment", "a programmatic deal", "the audio takeover",
		"a podcast sponsorship", "the self-serve tier", "an agency master agreement",
	},
	ArchPeople: {
		"the engineering pipeline", "the graduate intake", "an exec search",
		"the referral programme", "the interview loop", "the offer package",
	},
	ArchPeopleOps: {
		"the compensation review", "the benefits renewal", "the engagement survey",
		"the onboarding programme", "the office move", "the leave policy",
	},
	ArchLegal: {
		"a distribution agreement", "an NDA", "a licensing amendment",
		"a trademark filing", "a vendor MSA", "a settlement term sheet",
	},
	ArchCompliance: {
		"the DSAR queue", "the DPIA backlog", "the DSA transparency report",
		"the SOX control set", "the retention schedule", "the audit remediation plan",
	},
	ArchMarketing: {
		"the Wrapped campaign", "the podcast launch push", "the student offer",
		"the brand refresh", "the app-install campaign", "the winback flight",
	},
	ArchComms: {
		"the earnings release", "a product announcement", "the artist blog post",
		"the all-hands note", "a press briefing", "the policy statement",
	},
	ArchSupport: {
		"the billing queue", "the playback-issue queue", "the account-recovery flow",
		"the family-plan queue", "the refunds queue", "the app-crash queue",
	},
	ArchContent: {
		"Fresh Cuts Friday", "the hip-hop flagship", "the Nordic pop flagship",
		"the wellness vertical", "the local-language hub", "the release-radar refresh",
	},
	ArchLicensing: {
		"the major-label renewal", "a publishing deal", "an indie aggregator agreement",
		"a neighbouring-rights claim", "the audiobook rights window", "a territory expansion",
	},
	ArchIT: {
		"the identity provider", "the laptop fleet", "the MDM rollout",
		"the VPN concentrator", "the SaaS licence pool", "the meeting-room estate",
	},
	ArchStrategy: {
		"the three-year plan", "a partnership term sheet", "the build-vs-buy review",
		"the market-entry case", "the OKR cycle", "an acquisition thesis",
	},
}

var llmCauses = map[Archetype][]string{
	ArchService: {
		"connection pool exhaustion", "a bad config push", "cache stampede",
		"upstream DNS flakiness", "a slow database migration", "disk pressure on shard 3",
		"an expired TLS certificate", "GC pauses under load", "a retry storm",
		"a leaked goroutine in the fan-out path",
	},
	ArchData: {
		"a late upstream partition", "a schema change nobody announced",
		"a skewed join key", "a backfill competing for slots",
		"clock skew between producers", "a duplicated event batch",
	},
	ArchResearch: {
		"training-serving skew", "a leaked label in the feature set",
		"an under-powered experiment", "seasonality the holdout did not see",
		"a stale feature store", "a metric definition change mid-flight",
	},
	ArchSecurity: {
		"an unpatched transitive dependency", "an over-broad IAM role",
		"a leaked CI token", "a misconfigured storage bucket",
		"a bypassed review on a hotfix", "an unrotated service credential",
	},
	ArchFinance: {
		"a late accrual from a partner", "an FX swing", "an unbudgeted cloud commitment",
		"a headcount plan that moved", "a royalty rate change", "a reclassified cost centre",
	},
	ArchAccounting: {
		"an unreconciled intercompany balance", "a duplicate vendor invoice",
		"a bank feed that arrived late", "a manual journal without support",
		"a cut-off error at period end", "a mismatched payroll file",
	},
	ArchRevenue: {
		"a payment-processor decline spike", "an involuntary churn wave",
		"a price test that ran long", "a carrier billing outage",
		"a trial cohort that never converted", "a competitor promotion",
	},
	ArchSales: {
		"a delayed agency sign-off", "an unfilled premium inventory block",
		"a brand-safety hold", "a budget freeze at the client",
		"a measurement dispute", "seasonal softness in the category",
	},
	ArchPeople: {
		"an interview loop that could not be staffed", "a competing offer",
		"a visa timeline", "a hiring-manager holiday", "a level mismatch at offer",
		"a reopened requisition",
	},
	ArchPeopleOps: {
		"a benefits provider migration", "a payroll cut-off change",
		"a policy that needed works-council review", "a survey response rate below quorum",
		"an office lease negotiation",
	},
	ArchLegal: {
		"a counterparty redline round", "an indemnity cap disagreement",
		"a regulator information request", "a governing-law dispute",
		"an unavailable signatory", "a conflicting prior agreement",
	},
	ArchCompliance: {
		"an incomplete data map", "a processor without a signed DPA",
		"an unlogged retention exception", "a control owner who left",
		"a regulator deadline moved forward", "an audit sample that failed",
	},
	ArchMarketing: {
		"a creative approval that slipped", "rising auction prices",
		"an attribution window change", "a platform policy rejection",
		"a landing page that regressed", "audience saturation",
	},
	ArchComms: {
		"an embargo break", "a spokesperson conflict", "a leaked draft",
		"a news cycle that moved", "a translation that arrived late",
	},
	ArchSupport: {
		"a release that broke offline playback", "a billing incident upstream",
		"a staffing gap on the night shift", "a help-centre article that went stale",
		"a bot handoff loop", "an unusually long queue in one language",
	},
	ArchContent: {
		"a late master delivery", "an embargoed release date",
		"an artwork rights question", "a metadata mismatch from the label",
		"an editorial slot double-booked",
	},
	ArchLicensing: {
		"an unagreed minimum guarantee", "a territory carve-out",
		"a rights-holder dispute", "an unmatched recording ID",
		"a renewal notice period missed",
	},
	ArchIT: {
		"an identity-provider outage", "a certificate that expired on the VPN",
		"an MDM policy push that failed", "a licence pool that ran dry",
		"a firmware update that bricked docks",
	},
	ArchStrategy: {
		"a diligence finding", "a board calendar conflict",
		"a market assumption that did not hold", "a partner that went quiet",
		"an internal funding reallocation",
	},
}

var llmComponents = map[Archetype][]string{
	ArchService: {
		"ingest-worker", "api-frontend", "batch-scheduler", "kafka-consumer",
		"redis-proxy", "feature-store", "cdn-warmer", "auth-middleware",
	},
	ArchData: {
		"hourly-partitioner", "dbt-run", "airflow-scheduler", "schema-registry",
		"quality-suite", "compaction-job", "export-sink",
	},
}

var (
	llmMilestoneThemes = []string{
		"gapless playback", "podcast chapters", "smart shuffle", "ad frequency capping",
		"lossless tier", "creator analytics", "voice search", "offline downloads v2",
		"cross-device handoff", "royalty reporting revamp", "audiobook bundling",
		"in-app checkout", "family plan verification", "regional pricing",
	}
	llmVerbs  = []string{"rolled out", "shipped", "promoted", "canaried", "deployed"}
	llmOwners = []string{
		"dagny", "miles", "petra", "oskar", "yuki", "lennart", "amara", "silas",
		"priya", "tomas", "ines", "kwame", "hanna", "rafael", "noor", "elias",
	}
	llmErrTemplates = []string{
		"%s: request failed after 3 retries: %s",
		"%s: timeout waiting for upstream: %s",
		"%s: dropped 42 messages: %s",
		"%s: panic recovered in handler: %s",
		"%s: circuit breaker open: %s",
	}
	llmDecisionSubjects = []string{
		"a scope cut", "an unbudgeted spend", "a launch date",
		"a vendor selection", "a policy exception", "a headcount backfill",
		"a contract term outside guardrails", "a public commitment",
		"a deprecation with external impact", "a rollout to a new market",
	}
)

func (l *FakeLLM) pick(set []string) string { return set[l.rng.Intn(len(set))] }

func (l *FakeLLM) forArchetype(sets map[Archetype][]string, a Archetype) []string {
	if set, ok := sets[a]; ok && len(set) > 0 {
		return set
	}
	return sets[ArchService]
}

func (l *FakeLLM) Service(a Archetype) string { return l.pick(l.forArchetype(l.services(), a)) }

func (l *FakeLLM) Cause(a Archetype) string { return l.pick(l.forArchetype(l.causes(), a)) }

func (l *FakeLLM) Component(a Archetype) string { return l.pick(l.forArchetype(l.components(), a)) }

func (l *FakeLLM) Owner() string { return l.pick(llmOwners) }

func (l *FakeLLM) DecisionSubject() string { return l.pick(llmDecisionSubjects) }

func (l *FakeLLM) Version() string {
	return fmt.Sprintf("v%d.%d.%d", 1+l.rng.Intn(4), l.rng.Intn(20), l.rng.Intn(10))
}

func (l *FakeLLM) IncidentSummary(service, cause string) string {
	return fmt.Sprintf("%s degraded: %s", service, cause)
}

func (l *FakeLLM) DeployLabel(service, version string) string {
	return fmt.Sprintf("%s %s %s", l.pick(llmVerbs), service, version)
}

func (l *FakeLLM) RollbackLabel(service, version string) string {
	return fmt.Sprintf("rolled back %s to %s after failed canary", service, version)
}

func (l *FakeLLM) ReviewLabel(artifact, outcome string) string {
	return fmt.Sprintf("%s review %s", artifact, outcome)
}

func (l *FakeLLM) LogError(component, cause string) string {
	return fmt.Sprintf(l.pick(llmErrTemplates), component, cause)
}

func (l *FakeLLM) MilestoneName() string {
	return fmt.Sprintf("%s — phase %d", l.pick(l.milestones()), 1+l.rng.Intn(3))
}

func (l *FakeLLM) ItemTitle(a Archetype) string {
	if verbs, ok := llmItemVerbs[a]; ok {
		return fmt.Sprintf("%s %s", l.pick(verbs), l.pick(l.forArchetype(l.services(), a)))
	}
	return fmt.Sprintf("%s %s for %s", l.pick(llmGenericItemVerbs),
		l.pick(l.taskNouns()), l.pick(l.forArchetype(l.services(), a)))
}

func (l *FakeLLM) Objective(a Archetype) string { return l.pick(l.objectives()) }

func (l *FakeLLM) KeyResult() string {
	return fmt.Sprintf("%s to %d%% by quarter end", l.pick(llmKeyResults), 60+5*l.rng.Intn(8))
}

func (l *FakeLLM) TaskLabel(a Archetype) string {
	if verbs, ok := llmTaskVerbs[a]; ok {
		return fmt.Sprintf("%s %s", l.pick(verbs), l.pick(l.forArchetype(l.services(), a)))
	}
	return fmt.Sprintf("%s %s for %s", l.pick([]string{
		"reindexing", "backfilling", "compacting", "validating", "syncing", "profiling",
	}), l.pick(l.taskNouns()), l.pick(l.forArchetype(l.services(), a)))
}

var llmTaskNouns = []string{
	"playlists", "episode metadata", "ad segments", "audio blobs", "user vectors", "search shards",
}

var llmTaskVerbs = map[Archetype][]string{
	ArchFinance:    {"reforecasting", "reviewing variance on", "consolidating", "stress-testing"},
	ArchAccounting: {"reconciling", "posting journals for", "tying out", "closing"},
	ArchRevenue:    {"cohorting", "modelling churn on", "testing pricing for", "reviewing conversion on"},
	ArchSales:      {"pitching", "negotiating", "forecasting", "renewing"},
	ArchPeople:     {"screening for", "scheduling loops for", "debriefing", "extending offers on"},
	ArchPeopleOps:  {"reviewing", "communicating", "benchmarking", "rolling out"},
	ArchLegal:      {"redlining", "negotiating", "papering", "clearing"},
	ArchCompliance: {"evidencing", "triaging", "testing controls for", "remediating"},
	ArchMarketing:  {"briefing", "optimising", "reallocating budget on", "measuring"},
	ArchComms:      {"drafting", "briefing press on", "translating", "publishing"},
	ArchSupport:    {"triaging", "clearing backlog on", "escalating", "updating macros for"},
	ArchContent:    {"programming", "sequencing", "pitching", "refreshing"},
	ArchLicensing:  {"negotiating", "auditing rights on", "renewing", "reconciling"},
	ArchIT:         {"provisioning", "patching", "reclaiming licences on", "imaging"},
	ArchStrategy:   {"modelling", "diligencing", "framing", "reviewing"},
	ArchResearch:   {"training", "evaluating", "ablating", "shipping a holdout for"},
	ArchSecurity:   {"triaging", "threat-modelling", "patching", "hunting in"},
	ArchData:       {"backfilling", "repartitioning", "validating", "compacting"},
}

var llmGenericItemVerbs = []string{
	"reindex", "backfill", "compact", "validate", "sync", "profile",
}

var llmItemVerbs = map[Archetype][]string{
	ArchService:    {"harden", "migrate", "instrument", "shard", "deprecate"},
	ArchData:       {"backfill", "repartition", "reconcile", "document"},
	ArchResearch:   {"replicate", "ablate", "benchmark", "publish"},
	ArchSecurity:   {"patch", "threat-model", "rotate credentials for", "audit"},
	ArchFinance:    {"reforecast", "rebaseline", "stress-test", "consolidate"},
	ArchAccounting: {"reconcile", "close", "restate", "substantiate"},
	ArchRevenue:    {"segment", "price-test", "win back", "forecast"},
	ArchSales:      {"qualify", "renew", "expand", "close"},
	ArchPeople:     {"open a requisition for", "shortlist", "debrief", "level"},
	ArchPeopleOps:  {"roll out", "review", "benchmark", "consolidate"},
	ArchLegal:      {"redline", "counter-sign", "escalate", "close out"},
	ArchCompliance: {"evidence", "remediate", "re-certify", "map"},
	ArchMarketing:  {"brief", "launch", "retarget", "wind down"},
	ArchComms:      {"draft", "place", "brief press on", "schedule"},
	ArchSupport:    {"deflect", "macro", "escalate", "root-cause"},
	ArchContent:    {"commission", "schedule", "refresh", "retire"},
	ArchLicensing:  {"renegotiate", "clear rights for", "renew", "terminate"},
	ArchIT:         {"provision", "decommission", "image", "license"},
	ArchStrategy:   {"scope", "fund", "stage-gate", "sunset"},
}

var llmObjectives = []string{
	"Be the default choice in our largest market",
	"Make the platform boring to operate",
	"Grow paid conversion without discounting",
	"Cut the cost of serving an active user",
	"Shorten the path from idea to production",
	"Earn an unqualified audit",
	"Make onboarding survivable without a human",
	"Own the category conversation",
}

var llmKeyResults = []string{
	"Lift activation", "Cut unit cost", "Raise retention", "Reduce lead time",
	"Improve satisfaction", "Close the compliance gap", "Grow qualified pipeline",
}
