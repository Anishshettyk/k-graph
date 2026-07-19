# KGraph - Kubernetes Knowledge Graph CLI

## Vision

KGraph is a **local-first Kubernetes CLI** written entirely in **Go**.

It uses the user's existing `~/.kube/config` to connect to one or more
clusters and build a **knowledge graph** of Kubernetes resources. The
tool performs deterministic analysis---no AI, no cloud services, no
agents.

**Goal:** Answer *why*, *what changed*, *what depends on this*, and
*what breaks if I modify this?*

------------------------------------------------------------------------

# Principles

-   100% Go implementation
-   Learn Go deeply while building
-   No AI
-   No CRDs
-   No cluster-side installation
-   Uses only Kubernetes APIs via `client-go`
-   Fast, local, explainable

------------------------------------------------------------------------

# High-Level Architecture

``` text
kubeconfig
    │
client-go
    │
Collectors / Informers
    │
Knowledge Graph
    ├── Dependency Engine
    ├── Snapshot Store
    ├── Event History
    ├── Diff Engine
    └── Query Engine
             │
          CLI Commands
```

------------------------------------------------------------------------

# Core Modules

## 1. Collector

Responsible for fetching:

-   Deployments
-   ReplicaSets
-   Pods
-   Services
-   Endpoints
-   StatefulSets
-   DaemonSets
-   Jobs
-   CronJobs
-   Ingresses
-   ConfigMaps
-   Secrets
-   PVCs
-   PVs
-   StorageClasses
-   Nodes
-   Namespaces
-   NetworkPolicies
-   ServiceAccounts
-   Roles / RoleBindings

Uses `client-go` informers and watches.

------------------------------------------------------------------------

## 2. Graph Builder

Every object becomes a node.

Relationships become edges.

Examples:

Deployment → ReplicaSet

ReplicaSet → Pod

Service → Pod

Ingress → Service

Pod → Secret

Pod → ConfigMap

Pod → PVC

PVC → PV

Pod → Node

The graph is the heart of the application.

------------------------------------------------------------------------

## 3. Query Engine

Everything is implemented as graph traversals.

Algorithms:

-   BFS
-   DFS
-   Reachability
-   Reverse dependency lookup
-   Impact analysis
-   Cycle detection (future)

------------------------------------------------------------------------

## 4. Snapshot Engine

Stores periodic snapshots locally.

Future commands:

-   Compare snapshots
-   Reconstruct historical state
-   Detect drift

------------------------------------------------------------------------

## 5. History Engine

Persist Kubernetes watch events locally.

Provides timelines and object history.

------------------------------------------------------------------------

# CLI Commands (MVP)

## why

Explain why an object is unhealthy.

Example:

    kgraph why deployment/frontend

Walks dependencies until it finds the root cause.

------------------------------------------------------------------------

## deps

    kgraph deps redis

Shows everything depending on Redis.

------------------------------------------------------------------------

## impact

    kgraph impact secret/payment-db

Shows affected Deployments, Pods, Services and workloads.

------------------------------------------------------------------------

## network

Displays traffic path:

Ingress → Service → Pods → Dependencies

------------------------------------------------------------------------

## diff

    kgraph diff --since 2h

Shows semantic cluster changes instead of YAML differences.

------------------------------------------------------------------------

## history

Displays timeline of changes for a resource.

------------------------------------------------------------------------

## orphan

Finds unused:

-   Secrets
-   ConfigMaps
-   PVCs
-   Services

------------------------------------------------------------------------

## doctor (Phase 2)

Runs deterministic health checks.

Examples:

-   Missing endpoints
-   Broken selectors
-   Pending PVCs
-   Missing StorageClasses
-   Invalid Ingress backends

------------------------------------------------------------------------

# Suggested Project Structure

``` text
cmd/
internal/
    collector/
    graph/
    query/
    renderer/
    snapshot/
    history/
    rules/
pkg/
```

------------------------------------------------------------------------

# Rendering

Start with rich terminal output.

Possible libraries:

-   cobra
-   bubbletea
-   lipgloss

Add optional TUI later.

------------------------------------------------------------------------

# Roadmap

## Phase 1

-   Go project setup
-   Cobra CLI
-   client-go integration
-   Resource collector
-   Graph builder

## Phase 2

-   Dependency engine
-   `why`
-   `deps`
-   `impact`

## Phase 3

-   Snapshot engine
-   History engine
-   Diff engine

## Phase 4

-   Network visualization
-   TUI
-   Export graphs (Mermaid/DOT)

## Phase 5

-   Performance optimization
-   Multi-cluster support
-   Plugin architecture

------------------------------------------------------------------------

# Go Learning Goals

This project should intentionally teach:

-   Modules
-   Packages
-   Interfaces
-   Structs
-   Error handling
-   Generics (where useful)
-   Goroutines
-   Channels
-   Context
-   Worker pools
-   Mutexes
-   Testing
-   Benchmarking
-   Cobra
-   client-go
-   File persistence
-   Graph algorithms

------------------------------------------------------------------------

# Long-Term Vision

Become the CLI engineers install alongside `kubectl`.

Not another dashboard.

A deterministic Kubernetes reasoning engine that helps users understand
complex clusters quickly.
