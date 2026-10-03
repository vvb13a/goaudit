<script setup lang="ts">
import { Refresh } from '@primeicons/vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Message from 'primevue/message'
import Select from 'primevue/select'
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import * as api from '../api/client'
import { dataVersion } from '../lib/appState'
import { formatDuration } from '../lib/format'
import { useColumnLayout } from '../lib/columnLayout'
import { useDelayedLoading } from '../lib/loading'
import { auditScoreColor, palette } from '../lib/theme'
import type { AuditSnapshot, GraphSnapshot } from '../types'
import BarChart, { type Bar } from './BarChart.vue'
import ColumnToggle from './ColumnToggle.vue'

const props = defineProps<{ auditId: string }>()

const route = useRoute()
const router = useRouter()

// The timeline is scoped to one workflow: the check timeline records runs that
// performed checks, the graph timeline records runs that built the graph.
type Scope = 'checks' | 'graph'

const scope = ref<Scope>(route.query.scope === 'graph' ? 'graph' : 'checks')
const snapshots = ref<AuditSnapshot[]>([])
const graphSnapshots = ref<GraphSnapshot[]>([])
const loading = ref(false)
const showTableLoading = useDelayedLoading(loading)
const error = ref<string | null>(null)
let requestId = 0

const first = ref(0)
const rows = ref(25)
const pageSizeOptions = [10, 25, 50, 100]

const {
  labels: checkColumnLabels,
  columnOrder: checkColumnOrder,
  visibleFields: checkVisibleFields,
  visibleOrderedFields: checkVisibleOrderedFields,
  columnsKey: checkColumnsKey,
  reset: resetCheckColumns,
} = useColumnLayout('timeline-checks', [
  { field: 'time', label: 'Time' },
  { field: 'score', label: 'Score' },
  { field: 'urls', label: 'URLs' },
  { field: 'issues', label: 'Issues' },
  { field: 'criticals', label: 'Criticals' },
  { field: 'duration', label: 'Duration' },
])

const {
  labels: graphColumnLabels,
  columnOrder: graphColumnOrder,
  visibleFields: graphVisibleFields,
  visibleOrderedFields: graphVisibleOrderedFields,
  columnsKey: graphColumnsKey,
  reset: resetGraphColumns,
} = useColumnLayout('timeline-graph', [
  { field: 'time', label: 'Time' },
  { field: 'nodes', label: 'Nodes' },
  { field: 'edges', label: 'Edges' },
  { field: 'internal', label: 'Internal' },
  { field: 'external', label: 'External' },
  { field: 'roots', label: 'Roots' },
  { field: 'duration', label: 'Duration' },
])

function formatTime(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '–'
  return date.toLocaleString(undefined, {
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

// Two lines (date then time) so runs on the same day stay distinguishable;
// BarChart reserves two lines for every label so the bars stay comparable.
function barLabel(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '–'
  const day = date.toLocaleDateString(undefined, {
    month: 'short',
    day: '2-digit',
  })
  const time = date.toLocaleTimeString(undefined, {
    hour: '2-digit',
    minute: '2-digit',
  })
  return `${day}\n${time}`
}

// "total (+new)", omitting the new part when nothing is new.
function totalWithNew(total: number, newCount: number, showNew: boolean): string {
  return showNew && newCount > 0 ? `${total} (+${newCount})` : String(total)
}

interface CheckRow {
  id: string
  time: string
  score: number
  urls: string
  issues: string
  criticals: number
  duration: string
}

interface GraphRow {
  id: string
  time: string
  nodes: number
  edges: number
  internal: number
  external: number
  roots: number
  duration: string
}

const checkRows = computed<CheckRow[]>(() => {
  const total = snapshots.value.length
  // Snapshots are newest first, so the last row is the audit's first run,
  // where everything is new by definition and the (+new) hint is noise.
  return snapshots.value.map((s, i) => ({
    id: s.id,
    time: formatTime(s.created_at),
    score: s.overview.score,
    urls: totalWithNew(s.overview.total_urls, s.url_states.new, i !== total - 1),
    issues: totalWithNew(
      s.overview.total_issues,
      s.lifecycle_counts.new,
      i !== total - 1,
    ),
    criticals: s.severity_counts.fatal + s.severity_counts.error,
    duration:
      s.timings.total_ns > 0 ? formatDuration(s.timings.total_ns / 1e6) : '–',
  }))
})

const graphRows = computed<GraphRow[]>(() =>
  graphSnapshots.value.map((s) => ({
    id: s.id,
    time: formatTime(s.created_at),
    nodes: s.total_nodes,
    edges: s.total_edges,
    internal: s.internal_nodes,
    external: s.external_nodes,
    roots: s.root_nodes,
    duration: s.duration_ns > 0 ? formatDuration(s.duration_ns / 1e6) : '–',
  })),
)

const rowCount = computed(() =>
  scope.value === 'checks' ? snapshots.value.length : graphSnapshots.value.length,
)

const pageReport = computed(() => {
  if (rowCount.value === 0) return '0 of 0'
  const start = first.value + 1
  const end = Math.min(first.value + rows.value, rowCount.value)
  return `${start}–${end} of ${rowCount.value.toLocaleString()}`
})

// The latest five runs, oldest to newest, as compact bar charts.
const recentChecks = computed(() => snapshots.value.slice(0, 5).reverse())
const recentGraph = computed(() => graphSnapshots.value.slice(0, 5).reverse())

const activeCharts = computed<
  { title: string; color: string; format: (value: number) => string; bars: Bar[] }[]
>(() => {
  if (scope.value === 'checks') {
    return [
      {
        title: 'Score',
        color: palette.primary,
        format: (value: number) => value.toFixed(1),
        bars: recentChecks.value.map((s) => ({
          label: barLabel(s.created_at),
          value: s.overview.score,
          color: auditScoreColor(s.overview.score),
        })),
      },
      {
        title: 'URLs',
        color: palette.primary,
        format: (value: number) => String(value),
        bars: recentChecks.value.map((s) => ({
          label: barLabel(s.created_at),
          value: s.overview.total_urls,
        })),
      },
      {
        title: 'Issues',
        color: palette.primary,
        format: (value: number) => String(value),
        bars: recentChecks.value.map((s) => ({
          label: barLabel(s.created_at),
          value: s.overview.total_issues,
        })),
      },
      {
        title: 'Criticals',
        color: palette.danger,
        format: (value: number) => String(value),
        bars: recentChecks.value.map((s) => ({
          label: barLabel(s.created_at),
          value: s.severity_counts.fatal + s.severity_counts.error,
        })),
      },
    ]
  }
  return [
    {
      title: 'Nodes',
      color: palette.primary,
      format: (value: number) => String(value),
      bars: recentGraph.value.map((s) => ({
        label: barLabel(s.created_at),
        value: s.total_nodes,
      })),
    },
    {
      title: 'Edges',
      color: palette.primary,
      format: (value: number) => String(value),
      bars: recentGraph.value.map((s) => ({
        label: barLabel(s.created_at),
        value: s.total_edges,
      })),
    },
    {
      title: 'External',
      color: palette.info,
      format: (value: number) => String(value),
      bars: recentGraph.value.map((s) => ({
        label: barLabel(s.created_at),
        value: s.external_nodes,
      })),
    },
    {
      title: 'Roots',
      color: palette.success,
      format: (value: number) => String(value),
      bars: recentGraph.value.map((s) => ({
        label: barLabel(s.created_at),
        value: s.root_nodes,
      })),
    },
  ]
})

async function load(): Promise<void> {
  const id = props.auditId
  if (!id) return
  const request = ++requestId
  loading.value = true
  error.value = null
  try {
    if (scope.value === 'checks') {
      const data = await api.listSnapshots(id)
      if (request === requestId) snapshots.value = data
    } else {
      const data = await api.listGraphSnapshots(id)
      if (request === requestId) graphSnapshots.value = data
    }
    first.value = 0
  } catch (e) {
    if (request === requestId) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  } finally {
    if (request === requestId) loading.value = false
  }
}

function setScope(next: Scope): void {
  if (scope.value === next) return
  scope.value = next
  const query: Record<string, string> = {}
  for (const [key, value] of Object.entries(route.query)) {
    if (typeof value === 'string' && key !== 'scope') query[key] = value
  }
  query.scope = next
  router.replace({ query })
}

function onRowsChange(value: number | null): void {
  if (typeof value !== 'number' || value === rows.value) return
  rows.value = value
  first.value = 0
}

watch(
  () => route.query.scope,
  (value) => {
    const next: Scope = value === 'graph' ? 'graph' : 'checks'
    if (next !== scope.value) scope.value = next
  },
)
watch(() => props.auditId, load, { immediate: true })
watch(scope, load)
watch(dataVersion, () => {
  load()
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-4">
    <Message v-if="error" severity="error">{{ error }}</Message>

    <div class="inline-flex w-fit overflow-hidden rounded border border-slate-200">
      <button
        type="button"
        class="px-3 py-1.5 text-sm transition-colors"
        :class="
          scope === 'checks'
            ? 'bg-[var(--p-primary-color)] text-[var(--p-primary-contrast-color)]'
            : 'text-slate-600 hover:bg-slate-100'
        "
        @click="setScope('checks')"
      >
        Checks
      </button>
      <button
        type="button"
        class="px-3 py-1.5 text-sm transition-colors"
        :class="
          scope === 'graph'
            ? 'bg-[var(--p-primary-color)] text-[var(--p-primary-contrast-color)]'
            : 'text-slate-600 hover:bg-slate-100'
        "
        @click="setScope('graph')"
      >
        Graph
      </button>
    </div>

    <div
      v-if="rowCount"
      class="grid grid-cols-2 gap-4 lg:grid-cols-4"
    >
      <div
        v-for="chart in activeCharts"
        :key="chart.title"
        class="rounded-lg border border-slate-200 bg-white p-3"
      >
        <div
          class="mb-2 text-xs font-medium tracking-wide text-slate-500 uppercase"
        >
          {{ chart.title }}
        </div>
        <BarChart :bars="chart.bars" :color="chart.color" :format="chart.format" />
      </div>
    </div>

    <div class="min-h-0 flex-1">
      <DataTable
        v-if="scope === 'checks'"
        :key="checkColumnsKey"
        v-model:first="first"
        v-model:rows="rows"
        :value="checkRows"
        :loading="showTableLoading"
        paginator
        scrollable
        scroll-height="flex"
        data-key="id"
        paginator-template="FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink"
      >
        <template #paginatorstart>
          <span class="text-sm text-slate-500">{{ pageReport }}</span>
        </template>
        <template #paginatorend>
          <div class="flex items-center gap-1 text-sm text-slate-500">
            <ColumnToggle
              v-model:order="checkColumnOrder"
              v-model:visible="checkVisibleFields"
              :labels="checkColumnLabels"
              @reset="resetCheckColumns"
            />
            <Button
              label="Refresh"
              text
              size="small"
              aria-label="Refresh timeline"
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
          <p v-if="!loading" class="py-16 text-center text-sm text-slate-500">
            No check runs for this audit yet.
          </p>
        </template>

        <template v-for="field in checkVisibleOrderedFields" :key="field">
        <Column v-if="field === 'time'" field="time" header="Time" style="min-width: 11rem">
          <template #body="{ data }">
            <span class="text-sm text-slate-700">{{ data.time }}</span>
          </template>
        </Column>
        <Column v-else-if="field === 'score'" field="score" header="Score" style="min-width: 7rem">
          <template #body="{ data }">
            <span
              class="font-semibold tabular-nums"
              :style="{ color: auditScoreColor(data.score) }"
            >
              {{ data.score.toFixed(1) }}
            </span>
          </template>
        </Column>
        <Column v-else-if="field === 'urls'" field="urls" header="URLs" style="min-width: 9rem">
          <template #body="{ data }">
            <span class="tabular-nums text-slate-600">{{ data.urls }}</span>
          </template>
        </Column>
        <Column v-else-if="field === 'issues'" field="issues" header="Issues" style="min-width: 10rem">
          <template #body="{ data }">
            <span class="tabular-nums text-slate-600">{{ data.issues }}</span>
          </template>
        </Column>
        <Column v-else-if="field === 'criticals'" field="criticals" header="Criticals" style="min-width: 8rem">
          <template #body="{ data }">
            <span
              class="tabular-nums"
              :style="{
                color: data.criticals > 0 ? palette.danger : palette.neutral,
              }"
            >
              {{ data.criticals }}
            </span>
          </template>
        </Column>
        <Column v-else-if="field === 'duration'" field="duration" header="Duration" style="min-width: 8rem">
          <template #body="{ data }">
            <span class="tabular-nums text-slate-600">{{ data.duration }}</span>
          </template>
        </Column>
        </template>
      </DataTable>

      <DataTable
        v-else
        :key="graphColumnsKey"
        v-model:first="first"
        v-model:rows="rows"
        :value="graphRows"
        :loading="showTableLoading"
        paginator
        scrollable
        scroll-height="flex"
        data-key="id"
        paginator-template="FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink"
      >
        <template #paginatorstart>
          <span class="text-sm text-slate-500">{{ pageReport }}</span>
        </template>
        <template #paginatorend>
          <div class="flex items-center gap-1 text-sm text-slate-500">
            <ColumnToggle
              v-model:order="graphColumnOrder"
              v-model:visible="graphVisibleFields"
              :labels="graphColumnLabels"
              @reset="resetGraphColumns"
            />
            <Button
              label="Refresh"
              text
              size="small"
              aria-label="Refresh timeline"
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
          <p v-if="!loading" class="py-16 text-center text-sm text-slate-500">
            No graph runs for this audit yet.
          </p>
        </template>

        <template v-for="field in graphVisibleOrderedFields" :key="field">
        <Column v-if="field === 'time'" field="time" header="Time" style="min-width: 11rem">
          <template #body="{ data }">
            <span class="text-sm text-slate-700">{{ data.time }}</span>
          </template>
        </Column>
        <Column v-else-if="field === 'nodes'" field="nodes" header="Nodes" style="min-width: 7rem">
          <template #body="{ data }">
            <span class="tabular-nums text-slate-600">{{ data.nodes }}</span>
          </template>
        </Column>
        <Column v-else-if="field === 'edges'" field="edges" header="Edges" style="min-width: 7rem">
          <template #body="{ data }">
            <span class="tabular-nums text-slate-600">{{ data.edges }}</span>
          </template>
        </Column>
        <Column v-else-if="field === 'internal'" field="internal" header="Internal" style="min-width: 7rem">
          <template #body="{ data }">
            <span class="tabular-nums text-slate-600">{{ data.internal }}</span>
          </template>
        </Column>
        <Column v-else-if="field === 'external'" field="external" header="External" style="min-width: 7rem">
          <template #body="{ data }">
            <span class="tabular-nums text-slate-600">{{ data.external }}</span>
          </template>
        </Column>
        <Column v-else-if="field === 'roots'" field="roots" header="Roots" style="min-width: 6rem">
          <template #body="{ data }">
            <span class="tabular-nums text-slate-600">{{ data.roots }}</span>
          </template>
        </Column>
        <Column v-else-if="field === 'duration'" field="duration" header="Duration" style="min-width: 8rem">
          <template #body="{ data }">
            <span class="tabular-nums text-slate-600">{{ data.duration }}</span>
          </template>
        </Column>
        </template>
      </DataTable>
    </div>
  </div>
</template>
