<script setup lang="ts">
import {
  Box,
  ChartBar,
  ChevronDown,
  Clipboard,
  Clock,
  Cog,
  ExclamationTriangle,
  Heart,
  Link,
  List,
  Map,
  ShareAlt,
  Sitemap,
} from '@primeicons/vue'
import ConfirmDialog from 'primevue/confirmdialog'
import Menu from 'primevue/menu'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Toast from 'primevue/toast'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import { type Component, computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import * as api from './api/client'
import AuditSidebar from './components/AuditSidebar.vue'
import NewAuditDialog from './components/NewAuditDialog.vue'
import RunAuditDialog from './components/RunAuditDialog.vue'
import RunProgressDialog from './components/RunProgressDialog.vue'
import { notifyDataChanged, dataVersion } from './lib/appState'
import { useDelayedLoading } from './lib/loading'
import type { AuditSummary, RunOverride, RunStatus } from './types'

type Tab = 'dashboard' | 'health' | 'graph' | 'timeline' | 'config'

const tabs: { id: Tab; label: string; icon: Component }[] = [
  { id: 'dashboard', label: 'Dashboard', icon: ChartBar },
  { id: 'health', label: 'Health', icon: Heart },
  { id: 'graph', label: 'Graph', icon: Sitemap },
  { id: 'timeline', label: 'Timeline', icon: Clock },
  { id: 'config', label: 'Config', icon: Cog },
]

const tabNames = tabs.map((t) => t.id) as string[]

// The substeps each dropdown tab groups, with the sub-route used when the
// audit is selected while that tab is active.
const healthRoutes = ['urls', 'issues', 'checks']
const defaultRoute: Record<Tab, string> = {
  dashboard: 'dashboard',
  health: 'urls',
  graph: 'graph',
  timeline: 'timeline',
  config: 'config',
}

const route = useRoute()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()

const audits = ref<AuditSummary[]>([])
const loadingAudits = ref(false)
const error = ref<string | null>(null)
const showAuditsLoading = useDelayedLoading(loadingAudits)

const newDialogVisible = ref(false)

const runningAuditId = ref<string | null>(null)
const runningAuditName = ref('')
const runStatus = ref<RunStatus | null>(null)
const runDialogVisible = ref(false)
const runTarget = ref<AuditSummary | null>(null)
const runConfigVisible = ref(false)
let runTimer: ReturnType<typeof setTimeout> | undefined

const selectedId = computed(() => (route.params.auditId as string | undefined) ?? null)
// The Health tab groups the URL/issue/check sub-routes and the Graph tab groups
// the graph sub-routes; any sub-route keeps its dropdown highlighted.
const activeTab = computed<Tab>(() => {
  const name = route.name as string
  if (healthRoutes.includes(name)) return 'health'
  if (name && name.startsWith('graph')) return 'graph'
  return tabNames.includes(name) ? (name as Tab) : 'dashboard'
})

const healthMenu = ref<InstanceType<typeof Menu>>()
const graphMenu = ref<InstanceType<typeof Menu>>()

const healthMenuItems = computed(() => [
  { label: 'URLs', icon: Link, command: () => openSub('urls') },
  { label: 'Issues', icon: List, command: () => openSub('issues') },
  { label: 'Checks', icon: Clipboard, command: () => openSub('checks') },
])

const graphMenuItems = computed(() => [
  { label: 'Navigator', icon: Sitemap, command: () => openSub('graph-navigator') },
  { label: 'Visualization', icon: Map, command: () => openSub('graph-map') },
  { label: 'Nodes', icon: Box, command: () => openSub('graph-nodes') },
  { label: 'Edges', icon: ShareAlt, command: () => openSub('graph-edges') },
])

function toggleHealthMenu(event: Event): void {
  healthMenu.value?.toggle(event)
}

function toggleGraphMenu(event: Event): void {
  graphMenu.value?.toggle(event)
}

function openSub(name: string): void {
  if (selectedId.value) {
    router.push({ name, params: { auditId: selectedId.value } })
  } else {
    router.push({ name: 'audits' })
  }
}

function toMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

function notification(
  severity: 'success' | 'error' | 'info',
  summary: string,
  detail?: string,
): void {
  toast.add({ severity, summary, detail, life: 4000 })
}

// Load the audit list and keep the URL pointed at a valid audit: when the
// current one is missing (first load, deleted) redirect to the first audit.
async function loadAudits(): Promise<void> {
  loadingAudits.value = true
  error.value = null
  try {
    audits.value = await api.listAudits()
    const current = selectedId.value
    const exists = current && audits.value.some((a) => a.id === current)
    if (!exists) {
      if (audits.value.length) {
        await router.replace({
          name: 'dashboard',
          params: { auditId: audits.value[0].id },
        })
      } else if (current) {
        await router.replace({ name: 'audits' })
      }
    }
  } catch (e) {
    error.value = toMessage(e)
  } finally {
    loadingAudits.value = false
  }
}

function selectAudit(id: string): void {
  router.push({ name: defaultRoute[activeTab.value], params: { auditId: id } })
}

// A tab is a real link so it can be opened in a new tab/window (right-click or
// middle-click) and copied. Without a selected audit it falls back to the
// audit list.
function tabTarget(id: Tab) {
  return selectedId.value
    ? { name: id, params: { auditId: selectedId.value } }
    : { name: 'audits' }
}

// Open the audit on the current tab in a new browser tab, so two audits can be
// compared side by side.
function onOpenTab(audit: AuditSummary): void {
  const href = router.resolve({
    name: defaultRoute[activeTab.value],
    params: { auditId: audit.id },
  }).href
  window.open(href, '_blank', 'noopener,noreferrer')
}

function onCreate(): void {
  newDialogVisible.value = true
}

async function onCreated(audit: AuditSummary): Promise<void> {
  await loadAudits()
  router.push({ name: 'config', params: { auditId: audit.id } })
}

async function onDuplicate(audit: AuditSummary): Promise<void> {
  try {
    const clone = await api.duplicateAudit(audit.id)
    notifyDataChanged()
    notification('success', 'Audit duplicated', clone.name)
  } catch (e) {
    notification('error', 'Duplicate failed', toMessage(e))
  }
}

function onReset(audit: AuditSummary): void {
  confirm.require({
    header: 'Reset audit data',
    message: `Reset '${audit.name}'? This deletes its URLs, issues and run history. The configuration is kept.`,
    icon: ExclamationTriangle,
    acceptLabel: 'Reset',
    rejectLabel: 'Cancel',
    acceptProps: { severity: 'danger' },
    rejectProps: { text: true },
    accept: async () => {
      try {
        await api.resetAudit(audit.id)
        notifyDataChanged()
        notification('success', 'Audit reset', audit.name)
      } catch (e) {
        notification('error', 'Reset failed', toMessage(e))
      }
    },
  })
}

function onDelete(audit: AuditSummary): void {
  confirm.require({
    header: 'Delete audit',
    message: `Delete '${audit.name}'? This cannot be undone.`,
    icon: ExclamationTriangle,
    acceptLabel: 'Delete',
    rejectLabel: 'Cancel',
    acceptProps: { severity: 'danger' },
    rejectProps: { text: true },
    accept: async () => {
      try {
        await api.deleteAudit(audit.id)
        notifyDataChanged()
        notification('success', 'Audit deleted', audit.name)
      } catch (e) {
        notification('error', 'Delete failed', toMessage(e))
      }
    },
  })
}

function clearTimer(): void {
  if (runTimer) {
    clearTimeout(runTimer)
    runTimer = undefined
  }
}

// The Run action first opens a small config dialog; the actual run starts once
// the dialog confirms (and has persisted any engine-setting changes).
function onRunRequest(audit: AuditSummary): void {
  if (runningAuditId.value) {
    notification('info', 'A run is already in progress', runningAuditName.value)
    return
  }
  runTarget.value = audit
  runConfigVisible.value = true
}

function onRunConfirm(override: RunOverride): void {
  const audit = runTarget.value
  if (audit) void startRun(audit, override)
}

// The quick-run dialog only carries a couple of settings; this hands the user
// off to the full Config tab instead of running.
function onRunEdit(): void {
  const audit = runTarget.value
  if (audit) {
    router.push({ name: 'config', params: { auditId: audit.id } })
  }
}

async function startRun(
  audit: AuditSummary,
  override?: RunOverride,
): Promise<void> {
  runningAuditId.value = audit.id
  runningAuditName.value = audit.name
  runStatus.value = { state: 'running', completed: 0, total: 0 }
  runDialogVisible.value = true
  try {
    runStatus.value = await api.startRun(audit.id, override)
  } catch (e) {
    runningAuditId.value = null
    runStatus.value = { state: 'error', completed: 0, total: 0, error: toMessage(e) }
    return
  }
  schedulePoll()
}

function schedulePoll(): void {
  clearTimer()
  runTimer = setTimeout(pollRun, 1000)
}

async function pollRun(): Promise<void> {
  const id = runningAuditId.value
  if (!id) return
  try {
    const status = await api.getRunStatus(id)
    runStatus.value = status
    if (status.state === 'running') {
      schedulePoll()
      return
    }
    await finishRun(id, status)
  } catch {
    schedulePoll()
  }
}

async function finishRun(_id: string, status: RunStatus): Promise<void> {
  clearTimer()
  runningAuditId.value = null
  notifyDataChanged()
  if (status.state === 'done') {
    notification('success', 'Run complete', runningAuditName.value)
  } else {
    notification('error', 'Run failed', status.error)
  }
}

// Any data mutation (run, recheck, duplicate, reset, delete) refreshes the
// audit list and the currently mounted view.
watch(dataVersion, () => {
  void loadAudits()
})

onMounted(loadAudits)
onBeforeUnmount(clearTimer)
</script>

<template>
  <div class="flex h-screen bg-slate-50 text-slate-800">
    <AuditSidebar
      :audits="audits"
      :selected-id="selectedId"
      :loading="loadingAudits"
      :running-id="runningAuditId"
      @select="selectAudit"
      @refresh="loadAudits"
      @create="onCreate"
      @run="onRunRequest"
      @duplicate="onDuplicate"
      @open-tab="onOpenTab"
      @reset="onReset"
      @delete="onDelete"
    />

    <main class="flex flex-1 flex-col overflow-hidden">
      <nav class="flex items-center gap-1 border-b border-slate-200 bg-white px-6">
        <template v-for="tab in tabs" :key="tab.id">
          <button
            v-if="tab.id === 'health' || tab.id === 'graph'"
            type="button"
            class="flex items-center gap-2 border-b-2 px-3 py-3 text-sm font-medium transition-colors"
            :class="
              tab.id === activeTab
                ? 'border-[var(--p-primary-color)] text-[var(--p-primary-color)]'
                : 'border-transparent text-slate-400 hover:text-slate-600'
            "
            @click="tab.id === 'health' ? toggleHealthMenu($event) : toggleGraphMenu($event)"
          >
            <component :is="tab.icon" :size="16" />
            {{ tab.label }}
            <ChevronDown :size="12" />
          </button>
          <RouterLink
            v-else
            :to="tabTarget(tab.id)"
            class="flex items-center gap-2 border-b-2 px-3 py-3 text-sm font-medium transition-colors"
            :class="
              tab.id === activeTab
                ? 'border-[var(--p-primary-color)] text-[var(--p-primary-color)]'
                : 'border-transparent text-slate-400 hover:text-slate-600'
            "
          >
            <component :is="tab.icon" :size="16" />
            {{ tab.label }}
          </RouterLink>
        </template>
      </nav>
      <Menu ref="healthMenu" :model="healthMenuItems" :popup="true" />
      <Menu ref="graphMenu" :model="graphMenuItems" :popup="true" />

      <div class="flex min-h-0 flex-1 flex-col">
        <div v-if="error" class="px-6 pt-6">
          <Message severity="error">{{ error }}</Message>
        </div>

        <div
          v-if="!selectedId"
          class="flex flex-1 items-center justify-center text-slate-400"
        >
          <ProgressSpinner v-if="showAuditsLoading" />
          <span v-else>Select an audit to get started.</span>
        </div>

        <div
          v-else
          :class="
            activeTab === 'dashboard'
              ? 'flex-1 overflow-y-auto p-6'
              : 'min-h-0 flex-1 overflow-hidden p-6'
          "
        >
          <RouterView />
        </div>
      </div>
    </main>

    <NewAuditDialog v-model="newDialogVisible" @created="onCreated" />
    <RunAuditDialog
      v-model="runConfigVisible"
      :audit="runTarget"
      @confirm="onRunConfirm"
      @edit="onRunEdit"
    />
    <RunProgressDialog
      v-model="runDialogVisible"
      :name="runningAuditName"
      :status="runStatus"
    />
    <ConfirmDialog />
    <Toast />
  </div>
</template>
