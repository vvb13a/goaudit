import type {
  AuditConfig,
  AuditSnapshot,
  AuditSummary,
  CheckInfo,
  CheckQuery,
  ChecksResponse,
  DashboardResponse,
  Issue,
  IssueFilters,
  IssueQuery,
  IssuesResponse,
  RunOverride,
  RunStatus,
  UrlQuery,
  UrlsResponse,
} from '../types'

async function ensureOk(res: Response): Promise<void> {
  if (res.ok) return
  let message = `Request failed (${res.status})`
  try {
    const body = (await res.json()) as { error?: string }
    if (body.error) {
      message = body.error
    }
  } catch {
    // Non-JSON error body: keep the status message.
  }
  throw new Error(message)
}

async function requestJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)
  await ensureOk(res)
  return (await res.json()) as T
}

async function requestVoid(url: string, init?: RequestInit): Promise<void> {
  const res = await fetch(url, init)
  await ensureOk(res)
}

function getJSON<T>(url: string): Promise<T> {
  return requestJSON<T>(url)
}

function sendJSON<T>(url: string, method: string, body: unknown): Promise<T> {
  return requestJSON<T>(url, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export function listAudits(): Promise<AuditSummary[]> {
  return getJSON<AuditSummary[]>('/api/audits')
}

export function createAudit(payload: AuditConfig): Promise<AuditSummary> {
  return sendJSON<AuditSummary>('/api/audits', 'POST', payload)
}

export function getAudit(id: string): Promise<AuditSummary> {
  return getJSON<AuditSummary>(`/api/audits/${encodeURIComponent(id)}`)
}

export function deleteAudit(id: string): Promise<void> {
  return requestVoid(`/api/audits/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export function resetAudit(id: string): Promise<void> {
  return requestVoid(`/api/audits/${encodeURIComponent(id)}/reset`, {
    method: 'POST',
  })
}

export function duplicateAudit(id: string): Promise<AuditSummary> {
  return requestJSON<AuditSummary>(
    `/api/audits/${encodeURIComponent(id)}/duplicate`,
    { method: 'POST' },
  )
}

export function startRun(id: string, override?: RunOverride): Promise<RunStatus> {
  return requestJSON<RunStatus>(
    `/api/audits/${encodeURIComponent(id)}/run`,
    override
      ? {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(override),
        }
      : { method: 'POST' },
  )
}

export function getRunStatus(id: string): Promise<RunStatus> {
  return getJSON<RunStatus>(`/api/audits/${encodeURIComponent(id)}/run`)
}

export function recheckUrl(
  auditId: string,
  url: string,
): Promise<{ status: string; url: string }> {
  return sendJSON(
    `/api/audits/${encodeURIComponent(auditId)}/recheck`,
    'POST',
    { url },
  )
}

export function getDashboard(id: string): Promise<DashboardResponse> {
  return getJSON<DashboardResponse>(
    `/api/audits/${encodeURIComponent(id)}/dashboard`,
  )
}

export function listSnapshots(
  id: string,
  limit = 1000,
): Promise<AuditSnapshot[]> {
  return getJSON<AuditSnapshot[]>(
    `/api/audits/${encodeURIComponent(id)}/snapshots?limit=${limit}`,
  )
}

export function listChecks(): Promise<CheckInfo[]> {
  return getJSON<CheckInfo[]>('/api/checks')
}

export function getAuditConfig(id: string): Promise<AuditConfig> {
  return getJSON<AuditConfig>(`/api/audits/${encodeURIComponent(id)}/config`)
}

export function updateAuditConfig(
  id: string,
  payload: AuditConfig,
): Promise<AuditConfig> {
  return sendJSON<AuditConfig>(
    `/api/audits/${encodeURIComponent(id)}/config`,
    'PUT',
    payload,
  )
}

export function listIssues(
  auditId: string,
  query: IssueQuery,
): Promise<IssuesResponse> {
  const params = new URLSearchParams()
  params.set('page', String(query.page))
  params.set('limit', String(query.limit))
  if (query.sort) params.set('sort', query.sort)
  if (query.order) params.set('order', query.order)
  if (query.severities?.length) params.set('severity', query.severities.join(','))
  if (query.lifecycles?.length)
    params.set('lifecycle', query.lifecycles.join(','))
  if (query.checks?.length) params.set('check', query.checks.join(','))
  if (query.categories?.length)
    params.set('category', query.categories.join(','))
  if (query.url) params.set('url', query.url)
  if (query.message) params.set('message', query.message)

  return getJSON<IssuesResponse>(
    `/api/audits/${encodeURIComponent(auditId)}/issues?${params.toString()}`,
  )
}

export function getIssueFilters(auditId: string): Promise<IssueFilters> {
  return getJSON<IssueFilters>(
    `/api/audits/${encodeURIComponent(auditId)}/issue-filters`,
  )
}

export function listAuditChecks(
  auditId: string,
  query: CheckQuery,
): Promise<ChecksResponse> {
  const params = new URLSearchParams()
  params.set('page', String(query.page))
  params.set('limit', String(query.limit))
  if (query.sort) params.set('sort', query.sort)
  if (query.order) params.set('order', query.order)
  if (query.name) params.set('name', query.name)
  if (query.categories?.length) params.set('category', query.categories.join(','))
  if (query.highest?.length) params.set('highest', query.highest.join(','))
  if (query.fatalMin) params.set('fatal_min', String(query.fatalMin))
  if (query.errorMin) params.set('error_min', String(query.errorMin))
  if (query.warningMin) params.set('warning_min', String(query.warningMin))
  if (query.noticeMin) params.set('notice_min', String(query.noticeMin))
  if (query.successMin) params.set('success_min', String(query.successMin))

  return getJSON<ChecksResponse>(
    `/api/audits/${encodeURIComponent(auditId)}/checks?${params.toString()}`,
  )
}

export function listUrls(
  auditId: string,
  query: UrlQuery,
): Promise<UrlsResponse> {
  const params = new URLSearchParams()
  params.set('page', String(query.page))
  params.set('limit', String(query.limit))
  if (query.sort) params.set('sort', query.sort)
  if (query.order) params.set('order', query.order)
  if (query.states?.length) params.set('state', query.states.join(','))
  if (query.highest?.length) params.set('highest', query.highest.join(','))
  if (query.durationMin) params.set('duration_min', String(query.durationMin))
  if (query.durationMax) params.set('duration_max', String(query.durationMax))
  if (query.scoreMin) params.set('score_min', String(query.scoreMin))
  if (query.scoreMax) params.set('score_max', String(query.scoreMax))
  if (query.url) params.set('url', query.url)

  return getJSON<UrlsResponse>(
    `/api/audits/${encodeURIComponent(auditId)}/urls?${params.toString()}`,
  )
}

export function listUrlIssues(auditId: string, url: string): Promise<Issue[]> {
  const params = new URLSearchParams({ url })
  return getJSON<Issue[]>(
    `/api/audits/${encodeURIComponent(auditId)}/url-issues?${params.toString()}`,
  )
}

export function getIssue(auditId: string, issueId: string): Promise<Issue> {
  return getJSON<Issue>(
    `/api/audits/${encodeURIComponent(auditId)}/issues/${encodeURIComponent(issueId)}`,
  )
}
