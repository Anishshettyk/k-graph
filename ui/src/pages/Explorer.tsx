import { useState, useRef, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Loader2, RefreshCw } from 'lucide-react'
import { api } from '../api/client'
import { useStore } from '../store/useStore'
import { Sidebar } from '../components/Sidebar'
import { GraphCanvas } from '../components/GraphCanvas'
import { DetailsPanel } from '../components/DetailsPanel'
import type { NodeInfo } from '../types/api'

export function Explorer() {
    const { context, namespace, selectedNode, setSelectedNode } = useStore()
    const [filterKind, setFilterKind] = useState('')
    const [detailsWidth, setDetailsWidth] = useState(340)
    const isResizing = useRef(false)

    const handleSelect = (node: NodeInfo) => {
        setSelectedNode(node)
    }

    // Drag-to-resize the details panel. Dragging the handle left makes it wider.
    const startResize = useCallback((e: React.MouseEvent) => {
        isResizing.current = true
        const startX = e.clientX
        const startW = detailsWidth
        const onMove = (ev: MouseEvent) => {
            if (!isResizing.current) return
            const dx = startX - ev.clientX
            setDetailsWidth(Math.max(280, Math.min(700, startW + dx)))
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
        queryFn: () => api.graph(context, namespace),
        enabled: !!context,
        staleTime: 30_000,
    })

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
                    <button
                        onClick={() => graphQuery.refetch()}
                        className="text-xs text-accent hover:text-accent-bright transition-colors"
                    >
                        retry
                    </button>
                </div>
            </div>
        )
    }

    const data = graphQuery.data!

    return (
        <div className="flex-1 flex overflow-hidden">
            {/* Resource list */}
            <div className="w-64 flex-shrink-0 overflow-hidden">
                <Sidebar
                    nodes={data.nodes}
                    selectedUID={selectedNode?.uid ?? null}
                    filterKind={filterKind}
                    onSelect={handleSelect}
                    onFilterKind={setFilterKind}
                />
            </div>

            {/* Graph */}
            <div className="flex-1 relative overflow-hidden">
                {/* Graph header */}
                <div className="absolute top-3 left-3 z-10 flex items-center gap-2">
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
                            ← back to overview
                        </button>
                    )}
                    <button
                        onClick={() => graphQuery.refetch()}
                        className="p-1.5 rounded-full bg-space-900/90 backdrop-blur border border-space-700 text-slate-600 hover:text-accent transition-colors"
                    >
                        <RefreshCw className={`w-3 h-3 ${graphQuery.isFetching ? 'animate-spin text-accent' : ''}`} />
                    </button>
                </div>

                <GraphCanvas
                    data={data}
                    selectedUID={selectedNode?.uid ?? null}
                    onSelect={handleSelect}
                    filterKind={filterKind}
                />
            </div>

            {/* Details panel with drag-to-resize handle */}
            {selectedNode && (
                <>
                    {/* Resize handle — drag left to expand */}
                    <div
                        onMouseDown={startResize}
                        className="w-1 flex-shrink-0 cursor-col-resize bg-space-700 hover:bg-accent/60 transition-colors active:bg-accent"
                        title="Drag to resize"
                    />
                    <div
                        style={{ width: detailsWidth, minWidth: 280, maxWidth: 700 }}
                        className="flex-shrink-0 overflow-hidden bg-space-900"
                    >
                        <DetailsPanel node={selectedNode} context={context} namespace={namespace} />
                    </div>
                </>
            )}
        </div>
    )
}
