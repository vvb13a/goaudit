import type { Component } from 'vue'
import {
  Box,
  ChartBar,
  CheckSquare,
  Clock,
  Cog,
  Heart,
  Link,
  List,
  Map,
  ShareAlt,
  Sitemap,
} from '@primeicons/vue'

// The sidebar navigation model. Entries with children render a group caption
// followed by their directly clickable children (no reveal needed); the rest
// are plain links. `route` is the router name to open for an entry.
export interface NavChild {
  id: string
  label: string
  icon: Component
  route: string
}

export interface NavEntry {
  id: string
  label: string
  icon: Component
  route: string
  children?: NavChild[]
}

// Every section is a group caption with directly clickable children.
export const navEntries: NavEntry[] = [
  {
    id: 'navigation',
    label: 'Navigation',
    icon: ChartBar,
    route: 'dashboard',
    children: [
      { id: 'dashboard', label: 'Dashboard', icon: ChartBar, route: 'dashboard' },
      { id: 'timeline', label: 'Timeline', icon: Clock, route: 'timeline' },
      { id: 'config', label: 'Config', icon: Cog, route: 'config' },
    ],
  },
  {
    id: 'health',
    label: 'Health',
    icon: Heart,
    route: 'urls',
    children: [
      { id: 'urls', label: 'URLs', icon: Link, route: 'urls' },
      { id: 'issues', label: 'Issues', icon: List, route: 'issues' },
      { id: 'checks', label: 'Checks', icon: CheckSquare, route: 'checks' },
    ],
  },
  {
    id: 'graph',
    label: 'Graph',
    icon: Sitemap,
    route: 'graph-navigator',
    children: [
      { id: 'graph-navigator', label: 'Navigator', icon: Sitemap, route: 'graph-navigator' },
      { id: 'graph-map', label: 'Visualization', icon: Map, route: 'graph-map' },
      { id: 'graph-nodes', label: 'Nodes', icon: Box, route: 'graph-nodes' },
      { id: 'graph-edges', label: 'Edges', icon: ShareAlt, route: 'graph-edges' },
    ],
  },
]

// routeMatches reports whether the current route belongs to a nav entry. The
// node-detail graph route belongs to the Navigator entry.
export function routeMatches(routeName: string | undefined | null, target: string): boolean {
  if (!routeName) return false
  if (routeName === target) return true
  return target === 'graph-navigator' && routeName === 'graph-navigator-node'
}

// resumeRouteName is the route opened when switching audit while keeping the
// current section. Routes that require extra params fall back to their parent.
export function resumeRouteName(routeName: string | undefined | null): string {
  if (!routeName || routeName === 'audits' || routeName === 'graph') return 'dashboard'
  if (routeName === 'graph-navigator-node') return 'graph-navigator'
  return routeName
}
