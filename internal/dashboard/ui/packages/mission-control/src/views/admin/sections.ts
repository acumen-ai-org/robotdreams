export interface AdminSection {
  id: string;
  label: string;
  blurb: string;
  group: "settings" | "design" | "docs";
}

export const ADMIN_SECTIONS: AdminSection[] = [
  {
    id: "connection",
    label: "Connection",
    blurb: "Which control plane this dashboard talks to, and the token it uses.",
    group: "settings",
  },
  {
    id: "vocabulary",
    label: "Vocabulary",
    blurb: "What this deployment calls its hierarchy levels — the words the whole UI then uses.",
    group: "settings",
  },
  {
    id: "activity",
    label: "Activity",
    blurb: "The live control-plane feed, unfiltered — the whole fleet rather than one selected thing.",
    group: "settings",
  },
  {
    id: "storage",
    label: "Storage",
    blurb: "Every object in shared storage, whoever wrote it.",
    group: "settings",
  },
  {
    id: "updates",
    label: "Updates",
    blurb: "What has been announced to the fleet, and what every node said back.",
    group: "settings",
  },
  {
    id: "design/colors",
    label: "Colors",
    blurb: "The named palette, and the semantic tokens it resolves into — live, in both themes.",
    group: "design",
  },
  {
    id: "design/typography",
    label: "Typography",
    blurb: "Three voices, each with a job, and the scale they are set at.",
    group: "design",
  },
  {
    id: "design/spacing",
    label: "Spacing & layout",
    blurb: "The spacing scale, the radii, and the two width tiers.",
    group: "design",
  },
  {
    id: "design/iconography",
    label: "Iconography",
    blurb: "The inlined icon set, the place badges, and the role emoji.",
    group: "design",
  },
  {
    id: "design/motion",
    label: "Motion & depth",
    blurb: "How things move, and how surfaces stack.",
    group: "design",
  },
  {
    id: "design/primitives",
    label: "Primitive components",
    blurb: "Buttons, pills, badges, tags — the real classes, rendered live.",
    group: "design",
  },
  {
    id: "design/composites",
    label: "Composite components",
    blurb: "Stat tiles, tables, summary boxes — primitives assembled into the app's shapes.",
    group: "design",
  },
  {
    id: "design/voice",
    label: "Voice & content",
    blurb: "How the interface talks: sentence case, asserted numbers, honest empty states.",
    group: "design",
  },
  {
    id: "docs/infographic",
    label: "Infographic",
    blurb: "The whole system as one picture — click any part of it to expand what it is.",
    group: "docs",
  },
  {
    id: "docs/spine",
    label: "The spine",
    blurb: "Two swappable primitives and a control plane — the whole system in one picture.",
    group: "docs",
  },
  {
    id: "docs/hierarchy",
    label: "Hierarchy & identity",
    blurb: "Universe → World → Realm → Site → Node, and how each is named, numbered, and drawn.",
    group: "docs",
  },
  {
    id: "docs/reporting",
    label: "Reporting model",
    blurb: "A definition is the type; an instance is the data. And the seven facets.",
    group: "docs",
  },
  {
    id: "docs/categories",
    label: "Categories",
    blurb: "The nine standard subjects a report can be about — and what deliberately isn't one.",
    group: "docs",
  },
  {
    id: "docs/stances",
    label: "Stances",
    blurb: "Operational, strategic, diagnostic — what a report's numbers are measured against.",
    group: "docs",
  },
  {
    id: "docs/modalities",
    label: "Modalities & views",
    blurb: "Seven delivery modes, seven views \u2014 two of them templated rather than model-backed.",
    group: "docs",
  },
  {
    id: "docs/primitives",
    label: "Visual vocabulary",
    blurb: "Which drawing answers which question, the order a report is read in, and what we don't draw.",
    group: "docs",
  },
  {
    id: "docs/aggregation",
    label: "Aggregation",
    blurb: "How a number at a site becomes a number at a universe.",
    group: "docs",
  },
  {
    id: "docs/scope",
    label: "Scope & deep links",
    blurb: "Scope paths, prefix matching, and the URL grammar that addresses any view.",
    group: "docs",
  },
  {
    id: "docs/updates",
    label: "Updates",
    blurb: "How a new version is announced, and what each node says back.",
    group: "docs",
  },
  {
    id: "docs/library",
    label: "Component library",
    blurb: "What this dashboard exports as a package, at which level, and what a host brings to run it.",
    group: "docs",
  },
];

export const GROUP_LABELS: Record<AdminSection["group"], string> = {
  settings: "Settings",
  design: "Design system",
  docs: "Docs",
};

export function findSection(id: string, sections: AdminSection[] = ADMIN_SECTIONS): AdminSection {
  return sections.find((s) => s.id === id) || sections[0];
}
