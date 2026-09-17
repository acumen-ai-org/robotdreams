# Perspective: Work Hierarchy (Strategy Down to Subtask)

> **Two hierarchies, two docs.** This doc is about the *work-item*
> hierarchy — how work breaks down from strategy to subtask. The
> *organizational* hierarchy — Universe → Worlds → Realms → Sites,
> where nodes live and how reporting chains up — is a different axis
> entirely: see [[organization.md]]. The two shouldn't be conflated —
> one decomposes work, the other organizes the nodes doing it.

A generic planning hierarchy, independent of any tool, that Robot Dreams'
own vocabulary (Factory / Workflow / Agent) needs to sit inside of
coherently:

```
Goal / Objective
  └── Initiative / Programme
        ├── Workstream
        │     ├── Activity / Work Package
        │     │      ├── Task
        │     │      │    └── Subtask
        │     │      └── Task
        │     └── Activity
        └── Workstream

Milestones mark progress across the hierarchy (cross-cutting, not a level).
```

## Two questions this hierarchy answers

**"Are we doing the right thing?"** (strategy / portfolio layer)

```
Strategy → Goals/Objectives → Initiatives → Portfolio →
Programme/Project selection
```

**"Are we doing it right?"** (delivery / execution layer)

```
Project → Workstreams → Work Packages → Activities → Tasks → Subtasks
```

The line between the two is the line between *choosing* work and
*doing* work — portfolio management above it, project/task execution
below it.

## Where Robot Dreams sits

Robot Dreams' own nouns — **Factory**, **Workflow**, **Agent** — live on
the execution side ("are we doing it right?"). A Workflow is roughly a
Temporal-durable Task/Activity-equivalent: a concrete, executable unit
with a defined start and end, run by an Agent, inside a Factory.

What's *not yet placed* is how (or whether) Robot Dreams represents
anything above that line — Goals, Initiatives, Programmes. Options to
resolve:

1. Robot Dreams stays deliberately scoped to the execution layer only,
   and integrates with whatever the customer already uses for
   strategy/portfolio (Jira epics, Linear projects, a roadmap tool) —
   Workflows just point at an external ID.
2. Robot Dreams grows a lightweight version of the upper layers itself,
   so a Workflow can trace up to the Goal it serves, and dashboards can
   answer "are we doing the right thing?" as well as "is this Workflow
   healthy?"

This is a scope decision, not a detail — it changes what the product is.

**Resolved for the reference implementation**: option 1 — the Go build
represents no work hierarchy above Worker/Message/Storage at all. No
Goal/Initiative/Programme modeling, no external-ID pointer field, not
even a stub. This follows directly from core.md's own leaning answer
and keeps the initial build honest about what it does and doesn't own.
This is a decision for *this build*, not a closed question for the
project — option 2 remains legitimate future scope if a deployment
genuinely needs Robot Dreams to answer "are we doing the right thing?"
and not just "is this Workflow healthy?"

**Still true after the `plan` facet.** [[reporting.md]] later grew a
seventh facet that carries work items — id, title, state, owner, due,
size, `blocked_by`, and a `level` drawn from the vocabulary at the top of
this page. That looks at first glance like the modelling this section
declined, and it is worth being precise about why it is not.

The reporting layer *reports* plans; it does not *hold* them. There is no
item store (items ride inside a report instance, which the control store
already persists as opaque payload), no endpoint that mutates an item, no
identity tracked for one across two instances, and no parent link — the
one field that would actually build a hierarchy. `level` is a label, so
declaring an item a "goal" says how to read it and creates nothing. A
deployment that runs Jira or Linear has a node that publishes a board the
same way its finance node publishes a budget variance.

So the dashboard can now answer "are we doing the right thing?" from
what nodes tell it, while Robot Dreams still owns nothing above
Worker/Message/Storage. Option 1 stands. If a later change adds an
endpoint that creates or moves an item, or a field that nests one inside
another, that is option 2 being taken — and this page should say so
rather than the change arriving quietly.

See also: [[core.md]] for how this should shape the "what we want to
achieve" framing.
