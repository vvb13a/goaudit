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
}

export interface EngineConfig {
  max_concurrency: number
  request_delay_ms: number
  http_timeout_sec: number
  user_agent: string
  max_sitemap_depth: number
  link_cache_ttl_min: number
  enable_checks: boolean
  enable_graph: boolean
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
