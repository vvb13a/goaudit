<script setup lang="ts">
import { computed } from 'vue'
import { palette } from '../lib/theme'

export interface Bar {
  label: string
  value: number
  color?: string
}

const props = withDefaults(
  defineProps<{
    bars: Bar[]
    color?: string
    format?: (value: number) => string
  }>(),
  {
    color: palette.primary,
    format: (value: number) => String(value),
  },
)

const max = computed(() => Math.max(1, ...props.bars.map((bar) => bar.value)))

function barHeight(value: number): string {
  return `${Math.max(3, Math.round((value / max.value) * 100))}%`
}
</script>

<template>
  <div class="flex h-24 gap-2">
    <div
      v-for="(bar, index) in bars"
      :key="index"
      class="flex min-w-0 flex-1 flex-col items-center gap-1"
    >
      <span class="text-[10px] font-medium tabular-nums text-slate-500">
        {{ format(bar.value) }}
      </span>
      <div class="flex w-full flex-1 items-end">
        <div
          class="w-full rounded-t"
          :style="{
            height: barHeight(bar.value),
            backgroundColor: bar.color ?? color,
          }"
        />
      </div>
      <span
        class="block h-6 w-full whitespace-pre-line text-center text-[10px] leading-[1.05] text-slate-400"
      >
        {{ bar.label }}
      </span>
    </div>
  </div>
</template>
