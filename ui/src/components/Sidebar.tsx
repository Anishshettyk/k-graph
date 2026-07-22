import { useState, useMemo } from 'react'
import { Search, ChevronRight, ChevronDown, Package, Network, Settings2, Database, Shield, Server, Key, Users, Lock } from 'lucide-react'
import clsx from 'clsx'
import { kindIcon, kindColor } from '../lib/kinds'
import type { NodeInfo, EdgeInfo } from '../types/api'

interface Props {
    nodes: NodeInfo[]
    edges: EdgeInfo[]
    selectedUID: string | null
    filterKind: string
    onSelect: (node: NodeInfo) => void
    onFilterKind: (kind: string) => void
}

// ─── Flat categories (non-workload, non-security) ────────────────────────────
const FLAT_CATEGORIES = [
    { id: 'network',        label: 'Network',        icon: Network,   kinds: ['Service', 'Ingress', 'Endpoints'] },
    { id: 'infrastructure', label: 'Infrastructure', icon: Server,    kinds: ['Node', 'Namespace'] },
]

// ─── Security sub-groups ─────────────────────────────────────────────────────
const SECURITY_SUBGROUPS = [
    { id: 'rbac',     label: 'RBAC',              icon: Key,      kinds: ['Role', 'ClusterRole', 'RoleBinding', 'ClusterRoleBinding'] },
    { id: 'identity', label: 'Identities',        icon: Users,    kinds: ['ServiceAccount'] },
    { id: 'netpol',   label: 'Network Policies',  icon: Shield,   kinds: ['NetworkPolicy'] },
]

// ─── Config & Storage sub-groups ─────────────────────────────────────────────
const CONFIG_SUBGROUPS = [
    { id: 'configmaps', label: 'ConfigMaps', icon: Settings2, kinds: ['ConfigMap'] },
    { id: 'secrets',    label: 'Secrets',    icon: Lock,      kinds: ['Secret'] },
    { id: 'storage',    label: 'Storage',    icon: Database,  kinds: ['PersistentVolumeClaim', 'PersistentVolume'] },
]

// Kinds that live in the workloads tree
const WORKLOAD_KINDS = new Set(['Deployment', 'StatefulSet', 'DaemonSet', 'CronJob', 'Job', 'ReplicaSet', 'Pod'])
// Top-level workload kinds (roots of the tree)
const TOP_WORKLOAD_KINDS = new Set(['Deployment', 'StatefulSet', 'DaemonSet', 'CronJob'])

const DEFAULT_EXPANDED = new Set(['workloads', 'network', 'security', 'config'])
const DEFAULT_EXPANDED_SUBS = new Set(['rbac', 'identity', 'netpol', 'configmaps', 'secrets', 'storage'])

function sortNodes(items: NodeInfo[]): NodeInfo[] {
    return [...items].sort((a, b) => {
        const aU = a.kind === 'Pod' && !a.healthy ? 1 : 0
        const bU = b.kind === 'Pod' && !b.healthy ? 1 : 0
        if (aU !== bU) return bU - aU
        const nc = a.name.localeCompare(b.name)
        return nc !== 0 ? nc : (a.namespace || '').localeCompare(b.namespace || '')
    })
}

// ─── Node row (shared between flat and tree) ─────────────────────────────────
function NodeRow({ node, depth = 0, selectedUID, onSelect }: {
    node: NodeInfo
    depth?: number
    selectedUID: string | null
    onSelect: (node: NodeInfo) => void
}) {
    const isSelected = node.uid === selectedUID
    const isUnhealthy = node.kind === 'Pod' && !node.healthy
    const color = kindColor(node.kind)
    const NodeIcon = kindIcon(node.kind)
    const paddingLeft = 36 + depth * 14  // 36 = base indent (icon col)

    return (
        <button
            onClick={() => onSelect(node)}
            className={clsx(
                'w-full flex items-center gap-2 pr-3 py-1.5 text-left border-l-2 transition-all',
                isSelected
                    ? 'bg-accent/10 border-l-accent'
                    : 'border-l-transparent hover:bg-space-800/40 hover:border-l-space-600'
            )}
            style={{ paddingLeft }}
        >
            <div className="flex-shrink-0 w-4 h-4 flex items-center justify-center">
                {NodeIcon && <NodeIcon className="w-3.5 h-3.5" style={{ color }} />}
            </div>
            <div className="flex-1 min-w-0 flex flex-col justify-center">
                <span className={clsx(
                    'text-[11px] truncate leading-tight',
                    isSelected ? 'text-white font-medium' : 'text-slate-300',
                    isUnhealthy && !isSelected && 'text-red-400 font-medium'
                )}>
                    {node.name}
                </span>
                {node.namespace && (
                    <span className="text-[9px] text-slate-500 font-mono truncate leading-tight">
                        {node.namespace}
                    </span>
                )}
            </div>
            {(node.kind === 'Pod' || node.kind === 'PersistentVolumeClaim') && (
                <span className={clsx(
                    'w-1.5 h-1.5 rounded-full flex-shrink-0',
                    (node.kind === 'Pod' && node.healthy) || (node.kind === 'PersistentVolumeClaim' && node.fields?.phase === 'Bound')
                        ? 'bg-emerald-500'
                        : 'bg-red-500 animate-pulse'
                )} />
            )}
        </button>
    )
}

// ─── Workload tree node (recursive) ─────────────────────────────────────────
function WorkloadTreeNode({ node, depth, selectedUID, onSelect, childMap, nodeByUID }: {
    node: NodeInfo
    depth: number
    selectedUID: string | null
    onSelect: (node: NodeInfo) => void
    childMap: Record<string, string[]>
    nodeByUID: Record<string, NodeInfo>
}) {
    const children = (childMap[node.uid] || [])
        .map(uid => nodeByUID[uid])
        .filter(Boolean)
    const sortedChildren = sortNodes(children)
    const [open, setOpen] = useState(true)
    const hasChildren = sortedChildren.length > 0
    const isSelected = node.uid === selectedUID
    const isUnhealthy = node.kind === 'Pod' && !node.healthy
    const color = kindColor(node.kind)
    const NodeIcon = kindIcon(node.kind)
    const paddingLeft = 12 + depth * 14

    return (
        <div>
            <div className={clsx(
                'w-full flex items-center gap-1 pr-3 py-1.5 border-l-2 transition-all group',
                isSelected ? 'bg-accent/10 border-l-accent' : 'border-l-transparent hover:bg-space-800/40 hover:border-l-space-600'
            )} style={{ paddingLeft }}>
                {/* collapse toggle */}
                <button
                    onClick={() => hasChildren && setOpen(o => !o)}
                    className="flex-shrink-0 w-3.5 h-3.5 flex items-center justify-center text-slate-600"
                >
                    {hasChildren
                        ? (open ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />)
                        : <span className="w-3 h-3" />}
                </button>
                {/* clickable row */}
                <button onClick={() => onSelect(node)} className="flex items-center gap-2 flex-1 min-w-0 text-left">
                    <div className="flex-shrink-0 w-4 h-4 flex items-center justify-center">
                        {NodeIcon && <NodeIcon className="w-3.5 h-3.5" style={{ color }} />}
                    </div>
                    <div className="flex-1 min-w-0">
                        <span className={clsx(
                            'text-[11px] truncate leading-tight block',
                            isSelected ? 'text-white font-medium' : 'text-slate-300',
                            isUnhealthy && !isSelected && 'text-red-400 font-medium'
                        )}>{node.name}</span>
                        {node.namespace && (
                            <span className="text-[9px] text-slate-500 font-mono truncate leading-tight block">{node.namespace}</span>
                        )}
                    </div>
                    {node.kind === 'Pod' && (
                        <span className={clsx('w-1.5 h-1.5 rounded-full flex-shrink-0',
                            node.healthy ? 'bg-emerald-500' : 'bg-red-500 animate-pulse'
                        )} />
                    )}
                </button>
            </div>
            {open && hasChildren && (
                <div>
                    {sortedChildren.map(child => (
                        <WorkloadTreeNode key={child.uid} node={child} depth={depth + 1}
                            selectedUID={selectedUID} onSelect={onSelect}
                            childMap={childMap} nodeByUID={nodeByUID} />
                    ))}
                </div>
            )}
        </div>
    )
}

// ─── Collapsible sub-group header ────────────────────────────────────────────
function SubGroup({ id, label, icon: Icon, items, expanded, onToggle, selectedUID, onSelect, depth = 0 }: {
    id: string
    label: string
    icon: React.ElementType
    items: NodeInfo[]
    expanded: Set<string>
    onToggle: (id: string) => void
    selectedUID: string | null
    onSelect: (node: NodeInfo) => void
    depth?: number
}) {
    const isOpen = expanded.has(id)
    if (items.length === 0) return null
    const paddingLeft = 20 + depth * 14
    return (
        <div>
            <button
                onClick={() => onToggle(id)}
                className="w-full flex items-center gap-2 py-1.5 hover:bg-space-800/40 transition-colors text-left"
                style={{ paddingLeft }}
            >
                <span className="text-slate-600 w-3 flex-shrink-0">
                    {isOpen ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
                </span>
                <Icon className="w-3.5 h-3.5 text-slate-500 flex-shrink-0" />
                <span className="text-[10px] font-semibold uppercase tracking-wider text-slate-500 flex-1">{label}</span>
                <span className="text-[9px] text-slate-600 tabular-nums pr-3">{items.length}</span>
            </button>
            {isOpen && (
                <div className="space-y-px">
                    {sortNodes(items).map(node => (
                        <NodeRow key={node.uid} node={node} depth={1} selectedUID={selectedUID} onSelect={onSelect} />
                    ))}
                </div>
            )}
        </div>
    )
}

export function Sidebar({ nodes, edges, selectedUID, onSelect }: Props) {
    const [search, setSearch] = useState('')
    const [expanded, setExpanded] = useState<Set<string>>(new Set(DEFAULT_EXPANDED))
    const [expandedSubs, setExpandedSubs] = useState<Set<string>>(new Set(DEFAULT_EXPANDED_SUBS))

    const toggle = (id: string) =>
        setExpanded(prev => { const n = new Set(prev); n.has(id) ? n.delete(id) : n.add(id); return n })
    const toggleSub = (id: string) =>
        setExpandedSubs(prev => { const n = new Set(prev); n.has(id) ? n.delete(id) : n.add(id); return n })

    // ── Build ownership map from edges ────────────────────────────────────────
    const { childMap, nodeByUID, workloadRoots, flatWorkloads } = useMemo(() => {
        const nodeByUID: Record<string, NodeInfo> = {}
        for (const n of nodes) nodeByUID[n.uid] = n

        // childMap: parentUID → [childUID, ...] for "owns" edges within workloads
        const childMap: Record<string, string[]> = {}
        const ownedUIDs = new Set<string>()
        for (const e of edges) {
            if (e.rel !== 'owns') continue
            const parent = nodeByUID[e.from]
            const child  = nodeByUID[e.to]
            if (!parent || !child) continue
            if (!WORKLOAD_KINDS.has(parent.kind) || !WORKLOAD_KINDS.has(child.kind)) continue
            childMap[e.from] = childMap[e.from] || []
            childMap[e.from].push(e.to)
            ownedUIDs.add(e.to)
        }

        const workloadNodes = nodes.filter(n => WORKLOAD_KINDS.has(n.kind))
        // Roots: top-level workload kinds OR any workload not owned by another workload
        const workloadRoots = workloadNodes.filter(n =>
            TOP_WORKLOAD_KINDS.has(n.kind) || (!ownedUIDs.has(n.uid) && (n.kind === 'Job' || n.kind === 'Pod'))
        )
        // For search: flat list of all workload nodes
        const flatWorkloads = workloadNodes

        return { childMap, nodeByUID, workloadRoots, flatWorkloads }
    }, [nodes, edges])

    const lower = search.toLowerCase()

    const filteredNodes = useMemo(() =>
        nodes.filter(n => !lower || n.name.toLowerCase().includes(lower) || n.namespace?.toLowerCase().includes(lower) || n.kind.toLowerCase().includes(lower))
    , [nodes, lower])

    const filteredWorkloads = lower
        ? filteredNodes.filter(n => WORKLOAD_KINDS.has(n.kind))
        : workloadRoots

    const flatGroups = useMemo(() =>
        FLAT_CATEGORIES.map(cat => ({ ...cat, items: filteredNodes.filter(n => cat.kinds.includes(n.kind)) }))
            .filter(g => g.items.length > 0)
    , [filteredNodes])

    const securityNodes = filteredNodes.filter(n => SECURITY_SUBGROUPS.flatMap(s => s.kinds).includes(n.kind))
    const configNodes   = filteredNodes.filter(n => CONFIG_SUBGROUPS.flatMap(s => s.kinds).includes(n.kind))

    const totalUnhealthy = nodes.filter(n => n.kind === 'Pod' && !n.healthy).length
    const workloadUnhealthy = (lower ? filteredWorkloads : flatWorkloads).filter(n => n.kind === 'Pod' && !n.healthy).length

    // ── Category header helper ──────────────────────────────────────────────
    function CatHeader({ id, label, CatIcon, count, unhealthyCount }: {
        id: string; label: string; CatIcon: React.ElementType; count: number; unhealthyCount?: number
    }) {
        const isOpen = expanded.has(id)
        return (
            <button onClick={() => toggle(id)}
                className="w-full flex items-center gap-2 px-3 py-2 hover:bg-space-800/60 transition-colors group"
            >
                <span className="text-slate-600 group-hover:text-slate-400 transition-colors flex-shrink-0 w-3">
                    {isOpen ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />}
                </span>
                <CatIcon className="w-4 h-4 text-slate-400" />
                <span className="text-xs font-semibold uppercase tracking-widest flex-1 text-left text-slate-300">{label}</span>
                <div className="flex items-center gap-1.5 flex-shrink-0">
                    {(unhealthyCount ?? 0) > 0 && (
                        <span className="text-[10px] font-bold text-status-unhealthy bg-red-950/60 rounded px-1.5 py-0.5 leading-none">
                            {unhealthyCount} err
                        </span>
                    )}
                    <span className="text-[10px] text-slate-500 tabular-nums min-w-[18px] text-right">{count}</span>
                </div>
            </button>
        )
    }

    const noResults = filteredWorkloads.length === 0 && securityNodes.length === 0 && configNodes.length === 0 && flatGroups.every(g => g.items.length === 0)

    return (
        <div className="flex flex-col h-full bg-space-900 border-r border-space-700">
            {/* ── Search ──────────────────────────────────────── */}
            <div className="px-3 pt-3 pb-2 flex-shrink-0">
                <div className="relative">
                    <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-slate-600" />
                    <input
                        type="text"
                        placeholder="Search resources…"
                        value={search}
                        onChange={e => setSearch(e.target.value)}
                        className="w-full bg-space-800 border border-space-700 rounded-lg pl-8 pr-3 py-2 text-xs text-slate-300 placeholder-slate-600 focus:outline-none focus:border-accent/60 transition-colors"
                    />
                </div>
            </div>

            {/* ── Summary ─────────────────────────────────────── */}
            <div className="px-3 pb-2.5 flex items-center gap-3 flex-shrink-0">
                <span className="text-xs text-slate-500">{nodes.length} resources</span>
                {totalUnhealthy > 0 && (
                    <span className="text-xs text-status-unhealthy font-medium flex items-center gap-1.5">
                        <span className="w-2 h-2 rounded-full bg-status-unhealthy animate-pulse inline-block" />
                        {totalUnhealthy} unhealthy
                    </span>
                )}
            </div>

            <div className="h-px bg-space-700 mx-3 flex-shrink-0" />

            {/* ── Grouped list ────────────────────────────────── */}
            <div className="flex-1 overflow-y-auto py-2">
                {noResults && (
                    <div className="text-xs text-slate-600 text-center py-12">
                        {search ? `No matches for "${search}"` : 'No resources'}
                    </div>
                )}

                {/* ══ WORKLOADS (hierarchical tree) ══════════════════════════════ */}
                {filteredWorkloads.length > 0 && (
                    <div className="mb-1">
                        <CatHeader id="workloads" label="Workloads" CatIcon={Package}
                            count={lower ? filteredWorkloads.length : flatWorkloads.length}
                            unhealthyCount={workloadUnhealthy} />
                        {(expanded.has('workloads') || !!lower) && (
                            <div className="mb-2">
                                {lower
                                    ? sortNodes(filteredWorkloads).map(n => (
                                        <NodeRow key={n.uid} node={n} depth={0} selectedUID={selectedUID} onSelect={onSelect} />
                                    ))
                                    : sortNodes(filteredWorkloads).map(n => (
                                        <WorkloadTreeNode key={n.uid} node={n} depth={0}
                                            selectedUID={selectedUID} onSelect={onSelect}
                                            childMap={childMap} nodeByUID={nodeByUID} />
                                    ))
                                }
                            </div>
                        )}
                    </div>
                )}

                {/* ══ SECURITY (with sub-categories) ════════════════════════════ */}
                {securityNodes.length > 0 && (
                    <div className="mb-1">
                        <CatHeader id="security" label="Security" CatIcon={Shield}
                            count={securityNodes.length} />
                        {(expanded.has('security') || !!lower) && (
                            <div className="mb-2">
                                {SECURITY_SUBGROUPS.map(sub => (
                                    <SubGroup key={sub.id}
                                        id={sub.id} label={sub.label} icon={sub.icon}
                                        items={filteredNodes.filter(n => sub.kinds.includes(n.kind))}
                                        expanded={expandedSubs} onToggle={toggleSub}
                                        selectedUID={selectedUID} onSelect={onSelect} />
                                ))}
                            </div>
                        )}
                    </div>
                )}

                {/* ══ CONFIG & STORAGE (with sub-categories) ═════════════════════ */}
                {configNodes.length > 0 && (
                    <div className="mb-1">
                        <CatHeader id="config" label="Config & Storage" CatIcon={Settings2}
                            count={configNodes.length} />
                        {(expanded.has('config') || !!lower) && (
                            <div className="mb-2">
                                {CONFIG_SUBGROUPS.map(sub => (
                                    <SubGroup key={sub.id}
                                        id={sub.id} label={sub.label} icon={sub.icon}
                                        items={filteredNodes.filter(n => sub.kinds.includes(n.kind))}
                                        expanded={expandedSubs} onToggle={toggleSub}
                                        selectedUID={selectedUID} onSelect={onSelect} />
                                ))}
                            </div>
                        )}
                    </div>
                )}

                {/* ══ FLAT CATEGORIES (Network, Infrastructure) ═════════════════ */}
                {flatGroups.map(({ id, label, icon: CatIcon, items }) => {
                    const unhealthy = items.filter(n => n.kind === 'Pod' && !n.healthy).length
                    return (
                        <div key={id} className="mb-1">
                            <CatHeader id={id} label={label} CatIcon={CatIcon}
                                count={items.length} unhealthyCount={unhealthy} />
                            {(expanded.has(id) || !!lower) && (
                                <div className="mb-2 space-y-px">
                                    {sortNodes(items).map(node => (
                                        <NodeRow key={node.uid} node={node} depth={0}
                                            selectedUID={selectedUID} onSelect={onSelect} />
                                    ))}
                                </div>
                            )}
                        </div>
                    )
                })}
            </div>
        </div>
    )
}
