# KGraph — Kubernetes Knowledge Graph

> **"Why is my Deployment broken?" answered in one command.**

KGraph is a **local-first Kubernetes observability tool** that turns your cluster into an intelligent, queryable knowledge graph. It answers *why / what-depends-on-this / what-breaks-if-I-change-this* through **deterministic graph traversal — no AI, no CRDs, no cluster-side installation**.

Use it as a **CLI**, a fullscreen **terminal UI (TUI)**, or a browser-based **Web UI** (`kgraph serve`).

---

[![Go version](https://img.shields.io/badge/Go-1.21%2B-00ADD8?style=flat-square&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey?style=flat-square)](#installation)

---

## Why KGraph?

| Question | kubectl | k9s | KGraph |
|---|---|---|---|
| Why is this Pod failing? | `kubectl describe` + manual reading | Events panel | **Root-cause analysis with diagnosis** |
| What breaks if I delete this Secret? | Manual grep across manifests | ✗ | **`kgraph impact secret/my-secret`** |
| Can pod A reach pod B through NetworkPolicies? | ✗ | ✗ | **`kgraph can-reach pod/a pod/b`** |
| Are any ConfigMaps/Secrets unused? | ✗ | ✗ | **`kgraph orphan`** |
| Does my Deployment reference a Secret that doesn't exist? | `kubectl apply` error | ✗ | **`kgraph check deploy/my-app`** |
| Which pods are near their CPU/memory limits? | Separate metrics commands | ✗ | **Web UI Metrics page** |
| What patterns are emerging in cluster events? | `kubectl get events --all-ns` (wall of text) | ✗ | **Web UI Events intelligence** |

**No AI. No CRDs. No cluster-side installation. Read-only Kubernetes APIs only.**

---

## Features

### 🌐 Web UI (`kgraph serve`)

Launch a browser-based dashboard with a single command:

```bash
kgraph serve
# Opens http://localhost:7329 automatically
```

**Explorer** — Interactive graph canvas showing all resources and relationships. Click any node to inspect its manifest (YAML), dependency tree, failure diagnosis, events, and logs in the side panel. Filter by kind, search with ⌘K spotlight, toggle health heatmap, right-click context menu, drag-to-resize panel.

**Doctor** — Proactive health scanner. Flags HA risks (single replicas), missing liveness/readiness probes, root containers, privileged containers, missing ServiceAccounts, and workloads referencing non-existent resources. Grouped by severity: blocking vs warning.

**Metrics Intelligence** — Real-time CPU and memory utilisation for every Pod, cross-referenced against declared limits. Detects over-limit (>100%), critical (≥90%), and warning (≥75%) pods with actionable recommendations. Accumulates rolling sparkline history. Flags pods with no resource limits set.

**Events Intelligence** — Aggregates and deduplicates all cluster events over the last 2 hours. Detects rapid-repeat patterns (≥5 occurrences in 10 minutes), flags cluster-wide issues (same event affecting ≥3 objects), groups by severity so the signal isn't buried in noise.

**Orphans** — Lists every ConfigMap, Secret, PVC, Service, and ServiceAccount not referenced by any running workload.

**RBAC Path Tracer** — Interactive form to check whether a ServiceAccount can perform a verb on a resource. Shows the full binding → role → rule evaluation chain.

---

### 🖥️ Terminal UI (`kgraph explore`)

A fullscreen three-pane terminal dashboard built with Bubble Tea + Lipgloss:

```
┌──────────────────┬────────────────────────────────────┬─────────────────────┐
│  Resources       │  Graph                             │  Details            │
│  ─────────────   │                                    │  ─────────────────  │
│  ● frontend      │  Ingress ──routes──▶ Service       │  Pod: payments-x    │
│  ✖ payments      │           ──selects─▶ Pod          │  Status: Pending    │
│  ● web           │  Deployment ──owns──▶ ReplicaSet   │  ErrImagePull       │
│  ...             │              ──owns──▶ Pod          │  [events] [metrics] │
└──────────────────┴────────────────────────────────────┴─────────────────────┘
```

| Key | Action |
|---|---|
| `↑/↓` or `j/k` | Navigate resource list |
| `Enter` | Select resource and expand graph |
| `p` / `:problems` | Problems view (unhealthy pods + diagnosis) |
| `o` / `:orphan` | Orphan view |
| `c` | Context switcher |
| `:ns` | Namespace picker |
| `?` | Help / keybindings |
| `q` | Quit |

---

### ⌨️ CLI Commands

All commands accept `--context`, `--namespace`, `--kubeconfig`, and `--depth` flags. Resource names accept short aliases (`deploy`, `svc`, `po`, etc.).

#### `kgraph why <kind>/<name>`

Explains why a resource is in its current state using container status, Warning events, and previous-container log excerpts.

```bash
kgraph why pod/payments-abc-123
kgraph why deployment/api
kgraph why pvc/data-volume
```

```
⬡ why Pod/payments-abc-123

Diagnosis
─────────────────────────────────────────
  pod  payments-abc-123                     phase=Pending
  ✖    Image pull failed for container nginx
         ErrImagePull: failed to pull image "nginx:doesnotexist-9999"
  ⚡   [BackOff] Back-off pulling image "nginx:doesnotexist-9999"
```

---

#### `kgraph deps <kind>/<name>`

Shows the full dependency tree — everything the resource needs to run.

```bash
kgraph deps deployment/frontend
kgraph deps pod/api-abc-123
kgraph deps --depth 2 statefulset/postgres
```

```
▦ Deployment default/frontend
├─ ▣ ReplicaSet default/frontend-xxxx (owns)
│  └─ ● Pod default/frontend-abc (owns)
│     ├─ □ ConfigMap default/app-config (mounts)
│     ├─ □ Secret default/tls-cert (mounts)
│     └─ ◎ Node worker-1 (scheduled on)
└─ ● Pod default/frontend-def (owns)
   └─ ...
```

---

#### `kgraph impact <kind>/<name>`

Shows the reverse dependency tree — everything that would break if this resource changed or disappeared.

```bash
kgraph impact secret/db-password
kgraph impact configmap/app-config
kgraph impact node/worker-1
```

```
▦ Secret default/db-password
└─ ● Pod default/api-abc (mounts)
   └─ ▣ ReplicaSet default/api-rs (owns)
      └─ ▦ Deployment default/api (owns)
         └─ ◈ Service default/api (selects)
```

---

#### `kgraph doctor`

Proactive cluster health scan. Checks HA, reliability, security, storage, and network concerns.

```bash
kgraph doctor
kgraph doctor --namespace production
```

```
⚠  Reliability   Deployment default/web — single replica — no high availability
⚠  Security      Pod default/api — container "main" runs as root (runAsUser: 0)
⚠  Reliability   Pod default/worker — no liveness probe on container "main"
🔴 Health        Deployment default/payments — 0/2 replicas available
```

---

#### `kgraph can-reach <src> <dst> [--port N] [--proto TCP|UDP]`

Evaluates NetworkPolicies to determine whether a Pod can reach a destination. Purely deterministic — no network probes, no traffic generated.

```bash
kgraph can-reach pod/frontend pod/payments --port 8080
kgraph can-reach pod/worker svc/database --port 5432
```

```
✔  ALLOWED   pod/frontend → pod/payments :8080/TCP
   Egress:   no egress NetworkPolicy restricts this path
   Ingress:  allow-frontend policy permits port 8080 from app=frontend
```

---

#### `kgraph orphan`

Lists every ConfigMap, Secret, PVC, Service, and ServiceAccount that no running Pod references.

```bash
kgraph orphan
kgraph orphan --namespace staging
```

---

#### `kgraph rbac`

Traces RBAC permission paths for a ServiceAccount.

```bash
kgraph rbac --sa api-worker --verb get --resource secrets
kgraph rbac --sa ci-runner --verb create --resource deployments --apigroup apps
```

---

#### `kgraph check <kind>/<name>`

Cross-references a workload's manifest against the live cluster graph. Catches missing ConfigMaps, Secrets, PVCs, and ServiceAccounts before `kubectl apply`.

```bash
kgraph check deployment/api
kgraph check statefulset/postgres
```

---

#### `kgraph bottleneck`

Detects CPU and memory bottlenecks. Requires metrics-server.

```bash
kgraph bottleneck
kgraph bottleneck --namespace production
```

```
🔴 OVER     Pod payments-abc  cpu     142%  →  increase cpu limit or throttle request rate
⚠  WARNING  Pod worker-xyz    memory   81%  →  consider increasing memory limit
```

---

#### `kgraph ask "<question>"`

Maps natural-language questions to graph traversals using deterministic keyword rules. No AI or LLMs.

```bash
kgraph ask "what does deployment/frontend depend on"
kgraph ask "why is pod/payments unhealthy"
kgraph ask "what breaks if secret/tls-cert changes"
kgraph ask "which resources are orphaned"
```

---

#### `kgraph context`

```bash
kgraph context           # list all kubeconfig contexts
kgraph context current   # show current context
kgraph context use prod  # switch to "prod"
```

---

## Installation

### Go install (recommended)

```bash
go install github.com/anishetty/kgraph@latest
```

Requires Go 1.21+. The binary is fully self-contained — the Web UI is embedded inside it.

### Build from source

```bash
git clone https://github.com/Anishshettyk/k-graph.git
cd k-graph

# Build the binary (Web UI already embedded in repo)
go build -o kgraph .

# Run without installing
./kgraph --help

# Install to $GOPATH/bin
go install .
```

**Rebuilding the Web UI** (only needed if you modify `ui/src/`):

```bash
cd ui && npm install && npm run build
# Then rebuild the Go binary to embed the new ui/dist/
go build -o kgraph .
```

---

## Quick start

```bash
# Confirm kubectl can reach your cluster
kubectl cluster-info

# Launch the Web UI (opens browser automatically)
kgraph serve

# Or use the fullscreen TUI
kgraph explore

# Diagnose a failing Deployment
kgraph why deployment/payments

# Find what depends on a Secret
kgraph impact secret/db-password

# Proactive health scan
kgraph doctor

# Detect CPU/memory bottlenecks
kgraph bottleneck

# Find unused resources
kgraph orphan
```

---

## Global flags

| Flag | Default | Description |
|---|---|---|
| `--context` | current kubeconfig context | Kubernetes context to use |
| `--namespace`, `-n` | all namespaces | Scope the query to a namespace |
| `--kubeconfig` | `~/.kube/config` | Path to kubeconfig file |
| `--depth N` | 0 (unlimited) | Max traversal depth for tree commands |

---

## How it works

```
~/.kube/config
       │
       ▼
  client-go  (read-only list APIs — no watches in CLI mode, no CRDs)
       │
       ▼
  collector  →  Resources (typed in-memory snapshot)
       │
       ▼
  graph.Build  →  Graph (directed nodes + edges)
       │
       ├── query       BFS/DFS: deps, impact, why, orphan
       ├── diagnosis   container status + events + log excerpts → root cause
       ├── network     NetworkPolicy reachability (graph traversal only)
       ├── rbac        binding → role → rule path tracing
       ├── checker     manifest cross-reference validator
       ├── metrics     live CPU/mem from metrics-server (optional)
       ├── rules       proactive health scanner (doctor)
       ├── renderer    box-drawing tree output (lipgloss)
       ├── tui         Bubble Tea interactive TUI
       └── api/        HTTP JSON API + embedded React Web UI
```

**One collection pass per command.** Every result is deterministic and explainable — no probabilistic inference, no LLM.

---

## Graph relationships modelled

| Relationship | Source → Target |
|---|---|
| `owns` | Deployment → ReplicaSet → Pod |
| `selects` | Service → Pod |
| `routes to` | Ingress → Service |
| `mounts` | Pod → ConfigMap / Secret / PVC |
| `binds` | PVC → PV |
| `scheduled on` | Pod → Node |
| `uses` | Pod → ServiceAccount |
| `policy selects` | NetworkPolicy → Pod |
| `grants` | RoleBinding → ServiceAccount |
| `role ref` | RoleBinding → Role / ClusterRole |

---

## Supported resource kinds

Deployments, ReplicaSets, StatefulSets, DaemonSets, Jobs, CronJobs, Pods, Services, Ingresses, ConfigMaps, Secrets, PersistentVolumeClaims, PersistentVolumes, Nodes, ServiceAccounts, NetworkPolicies, Namespaces, Roles, ClusterRoles, RoleBindings, ClusterRoleBindings.

---

## Project layout

```
cmd/                    # Cobra CLI commands
internal/
    api/                # HTTP JSON API router + handlers
    collector/          # client-go read-only resource collection
    graph/              # graph model (nodes, edges, build)
    query/              # BFS/DFS, orphan detection, impact
    diagnosis/          # deterministic failure analysis
    network/            # NetworkPolicy reachability
    rbac/               # RBAC permission path tracer
    checker/            # manifest cross-reference validator
    health/             # shared Pod health checks
    metrics/            # metrics-server client
    rules/              # proactive health rules (doctor)
    renderer/           # lipgloss tree renderer
    tui/                # Bubble Tea TUI
ui/                     # React 18 + TypeScript + Vite Web UI
    src/
        pages/          # Explorer, Doctor, Metrics, Events, Orphans, RBAC
        components/     # GraphCanvas, DetailsPanel, Sparkline, LogViewer, …
        api/            # typed fetch client
        types/          # TypeScript types (mirrors Go structs)
main.go                 # embeds ui/dist via go:embed
```

---

## Design principles

- **100% Go.** No other languages, no shelling out to `kubectl`.
- **No AI / no LLM calls.** All analysis is deterministic and explainable.
- **No CRDs and no cluster-side components.** Read-only access via standard Kubernetes APIs only.
- **Local-first.** Nothing is sent to any cloud service.
- **The graph is the heart.** Every command is a traversal — never an ad-hoc API call.
- **One binary.** The Web UI is embedded. `kgraph serve` needs nothing else installed at runtime.

---

## Requirements

- Go 1.21+
- A kubeconfig with cluster access (`kubectl cluster-info` should succeed)
- **Optional:** [metrics-server](https://github.com/kubernetes-sigs/metrics-server) for `kgraph bottleneck` and Web UI Metrics page

---

## Contributing

Bug reports and pull requests are welcome. Please open an issue first for substantial changes.

```bash
go test ./...   # run tests
go vet ./...    # static checks
```

---

## License

MIT © [Anish Shetty](https://github.com/Anishshettyk)
