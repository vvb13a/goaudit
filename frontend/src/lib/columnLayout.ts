import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import { loadJSON, saveJSON } from './storage'

// ColumnDef describes one toggleable column of a table.
export interface ColumnDef {
  field: string
  label: string
}

interface StoredLayout {
  version: number
  order?: string[]
  visible?: string[]
}

const LAYOUT_VERSION = 1

export interface ColumnLayout {
  labels: Record<string, string>
  columnOrder: Ref<string[]>
  visibleFields: Ref<string[]>
  visibleOrderedFields: ComputedRef<string[]>
  columnsKey: ComputedRef<string>
  reset: () => void
}

// useColumnLayout owns the column order and visibility of one table and
// persists them under "columns:<key>". The DataTable must be keyed on
// columnsKey so a change re-renders its header and body.
export function useColumnLayout(storageKey: string, defs: ColumnDef[]): ColumnLayout {
  const fields = defs.map((d) => d.field)
  const known = new Set(fields)
  const labels = Object.fromEntries(defs.map((d) => [d.field, d.label]))

  const stored = loadJSON<StoredLayout>(`columns:${storageKey}`)

  const sanitizeOrder = (order: string[] | undefined): string[] => {
    const out = (order ?? []).filter((f) => known.has(f))
    for (const f of fields) {
      if (!out.includes(f)) out.push(f)
    }
    return out
  }

  const columnOrder = ref<string[]>(sanitizeOrder(stored?.order))

  const visible = (stored?.visible ?? fields).filter((f) => known.has(f))
  const visibleFields = ref<string[]>(visible)

  const visibleOrderedFields = computed(() =>
    columnOrder.value.filter((f) => visibleFields.value.includes(f)),
  )
  const columnsKey = computed(() => visibleOrderedFields.value.join('-'))

  watch(
    [columnOrder, visibleFields],
    () => {
      saveJSON(`columns:${storageKey}`, {
        version: LAYOUT_VERSION,
        order: columnOrder.value,
        visible: visibleFields.value,
      })
    },
    { deep: true },
  )

  function reset(): void {
    columnOrder.value = [...fields]
    visibleFields.value = [...fields]
  }

  return { labels, columnOrder, visibleFields, visibleOrderedFields, columnsKey, reset }
}
