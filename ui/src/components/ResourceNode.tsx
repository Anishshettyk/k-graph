import { memo } from 'react'
import React from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import clsx from 'clsx'
import {
    kindIcon, kindColor, kindBg, hexToRgba,
    ServiceIcon, IngressIcon, PVCIcon, PVIcon,
    NetworkPolicyIcon, ConfigMapIcon, SecretIcon,
    type SvgIconProps,
} from '../lib/kinds'
import type { NodeInfo } from '../types/api'

type ResourceNodeData = NodeInfo & { selected?: boolean; dimmed?: boolean; heatmapColor?: string }

const H: React.CSSProperties = { opacity: 0, width: 6, height: 6, background: 'transparent', border: 'none' }

// Eight invisible handles so dagre can route edges from any face.
function AllHandles() {
    return (
        <>
            <Handle id="top-s" type="source" position={Position.Top} style={H} />
            <Handle id="top-t" type="target" position={Position.Top} style={H} />
            <Handle id="bottom-s" type="source" position={Position.Bottom} style={H} />
            <Handle id="bottom-t" type="target" position={Position.Bottom} style={H} />
            <Handle id="left-s" type="source" position={Position.Left} style={H} />
            <Handle id="left-t" type="target" position={Position.Left} style={H} />
            <Handle id="right-s" type="source" position={Position.Right} style={H} />
            <Handle id="right-t" type="target" position={Position.Right} style={H} />
        </>
    )
}

// ─── Custom-designed cards for special Kubernetes kinds ───────────────────────

/** Shared base for Service / Ingress / PVC / PV / NetworkPolicy / ConfigMap / Secret */
function SpecialCard({
    data, selected, CustomIcon, kindLabel, badge, extra,
}: {
    data: ResourceNodeData
    selected?: boolean
    CustomIcon: React.ComponentType<SvgIconProps>
    kindLabel: string
    badge?: React.ReactNode
    extra?: React.ReactNode
}) {
    const color = kindColor(data.kind)

    return (
        <div
            className={clsx(
                'relative cursor-pointer select-none transition-all duration-200',
                'w-[210px] rounded-xl overflow-hidden',
                selected ? 'scale-[1.04]' : 'hover:scale-[1.02]'
            )}
            style={{
                background: selected
                    ? `linear-gradient(145deg, ${hexToRgba(color, 0.14)}, ${hexToRgba(color, 0.06)})`
                    : `linear-gradient(145deg, ${hexToRgba(color, 0.1)}, ${hexToRgba(color, 0.04)})`,
                border: `1.5px solid ${selected ? '#38bdf8' : hexToRgba(color, 0.4)}`,
                boxShadow: selected
                    ? `0 0 24px rgba(56,189,248,0.35), inset 0 1px 0 ${hexToRgba(color, 0.3)}`
                    : `0 4px 20px rgba(0,0,0,0.4), inset 0 1px 0 ${hexToRgba(color, 0.15)}`,
            }}
        >
            {/* Top gradient bar */}
            <div className="h-0.5 w-full" style={{ background: `linear-gradient(90deg, ${color}, ${color}44, transparent)` }} />

            {/* Icon + kind header */}
            <div className="flex items-center gap-2.5 px-3 pt-2.5 pb-1">
                <div
                    className="flex-shrink-0 w-9 h-9 rounded-lg flex items-center justify-center"
                    style={{
                        background: hexToRgba(color, 0.15),
                        border: `1px solid ${hexToRgba(color, 0.3)}`,
                        boxShadow: `0 0 12px ${hexToRgba(color, 0.2)}`,
                    }}
                >
                    <CustomIcon color={color} size={20} />
                </div>
                <div className="min-w-0 flex-1">
                    <div className="text-[9px] font-bold tracking-[0.18em] uppercase" style={{ color: hexToRgba(color, 0.85) }}>
                        {kindLabel}
                    </div>
                    <div className="text-[13px] font-bold text-white truncate leading-tight mt-0.5">
                        {data.name}
                    </div>
                </div>
            </div>

            {/* Namespace + badges */}
            <div className="px-3 pb-2.5 space-y-1.5">
                {data.namespace && (
                    <div className="text-[10px] font-mono text-slate-500 truncate">{data.namespace}</div>
                )}
                {badge && <div className="flex flex-wrap gap-1.5">{badge}</div>}
                {extra}
            </div>

            <AllHandles />
        </div>
    )
}

function StatusBadge({ text, ok }: { text: string; ok: boolean }) {
    return (
        <span className={clsx(
            'text-[10px] font-mono px-1.5 py-0.5 rounded border',
            ok ? 'text-emerald-400 bg-emerald-950/40 border-emerald-800'
                : 'text-amber-400 bg-amber-950/40 border-amber-800'
        )}>{text}</span>
    )
}

export const ResourceNode = memo(({ data, selected }: NodeProps & { data: ResourceNodeData }) => {
    const color = data.heatmapColor ?? kindColor(data.kind)
    const bg = kindBg(data.kind)
    const Icon = kindIcon(data.kind)
    const dimmed = data.dimmed && !selected

    // Wrap everything in a dimming container so hover-highlighting works without
    // re-running dagre. Opacity transition keeps it smooth.
    const wrapper = (children: React.ReactNode) => (
        <div style={{ opacity: dimmed ? 0.18 : 1, transition: 'opacity 0.15s ease', pointerEvents: dimmed ? 'none' : undefined }}>
            {children}
        </div>
    )

    // ── Pod — circle ─────────────────────────────────────────────────────────
    if (data.kind === 'Pod') {
        return wrapper(
            <div
                className={clsx(
                    'relative flex flex-col items-center justify-center text-center',
                    'transition-all duration-200 cursor-pointer select-none rounded-full',
                    'w-[130px] h-[130px]',
                    selected ? 'scale-[1.04]' : 'hover:scale-[1.02]'
                )}
                style={{
                    background: selected ? `linear-gradient(135deg, ${bg}, rgba(56,189,248,0.06))` : bg,
                    border: `2px solid ${selected ? '#38bdf8' : color + '55'}`,
                    boxShadow: selected
                        ? '0 0 20px rgba(56,189,248,0.4)'
                        : data.healthy ? `0 0 10px ${hexToRgba(color, 0.35)}` : undefined,
                }}
            >
                <div className={clsx('absolute inset-0 rounded-full', !data.healthy && 'animate-pulse ring-4 ring-red-500/50 shadow-[0_0_15px_rgba(248,113,113,0.6)]')} />
                {Icon && <Icon className="w-5 h-5 mb-1 z-10" style={{ color }} strokeWidth={2} />}
                <div className="text-[9px] font-semibold tracking-widest uppercase mb-0.5 z-10" style={{ color }}>POD</div>
                <div className="text-xs font-semibold text-white truncate w-full px-3 leading-snug z-10">{data.name}</div>
                {data.namespace && <div className="text-[9px] text-slate-500 font-mono truncate mt-0.5 w-full px-3 z-10">{data.namespace}</div>}
                {!data.healthy && data.reason && <span className="text-[9px] text-status-unhealthy font-mono truncate px-3 mt-1 z-10">{data.reason}</span>}
                <AllHandles />
            </div>
        )
    }

    // ── Service — hub card ────────────────────────────────────────────────────
    if (data.kind === 'Service') {
        return wrapper(
            <SpecialCard data={data} selected={selected} CustomIcon={ServiceIcon} kindLabel="Service"
                badge={data.fields?.type && <StatusBadge text={data.fields.type} ok />}
            />
        )
    }

    // ── Ingress — gateway card ────────────────────────────────────────────────
    if (data.kind === 'Ingress') {
        return (
            <SpecialCard data={data} selected={selected} CustomIcon={IngressIcon} kindLabel="Ingress"
                badge={data.fields?.hosts && (
                    <span className="text-[10px] font-mono text-slate-400 truncate">{data.fields.hosts.split(',')[0]}</span>
                )}
            />
        )
    }

    // ── PVC — storage claim card ──────────────────────────────────────────────
    if (data.kind === 'PersistentVolumeClaim') {
        const phase = data.fields?.phase ?? ''
        return (
            <SpecialCard data={data} selected={selected} CustomIcon={PVCIcon} kindLabel="PVC"
                badge={<>
                    {phase && <StatusBadge text={phase} ok={phase === 'Bound'} />}
                    {data.fields?.capacity && <span className="text-[10px] font-mono text-slate-500">{data.fields.capacity}</span>}
                </>}
            />
        )
    }

    // ── PV — physical volume card ─────────────────────────────────────────────
    if (data.kind === 'PersistentVolume') {
        const phase = data.fields?.phase ?? ''
        return (
            <SpecialCard data={data} selected={selected} CustomIcon={PVIcon} kindLabel="PersistentVolume"
                badge={<>
                    {phase && <StatusBadge text={phase} ok={phase === 'Bound' || phase === 'Available'} />}
                    {data.fields?.capacity && <span className="text-[10px] font-mono text-slate-500">{data.fields.capacity}</span>}
                </>}
            />
        )
    }

    // ── NetworkPolicy — shield card ───────────────────────────────────────────
    if (data.kind === 'NetworkPolicy') {
        return (
            <SpecialCard data={data} selected={selected} CustomIcon={NetworkPolicyIcon} kindLabel="NetworkPolicy" />
        )
    }

    // ── ConfigMap ─────────────────────────────────────────────────────────────
    if (data.kind === 'ConfigMap') {
        return (
            <SpecialCard data={data} selected={selected} CustomIcon={ConfigMapIcon} kindLabel="ConfigMap" />
        )
    }

    // ── Secret ────────────────────────────────────────────────────────────────
    if (data.kind === 'Secret') {
        return (
            <SpecialCard data={data} selected={selected} CustomIcon={SecretIcon} kindLabel="Secret" />
        )
    }

    // ── Generic rectangular card (Deployment, StatefulSet, DaemonSet, etc.) ──
    const shortKind = data.kind
        .replace('PersistentVolumeClaim', 'PVC')
        .replace('PersistentVolume', 'PV')
        .replace('ServiceAccount', 'SA')
        .replace('ReplicaSet', 'RS')

    return (
        <div
            className={clsx(
                'relative transition-all duration-200 cursor-pointer select-none',
                'min-w-[200px] max-w-[230px] rounded-xl',
                selected ? 'scale-[1.04]' : 'hover:scale-[1.02]'
            )}
            style={{
                background: selected ? `linear-gradient(135deg, ${bg}, rgba(56,189,248,0.06))` : bg,
                border: `2px solid ${selected ? '#38bdf8' : color + '55'}`,
                boxShadow: selected ? '0 0 20px rgba(56,189,248,0.4)' : undefined,
            }}
        >
            <div className="h-1 w-full rounded-t-xl" style={{ background: `linear-gradient(90deg, ${color}, ${color}44)` }} />
            <div className="px-3 py-2.5">
                <div className="flex items-center gap-1.5 mb-1.5">
                    {Icon && <Icon className="w-4 h-4" style={{ color }} strokeWidth={2.5} />}
                    <span className="text-[11px] font-semibold tracking-widest uppercase" style={{ color }}>{shortKind}</span>
                </div>
                <div className="text-sm font-semibold text-white truncate leading-snug">{data.name}</div>
                {data.namespace && <div className="text-[10px] text-slate-500 font-mono truncate mt-0.5">{data.namespace}</div>}
                <div className="mt-1.5 flex flex-wrap gap-1.5">
                    {(data.kind === 'Deployment' || data.kind === 'StatefulSet') && data.fields?.replicas !== undefined && (
                        <StatusBadge
                            text={`${data.fields?.readyReplicas ?? '0'}/${data.fields?.replicas} ${data.fields?.readyReplicas === data.fields?.replicas ? '✓' : '✖'}`}
                            ok={data.fields?.readyReplicas === data.fields?.replicas}
                        />
                    )}
                </div>
            </div>
            <AllHandles />
        </div>
    )
})

ResourceNode.displayName = 'ResourceNode'

