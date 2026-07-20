import { useCallback, useEffect, useMemo, useState } from 'react'
import {
    ReactFlow, Controls, MiniMap,
    useNodesState, useEdgesState, useReactFlow, ReactFlowProvider,
    type Node, type Edge,
} from '@xyflow/react'
import dagre from 'dagre'
import { Network } from 'lucide-react'
import '@xyflow/react/dist/style.css'
import { ResourceNode } from './ResourceNode'
import { kindColor } from '../lib/kinds'
import type { GraphResponse, NodeInfo, EdgeInfo } from '../types/api'
const nodeTypes = { resource: ResourceNode }

const TOP_LEVEL_KINDS = new Set([
    'Deployment', 'StatefulSet', 'DaemonSet', 'CronJob', 'Job',
    'Service', 'Ingress', 'NetworkPolicy',
])

/**
 * BFS in both directions from a starting node, returning all UIDs within
 * `forwardHops` hops forward (dependencies) and `backHops` hops backward
 * (things that depend on this resource). This powers the neighbourhood
 * graph shown when a resource is selected.
 */
function computeNeighborhood(
    startUID: string,
    edges: { from: string; to: string }[],
    forwardHops = 4,
    backHops = 2,
): Set<string> {
    const visited = new Set<string>([startUID])

    // Forward BFS (deps: follows outgoing edges)
    let frontier = [startUID]
    for (let hop = 0; hop < forwardHops && frontier.length; hop++) {
        const next: string[] = []
        for (const uid of frontier) {
            for (const e of edges) {
                if (e.from === uid && !visited.has(e.to)) {
                    visited.add(e.to)
                    next.push(e.to)
                }
            }
        }
        frontier = next
    }

    // Backward BFS (impact: follows incoming edges)
    frontier = [startUID]
    for (let hop = 0; hop < backHops && frontier.length; hop++) {
        const next: string[] = []
        for (const uid of frontier) {
            for (const e of edges) {
                if (e.to === uid && !visited.has(e.from)) {
                    visited.add(e.from)
                    next.push(e.from)
                }
            }
        }
        frontier = next
    }

    return visited
}

const NODE_W = 210
const NODE_H = 90
const POD_SIZE = 130  // Pod nodes are circles — reduced for readability

// Returns the dagre bounding box for a given node kind so layout avoids overlaps.
function nodeDimensions(kind: string): { width: number; height: number } {
    if (kind === 'Pod') return { width: POD_SIZE + 30, height: POD_SIZE + 30 }
    if (kind === 'Service') return { width: NODE_W + 20, height: NODE_H + 20 }  // hexagon needs padding
    if (kind === 'Ingress') return { width: NODE_W + 30, height: NODE_H + 20 }  // arrow shape
    return { width: NODE_W, height: NODE_H }
}

function deriveWorkloadEdges(data: GraphResponse): EdgeInfo[] {
    const nodeByUID = new Map(data.nodes.map(n => [n.uid, n]))

    const parentOf = new Map<string, string>()
    data.edges.forEach(e => {
        if (e.rel === 'owns') parentOf.set(e.to, e.from)
    })

    const WORKLOAD_KINDS = new Set(['Deployment', 'StatefulSet', 'DaemonSet', 'Job', 'CronJob'])
    const podToWorkload = new Map<string, string>()
    data.nodes
        .filter(n => n.kind === 'Pod')
        .forEach(pod => {
            let cur = pod.uid
            for (let i = 0; i < 5; i++) {
                const parent = parentOf.get(cur)
                if (!parent) break
                const pNode = nodeByUID.get(parent)
                if (pNode && WORKLOAD_KINDS.has(pNode.kind)) {
                    podToWorkload.set(pod.uid, parent)
                    break
                }
                cur = parent
            }
        })

    const svcToWorkloads = new Map<string, Set<string>>()
    data.edges
        .filter(e => e.rel === 'selects')
        .forEach(e => {
            const workload = podToWorkload.get(e.to)
            if (!workload) return
            if (!svcToWorkloads.has(e.from)) svcToWorkloads.set(e.from, new Set())
            svcToWorkloads.get(e.from)!.add(workload)
        })

    const derived: EdgeInfo[] = []
    svcToWorkloads.forEach((workloads, svcUID) => {
        workloads.forEach(workloadUID => {
            derived.push({ from: svcUID, to: workloadUID, rel: 'routes to' })
        })
    })
    return derived
}

interface Props {
    data: GraphResponse
    selectedUID: string | null
    onSelect: (node: NodeInfo) => void
    filterKind: string
    heatmapMode?: boolean
    onContextMenu?: (x: number, y: number, node: NodeInfo) => void
}

// ─── Heatmap health colour per node ─────────────────────────────────────────
function heatmapColor(node: NodeInfo, data: GraphResponse): string {
    if (node.kind === 'Pod') return node.healthy ? '#22c55e' : '#f87171'
    if (node.kind === 'Deployment' || node.kind === 'StatefulSet') {
        const ready = parseInt(node.fields?.readyReplicas ?? '-1')
        const total = parseInt(node.fields?.replicas ?? '0')
        if (ready < 0 || total === 0) return '#94a3b8'
        if (ready === total) return '#22c55e'
        if (ready === 0) return '#f87171'
        return '#fbbf24'
    }
    if (node.kind === 'PersistentVolumeClaim') {
        const phase = node.fields?.phase
        if (phase === 'Bound') return '#22c55e'
        if (phase === 'Lost') return '#f87171'
        if (phase === 'Pending') return '#fbbf24'
        return '#94a3b8'
    }
    if (node.kind === 'Service') {
        const hasSelector = node.fields?.selector && node.fields.selector !== ''
        const hasPods = data.edges.some(e => e.from === node.uid && e.rel === 'selects')
        if (hasSelector && !hasPods) return '#f87171'
        return '#22c55e'
    }
    return '#94a3b8'
}

// Relationships where we show a label (these have semantic meaning beyond
// what the node shapes already convey).
const LABELED_RELS = new Set(['routes to', 'selects', 'grants', 'role ref'])
// Relationships where the edge path itself should animate (show traffic flow).
const ANIMATED_RELS = new Set(['routes to', 'selects'])

function makeFlowEdge(id: string, source: string, target: string, label: string): Edge {
    const showLabel = LABELED_RELS.has(label)
    const animate = ANIMATED_RELS.has(label)
    return {
        id,
        source,
        target,
        // Only label meaningful routing relationships; structural ones (owns, mounts, etc.)
        // are implied by the node kinds and clutter the graph.
        label: showLabel ? label : undefined,
        type: 'default',  // bezier curves
        animated: animate,
        // No dashed box around labels — just plain text
        labelStyle: showLabel ? { fill: '#64748b', fontSize: 9, fontFamily: 'JetBrains Mono, monospace' } : undefined,
        labelBgStyle: showLabel ? { fill: '#080d1a', fillOpacity: 0.9 } : undefined,
        labelBgPadding: showLabel ? [4, 2] as [number, number] : undefined,
        style: { stroke: edgeColor(label), strokeWidth: animate ? 1.5 : 1 },
        markerEnd: { type: 'arrowclosed' as const, color: edgeColor(label), width: 14, height: 14 },
    }
}

function edgeColor(rel: string): string {
    switch (rel) {
        case 'routes to': return '#f97316'  // orange — traffic path
        case 'selects': return '#d946ef'  // fuchsia — service mesh
        case 'owns': return '#334155'  // dark slate — structural
        case 'mounts': return '#1e40af'  // navy — config reference
        case 'grants': return '#7c3aed'  // violet — RBAC
        default: return '#1e3254'  // default dark blue
    }
}

function GraphCanvasInner({ data, selectedUID, onSelect, filterKind, heatmapMode, onContextMenu }: Props) {
    const [nodes, setNodes, onNodesChange] = useNodesState<Node>([])
    const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])
    const { fitView } = useReactFlow()
    const [hoveredUID, setHoveredUID] = useState<string | null>(null)

    // Build the set of neighbours for the currently hovered node so we can dim the rest.
    const hoveredNeighbours = useMemo(() => {
        if (!hoveredUID) return null
        const nbrs = new Set([hoveredUID])
        data.edges.forEach(e => {
            if (e.from === hoveredUID) nbrs.add(e.to)
            if (e.to === hoveredUID) nbrs.add(e.from)
        })
        return nbrs
    }, [hoveredUID, data.edges])

    const { flowNodes, flowEdges } = useMemo(() => {
        let visibleUIDs: Set<string>

        if (selectedUID) {
            // Neighbourhood mode: the clicked resource + everything connected to it
            visibleUIDs = computeNeighborhood(selectedUID, data.edges)
            // Still apply kind filter if active (e.g. "show only Pods in this neighbourhood")
            if (filterKind) {
                visibleUIDs = new Set(
                    [...visibleUIDs].filter(uid => {
                        const n = data.nodes.find(n => n.uid === uid)
                        return n && n.kind === filterKind
                    })
                )
                // Always keep the selected node itself visible
                visibleUIDs.add(selectedUID)
            }
        } else if (filterKind) {
            // Kind filter active with no selection: show all of that kind
            visibleUIDs = new Set(
                data.nodes.filter(n => n.kind === filterKind).map(n => n.uid)
            )
        } else {
            // Default: topology overview — only workloads, services, ingresses
            visibleUIDs = new Set(
                data.nodes.filter(n => TOP_LEVEL_KINDS.has(n.kind)).map(n => n.uid)
            )
        }

        const rawNodes: Node[] = data.nodes
            .filter(n => visibleUIDs.has(n.uid))
            .map(n => ({
                id: n.uid,
                type: 'resource',
                position: { x: 0, y: 0 },
                data: {
                    ...n,
                    selected: n.uid === selectedUID,
                    dimmed: hoveredNeighbours ? !hoveredNeighbours.has(n.uid) : false,
                    heatmapColor: heatmapMode ? heatmapColor(n, data) : undefined,
                },
                selected: n.uid === selectedUID,
            }))

        const directEdges: Edge[] = data.edges
            .filter(e => visibleUIDs.has(e.from) && visibleUIDs.has(e.to))
            .map((e, i) => makeFlowEdge(`e${i}-${e.from}-${e.to}`, e.from, e.to, e.rel))

        // Virtual Service→Workload edges only in the default topology overview
        const virtualEdges: Edge[] = (!filterKind && !selectedUID)
            ? deriveWorkloadEdges(data)
                .filter(e => visibleUIDs.has(e.from) && visibleUIDs.has(e.to))
                .map((e, i) => makeFlowEdge(`v${i}-${e.from}-${e.to}`, e.from, e.to, e.rel))
            : []

        const directSet = new Set(directEdges.map(e => `${e.source}-${e.target}`))
        const uniqueVirtual = virtualEdges.filter(e => !directSet.has(`${e.source}-${e.target}`))
        const rawEdges = [...directEdges, ...uniqueVirtual]

        return { flowNodes: rawNodes, flowEdges: rawEdges }
    }, [data, filterKind, selectedUID, hoveredNeighbours, heatmapMode])

    useEffect(() => {
        if (flowNodes.length === 0) {
            setNodes([])
            setEdges([])
            return
        }

        const g = new dagre.graphlib.Graph()
        // In neighbourhood mode many node sizes vary (Pods are circles, PVCs are
        // cylinders). Use TB layout when many kinds are mixed so the dep chain
        // flows top-down; use LR for the clean topology overview.
        const dir = selectedUID ? 'TB' : 'LR'
        // More generous spacing in neighbourhood mode so mixed node sizes don’t crowd each other
        const nodesep = selectedUID ? 80 : 60
        const ranksep = selectedUID ? 160 : 140
        g.setGraph({ rankdir: dir, nodesep, ranksep, marginx: 50, marginy: 50 })
        g.setDefaultEdgeLabel(() => ({}))

        flowNodes.forEach(n => {
            const info = data.nodes.find(ni => ni.uid === n.id)
            const dims = info ? nodeDimensions(info.kind) : { width: NODE_W, height: NODE_H }
            g.setNode(n.id, dims)
        })
        flowEdges.forEach(e => {
            try { g.setEdge(e.source, e.target) } catch { /* skip invalid edges */ }
        })
        dagre.layout(g)

        const laid = flowNodes.map(n => {
            const info = data.nodes.find(ni => ni.uid === n.id)
            const dims = info ? nodeDimensions(info.kind) : { width: NODE_W, height: NODE_H }
            const pos = g.node(n.id)
            if (!pos) return n
            return { ...n, position: { x: pos.x - dims.width / 2, y: pos.y - dims.height / 2 } }
        })

        // After dagre positions are known, pick the best handle pair for each
        // edge based on the relative positions of its source and target nodes.
        // This routes edges through the nearest face of each node instead of
        // always forcing left→right, which creates a tangled mess in TB layouts.
        const posMap = new Map(laid.map(n => [n.id, n.position]))
        const routedEdges = flowEdges.map(edge => {
            const sp = posMap.get(edge.source)
            const tp = posMap.get(edge.target)
            if (!sp || !tp) return edge
            const dx = (tp.x) - (sp.x)
            const dy = (tp.y) - (sp.y)
            let sourceHandle: string, targetHandle: string
            if (Math.abs(dy) > Math.abs(dx) * 0.7) {
                if (dy > 0) { sourceHandle = 'bottom-s'; targetHandle = 'top-t' }
                else { sourceHandle = 'top-s'; targetHandle = 'bottom-t' }
            } else {
                if (dx > 0) { sourceHandle = 'right-s'; targetHandle = 'left-t' }
                else { sourceHandle = 'left-s'; targetHandle = 'right-t' }
            }
            return { ...edge, sourceHandle, targetHandle }
        })

        setNodes(laid)
        setEdges(routedEdges)
        // Two-pass fitView: quick pass for immediate feedback, second pass
        // after React Flow has fully painted the positioned nodes.
        setTimeout(() => fitView({ padding: 0.15, maxZoom: 1.1, duration: 300 }), 50)
        setTimeout(() => fitView({ padding: 0.15, maxZoom: 1.1, duration: 400 }), 300)
    }, [flowNodes, flowEdges, selectedUID, setNodes, setEdges, fitView])

    const handleNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
        const info = data.nodes.find(n => n.uid === node.id)
        if (info) onSelect(info)
    }, [data.nodes, onSelect])

    // Hover: imperatively dim unrelated edges without re-running dagre.
    const handleNodeMouseEnter = useCallback((_: React.MouseEvent, node: Node) => {
        setHoveredUID(node.id)
        setEdges(es => es.map(e => {
            const connected = e.source === node.id || e.target === node.id
            return {
                ...e,
                style: {
                    ...e.style,
                    opacity: connected ? 1 : 0.12,
                    stroke: connected ? (e.animated ? '#f97316' : '#38bdf8') : '#1e3254',
                    strokeWidth: connected ? 2 : 1,
                },
                markerEnd: connected
                    ? { type: 'arrowclosed' as const, color: e.animated ? '#f97316' : '#38bdf8', width: 14, height: 14 }
                    : { type: 'arrowclosed' as const, color: '#1e3254', width: 10, height: 10 },
            }
        }))
    }, [setHoveredUID, setEdges])

    const handleNodeMouseLeave = useCallback(() => {
        setHoveredUID(null)
        // Reset edge styles to defaults
        setEdges(es => es.map(e => ({
            ...e,
            style: { stroke: edgeColor(e.label as string ?? ''), strokeWidth: e.animated ? 1.5 : 1 },
            markerEnd: { type: 'arrowclosed' as const, color: edgeColor(e.label as string ?? ''), width: 14, height: 14 },
        })))
    }, [setHoveredUID, setEdges])

    // Right-click → context menu
    const handleNodeContextMenu = useCallback((e: React.MouseEvent, node: Node) => {
        e.preventDefault()
        const info = data.nodes.find(n => n.uid === node.id)
        if (info && onContextMenu) onContextMenu(e.clientX, e.clientY, info)
        // Also select the node so the context menu actions have a target
        if (info) onSelect(info)
    }, [data.nodes, onContextMenu, onSelect])

    // Keyboard navigation: j/k or ↑/↓ move between visible nodes when graph is focused.
    const containerRef = useCallback((el: HTMLDivElement | null) => {
        if (!el) return
        const sortedUIDs = () =>
            nodes
                .slice()
                .sort((a, b) => a.position.y !== b.position.y ? a.position.y - b.position.y : a.position.x - b.position.x)
                .map(n => n.id)

        const handler = (ev: KeyboardEvent) => {
            // Don't hijack keys when the user is typing
            const tag = (ev.target as HTMLElement)?.tagName
            if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return
            if (!['ArrowDown', 'ArrowUp', 'ArrowRight', 'ArrowLeft', 'j', 'k'].includes(ev.key)) return

            ev.preventDefault()
            const uids = sortedUIDs()
            if (uids.length === 0) return

            const cur = selectedUID ? uids.indexOf(selectedUID) : -1

            if (ev.key === 'ArrowDown' || ev.key === 'j') {
                const next = uids[(cur + 1) % uids.length]
                const info = data.nodes.find(n => n.uid === next)
                if (info) onSelect(info)
            } else if (ev.key === 'ArrowUp' || ev.key === 'k') {
                const prev = uids[(cur - 1 + uids.length) % uids.length]
                const info = data.nodes.find(n => n.uid === prev)
                if (info) onSelect(info)
            } else if (ev.key === 'ArrowRight' && selectedUID) {
                // Move to the first forward neighbour
                const neighbour = data.edges.find(e => e.from === selectedUID)
                if (neighbour) {
                    const info = data.nodes.find(n => n.uid === neighbour.to)
                    if (info) onSelect(info)
                }
            } else if (ev.key === 'ArrowLeft' && selectedUID) {
                // Move to the first backward neighbour
                const neighbour = data.edges.find(e => e.to === selectedUID)
                if (neighbour) {
                    const info = data.nodes.find(n => n.uid === neighbour.from)
                    if (info) onSelect(info)
                }
            }
        }
        el.addEventListener('keydown', handler)
        return () => el.removeEventListener('keydown', handler)
    }, [nodes, selectedUID, data.nodes, data.edges, onSelect])

    const visibleCount = nodes.length

    return (
        <div ref={containerRef} className="w-full h-full relative" tabIndex={0} style={{ outline: 'none' }}>
            <ReactFlow
                nodes={nodes}
                edges={edges}
                onNodesChange={onNodesChange}
                onEdgesChange={onEdgesChange}
                onNodeClick={handleNodeClick}
                onNodeMouseEnter={handleNodeMouseEnter}
                onNodeMouseLeave={handleNodeMouseLeave}
                onNodeContextMenu={handleNodeContextMenu}
                nodeTypes={nodeTypes}
                fitView
                fitViewOptions={{ padding: 0.12, maxZoom: 1.2 }}
                minZoom={0.05}
                maxZoom={3}
                proOptions={{ hideAttribution: true }}
                className="w-full h-full bg-hex"
            >
                <Controls showInteractive={false} className="!bottom-4 !left-4" />
                {visibleCount > 0 && visibleCount <= 80 && (
                    <MiniMap
                        nodeColor={n => {
                            const info = data.nodes.find(ni => ni.uid === n.id)
                            if (!info) return '#1e3254'
                            if (info.kind === 'Pod' && !info.healthy) return '#f87171'
                            return kindColor(info.kind) + '80'
                        }}
                        style={{ width: 140, height: 90 }}
                        className="!bottom-4 !right-4 !rounded-lg"
                        maskColor="rgba(4,6,14,0.7)"
                    />
                )}
            </ReactFlow>

            {visibleCount === 0 && (
                <div className="absolute inset-0 flex flex-col items-center justify-center text-slate-500 z-10 pointer-events-none">
                    <div className="bg-space-900/80 p-8 rounded-2xl border border-space-700 backdrop-blur-sm flex flex-col items-center shadow-xl">
                        <Network className="w-16 h-16 text-accent mb-4 opacity-80" strokeWidth={1} />
                        <h3 className="text-lg font-medium text-slate-300 mb-2">No Resource Selected</h3>
                        <p className="text-sm text-center max-w-xs mb-6 text-slate-400">
                            Click any resource in the sidebar to explore its dependencies and network topology.
                        </p>
                        <div className="flex items-center gap-4 text-xs font-mono opacity-80">
                            <span className="flex items-center gap-1.5"><div className="w-3 h-3 rounded bg-blue-500/20 border border-blue-500/50" /> Workload</span>
                            <span className="flex items-center gap-1.5"><div className="w-4 h-3 rounded bg-fuchsia-500/20 border border-fuchsia-500/50" style={{ clipPath: 'polygon(15% 0%,85% 0%,100% 50%,85% 100%,15% 100%,0% 50%)' }} /> Service</span>
                            <span className="flex items-center gap-1.5"><div className="w-5 h-3 bg-orange-500/20 border border-orange-500/50" style={{ clipPath: 'polygon(0% 0%,80% 0%,100% 50%,80% 100%,0% 100%,20% 50%)' }} /> Ingress</span>
                            <span className="flex items-center gap-1.5"><div className="w-3 h-3 rounded-full bg-green-500/20 border border-green-500/50" /> Pod</span>
                        </div>
                    </div>
                </div>
            )}
        </div>
    )
}

export function GraphCanvas(props: Props) {
    return (
        <ReactFlowProvider>
            <GraphCanvasInner {...props} />
        </ReactFlowProvider>
    )
}
