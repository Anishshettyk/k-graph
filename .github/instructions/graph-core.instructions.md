---
description: "Use when working in internal/graph, internal/query, or internal/collector — the graph core of KGraph. Covers node/edge modeling, traversal-as-command, and read-only client-go rules."
applyTo: "internal/graph/**, internal/query/**, internal/collector/**"
---

# Graph Core Conventions

The graph is the heart of KGraph. See [AGENTS.md](../../AGENTS.md) and [KGraph_Project_Plan.md](../../KGraph_Project_Plan.md) for the full vision.

## Modeling

- Every Kubernetes object is a **node**; every relationship is an **edge**. Never answer a question with ad-hoc API calls — build/query the graph.
- A `Node` identifies a resource by kind, namespace, name, and UID, and holds the raw typed object.
- An `Edge` is directional with a named relationship. Canonical edges:
  - Deployment→ReplicaSet→Pod (owner references)
  - Service→Pod (label selector match)
  - Ingress→Service
  - Pod→Secret / Pod→ConfigMap / Pod→PVC (volume + envFrom references)
  - PVC→PV, Pod→Node
- Derive ownership edges from `metadata.ownerReferences`; derive selector edges by matching labels — do not hardcode name conventions.

## Query engine

- Commands (`why`, `deps`, `impact`, `network`) are **graph traversals** — BFS/DFS, reachability, reverse-dependency lookup, impact analysis.
- `deps` = forward traversal from a node; `impact` = reverse traversal to dependents. Keep these as pure functions over the graph so they are testable without a cluster.

## Constraints

- **Read-only.** `internal/collector` may only list/watch via `client-go`; never create, update, or delete cluster resources, and never shell out to `kubectl`.
- Pass `context.Context` through collectors, watches, and long traversals.
- Wrap errors with `fmt.Errorf("...: %w", err)`; no panics in library code.
- Concurrency is encouraged for collectors, but guard shared graph state (mutex or single-writer channel).

## Testing

- Add table-driven tests (`*_test.go`) for edge construction and traversal logic. Build small in-memory graphs in tests — do not require a live cluster.
