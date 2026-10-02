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
import { dataVersion } from '../lib/appState'
import { filetypeClass } from '../lib/graph'
import { useDelayedLoading } from '../lib/loading'
import { useTableQuery } from '../lib/tableQuery'
import type { GraphEdgeRow } from '../types'
import EmptyState from './EmptyState.vue'

const props = defineProps<{ auditId: string }>()

const router = useRouter()
const menu = ref<InstanceType<typeof ContextMenu>>()
const menuEdge = ref<GraphEdgeRow | null>(null)

const menuItems = computed(() => [
  {
    label: 'Open source in navigator',
    icon: Sitemap,
    command: () => menuEdge.value && openNavigator(menuEdge.value.source_node_id),
  },
  {
    label: 'Open target in navigator',
    icon: Sitemap,
    command: () => menuEdge.value && openNavigator(menuEdge.value.target_node_id),
  },
  {
    label: 'Focus source in visualization',
    icon: Map,
    command: () => menuEdge.value && openMap(menuEdge.value.source_node_id),
  },
  {
    label: 'Focus target in visualization',
    icon: Map,
    command: () => menuEdge.value && openMap(menuEdge.value.target_node_id),
  },
  { separator: true },
  {
    label: 'Open source link',
    icon: ExternalLink,
    command: () => menuEdge.value && window.open(menuEdge.value.source_url, '_blank', 'noopener,noreferrer'),
  },
  {
    label: 'Open target link',
    icon: ExternalLink,
    command: () => menuEdge.value && window.open(menuEdge.value.target_url, '_blank', 'noopener,noreferrer'),
  },
])

const items = ref<GraphEdgeRow[]>([])
const total = ref(0)
const loading = ref(false)
const showTableLoading = useDelayedLoading(loading)
const error = ref<string | null>(null)

const rows = ref(50)
const pageSizeOptions = [25, 50, 100, 200]
const first = ref(0)
const sortField = ref<string | undefined>(undefined)
const sortOrder = ref<1 | 0 | -1 | undefined>(undefined)

type MatchMode = 'in' | 'contains'
interface ColumnFilter {
  value: unknown
  matchMode: MatchMode
}

function defaultFilters(): Record<string, ColumnFilter> {
  return {
    source: { value: null, matchMode: 'contains' },
    type: { value: null, matchMode: 'in' },
    target: { value: null, matchMode: 'contains' },
  }
}

const filters = ref<DataTableFilterMeta>(defaultFilters())

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
    source: { value: single('source'), matchMode: 'contains' },
    type: { value: list('type').length ? list('type') : null, matchMode: 'in' },
    target: { value: single('target'), matchMode: 'contains' },
  }
  sortField.value = single('sort') ?? undefined
  sortOrder.value = q.order === 'asc' ? 1 : q.order === 'desc' ? -1 : undefined
  const page = Number(q.page)
  first.value = Number.isFinite(page) && page > 1 ? (page - 1) * rows.value : 0
}

function serializeQuery(): Record<string, string> {
  const query: Record<string, string> = {}
  const source = textValue('source')
  if (source) query.source = source
  const types = selectedValues('type')
  if (types.length) query.type = types.join(',')
  const target = textValue('target')
  if (target) query.target = target
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
    const data = await api.listGraphEdges(id, {
      page: Math.floor(first.value / rows.value) + 1,
      limit: rows.value,
      sort: sortField.value,
      order: currentOrder(),
      types: selectedValues('type'),
      source: textValue('source'),
      target: textValue('target'),
    })
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
  keys: ['source', 'type', 'target', 'sort', 'order', 'page'],
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
  menuEdge.value = event.data as GraphEdgeRow
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
            aria-label="Refresh edges"
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
        field="source"
        header="Source"
        sortable
        filter-match-mode="contains"
        :filter-menu-style="{ minWidth: '16rem' }"
        style="min-width: 20rem"
      >
        <template #body="{ data }">
          <span class="flex items-center gap-2">
            <span
              class="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium"
              :class="filetypeClass(data.source_filetype)"
            >
              {{ data.source_filetype }}
            </span>
            <a
              :href="data.source_url"
              target="_blank"
              rel="noopener noreferrer"
              class="truncate text-[var(--p-primary-color)] hover:underline"
              :title="data.source_url"
            >
              {{ shortUrl(data.source_url) }}
            </a>
          </span>
        </template>
        <template #filter="{ filterModel, filterCallback }">
          <InputText
            v-model="filterModel.value"
            type="text"
            placeholder="Search source"
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
        :filter-menu-style="{ minWidth: '14rem' }"
        style="min-width: 8rem"
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

      <Column
        field="target"
        header="Target"
        sortable
        filter-match-mode="contains"
        :filter-menu-style="{ minWidth: '16rem' }"
        style="min-width: 20rem"
      >
        <template #body="{ data }">
          <span class="flex items-center gap-2">
            <span
              v-if="data.target_external"
              class="shrink-0 rounded bg-slate-200 px-1.5 py-0.5 text-[10px] font-semibold text-slate-600 uppercase"
            >
              ext
            </span>
            <span
              class="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium"
              :class="filetypeClass(data.target_filetype)"
            >
              {{ data.target_filetype }}
            </span>
            <a
              :href="data.target_url"
              target="_blank"
              rel="noopener noreferrer"
              class="truncate text-[var(--p-primary-color)] hover:underline"
              :title="data.target_url"
            >
              {{ shortUrl(data.target_url) }}
            </a>
          </span>
        </template>
        <template #filter="{ filterModel, filterCallback }">
          <InputText
            v-model="filterModel.value"
            type="text"
            placeholder="Search target"
            class="w-full"
            @input="filterCallback()"
          />
        </template>
      </Column>
    </DataTable>

    <ContextMenu ref="menu" :model="menuItems" @hide="menuEdge = null" />
  </div>
</template>
