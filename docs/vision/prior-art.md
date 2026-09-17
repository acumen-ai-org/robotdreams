# Perspective: Prior Art & Comparisons

Systems and ideas worth understanding as reference points — not to copy,
but to know where Robot Dreams agrees or deliberately diverges.

## Agentic memory systems

### Graphiti vs Mem0

**Graphiti** (Zep) is a temporal knowledge graph on Neo4j: entities are
nodes, facts are natural-language-rich edges with `valid_at`/
`invalid_at` timestamps — outdated facts are marked invalid, not
deleted, so full history survives. **Mem0** splits storage instead: a
vector store of atomic fact strings (primary) plus an optional graph
layer of thin entity→relation→entity triples with no natural-language
context — the two stores use separate IDs and can drift out of sync
with each other.

A benchmark comparing them (identical inputs/models, both systems) found
Graphiti scored higher on knowledge coverage (4.75/5 vs 3.25/5) and
contradiction handling (4.75/5 vs 3.0/5), at ~1.68x the token cost (87k
vs 52k). The **"context blindness" failure** is specific to Mem0's
split architecture: top-K crowding (emotionally-weighted embeddings
outrank relevant facts) and structural blindness (the graph layer knows
an entity matters but can't retrieve its facts because the graph and
vector stores aren't linked) — demonstrated when an update landed in
the graph layer but stale context still surfaced from the vector store.
Conclusion: Mem0 suits cheap, simple recall; Graphiti justifies its cost
for deeply interconnected reasoning; a hybrid (graph backbone + selective
vector RAG) is the pragmatic middle.

**Relevance to Robot Dreams:** the split-store drift Mem0 exhibits is
exactly the failure mode to design out of the git + Filestore/Postgres
split in [[memory.md]] — see design implication #2 there.

### "Agentic Memory: Seven Different Systems" (Sanjay Basu)

A taxonomy organized by retention horizon and read latency:

1. **Working Memory** — active context/KV cache, one request, µs–ms.
2. **Session Memory** — rolling scratchpad + running summaries,
   minutes–hours.
3. **Episodic Memory** — immutable timestamped event/tool-call log,
   months–years, cold path.
4. **Semantic Memory** — distilled facts via vector search, long-term.
5. **Relational Memory** — knowledge graph with temporal validity over
   facts.
6. **Procedural Memory** — cached successful plans/trajectories, the
   only tier that reduces cost over time.
7. **Durable State** — workflow checkpoints; explicitly "not cognitive
   memory" but required for tasks exceeding request timeouts.

Key caveat: benchmark scores are harness-dependent (the same underlying
Mem0 system scored 94.4 / 66.9 / 59.8 across three different harnesses),
so vendor benchmark numbers shouldn't be trusted across contexts.

**Relevance to Robot Dreams:** this taxonomy is the cleanest available
mapping for what [[architecture.md]] and [[memory.md]] are actually
building — see [[core.md]] for how Robot Dreams' concrete stores line
up against these seven tiers.

## Hermes AI

"Hermes AI" most likely refers to **Hermes Agent** by **Nous Research**
(launched Feb 2026) — the agent-product evolution of Nous's earlier
Hermes model series (open-weight LLMs fine-tuned for agentic/
function-calling use on Llama/Mistral/Qwen bases). It grew fast
(140k+ GitHub stars) and is reportedly the most-used agent on
OpenRouter.

**What it is:** a persistent, always-on personal/dev agent, run
locally or self-hosted, designed to accumulate competence over time
instead of starting fresh every session.

**Architecture, as documented:**
- **Persistent memory** — curated `MEMORY.md` / `USER.md` files
  (project facts, environment, preferences) injected into the prompt
  at session start. File-based, not a vector DB or structured store.
- **Self-improving skills** — when it solves a hard problem, it writes
  a reusable skill document (agentskills.io-compatible) — a crude
  process/outcome memory analog.
- **Multi-agent orchestration** — spawns isolated sub-agents for
  parallel workstreams, each its own conversation/terminal; a
  community extension coordinates ~17 specialized agents (research,
  planning, implementation, QA, infra).
- **Scheduling** — built-in cron for recurring unattended jobs,
  delivering output to Telegram/Discord/Slack/WhatsApp/CLI.
- Model-agnostic, local/on-device-optimized; newest feature is "Bot
  Mode" (named bot profiles, each with its own memory).

**How it compares to Robot Dreams:**
- Hermes is single-node/local-first and session-oriented — durability
  comes from flat memory files and skill docs, not a workflow engine.
  Robot Dreams' Temporal-based Workflows give real durable execution
  (replay, retries, versioned history) with no Hermes equivalent.
- Hermes's "memory" is prompt-injected markdown, not a structured,
  leased/versioned shared store — see [[architecture.md]]. Robot
  Dreams' GKE+Filestore shared filesystem with leases/locks/revisions
  is a materially stronger consistency model for *concurrent*
  multi-agent access, which Hermes doesn't need to solve (it's not
  multi-worker over shared state).
- Hermes's multi-agent spawning is lightweight parallel-conversation
  style, not a Factory of workers under durable workflow semantics.
- Good prior art for the *concept* of persistent process/outcome
  memory and self-written skills (see [[memory.md]]) — but
  architecturally a hobbyist/self-hosted personal-agent product, not
  an enterprise orchestration substrate. Useful talking point, not a
  direct competitor.

Sources: Arize ("How Hermes implements an open source agent harness
architecture"), NVIDIA Blog ("Hermes Unlocks Self-Improving AI
Agents"), hermes-agent.org, get-hermes.ai, Nous Research (Hermes 3),
MarkTechPost (Nous Research Ships Bot Mode), `opencode-hermes-multiagent`
on GitHub.

## OpenClaw

Open-source, self-hosted personal AI agent framework (formerly
Clawdbot, briefly MoltBot; originated by developer Peter Steinberger,
first released ~November 2025). Grew extremely fast (100k+ GitHub
stars per multiple sources).

**What it is:** an "agent application" you deploy and run
continuously, more than a library you import — the opposite pole from
a dev-tool primitive like LangChain/CrewAI.

**Architecture, as documented:**
- **Execution** — a single, deliberately simple ReAct loop
  (reason → act → observe) against plain LLM API calls plus a
  filesystem. No graph/DAG orchestration layer, no
  LangChain/CrewAI/LangGraph dependency. The agent decides at runtime
  whether to call tools, spawn sub-agents, compact memory, or route
  between sessions — orchestration is LLM-driven, not a fixed workflow
  engine.
- **Coordination/messaging** — a multi-channel gateway wired into
  external chat platforms (Telegram, Discord, WhatsApp, Slack, Signal,
  Matrix, Nostr). This is messaging *to/from humans and channels*, not
  a peer-to-peer agent-to-agent bus.
- **State/memory** — plain files, git-tracked. `SOUL.md` defines
  persistent identity/personality, `MEMORY.md` holds curated long-term
  facts, `memory/YYYY-MM-DD.md` daily logs accumulate session history.
  Hybrid BM25 (semantic + keyword) retrieval with auto-flush before
  context compaction. Agents edit their own instruction files and
  commit changes autonomously — memory and configuration share one
  substrate.

**How it compares to Robot Dreams:** OpenClaw is architecturally the
inverse of a two-primitive spine. It bundles a specific execution
model (ReAct loop), a specific coordination surface (chat-platform
gateway, not an abstract messaging primitive), and a specific storage
implementation (git-tracked markdown + BM25 index) into one
opinionated, single-agent-focused product. There's no clean separation
letting you swap in a different runtime, messaging transport, or
storage backend — those choices are load-bearing and intertwined with
its identity/memory design, whereas Robot Dreams stays agnostic on all
three. Like Hermes, its git+markdown memory is independent validation
of the storage bet in [[memory.md]].

Sources: github.com/openclaw, agor.live/blog/openclaw,
sfailabs.com/guides/openclaw-ai-agent-framework.

## Why this matters for Robot Dreams

Robot Dreams' storage model in [[memory.md]] is closer to "git + a
shared workspace filesystem" than to a purpose-built agent-memory
product (vector DB, knowledge graph). That's a deliberate bet: durable,
human-reviewable, diffable artifacts over an opaque retrieval index —
and it lands squarely on the "procedural + semantic memory via git,
working/session/durable-state memory via Filestore+Postgres" shape that
the research below independently arrives at. See [[core.md]] for the
synthesis.
