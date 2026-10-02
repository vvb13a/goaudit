<script setup lang="ts">
import {
  Copy,
  ExternalLink,
  Moon,
  Play,
  Plus,
  Refresh,
  Search,
  Sun,
  Trash,
} from '@primeicons/vue'
import Button from 'primevue/button'
import ContextMenu from 'primevue/contextmenu'
import InputText from 'primevue/inputtext'
import ProgressSpinner from 'primevue/progressspinner'
import SplitButton from 'primevue/splitbutton'
import Tag from 'primevue/tag'
import VirtualScroller from 'primevue/virtualscroller'
import type { MenuItem } from 'primevue/menuitem'
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import * as api from '../api/client'
import { dataVersion } from '../lib/appState'
import { isDark, toggleColorScheme } from '../lib/colorScheme'
import { timeAgo } from '../lib/format'
import { resumeRouteName } from '../lib/navigation'
import type { AuditCounts, AuditSummary } from '../types'
import SidebarNav from './SidebarNav.vue'

const props = defineProps<{
  audits: AuditSummary[]
  selectedId: string | null
  loading: boolean
  runningId?: string | null
}>()

const emit = defineEmits<{
  create: []
  run: [audit: AuditSummary]
  duplicate: [audit: AuditSummary]
  reset: [audit: AuditSummary]
  delete: [audit: AuditSummary]
}>()

const route = useRoute()
const router = useRouter()

const query = ref('')
// The current audit is shown above; this list is for switching to another one.
const otherAudits = computed(() =>
  props.audits.filter((a) => a.id !== props.selectedId),
)
const filteredAudits = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return otherAudits.value
  return otherAudits.value.filter((a) => (a.name || '').toLowerCase().includes(q))
})

const currentAudit = computed(
  () => props.audits.find((a) => a.id === props.selectedId) ?? null,
)

// Counts for the navigation badges of the current audit.
const counts = ref<AuditCounts | null>(null)

async function loadCounts(): Promise<void> {
  const id = props.selectedId
  if (!id) {
    counts.value = null
    return
  }
  try {
    counts.value = await api.getAuditCounts(id)
  } catch {
    counts.value = null
  }
}

watch(() => props.selectedId, loadCounts, { immediate: true })
watch(dataVersion, loadCounts)

// The Run button's attached dropdown acts on the current audit.
const actionItems = computed<MenuItem[]>(() => [
  {
    label: 'Duplicate',
    icon: Copy,
    command: () => currentAudit.value && emit('duplicate', currentAudit.value),
  },
  { separator: true },
  {
    label: 'Reset data',
    icon: Refresh,
    class: 'text-red-600',
    command: () => currentAudit.value && emit('reset', currentAudit.value),
  },
  {
    label: 'Delete',
    icon: Trash,
    class: 'text-red-600',
    command: () => currentAudit.value && emit('delete', currentAudit.value),
  },
])

function runCurrent(): void {
  if (currentAudit.value) emit('run', currentAudit.value)
}

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
    command: () => contextAudit.value && openTab(contextAudit.value),
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

// Switching audit keeps the current section (falling back to its parent route
// when the current route needs extra params, e.g. a graph node).
function switchAudit(auditId: string): void {
  router.push({
    name: resumeRouteName(route.name as string),
    params: { auditId },
  })
}

function openTab(audit: AuditSummary): void {
  const href = router.resolve({
    name: resumeRouteName(route.name as string),
    params: { auditId: audit.id },
  }).href
  window.open(href, '_blank', 'noopener,noreferrer')
}

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
    class="flex h-full w-72 shrink-0 flex-col overflow-hidden border-r border-slate-200 bg-white"
  >
    <!-- Current audit -->
    <div v-if="currentAudit" class="px-3 py-3">
      <div
        class="truncate text-lg font-semibold text-slate-800"
        :title="currentAudit.name"
      >
        {{ currentAudit.name || 'Untitled Audit' }}
      </div>
      <div
        v-if="currentAudit.description"
        class="mt-0.5 line-clamp-2 text-xs text-slate-400"
      >
        {{ currentAudit.description }}
      </div>
      <div class="mt-2 flex items-center justify-between gap-2">
        <Tag
          :value="currentAudit.score.toFixed(1)"
          :severity="severityFor(currentAudit.score)"
        />
        <span class="text-xs text-slate-400">
          {{ timeAgo(currentAudit.started_at) }}
        </span>
      </div>
      <SplitButton
        label="Run"
        :model="actionItems"
        severity="secondary"
        outlined
        fluid
        class="mt-3"
        @click="runCurrent"
      />
    </div>

    <div v-if="currentAudit" class="border-t border-slate-200" />

    <!-- Navigation -->
    <div class="px-2 py-2">
      <SidebarNav :counts="counts" />
    </div>

    <div class="border-t border-slate-200" />

    <!-- Audits: searchable, virtualised list so it stays fast with many audits -->
    <div class="flex min-h-0 flex-1 flex-col overflow-hidden px-3 pt-3 pb-2">
      <div class="flex items-center justify-between px-1 pb-2">
        <span class="text-xs font-semibold tracking-wide text-slate-400 uppercase">
          Audits
        </span>
        <Button
          text
          rounded
          size="small"
          aria-label="New audit"
          title="New audit"
          @click="emit('create')"
        >
          <template #icon><Plus :size="16" /></template>
        </Button>
      </div>

      <div class="relative mb-2">
        <Search
          :size="14"
          class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-slate-400"
        />
        <InputText
          v-model="query"
          class="w-full pl-9!"
          placeholder="Search audits"
        />
      </div>

      <p
        v-if="!filteredAudits.length && !loading"
        class="px-1 py-6 text-center text-sm text-slate-400"
      >
        {{
          otherAudits.length
            ? 'No audits match your search.'
            : 'No other audits. Create one to get started.'
        }}
      </p>

      <div v-else class="min-h-0 flex-1">
        <VirtualScroller
          :items="filteredAudits"
          :item-size="56"
          scroll-height="flex"
          class="h-full"
        >
          <template #item="{ item }">
            <div
              role="button"
              tabindex="0"
              class="flex h-14 cursor-pointer items-center justify-between gap-2 rounded-lg px-3 transition-colors"
              :class="
                item.id === selectedId
                  ? 'bg-slate-100 ring-1 ring-slate-200'
                  : 'hover:bg-slate-50'
              "
              @click="switchAudit(item.id)"
              @keydown.enter="switchAudit(item.id)"
              @contextmenu="onContextMenu($event, item)"
            >
              <span class="min-w-0">
                <span class="block truncate text-sm font-medium text-slate-700">
                  {{ item.name || 'Untitled Audit' }}
                </span>
                <span class="flex items-center gap-1.5 text-xs text-slate-400">
                  <ProgressSpinner
                    v-if="runningId === item.id"
                    class="h-3 w-3"
                    :stroke-width="8"
                  />
                  <span v-if="runningId === item.id">Running…</span>
                  <span v-else>{{ timeAgo(item.started_at) }}</span>
                </span>
              </span>
              <Tag
                v-if="runningId !== item.id"
                :value="item.score.toFixed(1)"
                :severity="severityFor(item.score)"
              />
            </div>
          </template>
        </VirtualScroller>
      </div>
    </div>

    <div class="border-t border-slate-200" />

    <!-- Brand -->
    <div class="flex items-center justify-between px-4 py-3">
      <div class="text-lg font-semibold text-slate-800">GoAudit</div>
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
    </div>

    <ContextMenu ref="menu" :model="menuItems" @hide="contextAudit = null" />
  </aside>
</template>
