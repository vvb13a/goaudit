<script setup lang="ts">
import {
  Copy,
  EllipsisV,
  ExternalLink,
  Moon,
  Play,
  Plus,
  Refresh,
  Sun,
  Trash,
} from '@primeicons/vue'
import Button from 'primevue/button'
import ContextMenu from 'primevue/contextmenu'
import ProgressSpinner from 'primevue/progressspinner'
import Tag from 'primevue/tag'
import { computed, ref } from 'vue'
import type { AuditSummary } from '../types'
import { timeAgo } from '../lib/format'
import { isDark, toggleColorScheme } from '../lib/colorScheme'

const props = defineProps<{
  audits: AuditSummary[]
  selectedId: string | null
  loading: boolean
  runningId?: string | null
}>()

const emit = defineEmits<{
  select: [id: string]
  refresh: []
  create: []
  run: [audit: AuditSummary]
  duplicate: [audit: AuditSummary]
  'open-tab': [audit: AuditSummary]
  reset: [audit: AuditSummary]
  delete: [audit: AuditSummary]
}>()

const menu = ref<InstanceType<typeof ContextMenu>>()
const contextAudit = ref<AuditSummary | null>(null)

const menuItems = computed(() => [
  {
    label: 'Run',
    icon: Play,
    disabled: props.runningId != null,
    command: () => contextAudit.value && emit('run', contextAudit.value),
  },
  {
    label: 'Duplicate',
    icon: Copy,
    command: () => contextAudit.value && emit('duplicate', contextAudit.value),
  },
  {
    label: 'Open in new tab',
    icon: ExternalLink,
    command: () => contextAudit.value && emit('open-tab', contextAudit.value),
  },
  { separator: true },
  {
    label: 'Reset data',
    icon: Refresh,
    class: 'text-red-600',
    command: () => contextAudit.value && emit('reset', contextAudit.value),
  },
  {
    label: 'Delete',
    icon: Trash,
    class: 'text-red-600',
    command: () => contextAudit.value && emit('delete', contextAudit.value),
  },
])

function openMenu(event: MouseEvent, audit: AuditSummary): void {
  contextAudit.value = audit
  menu.value?.show(event)
}

function onContextMenu(event: MouseEvent, audit: AuditSummary): void {
  event.preventDefault()
  openMenu(event, audit)
}

function severityFor(score: number): 'success' | 'warn' | 'danger' {
  if (score >= 70) return 'success'
  if (score >= 50) return 'warn'
  return 'danger'
}
</script>

<template>
  <aside
    class="relative flex h-full w-72 shrink-0 flex-col border-r border-slate-200 bg-white"
  >
    <div class="flex items-center justify-between px-4 py-3">
      <div>
        <div class="text-lg font-semibold text-slate-800">GoAudit</div>
        <div class="text-xs text-slate-400">Audits</div>
      </div>
      <div class="flex items-center gap-1">
        <Button
          text
          rounded
          :aria-label="isDark ? 'Switch to light mode' : 'Switch to dark mode'"
          :title="isDark ? 'Switch to light mode' : 'Switch to dark mode'"
          @click="toggleColorScheme"
        >
          <template #icon>
            <Sun v-if="isDark" :size="16" />
            <Moon v-else :size="16" />
          </template>
        </Button>
        <Button
          text
          rounded
          aria-label="Refresh audits"
          :loading="loading"
          @click="emit('refresh')"
        >
          <template #icon><Refresh :size="16" /></template>
        </Button>
      </div>
    </div>

    <div class="px-3 pb-2">
      <Button
        label="New Audit"
        class="w-full"
        severity="secondary"
        outlined
        @click="emit('create')"
      >
        <template #icon><Plus :size="16" /></template>
      </Button>
    </div>

    <nav class="flex-1 overflow-y-auto px-2 pb-3">
      <p
        v-if="!audits.length && !loading"
        class="px-3 py-6 text-center text-sm text-slate-400"
      >
        No audits yet. Create one to get started.
      </p>

      <div
        v-for="audit in audits"
        :key="audit.id"
        role="button"
        tabindex="0"
        class="group mb-1 flex w-full cursor-pointer items-center justify-between gap-2 rounded-lg px-3 py-2 text-left transition-colors"
        :class="
          audit.id === selectedId
            ? 'bg-slate-100 ring-1 ring-slate-200'
            : 'hover:bg-slate-50'
        "
        @click="emit('select', audit.id)"
        @keydown.enter="emit('select', audit.id)"
        @contextmenu="onContextMenu($event, audit)"
      >
        <span class="min-w-0">
          <span class="block truncate text-sm font-medium text-slate-700">
            {{ audit.name || 'Untitled Audit' }}
          </span>
          <span class="flex items-center gap-1.5 text-xs text-slate-400">
            <ProgressSpinner
              v-if="runningId === audit.id"
              class="h-3 w-3"
              :stroke-width="8"
            />
            <span v-if="runningId === audit.id">Running…</span>
            <span v-else>{{ timeAgo(audit.started_at) }}</span>
          </span>
        </span>
        <span class="flex shrink-0 items-center gap-1">
          <Tag
            v-if="runningId !== audit.id"
            :value="audit.score.toFixed(1)"
            :severity="severityFor(audit.score)"
          />
          <Button
            text
            rounded
            size="small"
            aria-label="Audit actions"
            class="opacity-0 transition-opacity group-hover:opacity-100 focus:opacity-100"
            @click.stop="openMenu($event, audit)"
          >
            <template #icon><EllipsisV :size="16" /></template>
          </Button>
        </span>
      </div>
    </nav>

    <div class="border-t border-slate-200 px-4 py-2 text-xs text-slate-400">
      Right-click an audit for actions
    </div>

    <ContextMenu ref="menu" :model="menuItems" @hide="contextAudit = null" />
  </aside>
</template>
