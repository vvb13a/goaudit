<script setup lang="ts">
import {
  Bars,
  Cog,
  Directions,
  ExternalLink,
  Eye,
  FilterSlash,
  Globe,
  Link,
  List,
  Pencil,
  Play,
  Refresh,
} from '@primeicons/vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Column from 'primevue/column'
import ContextMenu from 'primevue/contextmenu'
import DataTable, {
  type DataTableFilterEvent,
  type DataTableFilterMeta,
  type DataTablePageEvent,
  type DataTableRowClickEvent,
  type DataTableRowContextMenuEvent,
  type DataTableSortEvent,
} from 'primevue/datatable'
import Drawer from 'primevue/drawer'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import Popover from 'primevue/popover'
import Select from 'primevue/select'
import Skeleton from 'primevue/skeleton'
import { useToast } from 'primevue/usetoast'
import {
  type Component,
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from 'vue'
import { useRoute, useRouter, type LocationQuery } from 'vue-router'
import * as api from '../api/client'
import { auditScoreColor, palette, severities } from '../lib/theme'
import { formatDuration } from '../lib/format'
import { dataVersion, notifyDataChanged } from '../lib/appState'
import { useCheckMeta } from '../lib/checkMeta'
import { useDelayedLoading } from '../lib/loading'
import { loadJSON, saveJSON } from '../lib/storage'
import { useTableQuery } from '../lib/tableQuery'
import type { Issue, UrlAggregates, UrlRow } from '../types'
import EmptyState from './EmptyState.vue'
import MetricTile from './MetricTile.vue'
import SeverityTag from './SeverityTag.vue'

const props = defineProps<{ auditId: string }>()

const route = useRoute()
const router = useRouter()
const { label: checkLabel, description: checkDescription } = useCheckMeta()

const toast = useToast()

const items = ref<UrlRow[]>([])
const total = ref(0)
const loading = ref(false)
const showTableLoading = useDelayedLoading(loading)
const error = ref<string | null>(null)
const rerunning = ref(false)

const aggregates = ref<UrlAggregates>({
  new: 0,
  active: 0,
  missing: 0,
  total: 0,
  avg_duration_ms: 0,
  avg_score: 0,
})

// How the URL column renders each row: the whole URL, the URL without its
// scheme, or just the path.
type UrlDisplay = 'full' | 'domain' | 'path'

// Filters, sorting and page live in the URL query; only global preferences
// (page size, URL display, column layout) persist in local storage.
interface SavedUrlSettings {
  version: number
  rows: number
  urlDisplay?: UrlDisplay
  columns?: string[]
  visibleColumns?: string[]
}

const SETTINGS_KEY = 'urls-view'
const SETTINGS_VERSION = 3
// Version 2 introduced urlDisplay; version 3 adds the column layout. Older
// versions are accepted so their rows/urlDisplay preferences survive.
const MIN_SETTINGS_VERSION = 2
const storedSettings = loadJSON<SavedUrlSettings>(SETTINGS_KEY)
const saved =
  storedSettings &&
  storedSettings.version >= MIN_SETTINGS_VERSION &&
  storedSettings.version <= SETTINGS_VERSION
    ? storedSettings
    : null

const rows = ref(saved?.rows ?? 25)
const first = ref(0)
const sortField = ref<string | undefined>(undefined)
const sortOrder = ref<1 | 0 | -1 | undefined>(undefined)

const urlDisplay = ref<UrlDisplay>(saved?.urlDisplay ?? 'domain')
const urlDisplayOptions: { value: UrlDisplay; title: string; icon: Component }[] =
  [
    { value: 'full', title: 'Show full URL', icon: Link },
    { value: 'domain', title: 'Show domain and path', icon: Globe },
    { value: 'path', title: 'Show path only', icon: Directions },
  ]

// Column layout. Order and visibility persist with the other global
// preferences. The DataTable is keyed on the visible, ordered set so any change
// re-renders the header/body with the new layout.
const columnDefs = [
  { field: 'url', label: 'URL' },
  { field: 'title', label: 'Title' },
  { field: 'duration', label: 'Duration' },
  { field: 'state', label: 'State' },
  { field: 'highest', label: 'Highest' },
  { field: 'score', label: 'Score' },
]
const columnLabels = Object.fromEntries(
  columnDefs.map((c) => [c.field, c.label]),
)
const defaultColumnOrder = columnDefs.map((c) => c.field)
const knownColumns = new Set(defaultColumnOrder)

// sanitizeColumnOrder keeps only known fields, preserving the saved order and
// appending any fields a stored layout predates.
function sanitizeColumnOrder(order: string[] | undefined): string[] {
  const out = (order ?? []).filter((f) => knownColumns.has(f))
  for (const f of defaultColumnOrder) {
    if (!out.includes(f)) out.push(f)
  }
  return out
}

const columnOrder = ref<string[]>(sanitizeColumnOrder(saved?.columns))
const visibleFields = ref<string[]>(
  (saved?.visibleColumns ?? defaultColumnOrder).filter((f) =>
    knownColumns.has(f),
  ),
)
const visibleOrderedFields = computed(() =>
  columnOrder.value.filter((f) => visibleFields.value.includes(f)),
)
const columnsKey = computed(() => visibleOrderedFields.value.join('-'))
const columnPopover = ref<InstanceType<typeof Popover>>()

const dragIndex = ref<number | null>(null)
const dragOverIndex = ref<number | null>(null)

function toggleColumns(event: Event): void {
  columnPopover.value?.toggle(event)
}

function resetColumns(): void {
  columnOrder.value = [...defaultColumnOrder]
  visibleFields.value = [...defaultColumnOrder]
}

function onColumnDragStart(index: number): void {
  dragOverIndex.value = index
  requestAnimationFrame(() => {
    dragIndex.value = index
  })
}

function onColumnDragOver(index: number): void {
  if (index !== dragIndex.value) dragOverIndex.value = index
}

function onColumnDragEnd(): void {
  dragIndex.value = null
  dragOverIndex.value = null
}

function onColumnDrop(): void {
  const from = dragIndex.value
  const to = dragOverIndex.value
  if (from !== null && to !== null && from !== to) {
    const next = [...columnOrder.value]
    const [moved] = next.splice(from, 1)
    next.splice(to, 0, moved)
    columnOrder.value = next
  }
  onColumnDragEnd()
}

// Range bounds are kept outside the DataTable filter model so the numeric
// min/max pairs survive its filter cloning untouched.
const durationMin = ref<number | null>(null)
const durationMax = ref<number | null>(null)
const scoreMin = ref<number | null>(null)
const scoreMax = ref<number | null>(null)

type MatchMode = 'in' | 'contains'
interface ColumnFilter {
  value: unknown
  matchMode: MatchMode
}

function defaultFilters(): Record<string, ColumnFilter> {
  return {
    url: { value: null, matchMode: 'contains' },
    duration: { value: null, matchMode: 'contains' },
    state: { value: null, matchMode: 'in' },
    highest: { value: null, matchMode: 'in' },
    score: { value: null, matchMode: 'contains' },
  }
}

const filters = ref<DataTableFilterMeta>(defaultFilters())

const stateOptions = [
  { label: 'New', value: 'new' },
  { label: 'Active', value: 'active' },
  { label: 'Missing', value: 'missing' },
]
const highestOptions = severities.map((s) => ({ label: s.label, value: s.key }))

const pageSizeOptions = [10, 25, 50, 100]

const widgetTiles = computed(() => [
  { label: 'Total', value: String(aggregates.value.total), color: palette.primary },
  { label: 'New', value: String(aggregates.value.new), color: palette.primary },
  { label: 'Active', value: String(aggregates.value.active), color: palette.primary },
  { label: 'Missing', value: String(aggregates.value.missing), color: palette.primary },
  {
    label: 'Avg Duration',
    value: formatDuration(aggregates.value.avg_duration_ms),
    color: palette.primary,
  },
  {
    label: 'Avg Score',
    value: aggregates.value.avg_score.toFixed(1),
    color: palette.primary,
  },
])

const pageReport = computed(() => {
  if (total.value === 0) return '0 of 0'
  const start = first.value + 1
  const end = Math.min(first.value + rows.value, total.value)
  return `${start}–${end} of ${total.value.toLocaleString()}`
})

// ---- Drawer state ----
const drawerVisible = ref(false)
const activeUrl = ref<UrlRow | null>(null)
// The drawer header prefers the page title; the full URL is already shown in
// the detail body, so it is only a fallback for untitled pages.
const drawerTitle = computed(() => {
  if (!activeUrl.value) return 'URL'
  return activeUrl.value.title || displayUrl(activeUrl.value.url)
})
const urlIssues = ref<Issue[]>([])
const loadingIssues = ref(false)
const selectedIssue = ref<Issue | null>(null)
let detailRequest = 0

// The issues/evidence skeleton only appears once the load is slow enough to
// warrant it, so arrow navigation does not flash a placeholder.
const showIssuesSkeleton = useDelayedLoading(loadingIssues)

// The row whose drawer is open. Held as an index into the current page so
// arrow navigation can step to the neighbours (and across pages).
const listRef = ref<HTMLElement | null>(null)
const activeIndex = ref(-1)

const sortedIssues = computed(() => {
  const weight = (sev: string) => {
    const i = severities.findIndex((s) => s.key === sev)
    return i === -1 ? severities.length : i
  }
  return [...urlIssues.value].sort((a, b) => weight(a.severity) - weight(b.severity))
})

const evidence = computed(() => selectedIssue.value?.details ?? null)

// ---- Context menu ----
const menu = ref<InstanceType<typeof ContextMenu>>()
const menuUrl = ref<UrlRow | null>(null)

const menuItems = computed(() => [
  {
    label: 'View issues',
    icon: Eye,
    command: () => menuUrl.value && openUrl(menuUrl.value),
  },
  {
    label: 'Open link',
    icon: ExternalLink,
    command: () => menuUrl.value && openLink(menuUrl.value),
  },
  {
    label: 'Edit page',
    icon: Pencil,
    visible: !!menuUrl.value?.edit_url,
    command: () => menuUrl.value && openEdit(menuUrl.value),
  },
  {
    label: 'Open in Issues',
    icon: List,
    command: () => menuUrl.value && openIssues(menuUrl.value.url),
  },
  {
    label: 'Rerun URL',
    icon: Play,
    disabled: rerunning.value,
    command: () => menuUrl.value && rerunUrl(menuUrl.value),
  },
])

let requestId = 0
let filterTimer: ReturnType<typeof setTimeout> | undefined

function filterValue(field: string): unknown {
  const entry = filters.value[field] as { value?: unknown } | undefined
  return entry?.value
}

function selectedValues(field: string): string[] {
  const value = filterValue(field)
  if (Array.isArray(value)) return value as string[]
  if (typeof value === 'string' && value) return [value]
  return []
}

function textValue(field: string): string {
  const value = filterValue(field)
  return typeof value === 'string' ? value : ''
}

function currentOrder(): 'asc' | 'desc' | undefined {
  if (sortOrder.value === 1) return 'asc'
  if (sortOrder.value === -1) return 'desc'
  return undefined
}

function applyQuery(query: LocationQuery): void {
  const q = query as Record<string, string | string[] | undefined>
  const list = (key: string): string[] => {
    const value = q[key]
    if (Array.isArray(value)) return value
    if (typeof value === 'string' && value) return value.split(',')
    return []
  }
  const single = (key: string): string | null => {
    const value = q[key]
    return typeof value === 'string' && value ? value : null
  }
  const number = (key: string): number | null => {
    const value = Number(single(key))
    return Number.isFinite(value) && value > 0 ? value : null
  }

  filters.value = {
    url: { value: single('url'), matchMode: 'contains' },
    duration: { value: null, matchMode: 'contains' },
    state: { value: list('state').length ? list('state') : null, matchMode: 'in' },
    highest: { value: list('highest').length ? list('highest') : null, matchMode: 'in' },
    score: { value: null, matchMode: 'contains' },
  }
  durationMin.value = number('duration_min')
  durationMax.value = number('duration_max')
  scoreMin.value = number('score_min')
  scoreMax.value = number('score_max')
  sortField.value = typeof q.sort === 'string' ? q.sort : undefined
  sortOrder.value = q.order === 'asc' ? 1 : q.order === 'desc' ? -1 : undefined
  const page = Number(q.page)
  first.value = Number.isFinite(page) && page > 1 ? (page - 1) * rows.value : 0
}

function serializeQuery(): Record<string, string> {
  const query: Record<string, string> = {}
  const states = selectedValues('state')
  if (states.length) query.state = states.join(',')
  const highest = selectedValues('highest')
  if (highest.length) query.highest = highest.join(',')
  const url = textValue('url')
  if (url) query.url = url
  if (durationMin.value) query.duration_min = String(durationMin.value)
  if (durationMax.value) query.duration_max = String(durationMax.value)
  if (scoreMin.value) query.score_min = String(scoreMin.value)
  if (scoreMax.value) query.score_max = String(scoreMax.value)
  if (sortField.value) query.sort = sortField.value
  if (sortOrder.value === 1) query.order = 'asc'
  else if (sortOrder.value === -1) query.order = 'desc'
  if (first.value > 0) {
    query.page = String(Math.floor(first.value / rows.value) + 1)
  }
  return query
}

const { sync } = useTableQuery({
  keys: [
    'state',
    'highest',
    'url',
    'duration_min',
    'duration_max',
    'score_min',
    'score_max',
    'sort',
    'order',
    'page',
  ],
  apply: applyQuery,
  load,
  serialize: serializeQuery,
})

// The open drawer is encoded as ?row=<url> so it is shareable and the
// back/forward buttons work.
function setDrawerQuery(url: string | null): void {
  const current = typeof route.query.row === 'string' ? route.query.row : null
  if (current === url) return
  const query: Record<string, string> = {}
  for (const [key, value] of Object.entries(route.query)) {
    if (typeof value === 'string') query[key] = value
  }
  if (url) query.row = url
  else delete query.row
  router.replace({ query })
}

watch(
  () => route.query.row,
  (value) => {
    const url = typeof value === 'string' && value ? value : null
    if (!url) {
      if (activeUrl.value) closeDrawer()
      return
    }
    if (activeUrl.value?.url === url) return
    const row = items.value.find((r) => r.url === url) ?? ({ url } as UrlRow)
    // Defer past mount so the drawer plays its enter transition (opening it
    // during setup would leave it closed on reload).
    void nextTick(() => openUrl(row))
  },
  { immediate: true },
)

function openIssues(url: string): void {
  router.push({ name: 'issues', params: { auditId: props.auditId }, query: { url } })
}

async function load(): Promise<void> {
  const id = props.auditId
  if (!id) return
  const request = ++requestId
  loading.value = true
  error.value = null
  try {
    const data = await api.listUrls(id, {
      page: Math.floor(first.value / rows.value) + 1,
      limit: rows.value,
      sort: sortField.value,
      order: currentOrder(),
      states: selectedValues('state'),
      highest: selectedValues('highest'),
      durationMin: durationMin.value ?? undefined,
      durationMax: durationMax.value ?? undefined,
      scoreMin: scoreMin.value ?? undefined,
      scoreMax: scoreMax.value ?? undefined,
      url: textValue('url'),
    })
    if (request !== requestId) return
    items.value = data.items
    total.value = data.total
    aggregates.value = data.aggregates
    // Keep the active marker on the open row when it is on this page; a
    // different page (or a filter change) clears it. When the drawer was
    // opened from the URL before the page loaded, backfill its row metadata.
    activeIndex.value = activeUrl.value
      ? items.value.findIndex((u) => u.url === activeUrl.value!.url)
      : -1
    if (activeUrl.value) {
      const found = items.value.find((u) => u.url === activeUrl.value!.url)
      if (found) activeUrl.value = found
    }
  } catch (e) {
    if (request === requestId) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  } finally {
    if (request === requestId) loading.value = false
  }
}

function onPage(event: DataTablePageEvent): void {
  first.value = event.first
  rows.value = event.rows
  sync()
}

function onRowsChange(value: number | null): void {
  if (typeof value !== 'number' || value === rows.value) return
  rows.value = value
  first.value = 0
  sync()
}

function onSort(event: DataTableSortEvent): void {
  sortField.value = (event.sortField as string | undefined) ?? undefined
  sortOrder.value = (event.sortOrder ?? 0) as 1 | 0 | -1
  first.value = 0
  sync()
}

function onFilter(event: DataTableFilterEvent): void {
  filters.value = event.filters
  first.value = 0
  if (filterTimer) clearTimeout(filterTimer)
  filterTimer = setTimeout(sync, 250)
}

function onRangeFilter(): void {
  first.value = 0
  if (filterTimer) clearTimeout(filterTimer)
  filterTimer = setTimeout(sync, 250)
}

type RangeField = 'durationMin' | 'durationMax' | 'scoreMin' | 'scoreMax'

function parseRangeValue(raw: unknown): number | null {
  if (typeof raw === 'number') return Number.isFinite(raw) ? raw : null
  if (typeof raw === 'string' && raw.trim() !== '') {
    const n = Number(raw)
    return Number.isFinite(n) ? n : null
  }
  return null
}

// InputNumber only emits update:model-value on blur, and the filter popup
// unmounts before that when it is dismissed, so the range bounds read the
// live input event instead.
function onRangeInput(field: RangeField, event: { value?: unknown }): void {
  const value = parseRangeValue(event?.value)
  switch (field) {
    case 'durationMin':
      durationMin.value = value
      break
    case 'durationMax':
      durationMax.value = value
      break
    case 'scoreMin':
      scoreMin.value = value
      break
    case 'scoreMax':
      scoreMax.value = value
      break
  }
  onRangeFilter()
}

function resetFilters(): void {
  filters.value = defaultFilters()
  durationMin.value = null
  durationMax.value = null
  scoreMin.value = null
  scoreMax.value = null
  sortField.value = undefined
  sortOrder.value = undefined
  first.value = 0
  load()
}

function onRowClick(event: DataTableRowClickEvent): void {
  // Focus the list so the arrow shortcuts keep working after a click.
  listRef.value?.focus()
  openUrl(event.data as UrlRow)
}

function onRowContextMenu(event: DataTableRowContextMenuEvent): void {
  event.originalEvent.preventDefault()
  menuUrl.value = event.data as UrlRow
  menu.value?.show(event.originalEvent as MouseEvent)
}

function openLink(row: UrlRow): void {
  window.open(row.url, '_blank', 'noopener,noreferrer')
}

function openEdit(row: UrlRow): void {
  if (row.edit_url) {
    window.open(row.edit_url, '_blank', 'noopener,noreferrer')
  }
}

async function rerunUrl(row: UrlRow): Promise<void> {
  const id = props.auditId
  if (!id || rerunning.value) return
  rerunning.value = true
  try {
    await api.recheckUrl(id, row.url)
    notifyDataChanged()
    toast.add({ severity: 'success', summary: 'URL rerun', detail: row.url, life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Rerun failed',
      detail: e instanceof Error ? e.message : String(e),
      life: 5000,
    })
  } finally {
    rerunning.value = false
  }
}

// openUrl opens the drawer with the URL's issues. The drawer doubles as the
// TUI's issue + evidence panes: its left column lists the issues and its
// right column shows the evidence of the selected one.
async function openUrl(row: UrlRow): Promise<void> {
  const id = props.auditId
  if (!id) return
  const index = items.value.findIndex((u) => u.url === row.url)
  if (index >= 0) activeIndex.value = index
  activeUrl.value = row
  drawerVisible.value = true
  setDrawerQuery(row.url)

  const request = ++detailRequest
  loadingIssues.value = true
  try {
    const issues = await api.listUrlIssues(id, row.url)
    if (request !== detailRequest) return
    urlIssues.value = issues
    selectedIssue.value = sortedIssues.value[0] ?? null
  } catch (e) {
    if (request === detailRequest) {
      urlIssues.value = []
      selectedIssue.value = null
      toast.add({
        severity: 'error',
        summary: 'Failed to load issues',
        detail: e instanceof Error ? e.message : String(e),
        life: 5000,
      })
    }
  } finally {
    if (request === detailRequest) loadingIssues.value = false
  }
}

function closeDrawer(): void {
  drawerVisible.value = false
  detailRequest++
  activeUrl.value = null
  urlIssues.value = []
  selectedIssue.value = null
  setDrawerQuery(null)
}

function isActive(data: UrlRow): boolean {
  return data.url === activeUrl.value?.url
}

// rowClass paints the whole active row with the theme's primary color. The
// cells carry the color (with an important modifier) because the theme paints
// each td, hiding a background set on the tr. The important modifier also
// keeps the active color ahead of the DataTable's row-hover, which targets
// the same cells.
function rowClass(data: UrlRow): string {
  return isActive(data)
    ? 'cursor-pointer [&>td]:bg-[var(--p-primary-color)]! [&>td]:text-[var(--p-primary-contrast-color)]!'
    : 'cursor-pointer'
}

// gotoPage loads a page, then opens the given edge row so left/right keep
// flowing across page boundaries.
async function gotoPage(nextFirst: number, edge: 'first' | 'last'): Promise<void> {
  first.value = Math.max(0, nextFirst)
  await sync()
  const index = edge === 'first' ? 0 : items.value.length - 1
  if (index >= 0 && items.value[index]) {
    openUrl(items.value[index])
  }
}

// navigate opens the neighbour of the active row, crossing into the previous
// or next page when the active row sits on an edge.
async function navigate(delta: number): Promise<void> {
  if (!items.value.length) return
  let index = activeIndex.value
  if (index < 0) index = delta > 0 ? -1 : items.value.length
  const target = index + delta

  if (target < 0) {
    if (first.value > 0) {
      await gotoPage(first.value - rows.value, 'last')
    }
    return
  }
  if (target >= items.value.length) {
    if (first.value + items.value.length < total.value) {
      await gotoPage(first.value + rows.value, 'first')
    }
    return
  }
  openUrl(items.value[target])
}

// onWindowKeydown gives the list a keyboard mode while the drawer is open (or
// the list itself is focused). It is registered on the window so the shortcuts
// keep working when focus is inside the drawer, and it ignores typing in form
// fields.
function onWindowKeydown(event: KeyboardEvent): void {
  if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) {
    return
  }
  const target = event.target as HTMLElement | null
  if (
    target &&
    (target.tagName === 'INPUT' ||
      target.tagName === 'TEXTAREA' ||
      target.tagName === 'SELECT' ||
      target.isContentEditable)
  ) {
    return
  }
  const listHasFocus = !!listRef.value?.contains(document.activeElement)
  if (!drawerVisible.value && !listHasFocus) return

  switch (event.key) {
    case 'ArrowRight':
    case 'l':
      event.preventDefault()
      navigate(1)
      break
    case 'ArrowLeft':
    case 'h':
      event.preventDefault()
      navigate(-1)
      break
    case 'Enter':
      if (items.value.length) {
        event.preventDefault()
        openUrl(items.value[activeIndex.value >= 0 ? activeIndex.value : 0])
      }
      break
    case 'Escape':
      if (drawerVisible.value) {
        event.preventDefault()
        closeDrawer()
      }
      break
  }
}

function stateColor(state: string): string {
  switch (state) {
    case 'new':
      return palette.info
    case 'active':
      return palette.success
    case 'missing':
      return palette.danger
    default:
      return palette.neutral
  }
}

function shortUrl(url: string): string {
  return url.replace(/^https?:\/\//, '')
}

function displayUrl(url: string): string {
  switch (urlDisplay.value) {
    case 'full':
      return url
    case 'path':
      try {
        return new URL(url).pathname || '/'
      } catch {
        return url
      }
    default:
      return shortUrl(url)
  }
}

// Reload when audit data changes elsewhere (a run, a recheck, a delete).
watch(dataVersion, () => {
  load()
})

// Only global preferences persist in local storage; filters/sort/page live in
// the URL.
watch(
  [rows, urlDisplay, columnOrder, visibleFields],
  () => {
    saveJSON(SETTINGS_KEY, {
      version: SETTINGS_VERSION,
      rows: rows.value,
      urlDisplay: urlDisplay.value,
      columns: columnOrder.value,
      visibleColumns: visibleFields.value,
    })
  },
  { deep: true },
)

onMounted(() => {
  window.addEventListener('keydown', onWindowKeydown)
})

onBeforeUnmount(() => {
  if (filterTimer) clearTimeout(filterTimer)
  window.removeEventListener('keydown', onWindowKeydown)
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-4">
    <Message v-if="error" severity="error">{{ error }}</Message>

    <div class="hidden grid-cols-3 gap-3 sm:grid md:grid-cols-6">
      <MetricTile
        v-for="tile in widgetTiles"
        :key="tile.label"
        :label="tile.label"
        :value="tile.value"
        :color="tile.color"
      />
    </div>

    <div
      ref="listRef"
      tabindex="0"
      class="min-h-0 flex-1 outline-none"
      aria-label="Audited URLs"
    >
      <DataTable
        :key="columnsKey"
        v-model:filters="filters"
        v-model:first="first"
        v-model:rows="rows"
        v-model:sort-field="sortField"
        v-model:sort-order="sortOrder"
        :value="items"
        :loading="showTableLoading"
        :total-records="total"
        lazy
        paginator
        removable-sort
        filter-display="menu"
        row-hover
        :row-class="rowClass"
        scrollable
        scroll-height="flex"
        data-key="url"
        paginator-template="FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink"
        @page="onPage"
        @sort="onSort"
        @filter="onFilter"
        @row-click="onRowClick"
        @row-contextmenu="onRowContextMenu"
      >
        <template #paginatorstart>
          <span class="text-sm text-slate-500">{{ pageReport }}</span>
        </template>
        <template #paginatorend>
          <div class="flex items-center gap-1 text-sm text-slate-500">
            <Button label="Clear" text size="small" @click="resetFilters">
              <template #icon><FilterSlash :size="16" /></template>
            </Button>
            <Button
              label="Columns"
              text
              size="small"
              aria-label="Toggle columns"
              @click="toggleColumns"
            >
              <template #icon><Cog :size="16" /></template>
            </Button>
            <Button
              label="Refresh"
              text
              size="small"
              aria-label="Refresh URLs"
              :loading="loading"
              @click="load"
            >
              <template #icon><Refresh :size="16" /></template>
            </Button>
            <span class="mx-1 h-4 w-px bg-slate-200" aria-hidden="true" />
            <span class="whitespace-nowrap">Rows per page</span>
            <Select
              :model-value="rows"
              :options="pageSizeOptions"
              size="small"
              class="w-24"
              @update:model-value="onRowsChange"
            />
          </div>
        </template>
        <template #empty>
          <EmptyState v-if="!loading" @reset="resetFilters" />
        </template>

        <template v-for="field in visibleOrderedFields" :key="field">
        <Column
          v-if="field === 'url'"
          field="url"
          sortable
          filter-match-mode="contains"
          :filter-menu-style="{ minWidth: '16rem' }"
          style="min-width: 20rem"
        >
          <template #header>
            <span class="p-datatable-column-title">URL</span>
            <div
              class="ml-1 inline-flex overflow-hidden rounded border border-slate-200"
              @click.stop
            >
              <button
                v-for="opt in urlDisplayOptions"
                :key="opt.value"
                type="button"
                class="flex h-5 w-5 items-center justify-center transition-colors"
                :class="
                  urlDisplay === opt.value
                    ? 'bg-slate-200 text-slate-700'
                    : 'text-slate-400 hover:bg-slate-100'
                "
                v-tooltip.top="opt.title"
                :aria-label="opt.title"
                :aria-pressed="urlDisplay === opt.value"
                @click="urlDisplay = opt.value"
              >
                <component :is="opt.icon" :size="13" />
              </button>
            </div>
          </template>
          <template #body="{ data }">
            <a
              :href="data.url"
              target="_blank"
              rel="noopener noreferrer"
              class="hover:underline"
              :class="
                isActive(data)
                  ? 'text-[var(--p-primary-contrast-color)]'
                  : 'text-[var(--p-primary-color)]'
              "
              :title="data.url"
              @click.stop
            >
              {{ displayUrl(data.url) }}
            </a>
          </template>
          <template #filter="{ filterModel, filterCallback }">
            <InputText
              v-model="filterModel.value"
              type="text"
              placeholder="Search URL"
              class="w-full"
              @input="filterCallback()"
            />
          </template>
        </Column>

        <Column
          v-else-if="field === 'title'"
          field="title"
          header="Title"
          sortable
          style="min-width: 16rem"
        >
          <template #body="{ data }">
            <span
              v-if="data.title"
              class="block max-w-xs truncate"
              :title="data.title"
            >
              {{ data.title }}
            </span>
            <span v-else class="text-slate-400">–</span>
          </template>
        </Column>

        <Column
          v-else-if="field === 'duration'"
          field="duration"
          header="Duration"
          sortable
          :show-filter-match-modes="false"
          :show-apply-button="false"
          :show-clear-button="false"
          style="min-width: 11rem"
        >
          <template #body="{ data }">
            <span class="tabular-nums">
              {{ formatDuration(data.duration_ms) }}
            </span>
          </template>
          <template #filter>
            <div class="grid grid-cols-1 gap-2">
              <InputNumber
                :model-value="durationMin"
                placeholder="Min"
                :use-grouping="false"
                class="w-24"
                @input="onRangeInput('durationMin', $event)"
              />
              <InputNumber
                :model-value="durationMax"
                placeholder="Max"
                :use-grouping="false"
                class="w-24"
                @input="onRangeInput('durationMax', $event)"
              />
            </div>
          </template>
        </Column>

        <Column
          v-else-if="field === 'state'"
          field="state"
          header="State"
          sortable
          filter-match-mode="in"
          :show-filter-match-modes="false"
          :filter-menu-style="{ minWidth: '14rem' }"
          style="min-width: 8rem"
        >
          <template #body="{ data }">
            <span class="inline-flex items-center gap-1.5">
              <span
                class="h-2 w-2 rounded-full"
                :style="{ backgroundColor: stateColor(data.state) }"
              />
              <span class="text-sm capitalize">{{ data.state }}</span>
            </span>
          </template>
          <template #filter="{ filterModel, filterCallback }">
            <MultiSelect
              v-model="filterModel.value"
              :options="stateOptions"
              option-label="label"
              option-value="value"
              placeholder="Any"
              :show-clear="true"
              class="w-full"
              @change="filterCallback()"
            />
          </template>
        </Column>

        <Column
          v-else-if="field === 'highest'"
          field="highest"
          header="Highest"
          sortable
          filter-match-mode="in"
          :show-filter-match-modes="false"
          :filter-menu-style="{ minWidth: '14rem' }"
          style="min-width: 8rem"
        >
          <template #body="{ data }">
            <SeverityTag :severity="data.highest_severity" />
          </template>
          <template #filter="{ filterModel, filterCallback }">
            <MultiSelect
              v-model="filterModel.value"
              :options="highestOptions"
              option-label="label"
              option-value="value"
              placeholder="Any"
              :show-clear="true"
              class="w-full"
              @change="filterCallback()"
            />
          </template>
        </Column>

        <Column
          v-else-if="field === 'score'"
          field="score"
          header="Score"
          sortable
          :show-filter-match-modes="false"
          :show-apply-button="false"
          :show-clear-button="false"
          style="min-width: 11rem"
        >
          <template #body="{ data }">
            <span
              class="font-semibold tabular-nums"
              :style="{
                color: isActive(data)
                  ? 'var(--p-primary-contrast-color)'
                  : auditScoreColor(data.score),
              }"
            >
              {{ data.score.toFixed(1) }}
            </span>
          </template>
          <template #filter>
            <div class="grid grid-cols-1 gap-2">
              <InputNumber
                :model-value="scoreMin"
                placeholder="Min"
                :use-grouping="false"
                class="w-24"
                @input="onRangeInput('scoreMin', $event)"
              />
              <InputNumber
                :model-value="scoreMax"
                placeholder="Max"
                :use-grouping="false"
                class="w-24"
                @input="onRangeInput('scoreMax', $event)"
              />
            </div>
          </template>
        </Column>
        </template>
      </DataTable>
    </div>

    <ContextMenu ref="menu" :model="menuItems" @hide="menuUrl = null" />

    <Popover ref="columnPopover" class="w-56" pt:content="p-0!">
      <div
        class="flex items-center justify-between gap-2 border-b border-slate-200 px-3 py-2"
      >
        <span class="text-sm font-semibold text-slate-700">Columns</span>
        <Button label="Reset" text size="small" @click="resetColumns" />
      </div>
      <div
        class="max-h-80 overflow-auto py-1"
        @dragover.prevent
        @drop="onColumnDrop"
      >
        <div
          v-for="(field, index) in columnOrder"
          :key="field"
          class="mx-1 flex cursor-move items-center gap-2 rounded-md px-2 py-1.5 transition select-none"
          :class="[
            dragIndex === index ? 'opacity-40' : '',
            dragOverIndex === index && dragIndex !== index
              ? 'bg-slate-100 ring-1 ring-slate-300'
              : 'hover:bg-slate-100',
          ]"
          draggable="true"
          @dragstart="onColumnDragStart(index)"
          @dragover.prevent="onColumnDragOver(index)"
          @dragend="onColumnDragEnd"
        >
          <span class="flex text-slate-400"><Bars :size="14" /></span>
          <Checkbox
            v-model="visibleFields"
            :value="field"
            :input-id="`url-col-${field}`"
          />
          <label
            :for="`url-col-${field}`"
            class="cursor-pointer text-sm text-slate-700"
          >
            {{ columnLabels[field] }}
          </label>
        </div>
      </div>
    </Popover>

    <Drawer
      v-model:visible="drawerVisible"
      position="right"
      :modal="false"
      :dismissable="false"
      :header="drawerTitle"
      style="width: 56rem; max-width: 96vw"
      @hide="closeDrawer"
    >
      <div v-if="activeUrl" class="flex flex-col gap-4">
        <div class="flex flex-wrap items-center gap-3 text-sm">
          <span v-if="activeUrl.state" class="inline-flex items-center gap-1.5">
            <span
              class="h-2 w-2 rounded-full"
              :style="{ backgroundColor: stateColor(activeUrl.state) }"
            />
            <span class="text-slate-600 capitalize">{{ activeUrl.state }}</span>
          </span>
          <span class="text-slate-400">{{ formatDuration(activeUrl.duration_ms) }}</span>
          <SeverityTag v-if="activeUrl.highest_severity" :severity="activeUrl.highest_severity" />
          <span
            v-if="activeUrl.score != null"
            class="font-semibold tabular-nums"
            :style="{ color: auditScoreColor(activeUrl.score) }"
          >
            {{ activeUrl.score.toFixed(1) }}
          </span>
          <span v-if="activeUrl.status_code" class="text-xs text-slate-400">
            HTTP {{ activeUrl.status_code }}
          </span>
        </div>

        <a
          :href="activeUrl.url"
          target="_blank"
          rel="noopener noreferrer"
          class="break-all text-sm text-[var(--p-primary-color)] hover:underline"
        >
          {{ activeUrl.url }}
        </a>

        <div class="grid min-h-0 grid-cols-1 gap-4 md:grid-cols-2">
          <div class="min-h-0">
            <div
              class="mb-2 flex items-center gap-2 text-xs font-medium tracking-wide text-slate-400 uppercase"
            >
              Issues
              <span class="text-slate-300">{{ sortedIssues.length }}</span>
            </div>
            <div class="h-[58vh] overflow-y-auto rounded-lg border border-slate-200">
              <Transition name="fade" mode="out-in">
                <div v-if="showIssuesSkeleton" key="skeleton" class="flex flex-col">
                  <div
                    v-for="n in 6"
                    :key="n"
                    class="flex flex-col gap-2 border-b border-slate-100 px-3 py-2"
                  >
                    <div class="flex items-center gap-2">
                      <Skeleton
                        width="3.5rem"
                        height="1.25rem"
                        border-radius="9999px"
                      />
                      <Skeleton width="6rem" height="0.75rem" />
                    </div>
                    <Skeleton width="85%" height="0.75rem" />
                  </div>
                </div>
                <p
                  v-else-if="!loadingIssues && !sortedIssues.length"
                  key="empty"
                  class="p-3 text-sm text-slate-400"
                >
                  No issues for this URL.
                </p>
                <div v-else key="list">
                  <button
                    v-for="issue in sortedIssues"
                    :key="issue.id"
                    type="button"
                    class="flex w-full flex-col gap-1 border-b border-slate-100 px-3 py-2 text-left transition-colors"
                    :class="
                      selectedIssue?.id === issue.id
                        ? 'bg-slate-100'
                        : 'hover:bg-slate-50'
                    "
                    @click="selectedIssue = issue"
                  >
                    <span class="flex items-center gap-2">
                      <SeverityTag :severity="issue.severity" />
                      <span
                        class="text-xs text-slate-500"
                        v-tooltip.top="checkDescription(issue.check_name)"
                      >
                        {{ checkLabel(issue.check_name) }}
                      </span>
                    </span>
                    <span class="text-xs text-slate-600">{{ issue.message }}</span>
                  </button>
                </div>
              </Transition>
            </div>
          </div>

          <div class="min-h-0">
            <div
              class="mb-2 text-xs font-medium tracking-wide text-slate-400 uppercase"
            >
              Evidence
            </div>
            <div class="h-[58vh] overflow-auto rounded-lg bg-slate-50 p-3">
              <Transition name="fade" mode="out-in">
                <div
                  v-if="showIssuesSkeleton"
                  key="skeleton"
                  class="flex flex-col gap-2"
                >
                  <Skeleton width="40%" height="0.75rem" />
                  <Skeleton width="90%" height="0.75rem" />
                  <Skeleton width="72%" height="0.75rem" />
                  <Skeleton width="84%" height="0.75rem" />
                  <Skeleton width="58%" height="0.75rem" />
                </div>
                <p
                  v-else-if="!loadingIssues && !selectedIssue"
                  key="empty"
                  class="text-sm text-slate-400"
                >
                  Select an issue to see its evidence.
                </p>
                <p v-else-if="!evidence" key="none" class="text-sm text-slate-400">
                  No evidence recorded for this issue.
                </p>
                <pre
                  v-else
                  key="evidence"
                  class="font-mono text-xs whitespace-pre-wrap break-all text-slate-700"
                  >{{ JSON.stringify(evidence, null, 2) }}</pre
                >
              </Transition>
            </div>
          </div>
        </div>
      </div>
    </Drawer>
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
