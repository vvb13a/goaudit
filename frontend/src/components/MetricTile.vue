<script setup lang="ts">
import Tag from 'primevue/tag'
import { palette } from '../lib/theme'

const props = withDefaults(
  defineProps<{
    label: string
    value: string
    color?: string
    delta?: string
    moreIsGood?: boolean
  }>(),
  {
    color: palette.info,
    delta: '',
    moreIsGood: false,
  },
)

function deltaIsGood(): boolean {
  const negative = props.delta.startsWith('-')
  // A decrease is good unless the metric is one where more is better, in
  // which case an increase is good.
  return props.moreIsGood ? !negative : negative
}
</script>

<template>
  <div class="rounded-lg border border-slate-200 bg-white p-3">
    <div class="text-xs font-medium tracking-wide text-slate-500 uppercase">
      {{ label }}
    </div>
    <div class="mt-1 flex items-end gap-2">
      <span
        class="text-2xl font-semibold tabular-nums"
        :style="{ color }"
      >
        {{ value }}
      </span>
      <Tag
        v-if="delta"
        :value="delta"
        :severity="deltaIsGood() ? 'success' : 'danger'"
        class="px-1.5! py-1! text-[10px]! leading-none!"
      />
    </div>
  </div>
</template>
