import { useState, useEffect, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Loader2, RefreshCw, TrendingUp, AlertTriangle, CheckCircle2, BarChart3 } from 'lucide-react'
import clsx from 'clsx'
import { api } from '../api/client'
import { useStore } from '../store/useStore'
import { Sparkline, UsageBar } from '../components/Sparkline'
import type { PodMetricsInfo, Bottleneck } from '../types/api'

// Rolling history of snapshots accumulated client-side.
// Each entry is a map: "namespace/name" → [cpuPct, memPct]
type HistoryEntry = Record<string, { cpuPct: number; memPct: number }>

function fmtBytes(b: number): string {
    if (b === 0) return '0'
    if (b >= 1 << 30) return `${(b / (1 << 30)).toFixed(1)}Gi`
    if (b >= 1 << 20) return `${Math.round(b / (1 << 20))}Mi`
    return `${Math.round(b / 1024)}Ki`
}
function fmtMilli(m: number): string {
    if (m === 0) return '0'
    if (m >= 1000) return `${(m / 1000).toFixed(2)}`
    return `${m}m`
}

const SEV_LABEL: Record<string, string> = {
    over: 'OVER', critical: 'CRITICAL', warning: 'WARNING',
}
const SEV_COLOR: Record<string, string> = {
    over: 'text-red-300 bg-red-950/50 border-red-800',
    critical: 'text-red-400 bg-red-950/30 border-red-900',
    warning: 'text-amber-400 bg-amber-950/30 border-amber-900',
}

export function Metrics() {
    const { context, namespace } = useStore()
    const [history, setHistory] = useState<HistoryEntry[]>([])

    const query = useQuery({
        queryKey: ['metrics', context, namespace],
        queryFn: () => api.metricsSnapshot(context, namespace),
        enabled: !!context,
        refetchInterval: 30_000,
        staleTime: 25_000,
    })

    // Accumulate snapshots into rolling history (max 20 = ~10 minutes at 30s).
    useEffect(() => {
        if (!query.data?.pods) return
        const entry: HistoryEntry = {}
        query.data.pods.forEach(p => {
            entry[`${p.namespace}/${p.name}`] = { cpuPct: p.cpuPct, memPct: p.memPct }
        })
        setHistory(h => [...h.slice(-19), entry])
    }, [query.data])

    const getHistory = useCallback((key: string, resource: 'cpuPct' | 'memPct') =>
        history.map(h => Math.max(0, h[key]?.[resource] ?? 0)),
        [history])

    if (!context) return null
    if (query.isLoading) {
        return (
            <div className="flex-1 flex items-center justify-center">
                <Loader2 className="w-5 h-5 text-accent animate-spin" />
            </div>
        )
    }
    if (query.error) {
        return <div className="flex-1 flex items-center justify-center text-xs text-status-unhealthy">{(query.error as Error).message}</div>
    }

    const data = query.data!
    const bottlenecks = data.bottlenecks ?? []
    const pods = (data.pods ?? []).filter(p => !p.noLimits)

    return (
        <div className="flex-1 overflow-y-auto p-6 space-y-6">
            {/* Header */}
            <div className="flex items-center gap-4">
                <div>
                    <h1 className="text-base font-semibold text-slate-100">Metrics Intelligence</h1>
                    <p className="text-xs text-slate-600">Polling every 30s · {history.length} sample{history.length !== 1 ? 's' : ''} collected</p>
                </div>
                <div className="ml-auto flex items-center gap-3">
                    {!data.available && (
                        <span className="text-[10px] px-2 py-1 rounded border border-amber-900/40 bg-amber-950/20 text-amber-400">
                            metrics-server not installed — limits analysis only
                        </span>
                    )}
                    <button onClick={() => query.refetch()} className="p-1.5 rounded border border-space-700 text-slate-600 hover:text-accent transition-colors">
                        <RefreshCw className={`w-3.5 h-3.5 ${query.isFetching ? 'animate-spin text-accent' : ''}`} />
                    </button>
                </div>
            </div>

            {/* Bottleneck summary */}
            {bottlenecks.length > 0 && (
                <div>
                    <div className="flex items-center gap-2 mb-3">
                        <AlertTriangle className="w-4 h-4 text-status-warning" />
                        <h2 className="text-xs font-semibold uppercase tracking-widest text-status-warning">
                            {bottlenecks.length} Bottleneck{bottlenecks.length !== 1 ? 's' : ''} Detected
                        </h2>
                    </div>
                    <div className="space-y-2">
                        {bottlenecks.map((b: Bottleneck, i) => (
                            <div key={i} className={clsx('rounded-xl border p-3', SEV_COLOR[b.severity] || 'border-space-700')}>
                                <div className="flex items-center gap-2 mb-1.5">
                                    <span className={clsx('text-[9px] font-bold tracking-widest uppercase px-1.5 py-0.5 rounded border', SEV_COLOR[b.severity])}>
                                        {SEV_LABEL[b.severity]}
                                    </span>
                                    <span className="text-xs font-medium text-slate-200">{b.podName}</span>
                                    <span className="text-[10px] text-slate-500 font-mono ml-auto">{b.namespace}</span>
                                    <span className="text-xs font-bold" style={{ color: b.resource === 'cpu' ? '#60a5fa' : '#fbbf24' }}>
                                        {b.resource.toUpperCase()} {b.usagePct.toFixed(0)}%
                                    </span>
                                </div>
                                <p className="text-[11px] text-slate-400">{b.recommendation}</p>
                            </div>
                        ))}
                    </div>
                </div>
            )}

            {/* No-limit pods */}
            {(data.noLimitPods?.length ?? 0) > 0 && (
                <div>
                    <div className="flex items-center gap-2 mb-2">
                        <TrendingUp className="w-4 h-4 text-slate-500" />
                        <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-600">
                            {data.noLimitPods.length} Pod{data.noLimitPods.length !== 1 ? 's' : ''} Without Limits
                        </h2>
                    </div>
                    <div className="flex flex-wrap gap-1.5">
                        {data.noLimitPods.map(p => (
                            <span key={p} className="text-[10px] font-mono px-2 py-0.5 rounded border border-space-700 bg-space-850 text-slate-500">{p}</span>
                        ))}
                    </div>
                </div>
            )}

            {/* Pod sparkline table */}
            {pods.length > 0 && (
                <div>
                    <div className="flex items-center gap-2 mb-3">
                        <BarChart3 className="w-4 h-4 text-accent" />
                        <h2 className="text-xs font-semibold uppercase tracking-widest text-accent/70">Pod Usage</h2>
                        <span className="text-[10px] text-slate-600">({pods.length} with limits)</span>
                    </div>
                    <div className="rounded-xl border border-space-700 overflow-hidden">
                        <table className="w-full text-xs">
                            <thead>
                                <tr className="border-b border-space-700 bg-space-850">
                                    <th className="text-left px-3 py-2 text-slate-500 font-medium">Pod</th>
                                    <th className="text-left px-3 py-2 text-slate-500 font-medium">CPU</th>
                                    <th className="px-3 py-2 text-slate-500 font-medium">Trend</th>
                                    <th className="text-left px-3 py-2 text-slate-500 font-medium">Memory</th>
                                    <th className="px-3 py-2 text-slate-500 font-medium">Trend</th>
                                </tr>
                            </thead>
                            <tbody className="divide-y divide-space-800">
                                {pods.map((p: PodMetricsInfo) => {
                                    const key = `${p.namespace}/${p.name}`
                                    const cpuHist = getHistory(key, 'cpuPct')
                                    const memHist = getHistory(key, 'memPct')
                                    return (
                                        <tr key={key} className="hover:bg-space-850 transition-colors">
                                            <td className="px-3 py-2">
                                                <div className="font-medium text-slate-200 truncate max-w-[160px]">{p.name}</div>
                                                <div className="text-[10px] text-slate-600 font-mono">{p.namespace}</div>
                                            </td>
                                            <td className="px-3 py-2">
                                                <div className="flex flex-col gap-1">
                                                    <UsageBar pct={p.cpuPct} width={70} />
                                                    <span className="text-[10px] text-slate-500 font-mono">
                                                        {fmtMilli(p.cpuMilli)} / {fmtMilli(p.cpuLimitMilli)} ({p.cpuPct >= 0 ? `${p.cpuPct.toFixed(0)}%` : 'no limit'})
                                                    </span>
                                                </div>
                                            </td>
                                            <td className="px-3 py-2">
                                                <Sparkline values={cpuHist.length ? cpuHist : [p.cpuPct]} color="#60a5fa" threshold75 threshold90 />
                                            </td>
                                            <td className="px-3 py-2">
                                                <div className="flex flex-col gap-1">
                                                    <UsageBar pct={p.memPct} width={70} />
                                                    <span className="text-[10px] text-slate-500 font-mono">
                                                        {fmtBytes(p.memBytes)} / {fmtBytes(p.memLimitBytes)} ({p.memPct >= 0 ? `${p.memPct.toFixed(0)}%` : 'no limit'})
                                                    </span>
                                                </div>
                                            </td>
                                            <td className="px-3 py-2">
                                                <Sparkline values={memHist.length ? memHist : [p.memPct]} color="#fbbf24" threshold75 threshold90 />
                                            </td>
                                        </tr>
                                    )
                                })}
                            </tbody>
                        </table>
                    </div>
                </div>
            )}

            {pods.length === 0 && bottlenecks.length === 0 && (data.noLimitPods?.length ?? 0) === 0 && (
                <div className="flex flex-col items-center justify-center py-20 text-center">
                    <CheckCircle2 className="w-12 h-12 text-status-healthy mb-3 opacity-60" />
                    <div className="text-sm font-medium text-slate-300">No metrics data</div>
                    <div className="text-xs text-slate-600 mt-1">Ensure metrics-server is installed in the cluster</div>
                </div>
            )}
        </div>
    )
}
