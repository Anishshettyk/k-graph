import { useState, useMemo, useRef, useEffect } from 'react'
import { Search, Filter, Copy, Check, ChevronDown } from 'lucide-react'
import clsx from 'clsx'
import type { LogEntry, LogLevel, PodLogResult } from '../types/api'

// ─── Level styling ───────────────────────────────────────────────────────────

const LEVEL_META: Record<LogLevel, { label: string; rowCls: string; badgeCls: string }> = {
    FATAL: {
        label: 'FAT',
        rowCls: 'bg-red-950/40 border-l-2 border-red-600',
        badgeCls: 'bg-red-900 text-red-200 font-bold',
    },
    ERROR: {
        label: 'ERR',
        rowCls: 'bg-red-950/20 border-l-2 border-red-700/80',
        badgeCls: 'bg-red-900/70 text-red-300 font-bold',
    },
    WARN: {
        label: 'WRN',
        rowCls: 'bg-amber-950/20 border-l-2 border-amber-600/60',
        badgeCls: 'bg-amber-900/60 text-amber-300',
    },
    INFO: {
        label: 'INF',
        rowCls: 'border-l-2 border-transparent',
        badgeCls: 'bg-blue-900/40 text-blue-400',
    },
    DEBUG: {
        label: 'DBG',
        rowCls: 'border-l-2 border-transparent',
        badgeCls: 'bg-space-800 text-slate-600',
    },
    TRACE: {
        label: 'TRC',
        rowCls: 'border-l-2 border-transparent',
        badgeCls: 'bg-space-800 text-slate-700',
    },
    UNKNOWN: {
        label: '---',
        rowCls: 'border-l-2 border-transparent',
        badgeCls: 'bg-space-800 text-slate-600',
    },
}

const FILTER_LEVELS: (LogLevel | 'ALL')[] = ['ALL', 'ERROR', 'WARN', 'INFO', 'DEBUG']

// ─── Component ───────────────────────────────────────────────────────────────

interface Props {
    pods: PodLogResult[]
    isLoading: boolean
    error?: string
}

export function LogViewer({ pods, isLoading, error }: Props) {
    const [level, setLevel] = useState<LogLevel | 'ALL'>('ALL')
    const [search, setSearch] = useState('')
    const [activePod, setActivePod] = useState(0)
    const [autoScroll, setAutoScroll] = useState(true)
    const [copied, setCopied] = useState(false)
    const listRef = useRef<HTMLDivElement>(null)

    // Counts per level for badge display
    const allEntries = useMemo<LogEntry[]>(() => {
        if (!pods[activePod]) return []
        return pods[activePod].entries ?? []
    }, [pods, activePod])

    const levelCounts = useMemo(() => {
        const counts: Record<string, number> = {}
        allEntries.forEach(e => { counts[e.level] = (counts[e.level] ?? 0) + 1 })
        return counts
    }, [allEntries])

    const filtered = useMemo(() => {
        return allEntries.filter(e => {
            const levelOk = level === 'ALL' || e.level === level || (level === 'ERROR' && e.level === 'FATAL')
            const searchOk = !search || e.raw.toLowerCase().includes(search.toLowerCase())
            return levelOk && searchOk
        })
    }, [allEntries, level, search])

    // Auto-scroll to bottom
    useEffect(() => {
        if (autoScroll && listRef.current) {
            listRef.current.scrollTop = listRef.current.scrollHeight
        }
    }, [filtered, autoScroll])

    const copyAll = () => {
        const text = filtered.map(e => e.raw).join('\n')
        navigator.clipboard.writeText(text).then(() => {
            setCopied(true)
            setTimeout(() => setCopied(false), 1500)
        })
    }

    if (isLoading) {
        return (
            <div className="flex-1 flex items-center justify-center text-slate-600 text-xs font-mono animate-pulse">
                fetching logs…
            </div>
        )
    }
    if (error) {
        return <div className="p-3 text-xs text-status-unhealthy font-mono">{error}</div>
    }
    if (!pods.length) {
        return <div className="p-3 text-xs text-slate-600">no pods found</div>
    }

    const pod = pods[activePod]

    return (
        <div className="flex flex-col h-full overflow-hidden bg-space-950">
            {/* ── Controls ──────────────────────────────────────────────────── */}
            <div className="flex-shrink-0 border-b border-space-700 bg-space-900 px-2 py-1.5 space-y-1.5">
                {/* Pod + container selector */}
                {pods.length > 1 && (
                    <div className="flex gap-1 flex-wrap">
                        {pods.map((p, i) => (
                            <button
                                key={i}
                                onClick={() => setActivePod(i)}
                                className={clsx(
                                    'text-[10px] px-2 py-0.5 rounded font-mono transition-colors',
                                    i === activePod
                                        ? 'bg-accent/20 text-accent border border-accent/30'
                                        : 'text-slate-500 border border-space-700 hover:text-slate-300'
                                )}
                            >
                                {p.pod}{p.container ? `/${p.container}` : ''}
                            </button>
                        ))}
                    </div>
                )}

                {/* Level filters */}
                <div className="flex items-center gap-1.5">
                    <Filter className="w-3 h-3 text-slate-600 flex-shrink-0" />
                    {FILTER_LEVELS.map(l => {
                        const count = l === 'ALL'
                            ? allEntries.length
                            : (levelCounts[l] ?? 0) + (l === 'ERROR' ? (levelCounts['FATAL'] ?? 0) : 0)
                        if (l !== 'ALL' && count === 0) return null
                        return (
                            <button
                                key={l}
                                onClick={() => setLevel(l)}
                                className={clsx(
                                    'text-[10px] px-1.5 py-0.5 rounded font-mono transition-colors flex items-center gap-1',
                                    level === l ? levelBtnActive(l) : 'text-slate-600 hover:text-slate-400'
                                )}
                            >
                                {l}
                                <span className="opacity-60">({count})</span>
                            </button>
                        )
                    })}
                    <div className="flex-1" />
                    <button onClick={copyAll} className="p-1 text-slate-600 hover:text-slate-300 transition-colors">
                        {copied ? <Check className="w-3 h-3 text-status-healthy" /> : <Copy className="w-3 h-3" />}
                    </button>
                </div>

                {/* Search */}
                <div className="relative">
                    <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3 h-3 text-slate-600" />
                    <input
                        type="text"
                        placeholder="search logs…"
                        value={search}
                        onChange={e => setSearch(e.target.value)}
                        className="w-full bg-space-800 border border-space-700 rounded pl-6 pr-3 py-1 text-[11px] font-mono text-slate-300 placeholder-slate-700 focus:outline-none focus:border-accent/50"
                    />
                    {search && (
                        <span className="absolute right-2 top-1/2 -translate-y-1/2 text-[10px] text-slate-600">
                            {filtered.length}
                        </span>
                    )}
                </div>
            </div>

            {/* ── Error for this pod ───────────────────────────────────────── */}
            {pod.error && (
                <div className="px-2 py-1 text-[10px] font-mono text-status-unhealthy bg-red-950/20 border-b border-red-900/30">
                    ✖ {pod.error}
                </div>
            )}

            {/* ── Log lines ─────────────────────────────────────────────────── */}
            <div
                ref={listRef}
                className="flex-1 overflow-y-auto font-mono text-[11px] leading-5 select-text"
                onScroll={e => {
                    const el = e.currentTarget
                    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40
                    setAutoScroll(atBottom)
                }}
            >
                {filtered.length === 0 && (
                    <div className="text-center text-slate-700 py-8 text-xs">no matching lines</div>
                )}
                {filtered.map(entry => {
                    const meta = LEVEL_META[entry.level] ?? LEVEL_META.UNKNOWN
                    return (
                        <div
                            key={entry.lineNum}
                            className={clsx('flex items-start gap-1.5 px-2 py-px hover:bg-white/[0.03]', meta.rowCls)}
                        >
                            {/* Line number */}
                            <span className="text-slate-700 w-8 flex-shrink-0 text-right select-none pt-px">
                                {entry.lineNum}
                            </span>
                            {/* Level badge */}
                            <span className={clsx('text-[9px] px-1 rounded flex-shrink-0 mt-0.5', meta.badgeCls)}>
                                {meta.label}
                            </span>
                            {/* Raw log line — preserve whitespace */}
                            <span className={clsx('flex-1 break-all whitespace-pre-wrap', levelTextCls(entry.level))}>
                                {entry.raw}
                            </span>
                        </div>
                    )
                })}
            </div>

            {/* ── Footer ───────────────────────────────────────────────────── */}
            <div className="flex-shrink-0 border-t border-space-700 px-2 py-1 flex items-center gap-2 text-[10px] text-slate-600">
                <ChevronDown className="w-3 h-3" />
                <span>{filtered.length} lines shown</span>
                {!autoScroll && (
                    <button
                        onClick={() => {
                            setAutoScroll(true)
                            if (listRef.current) listRef.current.scrollTop = listRef.current.scrollHeight
                        }}
                        className="ml-auto text-accent hover:text-accent-bright transition-colors"
                    >
                        ↓ follow
                    </button>
                )}
            </div>
        </div>
    )
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

function levelTextCls(level: LogLevel): string {
    switch (level) {
        case 'FATAL': return 'text-red-300'
        case 'ERROR': return 'text-red-400'
        case 'WARN': return 'text-amber-300'
        case 'INFO': return 'text-slate-300'
        case 'DEBUG': return 'text-slate-500'
        case 'TRACE': return 'text-slate-600'
        default: return 'text-slate-400'
    }
}

function levelBtnActive(level: LogLevel | 'ALL'): string {
    switch (level) {
        case 'ERROR': return 'text-red-400 bg-red-950/30'
        case 'WARN': return 'text-amber-400 bg-amber-950/30'
        case 'INFO': return 'text-blue-400 bg-blue-950/30'
        case 'DEBUG': return 'text-slate-400 bg-space-800'
        default: return 'text-accent bg-accent/10'
    }
}
