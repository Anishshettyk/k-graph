import { useState, useMemo } from 'react'
import { Search, ChevronRight, ChevronDown, Package, Network, Settings2, Database, Shield, Server } from 'lucide-react'
import clsx from 'clsx'
import { kindIcon, kindColor } from '../lib/kinds'
import type { NodeInfo } from '../types/api'

interface Props {
    nodes: NodeInfo[]
    selectedUID: string | null
    filterKind: string
    onSelect: (node: NodeInfo) => void
    onFilterKind: (kind: string) => void
}

const CATEGORIES = [
    { id: 'workloads', label: 'Workloads', icon: Package, kinds: ['Deployment', 'StatefulSet', 'DaemonSet', 'CronJob', 'Job', 'Pod', 'ReplicaSet'] },
    { id: 'network', label: 'Network', icon: Network, kinds: ['Service', 'Ingress', 'Endpoints'] },
    { id: 'config', label: 'Config', icon: Settings2, kinds: ['ConfigMap', 'Secret'] },
    { id: 'storage', label: 'Storage', icon: Database, kinds: ['PersistentVolumeClaim', 'PersistentVolume'] },
    { id: 'security', label: 'Security', icon: Shield, kinds: ['NetworkPolicy', 'ServiceAccount', 'Role', 'ClusterRole', 'RoleBinding', 'ClusterRoleBinding'] },
    { id: 'infrastructure', label: 'Infrastructure', icon: Server, kinds: ['Node', 'Namespace'] },
]

const DEFAULT_EXPANDED = new Set(['workloads', 'network'])

export function Sidebar({ nodes, selectedUID, onSelect, onFilterKind }: Props) {
    const [search, setSearch] = useState('')
    const [expanded, setExpanded] = useState<Set<string>>(new Set(DEFAULT_EXPANDED))

    const toggle = (id: string) =>
        setExpanded(prev => {
            const next = new Set(prev)
            next.has(id) ? next.delete(id) : next.add(id)
            return next
        })

    const groups = useMemo(() => {
        const lower = search.toLowerCase()
        
        const filteredNodes = nodes.filter(n => {
            if (!lower) return true
            return n.name.toLowerCase().includes(lower) || n.namespace?.toLowerCase().includes(lower) || n.kind.toLowerCase().includes(lower)
        })
        
        return CATEGORIES.map(cat => {
            const items = filteredNodes.filter(n => cat.kinds.includes(n.kind))
            
            // Health-first sort: unhealthy top, then alphabetical by name
            items.sort((a, b) => {
                const aUnhealthy = a.kind === 'Pod' && !a.healthy ? 1 : 0
                const bUnhealthy = b.kind === 'Pod' && !b.healthy ? 1 : 0
                if (aUnhealthy !== bUnhealthy) return bUnhealthy - aUnhealthy
                
                const nameCmp = a.name.localeCompare(b.name)
                if (nameCmp !== 0) return nameCmp
                return (a.namespace || '').localeCompare(b.namespace || '')
            })
            
            return { ...cat, items }
        }).filter(g => g.items.length > 0)
    }, [nodes, search])

    const totalUnhealthy = nodes.filter(n => n.kind === 'Pod' && !n.healthy).length

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
                {groups.length === 0 && (
                    <div className="text-xs text-slate-600 text-center py-12">
                        {search ? `No matches for "${search}"` : 'No resources'}
                    </div>
                )}

                {groups.map(({ id, label, icon: CatIcon, items }) => {
                    const unhealthy = items.filter(n => n.kind === 'Pod' && !n.healthy).length
                    const isExpanded = expanded.has(id) || !!search

                    return (
                        <div key={id} className="mb-1">
                            {/* ── Category header ── */}
                            <button
                                onClick={() => toggle(id)}
                                className="w-full flex items-center gap-2 px-3 py-2 hover:bg-space-800/60 transition-colors group"
                            >
                                <span className="text-slate-600 group-hover:text-slate-400 transition-colors flex-shrink-0 w-3">
                                    {isExpanded ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />}
                                </span>

                                <CatIcon className="w-4 h-4 text-slate-400" />

                                <span className="text-xs font-semibold uppercase tracking-widest flex-1 text-left text-slate-300">
                                    {label}
                                </span>

                                <div className="flex items-center gap-1.5 flex-shrink-0">
                                    {unhealthy > 0 && (
                                        <span className="text-[10px] font-bold text-status-unhealthy bg-red-950/60 rounded px-1.5 py-0.5 leading-none">
                                            {unhealthy} err
                                        </span>
                                    )}
                                    <span className="text-[10px] text-slate-500 tabular-nums min-w-[18px] text-right">
                                        {items.length}
                                    </span>
                                </div>
                            </button>

                            {/* ── Resource rows ── */}
                            {isExpanded && (
                                <div className="mb-2 space-y-px">
                                    {items.map(node => {
                                        const isSelected = node.uid === selectedUID
                                        const isUnhealthy = node.kind === 'Pod' && !node.healthy
                                        const color = kindColor(node.kind)
                                        const NodeIcon = kindIcon(node.kind)

                                        return (
                                            <button
                                                key={node.uid}
                                                onClick={() => { onSelect(node); onFilterKind('') }}
                                                className={clsx(
                                                    'w-full flex items-center gap-2.5 pl-9 pr-3 py-1.5 text-left',
                                                    'border-l-2 transition-all',
                                                    isSelected
                                                        ? 'bg-accent/10 border-l-accent'
                                                        : 'border-l-transparent hover:bg-space-800/40 hover:border-l-space-600'
                                                )}
                                            >
                                                {/* Icon */}
                                                <div className="flex-shrink-0 w-4 h-4 flex items-center justify-center">
                                                    {NodeIcon && <NodeIcon className="w-3.5 h-3.5" style={{ color }} />}
                                                </div>

                                                {/* Text block */}
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

                                                {/* Status dot */}
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
                                    })}
                                </div>
                            )}
                        </div>
                    )
                })}
            </div>
        </div>
    )
}
