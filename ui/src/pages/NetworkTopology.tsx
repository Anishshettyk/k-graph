import { useMemo, useCallback, useState, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
    ReactFlow, Controls, MiniMap, Background, BackgroundVariant,
    useNodesState, useEdgesState, ReactFlowProvider,
    getBezierPath,
    type Node, type Edge, type EdgeProps,
    Handle, Position, MarkerType, useReactFlow,
} from '@xyflow/react'
import dagre from 'dagre'
import { Loader2, RefreshCw, AlertTriangle, Globe, ArrowRight, Box, Layers, ShieldCheck } from 'lucide-react'
import '@xyflow/react/dist/style.css'
import { api } from '../api/client'
import { useStore } from '../store/useStore'
import type { NodeInfo } from '../types/api'

const MAX_PODS_PER_SERVICE = 10
const MAX_SERVICES = 60

// ─── Design tokens ────────────────────────────────────────────────────────────
// Each type gets one accent color. Cards are uniformly dark; the accent is used
// for the left border, icon, and glow — nowhere else.
const T = {
    internet: '#22d3ee', // cyan
    ingress:  '#a78bfa', // violet
    service:  '#60a5fa', // blue
    pod_ok:   '#34d399', // emerald
    pod_bad:  '#f87171', // rose
    policy:   '#fbbf24', // amber
}

// ─── Shared card styles ───────────────────────────────────────────────────────
const CARD_BASE = {
    background: '#111827',
    borderColor: '#1f2937',
    borderRadius: 10,
}

const handle = (color: string, w = 8, h = 8) => ({
    background: '#1f2937',
    border: `2px solid ${color}`,
    width: w, height: h,
    boxShadow: `0 0 5px ${color}60`,
})

// ─── Card node component ──────────────────────────────────────────────────────
// All nodes use the same card layout. Only the accent color and icon differ.
// Left border strip = type identifier. Clean, readable, professional.

interface CardProps {
    accent: string
    icon: React.ReactNode
    label: string
    sub?: string
    badge?: string
    badgeColor?: string
    healthDot?: 'ok' | 'bad' | 'none'
    width?: number
    leftHandle?: boolean
    rightHandle?: boolean
}

function Card({ accent, icon, label, sub, badge, badgeColor, healthDot = 'none', width = 200, leftHandle, rightHandle }: CardProps) {
    return (
        <div style={{
            width,
            minHeight: 56,
            ...CARD_BASE,
            border: '1px solid #1f2937',
            borderLeft: `4px solid ${accent}`,
            display: 'flex',
            alignItems: 'stretch',
            position: 'relative',
            boxShadow: `0 0 0 0 transparent, 2px 0 12px ${accent}18`,
        }}>
            {leftHandle && (
                <Handle type="target" position={Position.Left}
                    style={{ ...handle(accent), left: -5, top: '50%' }} />
            )}

            {/* Icon column */}
            <div style={{
                width: 36, flexShrink: 0,
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                color: accent,
            }}>
                {icon}
            </div>

            {/* Text column */}
            <div style={{ flex: 1, minWidth: 0, padding: '8px 10px 8px 0' }}>
                <div style={{
                    fontSize: 12, fontWeight: 600, color: '#f1f5f9',
                    whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
                    lineHeight: '1.3',
                }}>
                    {label}
                </div>
                {sub && (
                    <div style={{
                        fontSize: 10, color: '#64748b', fontFamily: 'monospace',
                        marginTop: 1, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
                    }}>
                        {sub}
                    </div>
                )}
                {badge && (
                    <div style={{
                        display: 'inline-flex', alignItems: 'center',
                        marginTop: 3, padding: '1px 6px',
                        background: `${badgeColor ?? accent}20`,
                        border: `1px solid ${badgeColor ?? accent}50`,
                        borderRadius: 4,
                        fontSize: 9, fontWeight: 700, letterSpacing: '0.06em',
                        color: badgeColor ?? accent, textTransform: 'uppercase',
                    }}>
                        {badge}
                    </div>
                )}
            </div>

            {/* Health dot — top right */}
            {healthDot !== 'none' && (
                <div style={{
                    position: 'absolute', top: 6, right: 8,
                    width: 8, height: 8, borderRadius: '50%',
                    background: healthDot === 'ok' ? T.pod_ok : T.pod_bad,
                    boxShadow: `0 0 6px ${healthDot === 'ok' ? T.pod_ok : T.pod_bad}`,
                    animation: healthDot === 'bad' ? 'topo-blink 1.2s ease-in-out infinite' : undefined,
                }} />
            )}

            {rightHandle && (
                <Handle type="source" position={Position.Right}
                    style={{ ...handle(accent), right: -5, top: '50%' }} />
            )}
        </div>
    )
}

// ─── Internet entry node ──────────────────────────────────────────────────────
function InternetNode() {
    return (
        <div style={{ position: 'relative' }}>
            <Handle type="source" position={Position.Right}
                style={{ ...handle(T.internet), right: -5, top: '50%' }} />
            <div style={{
                width: 160, padding: '10px 14px',
                background: '#0c1a20',
                border: `1px solid ${T.internet}50`,
                borderLeft: `4px solid ${T.internet}`,
                borderRadius: 10,
                boxShadow: `0 0 20px ${T.internet}20`,
            }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                    <Globe style={{ width: 18, height: 18, color: T.internet, flexShrink: 0 }} />
                    <div>
                        <div style={{ fontSize: 12, fontWeight: 700, color: T.internet, letterSpacing: '0.05em' }}>
                            INTERNET
                        </div>
                        <div style={{ fontSize: 10, color: '#94a3b8', marginTop: 1 }}>External traffic</div>
                    </div>
                </div>
            </div>
        </div>
    )
}

// ─── Ingress node ─────────────────────────────────────────────────────────────
function IngressNode({ data }: { data: { node: NodeInfo } }) {
    const n = data.node
    return (
        <Card accent={T.ingress}
            icon={<ArrowRight style={{ width: 16, height: 16 }} />}
            label={n.name}
            sub={n.namespace}
            badge="Ingress"
            leftHandle rightHandle
        />
    )
}

// ─── Service node ─────────────────────────────────────────────────────────────
function ServiceNode({ data }: { data: { node: NodeInfo } }) {
    const n = data.node
    const type = n.fields?.type ?? ''
    const accent = type === 'LoadBalancer' ? T.ingress : T.service
    return (
        <Card accent={accent}
            icon={<Layers style={{ width: 16, height: 16 }} />}
            label={n.name}
            sub={n.namespace}
            badge={type || 'Service'}
            badgeColor={accent}
            leftHandle rightHandle
        />
    )
}

// ─── Pod node ─────────────────────────────────────────────────────────────────
function PodNode({ data }: { data: { node: NodeInfo } }) {
    const n = data.node
    const accent = n.healthy ? T.pod_ok : T.pod_bad
    // Show a short readable pod name: last 2 dash-separated segments
    const parts = n.name.split('-')
    const shortName = parts.length > 3 ? parts.slice(-2).join('-') : n.name
    return (
        <Card accent={accent}
            icon={<Box style={{ width: 15, height: 15 }} />}
            label={shortName}
            sub={n.fields?.phase ?? n.namespace}
            healthDot={n.healthy ? 'ok' : 'bad'}
            width={160}
            leftHandle
        />
    )
}

// ─── NetworkPolicy node ───────────────────────────────────────────────────────
function PolicyNode({ data }: { data: { node: NodeInfo } }) {
    const n = data.node
    return (
        <Card accent={T.policy}
            icon={<ShieldCheck style={{ width: 16, height: 16 }} />}
            label={n.name}
            sub={n.namespace}
            badge="NetworkPolicy"
            badgeColor={T.policy}
            width={180}
            rightHandle
        />
    )
}

const nodeTypes = { internet: InternetNode, ingress: IngressNode, service: ServiceNode, pod: PodNode, policy: PolicyNode }

// ─── Custom edge: clean animated arrow ───────────────────────────────────────
function TopoEdge({ id, sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, style, data }: EdgeProps) {
    const [path] = getBezierPath({ sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition })
    const color = (style?.stroke as string) ?? '#64748b'
    const dashed = data?.dashed as boolean

    return (
        <>
            {/* Glow halo */}
            <path d={path} fill="none" stroke={color} strokeWidth={4} strokeOpacity={0.08} />
            {/* Main line */}
            <path id={id} d={path} fill="none" stroke={color} strokeWidth={1.5}
                strokeDasharray={dashed ? '6 4' : '10 5'}
                style={{
                    animation: 'topo-flow .7s linear infinite',
                    filter: `drop-shadow(0 0 2px ${color}80)`,
                }}
            />
        </>
    )
}

const edgeTypes = { topo: TopoEdge }

// ─── CSS ──────────────────────────────────────────────────────────────────────
const CSS = `
@keyframes topo-flow { to { stroke-dashoffset: -15; } }
@keyframes topo-blink { 0%,100%{opacity:1} 50%{opacity:.25} }
`

// ─── Dagre layout ─────────────────────────────────────────────────────────────
function layout(nodes: Node[], edges: Edge[]): { nodes: Node[]; edges: Edge[] } {
    const g = new dagre.graphlib.Graph()
    g.setDefaultEdgeLabel(() => ({}))
    g.setGraph({ rankdir: 'LR', nodesep: 40, ranksep: 120, edgesep: 20 })

    const sizes: Record<string, [number, number]> = {
        internet: [160, 56],
        ingress:  [200, 56],
        service:  [200, 56],
        pod:      [160, 56],
        policy:   [180, 56],
    }

    nodes.forEach(n => {
        const [w, h] = sizes[n.type ?? ''] ?? [200, 56]
        g.setNode(n.id, { width: w, height: h })
    })
    edges.forEach(e => g.setEdge(e.source, e.target))
    dagre.layout(g)

    return {
        nodes: nodes.map(n => {
            const p = g.node(n.id)
            return { ...n, position: { x: p.x - p.width / 2, y: p.y - p.height / 2 } }
        }),
        edges,
    }
}

// ─── Canvas ───────────────────────────────────────────────────────────────────
function NetworkCanvas({ graphData }: {
    graphData: { nodes: NodeInfo[]; edges: { from: string; to: string; rel: string }[] }
}) {
    const [selected, setSelected] = useState<NodeInfo | null>(null)

    const { rfNodes, rfEdges, svcTruncated, totalSvcs } = useMemo(() => {
        const byUID: Record<string, NodeInfo> = {}
        for (const n of graphData.nodes) byUID[n.uid] = n

        const ingresses = graphData.nodes.filter(n => n.kind === 'Ingress')
        const services  = graphData.nodes.filter(n => n.kind === 'Service')
        const policies  = graphData.nodes.filter(n => n.kind === 'NetworkPolicy')
        const svcTruncated = services.length > MAX_SERVICES
        const totalSvcs = services.length

        const rfNodes: Node[] = []
        const rfEdges: Edge[] = []

        const edge = (src: string, tgt: string, color: string, label?: string, dashed = false): Edge => ({
            id: `${src}-${tgt}`,
            source: src, target: tgt,
            type: 'topo',
            data: { dashed },
            style: { stroke: color },
            markerEnd: { type: MarkerType.ArrowClosed, color, width: 12, height: 12 },
            ...(label ? {
                label,
                labelStyle: { fill: color, fontSize: 9, fontFamily: 'monospace', fontWeight: 600 },
                labelBgStyle: { fill: '#111827', fillOpacity: 0.95 },
                labelBgPadding: [4, 3] as [number, number],
                labelBgBorderRadius: 3,
            } : {}),
        } as Edge)

        // Internet → Ingresses
        if (ingresses.length > 0) {
            rfNodes.push({ id: '__internet__', type: 'internet', position: { x: 0, y: 0 }, data: {} })
            for (const ing of ingresses) {
                rfNodes.push({ id: ing.uid, type: 'ingress', position: { x: 0, y: 0 }, data: { node: ing } })
                rfEdges.push(edge('__internet__', ing.uid, T.internet, 'HTTP/S'))
            }
        }

        // Capped services
        const capped = services.slice(0, MAX_SERVICES)
        const cappedUIDs = new Set(capped.map(s => s.uid))
        for (const svc of capped) {
            rfNodes.push({ id: svc.uid, type: 'service', position: { x: 0, y: 0 }, data: { node: svc } })
        }

        // Ingress → Service
        for (const e of graphData.edges) {
            if (e.rel === 'routes to' && cappedUIDs.has(e.to)) {
                rfEdges.push(edge(e.from, e.to, T.ingress, 'routes to'))
            }
        }

        // Service → Pod (capped)
        const podCnt: Record<string, number> = {}
        for (const e of graphData.edges) {
            if (e.rel !== 'selects' || !cappedUIDs.has(e.from)) continue
            podCnt[e.from] = (podCnt[e.from] ?? 0) + 1
            if (podCnt[e.from] > MAX_PODS_PER_SERVICE) continue
            const pod = byUID[e.to]
            if (!pod) continue
            if (!rfNodes.find(n => n.id === pod.uid)) {
                rfNodes.push({ id: pod.uid, type: 'pod', position: { x: 0, y: 0 }, data: { node: pod } })
            }
            rfEdges.push(edge(e.from, e.to, pod.healthy ? T.pod_ok : T.pod_bad))
        }

        // NetworkPolicy → Pods
        const visiblePods = new Set(rfNodes.filter(n => n.type === 'pod').map(n => n.id))
        for (const pol of policies) {
            const governed = graphData.edges.filter(e => e.from === pol.uid && e.rel === 'policy selects' && visiblePods.has(e.to))
            if (!governed.length) continue
            rfNodes.push({ id: pol.uid, type: 'policy', position: { x: 0, y: 0 }, data: { node: pol } })
            for (const e of governed) {
                rfEdges.push(edge(pol.uid, e.to, T.policy, undefined, true))
            }
        }

        const laid = layout(rfNodes, rfEdges)
        return { rfNodes: laid.nodes, rfEdges: laid.edges, svcTruncated, totalSvcs }
    }, [graphData])

    const [nodes, , onNodesChange] = useNodesState(rfNodes)
    const [edges, , onEdgesChange] = useEdgesState(rfEdges)
    const { fitView } = useReactFlow()
    useEffect(() => { setTimeout(() => fitView({ padding: .15 }), 50) }, [rfNodes.length, fitView])

    const onNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
        const ni = (node.data as { node?: NodeInfo }).node
        setSelected(ni ? (selected?.uid === ni.uid ? null : ni) : null)
    }, [selected])

    return (
        <div className="relative w-full h-full">
            <style>{CSS}</style>
            <ReactFlow
                nodes={nodes} edges={edges}
                onNodesChange={onNodesChange} onEdgesChange={onEdgesChange}
                onNodeClick={onNodeClick}
                nodeTypes={nodeTypes} edgeTypes={edgeTypes}
                fitView fitViewOptions={{ padding: .15 }}
                minZoom={.1} maxZoom={4}
                style={{ background: '#0b1120' }}
                proOptions={{ hideAttribution: true }}
            >
                <Background variant={BackgroundVariant.Dots} gap={28} size={1} color="#1e293b" />
                <Controls style={{ background: '#111827', border: '1px solid #1f2937', borderRadius: 8 }} />
                <MiniMap
                    nodeColor={n => {
                        if (n.type === 'internet') return T.internet
                        if (n.type === 'ingress')  return T.ingress
                        if (n.type === 'service')  return T.service
                        if (n.type === 'policy')   return T.policy
                        return (n.data as { node?: NodeInfo }).node?.healthy ? T.pod_ok : T.pod_bad
                    }}
                    style={{ background: '#111827', border: '1px solid #1f2937', borderRadius: 8 }}
                    maskColor="#0b112088"
                />
            </ReactFlow>

            {svcTruncated && (
                <div className="absolute top-3 left-1/2 -translate-x-1/2 z-10 flex items-center gap-2 px-3 py-1.5 rounded-full bg-amber-950/90 backdrop-blur border border-amber-800/60 text-amber-400 text-[10px]">
                    <AlertTriangle className="w-3 h-3" />
                    Showing {MAX_SERVICES} of {totalSvcs} services — use namespace filter
                </div>
            )}

            {/* Details panel on click */}
            {selected && (
                <div className="absolute bottom-4 left-4 z-10 rounded-xl bg-[#111827] border border-[#1f2937] p-4 w-64 shadow-2xl">
                    <div className="flex items-center justify-between mb-3">
                        <span className="text-[10px] font-bold uppercase tracking-widest text-slate-500">{selected.kind}</span>
                        <button onClick={() => setSelected(null)} className="text-slate-600 hover:text-slate-400 text-xs">✕</button>
                    </div>
                    <div className="space-y-1.5 text-[11px]">
                        <div className="font-semibold text-slate-200 truncate">{selected.name}</div>
                        {selected.namespace && <div className="font-mono text-slate-500">{selected.namespace}</div>}
                        {selected.fields && Object.entries(selected.fields).map(([k, v]) =>
                            v ? (
                                <div key={k} className="flex gap-2">
                                    <span className="text-slate-600 flex-shrink-0">{k}:</span>
                                    <span className="text-slate-400 font-mono truncate">{v}</span>
                                </div>
                            ) : null
                        )}
                        {selected.kind === 'Pod' && (
                            <div className={`font-medium mt-1 ${selected.healthy ? 'text-emerald-400' : 'text-red-400'}`}>
                                {selected.healthy ? '● Running' : `✖ ${selected.reason ?? 'Unhealthy'}`}
                            </div>
                        )}
                    </div>
                </div>
            )}
        </div>
    )
}

// ─── Page ─────────────────────────────────────────────────────────────────────
export function NetworkTopology() {
    const { context, namespace } = useStore()

    const q = useQuery({
        queryKey: ['graph', context, namespace],
        queryFn: () => api.graph(context, namespace),
        enabled: !!context,
        staleTime: 30_000,
    })

    if (!context) return null
    if (q.isLoading) {
        return (
            <div className="flex-1 flex items-center justify-center gap-3">
                <Loader2 className="w-5 h-5 animate-spin text-blue-400" />
                <span className="text-sm text-slate-500">Building network topology…</span>
            </div>
        )
    }
    if (q.error) return <div className="flex-1 flex items-center justify-center text-xs text-status-unhealthy">{(q.error as Error).message}</div>

    const data = q.data!
    const counts = {
        ingress:  data.nodes.filter(n => n.kind === 'Ingress').length,
        service:  data.nodes.filter(n => n.kind === 'Service').length,
        pod:      data.nodes.filter(n => n.kind === 'Pod').length,
        policy:   data.nodes.filter(n => n.kind === 'NetworkPolicy').length,
        unhealthy: data.nodes.filter(n => n.kind === 'Pod' && !n.healthy).length,
    }

    return (
        <div className="flex-1 flex flex-col overflow-hidden">
            {/* Status bar */}
            <div className="flex items-center gap-4 px-5 py-2.5 border-b border-[#1f2937] bg-[#0f1725] flex-shrink-0 flex-wrap">
                <span className="text-[11px] font-semibold text-slate-400 uppercase tracking-widest">Network Topology</span>
                <div className="h-3 w-px bg-slate-800" />
                {[
                    { label: 'Ingress', val: counts.ingress,  color: T.ingress },
                    { label: 'Service', val: counts.service,  color: T.service },
                    { label: 'Pod',     val: counts.pod,      color: T.pod_ok  },
                    ...(counts.policy > 0 ? [{ label: 'Policy', val: counts.policy, color: T.policy }] : []),
                ].map(({ label, val, color }) => (
                    <div key={label} className="flex items-center gap-1.5 text-[11px]">
                        <span className="font-bold" style={{ color }}>{val}</span>
                        <span className="text-slate-600">{label}{val !== 1 ? 's' : ''}</span>
                    </div>
                ))}
                {counts.unhealthy > 0 && (
                    <div className="flex items-center gap-1.5 text-[11px]">
                        <span className="w-2 h-2 rounded-full" style={{ background: T.pod_bad }} />
                        <span className="font-bold" style={{ color: T.pod_bad }}>{counts.unhealthy} unhealthy</span>
                    </div>
                )}
                <div className="ml-auto flex items-center gap-2">
                    <span className="text-[10px] text-slate-600">Click a node for details</span>
                    <button onClick={() => q.refetch()} className="p-1.5 rounded border border-slate-800 text-slate-600 hover:text-blue-400 transition-colors">
                        <RefreshCw className={`w-3.5 h-3.5 ${q.isFetching ? 'animate-spin text-blue-400' : ''}`} />
                    </button>
                </div>
            </div>

            <div className="flex-1 overflow-hidden">
                <ReactFlowProvider>
                    <NetworkCanvas graphData={data} />
                </ReactFlowProvider>
            </div>
        </div>
    )
}
