<script setup lang="ts">
import Card from 'primevue/card'
import Message from 'primevue/message'
import MeterGroup from 'primevue/metergroup'
import ProgressSpinner from 'primevue/progressspinner'
import { computed, ref, watch } from 'vue'
import * as api from '../api/client'
import { dataVersion } from '../lib/appState'
import { useDelayedLoading } from '../lib/loading'
import type { AuditSnapshot, DashboardResponse } from '../types'
import {
  auditScoreColor,
  lifecycles,
  palette,
  severities,
} from '../lib/theme'
import {
  durationDelta,
  floatDelta,
  formatDuration,
  intDelta,
  timeAgo,
} from '../lib/format'
import MetricTile from './MetricTile.vue'

interface Tile {
  label: string
  value: string
  color: string
  delta?: string
  moreIsGood?: boolean
}

const props = defineProps<{ auditId: string }>()

const dashboard = ref<DashboardResponse | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)
const showLoading = useDelayedLoading(loading)
let requestId = 0

const audit = computed(() => dashboard.value!.audit)
const metrics = computed(() => dashboard.value!.metrics)
const previous = computed<AuditSnapshot | null>(() => dashboard.value!.previous)
const checks = computed(() => dashboard.value!.checks)

async function load(): Promise<void> {
  const id = props.auditId
  if (!id) return
  const request = ++requestId
  loading.value = true
  error.value = null
  try {
    const data = await api.getDashboard(id)
    if (request === requestId) dashboard.value = data
  } catch (e) {
    if (request === requestId) {
      dashboard.value = null
      error.value = e instanceof Error ? e.message : String(e)
    }
  } finally {
    if (request === requestId) loading.value = false
  }
}

watch(() => props.auditId, load, { immediate: true })
watch(dataVersion, () => {
  if (dashboard.value) load()
})

const totalUrls = computed(() => {
  const s = metrics.value.URLStates
  return s.new + s.active + s.missing
})

const totalIssues = computed(() => {
  const s = metrics.value.Severity
  return s.success + s.notice + s.warning + s.error + s.fatal
})

const overviewTiles = computed<Tile[]>(() => {
  const prev = previous.value
  return [
    {
      label: 'Total URLs',
      value: String(totalUrls.value),
      color: palette.info,
      delta: prev
        ? intDelta(totalUrls.value, prev.overview.total_urls)
        : undefined,
      moreIsGood: true,
    },
    {
      label: 'Total Issues',
      value: String(totalIssues.value),
      color: palette.warning,
      delta: prev
        ? intDelta(totalIssues.value, prev.overview.total_issues)
        : undefined,
    },
    {
      label: 'Total Checks',
      value: String(checks.value),
      color: palette.neutral,
      delta: prev
        ? intDelta(checks.value, prev.overview.total_checks)
        : undefined,
      moreIsGood: true,
    },
    {
      label: 'Score',
      value: audit.value.score.toFixed(1),
      color: auditScoreColor(audit.value.score),
      delta: prev ? floatDelta(audit.value.score, prev.overview.score) : undefined,
      moreIsGood: true,
    },
  ]
})

const urlStateTiles = computed<Tile[]>(() => {
  const s = metrics.value.URLStates
  const prev = previous.value?.url_states
  return [
    {
      label: 'New',
      value: String(s.new),
      color: palette.info,
      delta: prev ? intDelta(s.new, prev.new) : undefined,
      moreIsGood: true,
    },
    {
      label: 'Active',
      value: String(s.active),
      color: palette.success,
      delta: prev ? intDelta(s.active, prev.active) : undefined,
      moreIsGood: true,
    },
    {
      label: 'Missing',
      value: String(s.missing),
      color: palette.danger,
      delta: prev ? intDelta(s.missing, prev.missing) : undefined,
    },
  ]
})

const urlStateMeter = computed(() => {
  const s = metrics.value.URLStates
  return [
    { label: 'New', value: s.new, color: palette.info },
    { label: 'Active', value: s.active, color: palette.success },
    { label: 'Missing', value: s.missing, color: palette.danger },
  ]
})

const urlStateMax = computed(() => Math.max(1, totalUrls.value))

const severityTiles = computed<Tile[]>(() => {
  const s = metrics.value.Severity
  const prev = previous.value?.severity_counts
  return severities.map((meta) => ({
    label: meta.label,
    value: String(s[meta.key]),
    color: meta.color,
    delta: prev ? intDelta(s[meta.key], prev[meta.key]) : undefined,
    moreIsGood: meta.key === 'success',
  }))
})

// A single stacked MeterGroup replaces the per-metric bar chart. The tiles
// above it already act as the legend, so the meter renders only the bar.
const severityMeter = computed(() =>
  severities.map((meta) => ({
    label: meta.label,
    value: metrics.value.Severity[meta.key],
    color: meta.color,
  })),
)

const severityMax = computed(() => Math.max(1, totalIssues.value))

const lifecycleTiles = computed<Tile[]>(() => {
  const l = metrics.value.Lifecycle
  const prev = previous.value?.lifecycle_counts
  return lifecycles.map((meta) => ({
    label: meta.label,
    value: String(l[meta.key]),
    color: meta.color,
    delta: prev ? intDelta(l[meta.key], prev[meta.key]) : undefined,
    moreIsGood: meta.moreIsGood,
  }))
})

const lifecycleMeter = computed(() =>
  lifecycles.map((meta) => ({
    label: meta.label,
    value: metrics.value.Lifecycle[meta.key],
    color: meta.color,
  })),
)

const lifecycleMax = computed(() => Math.max(1, totalIssues.value))

const timingTiles = computed<Tile[]>(() => {
  const total = audit.value.duration_ms
  const perUrl = totalUrls.value > 0 ? total / totalUrls.value : 0
  const perIssue = totalIssues.value > 0 ? total / totalIssues.value : 0

  const prev = previous.value?.timings
  const prevTotal = prev ? prev.total_ns / 1e6 : 0
  const prevPerUrl = prev ? prev.per_url_ns / 1e6 : 0
  const prevPerIssue = prev ? prev.per_issue_ns / 1e6 : 0

  return [
    {
      label: 'Total Duration',
      value: formatDuration(total),
      color: palette.info,
      delta: prev ? durationDelta(total, prevTotal) : undefined,
    },
    {
      label: 'Per URL',
      value: formatDuration(perUrl),
      color: palette.neutral,
      delta: prev ? durationDelta(perUrl, prevPerUrl) : undefined,
    },
    {
      label: 'Per Issue',
      value: formatDuration(perIssue),
      color: palette.warning,
      delta: prev ? durationDelta(perIssue, prevPerIssue) : undefined,
    },
  ]
})
</script>

<template>
  <Message v-if="error" severity="error">{{ error }}</Message>
  <div
    v-else-if="showLoading && !dashboard"
    class="flex h-full items-center justify-center"
  >
    <ProgressSpinner />
  </div>
  <div v-else-if="dashboard" class="flex flex-col gap-5">
    <header class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-semibold text-slate-800">
          {{ audit.name || 'Untitled Audit' }}
        </h1>
        <p v-if="audit.description" class="mt-1 max-w-2xl text-sm text-slate-500">
          {{ audit.description }}
        </p>
        <p class="mt-1 text-xs text-slate-400">
          {{ audit.targets.length }}
          {{ audit.targets.length === 1 ? 'target' : 'targets' }} ·
          {{ audit.check_names.length }}
          {{ audit.check_names.length === 1 ? 'check' : 'checks' }} ·
          last run {{ timeAgo(audit.started_at) }}
        </p>
      </div>
      <div class="text-right">
        <div
          class="text-4xl font-bold tabular-nums"
          :style="{ color: auditScoreColor(audit.score) }"
        >
          {{ audit.score.toFixed(1) }}
        </div>
        <div class="text-xs tracking-wide text-slate-400 uppercase">Score</div>
      </div>
    </header>

    <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
      <Card class="lg:col-span-2">
        <template #title>Overview</template>
        <template #subtitle>
          Total audited URLs, issues found and checks executed in this run.
        </template>
        <template #content>
          <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
            <MetricTile
              v-for="tile in overviewTiles"
              :key="tile.label"
              v-bind="tile"
            />
          </div>
        </template>
      </Card>

      <Card>
        <template #title>Severity</template>
        <template #subtitle>
          Issues of the latest run grouped by severity, from fatal to pass.
        </template>
        <template #content>
          <div class="grid grid-cols-5 gap-3">
            <MetricTile
              v-for="tile in severityTiles"
              :key="tile.label"
              v-bind="tile"
            />
          </div>
          <div class="mt-4">
            <MeterGroup :value="severityMeter" :max="severityMax">
              <template #label />
            </MeterGroup>
          </div>
        </template>
      </Card>

      <Card>
        <template #title>Lifecycles</template>
        <template #subtitle>
          How every issue evolved between the previous and the latest run.
        </template>
        <template #content>
          <div class="grid grid-cols-3 gap-3 sm:grid-cols-4">
            <MetricTile
              v-for="tile in lifecycleTiles"
              :key="tile.label"
              v-bind="tile"
            />
          </div>
          <div class="mt-4">
            <MeterGroup :value="lifecycleMeter" :max="lifecycleMax">
              <template #label />
            </MeterGroup>
          </div>
        </template>
      </Card>

      <Card>
        <template #title>URL States</template>
        <template #subtitle>
          How the URLs relate to the previous run: first seen, still audited or
          missing.
        </template>
        <template #content>
          <div class="grid grid-cols-3 gap-3">
            <MetricTile
              v-for="tile in urlStateTiles"
              :key="tile.label"
              v-bind="tile"
            />
          </div>
          <div class="mt-4">
            <MeterGroup :value="urlStateMeter" :max="urlStateMax">
              <template #label />
            </MeterGroup>
          </div>
        </template>
      </Card>

      <Card>
        <template #title>Timing</template>
        <template #subtitle>
          Wall-clock duration of the run and its share per URL and per issue.
        </template>
        <template #content>
          <div class="grid grid-cols-3 gap-3">
            <MetricTile
              v-for="tile in timingTiles"
              :key="tile.label"
              v-bind="tile"
            />
          </div>
        </template>
      </Card>
    </div>
  </div>
</template>
