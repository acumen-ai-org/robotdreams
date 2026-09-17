# Onboarding templates

Implementation-level guide to the template library: what a template is, what
it deliberately is not, and how to add one.

```sh
dream node onboard --list-templates
dream node onboard --template linux-onstart-claude
```

## What a template is

`dream node onboard` prints a brief written for an AI agent as the reader:
how to register, message, use storage, handle update announcements. It is
generic on purpose — it describes the contracts and nothing about how you
run a node.

A **template** is the next step down: a concrete, opinionated recipe for one
particular kind of node. `linux-onstart-claude` is a Linux box that listens
for messages on boot and hands each one to an AI coding agent.

**A template is a document. Printing one installs nothing, writes nothing,
and starts nothing.** The output is text; the reader — a person or an agent
— decides what to do with it. There is no `--apply`, no scaffolding
generator, and no state left behind.

## Why it is only a document

`docs/vision/core.md`'s scope test:

> Something backend-specific and swappable belongs in the messaging or
> storage primitive. Something universal and non-negotiable about *running*
> or *trusting* the primitives belongs in the control plane or security
> layer. **Something agent- or work-definition-specific belongs in neither —
> it belongs in whatever plugs into Robot Dreams, not in Robot Dreams
> itself.**

"Start an AI coding agent per message on a Linux box" is as
agent-and-work-definition-specific as it gets, so it fails that test as
*machinery* — and Robot Dreams would stop being agnostic about node runtimes
the moment it shipped an updater, a supervisor, or an installer for one.

It passes as a *starting point*, which is the format
`docs/vision/environment.md` already blesses:

> Definitions ship as **templates**, not samples — copyable starting points
> … an environment file is inherently something a deployment copies and
> adapts, not an illustration to read.

So the library ships recipes and hands them to you. It does not run them,
depend on them, or track whether you followed them.

## What a template is not

**Not an AgentEnvironment.** Easy to conflate, since `configs/templates/`
holds environment templates and both are called templates. They are
different things:

| | AgentEnvironment (`configs/templates/`) | Onboarding template (this) |
| --- | --- | --- |
| Says what | *Where a node stands* — workspace, services, capabilities | *How to stand a node up* — units, scripts, a prompt |
| Contains a prompt | Never | Yes, that is the substance |
| Nature | Declared truth, verified by the control plane | A recipe, verified by nobody |
| Format | One YAML document | A Markdown brief |

`environment.md` is explicit that an environment "says nothing about
prompts, models, or workflows" and is "declared truth about the node rather
than a *provisioning order*." A template brief is precisely a prompt plus a
provisioning order, so it is a sibling concept — not an extension of that
layer, and not a step toward one.

**Not a supported runtime.** A template names tools Robot Dreams does not
own (`claude`, `jq`, `systemd`). Their interfaces can change without this
repo noticing. The briefs present those lines as the adjustable parts they
are.

## `linux-onstart-claude`

A systemd unit runs a listener; the listener follows the message stream and,
per message, starts an AI agent in one shared folder holding a `CLAUDE.md`
and the message itself; the agent reports back over Robot Dreams messaging;
the listener acks.

Three things the brief states plainly, because they are properties of the
design rather than details:

- **The message channel becomes an instruction channel.** Anything that can
  send this node a message can put text in front of an agent running on that
  machine, with whatever access it has. That is what the template is *for* —
  but it should be a decision, not a discovery. The brief carries two
  narrowings (accept only from the org-chart parent; accept only one message
  type) as commented-out lines rather than defaults, because only the
  operator knows what the node is for.
- **Handling is at-least-once.** The listener acks *after* the agent exits,
  so a crash in between re-runs the message. Acking first would lose the
  work instead. There is no visibility timeout, no dead-letter and no claim
  — ack is the only dedupe primitive the messaging contract has.
- **One shared folder** means concurrent messages collide and the previous
  task's files are visible to the next. Fine for a node doing one thing at a
  time; the brief names the symptom so it is recognisable.

### Why it needed `dream message tail --json`

The listener has to ack what it handled, and the human output line does not
carry the message id. Until `--json` existed, the loop could not be written
honestly — and an unacked message is redelivered from the start of the
backlog on every reconnect, so a restart would re-run the agent on every
historical message. `--json` prints one envelope per line (NDJSON), id
included. See `docs/quickstart.md`.

## Adding a template

```
internal/templates/library/<name>/
  meta.yaml    summary + assumptions
  brief.md     the recipe, printed verbatim
```

The library is embedded (`internal/templates/embed.go`) so
`dream node onboard --template` works from an installed binary with no repo
checkout. Briefs are printed verbatim — no Go templating — so what is
reviewed in the repo is exactly what a reader sees.

Two rules, both enforced by tests rather than by care:

- **`meta.yaml` must list what the recipe assumes.** It is printed first, so
  a reader who cannot meet the assumptions finds out before pasting
  anything.
- **Every command a brief prints must be real.** `cmd_onboard.go` has always
  carried the rule "every command in the brief is checked against the real
  flag set — if a flag changes, change the brief"; for templates it is
  mechanical: `TestTemplateBriefsOnlyPrintRealCommands` and
  `TestTemplateBriefFlagsExist` parse every `dream …` line out of every
  brief and check it against the live cobra command tree.

Write in the house voice: second person, bare section headings, two-space
indented commands, and a closing block whose last line is the next action.
