import { useMemo, useCallback, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
    ReactFlow, Controls, MiniMap, Background, BackgroundVariant,
    useNodesState, useEdgesState, ReactFlowProvider,
    type Node, type Edge,
    Handle, Position, MarkerType,
} from '@xyflow/react'
import dagre from 'dagre'
import { Loader2, Globe, Network, Shield, RefreshCw, Info } from 'lucide-react'
import clsx from 'clsx'
import '@xyflow/react/dist/style.css'
import { api } from '../api/client'
import { useStore } from '../store/useStore'
import type { NodeInfo } from '../types/api'

// ─── Colour palette ──────────────────────────────────────────────────────────
const C = {
    internet:  { bg: '#0f172a', border: '#38bdf8', glow: '#38bdf8', text: '#7dd3fc' },
    ingress:   { bg: '#1e0a1e', border: '#e879f9', glow: '#d946ef', text: '#f0abfc' },
    service:   { bg: '#0a1628', border: '#3b82f6', glow: '#2563eb', text: '#93c5fd' },
    pod_ok:    { bg: '#071a10', border: '#22c55e', glow: '#16a34a', text: '#86efac' },
    pod_bad:   { bg: '#1a0707', border: '#ef4444', glow: '#dc2626', text: '#fca5a5' },
    policy:    { bg: '#1a1000', border: '#f59e0b', glow: '#d97706', text: '#fcd34d' },
}

// ─── Custom node components ───────────────────────────────────────────────────

function glowStyle(color: string) {
    return { boxShadow: `0 0 14px ${color}44, 0 0 4px ${color}22, inset 0 0 12px ${color}11` }
}

function NodeBase({ c, icon, title, subtitle, badge, handles = 'both' }: {
    c: typeof C.ingress; icon: React.ReactNode; title: string
    subtitle?: string; badge?: string; handles?: 'both' | 'left' | 'right' | 'none'
}) {
    return (
        <div className="relative rounded-xl border px-3 py-2 min-w-[120px] max-w-[180px] select-none"
            style={{ backgroundColor: c.bg, borderColor: c.border, ...glowStyle(c.glow) }}>
            {(handles === 'both' || handles === 'left') && (
                <Handle type="target" position={Position.Left}
                    style={{ background: c.border, border: `2px solid ${c.glow}`, width: 8, height: 8 }} />
            )}
            <div className="flex items-center gap-1.5">
                <span style={{ color: c.text }}>{icon}</span>
                <span className="text-[11px] font-semibold truncate" style={{ color: c.text }}>{title}</span>
            </div>
            {subtitle && <div className="text-[9px] font-mono opacity-60 mt-0.5 truncate" style={{ color: c.text }}>{subtitle}</div>}
            {badge && (
                <div className="absolute -top-2 -right-2 text-[8px] font-bold px-1.5 py-0.5 rounded-full border"
                    style={{ backgroundColor: c.bg, borderColor: c.border, color: c.text }}>
                    {badge}
                </div>
            )}
            {(handles === 'both' || handles === 'right') && (
                <Handle type="source" position={Position.Right}
                    style={{ background: c.border, border: `2px solid ${c.glow}`, width: 8, height: 8 }} />
            )}
        </div>
    )
}

function InternetNode() {
    return (
        <NodeBase c={C.internet} icon={<Globe className="w-3.5 h-3.5" />}
            title="Internet" subtitle="external traffic" handles="right" />
    )
}
function IngressNode({ data }: { data: { node: NodeInfo } }) {
    const n = data.node
    return (
        <NodeBase c={C.ingress} icon={<Globe className="w-3.5 h-3.5" />}
            title={n.name} subtitle={n.namespace} badge="INGRESS" handles="both" />
    )
}
function ServiceNode({ data }: { data: { node: NodeInfo } }) {
    const n = data.node
    return (
        <NodeBase c={C.service} icon={<Network className="w-3.5 h-3.5" />}
            title={n.name} subtitle={n.fields?.type ?? n.namespace} badge="SVC" handles="both" />
    )
}
function PodNode({ data }: { data: { node: NodeInfo } }) {
    const n = data.node
    const c = n.healthy ? C.pod_ok : C.pod_bad
    return (
        <div className="relative rounded-full border px-3 py-2 min-w-[100px] max-w-[160px] select-none text-center"
            style={{ backgroundColor: c.bg, borderColor: c.border, ...glowStyle(c.glow) }}>
            <Handle type="target" position={Position.Left}
                style={{ background: c.border, border: `2px solid ${c.glow}`, width: 7, height: 7 }} />
            <div className="flex items-center justify-center gap-1">
                <span className="w-1.5 h-1.5 rounded-full flex-shrink-0" style={{ backgroundColor: c.border }} />
                <span className="text-[10px] font-mono truncate" style={{ color: c.text }}>{n.name.split('-').slice(-2).join('-')}</span>
            </div>
            {!n.healthy && n.reason && (
                <div className="text-[8px] opacity-70 mt-0.5 truncate" style={{ color: c.text }}>{n.reason}</div>
            )}
        </div>
    )
}
function PolicyNode({ data }: { data: { node: NodeInfo } }) {
    const n = data.node
    return (
        <NodeBase c={C.policy} icon={<Shield className="w-3.5 h-3.5" />}
            title={n.name} subtitle={n.namespace} badge="POLICY" handles="right" />
    )
}

const nodeTypes = {
    internet: InternetNode,
    ingress:  IngressNode,
    service:  ServiceNode,
    pod:      PodNode,
    policy:   PolicyNode,
}

// ─── Dagre layout ─────────────────────────────────────────────────────────────
function layout(nodes: Node[], edges: Edge[], dir = 'LR'): { nodes: Node[]; edges: Edge[] } {
    const g = new dagre.graphlib.Graph()
    g.setDefaultEdgeLabel(() => ({}))
    g.setGraph({ rankdir: dir, nodesep: 50, ranksep: 120, edgesep: 20 })
    nodes.forEach(n => g.setNode(n.id, { width: 190, height: 60 }))
    edges.forEach(e => g.setEdge(e.source, e.target))
    dagre.layout(g)
    return {
        nodes: nodes.map(n => {
            const pos = g.node(n.id)
            return { ...n, position: { x: pos.x - 95, y: pos.y - 30 } }
        }),
        edges,
    }
}

// ─── Main canvas ──────────────────────────────────────────────────────────────
function NetworkCanvas({ graphData }: { graphData: { nodes: NodeInfo[]; edges: { from: string; to: string; rel: string }[] } }) {
    const [selectedNode, setSelectedNode] = useState<NodeInfo | null>(null)

    const { rfNodes, rfEdges } = useMemo(() => {
        const nodeByUID: Record<string, NodeInfo> = {}
        for (const n of graphData.nodes) nodeByUID[n.uid] = n

        const ingresses  = graphData.nodes.filter(n => n.kind === 'Ingress')
        const services   = graphData.nodes.filter(n => n.kind === 'Service')
        const policies   = graphData.nodes.filter(n => n.kind === 'NetworkPolicy')

        const rfNodes: Node[] = []
        const rfEdges: Edge[] = []

        const edgeStyle = (color: string) => ({
            stroke: color, strokeWidth: 2,
        })

        // Add services and pods reachable from services
        const reachableServiceUIDs = new Set<string>()
        const reachablePodUIDs = new Set<string>()

        for (const svc of services) {
            reachableServiceUIDs.add(svc.uid)
            for (const e of graphData.edges) {
                if (e.from === svc.uid && e.rel === 'selects') {
                    reachablePodUIDs.add(e.to)
                }
            }
        }

        // Internet node (only if there are ingresses)
        if (ingresses.length > 0) {
            rfNodes.push({ id: '__internet__', type: 'internet', position: { x: 0, y: 0 }, data: {} })
            for (const ing of ingresses) {
                rfEdges.push({
                    id: `internet-${ing.uid}`,
                    source: '__internet__', target: ing.uid,
                    animated: true,
                    style: edgeStyle(C.ingress.border),
                    markerEnd: { type: MarkerType.ArrowClosed, color: C.ingress.border },
                    label: 'HTTP/S',
                    labelStyle: { fill: C.ingress.text, fontSize: 9, fontFamily: 'monospace' },
                    labelBgStyle: { fill: '#0f172a', fillOpacity: 0.9 },
                })
            }
        }

        // Ingress nodes
        for (const n of ingresses) {
            rfNodes.push({ id: n.uid, type: 'ingress', position: { x: 0, y: 0 }, data: { node: n } })
        }

        // Ingress → Service edges
        for (const e of graphData.edges) {
            if (e.rel === 'routes to') {
                const svc = nodeByUID[e.to]
                if (!svc) continue
                rfEdges.push({
                    id: `${e.from}-${e.to}`,
                    source: e.from, target: e.to,
                    animated: true,
                    style: edgeStyle(C.service.border),
                    markerEnd: { type: MarkerType.ArrowClosed, color: C.service.border },
                    label: 'routes to',
                    labelStyle: { fill: C.service.text, fontSize: 9, fontFamily: 'monospace' },
                    labelBgStyle: { fill: '#0f172a', fillOpacity: 0.9 },
                })
            }
        }

        // Service nodes (all)
        for (const n of services) {
            rfNodes.push({ id: n.uid, type: 'service', position: { x: 0, y: 0 }, data: { node: n } })
        }

        // Service → Pod edges
        for (const e of graphData.edges) {
            if (e.rel === 'selects') {
                const pod = nodeByUID[e.to]
                if (!pod) continue
                if (!rfNodes.find(n => n.id === pod.uid)) {
                    rfNodes.push({ id: pod.uid, type: 'pod', position: { x: 0, y: 0 }, data: { node: pod } })
                }
                rfEdges.push({
                    id: `${e.from}-${e.to}`,
                    source: e.from, target: e.to,
                    animated: true,
                    style: edgeStyle(pod.healthy ? C.pod_ok.border : C.pod_bad.border),
                    markerEnd: { type: MarkerType.ArrowClosed, color: pod.healthy ? C.pod_ok.border : C.pod_bad.border },
                })
            }
        }

        // NetworkPolicy nodes + edges
        for (const pol of policies) {
            rfNodes.push({ id: pol.uid, type: 'policy', position: { x: 0, y: 0 }, data: { node: pol } })
            for (const e of graphData.edges) {
                if (e.from === pol.uid && e.rel === 'policy selects') {
                    rfEdges.push({
                        id: `pol-${pol.uid}-${e.to}`,
                        source: pol.uid, target: e.to,
                        animated: false,
                        style: { stroke: C.policy.border, strokeWidth: 1.5, strokeDasharray: '4 3' },
                        markerEnd: { type: MarkerType.Arrow, color: C.policy.border },
                    })
                }
            }
        }

        const laid = layout(rfNodes, rfEdges)
        return { rfNodes: laid.nodes, rfEdges: laid.edges }
    }, [graphData])

    const [nodes, , onNodesChange] = useNodesState(rfNodes)
    const [edges, , onEdgesChange] = useEdgesState(rfEdges)

    const onNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
        const ni = (node.data as { node?: NodeInfo }).node
        setSelectedNode(ni ? (selectedNode?.uid === ni.uid ? null : ni) : null)
    }, [selectedNode])

    return (
        <div className="relative w-full h-full">
            <ReactFlow
                nodes={nodes}
                edges={edges}
                onNodesChange={onNodesChange}
                onEdgesChange={onEdgesChange}
                onNodeClick={onNodeClick}
                nodeTypes={nodeTypes}
                fitView
                fitViewOptions={{ padding: 0.15 }}
                minZoom={0.2}
                maxZoom={3}
                style={{ background: '#030712' }}
                proOptions={{ hideAttribution: true }}
            >
                <Background variant={BackgroundVariant.Dots} gap={24} size={1} color="#1e293b" />
                <Controls style={{ background: '#0f172a', border: '1px solid #1e293b' }} />
                <MiniMap
                    nodeColor={(n) => {
                        if (n.type === 'internet') return C.internet.border
                        if (n.type === 'ingress')  return C.ingress.border
                        if (n.type === 'service')  return C.service.border
                        if (n.type === 'policy')   return C.policy.border
                        const ni = (n.data as { node?: NodeInfo }).node
                        return ni?.healthy ? C.pod_ok.border : C.pod_bad.border
                    }}
                    style={{ background: '#0f172a', border: '1px solid #1e293b' }}
                    maskColor="#03071288"
                />
            </ReactFlow>

            {/* Selected node info panel */}
            {selectedNode && (
                <div className="absolute bottom-4 left-4 z-10 rounded-xl border border-space-700 bg-space-900/95 backdrop-blur p-3 w-64 shadow-2xl">
                    <div className="flex items-center gap-2 mb-2">
                        <Info className="w-3.5 h-3.5 text-accent" />
                        <span className="text-xs font-semibold text-slate-200">{selectedNode.kind}</span>
                        <button onClick={() => setSelectedNode(null)} className="ml-auto text-slate-600 hover:text-slate-300 text-[10px]">✕</button>
                    </div>
                    <div className="space-y-1 text-[10px] font-mono">
                        <div><span className="text-slate-600">name: </span><span className="text-slate-300">{selectedNode.name}</span></div>
                        {selectedNode.namespace && <div><span className="text-slate-600">ns: </span><span className="text-slate-300">{selectedNode.namespace}</span></div>}
                        {selectedNode.fields && Object.entries(selectedNode.fields).map(([k, v]) =>
                            v ? <div key={k}><span className="text-slate-600">{k}: </span><span className="text-slate-400">{v}</span></div> : null
                        )}
                        {selectedNode.kind === 'Pod' && (
                            <div className={selectedNode.healthy ? 'text-emerald-400' : 'text-red-400'}>
                                {selectedNode.healthy ? '● running' : `✖ ${selectedNode.reason ?? 'unhealthy'}`}
                            </div>
                        )}
                    </div>
                </div>
            )}
        </div>
    )
}

// ─── Page wrapper ─────────────────────────────────────────────────────────────
export function NetworkTopology() {
    const { context, namespace } = useStore()

    const graphQuery = useQuery({
        queryKey: ['graph', context, namespace],
        queryFn: () => api.graph(context, namespace),
        enabled: !!context,
        staleTime: 30_000,
    })

    if (!context) return null
    if (graphQuery.isLoading) {
        return (
            <div className="flex-1 flex flex-col items-center justify-center gap-3">
                <Loader2 className="w-6 h-6 text-accent animate-spin" />
                <span className="text-xs text-slate-500">Building network topology…</span>
            </div>
        )
    }
    if (graphQuery.error) {
        return <div className="flex-1 flex items-center justify-center text-xs text-status-unhealthy">{(graphQuery.error as Error).message}</div>
    }

    const data = graphQuery.data!
    const ingresses = data.nodes.filter(n => n.kind === 'Ingress').length
    const services  = data.nodes.filter(n => n.kind === 'Service').length
    const policies  = data.nodes.filter(n => n.kind === 'NetworkPolicy').length
    const pods      = data.nodes.filter(n => n.kind === 'Pod').length
    const unhealthy = data.nodes.filter(n => n.kind === 'Pod' && !n.healthy).length

    return (
        <div className="flex-1 flex flex-col overflow-hidden">
            {/* Top status bar */}
            <div className="flex items-center gap-4 px-4 py-2.5 border-b border-space-700 bg-space-900 flex-shrink-0 flex-wrap">
                <div className="flex items-center gap-1.5">
                    <div className="w-2 h-2 rounded-full bg-sky-400 animate-pulse" />
                    <span className="text-[10px] font-semibold text-sky-400 uppercase tracking-widest">Network Topology</span>
                </div>
                <div className="h-3 w-px bg-space-700" />
                <StatChip label="Ingresses" value={ingresses} color="text-fuchsia-400" />
                <StatChip label="Services"  value={services}  color="text-blue-400" />
                <StatChip label="Pods"      value={pods}      color="text-emerald-400" />
                {unhealthy > 0 && <StatChip label="Unhealthy" value={unhealthy} color="text-red-400" pulsing />}
                {policies > 0  && <StatChip label="Policies"  value={policies}  color="text-amber-400" />}
                <div className="ml-auto flex items-center gap-2">
                    <span className="text-[9px] text-slate-600">Click a node for details · scroll to zoom</span>
                    <button onClick={() => graphQuery.refetch()} className="p-1.5 rounded border border-space-700 text-slate-600 hover:text-accent transition-colors">
                        <RefreshCw className={`w-3 h-3 ${graphQuery.isFetching ? 'animate-spin text-accent' : ''}`} />
                    </button>
                </div>
            </div>

            {/* Canvas */}
            <div className="flex-1 overflow-hidden">
                <ReactFlowProvider>
                    <NetworkCanvas graphData={data} />
                </ReactFlowProvider>
            </div>
        </div>
    )
}

function StatChip({ label, value, color, pulsing }: { label: string; value: number; color: string; pulsing?: boolean }) {
    return (
        <div className="flex items-center gap-1.5 text-[10px]">
            {pulsing && <span className={clsx('w-1.5 h-1.5 rounded-full animate-pulse', color.replace('text-', 'bg-'))} />}
            <span className={clsx('font-bold tabular-nums', color)}>{value}</span>
            <span className="text-slate-600">{label}</span>
        </div>
    )
}
