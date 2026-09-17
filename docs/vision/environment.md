# Perspective: Agent Environment (declarative environment definition)

> **This extends the fixed operational layer with one new swappable
> dimension.** The environment *definition* — the Environment API, the
> capability registry, and the policy/identity/lifecycle rules around
> them — lives in the control plane ([[control-plane.md]]) and is the
> same shape for every deployment. What *realizes* a definition — a
> Kubernetes cluster, a VM pool, a laptop — is a **runtime adapter**,
> pluggable exactly the way messaging and storage backends are
> ([[core.md]]). Robot Dreams still does not execute agents; it now
> lets a deployment *describe* where its agents stand.

The spine says a node can be anything and run anywhere. That freedom
has an operational cost the messaging and storage primitives don't
cover: every node shows up carrying its own undocumented world — which
repo it has checked out, which database it can reach, which tools and
credentials it happens to hold. Nothing in the system can answer "what
does this node have access to, and was that on purpose?"

The **AgentEnvironment** is the answer: a single YAML document that
declares everything around a node that is not the node itself.

## The model

```
                     ┌─────────────────────┐
                     │ Agent Control Plane │
                     │                     │
                     │ Environment API     │
                     │ Capability Registry │
                     │ Policy              │
                     │ Identity            │
                     │ Lifecycle           │
                     └──────────┬──────────┘
                                │
                       AgentEnvironment
                                │
                ┌───────────────┼───────────────┐
                │               │               │
            Workspace        Services       Capabilities
                │               │               │
             Git repo       PostgreSQL       GitHub
             Filesystem     Redis            Browser
             Processes      APIs             Shell
                │               │             MCP
                └───────────────┼───────────────┘
                                │
                         Runtime Adapter
                          /           \
                        K8s            VM
                         │              │
                         └──────┬───────┘
                                │
                          Agent processes
                                │
                     ┌──────────┼──────────┐
                     │          │          │
                   Agent      Agent      Agent
                     A          B          C
```

An environment has exactly three branches:

- **Workspace** — the mutable working surface a node sees: git
  checkouts, filesystem mounts (scratch space, plus mounts backed by
  the storage primitive — the library, [[core.md]]), and long-running
  helper processes (a dev server, a watcher).
- **Services** — stateful or external dependencies provisioned or
  connected *beside* the node: databases, caches, third-party APIs.
  A service is something the node talks to over a connection string
  it did not create.
- **Capabilities** — granted abilities, resolved through the control
  plane's **capability registry**: shell, browser, GitHub, MCP
  servers. A capability is scoped (which repos, which permissions) and
  carries an identity story — the credential is minted per-node by the
  control plane where possible ([[security.md]]), not baked into the
  document.

The three answer different questions — *what am I standing in*
(workspace), *what can I reach* (services), *what am I allowed to do*
(capabilities) — and a definition may use any subset. An empty
environment is valid; it describes today's status quo.

## Runtime adapters: the new pluggable dimension

A definition is inert until a **runtime adapter** realizes it. The
adapter contract is deliberately the same shape as the
messaging/storage backend contracts: a small interface (realize,
verify, dispose, report), many implementations:

- **k8s** — provision the environment as pods/volumes in a cluster.
- **vm** — provision onto a VM pool.
- **local** — realize on the machine where the node runs (spawn the
  processes, run the checkouts, wire the services via containers).
- **external** — the node brings its own runtime; the control plane
  only *verifies and records* that the environment's claims hold.

The `external` adapter is what keeps the original promise honest: a
single agent on a laptop remains a complete, valid deployment
([[core.md]]). It attaches with `--environment <name>`, the control
plane checks what it can, and the environment document becomes the
*declared truth* about that node rather than a provisioning order.

## The scope test, applied honestly

[[core.md]]'s test: backend-specific and swappable → a primitive;
universal about running/trusting the primitives → control plane or
security; agent- or work-definition-specific → not Robot Dreams.

The environment layer passes on the same two-tier reading the control
plane did. *What an agent is* and *how work is defined* remain
outside — an environment says nothing about prompts, models, or
workflows. *Where an agent stands* turns out to be a control-plane
concern the moment more than one node exists: policy ("no node gets
GitHub write without review"), identity (who minted this credential),
and lifecycle (this environment expires in 8 hours) are exactly the
universal, non-negotiable operational questions the control plane
already owns for connectivity. The swappable part — provisioning —
stays behind the adapter contract, exactly like backends behind
primitives. If defining an environment ever requires knowing which
adapter will realize it, the abstraction has leaked and this layer has
quietly become a stack.

Like the control plane and the org chart before it, this is a
deliberate, explicit extension on top of the two-primitive spine — a
recorded decision, not scope drift.

## The YAML shape

```yaml
version: v1alpha1
kind: AgentEnvironment
name: build-standard

workspace:
  git:
    - repo: https://github.com/acumen-ai-org/robotdreams
      ref: main
      path: /work/robotdreams
  filesystem:
    - name: scratch
      path: /work/scratch
      ephemeral: true
    - name: shared
      path: /work/shared
      library: default          # backed by the storage primitive
  processes:
    - name: dev-server
      command: ["make", "dev"]
      workdir: /work/robotdreams
      restart: on-failure

services:
  - name: postgres
    type: postgres
    version: "16"
  - name: sum-api
    type: http
    url: https://api.example.com
    auth: { secretRef: sum-api-token }

capabilities:
  - use: shell
  - use: browser
  - use: github
    with:
      repos: [acumen-ai-org/robotdreams]
      permissions: [contents:write, pull_requests:write]
  - use: mcp
    with:
      servers:
        - name: serena
          command: ["serena", "serve"]

runtime:
  adapter: k8s                  # k8s | vm | local | external
  k8s:
    namespace: dreams
    resources: { cpu: "2", memory: 4Gi }

policy:
  network: { allow: [github.com, registry.npmjs.org] }
  maxLifetime: 8h

lifecycle:
  idleTimeout: 30m
  onDispose: snapshot-workspace # deposit final state in the library
```

`runtime`, `policy`, and `lifecycle` are deployment concerns layered
onto the portable three-branch core; the same `workspace / services /
capabilities` document should realize under any adapter.

## Operational surface

Environments are first-class control-plane state, owned like the org
chart ([[control-plane.md]]) — available even when a storage backend
is degraded, exposed read-only to everything else:

```
dream env apply my-environment.yaml
dream env list
dream env status build-standard
dream worker connect --server <id> --environment build-standard ...
```

Definitions ship as **templates**, not samples — copyable starting
points under `configs/templates/`, one per adapter shape
(`environment-k8s.yaml`, `environment-external.yaml`): an environment
file is inherently something a deployment copies and adapts, not an
illustration to read.

Mission Control shows environments the same way it shows the org
chart: which definitions exist, which nodes stand in which
environment, what each was granted, and drift — where a node's
verified reality no longer matches its declared environment.

## Open questions

- **Push or pull?** Does the control plane drive adapters
  (provisioning as a control-plane act), or do adapters watch the
  Environment API and reconcile (Kubernetes-operator style)? The
  reconcile model degrades better and keeps the control plane a
  registry rather than an orchestrator, but makes "is it ready yet?"
  an eventually-consistent answer.
- **Shared or per-node?** Is an environment a template many nodes
  instantiate (each with its own workspace copy), a shared standing
  space several nodes cohabit, or both — and if both, which parts are
  per-node (scratch, identity) versus shared (services)?
- **Capability credentials.** Where the control plane can mint scoped,
  short-lived credentials (GitHub apps, database roles) it should;
  where it can only carry a `secretRef`, rotation and revocation need
  the [[security.md]] treatment rather than a config field.
- **Verification depth for `external`.** What can the control plane
  actually check about a self-managed runtime beyond "the node says
  so" — and how is unverifiable-but-declared surfaced in Mission
  Control without pretending to a guarantee that isn't there?
- **Hierarchy interaction.** Can an organizational container
  ([[organization.md]]) set a default or a policy ceiling for
  environments of nodes within it (a Realm-wide network allowlist)?

See also: [[core.md]] for the spine and the scope test this layer must
keep passing, [[control-plane.md]] for the operational surface it
extends, [[security.md]] for identity and credential lifecycle,
[[organization.md]] for the hierarchy environments may one day be
scoped by.
