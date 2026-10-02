<script setup lang="ts">
import { FilterSlash } from '@primeicons/vue'
import Button from 'primevue/button'

withDefaults(defineProps<{ message?: string }>(), {
  message: 'No results found.',
})

const emit = defineEmits<{ reset: [] }>()
</script>

<template>
  <div class="flex flex-col items-center justify-center gap-3 px-6 py-16 text-center">
    <p class="text-sm text-slate-500">{{ message }}</p>
    <Button label="Reset filters" text size="small" @click="emit('reset')">
      <template #icon><FilterSlash :size="16" /></template>
    </Button>
  </div>
</template>
