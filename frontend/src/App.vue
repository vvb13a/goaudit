<script setup lang="ts">
import { ExclamationTriangle } from '@primeicons/vue'
import ConfirmDialog from 'primevue/confirmdialog'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Toast from 'primevue/toast'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterView, useRoute, useRouter } from 'vue-router'
import * as api from './api/client'
import AuditSidebar from './components/AuditSidebar.vue'
import NewAuditDialog from './components/NewAuditDialog.vue'
import RunAuditDialog from './components/RunAuditDialog.vue'
import RunProgressDialog from './components/RunProgressDialog.vue'
import { notifyDataChanged, dataVersion } from './lib/appState'
import { useDelayedLoading } from './lib/loading'
import type { AuditSummary, RunOverride, RunStatus } from './types'

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
const isDashboard = computed(() => route.name === 'dashboard')

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
      @create="onCreate"
      @run="onRunRequest"
      @duplicate="onDuplicate"
      @reset="onReset"
      @delete="onDelete"
    />

    <main class="flex min-h-0 flex-1 flex-col overflow-hidden">
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
          isDashboard
            ? 'flex-1 overflow-y-auto p-6'
            : 'min-h-0 flex-1 overflow-hidden p-6'
        "
      >
        <RouterView />
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
