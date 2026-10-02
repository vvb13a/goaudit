<script setup lang="ts">
import { Save, Undo } from '@primeicons/vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import { useToast } from 'primevue/usetoast'
import { ref, watch } from 'vue'
import * as api from '../api/client'
import { notifyDataChanged } from '../lib/appState'
import {
  defaultAuditConfig,
  normalizeAuditConfig,
  validateAuditConfig,
} from '../lib/config'
import { useDelayedLoading } from '../lib/loading'
import type { AuditConfig, CheckInfo } from '../types'
import AuditConfigForm from './AuditConfigForm.vue'

const props = defineProps<{ auditId: string }>()

const toast = useToast()

const loading = ref(false)
const showLoading = useDelayedLoading(loading)
const saving = ref(false)
const error = ref<string | null>(null)
const checks = ref<CheckInfo[]>([])
const form = ref<AuditConfig>(defaultAuditConfig())

function toMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

async function load(): Promise<void> {
  const id = props.auditId
  if (!id) return
  loading.value = true
  error.value = null
  try {
    const [config, allChecks] = await Promise.all([
      api.getAuditConfig(id),
      api.listChecks(),
    ])
    checks.value = allChecks
    // Audits without stored check names default to every check, matching the
    // TUI editor's handling of legacy records.
    const selected = config.check_names.length
      ? config.check_names
      : allChecks.map((c) => c.name)
    form.value = { ...config, check_names: selected }
  } catch (e) {
    error.value = toMessage(e)
  } finally {
    loading.value = false
  }
}

async function save(): Promise<void> {
  const id = props.auditId
  if (!id) return
  const payload = normalizeAuditConfig(form.value)
  const problem = validateAuditConfig(payload)
  if (problem) {
    toast.add({
      severity: 'warn',
      summary: 'Cannot save',
      detail: problem,
      life: 4000,
    })
    return
  }

  saving.value = true
  error.value = null
  try {
    const updated = await api.updateAuditConfig(id, payload)
    form.value = updated
    notifyDataChanged()
    toast.add({ severity: 'success', summary: 'Configuration saved', life: 3000 })
  } catch (e) {
    error.value = toMessage(e)
    toast.add({
      severity: 'error',
      summary: 'Save failed',
      detail: error.value,
      life: 5000,
    })
  } finally {
    saving.value = false
  }
}

watch(() => props.auditId, load, { immediate: true })
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-4">
    <div class="flex items-center justify-between px-1">
      <div>
        <div class="text-lg font-semibold text-slate-800">
          Audit Configuration
        </div>
        <div class="text-xs text-slate-400">
          Name, targets, engine settings and running checks.
        </div>
      </div>
      <div class="flex items-center gap-1">
        <Button
          label="Cancel"
          text
          size="small"
          :disabled="saving || loading"
          @click="load"
        >
          <template #icon><Undo :size="16" /></template>
        </Button>
        <Button
          label="Save"
          severity="success"
          size="small"
          :loading="saving"
          :disabled="loading"
          @click="save"
        >
          <template #icon><Save :size="16" /></template>
        </Button>
      </div>
    </div>

    <Message v-if="error" severity="error">{{ error }}</Message>

    <div v-if="showLoading" class="flex flex-1 items-center justify-center">
      <ProgressSpinner />
    </div>

    <AuditConfigForm v-else v-model="form" :checks="checks" class="min-h-0 flex-1" />
  </div>
</template>
