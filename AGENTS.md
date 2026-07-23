# AGENTS.md — KGraph

KGraph is a **local-first Kubernetes observability and troubleshooting tool**, written entirely in **Go**. It reads `~/.kube/config`, builds a knowledge graph of cluster resources via `client-go`, and answers *why / what-depends-on-this / what-breaks-if-I-change-this* through **deterministic graph traversal — no AI, no CRDs, no cluster-side install**.

Use it as a **CLI**, a fullscreen **TUI**, or a browser-based **Web UI** (`kgraph serve`).

Full CLI reference: [docs/COMMANDS.md](docs/COMMANDS.md) — Project plan: [docs/PROJECT_PLAN.md](docs/PROJECT_PLAN.md)

---

## Current status

All planned features implemented. Production-ready for public use.

**CLI commands (20):** `why`, `deps`, `impact`, `ask`, `doctor`, `orphan`, `check`, `can-reach`, `network`, `bottleneck`, `rightsizing`, `images`, `certs`, `rbac`, `history`, `watch`, `events`, `logs`, `explore`/`tui`, `serve`, `context`

**Web UI (9 pages):** Explorer, Doctor, Metrics (Overview · Rightsizing · Namespaces), Images, Network Topology, Certificates, Orphans, RBAC, Events

**Backend API (20 endpoints):** contexts, graph, deps, impact, why, doctor, orphan, rbac, network, can-reach, yaml, logs, metrics, rightsizing, images, certs, history, events, events/cluster

**Performance:** parallel collector (21 concurrent K8s API calls), 30s in-memory resource cache per context, 800-node graph cap with truncated flag, show-more sidebar

**Dev workflow:** `./dev.sh` — starts `air` (Go hot-reload) + Vite (HMR) in one command

---

## Hard constraints (do not violate)

- **100% Go.** No other languages, no shelling out to `kubectl`.
- **No AI / no LLM calls.** All analysis is deterministic and explainable.
- **No CRDs and no cluster-side components.** Read-only access via Kubernetes APIs only.
- **Local-first.** Nothing sent to any cloud service.
- Prefer clear, idiomatic Go over clever abstractions.

---

## Module structure

```
cmd/                         # 20 Cobra CLI command entrypoints
internal/
    api/                     # HTTP JSON API + router (served by kgraph serve)
      handlers.go            # graph, deps, impact, why, doctor, orphan, rbac, network, can-reach, events
      log_yaml.go            # /api/yaml, /api/logs
      metrics_api.go         # /api/metrics, /api/events/cluster
      images_api.go          # /api/images — image inventory + risk flags
      certs_api.go           # /api/certs — TLS cert expiry (crypto/x509)
      history_api.go         # /api/history — rollout history from ReplicaSets
      rightsizing_api.go     # /api/rightsizing — waste/under-provisioning + NS breakdown
      cache.go               # in-memory resource cache (30s TTL per context)
      server.go              # HTTP router + Handler struct
    collector/               # client-go read-only resource collection (21 parallel goroutines)
    graph/                   # nodes, edges, graph model (the core)
    query/                   # BFS/DFS traversals, orphan detection, impact
    diagnosis/               # deterministic Pod/PVC/Service failure analysis
    network/                 # NetworkPolicy reachability engine
    rbac/                    # RBAC permission path tracer
    checker/                 # manifest cross-reference validator
    health/                  # shared deterministic Pod health checks
    metrics/                 # metrics-server client (CPU/mem usage)
    rules/                   # proactive health rules (doctor) — 7 check categories
    renderer/                # lipgloss box-drawing tree renderer
    tui/                     # Bubble Tea interactive TUI (3-pane, 5 screens)
ui/                          # React 18 + TypeScript + Vite Web UI
    src/
        pages/               # Explorer, Doctor, Metrics, Images, NetworkTopology,
                             # Certificates, Orphan, RBAC, Events
        components/          # GraphCanvas, Sidebar, DetailsPanel, ResourceNode,
                             # Sparkline, LogViewer, YAMLPanel, SpotlightSearch,
                             # ContextMenu, TopBar
        api/client.ts        # typed fetch client (20 API calls)
        types/api.ts         # TypeScript types mirroring Go response structs
        store/useStore.ts    # Zustand store (context, namespace, selectedNode, page)
docs/
    COMMANDS.md              # full CLI reference
    PROJECT_PLAN.md          # original vision and roadmap
.air.toml                    # air live-reload config (Go hot-reload)
dev.sh                       # start both air + Vite with one command
main.go                      # embeds ui/dist via go:embed, calls cmd.Execute()
```

---

## Key architectural decisions

### Graph as the core
Every CLI command and API endpoint builds a `graph.Graph` from the `graph.Resources` snapshot and runs a traversal — never ad-hoc API calls. This makes results deterministic and consistent across CLI, TUI, and Web UI.

### Resource cache
`internal/api/cache.go` caches the `graph.Resources` snapshot per kubeconfig context with a 30-second TTL. All 20 API handlers share one `Collect()` sweep per window instead of each making 21 parallel K8s API calls. Dramatically reduces latency on large clusters.

### Parallel collector
`internal/collector/collector.go` fetches all 21 resource types concurrently using goroutines + `sync.WaitGroup`. Collection time is `O(max latency)` not `O(sum of latencies)`.

### Embedded Web UI
The React UI is compiled to `ui/dist/` and embedded into the Go binary via `//go:embed all:ui/dist`. `kgraph serve` is self-contained — no Node.js at runtime.

---

## Build and test

```bash
go build ./...        # compile all packages
go vet ./...          # static checks
go test ./...         # run tests
npx tsc --noEmit      # TypeScript check (from ui/)
./dev.sh              # dev mode: air + Vite HMR
```

Add table-driven tests alongside code (`*_test.go`). Graph and query tests build small in-memory graphs — never require a live cluster.

---

## Web UI pages

| Page | Route key | Description |
|---|---|---|
| Explorer | `explorer` | React Flow graph canvas with BFS neighbourhood, health heatmap, Ctrl+K spotlight, right-click context menu, drag-to-resize details panel. Details: info / yaml / why / events / logs / history (for Deployments). |
| Doctor | `doctor` | Proactive findings grouped by severity (blocking / warning) across 7 categories. |
| Metrics | `metrics` | 3 tabs: Overview (sparklines + bottleneck list), Rightsizing (waste/under), Namespaces (bar chart). |
| Images | `images` | Container image inventory, risk-flagged cards, expandable workload list. |
| Network | `network` | Animated React Flow canvas: Internet→Ingress→Service→Pod, NetworkPolicy edges, dagre layout, mini-map. |
| Certs | `certificates` | TLS cert expiry grouped by severity with days-left progress bars. |
| Orphans | `orphan` | Resources not referenced by any running workload, grouped by kind. |
| RBAC | `rbac` | SA combobox (live from cluster), verb pills, resource pills, auto-detected API group, grant path + binding evaluation result. |
| Events | `events` | Cluster event intelligence: deduplication, rapid-repeat pattern detection, cluster-wide issue flagging, severity grouping. |

---

## Coding conventions

- `error` wrapping: `fmt.Errorf("operation: %w", err)`. No panics in library code.
- Pass `context.Context` through collectors, watches, and long-running traversals.
- Namespace filtering in the API layer: `Collect()` always fetches all namespaces; namespace scoping is applied in-memory after the cache hit.
- JSON field names: use `json:"camelCase"` tags on all exported struct fields used in API responses. Lesson learned: diagnosis structs (`Finding`, `Event`, `Log`) had no tags — serialized with capital letters and broke the UI.
- Null slices: initialize to `[]T{}` not `var out []T` for API response slices to avoid `null` in JSON.
