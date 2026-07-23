import { useState, useRef, useCallback, useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, RefreshCw, Activity, Search, Thermometer } from 'lucide-react'
import clsx from 'clsx'
import { api } from '../api/client'
import { useStore } from '../store/useStore'
import { Sidebar } from '../components/Sidebar'
import { GraphCanvas } from '../components/GraphCanvas'
import { DetailsPanel } from '../components/DetailsPanel'
import { SpotlightSearch } from '../components/SpotlightSearch'
import { ContextMenu } from '../components/ContextMenu'
import type { NodeInfo } from '../types/api'

// Formats a relative time string ("2m ago", "just now", etc.)
function relativeTime(date: Date): string {
    const secs = Math.floor((Date.now() - date.getTime()) / 1000)
    if (secs < 10) return 'just now'
    if (secs < 60) return `${secs}s ago`
    if (secs < 3600) return `${Math.floor(secs / 60)}m ago`
    return `${Math.floor(secs / 3600)}h ago`
}

export function Explorer() {
    const { context, namespace, selectedNode, setSelectedNode } = useStore()
    const queryClient = useQueryClient()
    const [filterKind, setFilterKind] = useState('')
    const [detailsWidth, setDetailsWidth] = useState(340)
    const [heatmapMode, setHeatmapMode] = useState(false)
    const [isLive, setIsLive] = useState(false)
    const [lastRefresh, setLastRefresh] = useState(new Date())
    const [relTime, setRelTime] = useState('just now')
    const [showSpotlight, setShowSpotlight] = useState(false)
    const [contextMenu, setContextMenu] = useState<{ x: number; y: number; node: NodeInfo } | null>(null)
    const [detailsTab, setDetailsTab] = useState<'why' | 'logs' | 'yaml' | 'events'>('why')
    const isResizing = useRef(false)

    // ── Relative-time ticker ─────────────────────────────────────────────────
    useEffect(() => {
        const id = setInterval(() => setRelTime(relativeTime(lastRefresh)), 5_000)
        return () => clearInterval(id)
    }, [lastRefresh])

    // ── Auto-refresh interval ────────────────────────────────────────────────
    useEffect(() => {
        if (!isLive || !context) return
        const id = setInterval(() => {
            queryClient.invalidateQueries({ queryKey: ['graph', context, namespace] })
            setLastRefresh(new Date())
        }, 30_000)
        return () => clearInterval(id)
    }, [isLive, context, namespace, queryClient])

    // ── Ctrl/Cmd+K → spotlight ───────────────────────────────────────────────
    useEffect(() => {
        const handler = (e: KeyboardEvent) => {
            if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
                e.preventDefault()
                setShowSpotlight(s => !s)
            }
        }
        window.addEventListener('keydown', handler)
        return () => window.removeEventListener('keydown', handler)
    }, [])

    const handleSelect = useCallback((node: NodeInfo) => {
        setSelectedNode(node)
        setContextMenu(null)
    }, [setSelectedNode])

    const startResize = useCallback((e: React.MouseEvent) => {
        isResizing.current = true
        const startX = e.clientX
        const startW = detailsWidth
        const onMove = (ev: MouseEvent) => {
            if (!isResizing.current) return
            setDetailsWidth(Math.max(280, Math.min(700, startW + (startX - ev.clientX))))
        }
        const onUp = () => {
            isResizing.current = false
            window.removeEventListener('mousemove', onMove)
            window.removeEventListener('mouseup', onUp)
        }
        window.addEventListener('mousemove', onMove)
        window.addEventListener('mouseup', onUp)
        e.preventDefault()
    }, [detailsWidth])

    const graphQuery = useQuery({
        queryKey: ['graph', context, namespace],
        queryFn: () => { setLastRefresh(new Date()); setRelTime('just now'); return api.graph(context, namespace) },
        enabled: !!context,
        staleTime: 30_000,
    })

    const handleRefresh = useCallback(() => {
        // Pass refresh=true so the server bypasses its cache and does a fresh sweep
        queryClient.fetchQuery({
            queryKey: ['graph', context, namespace],
            queryFn: () => { setLastRefresh(new Date()); setRelTime('just now'); return api.graph(context, namespace, true) },
        })
    }, [context, namespace, queryClient])

    if (!context) {
        return (
            <div className="flex-1 flex items-center justify-center text-slate-600 text-sm">
                <div className="text-center space-y-2">
                    <div className="text-4xl text-accent">⬡</div>
                    <div>Loading cluster contexts…</div>
                    <Loader2 className="w-5 h-5 text-accent animate-spin mx-auto" />
                </div>
            </div>
        )
    }

    if (graphQuery.isLoading) {
        return (
            <div className="flex-1 flex items-center justify-center">
                <div className="text-center space-y-3">
                    <div className="text-5xl text-accent">⬡</div>
                    <div className="text-sm text-slate-400">Building cluster graph…</div>
                    <div className="text-xs text-slate-600 font-mono">{context}</div>
                    <Loader2 className="w-5 h-5 text-accent animate-spin mx-auto" />
                </div>
            </div>
        )
    }

    if (graphQuery.error) {
        return (
            <div className="flex-1 flex items-center justify-center">
                <div className="text-center space-y-2 max-w-sm">
                    <div className="text-status-unhealthy text-sm font-medium">Failed to load graph</div>
                    <div className="text-xs text-slate-500">{(graphQuery.error as Error).message}</div>
                    <button onClick={() => graphQuery.refetch()} className="text-xs text-accent hover:text-accent-bright">retry</button>
                </div>
            </div>
        )
    }

    const data = graphQuery.data!
    const stale = (Date.now() - lastRefresh.getTime()) > 120_000  // amber after 2m
    const truncated = data.truncated && data.total > data.nodes.length

    return (
        <div className="flex-1 flex overflow-hidden">
            {/* Spotlight search (Ctrl+K) */}
            {showSpotlight && (
                <SpotlightSearch
                    nodes={data.nodes}
                    onSelect={node => { handleSelect(node); setShowSpotlight(false) }}
                    onClose={() => setShowSpotlight(false)}
                />
            )}

            {/* Right-click context menu */}
            {contextMenu && (
                <ContextMenu
                    x={contextMenu.x}
                    y={contextMenu.y}
                    node={contextMenu.node}
                    onClose={() => setContextMenu(null)}
                    onOpenTab={tab => { setDetailsTab(tab); setContextMenu(null) }}
                />
            )}

            {/* Resource list */}
            <div className="w-64 flex-shrink-0 overflow-hidden">
                <Sidebar
                    nodes={data.nodes}
                    edges={data.edges}
                    selectedUID={selectedNode?.uid ?? null}
                    filterKind={filterKind}
                    onSelect={handleSelect}
                    onFilterKind={setFilterKind}
                />
            </div>

            {/* Large-cluster truncation warning */}
            {truncated && (
                <div className="absolute bottom-3 left-1/2 -translate-x-1/2 z-20">
                    <div className="flex items-center gap-2 px-3 py-2 rounded-full bg-amber-950/90 backdrop-blur border border-amber-800/60 text-amber-400 text-[11px] shadow-lg">
                        <Activity className="w-3.5 h-3.5 flex-shrink-0" />
                        Showing {data.nodes.length} of {data.total} resources — use namespace filter or kind filter to narrow
                    </div>
                </div>
            )}

            {/* Graph */}
            <div className="flex-1 relative overflow-hidden">
                {/* Graph header */}
                <div className="absolute top-3 left-3 z-10 flex items-center gap-2 flex-wrap">
                    <span className="text-xs text-slate-500 font-mono bg-space-900/90 backdrop-blur rounded-full px-2.5 py-1 border border-space-700">
                        {selectedNode
                            ? `deps of ${selectedNode.kind}/${selectedNode.name}`
                            : filterKind
                                ? data.nodes.filter(n => n.kind === filterKind).length + ' ' + filterKind
                                : data.nodes.filter(n => ['Deployment', 'StatefulSet', 'DaemonSet', 'CronJob', 'Job', 'Service', 'Ingress', 'NetworkPolicy'].includes(n.kind)).length + ' topology nodes'}
                    </span>

                    {selectedNode && (
                        <button
                            onClick={() => setSelectedNode(null)}
                            className="text-xs px-2 py-1 rounded-full bg-space-900/90 backdrop-blur border border-space-700 text-slate-500 hover:text-accent transition-colors"
                        >
                            ← overview
                        </button>
                    )}

                    {/* Spotlight shortcut hint */}
                    <button
                        onClick={() => setShowSpotlight(true)}
                        className="flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-space-900/90 backdrop-blur border border-space-700 text-slate-600 hover:text-accent transition-colors"
                        title="Search resources (⌘K)"
                    >
                        <Search className="w-3 h-3" />
                        <kbd className="text-[9px]">⌘K</kbd>
                    </button>

                    {/* Heatmap toggle */}
                    <button
                        onClick={() => setHeatmapMode(m => !m)}
                        className={clsx(
                            'flex items-center gap-1.5 px-2.5 py-1 rounded-full backdrop-blur border transition-colors',
                            heatmapMode
                                ? 'bg-accent/15 border-accent/40 text-accent'
                                : 'bg-space-900/90 border-space-700 text-slate-600 hover:text-accent'
                        )}
                        title="Health heatmap — recolor all nodes by health status"
                    >
                        <Thermometer className="w-3 h-3" />
                        <span className="text-[10px]">health</span>
                    </button>

                    {/* Refresh / live toggle */}
                    <div className="flex items-center gap-1 bg-space-900/90 backdrop-blur border border-space-700 rounded-full px-1 py-0.5">
                        <button
                            onClick={() => { handleRefresh(); setLastRefresh(new Date()) }}
                            className="p-1 text-slate-600 hover:text-accent transition-colors"
                        >
                            <RefreshCw className={`w-3 h-3 ${graphQuery.isFetching ? 'animate-spin text-accent' : ''}`} />
                        </button>
                        <span className={clsx('text-[10px] font-mono pr-1', stale ? 'text-status-warning' : 'text-slate-700')}>
                            {relTime}
                        </span>
                        <button
                            onClick={() => setIsLive(l => !l)}
                            className={clsx(
                                'flex items-center gap-1 px-1.5 py-0.5 rounded-full text-[10px] transition-colors',
                                isLive ? 'bg-status-healthy/20 text-status-healthy' : 'text-slate-600 hover:text-slate-400'
                            )}
                            title={isLive ? 'Live mode on — refreshes every 30s' : 'Enable live mode'}
                        >
                            <Activity className="w-2.5 h-2.5" />
                            {isLive ? 'live' : 'auto'}
                        </button>
                    </div>
                </div>

                <GraphCanvas
                    data={data}
                    selectedUID={selectedNode?.uid ?? null}
                    onSelect={handleSelect}
                    filterKind={filterKind}
                    heatmapMode={heatmapMode}
                    onContextMenu={(x, y, node) => { handleSelect(node); setContextMenu({ x, y, node }) }}
                />
            </div>

            {/* Details panel */}
            {selectedNode && (
                <>
                    <div
                        onMouseDown={startResize}
                        className="w-1 flex-shrink-0 cursor-col-resize bg-space-700 hover:bg-accent/60 transition-colors active:bg-accent"
                    />
                    <div style={{ width: detailsWidth, minWidth: 280, maxWidth: 700 }} className="flex-shrink-0 overflow-hidden bg-space-900">
                        <DetailsPanel node={selectedNode} context={context} namespace={namespace} defaultTab={detailsTab} />
                    </div>
                </>
            )}
        </div>
    )
}

