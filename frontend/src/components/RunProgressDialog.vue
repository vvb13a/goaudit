<script setup lang="ts">
import { Times } from '@primeicons/vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import ProgressBar from 'primevue/progressbar'
import { computed } from 'vue'
import type { RunStatus } from '../types'

const visible = defineModel<boolean>({ required: true })

const props = defineProps<{
  name: string
  status: RunStatus | null
}>()

const running = computed(() => props.status?.state === 'running')

const percent = computed(() => {
  const s = props.status
  if (!s || s.total <= 0) return 0
  return Math.min(100, Math.round((s.completed / s.total) * 100))
})

const shortCurrent = computed(() =>
  (props.status?.current_url ?? '').replace(/^https?:\/\//, ''),
)
</script>

<template>
  <Dialog
    v-model:visible="visible"
    modal
    :closable="!running"
    :close-on-escape="!running"
    :dismissable-mask="false"
    :header="running ? 'Running audit' : 'Audit run'"
    :style="{ width: '32rem', maxWidth: '95vw' }"
  >
    <div class="flex flex-col gap-4">
      <div class="text-sm text-slate-600">
        {{ name || 'Audit' }}
      </div>

      <ProgressBar :value="percent" :show-value="false" />

      <div class="flex items-center justify-between text-xs text-slate-400">
        <span>{{ status?.completed ?? 0 }} / {{ status?.total ?? 0 }} URLs</span>
        <span>{{ percent }}%</span>
      </div>

      <div
        v-if="running && shortCurrent"
        class="truncate font-mono text-xs text-slate-500"
        :title="status?.current_url"
      >
        {{ shortCurrent }}
      </div>

      <Message v-if="status?.state === 'done'" severity="success">
        Audit finished. The dashboard and issues have been refreshed.
      </Message>
      <Message v-else-if="status?.state === 'error'" severity="error">
        {{ status.error || 'The audit run failed.' }}
      </Message>

      <div class="flex justify-end">
        <Button
          label="Close"
          size="small"
          :disabled="running"
          @click="visible = false"
        >
          <template #icon><Times :size="16" /></template>
        </Button>
      </div>
    </div>
  </Dialog>
</template>
