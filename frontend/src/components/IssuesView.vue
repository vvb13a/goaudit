<script setup lang="ts">
import {
  Directions,
  ExternalLink,
  Eye,
  FilterSlash,
  Globe,
  Link,
  Play,
  Refresh,
} from '@primeicons/vue'
import Button from 'primevue/button'
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
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
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
import {
  categories,
  categoryLabel,
  lifecycles,
  lifecycleMeta,
  palette,
  severities,
} from '../lib/theme'
import { timeAgo } from '../lib/format'
import { dataVersion, notifyDataChanged } from '../lib/appState'
import { useCheckMeta } from '../lib/checkMeta'
import { useColumnLayout } from '../lib/columnLayout'
import { useDelayedLoading } from '../lib/loading'
import { loadJSON, saveJSON } from '../lib/storage'
import { useTableQuery } from '../lib/tableQuery'
import type { Issue, SeverityCounts } from '../types'
import ColumnToggle from './ColumnToggle.vue'
import EmptyState from './EmptyState.vue'
import MetricTile from './MetricTile.vue'
import SeverityTag from './SeverityTag.vue'

const props = defineProps<{ auditId: string }>()

const route = useRoute()
const router = useRouter()
const { label: checkLabel, description: checkDescription } = useCheckMeta()

const toast = useToast()

const items = ref<Issue[]>([])
const total = ref(0)
const loading = ref(false)
const showTableLoading = useDelayedLoading(loading)
const error = ref<string | null>(null)

const severityCounts = ref<SeverityCounts>({
  success: 0,
  notice: 0,
  warning: 0,
  error: 0,
  fatal: 0,
})

const rerunning = ref(false)

// How the URL column renders each row: the whole URL, the URL without its
// scheme, or just the path.
type UrlDisplay = 'full' | 'domain' | 'path'

// Filters, sorting and page live in the URL query; only global preferences
// (page size, URL display) persist in local storage.
interface SavedIssueSettings {
  version: number
  rows: number
  urlDisplay?: UrlDisplay
}

const SETTINGS_KEY = 'issues-view'
const SETTINGS_VERSION = 3
const storedSettings = loadJSON<SavedIssueSettings>(SETTINGS_KEY)
const savedSettings =
  storedSettings?.version === SETTINGS_VERSION ? storedSettings : null

const rows = ref(savedSettings?.rows ?? 25)
const first = ref(0)
const sortField = ref<string | undefined>(undefined)
const sortOrder = ref<1 | 0 | -1 | undefined>(undefined)

const urlDisplay = ref<UrlDisplay>(savedSettings?.urlDisplay ?? 'domain')
const urlDisplayOptions: { value: UrlDisplay; title: string; icon: Component }[] =
  [
    { value: 'full', title: 'Show full URL', icon: Link },
    { value: 'domain', title: 'Show domain and path', icon: Globe },
    { value: 'path', title: 'Show path only', icon: Directions },
  ]

const checkNames = ref<string[]>([])

// Drawer state. The row's brief issue shows immediately; its findings load on
// demand so browsing a page never fetches evidence for rows that are not
// opened.
const drawerVisible = ref(false)
const activeIssue = ref<Issue | null>(null)
const activeDetails = ref<Record<string, unknown> | null>(null)
const loadingDetail = ref(false)
const detailError = ref<string | null>(null)
let detailRequest = 0

// The row whose drawer is open. Held as an index into the current page so
// arrow navigation can step to the neighbours (and across pages).
const listRef = ref<HTMLElement | null>(null)
const activeIndex = ref(-1)

// Findings only flash a skeleton once the load is slow enough to warrant it.
const showDetailSkeleton = useDelayedLoading(loadingDetail)

type MatchMode = 'in' | 'contains'
interface ColumnFilter {
  value: unknown
  matchMode: MatchMode
}

// Unlike the TUI, the web view hides nothing by default: every severity,
// including successes, is shown until the user filters it out.
function defaultFilters(): Record<string, ColumnFilter> {
  return {
    severity: { value: null, matchMode: 'in' },
    lifecycle: { value: null, matchMode: 'in' },
    check: { value: null, matchMode: 'in' },
    category: { value: null, matchMode: 'in' },
    url: { value: null, matchMode: 'contains' },
    message: { value: null, matchMode: 'contains' },
  }
}

const filters = ref<DataTableFilterMeta>(defaultFilters())

const severityOptions = severities.map((s) => ({ label: s.label, value: s.key }))
const lifecycleOptions = lifecycles.map((l) => ({ label: l.label, value: l.key }))
const categoryOptions = categories.map((c) => ({ label: c.label, value: c.key }))
const checkOptions = computed(() =>
  checkNames.value.map((name) => ({ label: checkLabel(name), value: name })),
)

const pageSizeOptions = [10, 25, 50, 100]

const {
  labels: columnLabels,
  columnOrder,
  visibleFields,
  visibleOrderedFields,
  columnsKey,
  reset: resetColumns,
} = useColumnLayout('issues', [
  { field: 'severity', label: 'Severity' },
  { field: 'lifecycle', label: 'Lifecycle' },
  { field: 'check', label: 'Check' },
  { field: 'category', label: 'Category' },
  { field: 'url', label: 'URL' },
  { field: 'message', label: 'Issue' },
])

const pageReport = computed(() => {
  if (total.value === 0) return '0 of 0'
  const start = first.value + 1
  const end = Math.min(first.value + rows.value, total.value)
  return `${start}–${end} of ${total.value.toLocaleString()}`
})

const filteredTotal = computed(() =>
  severities.reduce((sum, meta) => sum + severityCounts.value[meta.key], 0),
)

// The widget reflects the whole filtered set, not just the current page. All
// tiles use the primary color so the view stays calm next to the semantic
// table cells and tags.
const severityTiles = computed(() => [
  { label: 'Total', value: String(filteredTotal.value), color: palette.primary },
  ...severities.map((meta) => ({
    label: meta.label,
    value: String(severityCounts.value[meta.key]),
    color: palette.primary,
  })),
])

// Context menu state. The menu acts on the issue of the right-clicked row.
const menu = ref<InstanceType<typeof ContextMenu>>()
const menuIssue = ref<Issue | null>(null)

const menuItems = computed(() => [
  {
    label: 'View evidence',
    icon: Eye,
    command: () => menuIssue.value && openIssue(menuIssue.value),
  },
  {
    label: 'Open link',
    icon: ExternalLink,
    command: () => menuIssue.value && openLink(menuIssue.value),
  },
  {
    label: 'Open in URLs',
    icon: Link,
    command: () => menuIssue.value && openUrls(menuIssue.value.url),
  },
  {
    label: 'Rerun issue',
    icon: Play,
    disabled: rerunning.value,
    command: () => menuIssue.value && rerunIssue(menuIssue.value),
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

  filters.value = {
    severity: { value: list('severity').length ? list('severity') : null, matchMode: 'in' },
    lifecycle: { value: list('lifecycle').length ? list('lifecycle') : null, matchMode: 'in' },
    check: { value: list('check').length ? list('check') : null, matchMode: 'in' },
    category: { value: list('category').length ? list('category') : null, matchMode: 'in' },
    url: { value: single('url'), matchMode: 'contains' },
    message: { value: single('message'), matchMode: 'contains' },
  }
  sortField.value = typeof q.sort === 'string' ? q.sort : undefined
  sortOrder.value = q.order === 'asc' ? 1 : q.order === 'desc' ? -1 : undefined
  const page = Number(q.page)
  first.value = Number.isFinite(page) && page > 1 ? (page - 1) * rows.value : 0
}

function serializeQuery(): Record<string, string> {
  const query: Record<string, string> = {}
  const putList = (key: string): void => {
    const values = selectedValues(key)
    if (values.length) query[key] = values.join(',')
  }
  const putText = (key: string): void => {
    const value = textValue(key)
    if (value) query[key] = value
  }
  putList('severity')
  putList('lifecycle')
  putList('check')
  putList('category')
  putText('url')
  putText('message')
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
    'severity',
    'lifecycle',
    'check',
    'category',
    'url',
    'message',
    'sort',
    'order',
    'page',
  ],
  apply: applyQuery,
  load,
  serialize: serializeQuery,
})

// The open drawer is encoded as ?issue=<id> so it is shareable and the
// back/forward buttons work.
function setDrawerQuery(id: string | null): void {
  const current = typeof route.query.issue === 'string' ? route.query.issue : null
  if (current === id) return
  const query: Record<string, string> = {}
  for (const [key, value] of Object.entries(route.query)) {
    if (typeof value === 'string') query[key] = value
  }
  if (id) query.issue = id
  else delete query.issue
  router.replace({ query })
}

watch(
  () => route.query.issue,
  (value) => {
    const id = typeof value === 'string' && value ? value : null
    if (!id) {
      if (activeIssue.value) closeDrawer()
      return
    }
    if (activeIssue.value?.id === id) return
    const issue = items.value.find((i) => i.id === id) ?? ({ id } as Issue)
    // Defer past mount so the drawer plays its enter transition (opening it
    // during setup would leave it closed on reload).
    void nextTick(() => openIssue(issue))
  },
  { immediate: true },
)

function openUrls(url: string): void {
  router.push({ name: 'urls', params: { auditId: props.auditId }, query: { url } })
}

async function load(): Promise<void> {
  const id = props.auditId
  if (!id) return
  const request = ++requestId
  loading.value = true
  error.value = null
  try {
    const data = await api.listIssues(id, {
      page: Math.floor(first.value / rows.value) + 1,
      limit: rows.value,
      sort: sortField.value,
      order: currentOrder(),
      severities: selectedValues('severity'),
      lifecycles: selectedValues('lifecycle'),
      checks: selectedValues('check'),
      categories: selectedValues('category'),
      url: textValue('url'),
      message: textValue('message'),
    })
    if (request !== requestId) return
    items.value = data.items
    total.value = data.total
    severityCounts.value = data.severity_counts
    // Keep the active marker on the open row when it is on this page; a
    // different page (or a filter change) clears it.
    activeIndex.value = activeIssue.value
      ? items.value.findIndex((i) => i.id === activeIssue.value!.id)
      : -1
  } catch (e) {
    if (request === requestId) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  } finally {
    if (request === requestId) loading.value = false
  }
}

async function loadCheckNames(): Promise<void> {
  const id = props.auditId
  if (!id) return
  try {
    const data = await api.getIssueFilters(id)
    checkNames.value = data.check_names
  } catch {
    checkNames.value = []
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

// Row filters fire on every keystroke / selection; debounce so typing a URL
// does not flood the API (and the URL history).
function onFilter(event: DataTableFilterEvent): void {
  filters.value = event.filters
  first.value = 0
  if (filterTimer) clearTimeout(filterTimer)
  filterTimer = setTimeout(sync, 250)
}

function resetFilters(): void {
  filters.value = defaultFilters()
  sortField.value = undefined
  sortOrder.value = undefined
  first.value = 0
  sync()
}

function onRowClick(event: DataTableRowClickEvent): void {
  // Focus the list so the arrow shortcuts keep working after a click.
  listRef.value?.focus()
  openIssue(event.data as Issue)
}

function onRowContextMenu(event: DataTableRowContextMenuEvent): void {
  event.originalEvent.preventDefault()
  menuIssue.value = event.data as Issue
  menu.value?.show(event.originalEvent as MouseEvent)
}

function openLink(issue: Issue): void {
  window.open(issue.url, '_blank', 'noopener,noreferrer')
}

// rerunIssue re-audits the target behind the issue's page and refreshes the
// list so the affected row reflects the fresh result.
async function rerunIssue(issue: Issue): Promise<void> {
  const id = props.auditId
  if (!id || rerunning.value) return
  rerunning.value = true
  try {
    await api.recheckUrl(id, issue.url)
    notifyDataChanged()
    toast.add({
      severity: 'success',
      summary: 'Issue rerun',
      detail: issue.url,
      life: 3000,
    })
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

function closeDrawer(): void {
  drawerVisible.value = false
  detailRequest++
  activeIssue.value = null
  activeDetails.value = null
  setDrawerQuery(null)
}

// openIssue points the drawer at the clicked row and loads that issue's
// findings. Rapid clicks are guarded so a slow response for a previous row
// cannot overwrite the current one.
async function openIssue(issue: Issue): Promise<void> {
  const id = props.auditId
  if (!id) return
  const index = items.value.findIndex((i) => i.id === issue.id)
  if (index >= 0) activeIndex.value = index
  activeIssue.value = issue
  activeDetails.value = null
  detailError.value = null
  drawerVisible.value = true
  setDrawerQuery(issue.id)

  const request = ++detailRequest
  loadingDetail.value = true
  try {
    const full = await api.getIssue(id, issue.id)
    if (request !== detailRequest) return
    activeIssue.value = full
    activeDetails.value = full.details ?? null
  } catch (e) {
    if (request === detailRequest) {
      detailError.value = e instanceof Error ? e.message : String(e)
    }
  } finally {
    if (request === detailRequest) loadingDetail.value = false
  }
}


function lifecycleColor(lifecycle: string): string {
  return lifecycleMeta(lifecycle)?.color ?? palette.neutral
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

function isActive(issue: Issue): boolean {
  return issue.id === activeIssue.value?.id
}

// rowClass paints the whole active row with the theme's primary color. The
// cells carry the color (with an important modifier) because the theme paints
// each td, hiding a background set on the tr. The important modifier also
// keeps the active color ahead of the DataTable's row-hover.
function rowClass(data: Issue): string {
  return isActive(data)
    ? 'cursor-pointer [&>td]:bg-[var(--p-primary-color)]! [&>td]:text-[var(--p-primary-contrast-color)]!'
    : 'cursor-pointer text-slate-600'
}

// gotoPage loads a page, then opens the given edge row so left/right keep
// flowing across page boundaries.
async function gotoPage(nextFirst: number, edge: 'first' | 'last'): Promise<void> {
  first.value = Math.max(0, nextFirst)
  await sync()
  const index = edge === 'first' ? 0 : items.value.length - 1
  if (index >= 0 && items.value[index]) {
    openIssue(items.value[index])
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
  openIssue(items.value[target])
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
        openIssue(items.value[activeIndex.value >= 0 ? activeIndex.value : 0])
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

// Check names feed the check filter options; refetch when the audit changes.
watch(() => props.auditId, loadCheckNames, { immediate: true })

// Reload when audit data changes elsewhere (a run, a recheck, a delete).
watch(dataVersion, () => {
  load()
})

// Only global preferences persist in local storage; filters/sort/page live in
// the URL.
watch([rows, urlDisplay], () => {
  saveJSON(SETTINGS_KEY, {
    version: SETTINGS_VERSION,
    rows: rows.value,
    urlDisplay: urlDisplay.value,
  })
})

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
        v-for="tile in severityTiles"
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
      aria-label="Issues"
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
        data-key="id"
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
            <ColumnToggle
              v-model:order="columnOrder"
              v-model:visible="visibleFields"
              :labels="columnLabels"
              @reset="resetColumns"
            />
            <Button
              label="Refresh"
              text
              size="small"
              aria-label="Refresh issues"
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
          v-if="field === 'severity'"
          field="severity"
          header="Severity"
          sortable
          filter-match-mode="in"
          :show-filter-match-modes="false"
          :filter-menu-style="{ minWidth: '16rem' }"
          style="min-width: 9rem"
        >
          <template #body="{ data }">
            <SeverityTag :severity="data.severity" />
          </template>
          <template #filter="{ filterModel, filterCallback }">
            <MultiSelect
              v-model="filterModel.value"
              :options="severityOptions"
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
          v-else-if="field === 'lifecycle'"
          field="lifecycle"
          header="Lifecycle"
          sortable
          filter-match-mode="in"
          :show-filter-match-modes="false"
          :filter-menu-style="{ minWidth: '16rem' }"
          style="min-width: 9rem"
        >
          <template #body="{ data }">
            <span class="inline-flex items-center gap-1.5">
              <span
                class="h-2 w-2 rounded-full"
                :style="{ backgroundColor: lifecycleColor(data.lifecycle) }"
              />
              <span class="text-sm">
                {{ lifecycleMeta(data.lifecycle)?.label ?? data.lifecycle }}
              </span>
            </span>
          </template>
          <template #filter="{ filterModel, filterCallback }">
            <MultiSelect
              v-model="filterModel.value"
              :options="lifecycleOptions"
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
          v-else-if="field === 'check'"
          field="check"
          header="Check"
          sortable
          filter-match-mode="in"
          :show-filter-match-modes="false"
          :filter-menu-style="{ minWidth: '16rem' }"
          style="min-width: 10rem"
        >
          <template #body="{ data }">
            <span
              class="text-sm"
              v-tooltip.top="checkDescription(data.check_name)"
            >
              {{ checkLabel(data.check_name) }}
            </span>
          </template>
          <template #filter="{ filterModel, filterCallback }">
            <MultiSelect
              v-model="filterModel.value"
              :options="checkOptions"
              option-label="label"
              option-value="value"
              placeholder="Any"
              :show-clear="true"
              filter
              class="w-full"
              @change="filterCallback()"
            />
          </template>
        </Column>

        <Column
          v-else-if="field === 'category'"
          field="category"
          header="Category"
          sortable
          filter-match-mode="in"
          :show-filter-match-modes="false"
          :filter-menu-style="{ minWidth: '16rem' }"
          style="min-width: 9rem"
        >
          <template #body="{ data }">
            <span class="text-sm">
              {{ categoryLabel(data.category) }}
            </span>
          </template>
          <template #filter="{ filterModel, filterCallback }">
            <MultiSelect
              v-model="filterModel.value"
              :options="categoryOptions"
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
          v-else-if="field === 'url'"
          field="url"
          sortable
          filter-match-mode="contains"
          :filter-menu-style="{ minWidth: '16rem' }"
          style="min-width: 18rem"
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
          v-else-if="field === 'message'"
          field="message"
          header="Issue"
          sortable
          filter-match-mode="contains"
          :filter-menu-style="{ minWidth: '16rem' }"
          style="min-width: 18rem"
        >
          <template #body="{ data }">
            <span class="text-sm" :title="data.message">
              {{ data.message }}
            </span>
          </template>
          <template #filter="{ filterModel, filterCallback }">
            <InputText
              v-model="filterModel.value"
              type="text"
              placeholder="Search issue"
              class="w-full"
              @input="filterCallback()"
            />
          </template>
        </Column>
        </template>
      </DataTable>
    </div>

    <Drawer
      v-model:visible="drawerVisible"
      position="right"
      :modal="false"
      :dismissable="false"
      :header="activeIssue ? checkLabel(activeIssue.check_name) : 'Issue'"
      style="width: 32rem; max-width: 90vw"
      @hide="closeDrawer"
    >
      <div v-if="activeIssue" class="flex flex-col gap-4">
        <div class="flex flex-wrap items-center gap-2">
          <SeverityTag :severity="activeIssue.severity" />
          <span class="inline-flex items-center gap-1.5 text-sm text-slate-600">
            <span
              class="h-2 w-2 rounded-full"
              :style="{ backgroundColor: lifecycleColor(activeIssue.lifecycle) }"
            />
            {{ lifecycleMeta(activeIssue.lifecycle)?.label ?? activeIssue.lifecycle }}
          </span>
          <span class="text-sm text-slate-400">
            {{ categoryLabel(activeIssue.category) }}
          </span>
          <span
            v-if="activeIssue.prior_severity"
            class="text-xs text-slate-400"
          >
            was {{ activeIssue.prior_severity }}
          </span>
        </div>

        <div>
          <div
            class="text-xs font-medium tracking-wide text-slate-400 uppercase"
          >
            Message
          </div>
          <p class="mt-1 text-sm text-slate-700">{{ activeIssue.message }}</p>
        </div>

        <div>
          <div
            class="text-xs font-medium tracking-wide text-slate-400 uppercase"
          >
            Page
          </div>
          <a
            :href="activeIssue.url"
            target="_blank"
            rel="noopener noreferrer"
            class="mt-1 block break-all text-sm text-[var(--p-primary-color)] hover:underline"
          >
            {{ activeIssue.url }}
          </a>
        </div>

        <div class="flex gap-6 text-xs text-slate-400">
          <span>First seen {{ timeAgo(activeIssue.created_at) }}</span>
          <span>Updated {{ timeAgo(activeIssue.updated_at) }}</span>
        </div>

        <div>
          <div
            class="mb-1 text-xs font-medium tracking-wide text-slate-400 uppercase"
          >
            Findings
          </div>

          <Message v-if="detailError" severity="error">
            {{ detailError }}
          </Message>
          <Transition v-else name="fade" mode="out-in">
            <div
              v-if="showDetailSkeleton"
              key="skeleton"
              class="flex flex-col gap-2"
            >
              <Skeleton width="40%" height="0.75rem" />
              <Skeleton width="90%" height="0.75rem" />
              <Skeleton width="72%" height="0.75rem" />
              <Skeleton width="84%" height="0.75rem" />
            </div>
            <p
              v-else-if="!loadingDetail && !activeDetails"
              key="empty"
              class="text-sm text-slate-400"
            >
              No findings recorded for this issue.
            </p>
            <pre
              v-else-if="activeDetails"
              key="findings"
              class="max-h-[50vh] overflow-auto rounded bg-slate-50 p-3 text-xs text-slate-700"
              >{{ JSON.stringify(activeDetails, null, 2) }}</pre
            >
          </Transition>
        </div>
      </div>
    </Drawer>

    <ContextMenu ref="menu" :model="menuItems" @hide="menuIssue = null" />
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
