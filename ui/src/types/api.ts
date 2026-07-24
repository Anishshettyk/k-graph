// Mirrors the Go API response structs exactly.

export interface ContextsResponse {
    current: string
    contexts: string[]
}

export interface NodeInfo {
    uid: string
    kind: string
    namespace: string
    name: string
    healthy: boolean
    reason?: string
    fields?: Record<string, string>
    labels?: Record<string, string>
}

export interface EdgeInfo {
    from: string
    to: string
    rel: string
}

export interface GraphResponse {
    context: string
    namespace: string
    nodes: NodeInfo[]
    edges: EdgeInfo[]
    total: number
    truncated: boolean
}

// ─── Blast Radius ─────────────────────────────────────────────────────────
export type BlastImpact = 'OUTAGE' | 'DEGRADED' | 'SAFE' | ''

export interface TreeNode {
    node: NodeInfo
    rel?: string
    cycle?: boolean
    blastImpact?: BlastImpact
    children: TreeNode[]
}

export interface TreeResponse {
    context: string
    root: TreeNode
}

export interface Finding {
    title: string
    detail: string
}

export interface DiagEvent {
    reason: string
    message: string
}

export interface DiagLog {
    container: string
    excerpt: string
}

export interface PodDiagnosis {
    pod: NodeInfo
    status: string
    summary?: string
    findings?: Finding[]
    events?: DiagEvent[]
    logs?: DiagLog[]
}

export interface WhyResponse {
    context: string
    node: NodeInfo
    tree: TreeNode
    diagnoses: PodDiagnosis[]
}

export interface FindingInfo {
    severity: 'blocking' | 'warning'
    category: string
    kind: string
    namespace: string
    name: string
    issue: string
    detail: string
    fix: string
}

export interface DoctorResponse {
    context: string
    findings: FindingInfo[]
}

export interface OrphanInfo {
    node: NodeInfo
    reason: string
}

export interface OrphanResponse {
    context: string
    orphans: OrphanInfo[]
}

export interface EventInfo {
    age: string
    type: string
    reason: string
    object: string
    message: string
}

export interface EventsResponse {
    context: string
    events: EventInfo[]
}

export interface GrantPath {
    bindingKind: string
    bindingName: string
    bindingNs: string
    roleKind: string
    roleName: string
    ruleIndex: number
}

export interface RBACCheckInfo {
    bindingKind: string
    bindingName: string
    bindingNs: string
    roleKind: string
    roleName: string
    subjectMatch: boolean
    grants: boolean
}

export interface RBACResponse {
    context: string
    serviceAccount: string
    namespace: string
    verb: string
    resource: string
    apiGroup: string
    verdict: 'GRANTED' | 'DENIED'
    paths?: GrantPath[]
    checks: RBACCheckInfo[]
}

export interface PodHop {
    name: string
    namespace: string
    healthy: boolean
    reason?: string
}

export interface NetworkHop {
    kind: string
    namespace: string
    name: string
    healthy: boolean
    ready: number
    total: number
    selector?: string
    pods?: PodHop[]
}

export interface NetworkResponse {
    context: string
    resource: string
    hops: NetworkHop[]
}

export interface ReachResult {
    dstPod: string
    verdict: string
    egressOpen: boolean
    ingressOpen: boolean
    suggestion?: string
}

export interface CanReachResponse {
    context: string
    source: string
    dest: string
    port: number
    protocol: string
    results: ReachResult[]
}

// ─── YAML / Manifest ─────────────────────────────────────────────────────────

export interface YAMLResponse {
    context: string
    resource: string
    yaml: string
}

// ─── Logs ────────────────────────────────────────────────────────────────────

export type LogLevel = 'FATAL' | 'ERROR' | 'WARN' | 'INFO' | 'DEBUG' | 'TRACE' | 'UNKNOWN'

export interface LogEntry {
    lineNum: number
    raw: string
    level: LogLevel
    timestamp?: string
    message?: string
}

export interface PodLogResult {
    pod: string
    container: string
    entries: LogEntry[]
    error?: string
}

export interface LogsResponse {
    context: string
    pods: PodLogResult[]
}

// ─── Metrics Intelligence ──────────────────────────────────────────────────

export type MetricsSeverity = 'over' | 'critical' | 'warning' | 'ok'

export interface PodMetricsInfo {
    namespace: string
    name: string
    cpuMilli: number
    cpuLimitMilli: number
    cpuRequestMilli: number
    cpuPct: number        // % of limit; -1 = no limit
    memBytes: number
    memLimitBytes: number
    memRequestBytes: number
    memPct: number        // % of limit; -1 = no limit
    noLimits: boolean
    cpuSev: MetricsSeverity
    memSev: MetricsSeverity
}

export interface Bottleneck {
    namespace: string
    podName: string
    resource: 'cpu' | 'memory'
    severity: MetricsSeverity
    usagePct: number
    recommendation: string
}

export interface MetricsSnapshotResponse {
    context: string
    namespace: string
    timestamp: string
    pods: PodMetricsInfo[]
    bottlenecks: Bottleneck[]
    noLimitPods: string[]
    available: boolean
}

// ─── Event Intelligence ────────────────────────────────────────────────────

export type EventSeverity = 'cluster-wide' | 'critical' | 'warning' | 'info'

export interface AggregatedEvent {
    reason: string
    message: string
    objects: string[]
    namespace: string
    count: number
    firstSeen: string
    lastSeen: string
    type: string
    severity: EventSeverity
    isPattern: boolean
    isClusterWide: boolean
}

export interface ClusterEventsResponse {
    context: string
    namespace: string
    events: AggregatedEvent[]
    patterns: number
}

// ─── Images ───────────────────────────────────────────────────────────────

export type ImageRisk = 'latest-tag' | 'no-tag' | 'unknown-reg'

export interface ImageWorkload {
    kind: string
    namespace: string
    name: string
}

export interface ImageInfo {
    image: string
    registry: string
    repository: string
    tag: string
    risks: ImageRisk[]
    workloads: ImageWorkload[]
    podCount: number
}

export interface ImagesResponse {
    context: string
    namespace: string
    images: ImageInfo[]
    total: number
    withRisks: number
    latestTags: number
}

// ─── Certificates ─────────────────────────────────────────────────────────

export type CertSeverity = 'critical' | 'warning' | 'watch' | 'ok'

export interface CertInfo {
    secretName: string
    namespace: string
    commonName: string
    dnsNames: string[]
    issuer: string
    notBefore: string
    notAfter: string
    daysLeft: number
    severity: CertSeverity
    expired: boolean
    parseError?: string
}

export interface CertsResponse {
    context: string
    namespace: string
    certs: CertInfo[]
    total: number
    expired: number
    critical: number
    warning: number
}

// ─── Rollout History ──────────────────────────────────────────────────────

export interface PodSummary {
    name: string
    healthy: boolean
    phase: string
}

export interface RSRevision {
    name: string
    namespace: string
    createdAt: string
    desiredReplicas: number
    readyReplicas: number
    replicas: number
    images: string[]
    isCurrent: boolean
    pods: PodSummary[]
}

export interface HistoryResponse {
    context: string
    kind: string
    name: string
    namespace: string
    revisions: RSRevision[]
}

// ─── Right-Sizing ─────────────────────────────────────────────────────────

export type RSizingSeverity = 'waste' | 'under-req' | 'spiky' | 'ok'
export type RSizingConfidence = 'low' | 'medium' | 'high'

export interface RangeStats {
    min: number
    max: number
    avg: number
    p95: number
    current: number
    samples: number
    windowMinutes: number
}

export interface ContainerSizing {
    resource: 'cpu' | 'memory'
    request: number        // millicores or bytes
    limit: number
    stats: RangeStats
    p95Pct: number         // p95 as % of request
    maxPct: number
    severity: RSizingSeverity
    recommendation: string
    confidence: RSizingConfidence
}

export interface PodSizing {
    namespace: string
    name: string
    cpu: ContainerSizing
    memory: ContainerSizing
    workload?: string
}

export interface NSResourceSummary {
    namespace: string
    cpuRequestMilli: number
    cpuUsedMilli: number
    memRequestBytes: number
    memUsedBytes: number
    podCount: number
}

export interface RightsizingResponse {
    context: string
    namespace: string
    timestamp: string
    pods: PodSizing[]
    available: boolean
    wasteCount: number
    namespaceBreakdown: NSResourceSummary[]
    typicalSamples: number
    windowMinutes: number
}

// ─── RBAC Audit ───────────────────────────────────────────────────────────

export type RBACRiskLevel = 'critical' | 'warning' | 'info'
export type RBACRiskCategory = 'cluster-admin' | 'wildcard-all' | 'wildcard-verb' | 'wildcard-res' | 'ghost-account'

export interface RBACRiskFinding {
    category: RBACRiskCategory
    level: RBACRiskLevel
    subject: string
    subjectKind: string
    subjectNs?: string
    binding: string
    bindingKind: string
    bindingNs?: string
    role: string
    roleKind: string
    detail: string
    remediation: string
}

export interface RBACRiskResponse {
    context: string
    findings: RBACRiskFinding[]
    critical: number
    warning: number
    info: number
}
