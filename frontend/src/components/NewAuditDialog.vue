<script setup lang="ts">
import { Plus, Times } from '@primeicons/vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import { useToast } from 'primevue/usetoast'
import { ref, watch } from 'vue'
import * as api from '../api/client'
import {
  defaultAuditConfig,
  normalizeAuditConfig,
  validateAuditConfig,
} from '../lib/config'
import { useDelayedLoading } from '../lib/loading'
import type { AuditConfig, AuditSummary, CheckInfo } from '../types'
import AuditConfigForm from './AuditConfigForm.vue'

const visible = defineModel<boolean>({ required: true })
const emit = defineEmits<{ created: [audit: AuditSummary] }>()

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
  loading.value = true
  error.value = null
  try {
    checks.value = await api.listChecks()
    const base = defaultAuditConfig()
    base.check_names = checks.value.map((c) => c.name)
    form.value = base
  } catch (e) {
    error.value = toMessage(e)
  } finally {
    loading.value = false
  }
}

async function save(): Promise<void> {
  const payload = normalizeAuditConfig(form.value)
  const problem = validateAuditConfig(payload)
  if (problem) {
    toast.add({
      severity: 'warn',
      summary: 'Cannot create',
      detail: problem,
      life: 4000,
    })
    return
  }

  saving.value = true
  error.value = null
  try {
    const audit = await api.createAudit(payload)
    emit('created', audit)
    visible.value = false
    toast.add({
      severity: 'success',
      summary: 'Audit created',
      detail: 'Use Rerun from the audit menu to run it.',
      life: 4000,
    })
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
    header="New Audit"
    :style="{ width: '64rem', maxWidth: '96vw' }"
  >
    <Message v-if="error" severity="error" class="mb-4">{{ error }}</Message>

    <div v-if="showLoading" class="flex h-[34rem] items-center justify-center">
      <ProgressSpinner />
    </div>

    <div v-else class="h-[34rem]">
      <AuditConfigForm v-model="form" :checks="checks" :disabled="saving" />
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
          label="Create"
          severity="success"
          :loading="saving"
          :disabled="loading"
          @click="save"
        >
          <template #icon><Plus :size="16" /></template>
        </Button>
      </div>
    </template>
  </Dialog>
</template>
