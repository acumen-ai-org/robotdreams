import type { ViewName } from "./routes";
import { readStored, writeStored } from "./storage";

export interface VariantDef {
  id: string;
  label: string;
  hint: string;
  built?: false;
  modality?: string;
  about?: string;
}

export const VIEW_VARIANTS: Record<ViewName, VariantDef[]> = {
  overview: [
    {
      id: "explore",
      label: "Explore",
      hint: "Walk the structure one level at a time",
      about:
        "The structure, walked rather than surveyed: a breadcrumb of where you are, a box for every place at the level you are on, and inside each box a glance at what is one level further down — capped, so a box stays readable. Nodes are not places and do not nest, so the ones reporting at the level you are on get a table underneath. The toggle at its top right swaps the drawing for the reporting hierarchy: who reports to whom, as a collapsible outline. Both read the same fleet; they answer different questions about it.",
    },
    {
      id: "galaxy",
      label: "Galaxy",
      hint: "The places given shape — packed flat, or with a camera among them",
      about:
        "Every node drawn inside the place it reports for — world, then realm, then site. Containment here is scope, not management: a lead is a dot inside its realm, not a circle around its reports. The toggle at its top right chooses how that containment is drawn: Planets packs it into circles on a plane, and Galaxy gives it volume — a world is a glass sphere, its realms are platforms on one floor inside it, its sites the factory silhouette stood on those platforms. Click anything to go to it.",
    },
    {
      id: "force",
      label: "Network",
      hint: "d3-force — the reporting edges as a physical network",
      about:
        "The reporting paths themselves — every edge is a reports-to relationship, and so the route a message takes between two nodes. Line thickness is how much traffic is running along that line right now: the event stream's per-edge message counts, faded as they age, so the weight reads as a rate rather than a total. Position carries no meaning; the forces only keep the edges legible.",
    },
  ],
  reporting: [
    {
      id: "panels",
      label: "Glance",
      modality: "glance",
      hint: "glance — where things stand, or how they have moved",
      about:
        "One tile per report at this scope, aggregated from everything beneath it. Built to be read in a second: status first, then the numbers behind it.",
    },
    {
      id: "timeline",
      label: "Timeline",
      modality: "storyboard",
      hint: "storyboard — the merged event feed",
      about:
        "Events from every report at this scope, merged into one time-ordered story. Each entry keeps the scope it came from; the toolbox chooses whether the story runs down the page or across it.",
    },
    {
      id: "topology",
      label: "Spatial",
      modality: "spatial",
      hint: "spatial — the scope tree, drawn with d3",
      about:
        "The scope tree these reports attach to — universe down to site — with how much each level carries. This is the shape aggregation rolls up through.",
    },
    {
      id: "plan",
      label: "Board",
      modality: "board",
      hint: "board \u2014 the committed work at this scope, in flow order",
      about:
        "Every plan at this scope on one board: columns are the union of every plan's declared columns (the sample library shares one flow vocabulary), and inside each column the items are grouped by the report they come from — a count first, expandable to the cards. When vocabularies conflict the merge is forced — every column still shows, ordered by average position — and the page says the order is a reading, not a fact.",
    },
    {
      id: "narrative",
      label: "Narrative",
      modality: "narrative",
      hint: "narrative — TL;DR briefings, templated from the reports (proposed)",
      about:
        "The same reports as sentences: what the scope looks like, what wants attention, and what landed recently. Proposed — templated rather than model-written.",
    },
  ],
  schedules: [
    {
      id: "default",
      label: "Schedules",
      hint: "Every cron schedule the control plane holds, by the worker it wakes",
      about:
        "The schedules registered on the control plane, grouped by the worker each one wakes: the cron expression with its next fire, who owns it, and when it last fired. A fire is a message to that worker; what the worker does with it is the worker's business.",
    },
  ],
  admin: [{ id: "default", label: "Admin", hint: "Settings and docs" }],
};

export function getVariant(view: ViewName): string {
  const defs = VIEW_VARIANTS[view];
  const stored = readStored("variant." + view);
  if (stored && defs.some((d) => d.id === stored && d.built !== false)) return stored;
  return defs[0].id;
}

export function setVariant(view: ViewName, id: string): void {
  writeStored("variant." + view, id);
}

export interface Perspective {
  id: string;
  label: string;
  hint: string;
  built: boolean;
  variants: VariantDef[];
}

const MODALITY_LABELS: Record<string, string> = {
  glance: "Glance",
  delta: "Delta",
  storyboard: "Timeline",
  board: "Board",
  spatial: "Spatial",
  narrative: "Narrative",
  conversational: "Ask",
};

export function perspectivesFor(view: ViewName): Perspective[] {
  const defs = VIEW_VARIANTS[view];
  if (!defs.some((d) => d.modality)) {
    return defs.map((d) => ({
      id: d.id,
      label: d.label,
      hint: d.hint,
      built: d.built !== false,
      variants: [d],
    }));
  }
  const order: string[] = [];
  const byModality = new Map<string, VariantDef[]>();
  for (const d of defs) {
    const m = d.modality || d.id;
    if (!byModality.has(m)) {
      byModality.set(m, []);
      order.push(m);
    }
    byModality.get(m)!.push(d);
  }
  return order.map((m) => {
    const variants = byModality.get(m)!;
    const built = variants.filter((v) => v.built !== false);
    return {
      id: m,
      label: MODALITY_LABELS[m] || m,
      hint: variants[0].hint,
      built: built.length > 0,
      variants: built.length ? built : variants,
    };
  });
}

export function perspectiveOf(view: ViewName, variantID: string): string {
  const d = VIEW_VARIANTS[view].find((v) => v.id === variantID);
  return d?.modality || variantID;
}

export function variantForPerspective(_view: ViewName, p: Perspective): string {
  return p.variants[0].id;
}

export function aboutVariant(view: ViewName, id: string): string | undefined {
  return VIEW_VARIANTS[view].find((v) => v.id === id)?.about;
}
