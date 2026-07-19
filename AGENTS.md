# AGENTS.md — KGraph

KGraph is a **local-first Kubernetes knowledge-graph CLI**, written entirely in **Go**. It reads the user's `~/.kube/config`, builds a graph of cluster resources via `client-go`, and answers *why / what-changed / what-depends-on-this / what-breaks-if-I-change-this* through **deterministic graph traversal — no AI, no CRDs, no cluster-side install**.

Full vision, module breakdown, CLI spec, and roadmap: [KGraph_Project_Plan.md](KGraph_Project_Plan.md). Treat that file as the source of truth; keep it in sync when scope changes.

## Project status

**Phase 2 complete + rich graph + interactive TUI + metrics.** Module `github.com/anishetty/kgraph` has a Cobra CLI (`cmd/`), a read-only best-effort `client-go` collector (`internal/collector/`) covering Deployments, ReplicaSets, StatefulSets, DaemonSets, Jobs, CronJobs, Pods, Services, Ingresses, ConfigMaps, Secrets, PVCs, PVs, Nodes, and ServiceAccounts. The graph core (`internal/graph/`) builds nodes + edges: ownership (owns), Service selectors (selects), Ingress routing (routes to), Pod→ConfigMap/Secret/PVC (mounts), PVC→PV (binds), Pod→Node (scheduled on), Pod→ServiceAccount (uses). A query engine (`internal/query/`) provides BFS traversals + cycle-safe tree builders (depth-limited), shared health checks live in `internal/health/`, and `internal/diagnosis/` deterministically correlates unhealthy Pod status, Warning events, and bounded previous-container logs. Live CPU/memory usage vs requests/limits comes from `internal/metrics/` (metrics.k8s.io), a colored, icon-annotated tree renderer with a legend in `internal/renderer/` (uses `lipgloss`), and an interactive terminal UI in `internal/tui/` (Bubble Tea + Bubbles) with a three-pane layout (resources · graph · details/health/metrics/diagnosis), gauge bars for CPU/mem utilization, a cluster summary (unhealthy / near-limit counts), bottleneck markers in the list, an async spinner-backed context switch, and adaptive width. Commands: `deps`, `impact`, `why` (with `--depth` and shell completion of live resource names), `ask` (deterministic NL → traversals, **no AI**), `context`, and `explore`/`tui`. Metrics and diagnosis are fused into `explore` (no standalone `top`). Next: snapshot/history/diff engines (Phase 3).

## Hard constraints (do not violate)

- **100% Go.** No other languages, no shelling out to `kubectl`.
- **No AI / no LLM calls** anywhere in the tool. Analysis must be deterministic and explainable.
- **No CRDs and no cluster-side components.** Read-only access via Kubernetes APIs (`client-go` informers/watches) only.
- **Local-first.** Snapshots, history, and state persist to the local filesystem, never to a cloud service.
- This is a **learning project** — prefer clear, idiomatic Go that demonstrates the concept over clever abstractions. See "Go learning goals" in the plan.

## Intended structure

```
cmd/                 # Cobra command entrypoints
internal/
    collector/       # client-go informers/watches → raw resources
    graph/           # nodes, edges, graph model (the core)
    query/           # BFS/DFS, reachability, reverse-deps, impact
    renderer/        # terminal output (lipgloss/bubbletea later)
    snapshot/        # local snapshot store + diff
    history/         # persisted watch events / timelines
    rules/           # deterministic health checks (doctor)
pkg/                 # reusable, importable packages only
```

Keep implementation details in `internal/`. Only put genuinely reusable APIs in `pkg/`.

## Conventions

- Use the standard `error` interface; wrap with `fmt.Errorf("...: %w", err)`. No panics in library code.
- Pass `context.Context` through collectors, watches, and long-running traversals.
- The **graph is the heart of the app** — model resources as nodes and relationships as edges (e.g. Deployment→ReplicaSet→Pod, Service→Pod, Ingress→Service, Pod→Secret/ConfigMap/PVC, PVC→PV, Pod→Node). Every CLI command should be a graph traversal, not ad-hoc API calls.
- Concurrency (goroutines, channels, worker pools, mutexes) is encouraged for collectors, but guard shared graph state.

## Build & test

```bash
go build ./...   # compile all packages
go vet ./...     # static checks
go test ./...    # run tests
go run . --help  # run the CLI locally
```

Add table-driven tests alongside code (`*_test.go`); Go's learning goals include testing and benchmarking, so cover graph and query logic well. Graph/query tests should build small in-memory graphs and never require a live cluster.

## CLI surface

Implemented: `deps` · `impact` · `why` · `ask` (natural-language, deterministic) · `context` (list/`current`/`use`) · `explore`/`tui` (interactive, with live metrics gauges + bottleneck markers). `deps`/`impact`/`why` take `--depth N` and offer shell completion of live `kind/name` resources.
Planned: `network` · `diff` · `history` · `orphan` · `doctor`. See the plan for semantics. Built on **cobra**; static output via **lipgloss**, interactive UI via **Bubble Tea** + **Bubbles**.

Rendering: static traversal results are drawn as box-drawing trees (`internal/renderer`). Color auto-enables only on a TTY and honors `NO_COLOR`. The interactive `explore` command (`internal/tui`) fuses topology, health, and live CPU/mem gauges into one three-pane view with a fuzzy context switcher (`c`); `:ns` opens a namespace picker whose first option restores all namespaces. `p` or `:problems` opens a current-scope view of unhealthy Pods, diagnosis evidence, and their affected-resource graph. Live metrics require a metrics-server in the cluster. The `ask` command maps English questions to traversals using deterministic keyword rules in [cmd/interpret.go](cmd/interpret.go) — never an LLM.
