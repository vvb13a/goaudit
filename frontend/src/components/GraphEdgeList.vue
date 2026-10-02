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
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import Select from 'primevue/select'
import { computed, ref, watch } from 'vue'
import { useRouter, type LocationQuery } from 'vue-router'
import * as api from '../api/client'
import { filetypeClass } from '../lib/graph'
import { useDelayedLoading } from '../lib/loading'
import { useTableQuery } from '../lib/tableQuery'
import type { GraphEdgeQuery, GraphEdgeRow } from '../types'
import EmptyState from './EmptyState.vue'

const props = defineProps<{
  auditId: string
  centerId: string
  direction: 'in' | 'out'
  prefix: string
}>()

const emit = defineEmits<{ navigate: [nodeId: string] }>()

const router = useRouter()

const menu = ref<InstanceType<typeof ContextMenu>>()
const menuEdge = ref<GraphEdgeRow | null>(null)

const menuItems = computed(() => [
  {
    label: 'Set as center node',
    icon: Sitemap,
    command: () => menuEdge.value && emit('navigate', endpoint(menuEdge.value).id),
  },
  {
    label: 'Focus in visualization',
    icon: Map,
    command: () => menuEdge.value && openMap(endpoint(menuEdge.value).id),
  },
  {
    label: 'Open link',
    icon: ExternalLink,
    command: () =>
      menuEdge.value &&
      window.open(endpoint(menuEdge.value).url, '_blank', 'noopener,noreferrer'),
  },
])

const items = ref<GraphEdgeRow[]>([])
const total = ref(0)
const loading = ref(false)
const showTableLoading = useDelayedLoading(loading)
const error = ref<string | null>(null)

const rows = ref(25)
const pageSizeOptions = [10, 25, 50, 100]
const first = ref(0)
const sortField = ref<string | undefined>(undefined)
const sortOrder = ref<1 | 0 | -1 | undefined>(undefined)

const edgeTypeOptions = [
  'hyperlink',
  'stylesheet',
  'script',
  'image',
  'source',
  'iframe',
  'video',
  'audio',
  'icon',
  'manifest',
  'preload',
].map((t) => ({ label: t, value: t }))

type MatchMode = 'in' | 'contains'
interface ColumnFilter {
  value: unknown
  matchMode: MatchMode
}

function defaultFilters(): Record<string, ColumnFilter> {
  return {
    node: { value: null, matchMode: 'contains' },
    type: { value: null, matchMode: 'in' },
  }
}

const filters = ref<DataTableFilterMeta>(defaultFilters())

const keys = [
  `${props.prefix}_node`,
  `${props.prefix}_type`,
  `${props.prefix}_sort`,
  `${props.prefix}_order`,
  `${props.prefix}_page`,
]

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

function apiSort(): string | undefined {
  if (sortField.value === 'node') return props.direction === 'out' ? 'target' : 'source'
  return sortField.value
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
    node: { value: single(`${props.prefix}_node`), matchMode: 'contains' },
    type: {
      value: list(`${props.prefix}_type`).length ? list(`${props.prefix}_type`) : null,
      matchMode: 'in',
    },
  }
  const sort = single(`${props.prefix}_sort`)
  sortField.value = sort === 'node' || sort === 'type' ? sort : undefined
  const order = single(`${props.prefix}_order`)
  sortOrder.value = order === 'asc' ? 1 : order === 'desc' ? -1 : undefined
  const page = Number(single(`${props.prefix}_page`))
  first.value = Number.isFinite(page) && page > 1 ? (page - 1) * rows.value : 0
}

function serializeQuery(): Record<string, string> {
  const query: Record<string, string> = {}
  const node = textValue('node')
  if (node) query[`${props.prefix}_node`] = node
  const types = selectedValues('type')
  if (types.length) query[`${props.prefix}_type`] = types.join(',')
  if (sortField.value) query[`${props.prefix}_sort`] = sortField.value
  if (sortOrder.value === 1) query[`${props.prefix}_order`] = 'asc'
  else if (sortOrder.value === -1) query[`${props.prefix}_order`] = 'desc'
  if (first.value > 0) {
    query[`${props.prefix}_page`] = String(Math.floor(first.value / rows.value) + 1)
  }
  return query
}

async function load(): Promise<void> {
  const id = props.auditId
  if (!id) return
  const request = ++requestId
  loading.value = true
  error.value = null

  const query: GraphEdgeQuery = {
    page: Math.floor(first.value / rows.value) + 1,
    limit: rows.value,
    sort: apiSort(),
    order: currentOrder(),
    types: selectedValues('type'),
  }
  const node = textValue('node')
  if (props.direction === 'out') {
    query.sourceId = props.centerId
    if (node) query.target = node
  } else {
    query.targetId = props.centerId
    if (node) query.source = node
  }

  try {
    const data = await api.listGraphEdges(id, query)
    if (request !== requestId) return
    items.value = data.items
    total.value = data.total
  } catch (e) {
    if (request === requestId) error.value = e instanceof Error ? e.message : String(e)
  } finally {
    if (request === requestId) loading.value = false
  }
}

const { sync } = useTableQuery({
  keys,
  apply: applyQuery,
  load,
  serialize: serializeQuery,
})

const pageReport = computed(() => {
  if (total.value === 0) return '0 of 0'
  const start = first.value + 1
  const end = Math.min(first.value + rows.value, total.value)
  return `${start}–${end} of ${total.value.toLocaleString()}`
})

// The node on the far side of the edge: the target of an outbound edge, the
// source of an inbound one.
interface Endpoint {
  id: string
  url: string
  filetype: string
  external: boolean
}

function endpoint(row: GraphEdgeRow): Endpoint {
  if (props.direction === 'out') {
    return {
      id: row.target_node_id,
      url: row.target_url,
      filetype: row.target_filetype,
      external: row.target_external,
    }
  }
  return {
    id: row.source_node_id,
    url: row.source_url,
    filetype: row.source_filetype,
    external: false,
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

function resetFilters(): void {
  if (filterTimer) clearTimeout(filterTimer)
  filters.value = defaultFilters()
  sortField.value = undefined
  sortOrder.value = undefined
  first.value = 0
  sync()
}

function shortUrl(url: string): string {
  return url.replace(/^https?:\/\//, '')
}

function rowClass(): string {
  return 'cursor-pointer'
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
  menuEdge.value = event.data as GraphEdgeRow
  menu.value?.show(event.originalEvent as MouseEvent)
}

// The list belongs to the center node; when it changes, its edges change too.
watch(
  () => props.centerId,
  () => {
    first.value = 0
    load()
  },
)
</script>

<template>
  <div class="flex min-h-0 flex-col">
    <Message v-if="error" severity="error" class="mb-2">{{ error }}</Message>

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
      :row-class="rowClass"
      scrollable
      scroll-height="flex"
      data-key="id"
      paginator-template="FirstPageLink PrevPageLink NextPageLink LastPageLink"
      @page="onPage"
      @sort="onSort"
      @filter="onFilter"
      @row-click="emit('navigate', endpoint($event.data).id)"
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
            :loading="loading"
            @click="load"
          >
            <template #icon><Refresh :size="16" /></template>
          </Button>
          <span class="mx-1 h-4 w-px bg-slate-200" aria-hidden="true" />
          <Select
            :model-value="rows"
            :options="pageSizeOptions"
            size="small"
            class="w-20"
            @update:model-value="onRowsChange"
          />
        </div>
      </template>
      <template #empty>
        <EmptyState v-if="!loading" @reset="resetFilters" />
      </template>

      <Column
        field="node"
        header="Node"
        sortable
        filter-match-mode="contains"
        :filter-menu-style="{ minWidth: '15rem' }"
        style="min-width: 16rem"
      >
        <template #body="{ data }">
          <span class="flex items-center gap-2">
            <span
              v-if="endpoint(data).external"
              class="shrink-0 rounded bg-slate-200 px-1.5 py-0.5 text-[10px] font-semibold text-slate-600 uppercase"
            >
              ext
            </span>
            <span
              class="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium"
              :class="filetypeClass(endpoint(data).filetype)"
            >
              {{ endpoint(data).filetype }}
            </span>
            <span
              class="truncate text-[var(--p-primary-color)]"
              :title="endpoint(data).url"
            >
              {{ shortUrl(endpoint(data).url) }}
            </span>
          </span>
        </template>
        <template #filter="{ filterModel, filterCallback }">
          <InputText
            v-model="filterModel.value"
            type="text"
            placeholder="Search"
            class="w-full"
            @input="filterCallback()"
          />
        </template>
      </Column>

      <Column
        field="type"
        header="Type"
        sortable
        filter-match-mode="in"
        :show-filter-match-modes="false"
        :filter-menu-style="{ minWidth: '13rem' }"
        style="min-width: 7rem"
      >
        <template #body="{ data }">
          <span class="text-sm capitalize">{{ data.type }}</span>
        </template>
        <template #filter="{ filterModel, filterCallback }">
          <MultiSelect
            v-model="filterModel.value"
            :options="edgeTypeOptions"
            option-label="label"
            option-value="value"
            placeholder="Any"
            :show-clear="true"
            class="w-full"
            @change="filterCallback()"
          />
        </template>
      </Column>
    </DataTable>

    <ContextMenu ref="menu" :model="menuItems" @hide="menuEdge = null" />
  </div>
</template>
