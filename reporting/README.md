# Reporting

Technical report capability contracts — the standard binding that lets
a hundred different reports aggregate into one dashboard, one timeline,
one pulse. Design rationale: `docs/vision/reporting.md`.

Two directories, same pattern as `configs/templates/` for environments:

- **`contracts/`** — the base vocabulary, versioned. Facet shapes,
  standard categories, visual primitives, consumption modalities,
  aggregation policies, media archetypes. These are what dashboards
  and other consumers are written against; extend them deliberately.
- **`library/`** — ready-to-use `ReportDefinition`s. Nine of them are
  the generic template per standard category (`activity`, `cost`,
  `decisions`, `delivery`, `incident`, `logs`, `performance`,
  `quality`, `roadmap`) — `roadmap` has several, because a plan is read
  at more than one altitude: `plan` and `strategy` above the work,
  `task-board`, `backlog` and `todo` at it. The rest are the reports a
  real company's departments actually produce — a month-end close, a budget bridge, a
  hiring funnel, a support queue, a subscriber bridge, a compliance
  matrix, a security posture, a company scorecard. Every one of them
  maps onto the same nine categories, which is the whole claim of the
  contracts: a hundred different reports, one set of filters. Copy one,
  rename it, adapt the data contract and aggregation policies. A
  definition may `extends:` another to inherit its facets and override
  selectively.

The distinction that matters:

- A **ReportDefinition** (these files) declares a report *type*: its
  facets, categories, data contract, and scope aggregation. Registered
  with the control plane (`dream report apply <file>`).
- A **report instance** is a data payload conforming to a definition,
  produced by a node at a scope. A definition may declare a plan;
  instances report items against it. Nothing here authors or mutates
  one — see `docs/vision/hierarchy.md`. Instances live in the library (the
  storage primitive) and are announced by pointer messages — ordinary
  spine traffic.

Consumers (Mission Control's Reporting section, media generators,
conversational debriefs) are written once, against the contracts —
never against individual reports. If a new report type requires
touching a renderer, the abstraction has leaked; file a bug.
