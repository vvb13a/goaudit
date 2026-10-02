<script setup lang="ts">
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import * as api from '../api/client'
import { navigatorStorageKey } from '../lib/graph'
import { loadJSON, saveJSON } from '../lib/storage'

const props = defineProps<{ auditId: string }>()

const router = useRouter()
const error = ref<string | null>(null)
const empty = ref(false)

// resolveDefault picks the center node for a bare navigator visit: the last one
// the user looked at for this audit when it still exists, otherwise the first
// internal HTML document (preferring a root, then the best-connected page).
async function resolveDefault(): Promise<string | null> {
  const id = props.auditId
  const stored = loadJSON<string>(navigatorStorageKey(id))
  if (stored) {
    try {
      await api.getGraphNode(id, stored)
      return stored
    } catch {
      // Stale id (graph rebuilt): fall through to a fresh pick.
    }
  }

  let chosen = (
    await api.listGraphNodes(id, {
      page: 1,
      limit: 1,
      root: true,
      filetypes: ['html'],
      external: false,
    })
  ).items[0]
  if (!chosen) {
    chosen = (
      await api.listGraphNodes(id, {
        page: 1,
        limit: 1,
        filetypes: ['html'],
        external: false,
        sort: 'out_links',
        order: 'desc',
      })
    ).items[0]
  }
  if (!chosen) {
    chosen = (
      await api.listGraphNodes(id, { page: 1, limit: 1, sort: 'out_links', order: 'desc' })
    ).items[0]
  }
  return chosen?.id ?? null
}

onMounted(async () => {
  try {
    const nodeId = await resolveDefault()
    if (!nodeId) {
      empty.value = true
      return
    }
    saveJSON(navigatorStorageKey(props.auditId), nodeId)
    await router.replace({
      name: 'graph-navigator-node',
      params: { auditId: props.auditId, nodeId },
    })
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  }
})
</script>

<template>
  <div class="flex h-full items-center justify-center">
    <Message v-if="error" severity="error">{{ error }}</Message>
    <div v-else-if="empty" class="text-center text-slate-400">
      No graph data yet. Run this audit with the graph workflow enabled.
    </div>
    <ProgressSpinner v-else />
  </div>
</template>
