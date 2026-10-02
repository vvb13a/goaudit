<script setup lang="ts">
import { Refresh } from '@primeicons/vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Message from 'primevue/message'
import Select from 'primevue/select'
import { computed, ref, watch } from 'vue'
import * as api from '../api/client'
import { dataVersion } from '../lib/appState'
import { formatDuration } from '../lib/format'
import { useDelayedLoading } from '../lib/loading'
import { auditScoreColor, palette } from '../lib/theme'
import type { AuditSnapshot } from '../types'
import BarChart, { type Bar } from './BarChart.vue'

const props = defineProps<{ auditId: string }>()

const snapshots = ref<AuditSnapshot[]>([])
const loading = ref(false)
const showTableLoading = useDelayedLoading(loading)
const error = ref<string | null>(null)
let requestId = 0

const first = ref(0)
const rows = ref(25)
const pageSizeOptions = [10, 25, 50, 100]

interface TimelineRow {
  id: string
  time: string
  score: number
  urls: string
  issues: string
  criticals: number
  duration: string
}

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

// "total (+new)", omitting the new part when nothing is new.
function totalWithNew(total: number, newCount: number, showNew: boolean): string {
  return showNew && newCount > 0 ? `${total} (+${newCount})` : String(total)
}

const tableRows = computed<TimelineRow[]>(() => {
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

const pageReport = computed(() => {
  const total = tableRows.value.length
  if (total === 0) return '0 of 0'
  const start = first.value + 1
  const end = Math.min(first.value + rows.value, total)
  return `${start}–${end} of ${total.toLocaleString()}`
})

// The latest five runs, oldest to newest, as compact bar charts.
const recent = computed(() => snapshots.value.slice(0, 5).reverse())

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

const charts = computed<
  { title: string; color: string; format: (value: number) => string; bars: Bar[] }[]
>(() => [
  {
    title: 'Score',
    color: palette.primary,
    format: (value: number) => value.toFixed(1),
    bars: recent.value.map((s) => ({
      label: barLabel(s.created_at),
      value: s.overview.score,
      color: auditScoreColor(s.overview.score),
    })),
  },
  {
    title: 'URLs',
    color: palette.primary,
    format: (value: number) => String(value),
    bars: recent.value.map((s) => ({
      label: barLabel(s.created_at),
      value: s.overview.total_urls,
    })),
  },
  {
    title: 'Issues',
    color: palette.primary,
    format: (value: number) => String(value),
    bars: recent.value.map((s) => ({
      label: barLabel(s.created_at),
      value: s.overview.total_issues,
    })),
  },
  {
    title: 'Criticals',
    color: palette.danger,
    format: (value: number) => String(value),
    bars: recent.value.map((s) => ({
      label: barLabel(s.created_at),
      value: s.severity_counts.fatal + s.severity_counts.error,
    })),
  },
])

async function load(): Promise<void> {
  const id = props.auditId
  if (!id) return
  const request = ++requestId
  loading.value = true
  error.value = null
  try {
    const data = await api.listSnapshots(id)
    if (request === requestId) snapshots.value = data
  } catch (e) {
    if (request === requestId) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  } finally {
    if (request === requestId) loading.value = false
  }
}

function onRowsChange(value: number | null): void {
  if (typeof value !== 'number' || value === rows.value) return
  rows.value = value
  first.value = 0
}

watch(() => props.auditId, load, { immediate: true })
watch(dataVersion, () => {
  load()
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-4">
    <Message v-if="error" severity="error">{{ error }}</Message>

    <div
      v-if="snapshots.length"
      class="grid grid-cols-2 gap-4 lg:grid-cols-4"
    >
      <div
        v-for="chart in charts"
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
        v-model:first="first"
        v-model:rows="rows"
        :value="tableRows"
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
          <p
            v-if="!loading"
            class="py-16 text-center text-sm text-slate-500"
          >
            No runs for this audit yet.
          </p>
        </template>

        <Column
          field="time"
          header="Time"
          style="min-width: 11rem"
        >
          <template #body="{ data }">
            <span class="text-sm text-slate-700">{{ data.time }}</span>
          </template>
        </Column>

        <Column field="score" header="Score" style="min-width: 7rem">
          <template #body="{ data }">
            <span
              class="font-semibold tabular-nums"
              :style="{ color: auditScoreColor(data.score) }"
            >
              {{ data.score.toFixed(1) }}
            </span>
          </template>
        </Column>

        <Column field="urls" header="URLs" style="min-width: 9rem">
          <template #body="{ data }">
            <span class="tabular-nums text-slate-600">{{ data.urls }}</span>
          </template>
        </Column>

        <Column field="issues" header="Issues" style="min-width: 10rem">
          <template #body="{ data }">
            <span class="tabular-nums text-slate-600">{{ data.issues }}</span>
          </template>
        </Column>

        <Column field="criticals" header="Criticals" style="min-width: 8rem">
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

        <Column field="duration" header="Duration" style="min-width: 8rem">
          <template #body="{ data }">
            <span class="tabular-nums text-slate-600">{{ data.duration }}</span>
          </template>
        </Column>
      </DataTable>
    </div>
  </div>
</template>
