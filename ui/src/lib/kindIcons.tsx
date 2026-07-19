// Custom inline SVG icons for Kubernetes resource kinds.
// Each is purpose-built to be instantly recognisable at small sizes in a graph.

export type SvgIconProps = { color: string; size?: number }

/** Service — network hub with 4 spokes */
export function ServiceIcon({ color, size = 22 }: SvgIconProps) {
    return (
        <svg width={size} height={size} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <circle cx="12" cy="12" r="3" fill={color} />
            <circle cx="4" cy="7" r="1.8" fill={color} opacity={0.55} />
            <circle cx="20" cy="7" r="1.8" fill={color} opacity={0.55} />
            <circle cx="4" cy="17" r="1.8" fill={color} opacity={0.55} />
            <circle cx="20" cy="17" r="1.8" fill={color} opacity={0.55} />
            <line x1="12" y1="12" x2="5.2" y2="8.1" stroke={color} strokeWidth="1.4" opacity={0.45} />
            <line x1="12" y1="12" x2="18.8" y2="8.1" stroke={color} strokeWidth="1.4" opacity={0.45} />
            <line x1="12" y1="12" x2="5.2" y2="15.9" stroke={color} strokeWidth="1.4" opacity={0.45} />
            <line x1="12" y1="12" x2="18.8" y2="15.9" stroke={color} strokeWidth="1.4" opacity={0.45} />
        </svg>
    )
}

/** Ingress — gateway arch with traffic arrow */
export function IngressIcon({ color, size = 22 }: SvgIconProps) {
    return (
        <svg width={size} height={size} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <rect x="2" y="6" width="3" height="13" rx="1" fill={color} opacity={0.8} />
            <rect x="19" y="6" width="3" height="13" rx="1" fill={color} opacity={0.8} />
            <path d="M5 9 Q12 3 19 9" stroke={color} strokeWidth="2" fill="none" strokeLinecap="round" />
            <path d="M8 14 h8" stroke={color} strokeWidth="2" strokeLinecap="round" />
            <path d="M13 11.5 l2.5 2.5 -2.5 2.5" stroke={color} strokeWidth="1.8" fill="none" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
    )
}

/** PVC — layered storage disks with claim arrow */
export function PVCIcon({ color, size = 22 }: SvgIconProps) {
    return (
        <svg width={size} height={size} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <ellipse cx="12" cy="6" rx="7" ry="2" stroke={color} strokeWidth="1.5" fill={color} fillOpacity={0.12} />
            <path d="M5 6v4" stroke={color} strokeWidth="1.5" />
            <path d="M19 6v4" stroke={color} strokeWidth="1.5" />
            <ellipse cx="12" cy="10" rx="7" ry="2" stroke={color} strokeWidth="1.5" fill={color} fillOpacity={0.08} />
            <path d="M5 10v4" stroke={color} strokeWidth="1.5" />
            <path d="M19 10v4" stroke={color} strokeWidth="1.5" />
            <ellipse cx="12" cy="14" rx="7" ry="2" stroke={color} strokeWidth="1.5" fill={color} fillOpacity={0.05} />
            <path d="M12 16.5 v3" stroke={color} strokeWidth="1.6" strokeLinecap="round" />
            <path d="M10 18 l2 2 2-2" stroke={color} strokeWidth="1.5" fill="none" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
    )
}

/** PV — solid cylinder with data tracks */
export function PVIcon({ color, size = 22 }: SvgIconProps) {
    return (
        <svg width={size} height={size} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M5 7v10" stroke={color} strokeWidth="1.5" />
            <path d="M19 7v10" stroke={color} strokeWidth="1.5" />
            <ellipse cx="12" cy="7" rx="7" ry="2.2" stroke={color} strokeWidth="1.5" fill={color} fillOpacity={0.3} />
            <ellipse cx="12" cy="17" rx="7" ry="2.2" stroke={color} strokeWidth="1.5" fill={color} fillOpacity={0.08} />
            <ellipse cx="12" cy="12" rx="7" ry="2.2" stroke={color} strokeWidth="1" strokeDasharray="2 2" fill="none" opacity={0.4} />
        </svg>
    )
}

/** NetworkPolicy — shield with lock */
export function NetworkPolicyIcon({ color, size = 22 }: SvgIconProps) {
    return (
        <svg width={size} height={size} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M12 2 L20 5.5 L20 12 C20 16.8 16.2 20.6 12 22 C7.8 20.6 4 16.8 4 12 L4 5.5 Z"
                stroke={color} strokeWidth="1.5" fill={color} fillOpacity={0.12} strokeLinejoin="round" />
            <rect x="9" y="12" width="6" height="4.5" rx="1" stroke={color} strokeWidth="1.3" fill={color} fillOpacity={0.2} />
            <path d="M10 12 v-1.5 a2 2 0 0 1 4 0 v1.5" stroke={color} strokeWidth="1.3" fill="none" strokeLinecap="round" />
        </svg>
    )
}

/** ConfigMap — document with folded corner + key lines */
export function ConfigMapIcon({ color, size = 22 }: SvgIconProps) {
    return (
        <svg width={size} height={size} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M4 3 h11 l5 5 v13 a1 1 0 0 1-1 1 H5 a1 1 0 0 1-1-1 Z"
                stroke={color} strokeWidth="1.5" fill={color} fillOpacity={0.1} strokeLinejoin="round" />
            <path d="M15 3 v5 h5" stroke={color} strokeWidth="1.3" fill="none" strokeLinejoin="round" />
            <line x1="7" y1="12" x2="17" y2="12" stroke={color} strokeWidth="1.3" opacity={0.7} />
            <line x1="7" y1="15" x2="14" y2="15" stroke={color} strokeWidth="1.3" opacity={0.5} />
        </svg>
    )
}

/** Secret — padlock with keyhole */
export function SecretIcon({ color, size = 22 }: SvgIconProps) {
    return (
        <svg width={size} height={size} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <rect x="5" y="11" width="14" height="10" rx="2" stroke={color} strokeWidth="1.5" fill={color} fillOpacity={0.15} />
            <path d="M8 11 v-3 a4 4 0 0 1 8 0 v3" stroke={color} strokeWidth="1.5" fill="none" strokeLinecap="round" />
            <circle cx="12" cy="16" r="2" stroke={color} strokeWidth="1.2" fill={color} fillOpacity={0.25} />
            <line x1="12" y1="18" x2="12" y2="20" stroke={color} strokeWidth="1.5" strokeLinecap="round" />
        </svg>
    )
}
