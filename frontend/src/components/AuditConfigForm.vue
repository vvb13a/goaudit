<script setup lang="ts">
import { Check, Times } from '@primeicons/vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { computed } from 'vue'
import { categoryLabel } from '../lib/theme'
import type { AuditConfig, CheckInfo, EngineConfig } from '../types'

const model = defineModel<AuditConfig>({ required: true })

const props = defineProps<{
  checks: CheckInfo[]
  disabled?: boolean
}>()

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
  { key: 'link_cache_ttl_min', label: 'Link cache TTL (min)', min: 1 },
]

const targetsText = computed({
  get: () => model.value.targets.join('\n'),
  set: (value: string) => {
    model.value.targets = value.split('\n')
  },
})

const selected = computed(() => new Set(model.value.check_names))

const groupedChecks = computed(() => {
  const groups = new Map<string, CheckInfo[]>()
  for (const check of props.checks) {
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
  if (next.has(checkName)) {
    next.delete(checkName)
  } else {
    next.add(checkName)
  }
  setChecks([...next])
}

function toggleCategory(list: CheckInfo[]): void {
  const next = new Set(model.value.check_names)
  const allSelected = list.every((c) => next.has(c.name))
  for (const check of list) {
    if (allSelected) {
      next.delete(check.name)
    } else {
      next.add(check.name)
    }
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
  <div class="grid h-full min-h-0 grid-cols-1 gap-4 lg:grid-cols-5">
    <div class="min-h-0 overflow-y-auto rounded-lg border border-slate-200 bg-white p-4 lg:col-span-3">
      <div class="flex flex-col gap-4">
        <div>
          <label class="mb-1 block text-sm font-medium text-slate-600">
            Name
          </label>
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

        <div>
          <label class="mb-1 block text-sm font-medium text-slate-600">
            Target URLs (one per line)
          </label>
          <Textarea
            v-model="targetsText"
            class="w-full"
            rows="5"
            :disabled="disabled"
            placeholder="https://example.com&#10;https://example.com/pricing"
          />
        </div>

        <div>
          <div class="mb-2 text-sm font-semibold text-slate-700">Workflow</div>
          <label class="flex items-center justify-between gap-4">
            <span class="text-sm text-slate-600">Run checks</span>
            <ToggleSwitch
              v-model="model.config.enable_checks"
              :disabled="disabled"
            />
          </label>
          <label class="mt-2 flex items-center justify-between gap-4">
            <span class="text-sm text-slate-600">Build link graph</span>
            <ToggleSwitch
              v-model="model.config.enable_graph"
              :disabled="disabled"
            />
          </label>
          <p
            v-if="!model.config.enable_checks && !model.config.enable_graph"
            class="mt-2 text-xs text-red-500"
          >
            Enable checks, the link graph, or both.
          </p>
        </div>

        <div>
          <div class="mb-2 text-sm font-semibold text-slate-700">
            Engine Configuration
          </div>
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
          <div class="mt-3">
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

        <div>
          <div class="mb-2 text-sm font-semibold text-slate-700">
            Notifications
          </div>
          <label class="flex items-center justify-between gap-4">
            <span class="text-sm text-slate-600">Enable notifications</span>
            <ToggleSwitch
              v-model="model.config.notifications_enabled"
              :disabled="disabled"
            />
          </label>

          <div class="mt-3">
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

          <label class="mt-3 flex items-center justify-between gap-4">
            <span class="text-sm text-slate-600">
              Notify after interface runs
            </span>
            <ToggleSwitch
              v-model="model.config.notify_on_direct"
              :disabled="disabled || !model.config.notifications_enabled"
            />
          </label>
          <label class="mt-2 flex items-center justify-between gap-4">
            <span class="text-sm text-slate-600">
              Notify after scheduled runs
            </span>
            <ToggleSwitch
              v-model="model.config.notify_on_schedule"
              :disabled="disabled || !model.config.notifications_enabled"
            />
          </label>
        </div>
      </div>
    </div>

    <div
      class="flex min-h-0 flex-col rounded-lg border border-slate-200 bg-white p-4 lg:col-span-2"
      :class="{ 'pointer-events-none opacity-50': !model.config.enable_checks }"
    >
      <div class="flex items-center justify-between">
        <div class="text-sm font-semibold text-slate-700">Running Checks</div>
        <div class="text-xs text-slate-400">
          {{ selected.size }} of {{ checks.length }} selected
        </div>
      </div>
      <div class="mt-2 flex items-center gap-1">
        <Button label="All" text size="small" :disabled="disabled" @click="selectAll">
          <template #icon><Check :size="16" /></template>
        </Button>
        <Button label="None" text size="small" :disabled="disabled" @click="clearAll">
          <template #icon><Times :size="16" /></template>
        </Button>
      </div>

      <div class="mt-2 min-h-0 flex-1 overflow-y-auto pr-1">
        <p
          v-if="!checks.length"
          class="py-4 text-center text-sm text-slate-400"
        >
          No checks available.
        </p>
        <div v-for="group in groupedChecks" :key="group.category" class="mb-3">
          <button
            type="button"
            class="mb-1 flex w-full items-center justify-between text-left text-xs font-semibold tracking-wide text-slate-400 uppercase hover:text-slate-600"
            :disabled="disabled"
            @click="toggleCategory(group.checks)"
          >
            <span>{{ group.label }}</span>
            <span>{{ group.checks.filter((c) => selected.has(c.name)).length }}/{{ group.checks.length }}</span>
          </button>
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
            <span class="text-xs text-slate-600">{{ check.label || check.name }}</span>
          </label>
        </div>
      </div>
    </div>
  </div>
</template>
