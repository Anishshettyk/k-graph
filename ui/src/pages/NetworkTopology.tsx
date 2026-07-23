import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Loader2, RefreshCw, Network, Shield, Globe, ArrowRight } from 'lucide-react'
import clsx from 'clsx'
import { api } from '../api/client'
import { useStore } from '../store/useStore'
import type { NodeInfo, EdgeInfo } from '../types/api'

// Network-relevant kinds for topology view
const KIND_COLOR: Record<string, string> = {
    Ingress: '#f472b6',       // pink
    Service: '#60a5fa',       // blue
    NetworkPolicy: '#f59e0b', // amber
    Pod: '#34d399',           // green
    Deployment: '#a78bfa',    // violet
}

const KIND_ICON: Record<string, React.ReactNode> = {
    Ingress:       <Globe className="w-3 h-3" />,
    Service:       <Network className="w-3 h-3" />,
    NetworkPolicy: <Shield className="w-3 h-3" />,
}

interface FlowNode { node: NodeInfo; children: FlowNode[] }
void (undefined as unknown as FlowNode)

export function NetworkTopology() {
    const { context, namespace } = useStore()
    const [selected, setSelected] = useState<string | null>(null)

    const graphQuery = useQuery({
        queryKey: ['graph', context, namespace],
        queryFn: () => api.graph(context, namespace),
        enabled: !!context,
        staleTime: 30_000,
    })

    const { ingresses, services, policies, edges, nodeByUID } = useMemo(() => {
        const d = graphQuery.data
        if (!d) return { ingresses: [], services: [], policies: [], edges: [], nodeByUID: {} }

        const nodeByUID: Record<string, NodeInfo> = {}
        for (const n of d.nodes) nodeByUID[n.uid] = n

        const ingresses = d.nodes.filter(n => n.kind === 'Ingress')
        const services  = d.nodes.filter(n => n.kind === 'Service')
        const policies  = d.nodes.filter(n => n.kind === 'NetworkPolicy')

        return { ingresses, services, policies, edges: d.edges, nodeByUID }
    }, [graphQuery.data])

    // For a given node, find all outgoing edges
    function getEdgesFrom(uid: string): EdgeInfo[] {
        return edges.filter(e => e.from === uid)
    }

    // Build traffic paths: Ingress → Service → Pods
    const paths = useMemo(() => {
        return ingresses.map(ing => {
            const svcEdges = getEdgesFrom(ing.uid).filter(e => e.rel === 'routes to')
            const svcNodes = svcEdges.map(e => nodeByUID[e.to]).filter(Boolean)
            return {
                ingress: ing,
                services: svcNodes.map(svc => {
                    const podEdges = getEdgesFrom(svc.uid).filter(e => e.rel === 'selects')
                    const pods = podEdges.map(e => nodeByUID[e.to]).filter(Boolean)
                    // Find NetworkPolicies governing these pods
                    const govPolicies: NodeInfo[] = []
                    for (const pod of pods) {
                        const policyEdges = edges.filter(e => e.to === pod.uid && e.rel === 'policy selects')
                        for (const pe of policyEdges) {
                            const pol = nodeByUID[pe.from]
                            if (pol && !govPolicies.find(p => p.uid === pol.uid)) {
                                govPolicies.push(pol)
                            }
                        }
                    }
                    return { svc, pods, policies: govPolicies }
                }),
            }
        })
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [ingresses, services, edges, nodeByUID])

    // Services not reached by any Ingress
    const orphanServices = useMemo(() => {
        const reachedUIDs = new Set(paths.flatMap(p => p.services.map(s => s.svc.uid)))
        return services.filter(s => !reachedUIDs.has(s.uid))
    }, [paths, services])

    if (!context) return null
    if (graphQuery.isLoading) return <div className="flex-1 flex items-center justify-center"><Loader2 className="w-5 h-5 text-accent animate-spin" /></div>
    if (graphQuery.error) return <div className="flex-1 flex items-center justify-center text-xs text-status-unhealthy">{(graphQuery.error as Error).message}</div>

    return (
        <div className="flex-1 overflow-y-auto p-6 space-y-6">
            {/* Header */}
            <div className="flex items-center gap-4">
                <div className="w-9 h-9 rounded-xl bg-blue-500/10 border border-blue-500/20 flex items-center justify-center flex-shrink-0">
                    <Network className="w-5 h-5 text-blue-400" />
                </div>
                <div>
                    <h1 className="text-base font-semibold text-slate-100">Network Topology</h1>
                    <p className="text-xs text-slate-600">
                        Traffic paths · {ingresses.length} ingress{ingresses.length !== 1 ? 'es' : ''} · {services.length} service{services.length !== 1 ? 's' : ''} · {policies.length} network polic{policies.length !== 1 ? 'ies' : 'y'}
                    </p>
                </div>
                <button onClick={() => graphQuery.refetch()} className="ml-auto p-1.5 rounded border border-space-700 text-slate-600 hover:text-accent transition-colors">
                    <RefreshCw className={`w-3.5 h-3.5 ${graphQuery.isFetching ? 'animate-spin text-accent' : ''}`} />
                </button>
            </div>

            {/* Legend */}
            <div className="flex gap-4 flex-wrap">
                {Object.entries(KIND_COLOR).map(([kind, color]) => (
                    <div key={kind} className="flex items-center gap-1.5 text-[10px] text-slate-500">
                        <div className="w-2.5 h-2.5 rounded-sm" style={{ backgroundColor: color }} />
                        {kind}
                    </div>
                ))}
                <div className="flex items-center gap-1.5 text-[10px] text-slate-500">
                    <Shield className="w-3 h-3 text-amber-400" /> NetworkPolicy governs traffic
                </div>
            </div>

            {/* Traffic paths from Ingresses */}
            {paths.length > 0 && (
                <section className="space-y-4">
                    <SectionHeader icon={<Globe className="w-3.5 h-3.5" />} label="Ingress Traffic Paths" color="text-pink-400" count={paths.length} />
                    {paths.map(path => (
                        <div key={path.ingress.uid} className="rounded-2xl border border-space-700 bg-space-900 overflow-hidden">
                            {/* Ingress header */}
                            <NodeChip node={path.ingress} selected={selected} onSelect={setSelected} />

                            {path.services.length === 0 && (
                                <div className="px-4 py-3 text-[10px] text-slate-600">↳ no Services configured</div>
                            )}

                            {path.services.map(({ svc, pods, policies: polList }) => (
                                <div key={svc.uid} className="border-t border-space-700">
                                    {/* Arrow + Service */}
                                    <div className="flex items-center gap-2 px-4 py-2">
                                        <ArrowRight className="w-3 h-3 text-slate-700 flex-shrink-0" />
                                        <NodeChip node={svc} selected={selected} onSelect={setSelected} inline />
                                        {svc.fields?.type && (
                                            <span className="text-[9px] font-mono text-slate-600 ml-1">{svc.fields.type}</span>
                                        )}
                                        {polList.length > 0 && (
                                            <div className="ml-auto flex gap-1">
                                                {polList.map(pol => (
                                                    <span key={pol.uid} title={`NetworkPolicy: ${pol.name}`}
                                                        className="flex items-center gap-1 text-[9px] px-1.5 py-0.5 rounded border border-amber-900/40 bg-amber-950/20 text-amber-400">
                                                        <Shield className="w-2.5 h-2.5" />{pol.name}
                                                    </span>
                                                ))}
                                            </div>
                                        )}
                                    </div>

                                    {/* Pods */}
                                    {pods.length > 0 && (
                                        <div className="px-8 pb-3 flex flex-wrap gap-1.5">
                                            {pods.map(pod => (
                                                <PodBadge key={pod.uid} pod={pod} selected={selected} onSelect={setSelected} />
                                            ))}
                                            {pods.length === 0 && (
                                                <span className="text-[10px] text-red-400">⚠ no matching pods</span>
                                            )}
                                        </div>
                                    )}
                                    {pods.length === 0 && (
                                        <div className="px-8 pb-3 text-[10px] text-red-400">⚠ no pods selected by this service</div>
                                    )}
                                </div>
                            ))}
                        </div>
                    ))}
                </section>
            )}

            {/* Services not reachable from any Ingress */}
            {orphanServices.length > 0 && (
                <section className="space-y-3">
                    <SectionHeader icon={<Network className="w-3.5 h-3.5" />} label="Internal Services (no Ingress)" color="text-blue-400" count={orphanServices.length} />
                    <div className="grid gap-2">
                        {orphanServices.map(svc => {
                            const podEdges = getEdgesFrom(svc.uid).filter(e => e.rel === 'selects')
                            const pods = podEdges.map(e => nodeByUID[e.to]).filter(Boolean)
                            return (
                                <div key={svc.uid} className="rounded-xl border border-space-700 bg-space-900 overflow-hidden">
                                    <NodeChip node={svc} selected={selected} onSelect={setSelected} />
                                    {pods.length > 0 && (
                                        <div className="px-8 pb-3 flex flex-wrap gap-1.5 border-t border-space-700 pt-2">
                                            {pods.map(pod => (
                                                <PodBadge key={pod.uid} pod={pod} selected={selected} onSelect={setSelected} />
                                            ))}
                                        </div>
                                    )}
                                    {pods.length === 0 && (
                                        <div className="px-4 pb-3 text-[10px] text-red-400">⚠ no pods selected</div>
                                    )}
                                </div>
                            )
                        })}
                    </div>
                </section>
            )}

            {/* Standalone NetworkPolicies */}
            {policies.length > 0 && (
                <section className="space-y-3">
                    <SectionHeader icon={<Shield className="w-3.5 h-3.5" />} label="NetworkPolicies" color="text-amber-400" count={policies.length} />
                    <div className="grid gap-2">
                        {policies.map(pol => {
                            const governed = edges.filter(e => e.from === pol.uid && e.rel === 'policy selects').map(e => nodeByUID[e.to]).filter(Boolean)
                            return (
                                <div key={pol.uid} className="rounded-xl border border-amber-900/30 bg-amber-950/5 p-3 space-y-2">
                                    <div className="flex items-center gap-2">
                                        <Shield className="w-3.5 h-3.5 text-amber-400 flex-shrink-0" />
                                        <span className="text-xs font-mono font-medium text-slate-200">{pol.name}</span>
                                        <span className="text-[10px] text-slate-600 font-mono ml-auto">{pol.namespace}</span>
                                    </div>
                                    {governed.length > 0 && (
                                        <div className="flex flex-wrap gap-1">
                                            {governed.map(pod => (
                                                <PodBadge key={pod.uid} pod={pod} selected={selected} onSelect={setSelected} />
                                            ))}
                                        </div>
                                    )}
                                    {governed.length === 0 && (
                                        <div className="text-[10px] text-slate-600">No pods currently match this policy's selector</div>
                                    )}
                                </div>
                            )
                        })}
                    </div>
                </section>
            )}

            {ingresses.length === 0 && services.length === 0 && policies.length === 0 && (
                <div className="flex flex-col items-center justify-center py-20 text-center">
                    <Network className="w-10 h-10 text-slate-700 mb-3" />
                    <div className="text-sm text-slate-500">No network resources found in scope</div>
                </div>
            )}
        </div>
    )
}

// ─── Shared sub-components ────────────────────────────────────────────────────

function SectionHeader({ icon, label, color, count }: { icon: React.ReactNode; label: string; color: string; count: number }) {
    return (
        <div className="flex items-center gap-2">
            <span className={clsx('flex items-center gap-1.5 text-xs font-semibold uppercase tracking-widest', color)}>
                {icon}{label}
            </span>
            <span className="text-[10px] text-slate-600">({count})</span>
            <div className="flex-1 h-px bg-space-700" />
        </div>
    )
}

function NodeChip({ node, selected, onSelect, inline }: {
    node: NodeInfo; selected: string | null; onSelect: (uid: string | null) => void; inline?: boolean
}) {
    const color = KIND_COLOR[node.kind] ?? '#94a3b8'
    const icon = KIND_ICON[node.kind]
    const isSelected = selected === node.uid
    return (
        <button
            onClick={() => onSelect(isSelected ? null : node.uid)}
            className={clsx(
                'flex items-center gap-2 text-left transition-colors',
                inline ? 'px-0 py-0' : 'px-4 py-3 w-full hover:bg-space-800/40',
                isSelected && 'bg-accent/5'
            )}
        >
            <div className="w-4 h-4 flex-shrink-0 flex items-center justify-center" style={{ color }}>
                {icon || <div className="w-2 h-2 rounded-full" style={{ backgroundColor: color }} />}
            </div>
            <span className="text-xs font-mono font-medium" style={{ color }}>{node.kind}</span>
            <span className="text-xs font-mono text-slate-300">{node.name}</span>
            <span className="text-[10px] text-slate-600 font-mono">{node.namespace}</span>
        </button>
    )
}

function PodBadge({ pod, selected, onSelect }: { pod: NodeInfo; selected: string | null; onSelect: (uid: string | null) => void }) {
    const isSelected = selected === pod.uid
    return (
        <button
            onClick={() => onSelect(isSelected ? null : pod.uid)}
            className={clsx(
                'flex items-center gap-1 text-[10px] font-mono px-2 py-1 rounded-full border transition-all',
                pod.healthy
                    ? isSelected ? 'bg-emerald-950/50 border-emerald-700 text-emerald-300' : 'bg-space-800 border-space-700 text-slate-400 hover:border-emerald-800 hover:text-emerald-400'
                    : isSelected ? 'bg-red-950/50 border-red-700 text-red-300' : 'bg-red-950/20 border-red-900/40 text-red-400'
            )}
        >
            <span className={clsx('w-1.5 h-1.5 rounded-full flex-shrink-0', pod.healthy ? 'bg-emerald-500' : 'bg-red-500 animate-pulse')} />
            {pod.name}
        </button>
    )
}
