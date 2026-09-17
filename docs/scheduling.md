# Scheduling

A **schedule** is a standing instruction to the control plane: on a cron
cadence, send one message from a worker to a worker. The control plane
keeps the appointment and nothing more — it does not know what the
message means, and it runs nothing itself. The recipient's inbox is where
the work starts, exactly as it would for any other message.

This is the control plane's first background activity. Everything else it
does happens in response to a request; a schedule is the one thing that
has to happen when nobody is asking.

## The model

`pkg/scheduling.Schedule`:

| field        | meaning                                                                                                  |
| ------------ | -------------------------------------------------------------------------------------------------------- |
| `id`         | client-chosen (`PUT`) or minted (`POST`); becomes the `causation_id` of every message the schedule sends |
| `owner`      | the registered worker the message is sent `from`                                                         |
| `to`         | the registered worker it is delivered to; defaults to the owner                                          |
| `cron`       | five fields — `minute hour day-of-month month day-of-week` — UTC unless prefixed `TZ=<location> `        |
| `subject`    | copied onto every message                                                                                |
| `body`       | JSON, copied onto every message                                                                          |
| `enabled`    | `false` keeps the schedule on file without firing it                                                     |
| `next_at`    | when it is next due (UTC)                                                                                |
| `last_at`    | when it last fired; absent until it has                                                                  |
| `created_at` | first registration; a re-registration keeps it                                                           |

Cron parsing is `github.com/robfig/cron/v3`'s standard parser: the five
fields, `@daily`-style descriptors, and a `TZ=` (or `CRON_TZ=`) prefix,
e.g. `TZ=Europe/Stockholm 0 9 * * 1-5`. There is no seconds field —
minute resolution is what a cron line means to the people writing them,
and the scheduler's tick is coarser than a second anyway.

## The message

When a schedule comes due the control plane emits one envelope of kind
**`scheduled`** (`messaging.TypeScheduled`):

```
type:         scheduled
from:         <owner>
to:           <to>
subject:      <subject>
body:         <body>
causation_id: <schedule id>
```

Routing is the simplest rule in `Server.EmitFromWorker`: exactly `to`,
which must be a registered worker, with no default and no hop to a parent.
The control plane is the only sender — `POST /api/messages` refuses the
kind, because accepting it would let any worker forge a schedule's
`causation_id`. A recipient tells schedules apart by that id and acks the
message like any other.

## Where schedules live

Schedules are control-plane state, kept in the control plane's own
datastore (`<data-dir>/control.db`, table `schedules`) beside the org
chart — not in the messaging backend. A schedule names two registered
workers and is owned by the server that fires it; the messaging backend
(embedded SQLite, Temporal, webhook — see
[messaging-backends.md](messaging-backends.md)) is a swappable transport
that would otherwise each need its own copy of the table. Every backend
therefore supports schedules identically; the fired message goes through
whichever backend is configured.

## Firing

`internal/server.Scheduler` runs beside the HTTP server (started by
`dream server init`, stopped with it) and ticks every **30 seconds**:
list every enabled schedule with `next_at <= now`, send its message,
advance `next_at` to the next occurrence after now, record `last_at`.

Two deliberate choices:

- **No catch-up.** A schedule that fell due while the server was down is
  advanced to its next occurrence at startup, with a log line, and not
  fired. A recurring message is a nudge — "it is nine o'clock, collect the
  ideas" — and a stack of twelve nudges delivered at once after an outage
  is noise the recipient would have to de-duplicate itself. Missed
  occurrences are not replayed.
- **A failed send still advances.** If the message cannot be emitted
  (the recipient was revoked, say) the failure is logged, `next_at`
  moves on and `last_at` is left where it was, so a dead recipient costs
  one log line per occurrence rather than one per tick.

Every fire — from the tick or from a manual `fire` — is broadcast on the
Mission Control event stream `GET /api/events` as a `schedule_fired`
event:

```json
{
  "schedule_id": "sched-spookify-podcasts-idea-collection",
  "owner": "idea-coordinator-01",
  "to": "idea-coordinator-01",
  "subject": "idea-collection",
  "message_id": "…",
  "fired_at": "2026-09-14T09:00:00Z",
  "next_at": "2026-09-15T09:00:00Z"
}
```

## API

| route                            | who                                      | what                                                                           |
| -------------------------------- | ---------------------------------------- | ------------------------------------------------------------------------------ |
| `PUT /api/schedules/{id}`        | registered identity with `message:send`  | upsert under a client-chosen id                                                |
| `POST /api/schedules`            | same                                     | the same, under a minted id                                                    |
| `GET /api/schedules[?worker_id]` | any authenticated token                  | every schedule, or those `worker_id` owns or receives                          |
| `GET /api/schedules/{id}`        | any authenticated token                  | one schedule                                                                   |
| `DELETE /api/schedules/{id}`     | owner or admin                           | remove it; it stops firing                                                     |
| `POST /api/schedules/{id}/fire`  | owner or admin                           | send the message now; `last_at` is recorded, `next_at` is **not** moved        |

Request body for `PUT`/`POST`:

```json
{
  "to": "leaf-1",
  "cron": "TZ=Europe/Stockholm 0 9 * * 1-5",
  "subject": "idea-collection",
  "body": { "scope": "spookify/podcasts", "workflow": "idea-collection" },
  "enabled": true,
  "owner": "lead-1"
}
```

`to` defaults to the caller; it must name a registered worker (400
otherwise). `owner` is honoured only from an admin token — the way an org
sync registers schedules on behalf of the workers it manages — and must
itself be registered. `enabled` defaults to `true`. A **delegated child**
is refused (403): it has no org-chart entry to send from, and it will be
gone before the schedule fires. Re-registering an id owned by another
worker is refused unless the caller is an admin.

The response, and every element of a `GET` list's `{"schedules": [...]}`,
is the schedule with the keys of the table above: `id, owner, to, cron,
subject, body, enabled, next_at, last_at, created_at`.

**Reads are open** to any authenticated token — a delegated child
included — the same posture as `GET /api/workers`. A schedule is part of
the org's shape ("who is nudged when"), not a private inbox, and the
read-only child token a dashboard proxy holds owns no schedules itself;
a self-scoped rule would leave that view empty. Changing a schedule stays
with its owner (or an admin).

`fire` answers `202` with `{"fired": <schedule_fired event>, "message":
<the envelope as sent>}`.

## CLI

```
dream schedule create --to leaf-1 --cron "0 9 * * 1-5" --subject idea-collection \
    [--body-file body.json] [--id sched-ideas] [--owner lead-1] [--disabled]
dream schedule list [--worker leaf-1] [--json]
dream schedule delete sched-ideas
dream schedule fire sched-ideas
```

`create` with `--id` is a `PUT` and idempotent — the same command on
every startup keeps one schedule, updating its definition and leaving its
history in place; without `--id` it is a `POST`, and running it twice
registers two. `--body-file -` reads the body from stdin.

## What this is not

- **Not a job runner.** Nothing executes on the control plane. A
  deployment that wants "run this workflow at nine" registers a schedule
  whose body names the workflow, and its worker's inbox handling does the
  rest.
- **Not durable across missed time.** See "No catch-up" above.
- **Not a calendar.** One cron line per schedule; a task with two
  cadences is two schedules.
