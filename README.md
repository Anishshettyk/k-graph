# KGraph

**Local-first Kubernetes knowledge-graph CLI.**

KGraph reads your `~/.kube/config`, builds a graph of every resource and relationship in your cluster, and answers the questions `kubectl` can't:

- *Why is this Deployment broken?*
- *What will break if I change this Secret?*
- *Can my frontend pod actually reach the payments service?*
- *What changed while I was away?*
- *Which ConfigMaps and Secrets are no longer used by anything?*
- *Will this manifest fail because a ConfigMap it references doesn't exist yet?*

**No AI. No CRDs. No cluster-side installation. Read-only access only.**

---

## Table of contents

- [Requirements](#requirements)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Global flags](#global-flags)
- [Commands](#commands)
  - [why](#why)
  - [deps](#deps)
  - [impact](#impact)
  - [ask](#ask)
  - [can-reach](#can-reach)
  - [check](#check)
  - [orphan](#orphan)
  - [doctor](#doctor)
  - [rbac](#rbac)
  - [context](#context)
  - [explore / tui](#explore--tui)
- [Interactive explorer (TUI)](#interactive-explorer-tui)
  - [Keyboard shortcuts](#keyboard-shortcuts)
  - [Command prompt](#command-prompt)
  - [Problems view](#problems-view)
  - [Orphan view](#orphan-view)
- [Resource kinds and aliases](#resource-kinds-and-aliases)
- [Snapshots storage](#snapshots-storage)
- [How it works](#how-it-works)
- [Building from source](#building-from-source)

---

## Requirements

| Requirement | Version |
|---|---|
| Go | 1.21 or later |
| Kubernetes cluster | 1.20 or later |
| kubeconfig | Standard `~/.kube/config` |

A `metrics-server` is optional. Live CPU/memory gauges in the TUI require it; everything else works without it.

---

## Installation

### From source (recommended)

```bash
git clone https://github.com/anishetty/kgraph
cd kgraph
go install .
```

The binary is placed at `$(go env GOPATH)/bin/kgraph`. Make sure that path is in your `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
# Add to ~/.zshrc or ~/.bashrc to make it permanent
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc
```

Verify the installation:

```bash
kgraph --help
```

### Build without installing

```bash
go build -o kgraph .
./kgraph --help
```

---

## Quick start

```bash
# See why a Deployment is unhealthy
kgraph why deployment/payments

# Show everything a Deployment depends on
kgraph deps deployment/frontend

# Show what breaks if a Secret changes
kgraph impact secret/db-credentials

# Check if pod/frontend can reach svc/payments on port 8080
kgraph can-reach pod/frontend svc/payments -n default --port 8080

# Validate a manifest against the live cluster before applying
kgraph check ./deploy.yaml -n production

# Find unused ConfigMaps, Secrets, PVCs, and Services
kgraph orphan -n default

# Proactive health and security scan
kgraph doctor -n default

# Trace whether a ServiceAccount can perform an action
kgraph rbac sa/api-worker can-get secrets -n default

# Launch the interactive explorer
kgraph explore
```

---

## Global flags

These flags apply to every command.

| Flag | Default | Description |
|---|---|---|
| `--kubeconfig` | `~/.kube/config` | Path to the kubeconfig file |
| `--context` | current-context | kubeconfig context to use |

```bash
kgraph --context production deps deployment/api
kgraph --kubeconfig /path/to/other.yaml why pod/payments-abc-xyz
```

---

## Commands

### `why`

Walks the dependency tree of a resource and identifies unhealthy pods as likely root causes.

```bash
kgraph why <kind>/<name>
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--depth N` | 0 (unlimited) | Maximum traversal depth |

**Examples**

```bash
kgraph why deployment/frontend
kgraph why statefulset/postgres --depth 3
kgraph why pod/payments-abc-xyz
```

**Sample output**

```
◆ Deployment default/frontend
└─ ◇ ReplicaSet default/frontend-abc (owns)
   └─ ● Pod default/frontend-abc-xyz (owns)  ✖ phase=Pending

Likely root cause(s):
  - Pod default/frontend-abc-xyz: phase=Pending
```

---

### `deps`

Shows everything a resource depends on (forward traversal through ownership, selector, mount, and scheduling edges).

```bash
kgraph deps <kind>/<name>
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--depth N` | 0 (unlimited) | Maximum traversal depth |

**Examples**

```bash
kgraph deps deployment/api
kgraph deps pod/payments-abc-xyz
kgraph deps ingress/main --depth 2
```

**Edge types shown**

| Edge | Meaning |
|---|---|
| `owns` | ownerReference (Deployment → ReplicaSet → Pod) |
| `selects` | Service label selector → Pod |
| `routes to` | Ingress → Service |
| `mounts` | Pod → ConfigMap / Secret / PVC |
| `binds` | PVC → PV |
| `scheduled on` | Pod → Node |
| `uses` | Pod → ServiceAccount |
| `policy selects` | NetworkPolicy → Pod |

---

### `impact`

Shows what depends on a resource — the blast radius of a change (reverse traversal).

```bash
kgraph impact <kind>/<name>
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--depth N` | 0 (unlimited) | Maximum traversal depth |

**Examples**

```bash
kgraph impact secret/db-password
kgraph impact configmap/app-config
kgraph impact node/worker-1
```

**Sample output**

```
▦ ConfigMap default/app-config
├─ ● Pod default/api-abc (mounts)
│  └─ ◇ ReplicaSet default/api-rs (owns) ↩
└─ ● Pod default/worker-xyz (mounts)
```

---

### `ask`

Answers natural-language questions using deterministic keyword matching. No AI or LLMs.

```bash
kgraph ask "<question>"
```

**Examples**

```bash
kgraph ask "what does deployment/frontend depend on"
kgraph ask "why is pod/payments unhealthy"
kgraph ask "what depends on secret/tls-cert"
kgraph ask "what breaks if configmap/app-config changes"
```

Internally, `ask` maps phrases like *"depends on"*, *"what uses"*, *"why is"*, and *"what breaks"* to the `deps`, `impact`, and `why` traversals respectively.

---

### `can-reach`

Evaluates Kubernetes NetworkPolicies to determine whether a source Pod is allowed to reach a destination Pod or Service. No network probes are made — the analysis is purely deterministic graph traversal.

```bash
kgraph can-reach <pod/name> <pod/name|svc/name> [flags]
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `-n, --namespace` | `default` | Namespace for resolving resource names |
| `-p, --port` | `0` (any port) | Destination port to check |
| `--protocol` | `TCP` | Protocol: `TCP`, `UDP`, or `SCTP` |

**Examples**

```bash
# Check if frontend can reach the payments service on port 80
kgraph can-reach pod/frontend svc/payments -n default --port 80

# Check Pod-to-Pod on a specific port
kgraph can-reach pod/web pod/db -n production --port 5432 --protocol TCP

# Check any port (ignores port restrictions)
kgraph can-reach pod/api svc/cache -n staging
```

**What it covers**

| Scenario | Evaluation |
|---|---|
| No NetworkPolicies | `ALLOWED` — all traffic unrestricted |
| Deny-all ingress | `BLOCKED` — shows which policy, no matching rule |
| Deny-all egress | `BLOCKED` — shows which policy |
| `podSelector` rules | Resolved from Pod labels |
| `namespaceSelector` rules | Resolved from Namespace labels |
| Port-specific rules | Checked numerically and by named port |
| `ipBlock` CIDR rules | `UNKNOWN` — cannot evaluate without live Pod IPs |
| Service destination | Expands to all endpoint Pods, shows per-Pod verdict |

When multiple pods match a name prefix (e.g., `frontend` matching two replica pods), KGraph picks one and notes that reachability is identical for all replicas of the same workload.

**Sample output**

```
Reachability check  (port 80/TCP)  context: kind-prod

  Source:       Pod default/frontend-54d-jj2lj
  Destination:  Service default/payments → 2 endpoint Pod(s)

  ─────────────────────────────────────────────────────────────────
  Verdict:  ✖ BLOCKED

  Egress (from default/frontend-54d-jj2lj)
    no NetworkPolicies restrict this direction → unrestricted

  Ingress (to default/payments-dcd-hfwng)
    NetworkPolicy default/payments-deny-all
      ✖ no rule permits this connection

  Suggestion
    add an ingress rule to NetworkPolicy "payments-deny-all" in namespace
    "default" allowing traffic from default/frontend-54d-jj2lj on port 80/TCP
```

**Exit codes**

| Code | Meaning |
|---|---|
| `0` | All destinations allowed |
| `1` | At least one destination blocked |

---

### `check`

Reads one or more local YAML manifests and cross-references every ConfigMap, Secret, PVC, ServiceAccount, and Ingress backend against the live cluster graph. Useful in CI/CD pipelines before `kubectl apply`.

```bash
kgraph check <file> [file...] [flags]
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `-n, --namespace` | context default | Default namespace for resources without an explicit namespace |

**Examples**

```bash
# Check a single manifest
kgraph check ./deploy.yaml -n production

# Check multiple files
kgraph check ./manifests/*.yaml -n staging

# Use in CI (exits 1 if any blocking issues found)
kgraph check ./k8s/api-deployment.yaml && kubectl apply -f ./k8s/api-deployment.yaml
```

**What is checked**

| Reference type | YAML path checked |
|---|---|
| ConfigMap | `volumes[].configMap`, `envFrom[].configMapRef`, `env[].valueFrom.configMapKeyRef` |
| Secret | `volumes[].secret`, `envFrom[].secretRef`, `env[].valueFrom.secretKeyRef`, `imagePullSecrets[]` |
| PVC | `volumes[].persistentVolumeClaim` |
| ServiceAccount | `spec.serviceAccountName` |
| Service | `spec.rules[].http.paths[].backend.service` (Ingress) |

**Sample output**

```
checking deploy.yaml (namespace: production)

  ✖  Secret api-tls                      not found in ns production  [volumes[1].secret]
  ✖  ConfigMap missing-config            not found in ns production  [env[0].valueFrom.configMapKeyRef]
  ●  ConfigMap app-config                exists  [volumes[0].configMap]
  ●  PVC data-vol                        exists  [volumes[2].persistentVolumeClaim]

  2 blocking · 0 warnings
```

**Exit codes**

| Code | Meaning |
|---|---|
| `0` | All references resolved |
| `1` | One or more blocking issues found |

---

### `orphan`

Scans the cluster graph for resources that no running Pod currently references. These are candidates for cleanup.

```bash
kgraph orphan [flags]
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `-n, --namespace` | all namespaces | Limit scan to one namespace |
| `-k, --kind` | all kinds | Limit to one kind: `ConfigMap`, `Secret`, `PVC`, `ServiceAccount`, `Service` |
| `--graph` | false | Show outgoing edges for each orphan (confirm it's truly unused) |

**Kinds checked**

| Kind | Orphan condition |
|---|---|
| `ConfigMap` | No Pod mounts or env-references it |
| `Secret` | No Pod mounts or env-references it |
| `PersistentVolumeClaim` | No Pod has it mounted |
| `ServiceAccount` | No Pod uses it |
| `Service` | Selector set but no matching running Pods, and no Ingress routes to it |

> **Note:** Detection uses live Pod graph edges, not Deployment pod templates. A ConfigMap referenced in a template with `replicas: 0` may appear as orphaned.

**Examples**

```bash
kgraph orphan
kgraph orphan -n default
kgraph orphan --kind Secret -n production
kgraph orphan --graph -n staging   # shows edges to help confirm
```

**Sample output**

```
Orphaned resources · namespace default · context: kind-prod

  ConfigMap
    ✖  default/old-config        no Pod mounts or references it
    ✖  default/temp-data         no Pod mounts or references it

  Secret
    ✖  default/old-tls-cert      no Pod mounts or references it

  Service
    ✖  default/legacy-api        selector matches no running Pods and no Ingress routes to it

  4 orphan(s) found
```

---

### `doctor`

Runs a suite of proactive, deterministic checks across the cluster graph and flags resources that are likely to cause outages, security incidents, or operational problems — before they happen.

```bash
kgraph doctor [flags]
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `-n, --namespace` | all namespaces | Limit scan to one namespace |

**Checks performed**

| Category | Check | Severity |
|---|---|---|
| High Availability | Deployment/StatefulSet with a single replica | ⚠ Warning |
| High Availability | Deployment with available replicas < desired replicas | ✖ Blocking |
| Reliability | Container with no liveness probe | ⚠ Warning |
| Reliability | Container with no readiness probe | ⚠ Warning |
| Reliability | Container with no resource limits | ⚠ Warning |
| Reliability | CronJob with `failedJobsHistoryLimit: 0` | ⚠ Warning |
| Security | Privileged container | ✖ Blocking |
| Security | Container running as root (UID 0) | ✖ Blocking |
| Security | Container with `allowPrivilegeEscalation: true` | ⚠ Warning |
| Storage | PVC without an explicit StorageClass | ⚠ Warning |
| Network | Service whose Pods have no NetworkPolicy in a policy-aware namespace | ⚠ Warning |

**Examples**

```bash
kgraph doctor
kgraph doctor -n production
```

**Sample output**

```
Doctor · all namespaces · context: kind-prod

  High Availability
    ✖  Deployment default/payments
          available replicas (0) < desired (2)
          some Pods are not ready; the service is running below capacity
          fix: run 'kgraph why deployment/payments' to find the root cause

    ⚠  Deployment default/web
          single replica — no high availability
          spec.replicas=1; a single Pod failure will cause downtime
          fix: set spec.replicas to at least 2 and configure a PodDisruptionBudget

  Security
    ✖  Pod default/debug-tools
          container "tools" runs as privileged
          a privileged container has full access to the host kernel and devices
          fix: remove securityContext.privileged or set it to false

  1 blocking · 2 warnings
```

**Exit codes**

| Code | Meaning |
|---|---|
| `0` | No issues found |
| `1` | Only warnings found |
| `2` | At least one blocking issue found |

---

### `rbac`

Traces whether a ServiceAccount is permitted to perform an action by evaluating Roles, ClusterRoles, RoleBindings, and ClusterRoleBindings. Shows the exact grant chain.

```bash
kgraph rbac <sa/name> <can-verb> <resource> [apiGroup] [flags]
```

**Arguments**

| Argument | Description |
|---|---|
| `sa/<name>` | ServiceAccount to check |
| `can-<verb>` | Action: `can-get`, `can-list`, `can-create`, `can-delete`, `can-update`, `can-patch`, `can-watch` |
| `<resource>` | Kubernetes resource: `pods`, `secrets`, `configmaps`, `deployments`, `pods/log`, etc. |
| `[apiGroup]` | Optional API group. Defaults to `""` (core). Use `apps` for Deployments, `batch` for Jobs. |

**Flags**

| Flag | Default | Description |
|---|---|---|
| `-n, --namespace` | `default` | Namespace of the ServiceAccount |

**Examples**

```bash
# Can the api-worker read Secrets?
kgraph rbac sa/api-worker can-get secrets -n default

# Can the batch-runner create Jobs?
kgraph rbac sa/batch-runner can-create jobs batch -n production

# Can the frontend read Pod logs?
kgraph rbac sa/frontend can-get pods/log -n default
```

**Sample output — GRANTED**

```
RBAC check · context: kind-prod

  ServiceAccount: default/api-worker
  Action:         get secrets  (apiGroup: core)

  ✓ GRANTED

  Grant paths
    RoleBinding/api-secret-reader
      namespace: default
      role: Role/secret-reader
      ✓ rules[0] grants get secrets

  Binding evaluation
    RoleBinding/api-secret-reader → Role/secret-reader  ✓ grants
```

**Sample output — DENIED**

```
RBAC check · context: kind-prod

  ServiceAccount: default/api-worker
  Action:         delete secrets  (apiGroup: core)

  ✖ DENIED

  Binding evaluation
    RoleBinding/api-secret-reader → Role/secret-reader  ✖ no matching rule
```

**Exit codes**

| Code | Meaning |
|---|---|
| `0` | Permission granted |
| `1` | Permission denied |

---

### `context`

Manages kubeconfig contexts.

```bash
kgraph context [subcommand]
```

| Subcommand | Description |
|---|---|
| `kgraph context list` | List all contexts in the kubeconfig |
| `kgraph context current` | Print the active context |
| `kgraph context use <name>` | Switch to a context |

**Examples**

```bash
kgraph context list
kgraph context current
kgraph context use production
```

---

### `explore` / `tui`

Launches the full interactive terminal explorer. All analysis features are available inside the TUI without typing separate commands.

```bash
kgraph explore
# Aliases:
kgraph tui
kgraph ui
```

---

## Interactive explorer (TUI)

The explorer is a three-pane terminal UI:

```
┌─────────────────┬───────────────────────────────┬────────────────────────┐
│  Resources      │  Graph (deps / impact tree)   │  Details + diagnosis   │
│                 │                               │                        │
│  Deployment api │  ◆ Deployment api             │  kind    Deployment    │
│  Pod api-abc    │  └─ ◇ ReplicaSet api-rs       │  ns      default       │
│  Service api    │     └─ ● Pod api-abc ✖Pending │  name    api           │
│  ...            │                               │                        │
│                 │                               │  Why it is failing     │
│                 │                               │  ✖ ImagePullBackOff    │
└─────────────────┴───────────────────────────────┴────────────────────────┘
 ↑/↓ select · tab deps⇄impact · p problems · o orphans · / filter · q quit
```

### Keyboard shortcuts

| Key | Action |
|---|---|
| `↑` / `↓` | Move selection in resource list |
| `Tab` | Toggle dependency / impact mode in graph pane |
| `PgUp` / `PgDn` | Scroll the graph pane |
| `Space` / `b` / `f` | Scroll the graph pane |
| `/` | Open fuzzy filter on resource list |
| `c` | Open context switcher |
| `p` | Open **Problems** view (unhealthy Pods, PVCs, Services) |
| `o` | Open **Orphan** view (unused ConfigMaps, Secrets, PVCs, etc.) |
| `:` | Open command prompt (k9s-style) |
| `q` / `Esc` | Quit |
| `Ctrl+C` | Force quit |

### Command prompt

Press `:` to open the command prompt. Available commands:

| Command | Effect |
|---|---|
| `:pods` | Show only Pod resources |
| `:deploy` | Show only Deployments |
| `:sts` | Show only StatefulSets |
| `:ds` | Show only DaemonSets |
| `:svc` | Show only Services |
| `:ing` | Show only Ingresses |
| `:cm` | Show only ConfigMaps |
| `:secret` | Show only Secrets |
| `:pvc` | Show only PersistentVolumeClaims |
| `:node` | Show only Nodes |
| `:sa` | Show only ServiceAccounts |
| `:ns` | Open namespace picker |
| `:problems` | Open Problems view |
| `:orphan` | Open Orphan view |
| `:all` | Clear all filters (show everything) |
| `:ctx` | Open context switcher |
| `:q` | Quit |

### Namespace picker

Press `:ns` to open the namespace picker. The first option is **All namespaces**. Selecting a namespace filters the resource list to that namespace while keeping the full graph intact (cross-namespace relationships remain visible in the graph pane).

### Problems view

Press `p` or type `:problems` to open the Problems view. It lists all currently unhealthy resources in scope:

- **Pods** in non-Running phases or not-Ready
- **PVCs** in Pending or Lost phase
- **Services** whose selector matches no running Pods

For each selected problem, the right pane shows:
- Immediate failure state (e.g., `phase=Pending`)
- **Why it is failing** — deterministic diagnosis from container status, Kubernetes Warning events, and bounded previous-container logs
- **Affected resources** — the reverse dependency graph showing which workloads and Services are impacted

### Orphan view

Press `o` or type `:orphan` to open the Orphan view. It lists all resources not referenced by any running Pod, grouped by kind. The right pane shows outgoing and incoming graph edges to help confirm whether the resource is truly unused before deletion.

---

## Resource kinds and aliases

All commands accept both full kind names and common aliases.

| Aliases | Canonical kind |
|---|---|
| `pod`, `pods`, `po` | `Pod` |
| `deploy`, `deployment`, `deployments` | `Deployment` |
| `rs`, `replicaset`, `replicasets` | `ReplicaSet` |
| `sts`, `statefulset`, `statefulsets` | `StatefulSet` |
| `ds`, `daemonset`, `daemonsets` | `DaemonSet` |
| `job`, `jobs` | `Job` |
| `cj`, `cronjob`, `cronjobs` | `CronJob` |
| `svc`, `service`, `services` | `Service` |
| `ing`, `ingress`, `ingresses` | `Ingress` |
| `cm`, `configmap`, `configmaps` | `ConfigMap` |
| `secret`, `secrets` | `Secret` |
| `pvc`, `persistentvolumeclaim` | `PersistentVolumeClaim` |
| `pv`, `persistentvolume` | `PersistentVolume` |
| `no`, `node`, `nodes` | `Node` |
| `sa`, `serviceaccount`, `serviceaccounts` | `ServiceAccount` |

---

## Snapshots storage

Snapshots are stored locally on disk at:

```
~/.kgraph/snapshots/<context-name>/<snapshot-name>.json
```

Each snapshot file captures:
- Timestamp and context name
- All resource nodes (kind, namespace, name, UID)
- Key extracted fields per kind (phase, images, replicas, selector, etc.)
- All graph edges (from, to, relationship type)

Snapshot files are human-readable JSON and can be inspected or committed to version control.

---

## How it works

KGraph uses `client-go` to read resources from your cluster via standard Kubernetes list APIs (read-only, no watches in CLI mode). From the collected resources it builds an in-memory directed graph:

```
kubeconfig
    │
    ▼
client-go (read-only list APIs)
    │
    ▼
collector  →  graph.Resources (typed snapshot)
    │
    ▼
graph.Build  →  graph.Graph (nodes + edges)
    │
    ├── query engine   (BFS/DFS traversals)
    ├── diagnosis      (status + events + logs)
    ├── network        (NetworkPolicy reachability)
    ├── snapshot       (serialise / diff)
    ├── checker        (manifest cross-reference)
    └── orphan         (reverse-reachability scan)
```

**Resources collected:** Deployments, ReplicaSets, StatefulSets, DaemonSets, Jobs, CronJobs, Pods, Services, Ingresses, ConfigMaps, Secrets, PVCs, PVs, Nodes, ServiceAccounts, NetworkPolicies, Namespaces.

**Graph edges modelled:**

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

Every CLI command is a graph traversal, not an ad-hoc API call. This makes results deterministic, explainable, and fast (one collection pass per command).

---

## Building from source

```bash
git clone https://github.com/anishetty/kgraph
cd kgraph

# Build
go build ./...

# Run all tests
go test ./...

# Static checks
go vet ./...

# Install binary
go install .

# Run locally without installing
go run . --help
```

**Project layout**

```
cmd/                    # Cobra command entrypoints
internal/
    collector/          # client-go read-only resource collection
    graph/              # nodes, edges, graph model
    query/              # BFS/DFS traversals, orphan detection
    diagnosis/          # deterministic failure analysis
    network/            # NetworkPolicy reachability engine
    snapshot/           # point-in-time graph serialisation + diff
    checker/            # manifest cross-reference validator
    health/             # deterministic Pod health checks
    metrics/            # live CPU/memory from metrics-server
    renderer/           # box-drawing tree output (lipgloss)
    tui/                # interactive terminal UI (Bubble Tea)
main.go
```

---

## Design principles

- **100% Go.** No other languages, no shelling out to `kubectl`.
- **No AI / no LLM calls.** All analysis is deterministic and explainable.
- **No CRDs and no cluster-side components.** Read-only access via Kubernetes APIs only.
- **Local-first.** Snapshots and state persist to the local filesystem, never to a cloud service.
- **The graph is the heart.** Every command is a traversal — never an ad-hoc API call.
