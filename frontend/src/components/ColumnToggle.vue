<script setup lang="ts">
import { Bars, Cog } from '@primeicons/vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Popover from 'primevue/popover'
import { ref, useId } from 'vue'

const props = defineProps<{
  order: string[]
  visible: string[]
  labels: Record<string, string>
  disabled?: boolean
}>()

const emit = defineEmits<{
  'update:order': [order: string[]]
  'update:visible': [visible: string[]]
  reset: []
}>()

// A per-instance prefix keeps checkbox ids unique when a page mounts more than
// one table (e.g. the timeline's runs and graph sections).
const idPrefix = useId()
const checkboxId = (field: string): string => `${idPrefix}-${field}`

const popover = ref<InstanceType<typeof Popover>>()
const dragIndex = ref<number | null>(null)
const dragOverIndex = ref<number | null>(null)

function toggle(event: Event): void {
  popover.value?.toggle(event)
}

function isVisible(field: string): boolean {
  return props.visible.includes(field)
}

function setVisible(field: string, value: boolean): void {
  if (value === isVisible(field)) return
  emit(
    'update:visible',
    value ? [...props.visible, field] : props.visible.filter((f) => f !== field),
  )
}

function onDragStart(index: number): void {
  dragOverIndex.value = index
  requestAnimationFrame(() => {
    dragIndex.value = index
  })
}

function onDragOver(index: number): void {
  if (index !== dragIndex.value) dragOverIndex.value = index
}

function onDragEnd(): void {
  dragIndex.value = null
  dragOverIndex.value = null
}

function onDrop(): void {
  const from = dragIndex.value
  const to = dragOverIndex.value
  if (from !== null && to !== null && from !== to) {
    const next = [...props.order]
    const [moved] = next.splice(from, 1)
    next.splice(to, 0, moved)
    emit('update:order', next)
  }
  onDragEnd()
}
</script>

<template>
  <Button
    label="Columns"
    text
    size="small"
    aria-label="Toggle columns"
    :disabled="disabled"
    @click="toggle"
  >
    <template #icon><Cog :size="16" /></template>
  </Button>

  <Popover ref="popover" class="w-56" pt:content="p-0!">
    <div
      class="flex items-center justify-between gap-2 border-b border-slate-200 px-3 py-2"
    >
      <span class="text-sm font-semibold text-slate-700">Columns</span>
      <Button label="Reset" text size="small" @click="emit('reset')" />
    </div>
    <div
      class="max-h-80 overflow-auto py-1"
      @dragover.prevent
      @drop="onDrop"
    >
      <div
        v-for="(field, index) in order"
        :key="field"
        class="mx-1 flex cursor-move items-center gap-2 rounded-md px-2 py-1.5 transition select-none"
        :class="[
          dragIndex === index ? 'opacity-40' : '',
          dragOverIndex === index && dragIndex !== index
            ? 'bg-slate-100 ring-1 ring-slate-300'
            : 'hover:bg-slate-100',
        ]"
        draggable="true"
        @dragstart="onDragStart(index)"
        @dragover.prevent="onDragOver(index)"
        @dragend="onDragEnd"
      >
        <span class="flex text-slate-400"><Bars :size="14" /></span>
        <Checkbox
          :model-value="isVisible(field)"
          binary
          :input-id="checkboxId(field)"
          @update:model-value="setVisible(field, $event as boolean)"
        />
        <label
          :for="checkboxId(field)"
          class="cursor-pointer text-sm text-slate-700"
        >
          {{ labels[field] ?? field }}
        </label>
      </div>
    </div>
  </Popover>
</template>
