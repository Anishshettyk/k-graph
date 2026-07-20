import { useState, useEffect, useRef, useCallback } from 'react'
import { Search } from 'lucide-react'
import clsx from 'clsx'
import { kindColor } from '../lib/kinds'
import type { NodeInfo } from '../types/api'

interface Props {
    nodes: NodeInfo[]
    onSelect: (node: NodeInfo) => void
    onClose: () => void
}

export function SpotlightSearch({ nodes, onSelect, onClose }: Props) {
    const [query, setQuery] = useState('')
    const [cursor, setCursor] = useState(0)
    const inputRef = useRef<HTMLInputElement>(null)
    const listRef = useRef<HTMLDivElement>(null)

    const results = query.trim()
        ? nodes.filter(n => {
            const q = query.toLowerCase()
            return n.name.toLowerCase().includes(q)
                || n.kind.toLowerCase().includes(q)
                || (n.namespace ?? '').toLowerCase().includes(q)
        })
        : nodes.slice(0, 12)

    useEffect(() => { inputRef.current?.focus() }, [])
    useEffect(() => { setCursor(0) }, [query])

    const confirm = useCallback((node: NodeInfo) => {
        onSelect(node)
        onClose()
    }, [onSelect, onClose])

    useEffect(() => {
        const handler = (e: KeyboardEvent) => {
            if (e.key === 'Escape') { onClose(); return }
            if (e.key === 'ArrowDown') { setCursor(c => Math.min(c + 1, results.length - 1)); e.preventDefault() }
            if (e.key === 'ArrowUp') { setCursor(c => Math.max(c - 1, 0)); e.preventDefault() }
            if (e.key === 'Enter' && results[cursor]) confirm(results[cursor])
        }
        window.addEventListener('keydown', handler)
        return () => window.removeEventListener('keydown', handler)
    }, [results, cursor, confirm, onClose])

    // Scroll active item into view
    useEffect(() => {
        const el = listRef.current?.children[cursor] as HTMLElement | undefined
        el?.scrollIntoView({ block: 'nearest' })
    }, [cursor])

    const healthDot = (n: NodeInfo) => {
        if (n.kind !== 'Pod') return null
        return (
            <span className={clsx('w-2 h-2 rounded-full flex-shrink-0',
                n.healthy ? 'bg-emerald-400' : 'bg-red-400 animate-pulse'
            )} />
        )
    }

    return (
        <div className="fixed inset-0 z-50 flex items-start justify-center pt-[15vh]" onClick={onClose}>
            <div
                className="w-[560px] rounded-2xl border border-space-700 bg-space-900/95 backdrop-blur-xl shadow-2xl overflow-hidden"
                onClick={e => e.stopPropagation()}
            >
                {/* Input */}
                <div className="flex items-center gap-3 px-4 py-3 border-b border-space-700">
                    <Search className="w-4 h-4 text-slate-500 flex-shrink-0" />
                    <input
                        ref={inputRef}
                        value={query}
                        onChange={e => setQuery(e.target.value)}
                        placeholder="Search resources by name, kind or namespace…"
                        className="flex-1 bg-transparent text-sm text-slate-100 placeholder-slate-600 focus:outline-none"
                    />
                    <kbd className="text-[10px] text-slate-600 border border-space-700 rounded px-1.5 py-0.5">ESC</kbd>
                </div>

                {/* Results */}
                <div ref={listRef} className="max-h-[380px] overflow-y-auto py-1">
                    {results.length === 0 && (
                        <div className="text-xs text-slate-600 text-center py-8">No resources match</div>
                    )}
                    {results.map((n, i) => {
                        const color = kindColor(n.kind)
                        return (
                            <div
                                key={n.uid}
                                onClick={() => confirm(n)}
                                onMouseEnter={() => setCursor(i)}
                                className={clsx(
                                    'flex items-center gap-3 px-4 py-2.5 cursor-pointer transition-colors',
                                    i === cursor ? 'bg-space-800' : 'hover:bg-space-850'
                                )}
                            >
                                {/* Kind accent */}
                                <div className="w-1 h-8 rounded-full flex-shrink-0" style={{ background: color }} />

                                {/* Kind badge */}
                                <span
                                    className="text-[9px] font-bold tracking-widest uppercase px-1.5 py-0.5 rounded flex-shrink-0"
                                    style={{ color, background: color + '20' }}
                                >
                                    {n.kind.replace('PersistentVolumeClaim', 'PVC').replace('PersistentVolume', 'PV').replace('ServiceAccount', 'SA')}
                                </span>

                                {/* Name */}
                                <span className="text-sm font-medium text-slate-100 truncate flex-1">{n.name}</span>

                                {/* Namespace */}
                                {n.namespace && (
                                    <span className="text-[10px] font-mono text-slate-600 flex-shrink-0">{n.namespace}</span>
                                )}

                                {healthDot(n)}

                                {/* Enter hint */}
                                {i === cursor && (
                                    <kbd className="text-[10px] text-slate-600 border border-space-700 rounded px-1.5 py-0.5 flex-shrink-0">↵</kbd>
                                )}
                            </div>
                        )
                    })}
                </div>

                {/* Footer hint */}
                <div className="px-4 py-2 border-t border-space-800 flex gap-4 text-[10px] text-slate-700">
                    <span>↑↓ navigate</span>
                    <span>↵ select</span>
                    <span>{nodes.length} resources</span>
                </div>
            </div>
        </div>
    )
}
