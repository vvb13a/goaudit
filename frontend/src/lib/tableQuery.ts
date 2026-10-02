import { watch } from 'vue'
import { useRoute, useRouter, type LocationQuery } from 'vue-router'

export interface TableQueryOptions {
  // The query keys this table owns; other keys (e.g. the open drawer) are
  // preserved untouched.
  keys: string[]
  // Apply the table's part of the route query to the component's state.
  apply: (query: LocationQuery) => void
  // Fetch the current page using the component's state.
  load: () => void | Promise<void>
  // Build the table's query object from the component's current state.
  serialize: () => Record<string, string>
}

function queryString(query: Record<string, unknown>): string {
  const params = new URLSearchParams()
  for (const key of Object.keys(query).sort()) {
    const value = query[key]
    if (value === undefined || value === null || value === '') continue
    params.set(key, String(value))
  }
  return params.toString()
}

// useTableQuery keeps a table's filters, sorting and page in the URL query.
// External navigation (back/forward, a shared link, a cross-table jump) applies
// the query and reloads. Local mutations call sync(), which writes the URL and
// reloads directly; the resulting route change is suppressed so it does not
// load twice. Query keys outside `keys` (the open drawer) are left alone.
export function useTableQuery(opts: TableQueryOptions) {
  const route = useRoute()
  const router = useRouter()
  let suppress = false

  function pickTable(query: Record<string, unknown>): Record<string, string> {
    const out: Record<string, string> = {}
    for (const key of opts.keys) {
      const value = query[key]
      if (value === undefined || value === null || value === '') continue
      out[key] = Array.isArray(value) ? value.join(',') : String(value)
    }
    return out
  }

  function routeKey(): string {
    return `${route.params.auditId ?? ''}?${queryString(pickTable(route.query as Record<string, unknown>))}`
  }

  watch(
    routeKey,
    () => {
      if (suppress) {
        suppress = false
        return
      }
      opts.apply(route.query)
      opts.load()
    },
    { immediate: true },
  )

  function sync(): Promise<void> {
    const query: Record<string, string> = {}
    // Preserve query keys this table does not own (the open drawer).
    for (const [key, value] of Object.entries(route.query)) {
      if (opts.keys.includes(key)) continue
      if (typeof value === 'string') query[key] = value
      else if (Array.isArray(value) && value.length) query[key] = String(value[0])
    }
    Object.assign(query, opts.serialize())

    if (queryString(query) !== queryString(route.query as Record<string, unknown>)) {
      suppress = true
      router
        .replace({ query })
        .catch(() => {})
        .finally(() => {
          setTimeout(() => {
            suppress = false
          }, 0)
        })
    }
    return Promise.resolve(opts.load())
  }

  return { sync }
}
