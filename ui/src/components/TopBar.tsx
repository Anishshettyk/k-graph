import { useQuery } from '@tanstack/react-query'
import {
    Settings, RefreshCw, ChevronDown,
    Network, Unlink, Shield, Stethoscope,
    BarChart3, Zap, GitBranch, Globe, Folder,
    Package, ShieldAlert,
} from 'lucide-react'
import { type LucideIcon } from 'lucide-react'
import clsx from 'clsx'
import { api } from '../api/client'
import { useStore } from '../store/useStore'

type Page = 'explorer' | 'doctor' | 'orphan' | 'rbac' | 'events' | 'metrics' | 'images' | 'certificates' | 'network'

const NAV_ITEMS: { id: Page; label: string; Icon: LucideIcon }[] = [
    { id: 'explorer',     label: 'Explorer', Icon: Network },
    { id: 'doctor',       label: 'Doctor',   Icon: Stethoscope },
    { id: 'metrics',      label: 'Metrics',  Icon: BarChart3 },
    { id: 'images',       label: 'Images',   Icon: Package },
    { id: 'network',      label: 'Network',  Icon: Globe },
    { id: 'certificates', label: 'Certs',    Icon: ShieldAlert },
    { id: 'orphan',       label: 'Orphans',  Icon: Unlink },
    { id: 'rbac',         label: 'RBAC',     Icon: Shield },
    { id: 'events',       label: 'Events',   Icon: Zap },
]

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

            {/* Context selector */}
            <div className="flex items-center gap-2 ml-1">
                <Globe className="w-4 h-4 text-slate-400 flex-shrink-0" />
                <div className="relative">
                    <select
                        value={context}
                        onChange={e => setContext(e.target.value)}
                        className="appearance-none bg-space-800 border border-space-700 text-slate-200 text-sm rounded-lg pl-3 pr-8 py-1.5 focus:outline-none focus:border-accent cursor-pointer max-w-[220px] font-medium"
                    >
                        {ctxQuery.data?.contexts.map(c => (
                            <option key={c} value={c}>{c}</option>
                        ))}
                    </select>
                    <ChevronDown className="absolute right-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-slate-500 pointer-events-none" />
                </div>
            </div>

            {/* Namespace selector */}
            <div className="flex items-center gap-2 ml-2">
                <Folder className="w-4 h-4 text-slate-400 flex-shrink-0" />
                <div className="relative">
                    <select
                        value={namespace}
                        onChange={e => setNamespace(e.target.value)}
                        className="appearance-none bg-space-800 border border-space-700 text-slate-200 text-sm rounded-lg pl-3 pr-8 py-1.5 focus:outline-none focus:border-accent cursor-pointer max-w-[160px]"
                    >
                        <option value="">all namespaces</option>
                        {namespaces.data?.filter(Boolean).map(ns => (
                            <option key={ns} value={ns}>{ns}</option>
                        ))}
                    </select>
                    <ChevronDown className="absolute right-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-slate-500 pointer-events-none" />
                </div>
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
