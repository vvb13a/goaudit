// Formatting helpers mirroring the TUI so both interfaces read the same.

export function formatDuration(ms: number | null | undefined): string {
  if (ms == null || !Number.isFinite(ms) || ms <= 0) {
    return '–'
  }
  if (ms < 1000) {
    return `${Math.round(ms)}ms`
  }
  return `${(ms / 1000).toFixed(1)}s`
}

export function timeAgo(iso: string): string {
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) {
    return ''
  }
  let d = Date.now() - then
  if (d < 0) {
    d = 0
  }

  const minute = 60_000
  const hour = 60 * minute
  const day = 24 * hour
  const month = 30 * day
  const year = 365 * day

  if (d < minute) return 'just now'
  const ago = (unit: number, singular: string, plural: string) => {
    const n = Math.max(1, Math.floor(d / unit))
    return `${n} ${n === 1 ? singular : plural} ago`
  }
  if (d < hour) return ago(minute, 'minute', 'minutes')
  if (d < day) return ago(hour, 'hour', 'hours')
  if (d < month) return ago(day, 'day', 'days')
  if (d < year) return ago(month, 'month', 'months')
  return ago(year, 'year', 'years')
}

// intDelta renders a signed change, or an empty string when unchanged.
export function intDelta(current: number, previous: number): string {
  if (current === previous) return ''
  const d = current - previous
  return `${d > 0 ? '+' : ''}${d}`
}

export function floatDelta(current: number, previous: number): string {
  if (current === previous) return ''
  const d = current - previous
  return `${d > 0 ? '+' : ''}${d.toFixed(1)}`
}

export function durationDelta(currentMs: number, previousMs: number): string {
  if (currentMs === previousMs) return ''
  const delta = Math.abs(currentMs - previousMs)
  return `${currentMs - previousMs > 0 ? '+' : '-'}${formatDuration(delta)}`
}

// deltaTone maps a signed delta to a good/bad/neutral tone. moreIsGood flips
// whether an increase is desirable (e.g. passed checks).
export function deltaTone(delta: string, moreIsGood = false): string {
  if (!delta || delta === '+0' || delta === '+0.0') return 'neutral'
  const negative = delta.startsWith('-')
  const good = moreIsGood ? !negative : negative
  return good ? 'good' : 'bad'
}
