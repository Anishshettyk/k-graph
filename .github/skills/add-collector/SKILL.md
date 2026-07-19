---
name: add-collector
description: "Add a new Kubernetes resource collector to KGraph — wires client-go listing/watching for a resource type, maps it into graph nodes and edges, and adds tests. Use when adding support for a new resource (e.g. StatefulSet, Ingress, NetworkPolicy, PVC) to the collector and graph."
argument-hint: "Resource type to add (e.g. StatefulSet, Ingress, PVC)"
---

# Add a Resource Collector

Use this when extending KGraph to understand a new Kubernetes resource type. Follow [AGENTS.md](../../../AGENTS.md) hard constraints: read-only `client-go` only, no `kubectl`, deterministic, local-first.

## When to Use

- Adding a new resource type to the collector (StatefulSet, DaemonSet, Job, CronJob, Ingress, ConfigMap, Secret, PVC, PV, StorageClass, Node, NetworkPolicy, etc.).
- Introducing new edges into the graph for a resource already partially supported.

## Procedure

1. **Confirm the resource and its edges.** Identify how it connects to existing nodes:
   - Owner-reference edges (e.g. StatefulSet→Pod, CronJob→Job→Pod).
   - Selector edges (e.g. Service→Pod, NetworkPolicy→Pod).
   - Reference edges (e.g. Pod→PVC→PV, Ingress→Service, Pod→Secret/ConfigMap).

2. **Collect (read-only).** In `internal/collector/`:
   - Add a list (and later informer/watch) call for the resource using the existing `kubernetes.Clientset`.
   - Accept and propagate `context.Context`.
   - Wrap errors: `fmt.Errorf("list <resource>: %w", err)`. Never mutate cluster state.

3. **Map into the graph.** In `internal/graph/`:
   - Create a `Node` per object (kind, namespace, name, UID, raw object).
   - Add `Edge`s derived from owner references and label selectors — do not hardcode names.
   - Guard shared graph state if collectors run concurrently.

4. **Expose in queries if relevant.** Ensure `deps`/`impact` traversals include the new edges (they should automatically if edges are added to the shared graph model).

5. **Test.** In `internal/graph/` add table-driven tests that build a small in-memory set of the new resource plus its neighbors and assert the expected nodes/edges. Do not require a live cluster.

6. **Verify.** Run `go build ./...`, `go vet ./...`, `go test ./...`.

## Checklist

- [ ] Read-only collection, `context.Context` threaded through
- [ ] Nodes created with stable UID identity
- [ ] Edges derived from ownerReferences / selectors / references
- [ ] Shared graph state guarded
- [ ] Table-driven test added
- [ ] `go build`, `go vet`, `go test` pass
