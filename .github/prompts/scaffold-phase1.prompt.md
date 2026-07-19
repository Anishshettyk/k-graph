---
description: "Scaffold KGraph Phase 1: go mod init, Cobra CLI skeleton, client-go wiring, resource collector, and graph builder. Use when starting the project from the empty plan."
name: "Scaffold KGraph Phase 1"
argument-hint: "Optional: module path (e.g. github.com/anishetty/kgraph)"
agent: "agent"
---

Scaffold **Phase 1** of KGraph following [AGENTS.md](../../AGENTS.md) and [KGraph_Project_Plan.md](../../KGraph_Project_Plan.md). Respect all hard constraints: 100% Go, no AI, no CRDs, no cluster-side install, local-first, read-only via `client-go`.

If a module path was provided as an argument, use it for `go mod init`; otherwise ask for it once before starting.

## Steps

1. **Module + tooling**
   - `go mod init <module-path>`
   - Add dependencies: `github.com/spf13/cobra`, `k8s.io/client-go`, `k8s.io/api`, `k8s.io/apimachinery`.
   - Run `go mod tidy`.

2. **Directory layout** (create empty packages per AGENTS.md):
   `cmd/`, `internal/collector/`, `internal/graph/`, `internal/query/`, `internal/renderer/`.

3. **Cobra CLI skeleton** (`cmd/`)
   - Root command `kgraph` with a persistent `--kubeconfig` flag (default `~/.kube/config`) and `--context` flag.
   - Register placeholder subcommands: `why`, `deps`, `impact`. Each prints "not implemented" for now.

4. **client-go wiring** (`internal/collector/`)
   - Build a `*rest.Config` from kubeconfig via `clientcmd.BuildConfigFromFlags` / `NewNonInteractiveDeferredLoadingClientConfig`.
   - Create a `kubernetes.Clientset`. Read-only usage only — never write to the cluster.
   - Pass `context.Context` through all calls; wrap errors with `fmt.Errorf("...: %w", err)`.

5. **Collector** (`internal/collector/`)
   - Start with a straightforward list-based collector for a few resource types (Deployments, ReplicaSets, Pods, Services) before adding informers.
   - Return raw typed objects to the graph builder.

6. **Graph builder** (`internal/graph/`)
   - Define `Node` (kind, namespace, name, UID, raw object) and `Edge` (from, to, relationship) types.
   - Build edges from owner references and selectors: Deployment→ReplicaSet→Pod, Service→Pod.
   - Guard shared graph state if built concurrently.

7. **Verify**
   - `go build ./...`, `go vet ./...`, `go test ./...`.
   - Add at least one table-driven test in `internal/graph/` for edge construction.
   - Update the "Build & test" section of [AGENTS.md](../../AGENTS.md) with the now-real commands.

Prefer clear, idiomatic Go that demonstrates the concept — this is a learning project, not a place for premature abstraction.
