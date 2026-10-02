<script setup lang="ts">
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import * as api from '../api/client'
import { filetypeClass, navigatorStorageKey } from '../lib/graph'
import { saveJSON } from '../lib/storage'
import type { GraphNode } from '../types'
import GraphEdgeList from './GraphEdgeList.vue'

const props = defineProps<{ auditId: string; nodeId: string }>()

const route = useRoute()
const router = useRouter()

const node = ref<GraphNode | null>(null)
const loading = ref(true)
const error = ref<string | null>(null)
let request = 0

async function loadCenter(nodeId: string): Promise<void> {
  const id = props.auditId
  if (!id || !nodeId) return
  const current = ++request
  loading.value = true
  error.value = null
  try {
    const found = await api.getGraphNode(id, nodeId)
    if (current !== request) return
    node.value = found
    // Remember the center so a bare navigator visit resumes here.
    saveJSON(navigatorStorageKey(id), found.id)
  } catch (e) {
    if (current === request) {
      error.value = e instanceof Error ? e.message : String(e)
      node.value = null
    }
  } finally {
    if (current === request) loading.value = false
  }
}

watch(() => props.nodeId, (value) => loadCenter(value), { immediate: true })

// selectNode moves the center. The center is the route param, so this is a
// navigation; the current filters/sort/page in the query are preserved.
function selectNode(nodeId: string): void {
  if (!nodeId || nodeId === props.nodeId) return
  router.push({
    name: 'graph-navigator-node',
    params: { auditId: props.auditId, nodeId },
    query: { ...route.query },
  })
}

function scopeLabel(external: boolean): string {
  return external ? 'external' : 'internal'
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-4">
    <Message v-if="error" severity="error">{{ error }}</Message>

    <div v-if="loading && !node" class="flex flex-1 items-center justify-center">
      <ProgressSpinner />
    </div>

    <template v-else-if="node">
      <div class="rounded-lg border border-slate-200 bg-white p-4">
        <div class="flex flex-wrap items-center gap-3">
          <span
            class="shrink-0 rounded px-2 py-0.5 text-xs font-medium"
            :class="filetypeClass(node.filetype)"
          >
            {{ node.filetype }}
          </span>
          <span
            v-if="node.is_root"
            class="shrink-0 rounded bg-[var(--p-primary-color)] px-1.5 py-0.5 text-[10px] font-semibold text-[var(--p-primary-contrast-color)] uppercase"
          >
            root
          </span>
          <span
            class="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase"
            :class="
              node.external
                ? 'bg-slate-200 text-slate-600'
                : 'bg-emerald-100 text-emerald-700'
            "
          >
            {{ scopeLabel(node.external) }}
          </span>
          <a
            :href="node.url"
            target="_blank"
            rel="noopener noreferrer"
            class="break-all text-sm font-medium text-[var(--p-primary-color)] hover:underline"
          >
            {{ node.url }}
          </a>
        </div>
        <div class="mt-2 flex flex-wrap items-center gap-4 text-xs text-slate-500">
          <span>{{ node.in_links.toLocaleString() }} incoming</span>
          <span>{{ node.out_links.toLocaleString() }} outgoing</span>
        </div>
      </div>

      <div class="grid min-h-0 flex-1 grid-cols-1 gap-4 lg:grid-cols-2">
        <div class="flex min-h-0 flex-col">
          <div class="mb-2 text-xs font-semibold tracking-wide text-slate-400 uppercase">
            Pointed to by
          </div>
          <GraphEdgeList
            :key="`in-${node.id}`"
            :audit-id="auditId"
            :center-id="node.id"
            direction="in"
            prefix="in"
            @navigate="selectNode"
          />
        </div>
        <div class="flex min-h-0 flex-col">
          <div class="mb-2 text-xs font-semibold tracking-wide text-slate-400 uppercase">
            Points to
          </div>
          <GraphEdgeList
            :key="`out-${node.id}`"
            :audit-id="auditId"
            :center-id="node.id"
            direction="out"
            prefix="out"
            @navigate="selectNode"
          />
        </div>
      </div>
    </template>

    <div
      v-else
      class="flex flex-1 items-center justify-center text-center text-slate-400"
    >
      No graph data yet. Run this audit with the graph workflow enabled.
    </div>
  </div>
</template>
