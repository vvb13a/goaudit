<script setup lang="ts">
import Tag from 'primevue/tag'
import { computed } from 'vue'
import { severityColor } from '../lib/theme'

const props = withDefaults(defineProps<{ severity?: string }>(), {
  severity: '',
})

const label = computed(() => {
  const s = props.severity
  return s ? s.charAt(0).toUpperCase() + s.slice(1).toLowerCase() : ''
})

// Tags painted with the app palette so they match the charts and dots, with
// white text, softened to 80% opacity so they do not shout.
const style = computed(() => {
  const color = severityColor(props.severity)
  return {
    backgroundColor: color,
    borderColor: color,
    color: '#ffffff',
    opacity: 0.8,
  }
})
</script>

<template>
  <Tag :value="label" :style="style" />
</template>
