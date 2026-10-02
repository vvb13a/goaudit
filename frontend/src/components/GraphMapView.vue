<script setup lang="ts">
import { ExternalLink, Search, Sitemap, Times } from '@primeicons/vue'
import ForceGraph from 'force-graph'
import type { LinkObject, NodeObject } from 'force-graph'
import Button from 'primevue/button'
import ContextMenu from 'primevue/contextmenu'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import ProgressSpinner from 'primevue/progressspinner'
import type { MenuItem } from 'primevue/menuitem'
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import * as api from '../api/client'
import { dataVersion } from '../lib/appState'
import { isDark } from '../lib/colorScheme'
import { filetypeColor } from '../lib/graph'
import type { GraphEdgeRow } from '../types'

const props = defineProps<{ auditId: string }>()

const router = useRouter()
const route = useRoute()

// Relation ring colors, chosen to contrast with the saturated filetype fills
// (an html node is blue, an image node is green, ...): the ring is separated
// from the fill by a white gap and glow, so these read on any node color.
const RING_FOCUS = '#7c3aed'
const RING_IN = '#db2777'
const RING_OUT = '#f97316'

interface VizNode extends NodeObject {
  id: string
  url: string
  filetype: string
  external: boolean
  is_root: boolean
  degree: number
  inLinks: number
  outLinks: number
}

interface VizLink extends LinkObject<VizNode> {
  type: string
}

const container = ref<HTMLElement | null>(null)
const graph = shallowRef<ForceGraph<VizNode, VizLink> | null>(null)
const loading = ref(true)
const error = ref<string | null>(null)
const truncated = ref(false)
const empty = ref(false)
const query = ref('')
const legend = ref<{ filetype: string; count: number }[]>([])
const selectedFiletypes = ref<string[]>([])
const focusId = ref<string | null>(null)
const nodesById = shallowRef<Map<string, VizNode>>(new Map())
const rawEdges = ref<GraphEdgeRow[]>([])

const menu = ref<InstanceType<typeof ContextMenu>>()
const menuNode = ref<VizNode | null>(null)

let observer: ResizeObserver | null = null
let request = 0

const filetypeOptions = computed(() =>
  legend.value.map((item) => ({
    label: ucfirst(item.filetype),
    value: item.filetype,
  })),
)

const focusNode = computed<VizNode | null>(() =>
  focusId.value ? nodesById.value.get(focusId.value) ?? null : null,
)

const incomingSet = computed(() => {
  const set = new Set<string>()
  const focus = focusId.value
  if (!focus) return set
  for (const edge of rawEdges.value) {
    if (edge.target_node_id === focus) set.add(edge.source_node_id)
  }
  return set
})

const outgoingSet = computed(() => {
  const set = new Set<string>()
  const focus = focusId.value
  if (!focus) return set
  for (const edge of rawEdges.value) {
    if (edge.source_node_id === focus) set.add(edge.target_node_id)
  }
  return set
})

const menuItems = computed<MenuItem[]>(() => {
  const items: MenuItem[] = [
    {
      label: 'Focus node',
      icon: Search,
      command: () => menuNode.value && focusNodeById(menuNode.value.id),
    },
    {
      label: 'Open in navigator',
      icon: Sitemap,
      command: () => menuNode.value && openNavigator(menuNode.value.id),
    },
    {
      label: 'Open link',
      icon: ExternalLink,
      command: () =>
        menuNode.value &&
        window.open(menuNode.value.url, '_blank', 'noopener,noreferrer'),
    },
  ]
  if (focusId.value) {
    items.push(
      { separator: true },
      {
        label: 'Clear focus',
        icon: Times,
        command: () => {
          focusId.value = null
        },
      },
    )
  }
  return items
})

function ucfirst(value: string): string {
  return value ? value.charAt(0).toUpperCase() + value.slice(1) : value
}

function shortUrl(url: string): string {
  return url.replace(/^https?:\/\//, '')
}

function toggleFiletype(filetype: string): void {
  const next = new Set(selectedFiletypes.value)
  if (next.has(filetype)) next.delete(filetype)
  else next.add(filetype)
  selectedFiletypes.value = [...next]
}

function isSelected(filetype: string): boolean {
  return selectedFiletypes.value.includes(filetype)
}

function matches(node: VizNode): boolean {
  if (selectedFiletypes.value.length && !selectedFiletypes.value.includes(node.filetype)) {
    return false
  }
  const q = query.value.trim().toLowerCase()
  if (!q) return true
  return node.url.toLowerCase().includes(q) || node.filetype.includes(q)
}

function focusNodeById(nodeId: string | null): void {
  if (!nodeId) return
  focusId.value = focusId.value === nodeId ? null : nodeId
}

function openNavigator(nodeId: string): void {
  router.push({
    name: 'graph-navigator-node',
    params: { auditId: props.auditId, nodeId },
  })
}

function openMenu(node: VizNode, event: MouseEvent): void {
  event.preventDefault()
  menuNode.value = node
  menu.value?.show(event)
}

// relationOf classifies a node relative to the focused node: the focus itself,
// an incoming neighbour (points at it), an outgoing neighbour (it points to),
// both, or unrelated.
function relationOf(node: VizNode): 'focus' | 'in' | 'out' | 'both' | 'other' {
  const focus = focusId.value
  if (!focus) return 'other'
  if (node.id === focus) return 'focus'
  const incoming = incomingSet.value.has(node.id)
  const outgoing = outgoingSet.value.has(node.id)
  if (incoming && outgoing) return 'both'
  if (incoming) return 'in'
  if (outgoing) return 'out'
  return 'other'
}

function endpointId(value: string | number | VizNode | undefined): string {
  return typeof value === 'object' && value !== null ? value.id : String(value)
}

function ringColor(relation: ReturnType<typeof relationOf>): string {
  switch (relation) {
    case 'in':
      return RING_IN
    case 'out':
      return RING_OUT
    default:
      return RING_FOCUS
  }
}

// The force-graph canvas is drawn by hand, so it does not pick up the theme
// from CSS; these helpers keep it in step with the color scheme.
function canvasBackground(): string {
  return isDark.value ? '#18181b' : '#ffffff'
}

function mutedNodeFill(): string {
  return isDark.value ? '#3f3f46' : '#cbd5e1'
}

function nodeOutline(): string {
  return isDark.value ? 'rgba(255,255,255,0.25)' : 'rgba(15,23,42,0.25)'
}

function labelColor(): string {
  return isDark.value ? '#e2e8f0' : '#0f172a'
}

function drawNode(
  node: VizNode,
  ctx: CanvasRenderingContext2D,
  globalScale: number,
): void {
  const x = node.x ?? 0
  const y = node.y ?? 0
  const radius = 2 + Math.sqrt(node.degree)
  const relation = relationOf(node)
  const related = !!focusId.value && relation !== 'other'

  let fill = filetypeColor(node.filetype)
  if (focusId.value) {
    if (relation === 'other') fill = mutedNodeFill()
  } else if (!matches(node)) {
    fill = 'rgba(148,163,184,0.35)'
  }

  const discRadius = radius + 6
  const ringRadius = radius + 3

  // Related nodes get a colored glow plus a white disc under the node, so the
  // ring is separated from both the fill and the background and stays visible
  // whatever the filetype color.
  if (related) {
    ctx.save()
    ctx.shadowColor = relation === 'both' ? RING_OUT : ringColor(relation)
    ctx.shadowBlur = 12
    ctx.beginPath()
    ctx.arc(x, y, radius, 0, Math.PI * 2)
    ctx.fillStyle = fill
    ctx.fill()
    ctx.restore()

    ctx.beginPath()
    ctx.arc(x, y, discRadius, 0, Math.PI * 2)
    ctx.fillStyle = canvasBackground()
    ctx.fill()
  }

  ctx.beginPath()
  ctx.arc(x, y, radius, 0, Math.PI * 2)
  ctx.fillStyle = fill
  ctx.fill()
  ctx.lineWidth = 0.6
  ctx.strokeStyle = nodeOutline()
  ctx.stroke()

  if (related) {
    ctx.lineWidth = relation === 'focus' ? 3 : 2
    if (relation === 'both') {
      ctx.beginPath()
      ctx.arc(x, y, ringRadius, Math.PI, Math.PI * 2)
      ctx.strokeStyle = RING_IN
      ctx.stroke()
      ctx.beginPath()
      ctx.arc(x, y, ringRadius, 0, Math.PI)
      ctx.strokeStyle = RING_OUT
      ctx.stroke()
    } else {
      ctx.beginPath()
      ctx.arc(x, y, ringRadius, 0, Math.PI * 2)
      ctx.strokeStyle = ringColor(relation)
      ctx.stroke()
    }
  }

  const showLabel = relation === 'focus' || related || globalScale > 3
  if (showLabel) {
    ctx.font = `${10 / globalScale}px Sans-Serif`
    ctx.textAlign = 'center'
    ctx.textBaseline = 'top'
    ctx.fillStyle = labelColor()
    ctx.fillText(shortUrl(node.url), x, y + (related ? discRadius : radius) + 1)
  }
}

function paintPointer(
  node: VizNode,
  color: string,
  ctx: CanvasRenderingContext2D,
): void {
  const x = node.x ?? 0
  const y = node.y ?? 0
  const radius = 2 + Math.sqrt(node.degree) + 2
  ctx.fillStyle = color
  ctx.beginPath()
  ctx.arc(x, y, radius, 0, Math.PI * 2)
  ctx.fill()
}

function linkColor(link: VizLink): string {
  const focus = focusId.value
  if (!focus) return 'rgba(100,116,139,0.18)'
  const source = endpointId(link.source)
  const target = endpointId(link.target)
  if (target === focus) return 'rgba(219,39,119,0.75)'
  if (source === focus) return 'rgba(249,115,22,0.75)'
  return 'rgba(148,163,184,0.06)'
}

// refreshDraw reseats the accessors with fresh closures and pokes the render
// loop. Re-seating marks the graph dirty, but while the engine is idle no frame
// is scheduled, so the redraw (and the offscreen pointer-area repaint that
// hit-testing depends on) would not happen. resumeAnimation processes the dirty
// flag, after which the loop auto-pauses again. It must not be paired with
// pauseAnimation: that loop also drives the d3 engine, and cancelling it breaks
// dragging and subsequent clicks.
function refreshDraw(): void {
  const g = graph.value
  if (!g) return
  g.nodeCanvasObject((n, c, s) => drawNode(n, c, s))
    .nodePointerAreaPaint((n, c, ctx) => paintPointer(n, c, ctx))
    .linkColor((l) => linkColor(l))
  g.resumeAnimation()
}

async function load(): Promise<void> {
  const id = props.auditId
  const g = graph.value
  if (!id || !g) return
  const current = ++request
  loading.value = true
  error.value = null
  try {
    const data = await api.getGraphData(id)
    if (current !== request) return

    const byId = new Map<string, VizNode>()
    for (const n of data.nodes) {
      byId.set(n.id, {
        id: n.id,
        url: n.url,
        filetype: n.filetype,
        external: n.external,
        is_root: n.is_root,
        degree: n.in_links + n.out_links,
        inLinks: n.in_links,
        outLinks: n.out_links,
      })
    }
    nodesById.value = byId
    rawEdges.value = data.edges
    if (focusId.value && !byId.has(focusId.value)) focusId.value = null

    const links: VizLink[] = []
    for (const e of data.edges) {
      if (!byId.has(e.source_node_id) || !byId.has(e.target_node_id)) continue
      links.push({ source: e.source_node_id, target: e.target_node_id, type: e.type })
    }

    const counts = new Map<string, number>()
    for (const n of byId.values()) {
      counts.set(n.filetype, (counts.get(n.filetype) ?? 0) + 1)
    }
    legend.value = [...counts.entries()]
      .map(([filetype, count]) => ({ filetype, count }))
      .sort((a, b) => b.count - a.count)

    truncated.value = data.truncated
    empty.value = byId.size === 0
    g.graphData({ nodes: [...byId.values()], links })
    if (!empty.value) {
      window.setTimeout(fit, 400)
    }
  } catch (e) {
    if (current === request) error.value = e instanceof Error ? e.message : String(e)
  } finally {
    if (current === request) loading.value = false
  }
}

function fit(): void {
  graph.value?.zoomToFit(500, 40)
}

onMounted(() => {
  const el = container.value
  if (!el) return

  const g = new ForceGraph<VizNode, VizLink>(el)
  g.backgroundColor(canvasBackground())
    .nodeId('id')
    .nodeLabel((n) => n.url)
    .linkWidth(0.5)
    .linkDirectionalArrowLength(2.5)
    .linkDirectionalArrowRelPos(1)
    .cooldownTicks(120)
    .nodeCanvasObject((n, c, s) => drawNode(n, c, s))
    .nodePointerAreaPaint((n, c, ctx) => paintPointer(n, c, ctx))
    .linkColor((l) => linkColor(l))
    .onNodeClick((n) => focusNodeById(n.id))
    .onNodeRightClick((n, event) => openMenu(n, event))
    .onBackgroundClick(() => {
      focusId.value = null
    })

  const resize = (): void => {
    if (!container.value) return
    g.width(container.value.clientWidth).height(container.value.clientHeight)
  }
  resize()
  observer = new ResizeObserver(resize)
  observer.observe(el)

  graph.value = g
  void load()
})

onBeforeUnmount(() => {
  request++
  observer?.disconnect()
  observer = null
  graph.value?._destructor()
  graph.value = null
})

watch([query, selectedFiletypes, focusId], refreshDraw)

// Repaint the canvas background and node colors when the scheme flips.
watch(isDark, () => {
  graph.value?.backgroundColor(canvasBackground())
  refreshDraw()
})

// The focused node lives in the URL (?focus=<id>) so a focus is shareable and
// survives a reload; it is kept in sync in both directions.
watch(
  () => route.query.focus,
  (value) => {
    const id = typeof value === 'string' && value ? value : null
    if (id !== focusId.value) focusId.value = id
  },
  { immediate: true },
)

watch(focusId, (value) => {
  const current = typeof route.query.focus === 'string' ? route.query.focus : null
  if (value === current) return
  const next: Record<string, string> = {}
  for (const [key, raw] of Object.entries(route.query)) {
    if (key === 'focus') continue
    if (typeof raw === 'string') next[key] = raw
    else if (Array.isArray(raw) && typeof raw[0] === 'string') next[key] = raw[0]
  }
  if (value) next.focus = value
  router.replace({ query: next })
})

watch(dataVersion, () => {
  load()
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-3">
    <Message v-if="error" severity="error">{{ error }}</Message>

    <div class="flex flex-wrap items-center gap-2">
      <InputText
        v-model="query"
        placeholder="Highlight URL or filetype"
        class="w-64"
      />
      <MultiSelect
        v-model="selectedFiletypes"
        :options="filetypeOptions"
        option-label="label"
        option-value="value"
        placeholder="Any filetype"
        :show-clear="true"
        :max-selected-labels="3"
        class="w-56"
      />
      <Button label="Fit" text size="small" @click="fit" />
      <Button
        label="Refresh"
        text
        size="small"
        :loading="loading"
        @click="load"
      />
      <div class="flex items-center gap-3 text-xs text-slate-500">
        <span class="inline-flex items-center gap-1">
          <span class="h-3 w-3 rounded-full border-2" style="border-color: #7c3aed" />
          Focus
        </span>
        <span class="inline-flex items-center gap-1">
          <span class="h-3 w-3 rounded-full border-2" style="border-color: #db2777" />
          Incoming
        </span>
        <span class="inline-flex items-center gap-1">
          <span class="h-3 w-3 rounded-full border-2" style="border-color: #f97316" />
          Outgoing
        </span>
      </div>
      <span class="flex-1" />
      <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
        <button
          v-for="item in legend"
          :key="item.filetype"
          type="button"
          class="inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-xs transition-colors"
          :class="
            isSelected(item.filetype)
              ? 'border-[var(--p-primary-color)] text-slate-700'
              : 'border-transparent text-slate-500 hover:bg-slate-100'
          "
          @click="toggleFiletype(item.filetype)"
        >
          <span
            class="h-2.5 w-2.5 rounded-full"
            :style="{ backgroundColor: filetypeColor(item.filetype) }"
          />
          {{ ucfirst(item.filetype) }}
          <span class="text-slate-400">{{ item.count.toLocaleString() }}</span>
        </button>
      </div>
    </div>

    <div v-if="focusNode" class="flex flex-wrap items-center gap-2 text-xs">
      <span class="font-medium text-slate-600">Focused:</span>
      <a
        :href="focusNode.url"
        target="_blank"
        rel="noopener noreferrer"
        class="break-all text-[var(--p-primary-color)] hover:underline"
      >
        {{ focusNode.url }}
      </a>
      <span class="text-slate-400">
        {{ focusNode.inLinks.toLocaleString() }} in ·
        {{ focusNode.outLinks.toLocaleString() }} out
      </span>
      <Button label="Clear focus" text size="small" @click="focusId = null" />
    </div>

    <p v-if="truncated" class="text-xs text-amber-600">
      Showing a capped subset of the graph (first 2000 nodes/edges).
    </p>

    <div
      class="relative min-h-0 flex-1 overflow-hidden rounded-lg border border-slate-200 bg-white"
    >
      <div ref="container" class="h-full w-full"></div>
      <div
        v-if="loading"
        class="absolute inset-0 flex items-center justify-center bg-white/60"
      >
        <ProgressSpinner />
      </div>
      <div
        v-else-if="empty"
        class="absolute inset-0 flex items-center justify-center text-center text-slate-400"
      >
        No graph data yet. Run this audit with the graph workflow enabled.
      </div>
    </div>

    <ContextMenu ref="menu" :model="menuItems" @hide="menuNode = null" />
  </div>
</template>
