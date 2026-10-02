<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { navEntries, routeMatches, type NavChild } from '../lib/navigation'
import type { AuditCounts } from '../types'

const props = defineProps<{ counts?: AuditCounts | null }>()

const route = useRoute()

const auditId = computed(() => (route.params.auditId as string | undefined) ?? null)
const routeName = computed(() => route.name as string | undefined)

function target(name: string) {
  return auditId.value
    ? { name, params: { auditId: auditId.value } }
    : { name: 'audits' }
}

function isActive(name: string): boolean {
  return routeMatches(routeName.value, name)
}

function groupActive(children: NavChild[]): boolean {
  return children.some((child) => isActive(child.route))
}

const baseClass = 'flex items-center gap-2 rounded py-1.5 text-sm transition-colors'

function itemClass(name: string): string {
  return isActive(name) ? 'nav-active' : 'text-slate-600 hover:bg-slate-100'
}

// Badges are shown for the item views that have a row count.
function countFor(id: string): number | null {
  const c = props.counts
  if (!c) return null
  switch (id) {
    case 'urls':
      return c.urls
    case 'issues':
      return c.issues
    case 'checks':
      return c.checks
    case 'graph-nodes':
      return c.nodes
    case 'graph-edges':
      return c.edges
    default:
      return null
  }
}
</script>

<template>
  <nav class="flex flex-col gap-0.5">
    <template v-for="entry in navEntries" :key="entry.id">
      <template v-if="entry.children?.length">
        <div
          class="px-3 pt-2 pb-1 text-xs font-semibold tracking-wide uppercase"
          :class="
            groupActive(entry.children)
              ? 'text-[var(--p-primary-color)]'
              : 'text-slate-400'
          "
        >
          {{ entry.label }}
        </div>
        <RouterLink
          v-for="child in entry.children"
          :key="child.id"
          :to="target(child.route)"
          :class="[baseClass, 'px-3', itemClass(child.route)]"
        >
          <component :is="child.icon" :size="16" class="shrink-0" />
          <span class="truncate">{{ child.label }}</span>
          <span
            v-if="countFor(child.id) !== null"
            class="ml-auto rounded-full bg-slate-100 px-1.5 text-[10px] font-medium tabular-nums text-slate-500"
          >
            {{ countFor(child.id) }}
          </span>
        </RouterLink>
      </template>

      <RouterLink
        v-else
        :to="target(entry.route)"
        :class="[baseClass, 'px-3', itemClass(entry.route)]"
      >
        <component :is="entry.icon" :size="16" class="shrink-0" />
        {{ entry.label }}
      </RouterLink>
    </template>
  </nav>
</template>
