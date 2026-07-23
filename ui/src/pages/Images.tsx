import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Loader2, RefreshCw, AlertTriangle, CheckCircle2, Tag, Server, Package } from 'lucide-react'
import clsx from 'clsx'
import { api } from '../api/client'
import { useStore } from '../store/useStore'
import type { ImageInfo, ImageRisk } from '../types/api'

const RISK_META: Record<ImageRisk, { label: string; cls: string }> = {
    'latest-tag': { label: ':latest', cls: 'bg-red-950/40 text-red-400 border-red-900/40' },
    'no-tag': { label: 'no tag', cls: 'bg-amber-950/40 text-amber-400 border-amber-900/40' },
    'unknown-reg': { label: 'unknown registry', cls: 'bg-violet-950/40 text-violet-400 border-violet-900/40' },
}

function parseTag(_image: string, tag: string): { label: string; safe: boolean } {
    if (!tag) return { label: 'untagged', safe: false }
    if (tag === 'latest') return { label: ':latest', safe: false }
    // semver-ish or sha looks safe
    return { label: tag, safe: true }
}

export function Images() {
    const { context, namespace } = useStore()
    const [filter, setFilter] = useState('')

    const query = useQuery({
        queryKey: ['images', context, namespace],
        queryFn: () => api.images(context, namespace),
        enabled: !!context,
        staleTime: 60_000,
        refetchInterval: 60_000,
    })

    if (!context) return null
    if (query.isLoading) return <div className="flex-1 flex items-center justify-center"><Loader2 className="w-5 h-5 text-accent animate-spin" /></div>
    if (query.error) return <div className="flex-1 flex items-center justify-center text-xs text-status-unhealthy">{(query.error as Error).message}</div>

    const data = query.data!
    const lower = filter.toLowerCase()
    const images = (data.images ?? []).filter(img =>
        !lower || img.image.toLowerCase().includes(lower) ||
        img.workloads.some(w => w.name.toLowerCase().includes(lower))
    )

    const risky = images.filter(i => i.risks.length > 0)
    const safe = images.filter(i => i.risks.length === 0)

    return (
        <div className="flex-1 overflow-y-auto p-6 space-y-5">
            {/* Header */}
            <div className="flex items-center gap-4">
                <div className="w-9 h-9 rounded-xl bg-violet-500/10 border border-violet-500/20 flex items-center justify-center flex-shrink-0">
                    <Package className="w-5 h-5 text-violet-400" />
                </div>
                <div>
                    <h1 className="text-base font-semibold text-slate-100">Container Image Inventory</h1>
                    <p className="text-xs text-slate-600">{data.total} unique images · {data.withRisks} with risk flags</p>
                </div>
                <div className="ml-auto flex items-center gap-3">
                    {data.latestTags > 0 && (
                        <span className="text-[10px] px-2 py-1 rounded border border-red-900/40 bg-red-950/20 text-red-400 font-medium flex items-center gap-1">
                            <AlertTriangle className="w-3 h-3" /> {data.latestTags} :latest tag{data.latestTags !== 1 ? 's' : ''}
                        </span>
                    )}
                    <button onClick={() => query.refetch()} className="p-1.5 rounded border border-space-700 text-slate-600 hover:text-accent transition-colors">
                        <RefreshCw className={`w-3.5 h-3.5 ${query.isFetching ? 'animate-spin text-accent' : ''}`} />
                    </button>
                </div>
            </div>

            {/* Search */}
            <input
                type="text"
                placeholder="Filter by image name or workload…"
                value={filter}
                onChange={e => setFilter(e.target.value)}
                className="w-full bg-space-800 border border-space-700 rounded-xl px-4 py-2.5 text-sm text-slate-300 placeholder-slate-600 focus:outline-none focus:border-accent/60 transition-colors"
            />

            {/* Risky images */}
            {risky.length > 0 && (
                <section className="space-y-2">
                    <div className="flex items-center gap-2">
                        <AlertTriangle className="w-3.5 h-3.5 text-amber-500" />
                        <span className="text-xs font-semibold uppercase tracking-widest text-amber-500">Flagged ({risky.length})</span>
                        <div className="flex-1 h-px bg-amber-900/30" />
                    </div>
                    {risky.map(img => <ImageCard key={img.image} img={img} />)}
                </section>
            )}

            {/* Safe images */}
            {safe.length > 0 && (
                <section className="space-y-2">
                    <div className="flex items-center gap-2">
                        <CheckCircle2 className="w-3.5 h-3.5 text-status-healthy" />
                        <span className="text-xs font-semibold uppercase tracking-widest text-slate-500">Clean ({safe.length})</span>
                        <div className="flex-1 h-px bg-space-700" />
                    </div>
                    {safe.map(img => <ImageCard key={img.image} img={img} />)}
                </section>
            )}

            {images.length === 0 && (
                <div className="text-center py-20 text-slate-600 text-sm">No images match "{filter}"</div>
            )}
        </div>
    )
}

function ImageCard({ img }: { img: ImageInfo }) {
    const [open, setOpen] = useState(false)
    const tagInfo = parseTag(img.image, img.tag)

    return (
        <div className={clsx(
            'rounded-xl border bg-space-900 overflow-hidden',
            img.risks.length > 0 ? 'border-amber-900/30' : 'border-space-700'
        )}>
            <button
                onClick={() => setOpen(o => !o)}
                className="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-space-800/40 transition-colors"
            >
                <div className="flex-shrink-0 w-8 h-8 rounded-lg bg-space-800 border border-space-700 flex items-center justify-center">
                    <Server className="w-4 h-4 text-slate-500" />
                </div>
                <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 flex-wrap">
                        <span className="text-sm font-mono text-slate-200 truncate">{img.registry}/{img.repository}</span>
                        <span className={clsx(
                            'text-[10px] font-mono px-1.5 py-0.5 rounded border flex-shrink-0 flex items-center gap-1',
                            tagInfo.safe
                                ? 'bg-emerald-950/30 text-emerald-400 border-emerald-900/30'
                                : 'bg-red-950/30 text-red-400 border-red-900/30'
                        )}>
                            <Tag className="w-2.5 h-2.5" />{tagInfo.label}
                        </span>
                        {img.risks.map(r => (
                            <span key={r} className={clsx('text-[9px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border', RISK_META[r].cls)}>
                                {RISK_META[r].label}
                            </span>
                        ))}
                    </div>
                    <div className="text-[10px] text-slate-600 font-mono mt-0.5">{img.image}</div>
                </div>
                <span className="text-[10px] text-slate-600 flex-shrink-0">{img.podCount} pod{img.podCount !== 1 ? 's' : ''}</span>
            </button>

            {open && (
                <div className="border-t border-space-700 px-4 py-3 space-y-2">
                    <div className="text-[10px] font-semibold uppercase tracking-widest text-slate-500 mb-1.5">Used by</div>
                    <div className="flex flex-wrap gap-1.5">
                        {img.workloads.map(w => (
                            <span key={`${w.namespace}/${w.name}`} className="text-[10px] font-mono px-2 py-1 rounded bg-space-800 border border-space-700 text-slate-400">
                                {w.kind}/{w.namespace}/{w.name}
                            </span>
                        ))}
                    </div>
                </div>
            )}
        </div>
    )
}
