import { useEffect, useRef } from 'react'
import type { NodeInfo } from '../types/api'

interface Action {
    label: string
    icon: string
    onClick: () => void
    destructive?: boolean
}

interface Props {
    x: number
    y: number
    node: NodeInfo
    onClose: () => void
    onOpenTab: (tab: 'why' | 'logs' | 'yaml' | 'events') => void
}

export function ContextMenu({ x, y, node, onClose, onOpenTab }: Props) {
    const ref = useRef<HTMLDivElement>(null)

    useEffect(() => {
        const handler = (e: MouseEvent) => {
            if (ref.current && !ref.current.contains(e.target as Node)) onClose()
        }
        const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
        setTimeout(() => window.addEventListener('mousedown', handler), 0)
        window.addEventListener('keydown', esc)
        return () => {
            window.removeEventListener('mousedown', handler)
            window.removeEventListener('keydown', esc)
        }
    }, [onClose])

    const copyText = (text: string) => {
        navigator.clipboard.writeText(text)
        onClose()
    }

    const actions: Action[] = [
        { label: 'Why is this failing?', icon: '⚕', onClick: () => { onOpenTab('why'); onClose() } },
        { label: 'View logs', icon: '◈', onClick: () => { onOpenTab('logs'); onClose() } },
        { label: 'View YAML manifest', icon: '▦', onClick: () => { onOpenTab('yaml'); onClose() } },
        { label: 'View events', icon: '◷', onClick: () => { onOpenTab('events'); onClose() } },
        { label: `Copy name`, icon: '⎘', onClick: () => copyText(node.name) },
        ...(node.namespace ? [{ label: 'Copy namespace/name', icon: '⎘', onClick: () => copyText(`${node.namespace}/${node.name}`) }] : []),
    ]

    // Keep menu inside viewport
    const menuW = 220
    const menuH = actions.length * 36 + 16
    const left = Math.min(x, window.innerWidth - menuW - 8)
    const top = Math.min(y, window.innerHeight - menuH - 8)

    return (
        <div
            ref={ref}
            className="fixed z-50 rounded-xl border border-space-700 bg-space-900/95 backdrop-blur-xl shadow-2xl py-1 overflow-hidden"
            style={{ left, top, width: menuW }}
        >
            {/* Header */}
            <div className="px-3 py-1.5 border-b border-space-800 mb-1">
                <div className="text-[10px] font-bold uppercase tracking-widest text-slate-600">{node.kind}</div>
                <div className="text-xs font-medium text-slate-300 truncate">{node.name}</div>
            </div>

            {actions.map((a, i) => (
                <button
                    key={i}
                    onClick={a.onClick}
                    className="w-full flex items-center gap-2.5 px-3 py-2 text-xs text-slate-300 hover:bg-space-800 hover:text-white transition-colors text-left"
                >
                    <span className="text-slate-500 w-4 text-center">{a.icon}</span>
                    {a.label}
                </button>
            ))}
        </div>
    )
}
