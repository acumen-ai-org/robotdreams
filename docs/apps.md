# Apps

What a node says it serves for a human to open: one URL and a short
description, kept in the central registry and offered as a link from
Mission Control.

```sh
# at registration
dream worker connect --worker-id web-01 \
    --app-url https://build.example.test --app-description "Build dashboard"

# any time after, including to change it
dream worker app set --url https://build.example.test --description "Build dashboard"
dream worker app clear
dream worker app list
```

## What this is

A node often *is* something a person would want to open — a dashboard, a
console, a report page. The org chart already knows the node exists; this
lets the node say where its face is, so an operator looking at the fleet
can go straight there instead of keeping a list somewhere else.

One app per node. The record is the node's own: the write is scoped to the
authenticated caller, so a node describes itself and nothing else, and
there is no way to declare an app on another node's behalf.

## What the control plane does and does not do

It carries the claim. It **never fetches the URL**, never checks that it
resolves or answers, and never verifies that what is there is what the node
said it is. That is the same posture the update contract takes toward a
node's report about its own runtime, for the same reason: a node is the
authority on itself.

The one thing it does check is **shape**: the URL must parse, must be
`http` or `https`, and must have a host. That is not second-guessing the
claim — it is the same kind of check as requiring a well-formed update
kind. It exists because of what happens to this particular string
downstream:

> Mission Control turns it into something an operator clicks. A
> `javascript:` URL in that position is not a false claim about a node — it
> is script running in the dashboard's own origin, offered by anything that
> can register. `data:` and `file:` are the same problem wearing different
> clothes. Only `http` and `https` are safe as a navigation target, so
> everything else is refused at the door.

The check runs twice on purpose: at the API, and again in the dashboard
next to the click. The guard belongs where the danger is, the dashboard may
be talking to an older server, and the cost is one function.

**An app URL is not secret.** `GET /api/apps` is open to any authenticated
token, matching the org chart it sits beside — `GET /api/workers` already
hands every connected worker every other worker's id, role, edges and
metadata (see [security-model.md](security-model.md)). Gating an app URL
behind admin would protect nothing while stopping the dashboard from
working with a worker token.

## Why it is not metadata

`dream worker connect --metadata` looks like the obvious home, and is not.

A second connect for a worker that is already registered is refused with
**409 Conflict**, not merged — so metadata can only ever be written at a
node's *first* registration, and can never be changed for the life of that
registration. A node that learns its own URL later, or moves it, would have
to be revoked and re-enrolled.

The same reasoning put update state in its own table:

> This is deliberately NOT `workers.metadata_json`: `UpsertWorker` replaces
> that column wholesale, so state living there is destroyed by the next
> write that does not carry it forward.

So an app is its own record, keyed by worker, written through its own
self-scoped endpoint. Setting it again replaces it wholesale — an app is
one statement a node makes about itself in full, not an accumulation of
partial reports, so there is no field-preserving merge here (unlike
`worker_update_state`, where there is one and it is load-bearing).

## HTTP surface

| Route | Auth | Notes |
| --- | --- | --- |
| `POST /api/apps` | any worker | Sets the **caller's own** app. No `worker_id` field; supplying one is a 400. |
| `DELETE /api/apps` | any worker | Clears the caller's own app. Clearing a nonexistent one is not an error. |
| `GET /api/apps` | any worker | Every declared app. |

Storage is `worker_apps`, one row per node, deleted alongside the worker so
a re-registered id does not inherit a ghost.

## In Mission Control

**Node detail** shows an "App" block with a link and the description. It is
placed above the reporting-scope sections, because a node with no reporting
scope can still serve something.

The link is deliberately *not* styled like the `.detail-chip` links above
it. Every one of those stays inside the dashboard; this one leaves it for a
destination the node chose, so it shows **the host it will actually open**
and carries an outward arrow rather than blending in. It opens in a new tab
with `rel="noopener noreferrer"` — `noopener` so the opened page cannot
reach back through `window.opener`, `noreferrer` so the dashboard's own URL
is not handed to a node-controlled site.

**The tree and network views** mark nodes that serve something with a small
outward arrow, so you can see which nodes have one without clicking each in
turn. It is a mark, not a link: those rows and circles are selection
controls, and a second click target inside them would fight the one that
selects the node.

**Preview** embeds the app in the panel, in a sandboxed frame. The sandbox
grants scripts and the app's own origin — almost nothing useful runs
without them — and deliberately withholds `allow-top-navigation`, so a
framed page cannot move the dashboard out from under the operator.
`referrerpolicy="no-referrer"` keeps the dashboard's own URL from reaching
a node-controlled site.

There is no way to detect, cross-origin, that a site refused to be framed:
the load event fires either way and the document cannot be read. So rather
than guess, the note under the frame says plainly what a blank frame means.

### What this costs the offline guarantee

Mission Control's standing constraint is that it loads nothing from the
network of its own accord — fonts and every library are bundled and
embedded. **Previewing an app is the one exception, and it is
operator-initiated:** the frame is behind a button, so until someone asks
for a specific node's app, the dashboard still fetches nothing from
anywhere. Opening the link in a new tab is not an exception at all — that
is a navigation the operator makes, not a request the page makes.

Nothing else about an app is ever fetched: no favicon, no preview
thumbnail, no prefetch, no health check.

## Limits worth knowing

- **One app per node.** A node that serves several things has to pick one,
  or point at an index page. Widening the key to `(worker_id, name)` is the
  obvious extension and would not change the wire shape much.
- **Nothing checks the URL is alive.** A stale app link stays in the
  registry until the node changes or clears it. The control plane will not
  discover that for you.
- **The description is capped** at 280 characters, so one node cannot push
  a wall of text into every operator's panel.
