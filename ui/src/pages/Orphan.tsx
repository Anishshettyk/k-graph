import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { api } from '../api/client'
import { useStore } from '../store/useStore'
import { kindIcon, kindColor } from '../lib/kinds'

export function Orphan() {
    const { context, namespace } = useStore()

    const query = useQuery({
        queryKey: ['orphan', context, namespace],
        queryFn: () => api.orphan(context, namespace),
        enabled: !!context,
        staleTime: 30_000,
    })

    if (query.isLoading) {
        return <div className="flex-1 flex items-center justify-center"><Loader2 className="w-5 h-5 text-accent animate-spin" /></div>
    }
    if (query.error) {
        return <div className="flex-1 flex items-center justify-center text-xs text-status-unhealthy">{(query.error as Error).message}</div>
    }

    const data = query.data!
    const orphans = data.orphans ?? []

    // Group by kind
    const byKind = orphans.reduce<Record<string, typeof orphans>>((acc, o) => {
        if (!acc[o.node.kind]) acc[o.node.kind] = []
        acc[o.node.kind].push(o)
        return acc
    }, {})

    return (
        <div className="flex-1 overflow-y-auto p-6">
            <div className="flex items-center gap-4 mb-6">
                <h1 className="text-base font-semibold text-slate-100">Orphaned Resources</h1>
                <span className="text-xs text-slate-600 font-mono">{context}</span>
                <div className="ml-auto">
                    {orphans.length === 0 ? (
                        <span className="text-xs px-2 py-1 rounded bg-emerald-950/40 border border-emerald-900/40 text-status-healthy">
                            ✓ No orphans
                        </span>
                    ) : (
                        <span className="text-xs px-2 py-1 rounded bg-amber-950/40 border border-amber-900/40 text-status-warning">
                            ◌ {orphans.length} orphaned
                        </span>
                    )}
                </div>
            </div>

            {orphans.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-20 text-center">
                    <div className="text-4xl mb-3 text-status-healthy">✓</div>
                    <div className="text-sm font-medium text-slate-300">No orphaned resources found</div>
                    <div className="text-xs text-slate-600 mt-1">All ConfigMaps, Secrets, PVCs, Services, and ServiceAccounts are referenced</div>
                </div>
            ) : (
                <div className="space-y-6">
                    {Object.entries(byKind).map(([kind, items]) => (
                        <div key={kind}>
                            <div className="flex items-center gap-2 mb-3">
                                {(() => { const Icon = kindIcon(kind); return Icon && <Icon className="w-4 h-4" style={{ color: kindColor(kind) }} />; })()}
                                <h2 className="text-xs font-semibold uppercase tracking-widest" style={{ color: kindColor(kind) }}>
                                    {kind}
                                </h2>
                                <span className="text-xs text-slate-600">({items.length})</span>
                                <div className="flex-1 h-px bg-space-700" />
                            </div>
                            <div className="space-y-2">
                                {items.map((o, i) => (
                                    <div key={i} className="flex items-center gap-3 rounded-lg border border-space-700 bg-space-850 px-3 py-2 hover:border-space-600 transition-colors">
                                        <div className="flex-shrink-0 flex items-center justify-center w-5 h-5">
                                            {(() => { const Icon = kindIcon(o.node.kind); return Icon && <Icon className="w-4 h-4" style={{ color: kindColor(o.node.kind) }} />; })()}
                                        </div>
                                        <div className="min-w-0 flex-1">
                                            <div className="text-xs font-medium text-slate-200 font-mono">{o.node.name}</div>
                                            {o.node.namespace && (
                                                <div className="text-[10px] text-slate-600 font-mono">{o.node.namespace}</div>
                                            )}
                                        </div>
                                        <div className="text-[10px] text-slate-500 text-right max-w-[200px]">{o.reason}</div>
                                    </div>
                                ))}
                            </div>
                        </div>
                    ))}
                </div>
            )}
        </div>
    )
}
