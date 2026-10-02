import {
  createRouter,
  createWebHistory,
  type RouteRecordRaw,
} from 'vue-router'
import EmptyView from './components/EmptyView.vue'

// Tab routes are lazy so each view is its own chunk. The path shape is
// /audits/:auditId/<tab>; filters, sorting and paging live in the query so a
// view is shareable and the browser back/forward works.
const routes: RouteRecordRaw[] = [
  { path: '/', redirect: '/audits' },
  { path: '/audits', name: 'audits', component: EmptyView },
  {
    path: '/audits/:auditId',
    redirect: (to) => ({ name: 'dashboard', params: to.params }),
  },
  {
    path: '/audits/:auditId/dashboard',
    name: 'dashboard',
    component: () => import('./components/DashboardView.vue'),
    props: true,
  },
  {
    path: '/audits/:auditId/urls',
    name: 'urls',
    component: () => import('./components/UrlsView.vue'),
    props: true,
  },
  {
    path: '/audits/:auditId/issues',
    name: 'issues',
    component: () => import('./components/IssuesView.vue'),
    props: true,
  },
  {
    path: '/audits/:auditId/checks',
    name: 'checks',
    component: () => import('./components/ChecksView.vue'),
    props: true,
  },
  {
    path: '/audits/:auditId/timeline',
    name: 'timeline',
    component: () => import('./components/TimelineView.vue'),
    props: true,
  },
  {
    path: '/audits/:auditId/config',
    name: 'config',
    component: () => import('./components/ConfigView.vue'),
    props: true,
  },
  { path: '/:pathMatch(.*)*', redirect: '/audits' },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
})
