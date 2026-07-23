import { useQuery } from '@tanstack/react-query'
import { Loader2, RefreshCw, Zap, AlertTriangle, Globe, Info } from 'lucide-react'
import clsx from 'clsx'
import { api } from '../api/client'
import { useStore } from '../store/useStore'
import type { AggregatedEvent, EventSeverity } from '../types/api'

const SEV_META: Record<EventSeverity, { label: string; icon: React.ReactNode; rowCls: string; badgeCls: string }> = {
    'cluster-wide': {
        label: 'CLUSTER-WIDE',
        icon: <Globe className="w-3.5 h-3.5" />,
        rowCls: 'border-l-4 border-red-600 bg-red-950/20',
        badgeCls: 'bg-red-900/60 text-red-200 font-bold border border-red-700',
    },
    critical: {
        label: 'PATTERN',
        icon: <Zap className="w-3.5 h-3.5" />,
        rowCls: 'border-l-4 border-amber-600 bg-amber-950/10',
        badgeCls: 'bg-amber-900/50 text-amber-300 font-bold border border-amber-800',
    },
    warning: {
        label: 'WARNING',
        icon: <AlertTriangle className="w-3.5 h-3.5" />,
        rowCls: 'border-l-2 border-amber-700/50',
        badgeCls: 'bg-amber-950/30 text-amber-500 border border-amber-900',
    },
    info: {
        label: 'INFO',
        icon: <Info className="w-3.5 h-3.5" />,
        rowCls: 'border-l-2 border-space-700',
        badgeCls: 'bg-space-800 text-slate-500 border border-space-700',
    },
}

function relAge(iso: string): string {
    const secs = Math.floor((Date.now() - new Date(iso).getTime()) / 1000)
    if (secs < 60) return `${secs}s ago`
    if (secs < 3600) return `${Math.floor(secs / 60)}m ago`
    return `${Math.floor(secs / 3600)}h ago`
}

export function Events() {
    const { context, namespace } = useStore()

    const query = useQuery({
        queryKey: ['cluster-events', context, namespace],
        queryFn: () => api.clusterEvents(context, namespace),
        enabled: !!context,
        refetchInterval: 15_000,
        staleTime: 12_000,
    })

    if (!context) return null
    if (query.isLoading) {
        return <div className="flex-1 flex items-center justify-center"><Loader2 className="w-5 h-5 text-accent animate-spin" /></div>
    }
    if (query.error) {
        return <div className="flex-1 flex items-center justify-center text-xs text-status-unhealthy">{(query.error as Error).message}</div>
    }

    const data = query.data!
    const events = data.events ?? []

    const bySeverity = (s: EventSeverity) => events.filter(e => e.severity === s)
    const clusterWide = bySeverity('cluster-wide')
    const patterns = bySeverity('critical')
    const warnings = bySeverity('warning')
    const infos = bySeverity('info')

    return (
        <div className="flex-1 overflow-y-auto p-6 space-y-5">
            {/* Header */}
            <div className="flex items-center gap-4">
                <div>
                    <h1 className="text-base font-semibold text-slate-100">Cluster Event Intelligence</h1>
                    <p className="text-xs text-slate-600">Last 2 hours · refreshes every 15s · {events.length} unique event group{events.length !== 1 ? 's' : ''}</p>
                </div>
                <div className="ml-auto flex items-center gap-3">
                    {data.patterns > 0 && (
                        <span className="text-[10px] px-2 py-1 rounded border border-red-900/40 bg-red-950/20 text-red-400 font-medium">
                            {data.patterns} active pattern{data.patterns !== 1 ? 's' : ''}
                        </span>
                    )}
                    <button onClick={() => query.refetch()} className="p-1.5 rounded border border-space-700 text-slate-600 hover:text-accent transition-colors">
                        <RefreshCw className={`w-3.5 h-3.5 ${query.isFetching ? 'animate-spin text-accent' : ''}`} />
                    </button>
                </div>
            </div>

            {events.length === 0 && (
                <div className="flex flex-col items-center justify-center py-20 text-center">
                    <div className="text-3xl mb-3">✓</div>
                    <div className="text-sm font-medium text-status-healthy">No events in the last 2 hours</div>
                    <div className="text-xs text-slate-600 mt-1">Cluster is quiet</div>
                </div>
            )}

            {[
                { label: 'Cluster-Wide Issues', items: clusterWide, sev: 'cluster-wide' as EventSeverity },
                { label: 'Rapid-Repeat Patterns', items: patterns, sev: 'critical' as EventSeverity },
                { label: 'Warnings', items: warnings, sev: 'warning' as EventSeverity },
                { label: 'Informational', items: infos, sev: 'info' as EventSeverity },
            ].map(({ label, items, sev }) => {
                if (items.length === 0) return null
                const meta = SEV_META[sev]
                return (
                    <div key={sev}>
                        <div className="flex items-center gap-2 mb-2">
                            <span className={clsx('text-[10px] font-bold tracking-widest uppercase px-2 py-0.5 rounded border flex items-center gap-1.5', meta.badgeCls)}>
                                {meta.icon}{label}
                            </span>
                            <span className="text-[10px] text-slate-600">({items.length})</span>
                            <div className="flex-1 h-px bg-space-700" />
                        </div>
                        <div className="space-y-2">
                            {items.map((ev: AggregatedEvent, i) => <EventCard key={i} ev={ev} meta={meta} />)}
                        </div>
                    </div>
                )
            })}
        </div>
    )
}

function EventCard({ ev, meta }: { ev: AggregatedEvent; meta: typeof SEV_META[EventSeverity] }) {
    return (
        <div className={clsx('rounded-xl bg-space-850 border border-space-700 p-3 space-y-1.5', meta.rowCls)}>
            <div className="flex items-start gap-2">
                <span className={clsx('text-[9px] font-bold tracking-widest uppercase px-1.5 py-0.5 rounded border flex-shrink-0 mt-0.5', meta.badgeCls)}>
                    {ev.reason}
                </span>
                <p className="text-xs text-slate-300 flex-1 leading-snug">{ev.message}</p>
                <span className="text-[10px] text-slate-600 font-mono flex-shrink-0">{relAge(ev.lastSeen)}</span>
            </div>

            {/* Objects affected */}
            <div className="flex flex-wrap gap-1">
                {ev.objects.slice(0, 6).map(obj => (
                    <span key={obj} className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-space-800 border border-space-700 text-slate-500">{obj}</span>
                ))}
                {ev.objects.length > 6 && (
                    <span className="text-[10px] text-slate-600">+{ev.objects.length - 6} more</span>
                )}
            </div>

            {/* Stats row */}
            <div className="flex items-center gap-3 text-[10px] text-slate-600">
                <span>count: <span className="text-slate-400 font-medium">{ev.count}</span></span>
                <span>ns: <span className="text-slate-400 font-mono">{ev.namespace}</span></span>
                {ev.isPattern && <span className="text-amber-500 font-medium">⚡ rapid repeat</span>}
                {ev.isClusterWide && <span className="text-red-400 font-medium">🌐 cluster-wide</span>}
            </div>
        </div>
    )
}
