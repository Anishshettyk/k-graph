import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, RefreshCw, Cpu, HardDrive, Loader2 } from 'lucide-react'
import { api } from '../api/client'
import { kindIcon, kindColor } from '../lib/kinds'
import type { NodeInfo, WhyResponse } from '../types/api'
import { LogViewer } from './LogViewer'
import { YAMLPanel } from './YAMLPanel'

interface Props {
    node: NodeInfo
    context: string
    namespace: string
    defaultTab?: 'info' | 'yaml' | 'why' | 'events' | 'logs'
}

export function DetailsPanel({ node, context, namespace, defaultTab }: Props) {
    const [tab, setTab] = useState<'info' | 'why' | 'events' | 'yaml' | 'logs'>(defaultTab ?? 'info')

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

    const color = kindColor(node.kind)
    const tabs = ['info', 'yaml', 'why', 'events', 'logs'] as const

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

// suppress unused import warnings
void ChevronDown; void RefreshCw; void Cpu; void HardDrive
