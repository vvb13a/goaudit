import { computed, ref } from 'vue'
import * as api from '../api/client'
import type { CheckInfo } from '../types'

// The check registry is static code metadata, not audit data. Fetch it once
// and cache it so every view can resolve a check's human-readable label and
// description without storing either in the database.
const registry = ref<CheckInfo[]>([])
let loadPromise: Promise<void> | null = null

const byName = computed(() => {
  const map = new Map<string, CheckInfo>()
  for (const check of registry.value) {
    map.set(check.name, check)
  }
  return map
})

function ensureLoaded(): void {
  if (registry.value.length || loadPromise) return
  loadPromise = api
    .listChecks()
    .then((data) => {
      registry.value = data
    })
    .catch(() => {
      // Best effort: callers fall back to the raw check name.
    })
    .finally(() => {
      loadPromise = null
    })
}

export function useCheckMeta() {
  ensureLoaded()

  function label(name: string): string {
    return byName.value.get(name)?.label || name
  }

  function description(name: string): string {
    return byName.value.get(name)?.description || ''
  }

  return { registry, label, description }
}
