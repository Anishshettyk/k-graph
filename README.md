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
| Which pods are near their CPU/memory limits? | Separate metrics commands | ✗ | **`kgraph rightsizing`** |
| Which TLS certs expire soon? | ✗ | ✗ | **`kgraph certs`** |
| What images are running and are any pinned to `:latest`? | ✗ | ✗ | **`kgraph images`** |
| What changed in this Deployment's rollout history? | `kubectl rollout history` (limited) | ✗ | **`kgraph history deploy/X`** |
| What patterns are emerging in cluster events? | Wall of text | ✗ | **Web UI Events intelligence** |

**No AI. No CRDs. No cluster-side installation. Read-only Kubernetes APIs only.**

---

## Features

### 🌐 Web UI (`kgraph serve`)

Launch a browser-based dashboard with a single command:

```bash
kgraph serve
# Opens http://localhost:7329 automatically
```

**9 pages:**

| Page | Description |
|---|---|
| **Explorer** | Interactive graph canvas with BFS neighbourhood, health heatmap, ⌘K spotlight, right-click menu, drag-to-resize panel. Details panel: info / yaml / why / events / logs / history. |
| **Doctor** | Proactive health scan: HA gaps, missing probes, root containers, unpinned images, cross-namespace routing issues — grouped blocking vs warning. |
| **Metrics** | Real-time CPU/memory with sparklines (Overview), waste/under-provisioning recommendations (Rightsizing), per-namespace allocation bars (Namespaces). |
| **Images** | Container image inventory grouped by risk — flags `:latest` tags, untagged images. Expandable workload list per image. |
| **Network** | Animated React Flow graph: Internet → Ingress → Service → Pod with glowing edges, NetworkPolicy overlays, dagre layout, mini-map. |
| **Certs** | TLS certificate expiry grouped by severity with days-left progress bars and DNS name list. |
| **Orphans** | Every ConfigMap, Secret, PVC, Service, and ServiceAccount not referenced by any running workload. |
| **RBAC** | Live ServiceAccount combobox, verb/resource pill selectors, auto-detected API groups, full binding → role → rule chain evaluation. |
| **Events** | Deduplication, rapid-repeat pattern detection (≥5 in 10m), cluster-wide issue flagging, severity grouping. |

---

### 🖥️ Terminal UI (`kgraph explore`)

```bash
kgraph explore
```

Three-pane layout: resource list (hierarchical by ownership) · graph · details.

| Key | Action |
|---|---|
| `↑/↓` or `j/k` | Navigate resource list |
| `Enter` | Select and expand graph |
| `p` / `:problems` | Problems view |
| `o` / `:orphan` | Orphan view |
| `c` | Context switcher |
| `:ns` | Namespace picker |
| `?` | Help |
| `q` | Quit |

---

### ⌨️ CLI — Full Command List

```bash
kgraph why <kind>/<name>          # root-cause analysis for any resource
kgraph deps <kind>/<name>         # dependency tree (what it needs)
kgraph impact <kind>/<name>       # reverse-dep tree (what breaks if it changes)
kgraph ask "<question>"           # plain-English → graph traversal (no AI)

kgraph doctor                     # proactive cluster health scan
kgraph orphan                     # find unused ConfigMaps, Secrets, PVCs…
kgraph check <kind>/<name>        # cross-reference manifest against live cluster

kgraph can-reach <src> <dst>      # NetworkPolicy reachability (no network probes)
kgraph network <kind>/<name>      # traffic path: Ingress → Service → Pods
kgraph rbac                       # RBAC permission path tracer

kgraph bottleneck                 # detect pods near CPU/mem limits
kgraph rightsizing                # find over/under-provisioned pods
kgraph images                     # image inventory + :latest flag
kgraph certs                      # TLS certificate expiry scan
kgraph history <kind>/<name>      # deployment rollout history

kgraph watch                      # stream live cluster changes
kgraph events <kind>/<name>       # Kubernetes events for a resource
kgraph logs <kind>/<name>         # logs with level detection + colour

kgraph explore / kgraph tui       # interactive terminal UI
kgraph serve                      # launch Web UI in browser
kgraph context                    # list / switch kubeconfig contexts
```

Full reference with flags and examples: [docs/COMMANDS.md](docs/COMMANDS.md)

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
go build -o kgraph .
./kgraph --help
```

To rebuild the embedded Web UI after UI changes:

```bash
cd ui && npm install && npm run build
go build -o kgraph .
```

---

## Quick start

```bash
# Confirm kubectl can reach your cluster
kubectl cluster-info

# Launch the Web UI
kgraph serve

# Or use the TUI
kgraph explore

# Diagnose a failing Deployment
kgraph why deployment/payments

# Find what depends on a Secret
kgraph impact secret/db-password

# Proactive health scan
kgraph doctor

# Detect over/under-provisioned pods
kgraph rightsizing

# Scan TLS cert expiry
kgraph certs

# Find unused resources
kgraph orphan
```

---

## Development (hot-reload)

```bash
# Install dependencies once
go install github.com/air-verse/air@latest
cd ui && npm install && cd ..

# Start dev environment — single command
./dev.sh
```

This starts two servers:
- **`air`** → Go API on `http://localhost:7329` — auto-rebuilds on any `.go` change (~2s)
- **`Vite`** → Web UI on `http://localhost:5173` — instant HMR on any `.tsx`/`.ts` change

Open **`http://localhost:5173`** during development. Ctrl+C stops both.

```bash
./dev.sh --api     # Go hot-reload only
./dev.sh --ui      # Vite HMR only
```

---

## Global flags

| Flag | Default | Description |
|---|---|---|
| `--context` | current kubeconfig context | Kubernetes context to use |
| `--namespace`, `-n` | all namespaces | Scope query to a namespace |
| `--kubeconfig` | `~/.kube/config` | Path to kubeconfig file |
| `--depth N` | 0 (unlimited) | Max traversal depth for tree commands |

---

## How it works

```
~/.kube/config
       │
       ▼
  client-go  (21 parallel read-only API calls — sync.WaitGroup)
       │
       ▼
  30s in-memory cache (per context)
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
       ├── rules       proactive health scanner (doctor) — 7 categories
       ├── renderer    lipgloss tree renderer
       ├── tui         Bubble Tea TUI
       └── api/        HTTP JSON API + embedded React Web UI
```

**One collection pass → all handlers.** The 30s cache means clicking across all 9 Web UI pages costs one `Collect()` sweep, not nine.

---

## Project layout

```
cmd/                    # 20 Cobra CLI commands
internal/
    api/                # HTTP JSON API (20 endpoints + resource cache)
    collector/          # client-go read-only resource collection (21 parallel)
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
docs/
    COMMANDS.md         # full CLI reference
    PROJECT_PLAN.md     # original vision and roadmap
.air.toml               # air live-reload config
dev.sh                  # dev: air + Vite with one command
main.go                 # embeds ui/dist via go:embed
```

---

## Design principles

- **100% Go.** No other languages, no shelling out to `kubectl`.
- **No AI / no LLM calls.** All analysis is deterministic and explainable.
- **No CRDs and no cluster-side components.** Read-only standard Kubernetes APIs only.
- **Local-first.** Nothing is sent to any cloud service.
- **The graph is the heart.** Every command is a traversal — never an ad-hoc API call.
- **One binary.** The Web UI is embedded. `kgraph serve` needs nothing else at runtime.

---

## Requirements

- Go 1.21+
- A kubeconfig with cluster access (`kubectl cluster-info` should succeed)
- **Optional:** [metrics-server](https://github.com/kubernetes-sigs/metrics-server) for `kgraph bottleneck`, `kgraph rightsizing`, and Web UI Metrics page

---

## Contributing

Bug reports and pull requests are welcome. Open an issue first for substantial changes.

```bash
go test ./...   # run tests
go vet ./...    # static checks
npx tsc --noEmit  # TypeScript check (from ui/)
```

---

## License

MIT © [Anish Shetty](https://github.com/Anishshettyk)
