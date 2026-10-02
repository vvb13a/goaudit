import type { AuditConfig } from '../types'

export function defaultAuditConfig(): AuditConfig {
  return {
    name: '',
    description: '',
    targets: [],
    check_names: [],
    config: {
      max_concurrency: 5,
      request_delay_ms: 100,
      http_timeout_sec: 15,
      user_agent: 'GoAuditEngine/1.0 (AuditBot; +https://example.com/bot)',
      max_sitemap_depth: 3,
      link_cache_ttl_min: 15,
      enable_checks: true,
      enable_graph: false,
      notifications_enabled: false,
      slack_webhook_url: '',
      notify_on_direct: true,
      notify_on_schedule: true,
    },
  }
}

// normalizeTargets trims each entered line, prepends https:// where missing
// and drops duplicates and blanks, matching the TUI editor.
export function normalizeTargets(lines: string[]): string[] {
  const seen = new Set<string>()
  const targets: string[] = []
  for (const line of lines) {
    let url = line.trim()
    if (!url) continue
    if (!/^https?:\/\//.test(url)) url = `https://${url}`
    if (seen.has(url)) continue
    seen.add(url)
    targets.push(url)
  }
  return targets
}

export function validateAuditConfig(cfg: AuditConfig): string | null {
  if (!cfg.name.trim()) return 'Audit name is required'
  if (!cfg.targets.some((t) => t.trim())) return 'Add at least one target URL'
  const c = cfg.config
  if (!c.enable_checks && !c.enable_graph) {
    return 'Enable checks, the link graph, or both'
  }
  if (c.enable_checks && !cfg.check_names.length) return 'Select at least one check'
  if (c.max_concurrency <= 0) return 'Max concurrency must be a positive integer'
  if (c.request_delay_ms < 0) return 'Request delay (ms) must be an integer >= 0'
  if (c.http_timeout_sec <= 0) return 'HTTP timeout (s) must be a positive integer'
  if (c.max_sitemap_depth <= 0) return 'Max sitemap depth must be a positive integer'
  if (c.link_cache_ttl_min <= 0) return 'Link cache TTL (min) must be a positive integer'
  if (!c.user_agent.trim()) return 'User agent must not be empty'
  const webhook = c.slack_webhook_url.trim()
  if (c.notifications_enabled && !webhook) {
    return 'A Slack webhook URL is required to enable notifications'
  }
  if (webhook && !/^https?:\/\//.test(webhook)) {
    return 'The Slack webhook URL must start with http:// or https://'
  }
  return null
}

// normalizeAuditConfig returns a copy safe to submit: trimmed name/user agent
// and canonical target URLs.
export function normalizeAuditConfig(cfg: AuditConfig): AuditConfig {
  return {
    name: cfg.name.trim(),
    description: cfg.description,
    targets: normalizeTargets(cfg.targets),
    check_names: [...cfg.check_names],
    config: {
      ...cfg.config,
      user_agent: cfg.config.user_agent.trim(),
      slack_webhook_url: cfg.config.slack_webhook_url.trim(),
    },
  }
}
