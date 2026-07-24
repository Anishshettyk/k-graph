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
import { Loader2, Globe, Network, Shield, RefreshCw, AlertTriangle } from 'lucide-react'
import '@xyflow/react/dist/style.css'
import { api } from '../api/client'
import { useStore } from '../store/useStore'
import type { NodeInfo } from '../types/api'

const MAX_PODS_PER_SERVICE = 8
const MAX_SERVICES = 60

// ─── Colour system ────────────────────────────────────────────────────────────
const C = {
    internet:  { main: '#06b6d4', dim: '#0e7490', bg: '#020d12' },
    ingress:   { main: '#a855f7', dim: '#7e22ce', bg: '#0d0514' },
    service:   { main: '#3b82f6', dim: '#1d4ed8', bg: '#030b1a' },
    pod_ok:    { main: '#10b981', dim: '#047857', bg: '#021108' },
    pod_bad:   { main: '#f43f5e', dim: '#be123c', bg: '#130209' },
    policy:    { main: '#f59e0b', dim: '#b45309', bg: '#110a00' },
}

// ─── CSS animations injected once ─────────────────────────────────────────────
const STYLE = `
@keyframes spin-slow { to { transform: rotate(360deg); } }
@keyframes pulse-ring { 0%,100% { opacity:.6; transform:scale(1); } 50% { opacity:1; transform:scale(1.12); } }
@keyframes blink-bad  { 0%,100% { opacity:1; } 50% { opacity:.3; } }
@keyframes flow-dash  { to { stroke-dashoffset: -24; } }
@keyframes packet     { 0% { offset-distance:0% } 100% { offset-distance:100% } }
@keyframes ping-anim  { 0% { transform:scale(.5);opacity:1 } 100% { transform:scale(2);opacity:0 } }
`

function StyleInjector() {
    useEffect(() => {
        const el = document.createElement('style')
        el.textContent = STYLE
        document.head.appendChild(el)
        return () => el.remove()
    }, [])
    return null
}

// ─── Handle style helper ──────────────────────────────────────────────────────
const handle = (color: string) => ({
    background: color, border: `2px solid ${color}88`, width: 8, height: 8,
    boxShadow: `0 0 6px ${color}`,
})

// ─── Internet Node ────────────────────────────────────────────────────────────
function InternetNode() {
    return (
        <div className="relative select-none" style={{ width: 100, height: 100 }}>
            <Handle type="source" position={Position.Right}
                style={{ ...handle(C.internet.main), top: '50%', right: -4 }} />
            {/* Outer orbit ring */}
            <div className="absolute inset-0 rounded-full border border-dashed"
                style={{ borderColor: `${C.internet.main}40`, animation: 'spin-slow 8s linear infinite' }} />
            <div className="absolute inset-2 rounded-full border border-dashed"
                style={{ borderColor: `${C.internet.main}25`, animation: 'spin-slow 5s linear infinite reverse' }} />
            {/* Core */}
            <div className="absolute inset-4 rounded-full flex flex-col items-center justify-center"
                style={{
                    background: `radial-gradient(circle, ${C.internet.bg} 0%, ${C.internet.main}22 100%)`,
                    border: `1.5px solid ${C.internet.main}88`,
                    boxShadow: `0 0 20px ${C.internet.main}44, inset 0 0 12px ${C.internet.main}11`,
                }}>
                <Globe className="w-5 h-5 mb-0.5" style={{ color: C.internet.main }} />
                <span className="text-[9px] font-bold tracking-widest uppercase" style={{ color: C.internet.main }}>INTERNET</span>
            </div>
            {/* Ping rings */}
            <div className="absolute inset-0 rounded-full"
                style={{ border: `1.5px solid ${C.internet.main}`, animation: 'ping-anim 2s ease-out infinite', opacity: 0 }} />
        </div>
    )
}

// ─── Ingress Node ─────────────────────────────────────────────────────────────
function IngressNode({ data }: { data: { node: NodeInfo } }) {
    const n = data.node
    const c = C.ingress
    // Parallelogram via clip-path
    return (
        <div className="relative select-none" style={{ width: 180, height: 70 }}>
            <Handle type="target" position={Position.Left} style={{ ...handle(c.main), top: '50%', left: -4 }} />
            <Handle type="source" position={Position.Right} style={{ ...handle(c.main), top: '50%', right: -4 }} />
            <div className="absolute inset-0 flex flex-col justify-center px-4"
                style={{
                    clipPath: 'polygon(8% 0%, 100% 0%, 92% 100%, 0% 100%)',
                    background: `linear-gradient(135deg, ${c.bg} 0%, ${c.main}18 100%)`,
                    border: `1px solid ${c.main}66`,
                    boxShadow: `0 0 18px ${c.main}33, inset 0 0 20px ${c.main}11`,
                }}>
                <div className="flex items-center gap-2">
                    <div className="w-5 h-5 flex-shrink-0 flex items-center justify-center rounded"
                        style={{ background: `${c.main}22`, border: `1px solid ${c.main}44` }}>
                        <Globe className="w-3 h-3" style={{ color: c.main }} />
                    </div>
                    <div className="min-w-0">
                        <div className="text-[10px] font-bold tracking-widest" style={{ color: `${c.main}88` }}>INGRESS</div>
                        <div className="text-xs font-semibold truncate" style={{ color: c.main }}>{n.name}</div>
                    </div>
                </div>
                {n.namespace && (
                    <div className="text-[8px] font-mono mt-0.5 ml-7" style={{ color: `${c.main}66` }}>{n.namespace}</div>
                )}
            </div>
        </div>
    )
}

// ─── Service Node — hexagonal ─────────────────────────────────────────────────
function ServiceNode({ data }: { data: { node: NodeInfo } }) {
    const n = data.node
    const c = C.service
    const type = n.fields?.type ?? ''
    return (
        <div className="relative select-none" style={{ width: 160, height: 80 }}>
            <Handle type="target" position={Position.Left} style={{ ...handle(c.main), top: '50%', left: -4 }} />
            <Handle type="source" position={Position.Right} style={{ ...handle(c.main), top: '50%', right: -4 }} />
            {/* Hex-ish shape */}
            <div className="absolute inset-0 flex flex-col items-center justify-center"
                style={{
                    clipPath: 'polygon(10% 0%,90% 0%,100% 50%,90% 100%,10% 100%,0% 50%)',
                    background: `linear-gradient(135deg, ${c.bg} 0%, ${c.main}1a 100%)`,
                    border: `1px solid ${c.main}55`,
                    boxShadow: `0 0 16px ${c.main}33, inset 0 0 16px ${c.main}0d`,
                }}>
                <div className="flex items-center gap-1.5">
                    <Network className="w-3.5 h-3.5 flex-shrink-0" style={{ color: c.main }} />
                    <span className="text-xs font-semibold truncate max-w-[90px]" style={{ color: c.main }}>{n.name}</span>
                </div>
                <div className="flex items-center gap-1.5 mt-0.5">
                    {type && (
                        <span className="text-[8px] font-mono px-1 rounded"
                            style={{ background: `${c.main}22`, color: `${c.main}bb`, border: `1px solid ${c.main}33` }}>
                            {type}
                        </span>
                    )}
                    <span className="text-[8px] font-mono" style={{ color: `${c.main}55` }}>{n.namespace}</span>
                </div>
            </div>
        </div>
    )
}

// ─── Pod Node — circular with health ring ─────────────────────────────────────
function PodNode({ data }: { data: { node: NodeInfo } }) {
    const n = data.node
    const c = n.healthy ? C.pod_ok : C.pod_bad
    const shortName = n.name.split('-').slice(-2).join('-')
    return (
        <div className="relative select-none" style={{ width: 80, height: 80 }}>
            <Handle type="target" position={Position.Left} style={{ ...handle(c.main), top: '50%', left: -4 }} />
            {/* Outer health ring */}
            <div className="absolute inset-0 rounded-full"
                style={{
                    border: `2px solid ${c.main}`,
                    animation: n.healthy ? 'pulse-ring 3s ease-in-out infinite' : 'blink-bad 1s ease-in-out infinite',
                    boxShadow: `0 0 12px ${c.main}66`,
                }} />
            {/* Inner fill */}
            <div className="absolute inset-1.5 rounded-full flex flex-col items-center justify-center"
                style={{
                    background: `radial-gradient(circle, ${c.bg} 40%, ${c.main}18 100%)`,
                    border: `1px solid ${c.main}44`,
                }}>
                <div className="w-2 h-2 rounded-full mb-0.5 flex-shrink-0"
                    style={{ background: c.main, boxShadow: `0 0 6px ${c.main}` }} />
                <span className="text-[8px] font-mono text-center leading-tight px-1"
                    style={{ color: c.main, maxWidth: 64, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {shortName}
                </span>
            </div>
        </div>
    )
}

// ─── Policy Node ──────────────────────────────────────────────────────────────
function PolicyNode({ data }: { data: { node: NodeInfo } }) {
    const n = data.node
    const c = C.policy
    return (
        <div className="relative select-none" style={{ width: 140, height: 70 }}>
            <Handle type="source" position={Position.Right} style={{ ...handle(c.main), top: '50%', right: -4 }} />
            <div className="absolute inset-0 flex flex-col items-center justify-center rounded-xl"
                style={{
                    background: `linear-gradient(135deg, ${c.bg} 0%, ${c.main}18 100%)`,
                    border: `1px solid ${c.main}55`,
                    clipPath: 'polygon(5% 0%, 95% 0%, 100% 30%, 100% 70%, 95% 100%, 5% 100%, 0% 70%, 0% 30%)',
                    boxShadow: `0 0 14px ${c.main}33`,
                }}>
                <Shield className="w-4 h-4 mb-0.5" style={{ color: c.main }} />
                <span className="text-[9px] font-semibold truncate max-w-[110px]" style={{ color: c.main }}>{n.name}</span>
                <span className="text-[8px] font-mono" style={{ color: `${c.main}55` }}>{n.namespace}</span>
            </div>
        </div>
    )
}

const nodeTypes = {
    internet: InternetNode,
    ingress:  IngressNode,
    service:  ServiceNode,
    pod:      PodNode,
    policy:   PolicyNode,
}

// ─── Custom animated edge ─────────────────────────────────────────────────────
function FlowEdge({
    id, sourceX, sourceY, targetX, targetY,
    sourcePosition, targetPosition,
    data, markerEnd, style,
}: EdgeProps) {
    const [edgePath] = getBezierPath({ sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition })
    const color = (style?.stroke as string) ?? '#3b82f6'
    const isDashed = data?.dashed as boolean

    return (
        <>
            {/* Base edge */}
            <path id={id} d={edgePath} fill="none"
                stroke={`${color}33`} strokeWidth={3}
                style={{ filter: `drop-shadow(0 0 4px ${color}44)` }} />
            {/* Animated overlay */}
            <path d={edgePath} fill="none"
                stroke={color} strokeWidth={1.5}
                strokeDasharray={isDashed ? '5 4' : '8 6'}
                style={{
                    animation: 'flow-dash .8s linear infinite',
                    filter: `drop-shadow(0 0 3px ${color})`,
                    opacity: .9,
                }}
            />
            {/* Arrowhead */}
            {markerEnd && <marker id={`arrow-${id}`} />}
        </>
    )
}

const edgeTypes = { flow: FlowEdge }

// ─── Layout ───────────────────────────────────────────────────────────────────
function layout(nodes: Node[], edges: Edge[]): { nodes: Node[]; edges: Edge[] } {
    const g = new dagre.graphlib.Graph()
    g.setDefaultEdgeLabel(() => ({}))
    g.setGraph({ rankdir: 'LR', nodesep: 70, ranksep: 150, edgesep: 30 })

    const dim: Record<string, { w: number; h: number }> = {
        internet: { w: 100, h: 100 },
        ingress:  { w: 180, h: 70 },
        service:  { w: 160, h: 80 },
        pod:      { w: 80,  h: 80 },
        policy:   { w: 140, h: 70 },
    }
    nodes.forEach(n => {
        const d = dim[n.type ?? ''] ?? { w: 160, h: 70 }
        g.setNode(n.id, { width: d.w, height: d.h })
    })
    edges.forEach(e => g.setEdge(e.source, e.target))
    dagre.layout(g)

    return {
        nodes: nodes.map(n => {
            const { x, y, width, height } = g.node(n.id)
            return { ...n, position: { x: x - width / 2, y: y - height / 2 } }
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

        const fe = (color: string, dashed = false) => ({
            type: 'flow', animated: false, data: { dashed },
            style: { stroke: color },
            markerEnd: { type: MarkerType.ArrowClosed, color, width: 14, height: 14 },
        })

        // Internet
        if (ingresses.length > 0) {
            rfNodes.push({ id: '__internet__', type: 'internet', position: { x: 0, y: 0 }, data: {} })
            for (const ing of ingresses) {
                rfEdges.push({ id: `net-${ing.uid}`, source: '__internet__', target: ing.uid,
                    ...fe(C.internet.main), label: 'HTTP/S',
                    labelStyle: { fill: C.internet.main, fontSize: 9, fontFamily: 'monospace' },
                    labelBgStyle: { fill: '#020d12', fillOpacity: .9 },
                } as Edge)
            }
        }

        for (const n of ingresses) {
            rfNodes.push({ id: n.uid, type: 'ingress', position: { x: 0, y: 0 }, data: { node: n } })
        }

        // Capped services
        const capped = services.slice(0, MAX_SERVICES)
        const cappedUIDs = new Set(capped.map(s => s.uid))
        for (const n of capped) {
            rfNodes.push({ id: n.uid, type: 'service', position: { x: 0, y: 0 }, data: { node: n } })
        }

        for (const e of graphData.edges) {
            if (e.rel === 'routes to' && cappedUIDs.has(e.to)) {
                rfEdges.push({ id: `rt-${e.from}-${e.to}`, source: e.from, target: e.to,
                    ...fe(C.ingress.main), label: 'routes',
                    labelStyle: { fill: C.ingress.main, fontSize: 8, fontFamily: 'monospace' },
                    labelBgStyle: { fill: '#0d0514', fillOpacity: .9 },
                } as Edge)
            }
        }

        // Pods (capped per service)
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
            const c = pod.healthy ? C.pod_ok.main : C.pod_bad.main
            rfEdges.push({ id: `sel-${e.from}-${e.to}`, source: e.from, target: e.to, ...fe(c) } as Edge)
        }

        // NetworkPolicy
        const visiblePods = new Set(rfNodes.filter(n => n.type === 'pod').map(n => n.id))
        for (const pol of policies) {
            const gov = graphData.edges.filter(e => e.from === pol.uid && e.rel === 'policy selects' && visiblePods.has(e.to))
            if (!gov.length) continue
            rfNodes.push({ id: pol.uid, type: 'policy', position: { x: 0, y: 0 }, data: { node: pol } })
            for (const e of gov) {
                rfEdges.push({ id: `pol-${pol.uid}-${e.to}`, source: pol.uid, target: e.to,
                    ...fe(C.policy.main, true),
                } as Edge)
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
            <StyleInjector />
            <ReactFlow
                nodes={nodes} edges={edges}
                onNodesChange={onNodesChange} onEdgesChange={onEdgesChange}
                onNodeClick={onNodeClick}
                nodeTypes={nodeTypes} edgeTypes={edgeTypes}
                fitView fitViewOptions={{ padding: .15 }}
                minZoom={.1} maxZoom={4}
                style={{ background: '#020408' }}
                proOptions={{ hideAttribution: true }}
            >
                {/* Circuit-board grid */}
                <Background variant={BackgroundVariant.Lines} gap={40} size={0.5} color="#0f2535" />
                <Background variant={BackgroundVariant.Dots} gap={40} size={1.5} color="#0a1a28" />
                <Controls style={{ background: '#030d14', border: '1px solid #0f2535', borderRadius: 8 }} />
                <MiniMap
                    nodeColor={n => {
                        if (n.type === 'internet') return C.internet.main
                        if (n.type === 'ingress')  return C.ingress.main
                        if (n.type === 'service')  return C.service.main
                        if (n.type === 'policy')   return C.policy.main
                        const ni = (n.data as { node?: NodeInfo }).node
                        return ni?.healthy ? C.pod_ok.main : C.pod_bad.main
                    }}
                    style={{ background: '#030d14', border: '1px solid #0f2535', borderRadius: 8 }}
                    maskColor="#0208108a"
                />
            </ReactFlow>

            {/* Truncation warning */}
            {svcTruncated && (
                <div className="absolute top-3 left-1/2 -translate-x-1/2 z-10 flex items-center gap-2 px-3 py-1.5 rounded-full bg-amber-950/90 backdrop-blur border border-amber-800/60 text-amber-400 text-[10px]">
                    <AlertTriangle className="w-3 h-3" />
                    Showing {MAX_SERVICES} of {totalSvcs} services — use namespace filter
                </div>
            )}

            {/* Selected node info panel */}
            {selected && (
                <div className="absolute bottom-4 left-4 z-10 rounded-xl border border-cyan-900/50 bg-[#020d12]/95 backdrop-blur p-3 w-64 shadow-2xl"
                    style={{ boxShadow: `0 0 24px ${C.internet.main}22` }}>
                    <div className="flex items-center gap-2 mb-2">
                        <div className="w-2 h-2 rounded-full animate-pulse" style={{ background: C.internet.main }} />
                        <span className="text-[10px] font-bold uppercase tracking-widest text-slate-400">{selected.kind}</span>
                        <button onClick={() => setSelected(null)} className="ml-auto text-slate-600 hover:text-slate-400 text-xs">✕</button>
                    </div>
                    <div className="space-y-1 text-[11px] font-mono">
                        <div><span className="text-slate-600">name: </span><span className="text-slate-300">{selected.name}</span></div>
                        {selected.namespace && <div><span className="text-slate-600">ns: </span><span className="text-slate-300">{selected.namespace}</span></div>}
                        {selected.fields && Object.entries(selected.fields).map(([k, v]) =>
                            v ? <div key={k}><span className="text-slate-600">{k}: </span><span className="text-slate-400">{v}</span></div> : null
                        )}
                        {selected.kind === 'Pod' && (
                            <div className={selected.healthy ? 'text-emerald-400' : 'text-red-400 animate-pulse'}>
                                {selected.healthy ? '● running' : `✖ ${selected.reason ?? 'unhealthy'}`}
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
                <div className="relative">
                    <Loader2 className="w-8 h-8 animate-spin" style={{ color: C.internet.main }} />
                    <div className="absolute inset-0 rounded-full animate-ping"
                        style={{ border: `2px solid ${C.internet.main}44` }} />
                </div>
                <span className="text-xs font-mono" style={{ color: C.internet.main }}>Building network topology…</span>
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
        <div className="flex-1 flex flex-col overflow-hidden" style={{ background: '#020408' }}>
            {/* Status bar */}
            <div className="flex items-center gap-4 px-4 py-2.5 border-b border-space-700 bg-[#030d14]/80 backdrop-blur flex-shrink-0 flex-wrap">
                <div className="flex items-center gap-2">
                    <div className="w-2 h-2 rounded-full animate-pulse" style={{ background: C.internet.main }} />
                    <span className="text-[10px] font-bold uppercase tracking-widest" style={{ color: C.internet.main }}>Network Topology</span>
                </div>
                <div className="h-3 w-px bg-space-700" />
                {[
                    { label: 'Ingresses', val: ingresses, color: C.ingress.main },
                    { label: 'Services',  val: services,  color: C.service.main },
                    { label: 'Pods',      val: pods,      color: C.pod_ok.main },
                    ...(policies > 0 ? [{ label: 'Policies', val: policies, color: C.policy.main }] : []),
                ].map(({ label, val, color }) => (
                    <div key={label} className="flex items-center gap-1.5 text-[10px]">
                        <span className="font-bold tabular-nums" style={{ color }}>{val}</span>
                        <span className="text-slate-600">{label}</span>
                    </div>
                ))}
                {unhealthy > 0 && (
                    <div className="flex items-center gap-1.5 text-[10px]">
                        <span className="w-1.5 h-1.5 rounded-full animate-pulse" style={{ background: C.pod_bad.main }} />
                        <span className="font-bold tabular-nums" style={{ color: C.pod_bad.main }}>{unhealthy}</span>
                        <span className="text-slate-600">Unhealthy</span>
                    </div>
                )}
                <div className="ml-auto flex items-center gap-2">
                    <span className="text-[9px] text-slate-600">Click a node for details</span>
                    <button onClick={() => graphQuery.refetch()} className="p-1.5 rounded border border-space-700 text-slate-600 hover:text-cyan-400 transition-colors">
                        <RefreshCw className={`w-3 h-3 ${graphQuery.isFetching ? 'animate-spin text-cyan-400' : ''}`} />
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
