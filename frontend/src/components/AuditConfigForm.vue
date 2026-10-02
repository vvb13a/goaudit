<script setup lang="ts">
import {
  Bell,
  CheckSquare,
  Cog,
  Globe,
  InfoCircle,
  Search,
  Shield,
} from '@primeicons/vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { computed, ref, type Component } from 'vue'
import { categoryLabel } from '../lib/theme'
import type { AuditConfig, CheckInfo, EngineConfig } from '../types'

const model = defineModel<AuditConfig>({ required: true })

const props = defineProps<{
  checks: CheckInfo[]
  disabled?: boolean
}>()

type SectionId =
  | 'general'
  | 'targets'
  | 'workflow'
  | 'engine'
  | 'notifications'
  | 'checks'

interface Section {
  id: SectionId
  label: string
  icon: Component
  hint: string
}

// Each group of fields is its own panel in the section nav.
const sections: Section[] = [
  { id: 'general', label: 'General', icon: InfoCircle, hint: 'Name and description' },
  { id: 'targets', label: 'Targets', icon: Globe, hint: 'The URLs this audit visits' },
  {
    id: 'workflow',
    label: 'Workflow',
    icon: Shield,
    hint: 'Which workflows run and how link targets are validated',
  },
  { id: 'engine', label: 'Engine', icon: Cog, hint: 'Concurrency, timeouts and user agent' },
  { id: 'notifications', label: 'Notifications', icon: Bell, hint: 'Where finished runs are announced' },
  { id: 'checks', label: 'Checks', icon: CheckSquare, hint: 'The checks applied to each page' },
]

const active = ref<SectionId>('general')

const activeSection = computed(
  () => sections.find((s) => s.id === active.value) ?? sections[0],
)

type NumericConfigKey =
  | 'max_concurrency'
  | 'request_delay_ms'
  | 'http_timeout_sec'
  | 'max_sitemap_depth'
  | 'link_cache_ttl_min'

const engineFields: {
  key: NumericConfigKey
  label: string
  min: number
}[] = [
  { key: 'max_concurrency', label: 'Max concurrency', min: 1 },
  { key: 'request_delay_ms', label: 'Request delay (ms)', min: 0 },
  { key: 'http_timeout_sec', label: 'HTTP timeout (s)', min: 1 },
  { key: 'max_sitemap_depth', label: 'Max sitemap depth', min: 1 },
]

const targetsText = computed({
  get: () => model.value.targets.join('\n'),
  set: (value: string) => {
    model.value.targets = value.split('\n')
  },
})

const selected = computed(() => new Set(model.value.check_names))

const workflowInvalid = computed(
  () => !model.value.config.enable_checks && !model.value.config.enable_graph,
)

const checkQuery = ref('')

const filteredChecks = computed(() => {
  const query = checkQuery.value.trim().toLowerCase()
  if (!query) return props.checks
  return props.checks.filter(
    (c) =>
      (c.label || c.name).toLowerCase().includes(query) ||
      c.name.toLowerCase().includes(query),
  )
})

const groupedChecks = computed(() => {
  const groups = new Map<string, CheckInfo[]>()
  for (const check of filteredChecks.value) {
    const list = groups.get(check.category) ?? []
    list.push(check)
    groups.set(check.category, list)
  }
  return [...groups.entries()].map(([category, list]) => ({
    category,
    label: categoryLabel(category),
    checks: list,
  }))
})

function setChecks(names: string[]): void {
  model.value.check_names = names
}

function toggleCheck(checkName: string): void {
  const next = new Set(model.value.check_names)
  if (next.has(checkName)) next.delete(checkName)
  else next.add(checkName)
  setChecks([...next])
}

function toggleCategory(list: CheckInfo[]): void {
  const next = new Set(model.value.check_names)
  const allSelected = list.every((c) => next.has(c.name))
  for (const check of list) {
    if (allSelected) next.delete(check.name)
    else next.add(check.name)
  }
  setChecks([...next])
}

function selectAll(): void {
  setChecks(props.checks.map((c) => c.name))
}

function clearAll(): void {
  setChecks([])
}

function engineValue(key: NumericConfigKey): number {
  return model.value.config[key] as number
}

function setEngine(key: NumericConfigKey, value: number | null): void {
  model.value.config = {
    ...(model.value.config as EngineConfig),
    [key]: value ?? 0,
  }
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-4 sm:flex-row">
    <!-- Section nav -->
    <nav
      class="flex shrink-0 gap-1 overflow-x-auto pb-1 sm:w-56 sm:flex-col sm:overflow-visible sm:pb-0"
    >
      <button
        v-for="section in sections"
        :key="section.id"
        type="button"
        class="flex items-center gap-2 rounded px-3 py-2 text-left text-sm whitespace-nowrap transition-colors"
        :class="
          active === section.id
            ? 'bg-[var(--p-primary-color)] text-[var(--p-primary-contrast-color)]'
            : 'text-slate-600 hover:bg-slate-100'
        "
        @click="active = section.id"
      >
        <component :is="section.icon" :size="16" class="shrink-0" />
        <span class="flex-1">{{ section.label }}</span>
        <span v-if="section.id === 'checks'" class="text-xs opacity-80">
          {{ selected.size }}
        </span>
        <span
          v-if="section.id === 'workflow' && workflowInvalid"
          class="h-2 w-2 shrink-0 rounded-full bg-red-500"
          aria-label="No workflow enabled"
        />
      </button>
    </nav>

    <!-- Active section -->
    <div class="min-h-0 flex-1 overflow-y-auto">
      <div class="rounded-lg border border-slate-200 bg-white p-5">
        <div class="mb-5">
          <div class="text-sm font-semibold text-slate-800">
            {{ activeSection.label }}
          </div>
          <div class="text-xs text-slate-400">{{ activeSection.hint }}</div>
        </div>

        <!-- General -->
        <div v-if="active === 'general'" class="flex flex-col gap-4">
          <div>
            <label class="mb-1 block text-sm font-medium text-slate-600">Name</label>
            <InputText
              v-model="model.name"
              class="w-full"
              :disabled="disabled"
              placeholder="e.g. Marketing Site Audit"
            />
          </div>
          <div>
            <label class="mb-1 block text-sm font-medium text-slate-600">
              Description
            </label>
            <Textarea
              v-model="model.description"
              class="w-full"
              rows="3"
              auto-resize
              :disabled="disabled"
              placeholder="What is being audited and why?"
            />
          </div>
        </div>

        <!-- Targets -->
        <div v-else-if="active === 'targets'" class="flex flex-col gap-3">
          <div>
            <label class="mb-1 block text-sm font-medium text-slate-600">
              Target URLs (one per line)
            </label>
            <Textarea
              v-model="targetsText"
              class="w-full"
              rows="10"
              :disabled="disabled"
              placeholder="https://example.com&#10;https://example.com/pricing"
            />
          </div>
          <p class="text-xs text-slate-400">
            Each target is resolved (including its sitemap) before the run.
          </p>
        </div>

        <!-- Workflow -->
        <div v-else-if="active === 'workflow'" class="flex flex-col gap-4">
          <label class="flex items-center justify-between gap-4">
            <span>
              <span class="block text-sm text-slate-600">Run checks</span>
              <span class="block text-xs text-slate-400">
                Apply the selected checks to every page.
              </span>
            </span>
            <ToggleSwitch v-model="model.config.enable_checks" :disabled="disabled" />
          </label>

          <label class="flex items-center justify-between gap-4">
            <span>
              <span class="block text-sm text-slate-600">Build link graph</span>
              <span class="block text-xs text-slate-400">
                Extract nodes and edges for the graph views.
              </span>
            </span>
            <ToggleSwitch v-model="model.config.enable_graph" :disabled="disabled" />
          </label>

          <label class="flex items-center justify-between gap-4">
            <span>
              <span class="block text-sm text-slate-600">Validate link targets</span>
              <span class="block text-xs text-slate-400">
                Bulk-check the graph's targets once per TTL. Needs the link graph.
              </span>
            </span>
            <ToggleSwitch
              v-model="model.config.enable_link_validation"
              :disabled="disabled || !model.config.enable_graph"
            />
          </label>

          <div v-if="model.config.enable_link_validation">
            <label class="mb-1 block text-sm font-medium text-slate-600">
              Link validation TTL (min)
            </label>
            <InputNumber
              :model-value="engineValue('link_cache_ttl_min')"
              :min="1"
              :use-grouping="false"
              :disabled="disabled"
              show-buttons
              class="w-full sm:w-48"
              @update:model-value="setEngine('link_cache_ttl_min', $event as number | null)"
            />
            <p class="mt-1 text-xs text-slate-400">
              A target is only re-fetched once its stored result is older than this.
            </p>
          </div>

          <p v-if="workflowInvalid" class="text-xs text-red-500">
            Enable checks, the link graph, or both.
          </p>
        </div>

        <!-- Engine -->
        <div v-else-if="active === 'engine'" class="flex flex-col gap-4">
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div v-for="field in engineFields" :key="field.key">
              <label class="mb-1 block text-sm font-medium text-slate-600">
                {{ field.label }}
              </label>
              <InputNumber
                :model-value="engineValue(field.key)"
                :min="field.min"
                :use-grouping="false"
                :disabled="disabled"
                show-buttons
                class="w-full"
                @update:model-value="setEngine(field.key, $event as number | null)"
              />
            </div>
          </div>
          <div>
            <label class="mb-1 block text-sm font-medium text-slate-600">
              User agent
            </label>
            <InputText
              v-model="model.config.user_agent"
              class="w-full"
              :disabled="disabled"
            />
          </div>
        </div>

        <!-- Notifications -->
        <div v-else-if="active === 'notifications'" class="flex flex-col gap-4">
          <label class="flex items-center justify-between gap-4">
            <span class="text-sm text-slate-600">Enable notifications</span>
            <ToggleSwitch
              v-model="model.config.notifications_enabled"
              :disabled="disabled"
            />
          </label>

          <div>
            <label class="mb-1 block text-sm font-medium text-slate-600">
              Slack webhook URL
            </label>
            <InputText
              v-model="model.config.slack_webhook_url"
              class="w-full"
              :disabled="disabled || !model.config.notifications_enabled"
              placeholder="https://hooks.slack.com/services/…"
            />
          </div>

          <label class="flex items-center justify-between gap-4">
            <span class="text-sm text-slate-600">Notify after interface runs</span>
            <ToggleSwitch
              v-model="model.config.notify_on_direct"
              :disabled="disabled || !model.config.notifications_enabled"
            />
          </label>
          <label class="flex items-center justify-between gap-4">
            <span class="text-sm text-slate-600">Notify after scheduled runs</span>
            <ToggleSwitch
              v-model="model.config.notify_on_schedule"
              :disabled="disabled || !model.config.notifications_enabled"
            />
          </label>
        </div>

        <!-- Checks -->
        <div v-else class="flex flex-col">
          <div class="flex flex-wrap items-center gap-2">
            <div class="relative min-w-48 flex-1">
              <Search
                :size="14"
                class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-slate-400"
              />
              <InputText
                v-model="checkQuery"
                class="w-full pl-9"
                :disabled="disabled"
                placeholder="Search checks"
              />
            </div>
            <Button
              label="All"
              text
              size="small"
              :disabled="disabled"
              @click="selectAll"
            />
            <Button
              label="None"
              text
              size="small"
              :disabled="disabled"
              @click="clearAll"
            />
            <span class="text-xs text-slate-400">
              {{ selected.size }} of {{ checks.length }} selected
            </span>
          </div>

          <div class="mt-3 flex flex-col gap-3">
            <p
              v-if="!groupedChecks.length"
              class="py-6 text-center text-sm text-slate-400"
            >
              No checks match your search.
            </p>
            <div v-for="group in groupedChecks" :key="group.category">
              <button
                type="button"
                class="mb-1 flex w-full items-center justify-between text-left text-xs font-semibold tracking-wide text-slate-400 uppercase hover:text-slate-600"
                :disabled="disabled"
                @click="toggleCategory(group.checks)"
              >
                <span>{{ group.label }}</span>
                <span>
                  {{ group.checks.filter((c) => selected.has(c.name)).length }}/{{
                    group.checks.length
                  }}
                </span>
              </button>
              <div class="grid grid-cols-1 gap-1 lg:grid-cols-2">
                <label
                  v-for="check in group.checks"
                  :key="check.name"
                  class="flex cursor-pointer items-start gap-2 rounded px-1 py-1 hover:bg-slate-50"
                  :title="check.description"
                >
                  <Checkbox
                    :model-value="selected.has(check.name)"
                    binary
                    :disabled="disabled"
                    class="mt-0.5"
                    @update:model-value="toggleCheck(check.name)"
                  />
                  <span class="text-xs text-slate-600">
                    {{ check.label || check.name }}
                  </span>
                  <span
                    v-if="check.scope === 'graph'"
                    class="ml-1 rounded bg-slate-100 px-1 text-[10px] font-medium tracking-wide text-slate-500 uppercase"
                  >
                    graph
                  </span>
                </label>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
