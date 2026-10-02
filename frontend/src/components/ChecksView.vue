<script setup lang="ts">
import { FilterSlash, List, Refresh } from '@primeicons/vue'
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
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import Select from 'primevue/select'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter, type LocationQuery } from 'vue-router'
import * as api from '../api/client'
import { categories, categoryLabel, palette, severities } from '../lib/theme'
import { dataVersion } from '../lib/appState'
import { useDelayedLoading } from '../lib/loading'
import { loadJSON, saveJSON } from '../lib/storage'
import { useTableQuery } from '../lib/tableQuery'
import type { CheckSummary, SeverityCounts } from '../types'
import EmptyState from './EmptyState.vue'
import MetricTile from './MetricTile.vue'
import SeverityTag from './SeverityTag.vue'

const props = defineProps<{ auditId: string }>()

const router = useRouter()

const items = ref<CheckSummary[]>([])
const total = ref(0)
const highestCounts = ref<SeverityCounts>({
  success: 0,
  notice: 0,
  warning: 0,
  error: 0,
  fatal: 0,
})
const loading = ref(false)
const showTableLoading = useDelayedLoading(loading)
const error = ref<string | null>(null)

const severityOrder: (keyof SeverityCounts)[] = [
  'fatal',
  'error',
  'warning',
  'notice',
  'success',
]

function highestSeverity(counts: SeverityCounts): string {
  return severityOrder.find((k) => counts[k] > 0) ?? ''
}

// Filters, sorting and page live in the URL query; only the page size persists
// in local storage.
interface SavedCheckSettings {
  version: number
  rows: number
}

const SETTINGS_KEY = 'checks-view'
const SETTINGS_VERSION = 2
const storedSettings = loadJSON<SavedCheckSettings>(SETTINGS_KEY)
const saved =
  storedSettings?.version === SETTINGS_VERSION ? storedSettings : null

const rows = ref(saved?.rows ?? 25)
const first = ref(0)
const sortField = ref<string | undefined>(undefined)
const sortOrder = ref<1 | 0 | -1 | undefined>(undefined)

const fatalMin = ref<number | null>(null)
const errorMin = ref<number | null>(null)
const warningMin = ref<number | null>(null)
const noticeMin = ref<number | null>(null)
const successMin = ref<number | null>(null)

type MatchMode = 'in' | 'contains'
interface ColumnFilter {
  value: unknown
  matchMode: MatchMode
}

function defaultFilters(): Record<string, ColumnFilter> {
  return {
    name: { value: null, matchMode: 'contains' },
    category: { value: null, matchMode: 'in' },
    highest: { value: null, matchMode: 'in' },
    fatal: { value: null, matchMode: 'contains' },
    error: { value: null, matchMode: 'contains' },
    warning: { value: null, matchMode: 'contains' },
    notice: { value: null, matchMode: 'contains' },
    success: { value: null, matchMode: 'contains' },
  }
}

const filters = ref<DataTableFilterMeta>(defaultFilters())

const categoryOptions = categories.map((c) => ({ label: c.label, value: c.key }))
const highestOptions = severities.map((s) => ({ label: s.label, value: s.key }))
const pageSizeOptions = [10, 25, 50, 100]

// Total checks plus one box per severity, counting checks by their highest
// severity rather than issues.
const widgetTiles = computed(() => [
  { label: 'Total', value: String(total.value), color: palette.primary },
  ...severities.map((meta) => ({
    label: meta.label,
    value: String(highestCounts.value[meta.key]),
    color: palette.primary,
  })),
])

const pageReport = computed(() => {
  if (total.value === 0) return '0 of 0'
  const start = first.value + 1
  const end = Math.min(first.value + rows.value, total.value)
  return `${start}–${end} of ${total.value.toLocaleString()}`
})

// Context menu state. The menu acts on the check of the right-clicked row.
const menu = ref<InstanceType<typeof ContextMenu>>()
const menuCheck = ref<CheckSummary | null>(null)

const menuItems = computed(() => [
  {
    label: 'Open in Issues',
    icon: List,
    command: () => menuCheck.value && openIssues(menuCheck.value),
  },
  {
    label: 'Open in Issues (highest only)',
    icon: List,
    command: () =>
      menuCheck.value &&
      openIssues(menuCheck.value, highestSeverity(menuCheck.value.severity)),
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
    const n = Number(single(key))
    return Number.isFinite(n) && n > 0 ? n : null
  }

  filters.value = {
    name: { value: single('name'), matchMode: 'contains' },
    category: { value: list('category').length ? list('category') : null, matchMode: 'in' },
    highest: { value: list('highest').length ? list('highest') : null, matchMode: 'in' },
    fatal: { value: null, matchMode: 'contains' },
    error: { value: null, matchMode: 'contains' },
    warning: { value: null, matchMode: 'contains' },
    notice: { value: null, matchMode: 'contains' },
    success: { value: null, matchMode: 'contains' },
  }
  fatalMin.value = number('fatal_min')
  errorMin.value = number('error_min')
  warningMin.value = number('warning_min')
  noticeMin.value = number('notice_min')
  successMin.value = number('success_min')
  sortField.value = typeof q.sort === 'string' ? q.sort : undefined
  sortOrder.value = q.order === 'asc' ? 1 : q.order === 'desc' ? -1 : undefined
  const page = Number(q.page)
  first.value = Number.isFinite(page) && page > 1 ? (page - 1) * rows.value : 0
}

function serializeQuery(): Record<string, string> {
  const query: Record<string, string> = {}
  const name = textValue('name')
  if (name) query.name = name
  const cats = selectedValues('category')
  if (cats.length) query.category = cats.join(',')
  const highest = selectedValues('highest')
  if (highest.length) query.highest = highest.join(',')
  if (fatalMin.value) query.fatal_min = String(fatalMin.value)
  if (errorMin.value) query.error_min = String(errorMin.value)
  if (warningMin.value) query.warning_min = String(warningMin.value)
  if (noticeMin.value) query.notice_min = String(noticeMin.value)
  if (successMin.value) query.success_min = String(successMin.value)
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
    'name',
    'category',
    'highest',
    'fatal_min',
    'error_min',
    'warning_min',
    'notice_min',
    'success_min',
    'sort',
    'order',
    'page',
  ],
  apply: applyQuery,
  load,
  serialize: serializeQuery,
})

async function load(): Promise<void> {
  const id = props.auditId
  if (!id) return
  const request = ++requestId
  loading.value = true
  error.value = null
  try {
    const data = await api.listAuditChecks(id, {
      page: Math.floor(first.value / rows.value) + 1,
      limit: rows.value,
      sort: sortField.value,
      order: currentOrder(),
      name: textValue('name'),
      categories: selectedValues('category'),
      highest: selectedValues('highest'),
      fatalMin: fatalMin.value ?? undefined,
      errorMin: errorMin.value ?? undefined,
      warningMin: warningMin.value ?? undefined,
      noticeMin: noticeMin.value ?? undefined,
      successMin: successMin.value ?? undefined,
    })
    if (request !== requestId) return
    items.value = data.items
    total.value = data.total
    highestCounts.value = data.highest_counts
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

type CountField = 'fatalMin' | 'errorMin' | 'warningMin' | 'noticeMin' | 'successMin'

function parseCount(raw: unknown): number | null {
  if (typeof raw === 'number') return Number.isFinite(raw) && raw > 0 ? raw : null
  if (typeof raw === 'string' && raw.trim() !== '') {
    const n = Number(raw)
    return Number.isFinite(n) && n > 0 ? n : null
  }
  return null
}

function onCountInput(field: CountField, event: { value?: unknown }): void {
  const value = parseCount(event?.value)
  switch (field) {
    case 'fatalMin':
      fatalMin.value = value
      break
    case 'errorMin':
      errorMin.value = value
      break
    case 'warningMin':
      warningMin.value = value
      break
    case 'noticeMin':
      noticeMin.value = value
      break
    case 'successMin':
      successMin.value = value
      break
  }
  first.value = 0
  if (filterTimer) clearTimeout(filterTimer)
  filterTimer = setTimeout(sync, 250)
}

function resetFilters(): void {
  filters.value = defaultFilters()
  fatalMin.value = null
  errorMin.value = null
  warningMin.value = null
  noticeMin.value = null
  successMin.value = null
  sortField.value = undefined
  sortOrder.value = undefined
  first.value = 0
  sync()
}

function openIssues(check: CheckSummary, highest?: string): void {
  const query: Record<string, string> = { check: check.name }
  if (highest) query.severity = highest
  router.push({ name: 'issues', params: { auditId: props.auditId }, query })
}

function onRowClick(event: DataTableRowClickEvent): void {
  openIssues(event.data as CheckSummary)
}

function onRowContextMenu(event: DataTableRowContextMenuEvent): void {
  event.originalEvent.preventDefault()
  menuCheck.value = event.data as CheckSummary
  menu.value?.show(event.originalEvent as MouseEvent)
}

// Reload when audit data changes elsewhere (a run, a recheck, a delete).
watch(dataVersion, () => {
  load()
})

// Only the page size persists in local storage; filters/sort/page live in the
// URL.
watch(rows, () => {
  saveJSON(SETTINGS_KEY, { version: SETTINGS_VERSION, rows: rows.value })
})

onBeforeUnmount(() => {
  if (filterTimer) clearTimeout(filterTimer)
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-4">
    <Message v-if="error" severity="error">{{ error }}</Message>

    <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-6">
      <MetricTile
        v-for="tile in widgetTiles"
        :key="tile.label"
        :label="tile.label"
        :value="tile.value"
        :color="tile.color"
      />
    </div>

    <div class="min-h-0 flex-1">
      <DataTable
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
        :row-class="() => 'cursor-pointer'"
        scrollable
        scroll-height="flex"
        data-key="name"
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
              label="Refresh"
              text
              size="small"
              aria-label="Refresh checks"
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

        <Column
          field="name"
          header="Check"
          sortable
          filter-match-mode="contains"
          :filter-menu-style="{ minWidth: '16rem' }"
          style="min-width: 15rem"
        >
          <template #body="{ data }">
            <span class="text-sm font-medium text-slate-700">
              {{ data.label || data.name }}
            </span>
          </template>
          <template #filter="{ filterModel, filterCallback }">
            <InputText
              v-model="filterModel.value"
              type="text"
              placeholder="Search check"
              class="w-full"
              @input="filterCallback()"
            />
          </template>
        </Column>

        <Column header="Description" style="min-width: 22rem">
          <template #body="{ data }">
            <span class="text-sm text-slate-600">{{ data.description }}</span>
          </template>
        </Column>

        <Column
          field="category"
          header="Category"
          sortable
          filter-match-mode="in"
          :show-filter-match-modes="false"
          :filter-menu-style="{ minWidth: '16rem' }"
          style="min-width: 9rem"
        >
          <template #body="{ data }">
            <span class="text-sm text-slate-600">
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
          field="highest"
          header="Highest"
          sortable
          filter-match-mode="in"
          :show-filter-match-modes="false"
          :filter-menu-style="{ minWidth: '14rem' }"
          style="min-width: 8rem"
        >
          <template #body="{ data }">
            <SeverityTag
              v-if="highestSeverity(data.severity)"
              :severity="highestSeverity(data.severity)"
            />
            <span v-else class="text-slate-400">–</span>
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
          v-for="sev in severities"
          :key="sev.key"
          :field="sev.key"
          :header="sev.label"
          sortable
          :show-filter-match-modes="false"
          :show-apply-button="false"
          :show-clear-button="false"
          :filter-menu-style="{ minWidth: '10rem' }"
          style="min-width: 7rem"
        >
          <template #body="{ data }">
            <span class="tabular-nums text-slate-600">
              {{ data.severity[sev.key] }}
            </span>
          </template>
          <template #filter>
            <div class="flex flex-col gap-1">
              <span class="text-xs text-slate-400">Minimum</span>
              <InputNumber
                :model-value="
                  sev.key === 'fatal'
                    ? fatalMin
                    : sev.key === 'error'
                      ? errorMin
                      : sev.key === 'warning'
                        ? warningMin
                        : sev.key === 'notice'
                          ? noticeMin
                          : successMin
                "
                placeholder="≥"
                :min="0"
                :use-grouping="false"
                fluid
                @input="
                  onCountInput(
                    sev.key === 'fatal'
                      ? 'fatalMin'
                      : sev.key === 'error'
                        ? 'errorMin'
                        : sev.key === 'warning'
                          ? 'warningMin'
                          : sev.key === 'notice'
                            ? 'noticeMin'
                            : 'successMin',
                    $event,
                  )
                "
              />
            </div>
          </template>
        </Column>
      </DataTable>
    </div>

    <ContextMenu ref="menu" :model="menuItems" @hide="menuCheck = null" />
  </div>
</template>
