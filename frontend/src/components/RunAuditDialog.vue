<script setup lang="ts">
import { Cog, Play, Times } from '@primeicons/vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import ToggleSwitch from 'primevue/toggleswitch'
import { computed, ref, watch } from 'vue'
import * as api from '../api/client'
import { notifyDataChanged } from '../lib/appState'
import type { AuditConfig, AuditSummary, EngineConfig, RunOverride } from '../types'

const visible = defineModel<boolean>({ required: true })
const props = defineProps<{ audit: AuditSummary | null }>()
const emit = defineEmits<{ confirm: [override: RunOverride]; edit: [] }>()

const loading = ref(false)
const saving = ref(false)
const error = ref<string | null>(null)
const original = ref<AuditConfig | null>(null)
const maxConcurrency = ref(5)
const requestDelayMs = ref(100)
const enableChecks = ref(true)
const enableGraph = ref(false)
const enableLinkValidation = ref(false)
const notify = ref(true)
const saveChanges = ref(true)

// The notify toggle only makes sense when the audit actually has notifications
// configured, and defaults to its notify-on-direct setting.
const showNotify = computed(
  () =>
    !!original.value &&
    original.value.config.notifications_enabled &&
    original.value.config.slack_webhook_url.trim() !== '',
)

const changed = computed(
  () =>
    !!original.value &&
    (maxConcurrency.value !== original.value.config.max_concurrency ||
      requestDelayMs.value !== original.value.config.request_delay_ms ||
      enableChecks.value !== original.value.config.enable_checks ||
      enableGraph.value !== original.value.config.enable_graph ||
      enableLinkValidation.value !== original.value.config.enable_link_validation),
)

function toMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

async function load(): Promise<void> {
  const audit = props.audit
  if (!audit) return
  loading.value = true
  error.value = null
  try {
    const config = await api.getAuditConfig(audit.id)
    original.value = config
    maxConcurrency.value = config.config.max_concurrency
    requestDelayMs.value = config.config.request_delay_ms
    enableChecks.value = config.config.enable_checks
    enableGraph.value = config.config.enable_graph
    enableLinkValidation.value = config.config.enable_link_validation
    notify.value = config.config.notify_on_direct
    saveChanges.value = true
  } catch (e) {
    error.value = toMessage(e)
  } finally {
    loading.value = false
  }
}

async function confirm(): Promise<void> {
  const audit = props.audit
  const config = original.value
  if (!audit || !config) return

  if (maxConcurrency.value == null || maxConcurrency.value <= 0) {
    error.value = 'Max concurrency must be a positive integer'
    return
  }
  if (requestDelayMs.value == null || requestDelayMs.value < 0) {
    error.value = 'Request delay must be an integer >= 0'
    return
  }
  if (!enableChecks.value && !enableGraph.value) {
    error.value = 'Enable checks, the link graph, or both'
    return
  }

  const inlineConfig: EngineConfig = {
    ...config.config,
    max_concurrency: maxConcurrency.value,
    request_delay_ms: requestDelayMs.value,
    enable_checks: enableChecks.value,
    enable_graph: enableGraph.value,
    enable_link_validation: enableLinkValidation.value,
  }

  saving.value = true
  error.value = null
  try {
    // The inline values are always used for the run; save them to the audit
    // configuration only when the user opted in.
    if (saveChanges.value && changed.value) {
      await api.updateAuditConfig(audit.id, { ...config, config: inlineConfig })
      notifyDataChanged()
    }
    emit('confirm', {
      config: inlineConfig,
      notify: showNotify.value ? notify.value : undefined,
    })
    visible.value = false
  } catch (e) {
    error.value = toMessage(e)
  } finally {
    saving.value = false
  }
}

watch(visible, (open) => {
  if (open) load()
})
</script>

<template>
  <Dialog
    v-model:visible="visible"
    modal
    header="Run audit"
    :style="{ width: '28rem', maxWidth: '95vw' }"
  >
    <Message v-if="error" severity="error" class="mb-4">{{ error }}</Message>

    <div v-if="loading" class="flex h-40 items-center justify-center">
      <ProgressSpinner />
    </div>

    <div v-else class="flex flex-col gap-4">
      <p class="text-sm text-slate-500">
        Run
        <span class="font-medium text-slate-700">
          {{ audit?.name || 'this audit' }}
        </span>
        . Adjust the engine settings for this run if needed.
      </p>

      <div>
        <label class="mb-1 block text-sm font-medium text-slate-600">
          Max concurrency
        </label>
        <InputNumber
          v-model="maxConcurrency"
          :min="1"
          :use-grouping="false"
          show-buttons
          class="w-full"
        />
      </div>

      <div>
        <label class="mb-1 block text-sm font-medium text-slate-600">
          Request delay (ms)
        </label>
        <InputNumber
          v-model="requestDelayMs"
          :min="0"
          :use-grouping="false"
          show-buttons
          class="w-full"
        />
      </div>

      <label class="flex items-center justify-between gap-4 border-t border-slate-200 pt-4">
        <span class="text-sm text-slate-600">Run checks</span>
        <ToggleSwitch v-model="enableChecks" />
      </label>

      <label class="flex items-center justify-between gap-4">
        <span class="text-sm text-slate-600">Build link graph</span>
        <ToggleSwitch v-model="enableGraph" />
      </label>

      <label class="flex items-center justify-between gap-4">
        <span class="text-sm text-slate-600">Validate link targets</span>
        <ToggleSwitch v-model="enableLinkValidation" :disabled="!enableGraph" />
      </label>

      <label
        v-if="showNotify"
        class="flex items-center justify-between gap-4 border-t border-slate-200 pt-4"
      >
        <span class="text-sm text-slate-600">Notify when finished</span>
        <ToggleSwitch v-model="notify" />
      </label>

      <label
        v-if="changed"
        class="flex items-center justify-between gap-4 rounded bg-slate-50 px-3 py-2"
      >
        <span class="text-sm text-slate-600">
          Save these changes to the configuration
        </span>
        <ToggleSwitch v-model="saveChanges" />
      </label>

      <p class="text-xs text-slate-400">
        {{
          changed && saveChanges
            ? 'The engine settings are saved to the audit configuration.'
            : 'The engine settings apply to this run only.'
        }}
      </p>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <Button
          label="Cancel"
          text
          :disabled="saving"
          @click="visible = false"
        >
          <template #icon><Times :size="16" /></template>
        </Button>
        <Button
          label="Edit config"
          text
          :disabled="saving || loading"
          @click="emit('edit'); visible = false"
        >
          <template #icon><Cog :size="16" /></template>
        </Button>
        <Button
          label="Run"
          severity="success"
          :loading="saving"
          :disabled="loading"
          @click="confirm"
        >
          <template #icon><Play :size="16" /></template>
        </Button>
      </div>
    </template>
  </Dialog>
</template>
