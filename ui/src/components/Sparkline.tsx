/** Minimal SVG sparkline — no library needed. */
export function Sparkline({
    values,
    color,
    width = 80,
    height = 28,
    threshold75 = false,
    threshold90 = false,
}: {
    values: number[]
    color: string
    width?: number
    height?: number
    threshold75?: boolean
    threshold90?: boolean
}) {
    if (values.length < 2) {
        return <svg width={width} height={height} />
    }
    const max = Math.max(...values, 100)
    const pts = values
        .map((v, i) => {
            const x = (i / (values.length - 1)) * (width - 2) + 1
            const y = height - 2 - (Math.min(v, max) / max) * (height - 4)
            return `${x.toFixed(1)},${y.toFixed(1)}`
        })
        .join(' ')

    const last = values[values.length - 1]
    const dotColor = last > 90 ? '#f87171' : last > 75 ? '#fbbf24' : color

    const lastX = width - 1
    const lastY = height - 2 - (Math.min(last, max) / max) * (height - 4)

    return (
        <svg width={width} height={height} className="flex-shrink-0 overflow-visible">
            {/* threshold lines */}
            {threshold90 && (
                <line
                    x1="0" y1={(height - 2 - 0.9 * (height - 4)).toFixed(1)}
                    x2={width} y2={(height - 2 - 0.9 * (height - 4)).toFixed(1)}
                    stroke="#f87171" strokeWidth="0.5" strokeDasharray="2 2" opacity={0.4}
                />
            )}
            {threshold75 && (
                <line
                    x1="0" y1={(height - 2 - 0.75 * (height - 4)).toFixed(1)}
                    x2={width} y2={(height - 2 - 0.75 * (height - 4)).toFixed(1)}
                    stroke="#fbbf24" strokeWidth="0.5" strokeDasharray="2 2" opacity={0.4}
                />
            )}
            {/* sparkline */}
            <polyline
                points={pts}
                fill="none"
                stroke={color}
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
                opacity={0.8}
            />
            {/* last value dot */}
            <circle cx={lastX} cy={lastY.toFixed(1)} r="2.5" fill={dotColor} />
        </svg>
    )
}

/** Horizontal progress bar showing pct of limit. */
export function UsageBar({ pct, width = 80 }: { pct: number; width?: number }) {
    const clamped = Math.min(Math.max(pct, 0), 130)
    const fill = Math.min(clamped, 100) / 100
    const color = pct > 100 ? '#f87171' : pct >= 90 ? '#f87171' : pct >= 75 ? '#fbbf24' : '#34d399'
    return (
        <svg width={width} height={6} className="flex-shrink-0">
            <rect x="0" y="0" width={width} height="6" rx="3" fill="#1e3254" />
            <rect x="0" y="0" width={Math.round(fill * width)} height="6" rx="3" fill={color} />
            {pct > 100 && (
                <rect x={Math.round(fill * width) - 2} y="0" width="4" height="6" rx="1" fill="#ef4444" />
            )}
        </svg>
    )
}
