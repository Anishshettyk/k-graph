import { useQuery } from '@tanstack/react-query'
import { useRef, useState, useEffect, useMemo } from 'react'
import {
    Settings, RefreshCw, ChevronDown, Search,
    Network, Unlink, Shield, Stethoscope,
    BarChart3, Zap, GitBranch, Globe, Folder,
    Package, ShieldAlert, Check,
} from 'lucide-react'
import { type LucideIcon } from 'lucide-react'
import clsx from 'clsx'
import { api } from '../api/client'
import { useStore } from '../store/useStore'

type Page = 'explorer' | 'doctor' | 'orphan' | 'rbac' | 'events' | 'metrics' | 'images' | 'certificates' | 'network'

const NAV_ITEMS: { id: Page; label: string; Icon: LucideIcon }[] = [
    { id: 'explorer', label: 'Explorer', Icon: Network },
    { id: 'doctor', label: 'Doctor', Icon: Stethoscope },
    { id: 'metrics', label: 'Metrics', Icon: BarChart3 },
    { id: 'images', label: 'Images', Icon: Package },
    { id: 'network', label: 'Network', Icon: Globe },
    { id: 'certificates', label: 'Certs', Icon: ShieldAlert },
    { id: 'orphan', label: 'Orphans', Icon: Unlink },
    { id: 'rbac', label: 'RBAC', Icon: Shield },
    { id: 'events', label: 'Events', Icon: Zap },
]

// ─── Searchable combobox ──────────────────────────────────────────────────────
function SearchCombobox({ value, options, onChange, placeholder, maxWidth = 220, icon: Icon }: {
    value: string
    options: string[]
    onChange: (v: string) => void
    placeholder?: string
    maxWidth?: number
    icon?: LucideIcon
}) {
    const [open, setOpen] = useState(false)
    const [query, setQuery] = useState('')
    const [activeIdx, setActiveIdx] = useState(-1)
    const inputRef = useRef<HTMLInputElement>(null)
    const listRef = useRef<HTMLUListElement>(null)
    const containerRef = useRef<HTMLDivElement>(null)

    // Close on outside click
    useEffect(() => {
        const handler = (e: MouseEvent) => {
            if (!containerRef.current?.contains(e.target as Node)) {
                setOpen(false)
                setQuery('')
            }
        }
        document.addEventListener('mousedown', handler)
        return () => document.removeEventListener('mousedown', handler)
    }, [])

    // Reset active index when query changes
    useEffect(() => { setActiveIdx(-1) }, [query])

    const filtered = useMemo(() => {
        const q = query.trim().toLowerCase()
        if (!q) return options
        return options.filter(o => o.toLowerCase().includes(q))
    }, [options, query])

    const select = (opt: string) => {
        onChange(opt)
        setOpen(false)
        setQuery('')
        inputRef.current?.blur()
    }

    const handleKeyDown = (e: React.KeyboardEvent) => {
        switch (e.key) {
            case 'ArrowDown':
                e.preventDefault()
                setActiveIdx(i => Math.min(i + 1, filtered.length - 1))
                break
            case 'ArrowUp':
                e.preventDefault()
                setActiveIdx(i => Math.max(i - 1, 0))
                break
            case 'Enter':
                e.preventDefault()
                if (activeIdx >= 0 && filtered[activeIdx]) {
                    select(filtered[activeIdx])
                } else if (filtered.length === 1) {
                    select(filtered[0])
                }
                break
            case 'Escape':
                setOpen(false)
                setQuery('')
                inputRef.current?.blur()
                break
        }
    }

    // Scroll active item into view
    useEffect(() => {
        if (activeIdx < 0 || !listRef.current) return
        const el = listRef.current.children[activeIdx] as HTMLElement
        el?.scrollIntoView({ block: 'nearest' })
    }, [activeIdx])

    const displayValue = open ? query : (value || placeholder || '')

    return (
        <div ref={containerRef} className="relative">
            {/* Trigger */}
            <div
                className={clsx(
                    'flex items-center gap-1.5 rounded-lg border bg-space-800 px-2.5 py-1.5 cursor-text transition-colors',
                    open ? 'border-accent/60 ring-1 ring-accent/20' : 'border-space-700 hover:border-space-600'
                )}
                style={{ maxWidth }}
                onClick={() => { setOpen(true); setTimeout(() => inputRef.current?.focus(), 10) }}
            >
                {Icon && <Icon className="w-3.5 h-3.5 text-slate-500 flex-shrink-0" />}
                <input
                    ref={inputRef}
                    className="flex-1 min-w-0 bg-transparent text-sm text-slate-200 placeholder-slate-500 outline-none truncate"
                    placeholder={placeholder}
                    value={displayValue}
                    readOnly={!open}
                    onChange={e => setQuery(e.target.value)}
                    onFocus={() => setOpen(true)}
                    onKeyDown={handleKeyDown}
                    style={{ maxWidth: maxWidth - 48 }}
                />
                <ChevronDown className={clsx(
                    'w-3 h-3 text-slate-600 flex-shrink-0 transition-transform duration-150',
                    open && 'rotate-180'
                )} />
            </div>

            {/* Dropdown */}
            {open && (
                <div className="absolute top-full left-0 mt-1 z-50 rounded-xl border border-space-700 bg-space-900 shadow-2xl overflow-hidden"
                    style={{ minWidth: 260, maxWidth: 380 }}>
                    {/* Search box inside dropdown */}
                    <div className="flex items-center gap-2 px-3 py-2 border-b border-space-700">
                        <Search className="w-3.5 h-3.5 text-slate-600 flex-shrink-0" />
                        <input
                            ref={inputRef}
                            className="flex-1 bg-transparent text-sm text-slate-200 placeholder-slate-600 outline-none"
                            placeholder="Type to filter…"
                            value={query}
                            autoComplete="off"
                            onChange={e => setQuery(e.target.value)}
                            onKeyDown={handleKeyDown}
                        />
                        {query && (
                            <button onClick={() => setQuery('')} className="text-slate-600 hover:text-slate-400 text-xs">✕</button>
                        )}
                    </div>
                    <div className="text-[10px] text-slate-600 px-3 py-1.5 border-b border-space-800">
                        {filtered.length === options.length
                            ? `${options.length} options`
                            : `${filtered.length} of ${options.length} match`}
                    </div>
                    <ul ref={listRef} className="max-h-64 overflow-y-auto py-1">
                        {filtered.length === 0 && (
                            <li className="px-3 py-3 text-xs text-slate-600 text-center">No matches</li>
                        )}
                        {filtered.map((opt, i) => (
                            <li key={opt}>
                                <button
                                    onMouseDown={e => { e.preventDefault(); select(opt) }}
                                    onMouseEnter={() => setActiveIdx(i)}
                                    className={clsx(
                                        'w-full flex items-center gap-2 px-3 py-2 text-left text-sm transition-colors',
                                        i === activeIdx ? 'bg-accent/15 text-accent'
                                            : opt === value ? 'text-accent/80'
                                                : 'text-slate-300 hover:bg-space-800'
                                    )}
                                >
                                    <span className="flex-1 truncate font-mono text-xs">{opt}</span>
                                    {opt === value && <Check className="w-3 h-3 text-accent flex-shrink-0" />}
                                </button>
                            </li>
                        ))}
                    </ul>
                </div>
            )}
        </div>
    )
}

export function TopBar() {
    const { context, namespace, page, setContext, setNamespace, setPage } = useStore()

    const ctxQuery = useQuery({
        queryKey: ['contexts'],
        queryFn: api.contexts,
        staleTime: 60_000,
    })

    if (ctxQuery.data && !context) {
        setContext(ctxQuery.data.current)
        setNamespace('default')
    }

    const namespaces = useQuery({
        queryKey: ['namespaces', context],
        queryFn: async () => {
            const g = await api.graph(context, '')
            const ns = [...new Set(g.nodes.map(n => n.namespace).filter(Boolean))].sort()
            return ['', ...ns]
        },
        enabled: !!context,
        staleTime: 30_000,
    })

    return (
        <header className="h-13 flex items-center gap-3 px-4 border-b border-space-700 bg-space-900 flex-shrink-0" style={{ height: '52px' }}>
            {/* Logo */}
            <div className="flex items-center gap-2 mr-3">
                <GitBranch className="w-5 h-5 text-accent" strokeWidth={2.5} />
                <span className="font-bold text-white tracking-tight text-base">KGraph</span>
            </div>

            <div className="h-5 w-px bg-space-600" />

            {/* Context selector — searchable combobox */}
            <div className="flex items-center gap-2 ml-1">
                <SearchCombobox
                    value={context}
                    options={ctxQuery.data?.contexts ?? []}
                    onChange={setContext}
                    placeholder="context…"
                    maxWidth={240}
                    icon={Globe}
                />
            </div>

            {/* Namespace selector — searchable combobox */}
            <div className="flex items-center gap-2 ml-1">
                <SearchCombobox
                    value={namespace}
                    options={['', ...(namespaces.data?.filter(Boolean) ?? [])]}
                    onChange={setNamespace}
                    placeholder="all namespaces"
                    maxWidth={180}
                    icon={Folder}
                />
            </div>

            {/* Loading indicator next to context */}
            <div className="w-5 h-5 flex items-center justify-center">
                {ctxQuery.isFetching && (
                    <span className="w-2 h-2 rounded-full bg-accent animate-ping opacity-75" />
                )}
            </div>

            <div className="h-5 w-px bg-space-600 ml-1 mr-2" />

            {/* Nav */}
            <nav className="flex items-center gap-1">
                {NAV_ITEMS.map(({ id, label, Icon }) => (
                    <button
                        key={id}
                        onClick={() => setPage(id)}
                        className={`flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium transition-all ${page === id
                            ? 'bg-accent/15 text-accent shadow-glow-sm'
                            : 'text-slate-400 hover:text-slate-200 hover:bg-space-800/50'
                            }`}
                    >
                        <Icon className="w-4 h-4" strokeWidth={page === id ? 2.5 : 2} />
                        {label}
                    </button>
                ))}
            </nav>

            <div className="ml-auto flex items-center gap-3">
                <button
                    className="p-1.5 rounded-lg text-slate-500 hover:text-slate-300 hover:bg-space-800 transition-colors"
                    title="Refresh Data"
                >
                    <RefreshCw className={clsx("w-4 h-4", ctxQuery.isFetching && "animate-spin text-accent")} />
                </button>
                <button
                    className="p-1.5 rounded-lg text-slate-500 hover:text-slate-300 hover:bg-space-800 transition-colors"
                    title="Settings"
                >
                    <Settings className="w-4 h-4" />
                </button>
            </div>
        </header>
    )
}
