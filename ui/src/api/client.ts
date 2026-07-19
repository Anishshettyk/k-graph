import type {
    ContextsResponse, GraphResponse, TreeResponse, WhyResponse,
    DoctorResponse, OrphanResponse, EventsResponse, RBACResponse,
    NetworkResponse, CanReachResponse, YAMLResponse, LogsResponse,
} from '../types/api'

const BASE = '/api'

async function get<T>(path: string, params?: Record<string, string>): Promise<T> {
    const url = new URL(BASE + path, window.location.origin)
    if (params) {
        Object.entries(params).forEach(([k, v]) => v && url.searchParams.set(k, v))
    }
    const res = await fetch(url.toString())
    if (!res.ok) {
        const err = await res.json().catch(() => ({ error: res.statusText }))
        throw new Error(err.error ?? res.statusText)
    }
    return res.json()
}

export const api = {
    contexts: () =>
        get<ContextsResponse>('/contexts'),

    graph: (context: string, namespace: string) =>
        get<GraphResponse>('/graph', { context, namespace }),

    deps: (context: string, namespace: string, resource: string) =>
        get<TreeResponse>('/deps', { context, namespace, resource }),

    impact: (context: string, namespace: string, resource: string) =>
        get<TreeResponse>('/impact', { context, namespace, resource }),

    why: (context: string, namespace: string, resource: string) =>
        get<WhyResponse>('/why', { context, namespace, resource }),

    doctor: (context: string, namespace: string) =>
        get<DoctorResponse>('/doctor', { context, namespace }),

    orphan: (context: string, namespace: string) =>
        get<OrphanResponse>('/orphan', { context, namespace }),

    events: (context: string, namespace: string, resource: string) =>
        get<EventsResponse>('/events', { context, namespace, resource }),

    rbac: (context: string, namespace: string, sa: string, verb: string, resource: string, apiGroup: string) =>
        get<RBACResponse>('/rbac', { context, namespace, sa, verb, resource, apiGroup }),

    network: (context: string, namespace: string, resource: string) =>
        get<NetworkResponse>('/network', { context, namespace, resource }),

    canReach: (context: string, namespace: string, src: string, dst: string, port: string, protocol: string) =>
        get<CanReachResponse>('/can-reach', { context, namespace, src, dst, port, protocol }),

    yaml: (context: string, namespace: string, resource: string) =>
        get<YAMLResponse>('/yaml', { context, namespace, resource }),

    logs: (context: string, namespace: string, resource: string, tail = '300', container = '') =>
        get<LogsResponse>('/logs', { context, namespace, resource, tail, container }),
}
