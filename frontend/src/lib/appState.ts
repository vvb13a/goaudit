import { ref } from 'vue'

// A monotonically increasing counter bumped whenever audit data changes (a run
// finishes, a URL is rechecked, an audit is duplicated/reset/deleted). Views
// watch it to refresh themselves, and the shell re-loads the audit list.
export const dataVersion = ref(0)

export function notifyDataChanged(): void {
  dataVersion.value++
}
