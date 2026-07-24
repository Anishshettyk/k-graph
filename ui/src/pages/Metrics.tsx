import { useState, useEffect, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Loader2, RefreshCw, TrendingUp, AlertTriangle, CheckCircle2, BarChart3, Scissors, ArrowRight } from 'lucide-react'
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
    const [tab, setTab] = useState<'overview' | 'rightsizing' | 'namespaces'>('overview')

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

    const rsQuery = useQuery({
        queryKey: ['rightsizing', context, namespace],
        queryFn: () => api.rightsizing(context, namespace),
        enabled: !!context && (tab === 'rightsizing' || tab === 'namespaces'),
        staleTime: 30_000,
    })

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
            {/* Header + tabs */}
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

            {/* Tab switcher */}
            <div className="flex gap-1 border-b border-space-700 -mx-6 px-6 pb-0">
                {([
                    { id: 'overview', label: 'Overview', icon: <BarChart3 className="w-3.5 h-3.5" /> },
                    { id: 'rightsizing', label: 'Rightsizing', icon: <Scissors className="w-3.5 h-3.5" /> },
                    { id: 'namespaces', label: 'Namespaces', icon: <TrendingUp className="w-3.5 h-3.5" /> },
                ] as const).map(t => (
                    <button key={t.id} onClick={() => setTab(t.id)}
                        className={clsx(
                            'flex items-center gap-1.5 px-3 py-2 text-xs font-medium border-b-2 -mb-px transition-colors',
                            tab === t.id
                                ? 'border-accent text-accent'
                                : 'border-transparent text-slate-500 hover:text-slate-300'
                        )}>
                        {t.icon}{t.label}
                    </button>
                ))}
            </div>

            {/* ── Overview tab ── */}
            {tab === 'overview' && (<>

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
            </>)}

            {/* ── Rightsizing tab ── */}
            {tab === 'rightsizing' && (
                <RightsizingTab rsQuery={rsQuery} />
            )}

            {/* ── Namespaces tab ── */}
            {tab === 'namespaces' && (
                <NamespacesTab rsQuery={rsQuery} />
            )}
        </div>
    )
}

// ─── Rightsizing Tab ─────────────────────────────────────────────────────────

const CONF_META = {
    low:    { cls: 'text-slate-600 border-slate-800 bg-space-800',   label: 'LOW confidence — collecting data' },
    medium: { cls: 'text-amber-500 border-amber-900/40 bg-amber-950/20', label: 'MEDIUM confidence — short window' },
    high:   { cls: 'text-emerald-400 border-emerald-900/40 bg-emerald-950/20', label: 'HIGH confidence' },
}

const SEV_META: Record<string, { label: string; cls: string }> = {
    waste:     { label: 'WASTE',  cls: 'text-amber-400 border-amber-900/40 bg-amber-950/30' },
    'under-req':  { label: 'UNDER',  cls: 'text-red-400 border-red-900/40 bg-red-950/30' },
    spiky:     { label: 'SPIKY',  cls: 'text-violet-400 border-violet-900/40 bg-violet-950/30' },
}

function fmtCPU(m: number) { return m >= 1000 ? `${(m/1000).toFixed(2)}` : `${m}m` }
function fmtMem(b: number) {
    if (b >= 1<<30) return `${(b/(1<<30)).toFixed(1)}Gi`
    if (b >= 1<<20) return `${Math.round(b/(1<<20))}Mi`
    return `${Math.round(b/1024)}Ki`
}

// Range bar: shows min / avg / p95 / max relative to the request
function RangeBar({ stats, request, resource }: {
    stats: import('../types/api').RangeStats
    request: number
    resource: 'cpu' | 'memory'
}) {
    if (!request || stats.samples === 0) return null
    const pct = (v: number) => Math.min(120, (v / request) * 100)
    const fmt = resource === 'cpu' ? fmtCPU : fmtMem

    return (
        <div className="space-y-1.5">
            {/* Track */}
            <div className="relative h-3 bg-space-800 rounded-full overflow-visible">
                {/* Request line (100%) */}
                <div className="absolute top-0 bottom-0 w-px bg-slate-600 z-10" style={{ left: '83.3%' }}
                    title="Request" />
                {/* Min → Max range bar */}
                <div className="absolute top-0.5 bottom-0.5 rounded-full bg-blue-900/40"
                    style={{ left: `${pct(stats.min)}%`, right: `${100 - pct(stats.max)}%` }} />
                {/* Avg dot */}
                <div className="absolute top-0.5 bottom-0.5 w-1 rounded-full bg-blue-400 z-10"
                    style={{ left: `${pct(stats.avg) - 0.25}%` }} title={`avg: ${fmt(stats.avg)}`} />
                {/* P95 dot */}
                <div className="absolute top-0 bottom-0 w-1.5 rounded-full bg-amber-400 z-20"
                    style={{ left: `${pct(stats.p95) - 0.375}%` }} title={`p95: ${fmt(stats.p95)}`} />
            </div>
            {/* Legend */}
            <div className="flex items-center gap-3 text-[9px]">
                <span className="text-slate-600">min <span className="text-slate-400 font-mono">{fmt(stats.min)}</span></span>
                <span className="text-slate-600">avg <span className="text-blue-400 font-mono">{fmt(stats.avg)}</span></span>
                <span className="text-slate-600">p95 <span className="text-amber-400 font-mono">{fmt(stats.p95)}</span></span>
                <span className="text-slate-600">max <span className="text-slate-400 font-mono">{fmt(stats.max)}</span></span>
                <span className="ml-auto text-slate-700">req <span className="font-mono">{fmt(request)}</span></span>
            </div>
        </div>
    )
}

function SizingRow({ p }: { p: import('../types/api').PodSizing }) {
    const cpuSev = p.cpu.severity !== 'ok'
    const memSev = p.memory.severity !== 'ok'
    const conf = p.cpu.confidence ?? 'low'
    const confMeta = CONF_META[conf]

    const borderCls =
        p.cpu.severity === 'waste' || p.memory.severity === 'waste' ? 'border-amber-900/30 bg-amber-950/5' :
        p.cpu.severity === 'under-req' || p.memory.severity === 'under-req' ? 'border-red-900/30 bg-red-950/5' :
        p.cpu.severity === 'spiky' || p.memory.severity === 'spiky' ? 'border-violet-900/30 bg-violet-950/5' :
        'border-space-700 bg-space-900'

    return (
        <div className={clsx('rounded-xl border p-3 space-y-3', borderCls)}>
            {/* Pod header */}
            <div className="flex items-start gap-2">
                <div className="flex-1 min-w-0">
                    <div className="text-xs font-mono text-slate-300 truncate">{p.namespace}/{p.name}</div>
                    {p.workload && <div className="text-[9px] text-slate-600 font-mono mt-0.5">{p.workload}</div>}
                </div>
                {/* Confidence badge */}
                <span className={clsx('text-[8px] font-bold uppercase tracking-widest px-1.5 py-0.5 rounded border flex-shrink-0', confMeta.cls)}>
                    {conf === 'low' ? `${p.cpu.stats.samples ?? 0} samples` : `${conf} · ${p.cpu.stats.samples ?? 0}s`}
                </span>
            </div>

            {/* CPU section */}
            {p.cpu.stats.samples > 0 && (
                <div className="space-y-1.5">
                    <div className="flex items-center gap-2">
                        <span className="text-[9px] font-bold uppercase tracking-widest text-slate-500">CPU</span>
                        {cpuSev && SEV_META[p.cpu.severity] && (
                            <span className={clsx('text-[8px] font-bold uppercase px-1.5 py-0.5 rounded border', SEV_META[p.cpu.severity].cls)}>
                                {SEV_META[p.cpu.severity].label}
                            </span>
                        )}
                    </div>
                    <RangeBar stats={p.cpu.stats} request={p.cpu.request} resource="cpu" />
                    {(cpuSev || conf !== 'low') && (
                        <p className="text-[10px] text-slate-400 leading-snug">{p.cpu.recommendation}</p>
                    )}
                </div>
            )}

            {/* Memory section */}
            {p.memory.stats.samples > 0 && (
                <div className="space-y-1.5">
                    <div className="flex items-center gap-2">
                        <span className="text-[9px] font-bold uppercase tracking-widest text-slate-500">MEMORY</span>
                        {memSev && SEV_META[p.memory.severity] && (
                            <span className={clsx('text-[8px] font-bold uppercase px-1.5 py-0.5 rounded border', SEV_META[p.memory.severity].cls)}>
                                {SEV_META[p.memory.severity].label}
                            </span>
                        )}
                    </div>
                    <RangeBar stats={p.memory.stats} request={p.memory.request} resource="memory" />
                    {(memSev || conf !== 'low') && (
                        <p className="text-[10px] text-slate-400 leading-snug">{p.memory.recommendation}</p>
                    )}
                </div>
            )}
        </div>
    )
}

function RightsizingTab({ rsQuery }: { rsQuery: ReturnType<typeof useQuery> }) {
    const data = rsQuery.data as import('../types/api').RightsizingResponse | undefined
    if (rsQuery.isLoading) return <div className="flex justify-center py-12"><Loader2 className="w-5 h-5 text-accent animate-spin" /></div>
    if (rsQuery.error) return <div className="text-xs text-status-unhealthy">{(rsQuery.error as Error).message}</div>
    if (!data) return null

    const pods = data.pods ?? []
    const wasted  = pods.filter(p => p.cpu.severity === 'waste'     || p.memory.severity === 'waste')
    const under   = pods.filter(p => p.cpu.severity === 'under-req' || p.memory.severity === 'under-req')
    const spiky   = pods.filter(p => p.cpu.severity === 'spiky'     || p.memory.severity === 'spiky')
    const ok      = pods.filter(p => !['waste','under-req','spiky'].includes(p.cpu.severity) && !['waste','under-req','spiky'].includes(p.memory.severity))

    const typical = data.typicalSamples ?? 0
    const window  = data.windowMinutes  ?? 0

    return (
        <div className="space-y-5">
            {!data.available && (
                <div className="rounded-xl border border-amber-900/30 bg-amber-950/10 p-3 text-xs text-amber-400">
                    metrics-server not available — rightsizing requires live usage data
                </div>
            )}

            {/* Data quality notice */}
            <div className={clsx(
                'rounded-xl border p-3 text-[10px] flex items-start gap-2',
                typical < 3  ? 'border-slate-800 bg-space-900 text-slate-500' :
                typical < 10 ? 'border-amber-900/30 bg-amber-950/5 text-amber-500' :
                               'border-emerald-900/30 bg-emerald-950/5 text-emerald-400'
            )}>
                <span className="flex-shrink-0 font-bold">
                    {typical < 3 ? '⏳' : typical < 10 ? '◑' : '✓'}
                </span>
                <div>
                    {typical < 3
                        ? `Collecting data — ${typical} sample${typical !== 1 ? 's' : ''} gathered so far. Leave this tab open for ~15 minutes for reliable recommendations.`
                        : typical < 10
                            ? `${typical} samples over ~${window.toFixed(0)}m — recommendations are tentative. Continue monitoring for higher confidence.`
                            : `${typical} samples over ~${window.toFixed(0)}m — recommendations are based on a solid usage pattern.`
                    }
                    <span className="text-slate-600 ml-1">Range bar: <span className="text-blue-400">avg</span> · <span className="text-amber-400">p95</span> · min/max</span>
                </div>
            </div>

            {/* Summary counts */}
            <div className="grid grid-cols-4 gap-3 text-xs">
                {[
                    { n: wasted.length,  color: 'text-amber-400',   bg: 'border-amber-900/30 bg-amber-950/10',  label: 'Over-prov.' },
                    { n: under.length,   color: 'text-red-400',     bg: 'border-red-900/30 bg-red-950/10',      label: 'Under-prov.' },
                    { n: spiky.length,   color: 'text-violet-400',  bg: 'border-violet-900/30 bg-violet-950/10', label: 'Spiky' },
                    { n: ok.length,      color: 'text-emerald-400', bg: 'border-emerald-900/30 bg-emerald-950/10', label: 'Well-sized' },
                ].map(({ n, color, bg, label }) => (
                    <div key={label} className={clsx('rounded-xl border p-3 text-center', bg)}>
                        <div className={clsx('text-xl font-bold', color)}>{n}</div>
                        <div className="text-slate-600 mt-0.5 text-[10px]">{label}</div>
                    </div>
                ))}
            </div>

            {wasted.length > 0 && (
                <Section label="Over-Provisioned" color="text-amber-500" pods={wasted} />
            )}
            {under.length > 0 && (
                <Section label="Under-Provisioned" color="text-red-400" pods={under} />
            )}
            {spiky.length > 0 && (
                <Section label="Spiky (high variance)" color="text-violet-400" pods={spiky} />
            )}
            {ok.length > 0 && (
                <Section label="Well-Sized" color="text-slate-500" pods={ok} />
            )}
            {pods.length === 0 && (
                <div className="text-center py-12 text-slate-600 text-sm">No pods with resource requests found</div>
            )}
        </div>
    )
}

function Section({ label, color, pods }: { label: string; color: string; pods: import('../types/api').PodSizing[] }) {
    return (
        <div className="space-y-2">
            <div className={clsx('text-[10px] font-semibold uppercase tracking-widest', color)}>{label} ({pods.length})</div>
            {pods.map(p => <SizingRow key={`${p.namespace}/${p.name}`} p={p} />)}
        </div>
    )
}

// ─── Namespaces Tab ───────────────────────────────────────────────────────────

function NamespacesTab({ rsQuery }: { rsQuery: ReturnType<typeof useQuery> }) {
    const data = rsQuery.data as import('../types/api').RightsizingResponse | undefined
    if (rsQuery.isLoading) return <div className="flex justify-center py-12"><Loader2 className="w-5 h-5 text-accent animate-spin" /></div>
    if (rsQuery.error) return <div className="text-xs text-status-unhealthy">{(rsQuery.error as Error).message}</div>
    if (!data) return null

    const nsList = data.namespaceBreakdown ?? []
    if (nsList.length === 0) return <div className="text-center py-12 text-slate-600 text-sm">No namespace data available</div>

    const maxCPU = Math.max(...nsList.map(n => n.cpuRequestMilli), 1)
    const maxMem = Math.max(...nsList.map(n => n.memRequestBytes), 1)

    return (
        <div className="space-y-3">
            <p className="text-xs text-slate-600">CPU and memory requests by namespace — shows cluster resource allocation distribution</p>
            {nsList.map((ns: import('../types/api').NSResourceSummary) => {
                const cpuPct = (ns.cpuRequestMilli / maxCPU) * 100
                const memPct = (ns.memRequestBytes / maxMem) * 100
                const cpuUsePct = ns.cpuRequestMilli > 0 ? (ns.cpuUsedMilli / ns.cpuRequestMilli) * 100 : 0
                const memUsePct = ns.memRequestBytes > 0 ? (ns.memUsedBytes / ns.memRequestBytes) * 100 : 0
                return (
                    <div key={ns.namespace} className="rounded-xl border border-space-700 bg-space-900 p-4 space-y-3">
                        <div className="flex items-center gap-2">
                            <span className="text-sm font-mono font-medium text-slate-200">{ns.namespace}</span>
                            <span className="text-[10px] text-slate-600 ml-auto">{ns.podCount} pod{ns.podCount !== 1 ? 's' : ''}</span>
                        </div>
                        {/* CPU */}
                        <div className="space-y-1">
                            <div className="flex items-center justify-between text-[10px] text-slate-500">
                                <span>CPU request</span>
                                <span className="font-mono">{ns.cpuRequestMilli}m
                                    {data.available && ns.cpuUsedMilli > 0 && (
                                        <span className="text-slate-600"> · {cpuUsePct.toFixed(0)}% used</span>
                                    )}
                                </span>
                            </div>
                            <div className="relative h-2 bg-space-800 rounded-full overflow-hidden">
                                <div className="absolute h-full bg-blue-700/40 rounded-full" style={{ width: `${cpuPct}%` }} />
                                {data.available && ns.cpuUsedMilli > 0 && (
                                    <div className="absolute h-full bg-blue-400 rounded-full" style={{ width: `${Math.min(cpuPct * cpuUsePct / 100, 100)}%` }} />
                                )}
                            </div>
                        </div>
                        {/* Memory */}
                        <div className="space-y-1">
                            <div className="flex items-center justify-between text-[10px] text-slate-500">
                                <span>Memory request</span>
                                <span className="font-mono">{fmtBytes2(ns.memRequestBytes)}
                                    {data.available && ns.memUsedBytes > 0 && (
                                        <span className="text-slate-600"> · {memUsePct.toFixed(0)}% used</span>
                                    )}
                                </span>
                            </div>
                            <div className="relative h-2 bg-space-800 rounded-full overflow-hidden">
                                <div className="absolute h-full bg-amber-700/40 rounded-full" style={{ width: `${memPct}%` }} />
                                {data.available && ns.memUsedBytes > 0 && (
                                    <div className="absolute h-full bg-amber-400 rounded-full" style={{ width: `${Math.min(memPct * memUsePct / 100, 100)}%` }} />
                                )}
                            </div>
                        </div>
                    </div>
                )
            })}
        </div>
    )
}

function fmtBytes2(b: number): string {
    if (b === 0) return '0'
    if (b >= 1 << 30) return `${(b / (1 << 30)).toFixed(1)}Gi`
    if (b >= 1 << 20) return `${Math.round(b / (1 << 20))}Mi`
    return `${Math.round(b / 1024)}Ki`
}

// suppress unused import warning
void ArrowRight
