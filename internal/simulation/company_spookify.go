package simulation

func init() {
	registerUniverse(UniverseDef{
		Name:     "spookify",
		Title:    "Spookify, a streaming company",
		Tagline:  "Consumer streaming at company scale — 13 divisions, mostly not engineering",
		RootID:   CEOID,
		RootRole: "ceo",
		Scale:    ScaleEnterprise,
		MinFTE:   1_000,
		MaxFTE:   12_000,
		Teams:    spookifyTeams,
		Scorecard: ScorecardProfile{
			UnitLabels:     [2]string{"users", "subscribers"},
			Users:          696e6,
			PayingUnits:    276e6,
			RevenuePerUnit: 4.57,
			GrossMarginPct: 31.5,
			UserTarget:     700e6,
			PayingTarget:   276e6,
			RevenueShare: map[string]float64{
				"music": 0.74, "advertising": 0.12, "podcasts": 0.09, "audiobooks": 0.05,
			},
		},
	})
}

var spookifyTeams = []Team{
	{Division: "music", Department: "playback", Name: "player-squad", Archetype: ArchService, Region: RegionNordics, Roles: rolesService, Headcount: 140},
	{Division: "music", Department: "playback", Name: "streaming-squad", Archetype: ArchService, Region: RegionNordics, Roles: rolesService, Headcount: 165},
	{Division: "music", Department: "platform-engineering", Name: "deploy-squad", Archetype: ArchService, Region: RegionNordics, Roles: rolesSmallSvc, Headcount: 90},
	{Division: "music", Department: "platform-engineering", Name: "infra-squad", Archetype: ArchService, Region: RegionNordics, Roles: rolesService, Headcount: 115},
	{Division: "music", Department: "personalization", Name: "recommendations-squad", Archetype: ArchResearch, Region: RegionNorthAm, Roles: rolesResearch, Headcount: 185},
	{Division: "music", Department: "personalization", Name: "search-squad", Archetype: ArchService, Region: RegionNorthAm, Roles: rolesService, Headcount: 100},

	{Division: "podcasts", Department: "creation", Name: "studio-squad", Archetype: ArchService, Region: RegionNorthAm, Roles: rolesService, Headcount: 120},
	{Division: "podcasts", Department: "creation", Name: "creator-tools-squad", Archetype: ArchService, Region: RegionNorthAm, Roles: rolesSmallSvc, Headcount: 76},
	{Division: "podcasts", Department: "discovery", Name: "browse-squad", Archetype: ArchService, Region: RegionNordics, Roles: rolesSmallSvc, Headcount: 84},
	{Division: "podcasts", Department: "discovery", Name: "charts-squad", Archetype: ArchResearch, Region: RegionNordics, Roles: rolesResearch, Headcount: 56},

	{Division: "audiobooks", Department: "catalog-engineering", Name: "ingestion-squad", Archetype: ArchService, Region: RegionEMEA, Variant: "books", Roles: rolesSmallSvc, Headcount: 68},
	{Division: "audiobooks", Department: "catalog-engineering", Name: "reader-squad", Archetype: ArchService, Region: RegionEMEA, Variant: "books", Roles: rolesSmallSvc, Headcount: 60},
	{Division: "audiobooks", Department: "publisher-relations", Name: "publisher-partnerships", Archetype: ArchLicensing, Region: RegionEMEA, Variant: "publishing", Roles: rolesRights, Headcount: 44},

	{Division: "advertising", Department: "adtech", Name: "ad-serving-squad", Archetype: ArchService, Region: RegionNorthAm, Roles: rolesService, Headcount: 135},
	{Division: "advertising", Department: "adtech", Name: "targeting-squad", Archetype: ArchResearch, Region: RegionNorthAm, Roles: rolesResearch, Headcount: 98},
	{Division: "advertising", Department: "ad-sales", Name: "brand-sales", Archetype: ArchSales, Region: RegionNorthAm, Roles: rolesOutreach, Headcount: 235},
	{Division: "advertising", Department: "ad-sales", Name: "self-serve-ads", Archetype: ArchSales, Region: RegionEMEA, Roles: rolesOutreach, Headcount: 105},

	{Division: "platform", Department: "core-infrastructure", Name: "compute-squad", Archetype: ArchService, Region: RegionNordics, Roles: rolesService, Headcount: 155},
	{Division: "platform", Department: "core-infrastructure", Name: "storage-squad", Archetype: ArchService, Region: RegionNordics, Roles: rolesService, Headcount: 125},
	{Division: "platform", Department: "data", Name: "pipelines-squad", Archetype: ArchData, Region: RegionNordics, Roles: rolesData, Headcount: 110},
	{Division: "platform", Department: "data", Name: "warehouse-squad", Archetype: ArchData, Region: RegionEMEA, Roles: rolesData, Headcount: 94},
	{Division: "platform", Department: "machine-learning", Name: "models-squad", Archetype: ArchResearch, Region: RegionNorthAm, Roles: rolesResearch, Headcount: 120},
	{Division: "platform", Department: "machine-learning", Name: "evaluation-squad", Archetype: ArchResearch, Region: RegionNorthAm, Roles: rolesResearch, Headcount: 64},
	{Division: "platform", Department: "security", Name: "application-security", Archetype: ArchSecurity, Region: RegionNordics, Variant: "appsec", Roles: rolesSecurity, Headcount: 72},
	{Division: "platform", Department: "security", Name: "threat-detection", Archetype: ArchSecurity, Region: RegionEMEA, Roles: rolesSecurity, Headcount: 60},

	{Division: "content", Department: "licensing", Name: "label-relations", Archetype: ArchLicensing, Region: RegionNorthAm, Roles: rolesRights, Headcount: 180},
	{Division: "content", Department: "licensing", Name: "publishing-rights", Archetype: ArchLicensing, Region: RegionEMEA, Variant: "publishing", Roles: rolesRights, Headcount: 125},
	{Division: "content", Department: "editorial", Name: "playlist-editorial", Archetype: ArchContent, Region: RegionNordics, Roles: rolesCurate, Headcount: 255},
	{Division: "content", Department: "editorial", Name: "genre-curation", Archetype: ArchContent, Region: RegionEMEA, Variant: "hubs", Roles: rolesCurate, Headcount: 190},

	{Division: "markets", Department: "americas", Name: "north-america", Archetype: ArchRevenue, Region: RegionNorthAm, Roles: rolesAnalyst, Headcount: 210},
	{Division: "markets", Department: "americas", Name: "latin-america", Archetype: ArchRevenue, Region: RegionLatAm, Roles: rolesAnalyst, Headcount: 155},
	{Division: "markets", Department: "emea", Name: "nordics", Archetype: ArchRevenue, Region: RegionNordics, Roles: rolesAnalyst, Headcount: 98},
	{Division: "markets", Department: "emea", Name: "growth-markets", Archetype: ArchRevenue, Region: RegionEMEA, Roles: rolesAnalyst, Headcount: 165},
	{Division: "markets", Department: "apac", Name: "india", Archetype: ArchRevenue, Region: RegionAPAC, Roles: rolesAnalyst, Headcount: 145},
	{Division: "markets", Department: "apac", Name: "southeast-asia", Archetype: ArchRevenue, Region: RegionAPAC, Roles: rolesAnalyst, Headcount: 120},

	{Division: "finance", Department: "fp-and-a", Name: "corporate-planning", Archetype: ArchFinance, Region: RegionNordics, Roles: rolesAnalyst, Headcount: 84},
	{Division: "finance", Department: "fp-and-a", Name: "business-analysis", Archetype: ArchFinance, Region: RegionNorthAm, Roles: rolesAnalyst, Headcount: 72},
	{Division: "finance", Department: "accounting", Name: "revenue-accounting", Archetype: ArchAccounting, Region: RegionNordics, Roles: rolesControl, Headcount: 140},
	{Division: "finance", Department: "accounting", Name: "payroll", Archetype: ArchAccounting, Region: RegionNordics, Roles: rolesControl, Headcount: 64},
	{Division: "finance", Department: "treasury", Name: "cash-management", Archetype: ArchFinance, Region: RegionEMEA, Roles: rolesAnalyst, Headcount: 48},
	{Division: "finance", Department: "treasury", Name: "financial-risk", Archetype: ArchFinance, Region: RegionEMEA, Roles: rolesControl, Headcount: 40},

	{Division: "people", Department: "talent-acquisition", Name: "emea-recruiting", Archetype: ArchPeople, Region: RegionNordics, Roles: rolesOutreach, Headcount: 105},
	{Division: "people", Department: "talent-acquisition", Name: "americas-recruiting", Archetype: ArchPeople, Region: RegionNorthAm, Roles: rolesOutreach, Headcount: 90},
	{Division: "people", Department: "people-operations", Name: "benefits-and-compensation", Archetype: ArchPeopleOps, Region: RegionNordics, Roles: rolesAnalyst, Headcount: 76},
	{Division: "people", Department: "people-operations", Name: "workplace", Archetype: ArchPeopleOps, Region: RegionNordics, Roles: rolesITOps, Headcount: 125},

	{Division: "legal", Department: "commercial-legal", Name: "commercial-contracts", Archetype: ArchLegal, Region: RegionNordics, Roles: rolesCounsel, Headcount: 94},
	{Division: "legal", Department: "commercial-legal", Name: "intellectual-property", Archetype: ArchLegal, Region: RegionNorthAm, Roles: rolesCounsel, Headcount: 56},
	{Division: "legal", Department: "compliance", Name: "privacy-office", Archetype: ArchCompliance, Region: RegionEMEA, Variant: "privacy", Roles: rolesControl, Headcount: 68},
	{Division: "legal", Department: "compliance", Name: "regulatory-affairs", Archetype: ArchCompliance, Region: RegionEMEA, Roles: rolesControl, Headcount: 52},

	{Division: "marketing", Department: "brand", Name: "brand-campaigns", Archetype: ArchMarketing, Region: RegionNorthAm, Roles: rolesCreative, Headcount: 150},
	{Division: "marketing", Department: "brand", Name: "creative-studio", Archetype: ArchMarketing, Region: RegionNordics, Roles: rolesCreative, Headcount: 120},
	{Division: "marketing", Department: "growth-marketing", Name: "lifecycle-marketing", Archetype: ArchMarketing, Region: RegionEMEA, Roles: rolesAnalyst, Headcount: 98},
	{Division: "marketing", Department: "growth-marketing", Name: "performance-marketing", Archetype: ArchMarketing, Region: RegionNorthAm, Roles: rolesAnalyst, Headcount: 110},
	{Division: "marketing", Department: "communications", Name: "press-office", Archetype: ArchComms, Region: RegionNordics, Roles: rolesOutreach, Headcount: 60},
	{Division: "marketing", Department: "communications", Name: "internal-communications", Archetype: ArchComms, Region: RegionNordics, Roles: rolesCreative, Headcount: 36},

	{Division: "user-support", Department: "care-operations", Name: "tier1-care", Archetype: ArchSupport, Region: RegionEMEA, Variant: "tier1", Roles: rolesCare, Headcount: 600},
	{Division: "user-support", Department: "care-operations", Name: "escalations", Archetype: ArchSupport, Region: RegionNorthAm, Roles: rolesCare, Headcount: 170},
	{Division: "user-support", Department: "support-insights", Name: "voice-of-user", Archetype: ArchResearch, Region: RegionEMEA, Roles: rolesAnalyst, Headcount: 52},
	{Division: "user-support", Department: "support-insights", Name: "support-tooling", Archetype: ArchService, Region: RegionEMEA, Roles: rolesSmallSvc, Headcount: 64},

	{Division: "corporate", Department: "strategy", Name: "corporate-development", Archetype: ArchStrategy, Region: RegionNordics, Roles: rolesStrategy, Headcount: 56},
	{Division: "corporate", Department: "strategy", Name: "business-development", Archetype: ArchStrategy, Region: RegionNorthAm, Roles: rolesOutreach, Headcount: 80},
	{Division: "corporate", Department: "it-services", Name: "helpdesk", Archetype: ArchIT, Region: RegionNordics, Variant: "helpdesk", Roles: rolesITOps, Headcount: 135, Reports: append([]string{"workflow"}, archetypeReports[ArchIT]...)},
	{Division: "corporate", Department: "it-services", Name: "corporate-systems", Archetype: ArchIT, Region: RegionEMEA, Roles: rolesITOps, Headcount: 100},
}
