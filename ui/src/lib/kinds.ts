import {
    Layers, Database, Server, Clock, CalendarClock,
    GitMerge, Globe, Circle, FileText, Lock, HardDrive,
    Monitor, KeyRound, Shield, ArrowRight, Folder
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

export const KIND_ICONS: Record<string, LucideIcon> = {
    Deployment: Layers,
    StatefulSet: Database,
    DaemonSet: Server,
    ReplicaSet: Layers,
    CronJob: CalendarClock,
    Job: Clock,
    Pod: Circle,
    Service: GitMerge,
    Ingress: Globe,
    ConfigMap: FileText,
    Secret: Lock,
    PersistentVolumeClaim: HardDrive,
    PersistentVolume: HardDrive,
    Node: Monitor,
    ServiceAccount: KeyRound,
    NetworkPolicy: Shield,
    Namespace: Folder,
    Role: Shield,
    ClusterRole: Shield,
    RoleBinding: ArrowRight,
    ClusterRoleBinding: ArrowRight,
}

export const KIND_COLORS: Record<string, string> = {
    Deployment: '#60a5fa',
    StatefulSet: '#3b82f6',
    DaemonSet: '#0ea5e9',
    ReplicaSet: '#22d3ee',
    CronJob: '#a855f7',
    Job: '#c084fc',
    Pod: '#22c55e',
    Service: '#d946ef',
    Ingress: '#f97316',
    ConfigMap: '#7dd3fc',
    Secret: '#f87171',
    PersistentVolumeClaim: '#fbbf24',
    PersistentVolume: '#fcd34d',
    Node: '#94a3b8',
    ServiceAccount: '#86efac',
    NetworkPolicy: '#fb923c',
    Namespace: '#38bdf8',
    Role: '#a78bfa',
    ClusterRole: '#a78bfa',
    RoleBinding: '#818cf8',
    ClusterRoleBinding: '#818cf8',
}

export function hexToRgba(hex: string, alpha: number) {
    const r = parseInt(hex.slice(1, 3), 16)
    const g = parseInt(hex.slice(3, 5), 16)
    const b = parseInt(hex.slice(5, 7), 16)
    return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

export function kindIcon(kind: string): LucideIcon | null {
    return KIND_ICONS[kind] || null
}

export function kindColor(kind: string) {
    return KIND_COLORS[kind] ?? '#94a3b8'
}

export function kindBg(kind: string) {
    const color = kindColor(kind)
    return hexToRgba(color, 0.08)
}

// Re-export custom SVG icons from the .tsx file so consumers can import from kinds.
export type { SvgIconProps } from './kindIcons'
export {
    ServiceIcon, IngressIcon, PVCIcon, PVIcon,
    NetworkPolicyIcon, ConfigMapIcon, SecretIcon,
} from './kindIcons'
