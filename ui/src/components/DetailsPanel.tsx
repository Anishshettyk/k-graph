import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, RefreshCw, Cpu, HardDrive, Loader2, Clock, CheckCircle2, XCircle, ArrowRight } from 'lucide-react'
import clsx from 'clsx'
import { api } from '../api/client'
import { kindIcon, kindColor } from '../lib/kinds'
import type { NodeInfo, WhyResponse, HistoryResponse } from '../types/api'
import { LogViewer } from './LogViewer'
import { YAMLPanel } from './YAMLPanel'

interface Props {
    node: NodeInfo
    context: string
    namespace: string
    defaultTab?: 'info' | 'yaml' | 'why' | 'events' | 'logs' | 'history'
}

const HISTORY_KINDS = new Set(['Deployment', 'StatefulSet'])

export function DetailsPanel({ node, context, namespace, defaultTab }: Props) {
    const showHistory = HISTORY_KINDS.has(node.kind)
    type Tab = 'info' | 'why' | 'events' | 'yaml' | 'logs' | 'history'
    const allTabs: Tab[] = showHistory
        ? ['info', 'yaml', 'why', 'events', 'logs', 'history']
        : ['info', 'yaml', 'why', 'events', 'logs']

    const [tab, setTab] = useState<Tab>(defaultTab ?? 'info')

    const whyQuery = useQuery({
        queryKey: ['why', context, namespace, `${node.kind}/${node.name}`],
        queryFn: () => api.why(context, namespace, `${node.kind.toLowerCase()}/${node.name}`),
        enabled: tab === 'why',
        staleTime: 20_000,
    })

    const eventsQuery = useQuery({
        queryKey: ['events', context, namespace, `${node.kind}/${node.name}`],
        queryFn: () => api.events(context, namespace, `${node.kind.toLowerCase()}/${node.name}`),
        enabled: tab === 'events',
        staleTime: 20_000,
    })

    const yamlQuery = useQuery({
        queryKey: ['yaml', context, namespace, `${node.kind}/${node.name}`],
        queryFn: () => api.yaml(context, namespace, `${node.kind.toLowerCase()}/${node.name}`),
        enabled: tab === 'yaml',
        staleTime: 60_000,
    })

    const logsQuery = useQuery({
        queryKey: ['logs', context, namespace, `${node.kind}/${node.name}`],
        queryFn: () => api.logs(context, namespace, `${node.kind.toLowerCase()}/${node.name}`),
        enabled: tab === 'logs',
        staleTime: 30_000,
    })

    const historyQuery = useQuery({
        queryKey: ['history', context, namespace, `${node.kind}/${node.name}`],
        queryFn: () => api.history(context, namespace, `${node.kind}/${node.name}`),
        enabled: tab === 'history' && showHistory,
        staleTime: 30_000,
    })

    const color = kindColor(node.kind)
    const tabs = allTabs

    return (
        <div className="flex flex-col h-full overflow-hidden">
            {/* Header */}
            <div
                className="px-4 py-3 border-b border-space-700 flex-shrink-0"
                style={{ borderTopColor: color, borderTopWidth: 2 }}
            >
                <div className="flex items-center gap-2">
                    {(() => { const Icon = kindIcon(node.kind); return Icon && <Icon className="w-5 h-5" style={{ color }} />; })()}
                    <div className="min-w-0">
                        <div className="font-semibold text-slate-100 truncate text-sm">{node.name}</div>
                        {node.namespace && (
                            <div className="text-xs text-slate-500 font-mono">{node.namespace}</div>
                        )}
                    </div>
                    {node.kind === 'Pod' && (
                        <span className={`ml-auto text-xs px-2 py-0.5 rounded-full font-medium ${node.healthy
                            ? 'bg-emerald-950/50 text-status-healthy'
                            : 'bg-red-950/50 text-status-unhealthy'
                            }`}>
                            {node.healthy ? '● Ready' : '✖ ' + (node.reason ?? 'Unhealthy')}
                        </span>
                    )}
                </div>
            </div>

            {/* Tabs */}
            <div className="flex border-b border-space-700 flex-shrink-0 overflow-x-auto">
                {tabs.map(t => (
                    <button
                        key={t}
                        onClick={() => setTab(t)}
                        className={`px-3 py-2 text-xs font-medium capitalize transition-colors whitespace-nowrap ${tab === t
                            ? 'text-accent border-b-2 border-accent'
                            : 'text-slate-500 hover:text-slate-300'
                            }`}
                    >
                        {t}
                    </button>
                ))}
            </div>

            {/* Content — full height for yaml/logs, scrollable for others */}
            {(tab === 'yaml' || tab === 'logs') ? (
                <div className="flex-1 overflow-hidden">
                    {tab === 'yaml' && (
                        <YAMLPanel
                            yaml={yamlQuery.data?.yaml ?? ''}
                            isLoading={yamlQuery.isLoading}
                            error={yamlQuery.error ? (yamlQuery.error as Error).message : undefined}
                        />
                    )}
                    {tab === 'logs' && (
                        <LogViewer
                            pods={logsQuery.data?.pods ?? []}
                            isLoading={logsQuery.isLoading}
                            error={logsQuery.error ? (logsQuery.error as Error).message : undefined}
                        />
                    )}
                </div>
            ) : (
                <div className="flex-1 overflow-y-auto p-4 space-y-3">
                    {tab === 'info' && <InfoTab node={node} />}
                    {tab === 'why' && <WhyTab query={whyQuery} />}
                    {tab === 'events' && <EventsTab query={eventsQuery} />}
                    {tab === 'history' && <HistoryTab query={historyQuery} />}
                </div>
            )}
        </div>
    )
}

function InfoTab({ node }: { node: NodeInfo }) {
    return (
        <div className="space-y-0 divide-y divide-space-800">
            <Row label="Kind" value={node.kind} />
            {node.namespace && <Row label="Namespace" value={node.namespace} mono />}
            <Row label="Name" value={node.name} mono />
            {node.uid && <Row label="UID" value={node.uid} mono small />}
            {node.fields && Object.entries(node.fields).map(([k, v]) =>
                v ? <Row key={k} label={k} value={v} mono /> : null
            )}
            {node.labels && Object.keys(node.labels).length > 0 && (
                <div className="py-2">
                    <div className="text-[10px] text-slate-500 uppercase tracking-wider mb-1.5">Labels</div>
                    <div className="flex flex-wrap gap-1">
                        {Object.entries(node.labels).map(([k, v]) => (
                            <span key={k} className="text-[10px] font-mono bg-space-800 border border-space-700 rounded px-1.5 py-0.5 text-slate-400 break-all">
                                {k}={v}
                            </span>
                        ))}
                    </div>
                </div>
            )}
        </div>
    )
}

// Row uses a two-column table-like layout: fixed label on the left, value wraps
// on the right. Prevents the two columns from overlapping in narrow panels.
function Row({ label, value, mono, small }: { label: string; value: string; mono?: boolean; small?: boolean }) {
    return (
        <div className="grid grid-cols-[6rem_1fr] gap-2 py-1.5 items-start">
            <span className="text-[10px] text-slate-500 uppercase tracking-wider pt-0.5 leading-tight">{label}</span>
            <span className={`text-xs break-words min-w-0 ${mono ? 'font-mono text-slate-300' : 'text-slate-200'} ${small ? 'text-[10px] text-slate-500 leading-snug' : ''}`}>
                {value}
            </span>
        </div>
    )
}

function WhyTab({ query }: { query: ReturnType<typeof useQuery<WhyResponse>> }) {
    if (query.isLoading) return <Spinner />
    if (query.error) return <ErrorMsg message={(query.error as Error).message} />
    if (!query.data || !query.data.diagnoses?.length) {
        return (
            <div className="flex flex-col items-center justify-center py-8 text-center">
                <span className="text-2xl mb-2">✓</span>
                <span className="text-sm text-status-healthy">No issues detected</span>
            </div>
        )
    }
    return (
        <div className="space-y-4">
            {query.data.diagnoses.map((d, i) => (
                <div key={i} className="rounded-lg border border-red-900/40 bg-red-950/10 p-3 space-y-2">
                    <div className="flex items-center gap-2">
                        <span className="text-status-unhealthy">✖</span>
                        <span className="text-sm font-medium text-slate-200 font-mono">{d.pod.name}</span>
                    </div>
                    {d.status && (
                        <div className="text-xs text-slate-400">status: <span className="text-status-unhealthy">{d.status}</span></div>
                    )}
                    {d.summary && (
                        <div className="text-xs text-status-unhealthy font-medium">{d.summary}</div>
                    )}
                    {d.findings?.map((f, fi) => (
                        <div key={fi} className="text-xs">
                            <span className="text-status-warning">{f.title}:</span>{' '}
                            <span className="text-slate-400">{f.detail}</span>
                        </div>
                    ))}
                    {d.events?.map((ev, ei) => (
                        <div key={ei} className="text-xs font-mono text-slate-500">
                            <span className="text-accent/70">[{ev.reason}]</span> {ev.message}
                        </div>
                    ))}
                    {d.logs?.map((lg, li) => (
                        <div key={li} className="text-[10px] font-mono bg-space-900 rounded p-2 text-slate-500 border border-space-700">
                            <div className="text-accent/60 mb-1">{lg.container}</div>
                            <pre className="whitespace-pre-wrap break-all">{lg.excerpt.split('\n').slice(-3).join('\n')}</pre>
                        </div>
                    ))}
                </div>
            ))}
        </div>
    )
}

function EventsTab({ query }: { query: ReturnType<typeof useQuery> }) {
    const data = query.data as import('../types/api').EventsResponse | undefined
    if (query.isLoading) return <Spinner />
    if (query.error) return <ErrorMsg message={(query.error as Error).message} />
    if (!data?.events?.length) {
        return (
            <div className="text-center py-8 space-y-1">
                <div className="text-slate-600 text-xs">No events found</div>
                <div className="text-slate-700 text-[10px]">
                    Events are stored for ~1 hour in Kubernetes.
                    If the resource is stable, no recent events may exist.
                </div>
            </div>
        )
    }
    return (
        <div className="space-y-1.5">
            {data.events.map((ev, i) => (
                <div key={i} className="rounded border border-space-700 bg-space-850 p-2 space-y-0.5">
                    <div className="flex items-center gap-2">
                        <span className={`text-[10px] font-semibold px-1.5 py-px rounded ${ev.type === 'Warning' ? 'bg-amber-950/50 text-status-warning' : 'bg-blue-950/50 text-blue-400'
                            }`}>{ev.type}</span>
                        <span className="text-xs font-medium text-slate-300">{ev.reason}</span>
                        <span className="ml-auto text-[10px] text-slate-600 font-mono">{ev.age}</span>
                    </div>
                    <div className="text-[10px] text-slate-500 font-mono">{ev.object}</div>
                    <div className="text-[11px] text-slate-400 break-words">{ev.message}</div>
                </div>
            ))}
        </div>
    )
}

function Spinner() {
    return (
        <div className="flex justify-center py-8">
            <Loader2 className="w-5 h-5 text-accent animate-spin" />
        </div>
    )
}

function ErrorMsg({ message }: { message: string }) {
    return <div className="text-xs text-status-unhealthy bg-red-950/20 rounded p-2">{message}</div>
}

// ─── History Tab ─────────────────────────────────────────────────────────────

function HistoryTab({ query }: { query: ReturnType<typeof useQuery<HistoryResponse>> }) {
    const [selected, setSelected] = useState<Set<string>>(new Set())
    const [showDiff, setShowDiff] = useState(false)

    if (query.isLoading) return <Spinner />
    if (query.error) return <ErrorMsg message={(query.error as Error).message} />
    if (!query.data || !query.data.revisions?.length) {
        return <div className="text-center py-8 text-slate-600 text-xs">No rollout history found</div>
    }

    const revisions = query.data.revisions
    const total = revisions.length

    const toggleSelect = (name: string) => {
        setShowDiff(false)
        setSelected(prev => {
            const next = new Set(prev)
            if (next.has(name)) { next.delete(name); return next }
            if (next.size >= 2) {
                // replace oldest selection
                const [first] = next
                next.delete(first)
            }
            next.add(name)
            return next
        })
    }

    const selectedRevs = revisions.filter(r => selected.has(r.name))
    const canCompare = selectedRevs.length === 2

    if (showDiff && canCompare) {
        // older = higher index (lower revNum); newer = lower index (higher revNum)
        const idxA = revisions.findIndex(r => r.name === selectedRevs[0].name)
        const idxB = revisions.findIndex(r => r.name === selectedRevs[1].name)
        const [newer, older] = idxA < idxB ? [selectedRevs[0], selectedRevs[1]] : [selectedRevs[1], selectedRevs[0]]
        return <RevisionDiff newer={newer} older={older} total={total} revisions={revisions}
            onBack={() => setShowDiff(false)} />
    }

    return (
        <div className="space-y-2">
            <div className="flex items-center gap-2 mb-3">
                <Clock className="w-3.5 h-3.5 text-slate-500" />
                <span className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">
                    {total} revision{total !== 1 ? 's' : ''} — newest first
                </span>
                {canCompare && (
                    <button onClick={() => setShowDiff(true)}
                        className="ml-auto flex items-center gap-1 text-[10px] font-semibold px-2 py-1 rounded border border-accent/40 bg-accent/10 text-accent hover:bg-accent/20 transition-colors">
                        <CheckCircle2 className="w-3 h-3" /> Compare
                    </button>
                )}
                {selected.size === 1 && (
                    <span className="ml-auto text-[9px] text-slate-600">Select one more to compare</span>
                )}
                {selected.size === 0 && total > 1 && (
                    <span className="ml-auto text-[9px] text-slate-600">Select up to 2 to compare</span>
                )}
            </div>

            {revisions.map((rev, i) => {
                const revNum = total - i
                const healthy = rev.readyReplicas === rev.desiredReplicas && rev.desiredReplicas > 0
                const idle    = rev.replicas === 0
                const isSelected = selected.has(rev.name)
                return (
                    <div key={rev.name}
                        onClick={() => toggleSelect(rev.name)}
                        className={clsx(
                            'rounded-lg border p-3 space-y-2 cursor-pointer transition-all',
                            rev.isCurrent && !isSelected ? 'border-accent/40 bg-accent/5 hover:bg-accent/10' :
                            isSelected ? 'border-violet-500/60 bg-violet-950/20 ring-1 ring-violet-500/30' :
                            'border-space-700 bg-space-850 hover:border-space-600'
                        )}>
                        <div className="flex items-center gap-2">
                            {/* Selection indicator */}
                            <div className={clsx(
                                'w-4 h-4 rounded border flex-shrink-0 flex items-center justify-center transition-all',
                                isSelected ? 'border-violet-500 bg-violet-500' : 'border-space-600'
                            )}>
                                {isSelected && <span className="text-[8px] text-white font-bold">✓</span>}
                            </div>
                            <span className={clsx(
                                'text-[9px] font-bold uppercase px-1.5 py-0.5 rounded border flex-shrink-0',
                                rev.isCurrent ? 'text-accent border-accent/40 bg-accent/10' : 'text-slate-600 border-space-700 bg-space-800'
                            )}>rev {revNum}</span>
                            {rev.isCurrent && <span className="text-[9px] font-bold text-emerald-400">▶ CURRENT</span>}
                            <span className="text-[10px] text-slate-600 font-mono ml-auto flex-shrink-0">
                                {new Date(rev.createdAt).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })}
                            </span>
                        </div>
                        {!idle && (
                            <div className="flex items-center gap-2">
                                <div className="flex gap-0.5 flex-1">
                                    {Array.from({ length: rev.desiredReplicas }).map((_, j) => (
                                        <div key={j} className={clsx('h-1.5 flex-1 rounded-sm',
                                            j < rev.readyReplicas ? 'bg-emerald-500' : 'bg-red-700/60')} />
                                    ))}
                                </div>
                                <span className={clsx('text-[10px] font-mono flex-shrink-0', healthy ? 'text-emerald-400' : 'text-red-400')}>
                                    {rev.readyReplicas}/{rev.desiredReplicas}
                                </span>
                            </div>
                        )}
                        {idle && <div className="text-[10px] text-slate-600">scaled down</div>}
                        {(rev.images ?? []).map(img => (
                            <div key={img} className="flex items-center gap-1.5 text-[10px]">
                                <ArrowRight className="w-3 h-3 text-slate-700 flex-shrink-0" />
                                <span className="font-mono text-slate-500 truncate">{img}</span>
                            </div>
                        ))}
                        <div className="text-[9px] font-mono text-slate-700 truncate">{rev.name}</div>
                    </div>
                )
            })}
        </div>
    )
}

function RevisionDiff({ newer, older, total, revisions, onBack }: {
    newer: import('../types/api').RSRevision
    older: import('../types/api').RSRevision
    total: number
    revisions: import('../types/api').RSRevision[]
    onBack: () => void
}) {
    const newerIdx = revisions.findIndex(r => r.name === newer.name)
    const olderIdx = revisions.findIndex(r => r.name === older.name)
    const newerRev = total - newerIdx
    const olderRev = total - olderIdx

    const newerImgs = new Set(newer.images ?? [])
    const olderImgs = new Set(older.images ?? [])
    const addedImgs   = [...newerImgs].filter(i => !olderImgs.has(i))
    const removedImgs = [...olderImgs].filter(i => !newerImgs.has(i))
    const sameImgs    = [...newerImgs].filter(i => olderImgs.has(i))

    const daysDiff = Math.round(
        (new Date(newer.createdAt).getTime() - new Date(older.createdAt).getTime()) / 86400000
    )

    return (
        <div className="space-y-3">
            <div className="flex items-center gap-2">
                <button onClick={onBack} className="text-[10px] text-slate-500 hover:text-accent flex items-center gap-1">
                    ← back
                </button>
                <span className="text-[10px] font-semibold uppercase tracking-widest text-slate-500 ml-1">
                    Comparing rev {olderRev} → rev {newerRev}
                </span>
            </div>

            {/* Side-by-side header */}
            <div className="grid grid-cols-2 gap-2">
                <div className="rounded-lg border border-slate-700 bg-space-850 p-2 text-center">
                    <div className="text-[9px] text-slate-600 mb-0.5">OLDER</div>
                    <div className="text-[10px] font-bold text-slate-400">rev {olderRev}</div>
                    <div className="text-[9px] text-slate-600 font-mono">{new Date(older.createdAt).toLocaleDateString()}</div>
                </div>
                <div className="rounded-lg border border-accent/40 bg-accent/5 p-2 text-center">
                    <div className="text-[9px] text-accent mb-0.5">NEWER</div>
                    <div className="text-[10px] font-bold text-accent">rev {newerRev}</div>
                    <div className="text-[9px] text-slate-600 font-mono">{new Date(newer.createdAt).toLocaleDateString()}</div>
                </div>
            </div>

            {/* Time gap */}
            <div className="text-center text-[10px] text-slate-600">
                {Math.abs(daysDiff)} day{Math.abs(daysDiff) !== 1 ? 's' : ''} between revisions
            </div>

            {/* Replicas diff */}
            {older.desiredReplicas !== newer.desiredReplicas && (
                <DiffRow label="Replicas" oldVal={`${older.desiredReplicas}`} newVal={`${newer.desiredReplicas}`} />
            )}
            {older.desiredReplicas === newer.desiredReplicas && (
                <div className="text-[10px] text-slate-600 flex items-center gap-1.5">
                    <CheckCircle2 className="w-3 h-3 text-slate-600" />
                    Replicas unchanged ({newer.desiredReplicas})
                </div>
            )}

            {/* Image diffs */}
            {addedImgs.length > 0 && (
                <div className="space-y-1">
                    <div className="text-[9px] font-bold uppercase tracking-widest text-emerald-500">+ Added images</div>
                    {addedImgs.map(img => (
                        <div key={img} className="text-[10px] font-mono text-emerald-400 bg-emerald-950/20 rounded px-2 py-1 border border-emerald-900/30">
                            + {img}
                        </div>
                    ))}
                </div>
            )}
            {removedImgs.length > 0 && (
                <div className="space-y-1">
                    <div className="text-[9px] font-bold uppercase tracking-widest text-red-400">- Removed images</div>
                    {removedImgs.map(img => (
                        <div key={img} className="text-[10px] font-mono text-red-400 bg-red-950/20 rounded px-2 py-1 border border-red-900/30 line-through opacity-70">
                            - {img}
                        </div>
                    ))}
                </div>
            )}
            {sameImgs.length > 0 && (
                <div className="space-y-1">
                    <div className="text-[9px] font-semibold text-slate-600">Unchanged images</div>
                    {sameImgs.map(img => (
                        <div key={img} className="text-[10px] font-mono text-slate-600 px-2 py-0.5">
                            = {img}
                        </div>
                    ))}
                </div>
            )}

            {/* RS names */}
            <div className="grid grid-cols-2 gap-2 pt-1 border-t border-space-700">
                <div className="text-[9px] font-mono text-slate-700 truncate">{older.name}</div>
                <div className="text-[9px] font-mono text-slate-600 truncate">{newer.name}</div>
            </div>
        </div>
    )
}

function DiffRow({ label, oldVal, newVal }: { label: string; oldVal: string; newVal: string }) {
    return (
        <div className="space-y-1">
            <div className="text-[9px] font-bold uppercase tracking-widest text-amber-500">{label} changed</div>
            <div className="grid grid-cols-2 gap-2">
                <div className="text-[10px] font-mono text-red-400 bg-red-950/20 rounded px-2 py-1 border border-red-900/30 line-through opacity-70">
                    {oldVal}
                </div>
                <div className="text-[10px] font-mono text-emerald-400 bg-emerald-950/20 rounded px-2 py-1 border border-emerald-900/30">
                    {newVal}
                </div>
            </div>
        </div>
    )
}

// suppress unused import warnings
void ChevronDown; void RefreshCw; void Cpu; void HardDrive; void XCircle
