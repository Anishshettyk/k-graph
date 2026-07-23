# KGraph — CLI Command Reference

All commands accept global flags. Resources use `kind/name` syntax with aliases (e.g. `deploy/frontend`, `svc/api`, `po/worker-abc`).

---

## Global Flags

| Flag | Default | Description |
|---|---|---|
| `--context` | current kubeconfig context | Kubernetes context to use |
| `--namespace`, `-n` | all namespaces | Scope query to a namespace |
| `--kubeconfig` | `~/.kube/config` | Path to kubeconfig file |
| `--depth N` | 0 (unlimited) | Max traversal depth (deps/impact/why) |

---

## Analysis Commands

### `kgraph why <kind>/<name>`
Explain why a resource is unhealthy. For Pods: inspects container status, Warning events, and previous-container logs. For Services: checks endpoint binding. For PVCs: reports bind phase.

```bash
kgraph why pod/payments-abc-123
kgraph why deployment/api
kgraph why pvc/data-volume
kgraph why -n production svc/checkout
```

---

### `kgraph deps <kind>/<name>`
Show the full dependency tree — everything a resource needs to run.

```bash
kgraph deps deployment/frontend
kgraph deps --depth 2 statefulset/postgres
kgraph deps pod/api-abc-123
```

---

### `kgraph impact <kind>/<name>`
Show the reverse dependency tree — what breaks if this resource changes or disappears.

```bash
kgraph impact secret/db-password
kgraph impact configmap/app-config
kgraph impact node/worker-1
```

---

### `kgraph ask "<question>"`
Maps natural-language questions to graph traversals. Deterministic keyword rules — no AI.

```bash
kgraph ask "what does deployment/frontend depend on"
kgraph ask "why is pod/payments unhealthy"
kgraph ask "what breaks if secret/tls-cert changes"
kgraph ask "which resources are orphaned"
kgraph ask "show me all failing pods"
```

---

## Health & Audit Commands

### `kgraph doctor`
Proactive cluster health scan: HA gaps, missing probes, security issues, image risks, cross-namespace routing problems.

```bash
kgraph doctor
kgraph doctor -n production
```

Output categories: `High Availability`, `Reliability`, `Security`, `Storage`, `Network`, `Images`

---

### `kgraph orphan`
List ConfigMaps, Secrets, PVCs, Services, and ServiceAccounts not referenced by any running workload.

```bash
kgraph orphan
kgraph orphan -n staging
```

---

### `kgraph check <kind>/<name>`
Cross-reference a workload's manifest against the live cluster. Catches missing ConfigMaps, Secrets, PVCs, and ServiceAccounts before `kubectl apply`.

```bash
kgraph check deployment/api
kgraph check statefulset/postgres
kgraph check -n production deploy/frontend
```

---

## Network Commands

### `kgraph can-reach <src> <dst> [--port N] [--proto TCP|UDP]`
Evaluate NetworkPolicies to determine whether a Pod can reach a destination. No network probes — purely deterministic graph analysis.

```bash
kgraph can-reach pod/frontend pod/payments --port 8080
kgraph can-reach pod/worker svc/database --port 5432
kgraph can-reach -n default pod/api-abc pod/db-xyz --port 5432 --proto TCP
```

---

### `kgraph network <kind>/<name>`
Trace the full traffic path from Ingress → Service → Pods for a given resource, including active NetworkPolicy rules.

```bash
kgraph network svc/frontend
kgraph network ingress/main
```

---

## Metrics & Resource Commands

### `kgraph bottleneck`
Detect CPU and memory bottlenecks (requires metrics-server). Flags pods at `OVER` (>100%), `CRITICAL` (≥90%), or `WARNING` (≥75%) of their limits.

```bash
kgraph bottleneck
kgraph bottleneck -n production
```

---

### `kgraph rightsizing`
Identify over-provisioned pods (using <20% of request) and under-provisioned pods (≥85% of request). Requires metrics-server.

```bash
kgraph rightsizing
kgraph rightsizing -n default
```

---

### `kgraph images`
List all unique container images across running pods. Flags `:latest` tags, untagged images, and shows which workloads use each image.

```bash
kgraph images
kgraph images -n production
```

---

### `kgraph certs`
Scan all `kubernetes.io/tls` Secrets and report TLS certificate expiry. Severity: `CRITICAL` (≤7 days or expired), `WARNING` (≤30 days), `WATCH` (≤90 days), `OK`.

```bash
kgraph certs
kgraph certs -n ingress-nginx
```

---

## RBAC Command

### `kgraph rbac`
Trace RBAC permission paths for a ServiceAccount. Shows the full binding → role → rule evaluation chain.

```bash
kgraph rbac --sa api-worker --verb get --resource secrets
kgraph rbac --sa ci-runner --verb create --resource deployments --apigroup apps
kgraph rbac -n production --sa default --verb list --resource pods
```

Flags: `--sa`, `--verb`, `--resource`, `--apigroup` (optional)

---

## History Command

### `kgraph history <kind>/<name>`
Show rollout history for a Deployment or StatefulSet, reconstructed from owned ReplicaSets. Sorted newest-first with images and replica counts.

```bash
kgraph history deployment/frontend
kgraph history deployment/payments
kgraph history statefulset/postgres
```

---

## Observation Commands

### `kgraph watch`
Stream live cluster changes as they happen. Detects topology changes: Pods transitioning phase, Deployments scaling, Services losing endpoints.

```bash
kgraph watch
kgraph watch -n production
kgraph watch --interval 10          # poll every 10 seconds (default: 5)
kgraph watch --problems-only        # only surface degraded resources
```

---

### `kgraph events <kind>/<name>`
Show Kubernetes events for a resource and its Pod descendants.

```bash
kgraph events deployment/payments
kgraph events pod/api-abc-123
kgraph events -n staging svc/frontend
```

---

### `kgraph logs <kind>/<name>`
Stream and parse logs with automatic level detection (FATAL/ERROR/WARN/INFO/DEBUG/TRACE), colour coding, and optional level filtering.

```bash
kgraph logs pod/api-abc-123
kgraph logs -n production deploy/frontend
kgraph logs pod/worker-xyz --level ERROR    # show ERROR and above only
kgraph logs pod/worker-xyz --tail 100       # last 100 lines
```

---

## Interactive Commands

### `kgraph explore` / `kgraph tui`
Launch the interactive fullscreen terminal UI (TUI) with three-pane layout: resource list, graph, and details/health/metrics panel.

```bash
kgraph explore
kgraph tui
kgraph explore -n production
```

**TUI keybindings:**

| Key | Action |
|---|---|
| `↑/↓` or `j/k` | Navigate resource list |
| `Enter` | Select resource and expand graph |
| `p` / `:problems` | Switch to Problems view |
| `o` / `:orphan` | Switch to Orphan view |
| `c` | Open context switcher |
| `:ns` | Open namespace picker |
| `?` | Show help / keybindings |
| `q` | Quit |

---

### `kgraph serve`
Launch the KGraph Web UI in the browser. The UI is embedded in the binary — no Node.js required at runtime.

```bash
kgraph serve
kgraph serve --port 8080        # custom port (default: 7329)
kgraph serve --open=false       # don't auto-open browser
```

Opens `http://localhost:7329`. The Web UI has 9 pages:
- **Explorer** — interactive graph canvas with health heatmap, spotlight search, and details panel
- **Doctor** — proactive health scan findings
- **Metrics** — CPU/memory usage with sparklines, rightsizing, namespace breakdown
- **Images** — container image inventory with risk flags
- **Network** — animated traffic topology (Ingress → Service → Pod)
- **Certs** — TLS certificate expiry with countdown bars
- **Orphans** — unused resources
- **RBAC** — interactive permission path tracer
- **Events** — cluster event intelligence with pattern detection

---

## Context Commands

### `kgraph context`

```bash
kgraph context               # list all kubeconfig contexts
kgraph context current       # print the current context name
kgraph context use prod      # switch to context "prod"
```

---

## Resource Kind Aliases

All commands that accept `<kind>/<name>` support these aliases:

| Aliases | Kind |
|---|---|
| `pod`, `pods`, `po` | Pod |
| `deploy`, `deployment`, `deployments` | Deployment |
| `rs`, `replicaset` | ReplicaSet |
| `sts`, `statefulset` | StatefulSet |
| `ds`, `daemonset` | DaemonSet |
| `job`, `jobs` | Job |
| `cj`, `cronjob` | CronJob |
| `svc`, `service` | Service |
| `ing`, `ingress` | Ingress |
| `cm`, `configmap` | ConfigMap |
| `secret`, `secrets` | Secret |
| `pvc` | PersistentVolumeClaim |
| `pv` | PersistentVolume |
| `no`, `node` | Node |
| `sa`, `serviceaccount` | ServiceAccount |

---

## Development

```bash
# Install once
go install github.com/air-verse/air@latest
cd ui && npm install

# Start dev servers (Go hot-reload + Vite HMR)
./dev.sh

# Go changes → air auto-rebuilds and restarts (http://localhost:7329)
# UI changes → Vite HMR updates browser instantly (http://localhost:5173)
# Open http://localhost:5173 during development
```

## Build from source

```bash
git clone https://github.com/Anishshettyk/k-graph.git
cd k-graph
go build -o kgraph .
./kgraph --help
```

To rebuild the embedded Web UI:
```bash
cd ui && npm install && npm run build
go build -o kgraph .
```
