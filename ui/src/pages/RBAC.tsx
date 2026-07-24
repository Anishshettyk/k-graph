import { useState, useMemo, useRef, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Loader2, Shield, ChevronDown, Search, CheckCircle2, XCircle, ArrowRight, Zap, AlertTriangle, Ghost } from 'lucide-react'
import clsx from 'clsx'
import { api } from '../api/client'
import { useStore } from '../store/useStore'
import type { RBACRiskFinding, RBACRiskLevel } from '../types/api'

const VERBS = ['get', 'list', 'watch', 'create', 'update', 'patch', 'delete', 'deletecollection']

const RESOURCES: { value: string; group: string; label?: string }[] = [
    { value: 'pods', group: '' },
    { value: 'pods/log', group: '', label: 'pods/log' },
    { value: 'pods/exec', group: '', label: 'pods/exec' },
    { value: 'secrets', group: '' },
    { value: 'configmaps', group: '' },
    { value: 'serviceaccounts', group: '' },
    { value: 'services', group: '' },
    { value: 'endpoints', group: '' },
    { value: 'namespaces', group: '' },
    { value: 'nodes', group: '' },
    { value: 'events', group: '' },
    { value: 'persistentvolumeclaims', group: '', label: 'PVCs' },
    { value: 'deployments', group: 'apps' },
    { value: 'replicasets', group: 'apps' },
    { value: 'statefulsets', group: 'apps' },
    { value: 'daemonsets', group: 'apps' },
    { value: 'jobs', group: 'batch' },
    { value: 'cronjobs', group: 'batch' },
    { value: 'ingresses', group: 'networking.k8s.io', label: 'ingresses' },
    { value: 'networkpolicies', group: 'networking.k8s.io' },
    { value: 'roles', group: 'rbac.authorization.k8s.io' },
    { value: 'clusterroles', group: 'rbac.authorization.k8s.io' },
    { value: 'rolebindings', group: 'rbac.authorization.k8s.io' },
    { value: 'clusterrolebindings', group: 'rbac.authorization.k8s.io' },
]

// ─── Combobox for ServiceAccount ─────────────────────────────────────────────
function SACombobox({ value, onChange, options }: {
    value: string
    onChange: (v: string) => void
    options: string[]
}) {
    const [open, setOpen] = useState(false)
    const [filter, setFilter] = useState(value)
    const ref = useRef<HTMLDivElement>(null)

    useEffect(() => { setFilter(value) }, [value])
    useEffect(() => {
        const handler = (e: MouseEvent) => { if (!ref.current?.contains(e.target as Node)) setOpen(false) }
        document.addEventListener('mousedown', handler)
        return () => document.removeEventListener('mousedown', handler)
    }, [])

    const filtered = useMemo(() =>
        options.filter(o => o.toLowerCase().includes(filter.toLowerCase())).slice(0, 20)
        , [options, filter])

    return (
        <div ref={ref} className="relative">
            <div className={clsx(
                'flex items-center gap-2 rounded-xl border bg-space-800 px-3 py-2.5 transition-colors',
                open ? 'border-accent/60' : 'border-space-700 hover:border-space-600'
            )}>
                <Search className="w-3.5 h-3.5 text-slate-600 flex-shrink-0" />
                <input
                    className="flex-1 bg-transparent text-sm text-slate-200 placeholder-slate-600 outline-none min-w-0"
                    placeholder="Select or type a ServiceAccount…"
                    value={filter}
                    onFocus={() => setOpen(true)}
                    onChange={e => { setFilter(e.target.value); onChange(e.target.value); setOpen(true) }}
                />
                {options.length > 0 && (
                    <button onClick={() => setOpen(o => !o)} className="text-slate-600 hover:text-slate-400">
                        <ChevronDown className={clsx('w-3.5 h-3.5 transition-transform', open && 'rotate-180')} />
                    </button>
                )}
            </div>
            {open && filtered.length > 0 && (
                <div className="absolute z-50 mt-1 w-full rounded-xl border border-space-700 bg-space-850 shadow-xl overflow-hidden">
                    <div className="max-h-52 overflow-y-auto py-1">
                        {filtered.map(opt => (
                            <button
                                key={opt}
                                onMouseDown={e => { e.preventDefault(); onChange(opt); setFilter(opt); setOpen(false) }}
                                className={clsx(
                                    'w-full flex items-center gap-2 px-3 py-2 text-left text-sm transition-colors',
                                    opt === value ? 'bg-accent/15 text-accent' : 'text-slate-300 hover:bg-space-800'
                                )}
                            >
                                <div className="w-5 h-5 rounded-full bg-accent/10 border border-accent/20 flex items-center justify-center flex-shrink-0">
                                    <span className="text-[8px] font-bold text-accent/80">{opt[0]?.toUpperCase()}</span>
                                </div>
                                {opt}
                                {opt === value && <CheckCircle2 className="w-3.5 h-3.5 text-accent ml-auto" />}
                            </button>
                        ))}
                    </div>
                </div>
            )}
        </div>
    )
}

export function RBAC() {
    const [activeTab, setActiveTab] = useState<'tracer' | 'audit'>('tracer')

    return (
        <div className="flex-1 overflow-y-auto p-6 max-w-3xl space-y-4">
            {/* Tab switcher */}
            <div className="flex gap-1 border-b border-space-700 -mx-6 px-6 pb-0">
                <button onClick={() => setActiveTab('tracer')} className={clsx(
                    'flex items-center gap-1.5 px-3 py-2 text-xs font-medium border-b-2 -mb-px transition-colors',
                    activeTab === 'tracer' ? 'border-accent text-accent' : 'border-transparent text-slate-500 hover:text-slate-300'
                )}><Shield className="w-3.5 h-3.5" />Path Tracer</button>
                <button onClick={() => setActiveTab('audit')} className={clsx(
                    'flex items-center gap-1.5 px-3 py-2 text-xs font-medium border-b-2 -mb-px transition-colors',
                    activeTab === 'audit' ? 'border-red-400 text-red-400' : 'border-transparent text-slate-500 hover:text-slate-300'
                )}><AlertTriangle className="w-3.5 h-3.5" />Overprivilege Audit</button>
            </div>

            {activeTab === 'tracer' && <RBACTracer />}
            {activeTab === 'audit' && <RBACRiskAudit />}
        </div>
    )
}

function RBACTracer() {
    const { context, namespace } = useStore()
    const [sa, setSa] = useState('')
    const [verb, setVerb] = useState('get')
    const [resource, setResource] = useState('secrets')
    const [submitted, setSubmitted] = useState(false)
    const [queryKey, setQueryKey] = useState<unknown[]>([])

    // Live SA list from graph
    const graphQuery = useQuery({
        queryKey: ['graph-sa', context, namespace],
        queryFn: () => api.graph(context, namespace),
        enabled: !!context,
        staleTime: 60_000,
        select: d => d.nodes.filter(n => n.kind === 'ServiceAccount').map(n => n.name).sort(),
    })
    const saOptions = graphQuery.data ?? []

    // Auto-infer API group from resource selection
    const selectedResource = RESOURCES.find(r => r.value === resource)
    const apiGroup = selectedResource?.group ?? ''

    const rbacQuery = useQuery({
        queryKey: ['rbac', ...queryKey],
        queryFn: () => api.rbac(context, namespace || 'default', sa, verb, resource, apiGroup),
        enabled: submitted && !!sa,
        staleTime: 60_000,
        retry: false,
    })

    const handleSubmit = (e?: React.FormEvent) => {
        e?.preventDefault()
        if (!sa) return
        setQueryKey([context, namespace, sa, verb, resource, apiGroup, Date.now()])
        setSubmitted(true)
    }

    const handleResourceChange = (r: string) => {
        setResource(r)
        // re-run if already submitted
        if (submitted && sa) {
            const grp = RESOURCES.find(x => x.value === r)?.group ?? ''
            setQueryKey([context, namespace, sa, verb, r, grp, Date.now()])
        }
    }

    const handleVerbChange = (v: string) => {
        setVerb(v)
        if (submitted && sa) {
            setQueryKey([context, namespace, sa, v, resource, apiGroup, Date.now()])
        }
    }

    const granted = rbacQuery.data?.verdict === 'GRANTED'

    return (
        <div className="flex-1 overflow-y-auto p-6 max-w-3xl space-y-6">

            {/* ── Header ──────────────────────────────────────────────────────── */}
            <div className="flex items-center gap-3">
                <div className="w-9 h-9 rounded-xl bg-accent/10 border border-accent/20 flex items-center justify-center flex-shrink-0">
                    <Shield className="w-5 h-5 text-accent" />
                </div>
                <div>
                    <h1 className="text-base font-semibold text-slate-100">RBAC Path Tracer</h1>
                    <p className="text-xs text-slate-600">Check if a ServiceAccount can perform an action — traces the full binding → role → rule chain</p>
                </div>
            </div>

            {/* ── Form card ───────────────────────────────────────────────────── */}
            <form onSubmit={handleSubmit} className="rounded-2xl border border-space-700 bg-space-900 p-5 space-y-5">

                {/* SA + namespace row */}
                <div className="grid grid-cols-[1fr_160px] gap-3">
                    <div className="space-y-1.5">
                        <label className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">ServiceAccount</label>
                        <SACombobox value={sa} onChange={setSa} options={saOptions} />
                    </div>
                    <div className="space-y-1.5">
                        <label className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Namespace</label>
                        <div className="rounded-xl border border-space-700 bg-space-800/40 px-3 py-2.5 text-sm text-slate-500 font-mono">
                            {namespace || 'default'}
                        </div>
                    </div>
                </div>

                {/* Verb pills */}
                <div className="space-y-1.5">
                    <label className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Verb</label>
                    <div className="flex flex-wrap gap-1.5">
                        {VERBS.map(v => (
                            <button
                                key={v}
                                type="button"
                                onClick={() => handleVerbChange(v)}
                                className={clsx(
                                    'px-3 py-1 rounded-full text-xs font-medium transition-all border',
                                    verb === v
                                        ? 'bg-accent/20 border-accent/50 text-accent'
                                        : 'bg-space-800 border-space-700 text-slate-500 hover:text-slate-300 hover:border-space-600'
                                )}
                            >{v}</button>
                        ))}
                    </div>
                </div>

                {/* Resource selector */}
                <div className="space-y-1.5">
                    <label className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Resource</label>
                    <div className="flex flex-wrap gap-1.5">
                        {RESOURCES.map(r => (
                            <button
                                key={r.value}
                                type="button"
                                onClick={() => handleResourceChange(r.value)}
                                className={clsx(
                                    'px-3 py-1 rounded-full text-xs font-medium transition-all border',
                                    resource === r.value
                                        ? 'bg-violet-500/20 border-violet-500/50 text-violet-300'
                                        : 'bg-space-800 border-space-700 text-slate-500 hover:text-slate-300 hover:border-space-600'
                                )}
                            >{r.label ?? r.value}</button>
                        ))}
                    </div>
                    {apiGroup && (
                        <p className="text-[10px] text-slate-600">
                            API group auto-detected: <span className="font-mono text-accent/70">{apiGroup}</span>
                        </p>
                    )}
                </div>

                {/* Submit */}
                <button
                    type="submit"
                    disabled={!sa}
                    className={clsx(
                        'w-full py-2.5 rounded-xl text-sm font-semibold transition-all flex items-center justify-center gap-2',
                        sa
                            ? 'bg-accent/20 border border-accent/40 text-accent hover:bg-accent/30'
                            : 'bg-space-800 border border-space-700 text-slate-600 cursor-not-allowed'
                    )}
                >
                    {rbacQuery.isLoading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Zap className="w-4 h-4" />}
                    Check Permission
                </button>
            </form>

            {/* ── Error ───────────────────────────────────────────────────────── */}
            {rbacQuery.error && (
                <div className="rounded-xl border border-red-900/40 bg-red-950/10 p-4 text-xs text-status-unhealthy flex items-center gap-2">
                    <XCircle className="w-4 h-4 flex-shrink-0" />
                    {(rbacQuery.error as Error).message}
                </div>
            )}

            {/* ── Result ──────────────────────────────────────────────────────── */}
            {rbacQuery.data && (() => {
                const d = rbacQuery.data
                const matchingChecks = d.checks.filter(c => c.subjectMatch)
                return (
                    <div className="space-y-4">

                        {/* Verdict banner */}
                        <div className={clsx(
                            'rounded-2xl border p-5 flex items-center gap-4',
                            granted
                                ? 'border-emerald-800/40 bg-emerald-950/10'
                                : 'border-red-800/40 bg-red-950/10'
                        )}>
                            <div className={clsx(
                                'w-12 h-12 rounded-2xl flex items-center justify-center flex-shrink-0 text-2xl',
                                granted ? 'bg-emerald-950/60 text-status-healthy' : 'bg-red-950/60 text-status-unhealthy'
                            )}>
                                {granted ? '✓' : '✖'}
                            </div>
                            <div className="flex-1 min-w-0">
                                <div className={clsx('text-lg font-bold tracking-tight', granted ? 'text-status-healthy' : 'text-status-unhealthy')}>
                                    {granted ? 'GRANTED' : 'DENIED'}
                                </div>
                                <div className="flex items-center gap-1.5 mt-0.5 flex-wrap">
                                    <span className="text-xs font-mono bg-space-800 border border-space-700 rounded-full px-2 py-0.5 text-slate-400">{d.serviceAccount}</span>
                                    <ArrowRight className="w-3 h-3 text-slate-600" />
                                    <span className="text-xs font-mono bg-space-800 border border-space-700 rounded-full px-2 py-0.5 text-accent/80">{verb}</span>
                                    <span className="text-xs font-mono bg-space-800 border border-space-700 rounded-full px-2 py-0.5 text-violet-400">{resource}</span>
                                    {apiGroup && <span className="text-xs font-mono text-slate-600">({apiGroup})</span>}
                                </div>
                            </div>
                        </div>

                        {/* Grant paths */}
                        {d.paths && d.paths.length > 0 && (
                            <div className="rounded-2xl border border-space-700 bg-space-900 overflow-hidden">
                                <div className="px-4 py-3 border-b border-space-700 flex items-center gap-2">
                                    <CheckCircle2 className="w-4 h-4 text-status-healthy" />
                                    <span className="text-xs font-semibold uppercase tracking-widest text-slate-400">Grant Paths</span>
                                </div>
                                <div className="divide-y divide-space-800">
                                    {d.paths.map((p, i) => (
                                        <div key={i} className="px-4 py-3 flex items-center gap-2 flex-wrap">
                                            <span className="text-[10px] font-mono text-slate-500">{p.bindingKind}</span>
                                            <span className="text-xs font-mono font-medium text-accent">{p.bindingName}</span>
                                            {p.bindingNs && <span className="text-[10px] text-slate-600 font-mono">({p.bindingNs})</span>}
                                            <ArrowRight className="w-3 h-3 text-slate-700 flex-shrink-0" />
                                            <span className="text-[10px] font-mono text-slate-500">{p.roleKind}/</span>
                                            <span className="text-xs font-mono font-medium text-slate-300">{p.roleName}</span>
                                            <span className="ml-auto text-[10px] text-status-healthy bg-emerald-950/40 border border-emerald-900/30 rounded-full px-2 py-0.5">
                                                rules[{p.ruleIndex}] ✓
                                            </span>
                                        </div>
                                    ))}
                                </div>
                            </div>
                        )}

                        {/* Binding evaluation */}
                        <div className="rounded-2xl border border-space-700 bg-space-900 overflow-hidden">
                            <div className="px-4 py-3 border-b border-space-700 flex items-center gap-2">
                                <Shield className="w-4 h-4 text-slate-500" />
                                <span className="text-xs font-semibold uppercase tracking-widest text-slate-400">Binding Evaluation</span>
                                <span className="text-[10px] text-slate-600 ml-auto">{matchingChecks.length} binding{matchingChecks.length !== 1 ? 's' : ''} checked</span>
                            </div>
                            {matchingChecks.length === 0 ? (
                                <div className="px-4 py-4 text-xs text-slate-600 flex items-center gap-2">
                                    <XCircle className="w-3.5 h-3.5" />
                                    No bindings reference this ServiceAccount
                                </div>
                            ) : (
                                <div className="divide-y divide-space-800">
                                    {matchingChecks.map((c, i) => (
                                        <div key={i} className="px-4 py-2.5 flex items-center gap-3">
                                            <span className={clsx(
                                                'w-5 h-5 rounded-full flex items-center justify-center flex-shrink-0 text-xs font-bold',
                                                c.grants ? 'bg-emerald-950/60 text-status-healthy' : 'bg-space-800 text-slate-600'
                                            )}>
                                                {c.grants ? '✓' : '·'}
                                            </span>
                                            <span className="text-xs font-mono text-slate-400 truncate">{c.bindingKind}/{c.bindingName}</span>
                                            <ArrowRight className="w-3 h-3 text-slate-700 flex-shrink-0" />
                                            <span className="text-xs font-mono text-slate-500 truncate">{c.roleKind}/{c.roleName}</span>
                                            <span className={clsx(
                                                'ml-auto text-[10px] rounded-full px-2 py-0.5 flex-shrink-0 font-medium',
                                                c.grants
                                                    ? 'text-status-healthy bg-emerald-950/40 border border-emerald-900/30'
                                                    : 'text-slate-600 bg-space-800 border border-space-700'
                                            )}>
                                                {c.grants ? 'grants' : 'no match'}
                                            </span>
                                        </div>
                                    ))}
                                </div>
                            )}
                        </div>
                    </div>
                )
            })()}
        </div>
    )
}

// ─── RBAC Overprivilege Audit ─────────────────────────────────────────────

const LEVEL_META: Record<RBACRiskLevel, { label: string; cls: string; badge: string }> = {
    critical: { label: 'CRITICAL', cls: 'border-red-800/40 bg-red-950/10', badge: 'text-red-400 bg-red-950/50 border-red-800/50' },
    warning: { label: 'WARNING', cls: 'border-amber-800/40 bg-amber-950/10', badge: 'text-amber-400 bg-amber-950/50 border-amber-800/50' },
    info: { label: 'INFO', cls: 'border-space-700 bg-space-900', badge: 'text-slate-500 bg-space-800 border-space-700' },
}

const CATEGORY_LABEL: Record<string, string> = {
    'cluster-admin': 'Cluster Admin',
    'wildcard-all': '* verbs on * resources',
    'wildcard-verb': '* verbs',
    'wildcard-res': '* resources (write)',
    'ghost-account': 'Ghost Account',
}

function RBACRiskAudit() {
    const { context, namespace } = useStore()

    const query = useQuery({
        queryKey: ['rbac-audit', context, namespace],
        queryFn: () => api.rbacAudit(context, namespace),
        enabled: !!context,
        staleTime: 60_000,
    })

    if (!context) return null
    if (query.isLoading) return <div className="flex justify-center py-12"><Loader2 className="w-5 h-5 text-accent animate-spin" /></div>
    if (query.error) return <div className="text-xs text-status-unhealthy">{(query.error as Error).message}</div>

    const data = query.data!
    const findings = data.findings ?? []

    return (
        <div className="space-y-5">
            {/* Header */}
            <div className="flex items-center gap-4">
                <div className="w-9 h-9 rounded-xl bg-red-500/10 border border-red-500/20 flex items-center justify-center">
                    <AlertTriangle className="w-5 h-5 text-red-400" />
                </div>
                <div>
                    <h1 className="text-base font-semibold text-slate-100">RBAC Overprivilege Audit</h1>
                    <p className="text-xs text-slate-600">Scans all bindings for wildcard rules, cluster-admin grants, and ghost accounts</p>
                </div>
                <div className="ml-auto flex items-center gap-2 text-[10px]">
                    {data.critical > 0 && <span className="px-2 py-1 rounded border border-red-900/40 bg-red-950/20 text-red-400 font-bold">{data.critical} critical</span>}
                    {data.warning > 0 && <span className="px-2 py-1 rounded border border-amber-900/40 bg-amber-950/20 text-amber-400">{data.warning} warning</span>}
                    {data.info > 0 && <span className="px-2 py-1 rounded border border-space-700 bg-space-800 text-slate-500">{data.info} info</span>}
                </div>
            </div>

            {findings.length === 0 && (
                <div className="flex flex-col items-center justify-center py-20 text-center">
                    <CheckCircle2 className="w-10 h-10 text-status-healthy mb-3" />
                    <div className="text-sm font-medium text-slate-300">No overprivilege findings</div>
                    <div className="text-xs text-slate-600 mt-1">All RBAC bindings look appropriately scoped</div>
                </div>
            )}

            {(['critical', 'warning', 'info'] as RBACRiskLevel[]).map(level => {
                const group = findings.filter(f => f.level === level)
                if (group.length === 0) return null
                const meta = LEVEL_META[level]
                return (
                    <div key={level} className="space-y-2">
                        <div className="flex items-center gap-2">
                            <span className={clsx('text-[9px] font-bold uppercase tracking-widest px-2 py-0.5 rounded border', meta.badge)}>
                                {meta.label} ({group.length})
                            </span>
                            <div className="flex-1 h-px bg-space-700" />
                        </div>
                        {group.map((f: RBACRiskFinding, i: number) => (
                            <div key={i} className={clsx('rounded-xl border p-4 space-y-2', meta.cls)}>
                                <div className="flex items-start gap-2 flex-wrap">
                                    <span className={clsx('text-[9px] font-bold uppercase px-1.5 py-0.5 rounded border flex-shrink-0', meta.badge)}>
                                        {CATEGORY_LABEL[f.category] ?? f.category}
                                    </span>
                                    <span className="text-xs font-mono text-slate-300 flex-1">{f.subject}</span>
                                </div>
                                <div className="flex items-center gap-2 text-[10px] flex-wrap">
                                    <span className="font-mono text-slate-500">{f.bindingKind}/</span>
                                    <span className="font-mono text-slate-400 font-medium">{f.binding}</span>
                                    <ArrowRight className="w-3 h-3 text-slate-700" />
                                    <span className="font-mono text-slate-500">{f.roleKind}/</span>
                                    <span className="font-mono text-accent/70">{f.role}</span>
                                </div>
                                <p className="text-[11px] text-slate-400">{f.detail}</p>
                                <div className="flex items-start gap-1.5 text-[10px] text-slate-500">
                                    <Zap className="w-3 h-3 text-accent/50 flex-shrink-0 mt-0.5" />
                                    <span>{f.remediation}</span>
                                </div>
                            </div>
                        ))}
                    </div>
                )
            })}
        </div>
    )
}

// suppress unused import
void Ghost
