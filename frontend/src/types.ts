export interface AuditSummary {
  id: string
  name: string
  description: string
  targets: string[]
  check_names: string[]
  started_at: string
  duration_ms: number
  score: number
}

export interface SeverityCounts {
  success: number
  notice: number
  warning: number
  error: number
  fatal: number
}

export interface UrlStates {
  new: number
  active: number
  missing: number
}

export interface Lifecycles {
  new: number
  open: number
  resurfaced: number
  resolved: number
  improved: number
  degraded: number
  passed: number
}

export interface AuditMetrics {
  URLStates: UrlStates
  Severity: SeverityCounts
  Lifecycle: Lifecycles
}

export interface SnapshotOverview {
  total_urls: number
  total_issues: number
  total_checks: number
  score: number
}

export interface SnapshotTimings {
  total_ns: number
  per_url_ns: number
  per_issue_ns: number
}

export interface AuditSnapshot {
  id: string
  audit_id: string
  created_at: string
  overview: SnapshotOverview
  url_states: UrlStates
  severity_counts: SeverityCounts
  lifecycle_counts: Lifecycles
  timings: SnapshotTimings
}

export interface AuditCounts {
  urls: number
  issues: number
  checks: number
  nodes: number
  edges: number
}

export interface DashboardResponse {
  audit: AuditSummary
  metrics: AuditMetrics
  checks: number
  previous: AuditSnapshot | null
}

export interface CheckInfo {
  name: string
  label: string
  description: string
  category: string
  scope: string
}

export interface EngineConfig {
  max_concurrency: number
  request_delay_ms: number
  http_timeout_sec: number
  user_agent: string
  max_sitemap_depth: number
  link_cache_ttl_min: number
  asset_request_delay_ms: number
  asset_max_concurrency: number
  enable_checks: boolean
  enable_graph: boolean
  enable_link_validation: boolean
  notifications_enabled: boolean
  slack_webhook_url: string
  notify_on_direct: boolean
  notify_on_schedule: boolean
}

export interface AuditConfig {
  name: string
  description: string
  targets: string[]
  check_names: string[]
  config: EngineConfig
}

// RunOverride carries one-time values for a single run: the engine config and,
// when notifications are configured, whether to notify for this run.
export interface RunOverride {
  config: EngineConfig
  notify?: boolean
}

export type RunState = 'idle' | 'running' | 'done' | 'error'

export interface RunStatus {
  state: RunState
  phase?: string
  current_url?: string
  completed: number
  total: number
  error?: string
  started_at?: string
  finished_at?: string
}

export interface Issue {
  id: string
  url: string
  check_name: string
  category: string
  severity: string
  message: string
  details?: Record<string, unknown>
  prior_severity?: string
  lifecycle: string
  created_at: string
  updated_at: string
}

export interface IssuesResponse {
  items: Issue[]
  total: number
  severity_counts: SeverityCounts
}

export interface IssueFilters {
  check_names: string[]
}

export interface UrlRow {
  url: string
  final_url: string
  status_code: number
  duration_ms: number
  state: string
  highest_severity: string
  score: number
  severity_counts: SeverityCounts
  created_at: string
  last_audited_at: string
}

export interface UrlAggregates {
  new: number
  active: number
  missing: number
  total: number
  avg_duration_ms: number
  avg_score: number
}

export interface UrlsResponse {
  items: UrlRow[]
  total: number
  aggregates: UrlAggregates
}

export interface CheckSummary {
  name: string
  label: string
  description: string
  category: string
  total: number
  severity: SeverityCounts
}

export interface ChecksResponse {
  items: CheckSummary[]
  total: number
  highest_counts: SeverityCounts
}

export interface GraphNode {
  id: string
  url: string
  filetype: string
  external: boolean
  is_root: boolean
  in_links: number
  out_links: number
  first_seen: string
  last_seen: string
  status_code: number
  last_validated: string
}

export interface GraphEdgeRow {
  id: string
  type: string
  count: number
  source_node_id: string
  source_url: string
  source_filetype: string
  target_node_id: string
  target_url: string
  target_filetype: string
  target_external: boolean
}

export interface FiletypeCount {
  filetype: string
  count: number
}

export interface StatusCodeCount {
  status_code: number
  count: number
}

export interface GraphSummary {
  total_nodes: number
  total_edges: number
  external_nodes: number
  root_nodes: number
  filetypes: FiletypeCount[]
  statuses: StatusCodeCount[]
}

export interface GraphSnapshot {
  id: string
  audit_id: string
  created_at: string
  total_nodes: number
  total_edges: number
  internal_nodes: number
  external_nodes: number
  root_nodes: number
  filetypes: FiletypeCount[]
  duration_ns: number
}

export interface GraphAllResponse {
  nodes: GraphNode[]
  edges: GraphEdgeRow[]
  truncated: boolean
}

export interface GraphNodesResponse {
  items: GraphNode[]
  total: number
}

export interface GraphEdgesResponse {
  items: GraphEdgeRow[]
  total: number
}

export interface GraphNodeQuery {
  page: number
  limit: number
  sort?: string
  order?: 'asc' | 'desc'
  filetypes?: string[]
  external?: boolean
  root?: boolean
  url?: string
  firstSeenFrom?: string
  firstSeenTo?: string
  lastSeenFrom?: string
  lastSeenTo?: string
  statuses?: number[]
  lastValidatedFrom?: string
  lastValidatedTo?: string
}

export interface GraphEdgeQuery {
  page: number
  limit: number
  sort?: string
  order?: 'asc' | 'desc'
  types?: string[]
  sourceId?: string
  targetId?: string
  source?: string
  target?: string
}

export interface CheckQuery {
  page: number
  limit: number
  sort?: string
  order?: 'asc' | 'desc'
  name?: string
  categories?: string[]
  highest?: string[]
  fatalMin?: number
  errorMin?: number
  warningMin?: number
  noticeMin?: number
  successMin?: number
}

export interface UrlQuery {
  page: number
  limit: number
  sort?: string
  order?: 'asc' | 'desc'
  states?: string[]
  highest?: string[]
  durationMin?: number
  durationMax?: number
  scoreMin?: number
  scoreMax?: number
  url?: string
}

export interface IssueQuery {
  page: number
  limit: number
  sort?: string
  order?: 'asc' | 'desc'
  severities?: string[]
  lifecycles?: string[]
  checks?: string[]
  categories?: string[]
  url?: string
  message?: string
}
