import type { ViewTarget } from "./ViewBar";

export const PLACE_TARGETS: ViewTarget[] = [
  { id: "explore", label: "Explore", view: "overview", hint: "Walk into it, one level at a time" },
  { id: "galaxy", label: "Galaxy", view: "overview", hint: "See it given shape, with what it contains" },
  { id: "panels", label: "Glance", view: "reporting", hint: "Every report at this scope, at a glance" },
  { id: "timeline", label: "Timeline", view: "reporting", hint: "What has happened here, in order" },
  { id: "topology", label: "Spatial", view: "reporting", hint: "Where it sits in the tree reports roll up through" },
  { id: "plan", label: "Board", view: "reporting", hint: "The committed work at this scope" },
  { id: "narrative", label: "Narrative", view: "reporting", hint: "The same reports, written out" },
];

export const NODE_TARGETS: ViewTarget[] = [
  { id: "explore", label: "Explore", view: "overview", hint: "Find it in the place it reports for" },
  { id: "galaxy", label: "Galaxy", view: "overview", hint: "See it inside the place it belongs to" },
  { id: "force", label: "Network", view: "overview", hint: "See who it reports to, and the traffic on that line" },
];
