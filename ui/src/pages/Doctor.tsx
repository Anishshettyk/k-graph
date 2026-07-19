import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { api } from '../api/client'
import { useStore } from '../store/useStore'
import type { FindingInfo } from '../types/api'

const CATEGORY_ICONS: Record<string, string> = {
    'High Availability': '⚖',
    'Reliability': '⟳',
    'Security': '⊘',
    'Storage': '▥',
    'Network': '◈',
}

export function Doctor() {
    const { context, namespace } = useStore()

    const query = useQuery({
        queryKey: ['doctor', context, namespace],
        queryFn: () => api.doctor(context, namespace),
        enabled: !!context,
        staleTime: 30_000,
    })

    if (query.isLoading) return <LoadingState />
    if (query.error) return <ErrorState message={(query.error as Error).message} />
    const data = query.data!

    const blocking = data.findings.filter(f => f.severity === 'blocking')
    const warnings = data.findings.filter(f => f.severity === 'warning')

    // Group by category
    const byCategory = data.findings.reduce<Record<string, FindingInfo[]>>((acc, f) => {
        if (!acc[f.category]) acc[f.category] = []
        acc[f.category].push(f)
        return acc
    }, {})

    return (
        <div className="flex-1 overflow-y-auto p-6">
            {/* Stats row */}
            <div className="flex items-center gap-4 mb-6">
                <h1 className="text-base font-semibold text-slate-100">Doctor — Proactive Health Scan</h1>
                <span className="text-xs text-slate-600 font-mono">{context}</span>
                <div className="ml-auto flex gap-3">
                    {blocking.length > 0 && (
                        <span className="text-xs px-2 py-1 rounded bg-red-950/40 border border-red-900/40 text-status-unhealthy">
                            ✖ {blocking.length} blocking
                        </span>
                    )}
                    {warnings.length > 0 && (
                        <span className="text-xs px-2 py-1 rounded bg-amber-950/40 border border-amber-900/40 text-status-warning">
                            ⚠ {warnings.length} warnings
                        </span>
                    )}
                    {data.findings.length === 0 && (
                        <span className="text-xs px-2 py-1 rounded bg-emerald-950/40 border border-emerald-900/40 text-status-healthy">
                            ✓ All clear
                        </span>
                    )}
                </div>
            </div>

            {data.findings.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-20 text-center">
                    <div className="text-4xl mb-3 text-status-healthy">✓</div>
                    <div className="text-sm font-medium text-slate-300">No issues found</div>
                    <div className="text-xs text-slate-600 mt-1">
                        {namespace ? `namespace: ${namespace}` : 'all namespaces'} looks healthy
                    </div>
                </div>
            ) : (
                <div className="space-y-6">
                    {Object.entries(byCategory).map(([category, findings]) => (
                        <div key={category}>
                            <div className="flex items-center gap-2 mb-3">
                                <span className="text-slate-400">{CATEGORY_ICONS[category] ?? '•'}</span>
                                <h2 className="text-xs font-semibold uppercase tracking-widest text-accent/70">{category}</h2>
                                <div className="flex-1 h-px bg-space-700" />
                            </div>
                            <div className="space-y-2">
                                {findings.map((f, i) => (
                                    <FindingCard key={i} finding={f} />
                                ))}
                            </div>
                        </div>
                    ))}
                </div>
            )}
        </div>
    )
}

function FindingCard({ finding: f }: { finding: FindingInfo }) {
    const isBlocking = f.severity === 'blocking'
    return (
        <div className={`rounded-lg border p-3 ${isBlocking
                ? 'border-red-900/50 bg-red-950/10'
                : 'border-amber-900/30 bg-amber-950/5'
            }`}>
            <div className="flex items-start gap-2">
                <span className={isBlocking ? 'text-status-unhealthy' : 'text-status-warning'}>
                    {isBlocking ? '✖' : '⚠'}
                </span>
                <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 flex-wrap">
                        <span className="text-xs font-medium text-slate-200">{f.kind} {f.namespace}/{f.name}</span>
                    </div>
                    <div className="text-xs text-slate-300 mt-0.5">{f.issue}</div>
                    {f.detail && <div className="text-[11px] text-slate-500 mt-0.5">{f.detail}</div>}
                    {f.fix && (
                        <div className="text-[11px] mt-1.5">
                            <span className="text-status-healthy">fix:</span>{' '}
                            <span className="text-slate-400">{f.fix}</span>
                        </div>
                    )}
                </div>
            </div>
        </div>
    )
}

function LoadingState() {
    return (
        <div className="flex-1 flex items-center justify-center">
            <Loader2 className="w-5 h-5 text-accent animate-spin" />
        </div>
    )
}

function ErrorState({ message }: { message: string }) {
    return (
        <div className="flex-1 flex items-center justify-center">
            <div className="text-xs text-status-unhealthy">{message}</div>
        </div>
    )
}
