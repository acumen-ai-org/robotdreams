# Perspective: Organizational Hierarchy (Universe → Worlds → Realms → Sites)

> *"A universe big enough for wherever your dreams may take you."*

An optional, high-level way to organize nodes ([[core.md]]) and chain
reporting up and down. This is a different axis from the *work-item*
hierarchy in [[hierarchy.md]] (Goal → Initiative → Workstream → Task):
that one is about how work breaks down; this one is about where nodes
live and how their reporting rolls up. Don't conflate them.

## The four levels

```
Universe
  └── World
        └── Realm
              └── Site        ← where work gets done
```

A Universe contains Worlds, Worlds contain Realms, Realms contain
Sites. Sites are where work actually happens — but see the two
principles below before reading that as a rule.

A business mapping, as one worked example rather than the definition:

- **Universe** — you, the entrepreneur.
- **World** — your company (or companies).
- **Realm** — a department: Dev, CS, Sales…
- **Site** — where the work actually happens.

## Two load-bearing principles

1. **A single node working alone in one Site is a complete, valid
   deployment.** The hierarchy shows what *can* exist, not what must.
   Nothing about Robot Dreams gets simpler to reason about, or harder
   to trust, because a deployment declined the upper levels.
2. **Work can happen at any level, and no level is mandatory.** A node
   can live and work in a World or a Realm just as legitimately as in
   a Site. Whoever implements chooses how much structure to use — but
   the ceiling is deliberately high. Think big.

## Nodes, not containers

The hierarchy is made of containers; the actors in it are **nodes** —
the entities that connect themselves and do the communicating (see
[[core.md]] for the term, and for the mapping node = *worker* in the
implementation's vocabulary). Nodes can exist at any level, and the
reporting structure is defined **node-to-node**, not
container-to-container. That means reporting edges may skip levels: a
node down in "Site X-001" can report directly to a boss-node up in
"World X". The containers organize; the nodes report. See
[[control-plane.md]] for how this reconciles with the reference
implementation's single-parent org chart — short version: it already
is exactly this, edges between nodes.

Because the spine embodies the ecosystem rather than standing beside
it ([[core.md]]), the hierarchy is never depicted as a structure wired
into a separate spine object. The messaging substrate is alive at
every level of it, and the library is reachable from every level of it
— a node in a Site and a node in a World deposit and retrieve books
from the same shelves.

## What the implementation does and doesn't enforce

To keep this doc truthful: **the four named levels are a conceptual /
organizational model, not something the Go reference implementation
enforces today.** The build has a flat worker registry with
reports-to edges between workers — no Universe/World/Realm/Site
entities, no container modeling at all. That's not a gap so much as
the model showing through: since reporting is node-to-node by design,
the graph the implementation already keeps *is* the load-bearing
structure, and the four levels are a way of naming and reading the
shape a deployment grows into. Whether the levels ever become
first-class entities (labels on workers? namespacing? nothing?) is
open, and deliberately so — resolving it before a real deployment
needs it would be inventing requirements.

See also: [[core.md]] for nodes, the library, and the substrate;
[[control-plane.md]] for the org chart this hierarchy is read on top
of; [[hierarchy.md]] for the separate work-item hierarchy.
