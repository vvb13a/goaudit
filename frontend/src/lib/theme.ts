// Semantic color system for the web app. These are the standard Tailwind /
// PrimeVue palette colors (not the TUI's accents), so a role reads the same
// everywhere: success green, info sky, warning orange, danger red and a
// darker red for fatal.
export const palette = {
  primary: 'var(--p-primary-color)',
  fatal: '#991b1b', // red-800, kept distinct from danger
  danger: '#dc2626', // red-600
  warning: '#d97706', // amber-600
  info: '#2563eb', // blue-600
  success: '#16a34a', // green-600
  neutral: '#64748b', // slate-500
} as const

export type SemanticColor = keyof typeof palette

export function auditScoreColor(score: number): string {
  if (score >= 90) return palette.success
  if (score >= 70) return palette.warning
  return palette.danger
}

export interface SeverityMeta {
  key: 'fatal' | 'error' | 'warning' | 'notice' | 'success'
  label: string
  color: string
}

// Ordered most severe first, matching the dashboard's severity widget.
export const severities: SeverityMeta[] = [
  { key: 'fatal', label: 'Fatal', color: palette.fatal },
  { key: 'error', label: 'Error', color: palette.danger },
  { key: 'warning', label: 'Warning', color: palette.warning },
  { key: 'notice', label: 'Notice', color: palette.info },
  { key: 'success', label: 'Success', color: palette.success },
]

export function severityMeta(key: string): SeverityMeta | undefined {
  return severities.find((s) => s.key === key)
}

export function severityColor(key: string): string {
  return severityMeta(key)?.color ?? palette.neutral
}

export interface LifecycleMeta {
  key:
    | 'new'
    | 'open'
    | 'resurfaced'
    | 'resolved'
    | 'improved'
    | 'degraded'
    | 'passed'
  label: string
  color: string
  moreIsGood: boolean
}

export const lifecycles: LifecycleMeta[] = [
  { key: 'new', label: 'New', color: palette.info, moreIsGood: false },
  { key: 'open', label: 'Open', color: palette.warning, moreIsGood: false },
  {
    key: 'resurfaced',
    label: 'Resurfaced',
    color: palette.danger,
    moreIsGood: false,
  },
  {
    key: 'resolved',
    label: 'Resolved',
    color: palette.success,
    moreIsGood: true,
  },
  {
    key: 'improved',
    label: 'Improved',
    color: palette.success,
    moreIsGood: true,
  },
  {
    key: 'degraded',
    label: 'Degraded',
    color: palette.danger,
    moreIsGood: false,
  },
  { key: 'passed', label: 'Passed', color: palette.success, moreIsGood: true },
]

export function lifecycleMeta(key: string): LifecycleMeta | undefined {
  return lifecycles.find((l) => l.key === key)
}

export interface CategoryMeta {
  key: string
  label: string
}

export const categories: CategoryMeta[] = [
  { key: 'seo', label: 'SEO' },
  { key: 'security', label: 'Security' },
  { key: 'performance', label: 'Performance' },
  { key: 'accessibility', label: 'Accessibility' },
  { key: 'headers', label: 'HTTP Headers' },
  { key: 'content', label: 'Content & Markup' },
  { key: 'general', label: 'General' },
]

export function categoryLabel(key: string): string {
  return categories.find((c) => c.key === key)?.label ?? key
}
