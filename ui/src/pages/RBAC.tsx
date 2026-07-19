import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { api } from '../api/client'
import { useStore } from '../store/useStore'

const COMMON_VERBS = ['get', 'list', 'watch', 'create', 'update', 'patch', 'delete']
const COMMON_RESOURCES = ['pods', 'secrets', 'configmaps', 'deployments', 'services', 'nodes', 'namespaces', 'jobs', 'cronjobs']

export function RBAC() {
    const { context, namespace } = useStore()
    const [sa, setSa] = useState('')
    const [verb, setVerb] = useState('get')
    const [resource, setResource] = useState('secrets')
    const [apiGroup, setApiGroup] = useState('')
    const [submitted, setSubmitted] = useState(false)
    const [queryKey, setQueryKey] = useState<unknown[]>([])

    const query = useQuery({
        queryKey: ['rbac', ...queryKey],
        queryFn: () => api.rbac(context, namespace || 'default', sa, verb, resource, apiGroup),
        enabled: submitted && !!sa,
        staleTime: 60_000,
        retry: false,
    })

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault()
        setQueryKey([context, namespace, sa, verb, resource, apiGroup, Date.now()])
        setSubmitted(true)
    }

    return (
        <div className="flex-1 overflow-y-auto p-6 max-w-2xl">
            <h1 className="text-base font-semibold text-slate-100 mb-6">RBAC Path Tracer</h1>

            <form onSubmit={handleSubmit} className="space-y-4 mb-6">
                <div className="grid grid-cols-2 gap-3">
                    <div>
                        <label className="block text-xs text-slate-500 mb-1">ServiceAccount name</label>
                        <input
                            type="text"
                            value={sa}
                            onChange={e => setSa(e.target.value)}
                            placeholder="e.g. api-worker"
                            className="w-full bg-space-800 border border-space-700 rounded px-3 py-2 text-sm text-slate-200 focus:outline-none focus:border-accent placeholder-slate-600"
                        />
                    </div>
                    <div>
                        <label className="block text-xs text-slate-500 mb-1">Namespace</label>
                        <input
                            type="text"
                            value={namespace || 'default'}
                            readOnly
                            className="w-full bg-space-850 border border-space-700 rounded px-3 py-2 text-sm text-slate-500 cursor-not-allowed"
                        />
                    </div>
                </div>

                <div className="grid grid-cols-3 gap-3">
                    <div>
                        <label className="block text-xs text-slate-500 mb-1">Verb</label>
                        <select
                            value={verb}
                            onChange={e => setVerb(e.target.value)}
                            className="w-full bg-space-800 border border-space-700 rounded px-3 py-2 text-sm text-slate-200 focus:outline-none focus:border-accent"
                        >
                            {COMMON_VERBS.map(v => <option key={v} value={v}>{v}</option>)}
                        </select>
                    </div>
                    <div>
                        <label className="block text-xs text-slate-500 mb-1">Resource</label>
                        <select
                            value={resource}
                            onChange={e => setResource(e.target.value)}
                            className="w-full bg-space-800 border border-space-700 rounded px-3 py-2 text-sm text-slate-200 focus:outline-none focus:border-accent"
                        >
                            {COMMON_RESOURCES.map(r => <option key={r} value={r}>{r}</option>)}
                        </select>
                    </div>
                    <div>
                        <label className="block text-xs text-slate-500 mb-1">API Group <span className="text-slate-600">(optional)</span></label>
                        <input
                            type="text"
                            value={apiGroup}
                            onChange={e => setApiGroup(e.target.value)}
                            placeholder="apps, batch…"
                            className="w-full bg-space-800 border border-space-700 rounded px-3 py-2 text-sm text-slate-200 focus:outline-none focus:border-accent placeholder-slate-600"
                        />
                    </div>
                </div>

                <button
                    type="submit"
                    disabled={!sa}
                    className="px-4 py-2 rounded bg-accent/20 border border-accent/30 text-accent text-sm font-medium hover:bg-accent/30 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                >
                    Check Permission
                </button>
            </form>

            {query.isLoading && (
                <div className="flex items-center gap-2 text-xs text-slate-500">
                    <Loader2 className="w-4 h-4 text-accent animate-spin" />
                    Evaluating RBAC policies…
                </div>
            )}

            {query.error && (
                <div className="rounded-lg border border-red-900/40 bg-red-950/10 p-3 text-xs text-status-unhealthy">
                    {(query.error as Error).message}
                </div>
            )}

            {query.data && (
                <div className="space-y-4">
                    {/* Verdict */}
                    <div className={`rounded-lg border p-4 flex items-center gap-3 ${query.data.verdict === 'GRANTED'
                            ? 'border-emerald-900/50 bg-emerald-950/10'
                            : 'border-red-900/50 bg-red-950/10'
                        }`}>
                        <span className={`text-2xl ${query.data.verdict === 'GRANTED' ? 'text-status-healthy' : 'text-status-unhealthy'}`}>
                            {query.data.verdict === 'GRANTED' ? '✓' : '✖'}
                        </span>
                        <div>
                            <div className={`text-sm font-semibold ${query.data.verdict === 'GRANTED' ? 'text-status-healthy' : 'text-status-unhealthy'}`}>
                                {query.data.verdict}
                            </div>
                            <div className="text-xs text-slate-500 font-mono">
                                {verb} {resource} {apiGroup ? `(${apiGroup})` : '(core)'}
                            </div>
                        </div>
                    </div>

                    {/* Grant paths */}
                    {query.data.paths && query.data.paths.length > 0 && (
                        <div>
                            <div className="text-xs uppercase tracking-widest text-accent/70 mb-2">Grant paths</div>
                            {query.data.paths.map((p, i) => (
                                <div key={i} className="rounded-lg border border-space-700 bg-space-850 p-3 mb-2 space-y-1">
                                    <div className="text-xs font-mono">
                                        <span className="text-slate-500">{p.bindingKind}/</span>
                                        <span className="text-accent">{p.bindingName}</span>
                                        {p.bindingNs && <span className="text-slate-600"> ({p.bindingNs})</span>}
                                    </div>
                                    <div className="text-xs text-slate-500">
                                        → {p.roleKind}/<span className="text-slate-300">{p.roleName}</span>
                                        <span className="text-status-healthy ml-2">rules[{p.ruleIndex}] ✓</span>
                                    </div>
                                </div>
                            ))}
                        </div>
                    )}

                    {/* Binding evaluation */}
                    <div>
                        <div className="text-xs uppercase tracking-widest text-accent/70 mb-2">Binding evaluation</div>
                        {query.data.checks.filter(c => c.subjectMatch).map((c, i) => (
                            <div key={i} className="flex items-center gap-2 text-xs py-1.5 border-b border-space-800">
                                <span className={c.grants ? 'text-status-healthy' : 'text-status-unhealthy'}>
                                    {c.grants ? '✓' : '✖'}
                                </span>
                                <span className="font-mono text-slate-400">{c.bindingKind}/{c.bindingName}</span>
                                <span className="text-slate-600">→</span>
                                <span className="font-mono text-slate-500">{c.roleKind}/{c.roleName}</span>
                                <span className="ml-auto text-slate-600 text-[10px]">
                                    {c.grants ? 'grants' : 'no matching rule'}
                                </span>
                            </div>
                        ))}
                        {query.data.checks.filter(c => c.subjectMatch).length === 0 && (
                            <div className="text-xs text-slate-600">No bindings reference this ServiceAccount</div>
                        )}
                    </div>
                </div>
            )}
        </div>
    )
}
