import { memo } from 'react'
import React from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import clsx from 'clsx'
import { kindIcon, kindColor, kindBg, hexToRgba } from '../lib/kinds'
import { Lock } from 'lucide-react'
import type { NodeInfo } from '../types/api'

type ResourceNodeData = NodeInfo & { selected?: boolean }

// Invisible handle style — connection points exist on all 4 sides of every node
// so dagre’s dynamic handle selection can route edges through the nearest face.
const H: React.CSSProperties = { opacity: 0, width: 6, height: 6, background: 'transparent', border: 'none' }

export const ResourceNode = memo(({ data, selected }: NodeProps & { data: ResourceNodeData }) => {
    const color = kindColor(data.kind)
    const bg = kindBg(data.kind)
    const Icon = kindIcon(data.kind)

    if (data.kind === 'Pod') {
        return (
            <div
                className={clsx(
                    'relative flex flex-col items-center justify-center text-center transition-all duration-200 cursor-pointer select-none rounded-full',
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
                <div className={clsx("absolute inset-0 rounded-full", !data.healthy && "animate-pulse ring-4 ring-red-500/50 shadow-[0_0_15px_rgba(248,113,113,0.6)]")} />
                {Icon && <Icon className="w-5 h-5 mb-1 z-10" style={{ color }} strokeWidth={2} />}
                <div className="text-[9px] font-semibold tracking-widest uppercase mb-0.5 z-10" style={{ color }}>POD</div>
                <div className="text-xs font-semibold text-white truncate w-full px-3 leading-snug z-10">{data.name}</div>
                {data.namespace && (
                    <div className="text-[9px] text-slate-500 font-mono truncate mt-0.5 w-full px-3 z-10">{data.namespace}</div>
                )}
                {!data.healthy && data.reason && (
                    <span className="text-[9px] text-status-unhealthy font-mono truncate px-3 mt-1 z-10">{data.reason}</span>
                )}
                {/* All 4 bidirectional handles so dagre can route edges correctly */}
                <Handle id="top-s" type="source" position={Position.Top} style={H} />
                <Handle id="top-t" type="target" position={Position.Top} style={H} />
                <Handle id="bottom-s" type="source" position={Position.Bottom} style={H} />
                <Handle id="bottom-t" type="target" position={Position.Bottom} style={H} />
                <Handle id="left-s" type="source" position={Position.Left} style={H} />
                <Handle id="left-t" type="target" position={Position.Left} style={H} />
                <Handle id="right-s" type="source" position={Position.Right} style={H} />
                <Handle id="right-t" type="target" position={Position.Right} style={H} />
            </div>
        )
    }

    const isSpecialShape = ['Service', 'Ingress', 'NetworkPolicy', 'PersistentVolumeClaim', 'PersistentVolume', 'ConfigMap'].includes(data.kind)
    const isDaemonSet = data.kind === 'DaemonSet'

    const nodeClasses = clsx(
        'relative transition-all duration-200 cursor-pointer select-none',
        'min-w-[200px] max-w-[230px]',
        isDaemonSet ? 'rounded-full px-2' : 'rounded-xl',
        selected ? 'scale-[1.04]' : 'hover:scale-[1.02]'
    )

    let SvgBackground = null
    const strokeColor = selected ? '#38bdf8' : color + '55'
    const fillColor = selected ? bg : hexToRgba(color, 0.05)

    if (data.kind === 'Service') {
        // Hexagon: gentle 10% indent on each side gives enough room for content
        SvgBackground = (
            <svg viewBox="0 0 100 100" preserveAspectRatio="none" className="absolute inset-0 w-full h-full -z-10 drop-shadow-lg">
                <polygon points="10,2 90,2 98,50 90,98 10,98 2,50" fill={fillColor} stroke={strokeColor} strokeWidth="2" vectorEffect="non-scaling-stroke" />
            </svg>
        )
    } else if (data.kind === 'Ingress') {
        // Right-pointing arrow — no left cut-in so content width is full node width.
        // The distinctive shape is the right arrow point.
        SvgBackground = (
            <svg viewBox="0 0 110 100" preserveAspectRatio="none" className="absolute inset-0 w-full h-full -z-10 drop-shadow-lg">
                <polygon points="0,2 88,2 110,50 88,98 0,98" fill={fillColor} stroke={strokeColor} strokeWidth="2" vectorEffect="non-scaling-stroke" />
            </svg>
        )
    } else if (data.kind === 'NetworkPolicy') {
        SvgBackground = (
            <svg viewBox="0 0 100 100" preserveAspectRatio="none" className="absolute inset-0 w-full h-full -z-10 drop-shadow-lg">
                <path d="M 50,2 C 75,2 90,8 98,15 C 98,45 85,80 50,98 C 15,80 2,45 2,15 C 10,8 25,2 50,2 Z" fill={fillColor} stroke={strokeColor} strokeWidth="2" vectorEffect="non-scaling-stroke" />
            </svg>
        )
    } else if (data.kind === 'PersistentVolumeClaim' || data.kind === 'PersistentVolume') {
        SvgBackground = (
            <svg viewBox="0 0 100 100" preserveAspectRatio="none" className="absolute inset-0 w-full h-full -z-10 drop-shadow-lg">
                <path d="M 2,15 L 2,85 A 48,15 0 0 0 98,85 L 98,15 A 48,15 0 0 0 2,15 Z" fill={fillColor} stroke={strokeColor} strokeWidth="2" vectorEffect="non-scaling-stroke" />
                <ellipse cx="50" cy="15" rx="48" ry="13" fill={hexToRgba(color, 0.1)} stroke={strokeColor} strokeWidth="2" vectorEffect="non-scaling-stroke" />
            </svg>
        )
    } else if (data.kind === 'ConfigMap') {
        SvgBackground = (
            <svg viewBox="0 0 100 100" preserveAspectRatio="none" className="absolute inset-0 w-full h-full -z-10 drop-shadow-lg">
                <polygon points="2,2 80,2 98,20 98,98 2,98" fill={fillColor} stroke={strokeColor} strokeWidth="2" vectorEffect="non-scaling-stroke" />
                <polygon points="80,2 80,20 98,20" fill={hexToRgba(color, 0.15)} stroke={strokeColor} strokeWidth="2" vectorEffect="non-scaling-stroke" />
            </svg>
        )
    }

    const shortKindName = data.kind.replace('PersistentVolumeClaim', 'PVC').replace('PersistentVolume', 'PV').replace('ServiceAccount', 'SA').replace('NetworkPolicy', 'NetPolicy')

    return (
        <div
            className={nodeClasses}
            style={!SvgBackground ? {
                background: selected ? `linear-gradient(135deg, ${bg}, rgba(56,189,248,0.06))` : bg,
                border: `2px solid ${strokeColor}`,
                boxShadow: selected ? '0 0 20px rgba(56,189,248,0.4)' : undefined,
            } : {
                boxShadow: selected ? '0 0 20px rgba(56,189,248,0.4)' : undefined,
                minHeight: isSpecialShape ? '85px' : undefined,
            }}
        >
            {data.kind === 'StatefulSet' && !SvgBackground && (
                <>
                    <div className="absolute inset-0 translate-y-1.5 translate-x-1.5 border rounded-xl -z-10 transition-all" style={{ borderColor: hexToRgba(color, 0.2), background: hexToRgba(color, 0.05) }} />
                    <div className="absolute inset-0 translate-y-3 translate-x-3 border rounded-xl -z-20 transition-all" style={{ borderColor: hexToRgba(color, 0.1), background: hexToRgba(color, 0.02) }} />
                </>
            )}

            {data.kind === 'Node' && !SvgBackground && (
                <div className="absolute right-3 top-3 bottom-3 w-4 flex flex-col justify-between opacity-30 z-0">
                    <div className="h-1 rounded-sm w-full" style={{ background: color }} />
                    <div className="h-1 rounded-sm w-full" style={{ background: color }} />
                    <div className="h-1 rounded-sm w-full" style={{ background: color }} />
                </div>
            )}

            {data.kind === 'Deployment' && !SvgBackground && (
                <div className="absolute left-0 top-[-2px] bottom-[-2px] w-[6px] rounded-l-lg z-0" style={{ background: color }} />
            )}

            {SvgBackground}

            <div className={clsx("relative z-10", isSpecialShape ? "p-5" : "px-3 py-2.5", data.kind === 'Deployment' ? 'ml-1' : '')}>
                {/* Kind row */}
                <div className="flex items-center gap-1.5 mb-1.5">
                    {Icon && <Icon className="w-4 h-4" style={{ color }} strokeWidth={2.5} />}
                    <span className="text-[11px] font-semibold tracking-widest uppercase" style={{ color }}>
                        {shortKindName}
                    </span>

                    {/* Badges */}
                    {data.kind === 'Job' && (
                        <div className="ml-auto flex items-center justify-center w-5 h-5 rounded bg-space-900 border" style={{ borderColor: color }}>
                            {Icon && <Icon className="w-3 h-3" style={{ color }} />}
                        </div>
                    )}
                    {data.kind === 'CronJob' && (
                        <div className="ml-auto flex items-center justify-center w-5 h-5 rounded bg-space-900 border" style={{ borderColor: color }}>
                            {Icon && <Icon className="w-3 h-3" style={{ color }} />}
                        </div>
                    )}
                    {data.kind === 'Secret' && (
                        <div className="ml-auto text-red-400 opacity-80">
                            <Lock className="w-4 h-4" />
                        </div>
                    )}
                </div>

                <div className="text-sm font-semibold text-white truncate leading-snug">
                    {data.name}
                </div>

                {data.namespace && (
                    <div className="text-[10px] text-slate-500 font-mono truncate mt-0.5">
                        {data.namespace}
                    </div>
                )}

                <div className="mt-2 flex items-center gap-2 flex-wrap">
                    {(data.kind === 'Deployment' || data.kind === 'StatefulSet') && data.fields?.replicas !== undefined && (
                        <span className={clsx(
                            'text-[10px] font-mono px-1.5 py-0.5 rounded border',
                            data.fields?.readyReplicas === data.fields?.replicas
                                ? 'text-status-healthy bg-emerald-950/50 border-emerald-900'
                                : 'text-status-warning bg-amber-950/50 border-amber-900'
                        )}>
                            {data.fields?.readyReplicas ?? '0'}/{data.fields?.replicas} {data.fields?.readyReplicas === data.fields?.replicas ? '✓' : '✖'}
                        </span>
                    )}
                    {data.kind === 'Service' && data.fields?.type && (
                        <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-space-800 text-slate-300 border border-space-600">
                            {data.fields.type}
                        </span>
                    )}
                    {data.kind === 'PersistentVolumeClaim' && data.fields?.phase && (
                        <span className={clsx(
                            'text-[10px] font-mono rounded px-1.5 py-0.5 border',
                            data.fields.phase === 'Bound' ? 'text-status-healthy border-emerald-900 bg-emerald-950/30' : 'text-status-warning border-amber-900 bg-amber-950/30'
                        )}>{data.fields.phase}</span>
                    )}
                </div>
            </div>

            <Handle id="top-s" type="source" position={Position.Top} style={H} />
            <Handle id="top-t" type="target" position={Position.Top} style={H} />
            <Handle id="bottom-s" type="source" position={Position.Bottom} style={H} />
            <Handle id="bottom-t" type="target" position={Position.Bottom} style={H} />
            <Handle id="left-s" type="source" position={Position.Left} style={H} />
            <Handle id="left-t" type="target" position={Position.Left} style={H} />
            <Handle id="right-s" type="source" position={Position.Right} style={H} />
            <Handle id="right-t" type="target" position={Position.Right} style={H} />
        </div>
    )
})

ResourceNode.displayName = 'ResourceNode'
