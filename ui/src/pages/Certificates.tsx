import { useQuery } from '@tanstack/react-query'
import { Loader2, RefreshCw, ShieldAlert, ShieldCheck, ShieldX, Clock, Calendar } from 'lucide-react'
import clsx from 'clsx'
import { api } from '../api/client'
import { useStore } from '../store/useStore'
import type { CertInfo, CertSeverity } from '../types/api'

const SEV_META: Record<CertSeverity, { icon: React.ReactNode; label: string; row: string; badge: string }> = {
    critical: {
        icon: <ShieldX className="w-4 h-4" />,
        label: 'CRITICAL',
        row: 'border-red-800/40 bg-red-950/10',
        badge: 'bg-red-950/50 text-red-300 border-red-800/50',
    },
    warning: {
        icon: <ShieldAlert className="w-4 h-4" />,
        label: 'WARNING',
        row: 'border-amber-800/40 bg-amber-950/10',
        badge: 'bg-amber-950/50 text-amber-300 border-amber-800/50',
    },
    watch: {
        icon: <Clock className="w-4 h-4" />,
        label: 'WATCH',
        row: 'border-yellow-800/40 bg-yellow-950/5',
        badge: 'bg-yellow-950/30 text-yellow-400 border-yellow-900/40',
    },
    ok: {
        icon: <ShieldCheck className="w-4 h-4" />,
        label: 'OK',
        row: 'border-space-700 bg-space-900',
        badge: 'bg-emerald-950/30 text-emerald-400 border-emerald-900/30',
    },
}

function daysBar(daysLeft: number, expired: boolean): React.ReactNode {
    if (expired) return <span className="text-red-400 font-bold text-xs">EXPIRED</span>
    const total = 365
    const pct = Math.min(100, Math.max(0, (daysLeft / total) * 100))
    const color = daysLeft <= 7 ? '#f87171' : daysLeft <= 30 ? '#fb923c' : daysLeft <= 90 ? '#facc15' : '#34d399'
    return (
        <div className="flex items-center gap-2">
            <div className="w-24 h-1.5 bg-space-800 rounded-full overflow-hidden">
                <div className="h-full rounded-full" style={{ width: `${pct}%`, backgroundColor: color }} />
            </div>
            <span className="text-[11px] font-mono" style={{ color }}>{daysLeft}d left</span>
        </div>
    )
}

export function Certificates() {
    const { context, namespace } = useStore()

    const query = useQuery({
        queryKey: ['certs', context, namespace],
        queryFn: () => api.certs(context, namespace),
        enabled: !!context,
        staleTime: 120_000,
        refetchInterval: 300_000,
    })

    if (!context) return null
    if (query.isLoading) return <div className="flex-1 flex items-center justify-center"><Loader2 className="w-5 h-5 text-accent animate-spin" /></div>
    if (query.error) return <div className="flex-1 flex items-center justify-center text-xs text-status-unhealthy">{(query.error as Error).message}</div>

    const data = query.data!
    const certs = data.certs ?? []

    const groups: { sev: CertSeverity; label: string; items: CertInfo[] }[] = [
        { sev: 'critical', label: 'Critical / Expired', items: certs.filter(c => c.severity === 'critical') },
        { sev: 'warning',  label: 'Expiring Soon (≤30d)', items: certs.filter(c => c.severity === 'warning') },
        { sev: 'watch',    label: 'Watch (≤90d)',          items: certs.filter(c => c.severity === 'watch') },
        { sev: 'ok',       label: 'Healthy',               items: certs.filter(c => c.severity === 'ok') },
    ]

    return (
        <div className="flex-1 overflow-y-auto p-6 space-y-5">
            {/* Header */}
            <div className="flex items-center gap-4">
                <div className="w-9 h-9 rounded-xl bg-amber-500/10 border border-amber-500/20 flex items-center justify-center flex-shrink-0">
                    <ShieldAlert className="w-5 h-5 text-amber-400" />
                </div>
                <div>
                    <h1 className="text-base font-semibold text-slate-100">TLS Certificate Expiry</h1>
                    <p className="text-xs text-slate-600">Scans all <span className="font-mono">kubernetes.io/tls</span> Secrets · refreshes every 5m</p>
                </div>
                <div className="ml-auto flex items-center gap-3">
                    {data.expired > 0 && (
                        <span className="text-[10px] px-2 py-1 rounded border border-red-900/40 bg-red-950/20 text-red-400 font-bold">
                            {data.expired} EXPIRED
                        </span>
                    )}
                    {data.critical > 0 && (
                        <span className="text-[10px] px-2 py-1 rounded border border-red-900/40 bg-red-950/20 text-red-400">
                            {data.critical} critical
                        </span>
                    )}
                    {data.warning > 0 && (
                        <span className="text-[10px] px-2 py-1 rounded border border-amber-900/40 bg-amber-950/20 text-amber-400">
                            {data.warning} warning
                        </span>
                    )}
                    <button onClick={() => query.refetch()} className="p-1.5 rounded border border-space-700 text-slate-600 hover:text-accent transition-colors">
                        <RefreshCw className={`w-3.5 h-3.5 ${query.isFetching ? 'animate-spin text-accent' : ''}`} />
                    </button>
                </div>
            </div>

            {certs.length === 0 && (
                <div className="flex flex-col items-center justify-center py-20 text-center">
                    <ShieldCheck className="w-10 h-10 text-status-healthy mb-3" />
                    <div className="text-sm font-medium text-slate-300">No TLS Secrets found</div>
                    <div className="text-xs text-slate-600 mt-1">No Secrets of type kubernetes.io/tls in scope</div>
                </div>
            )}

            {groups.map(({ sev, label, items }) => {
                if (items.length === 0) return null
                const meta = SEV_META[sev]
                return (
                    <div key={sev} className="space-y-2">
                        <div className="flex items-center gap-2">
                            <span className={clsx('flex items-center gap-1.5 text-[10px] font-bold uppercase tracking-widest px-2 py-1 rounded border', meta.badge)}>
                                {meta.icon}{label}
                            </span>
                            <span className="text-[10px] text-slate-600">({items.length})</span>
                            <div className="flex-1 h-px bg-space-700" />
                        </div>
                        {items.map(cert => <CertCard key={`${cert.namespace}/${cert.secretName}`} cert={cert} />)}
                    </div>
                )
            })}
        </div>
    )
}

function CertCard({ cert }: { cert: CertInfo }) {
    const meta = SEV_META[cert.severity]
    return (
        <div className={clsx('rounded-xl border p-4 space-y-2', meta.row)}>
            {cert.parseError ? (
                <div className="flex items-center gap-3">
                    <div className={clsx('text-xs font-bold flex-shrink-0', meta.badge.includes('red') ? 'text-red-400' : 'text-amber-400')}>
                        {cert.namespace}/{cert.secretName}
                    </div>
                    <div className="text-xs text-status-unhealthy ml-auto">{cert.parseError}</div>
                </div>
            ) : (
                <>
                    <div className="flex items-start gap-3">
                        <div className="flex-1 min-w-0 space-y-0.5">
                            <div className="flex items-center gap-2 flex-wrap">
                                <span className="text-sm font-mono font-medium text-slate-200">{cert.commonName || cert.secretName}</span>
                                <span className={clsx('text-[9px] font-bold uppercase tracking-widest px-1.5 py-0.5 rounded border', meta.badge)}>
                                    {meta.label}
                                </span>
                            </div>
                            <div className="text-[10px] font-mono text-slate-600">{cert.namespace}/{cert.secretName}</div>
                            {cert.dnsNames?.length > 0 && (
                                <div className="flex flex-wrap gap-1 mt-1">
                                    {cert.dnsNames.slice(0, 5).map(d => (
                                        <span key={d} className="text-[9px] font-mono px-1.5 py-0.5 rounded bg-space-800 border border-space-700 text-slate-500">{d}</span>
                                    ))}
                                    {cert.dnsNames.length > 5 && <span className="text-[9px] text-slate-600">+{cert.dnsNames.length - 5}</span>}
                                </div>
                            )}
                        </div>
                        <div className="flex-shrink-0 text-right space-y-1">
                            {daysBar(cert.daysLeft, cert.expired)}
                            <div className="flex items-center gap-1 text-[10px] text-slate-600 justify-end">
                                <Calendar className="w-3 h-3" />
                                {new Date(cert.notAfter).toLocaleDateString()}
                            </div>
                        </div>
                    </div>
                    {cert.issuer && cert.issuer !== cert.commonName && (
                        <div className="text-[10px] text-slate-600">Issued by: <span className="text-slate-500">{cert.issuer}</span></div>
                    )}
                </>
            )}
        </div>
    )
}
