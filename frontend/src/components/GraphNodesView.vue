<script setup lang="ts">
import { ExternalLink, FilterSlash, Map, Refresh, Sitemap } from '@primeicons/vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import ContextMenu from 'primevue/contextmenu'
import DataTable, {
  type DataTableFilterEvent,
  type DataTableFilterMeta,
  type DataTablePageEvent,
  type DataTableRowContextMenuEvent,
  type DataTableSortEvent,
} from 'primevue/datatable'
import DatePicker from 'primevue/datepicker'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import Select from 'primevue/select'
import { computed, ref, watch } from 'vue'
import { useRouter, type LocationQuery } from 'vue-router'
import * as api from '../api/client'
import { dataVersion } from '../lib/appState'
import { useDelayedLoading } from '../lib/loading'
import { filetypeClass } from '../lib/graph'
import { useTableQuery } from '../lib/tableQuery'
import type { FiletypeCount, GraphNode } from '../types'
import EmptyState from './EmptyState.vue'

const props = defineProps<{ auditId: string }>()

const router = useRouter()
const menu = ref<InstanceType<typeof ContextMenu>>()
const menuNode = ref<GraphNode | null>(null)

const menuItems = computed(() => [
  {
    label: 'Open in navigator',
    icon: Sitemap,
    command: () => menuNode.value && openNavigator(menuNode.value.id),
  },
  {
    label: 'Focus in visualization',
    icon: Map,
    command: () => menuNode.value && openMap(menuNode.value.id),
  },
  {
    label: 'Open link',
    icon: ExternalLink,
    command: () => menuNode.value && window.open(menuNode.value.url, '_blank', 'noopener,noreferrer'),
  },
])

const items = ref<GraphNode[]>([])
const total = ref(0)
const filetypes = ref<FiletypeCount[]>([])
const loading = ref(false)
const showTableLoading = useDelayedLoading(loading)
const error = ref<string | null>(null)

const rows = ref(50)
const pageSizeOptions = [25, 50, 100, 200]
const first = ref(0)
const sortField = ref<string | undefined>(undefined)
const sortOrder = ref<1 | 0 | -1 | undefined>(undefined)

// Seen-range bounds live outside the DataTable filter model so the range
// survives its filter cloning, like the numeric ranges in the URLs view.
const firstSeenRange = ref<Date[] | null>(null)
const lastSeenRange = ref<Date[] | null>(null)
const lastValidatedRange = ref<Date[] | null>(null)
const statusMin = ref<number | null>(null)
const statusMax = ref<number | null>(null)

function formatDateTime(iso: string): string {
  if (!iso || iso.startsWith('0001-01-01')) return '–'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '–'
  return date.toLocaleString(undefined, {
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function formatStatus(code: number): string {
  return code > 0 ? String(code) : '–'
}

function statusClass(code: number): string {
  if (code <= 0) return 'text-slate-400'
  if (code < 300) return 'text-emerald-600'
  if (code < 400) return 'text-sky-600'
  if (code < 500) return 'text-amber-600'
  return 'text-red-600'
}

function rangeFrom(range: Date[] | null): string {
  return range && range[0] ? range[0].toISOString() : ''
}

function rangeTo(range: Date[] | null): string {
  const end = range && range[1] ? range[1] : range && range[0] ? range[0] : null
  if (!end) return ''
  const e = new Date(end)
  e.setHours(23, 59, 59, 999)
  return e.toISOString()
}

function parseRange(from: string | null, to: string | null): Date[] | null {
  const f = from ? new Date(from) : null
  const t = to ? new Date(to) : null
  if (!f && !t) return null
  const start = f ?? (t as Date)
  const end = t ?? (f as Date)
  return [start, end]
}

type MatchMode = 'in' | 'contains'
interface ColumnFilter {
  value: unknown
  matchMode: MatchMode
}

function defaultFilters(): Record<string, ColumnFilter> {
  return {
    url: { value: null, matchMode: 'contains' },
    filetype: { value: null, matchMode: 'in' },
    external: { value: null, matchMode: 'in' },
    // The seen/validated ranges and the status range are filtered by their
    // dedicated refs below, but the DataTable needs a filter-model entry per
    // column to render its menu.
    first_seen: { value: null, matchMode: 'contains' },
    last_seen: { value: null, matchMode: 'contains' },
    status_code: { value: null, matchMode: 'contains' },
    last_validated: { value: null, matchMode: 'contains' },
  }
}

const filters = ref<DataTableFilterMeta>(defaultFilters())

const externalOptions = [
  { label: 'Internal', value: 'internal' },
  { label: 'External', value: 'external' },
]

const filetypeOptions = computed(() =>
  filetypes.value.map((f) => ({
    label: `${f.filetype} (${f.count.toLocaleString()})`,
    value: f.filetype,
  })),
)

const pageReport = computed(() => {
  if (total.value === 0) return '0 of 0'
  const start = first.value + 1
  const end = Math.min(first.value + rows.value, total.value)
  return `${start}–${end} of ${total.value.toLocaleString()}`
})

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

function externalParam(): boolean | undefined {
  const values = selectedValues('external')
  if (values.length !== 1) return undefined
  return values[0] === 'external'
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
    url: { value: single('url'), matchMode: 'contains' },
    filetype: { value: list('filetype').length ? list('filetype') : null, matchMode: 'in' },
    external: { value: list('external').length ? list('external') : null, matchMode: 'in' },
    first_seen: { value: null, matchMode: 'contains' },
    last_seen: { value: null, matchMode: 'contains' },
    status_code: { value: null, matchMode: 'contains' },
    last_validated: { value: null, matchMode: 'contains' },
  }
  const num = (key: string): number | null => {
    const value = single(key)
    if (value === null) return null
    const n = Number(value)
    return Number.isFinite(n) ? n : null
  }
  firstSeenRange.value = parseRange(single('first_seen_min'), single('first_seen_max'))
  lastSeenRange.value = parseRange(single('last_seen_min'), single('last_seen_max'))
  lastValidatedRange.value = parseRange(
    single('last_validated_min'),
    single('last_validated_max'),
  )
  statusMin.value = num('status_min')
  statusMax.value = num('status_max')
  sortField.value = single('sort') ?? undefined
  sortOrder.value = q.order === 'asc' ? 1 : q.order === 'desc' ? -1 : undefined
  const page = Number(q.page)
  first.value = Number.isFinite(page) && page > 1 ? (page - 1) * rows.value : 0
}

function serializeQuery(): Record<string, string> {
  const query: Record<string, string> = {}
  const url = textValue('url')
  if (url) query.url = url
  const filetype = selectedValues('filetype')
  if (filetype.length) query.filetype = filetype.join(',')
  const external = selectedValues('external')
  if (external.length) query.external = external.join(',')
  const firstFrom = rangeFrom(firstSeenRange.value)
  if (firstFrom) query.first_seen_min = firstFrom
  const firstTo = rangeTo(firstSeenRange.value)
  if (firstTo) query.first_seen_max = firstTo
  const lastFrom = rangeFrom(lastSeenRange.value)
  if (lastFrom) query.last_seen_min = lastFrom
  const lastTo = rangeTo(lastSeenRange.value)
  if (lastTo) query.last_seen_max = lastTo
  if (statusMin.value !== null) query.status_min = String(statusMin.value)
  if (statusMax.value !== null) query.status_max = String(statusMax.value)
  const validatedFrom = rangeFrom(lastValidatedRange.value)
  if (validatedFrom) query.last_validated_min = validatedFrom
  const validatedTo = rangeTo(lastValidatedRange.value)
  if (validatedTo) query.last_validated_max = validatedTo
  if (sortField.value) query.sort = sortField.value
  if (sortOrder.value === 1) query.order = 'asc'
  else if (sortOrder.value === -1) query.order = 'desc'
  if (first.value > 0) query.page = String(Math.floor(first.value / rows.value) + 1)
  return query
}

async function load(): Promise<void> {
  const id = props.auditId
  if (!id) return
  const request = ++requestId
  loading.value = true
  error.value = null
  try {
    const [summary, data] = await Promise.all([
      api.getGraphSummary(id),
      api.listGraphNodes(id, {
        page: Math.floor(first.value / rows.value) + 1,
        limit: rows.value,
        sort: sortField.value,
        order: currentOrder(),
        filetypes: selectedValues('filetype'),
        external: externalParam(),
        url: textValue('url'),
        firstSeenFrom: rangeFrom(firstSeenRange.value) || undefined,
        firstSeenTo: rangeTo(firstSeenRange.value) || undefined,
        lastSeenFrom: rangeFrom(lastSeenRange.value) || undefined,
        lastSeenTo: rangeTo(lastSeenRange.value) || undefined,
        statusMin: statusMin.value ?? undefined,
        statusMax: statusMax.value ?? undefined,
        lastValidatedFrom: rangeFrom(lastValidatedRange.value) || undefined,
        lastValidatedTo: rangeTo(lastValidatedRange.value) || undefined,
      }),
    ])
    if (request !== requestId) return
    filetypes.value = summary.filetypes
    items.value = data.items
    total.value = data.total
  } catch (e) {
    if (request === requestId) error.value = e instanceof Error ? e.message : String(e)
  } finally {
    if (request === requestId) loading.value = false
  }
}

const { sync } = useTableQuery({
  keys: [
    'url',
    'filetype',
    'external',
    'first_seen_min',
    'first_seen_max',
    'last_seen_min',
    'last_seen_max',
    'status_min',
    'status_max',
    'last_validated_min',
    'last_validated_max',
    'sort',
    'order',
    'page',
  ],
  apply: applyQuery,
  load,
  serialize: serializeQuery,
})

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

function onDateFilter(): void {
  first.value = 0
  if (filterTimer) clearTimeout(filterTimer)
  filterTimer = setTimeout(sync, 250)
}

function onStatusInput(field: 'statusMin' | 'statusMax', event: { value?: unknown }): void {
  const raw = event?.value
  const value =
    typeof raw === 'number' ? raw : typeof raw === 'string' && raw !== '' ? Number(raw) : null
  const parsed = value !== null && Number.isFinite(value) ? value : null
  if (field === 'statusMin') statusMin.value = parsed
  else statusMax.value = parsed
  onDateFilter()
}

function resetFilters(): void {
  if (filterTimer) clearTimeout(filterTimer)
  filters.value = defaultFilters()
  firstSeenRange.value = null
  lastSeenRange.value = null
  lastValidatedRange.value = null
  statusMin.value = null
  statusMax.value = null
  sortField.value = undefined
  sortOrder.value = undefined
  first.value = 0
  sync()
}

function shortUrl(url: string): string {
  return url.replace(/^https?:\/\//, '')
}

function openNavigator(nodeId: string): void {
  router.push({
    name: 'graph-navigator-node',
    params: { auditId: props.auditId, nodeId },
  })
}

function openMap(nodeId: string): void {
  router.push({
    name: 'graph-map',
    params: { auditId: props.auditId },
    query: { focus: nodeId },
  })
}

function onRowContextMenu(event: DataTableRowContextMenuEvent): void {
  event.originalEvent.preventDefault()
  menuNode.value = event.data as GraphNode
  menu.value?.show(event.originalEvent as MouseEvent)
}

watch(dataVersion, () => {
  load()
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-4">
    <Message v-if="error" severity="error">{{ error }}</Message>

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
      scrollable
      scroll-height="flex"
      data-key="id"
      paginator-template="FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink"
      @page="onPage"
      @sort="onSort"
      @filter="onFilter"
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
            aria-label="Refresh nodes"
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
        field="url"
        header="URL"
        sortable
        filter-match-mode="contains"
        :filter-menu-style="{ minWidth: '16rem' }"
        style="min-width: 24rem"
      >
        <template #body="{ data }">
          <div class="flex items-center gap-2">
            <a
              :href="data.url"
              target="_blank"
              rel="noopener noreferrer"
              class="truncate text-[var(--p-primary-color)] hover:underline"
              :title="data.url"
            >
              {{ shortUrl(data.url) }}
            </a>
            <span
              v-if="data.is_root"
              class="shrink-0 rounded bg-[var(--p-primary-color)] px-1.5 py-0.5 text-[10px] font-semibold text-[var(--p-primary-contrast-color)] uppercase"
            >
              root
            </span>
          </div>
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
        field="filetype"
        header="Filetype"
        sortable
        filter-match-mode="in"
        :show-filter-match-modes="false"
        :filter-menu-style="{ minWidth: '14rem' }"
        style="min-width: 8rem"
      >
        <template #body="{ data }">
          <span
            class="inline-block rounded px-2 py-0.5 text-xs font-medium"
            :class="filetypeClass(data.filetype)"
          >
            {{ data.filetype }}
          </span>
        </template>
        <template #filter="{ filterModel, filterCallback }">
          <MultiSelect
            v-model="filterModel.value"
            :options="filetypeOptions"
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
        field="external"
        header="Scope"
        sortable
        filter-match-mode="in"
        :show-filter-match-modes="false"
        :filter-menu-style="{ minWidth: '12rem' }"
        style="min-width: 7rem"
      >
        <template #body="{ data }">
          <span
            class="rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase"
            :class="data.external ? 'bg-slate-200 text-slate-600' : 'bg-emerald-100 text-emerald-700'"
          >
            {{ data.external ? 'external' : 'internal' }}
          </span>
        </template>
        <template #filter="{ filterModel, filterCallback }">
          <MultiSelect
            v-model="filterModel.value"
            :options="externalOptions"
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
        field="first_seen"
        header="First seen"
        sortable
        :show-filter-match-modes="false"
        :show-apply-button="false"
        :show-clear-button="false"
        :filter-menu-style="{ minWidth: '17rem' }"
        style="min-width: 10rem"
      >
        <template #body="{ data }">
          <span class="tabular-nums text-slate-600">
            {{ formatDateTime(data.first_seen) }}
          </span>
        </template>
        <template #filter>
          <DatePicker
            v-model="firstSeenRange"
            selection-mode="range"
            :manual-input="false"
            show-button-bar
            placeholder="Any date"
            class="w-full"
            @update:model-value="onDateFilter"
          />
        </template>
      </Column>

      <Column
        field="last_seen"
        header="Last seen"
        sortable
        :show-filter-match-modes="false"
        :show-apply-button="false"
        :show-clear-button="false"
        :filter-menu-style="{ minWidth: '17rem' }"
        style="min-width: 10rem"
      >
        <template #body="{ data }">
          <span class="tabular-nums text-slate-600">
            {{ formatDateTime(data.last_seen) }}
          </span>
        </template>
        <template #filter>
          <DatePicker
            v-model="lastSeenRange"
            selection-mode="range"
            :manual-input="false"
            show-button-bar
            placeholder="Any date"
            class="w-full"
            @update:model-value="onDateFilter"
          />
        </template>
      </Column>

      <Column
        field="status_code"
        header="Status"
        sortable
        :show-filter-match-modes="false"
        :show-apply-button="false"
        :show-clear-button="false"
        :filter-menu-style="{ minWidth: '12rem' }"
        style="min-width: 6rem"
      >
        <template #body="{ data }">
          <span class="font-medium tabular-nums" :class="statusClass(data.status_code)">
            {{ formatStatus(data.status_code) }}
          </span>
        </template>
        <template #filter>
          <div class="grid grid-cols-1 gap-2">
            <InputNumber
              :model-value="statusMin"
              placeholder="Min"
              :use-grouping="false"
              class="w-24"
              @input="onStatusInput('statusMin', $event)"
            />
            <InputNumber
              :model-value="statusMax"
              placeholder="Max"
              :use-grouping="false"
              class="w-24"
              @input="onStatusInput('statusMax', $event)"
            />
          </div>
        </template>
      </Column>

      <Column
        field="last_validated"
        header="Last validated"
        sortable
        :show-filter-match-modes="false"
        :show-apply-button="false"
        :show-clear-button="false"
        :filter-menu-style="{ minWidth: '17rem' }"
        style="min-width: 10rem"
      >
        <template #body="{ data }">
          <span class="tabular-nums text-slate-600">
            {{ formatDateTime(data.last_validated) }}
          </span>
        </template>
        <template #filter>
          <DatePicker
            v-model="lastValidatedRange"
            selection-mode="range"
            :manual-input="false"
            show-button-bar
            placeholder="Any date"
            class="w-full"
            @update:model-value="onDateFilter"
          />
        </template>
      </Column>

      <Column field="in_links" header="In" sortable style="min-width: 5rem">
        <template #body="{ data }">
          <span class="tabular-nums">{{ data.in_links }}</span>
        </template>
      </Column>
      <Column field="out_links" header="Out" sortable style="min-width: 5rem">
        <template #body="{ data }">
          <span class="tabular-nums">{{ data.out_links }}</span>
        </template>
      </Column>
    </DataTable>

    <ContextMenu ref="menu" :model="menuItems" @hide="menuNode = null" />
  </div>
</template>
