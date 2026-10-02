import { onBeforeUnmount, ref, watch, type Ref } from 'vue'

export interface DelayedLoadingOptions {
  // How long loading must last before the placeholder is shown. Keeps fast
  // loads from flashing a skeleton.
  delay?: number
  // Once shown, how long the placeholder stays. Keeps a boundary case (a load
  // that finishes just after the delay) from flickering.
  minVisible?: number
}

// useDelayedLoading turns a boolean loading flag into a flag that is suited
// for rendering a placeholder: it only becomes true after `delay` and, once
// true, stays true for at least `minVisible`. This removes the flash of a
// spinner/skeleton on fast loads without making the UI feel slower.
export function useDelayedLoading(
  loading: Ref<boolean>,
  options: DelayedLoadingOptions = {},
): Ref<boolean> {
  const { delay = 150, minVisible = 400 } = options
  const visible = ref(false)

  let showTimer: ReturnType<typeof setTimeout> | undefined
  let hideTimer: ReturnType<typeof setTimeout> | undefined
  let shownAt = 0

  function clearShow(): void {
    if (showTimer) {
      clearTimeout(showTimer)
      showTimer = undefined
    }
  }

  function clearHide(): void {
    if (hideTimer) {
      clearTimeout(hideTimer)
      hideTimer = undefined
    }
  }

  watch(
    loading,
    (isLoading) => {
      if (isLoading) {
        clearHide()
        if (!visible.value) {
          clearShow()
          showTimer = setTimeout(() => {
            visible.value = true
            shownAt = Date.now()
          }, delay)
        }
        return
      }

      clearShow()
      if (visible.value) {
        const elapsed = Date.now() - shownAt
        const remaining = Math.max(0, minVisible - elapsed)
        clearHide()
        hideTimer = setTimeout(() => {
          visible.value = false
        }, remaining)
      }
    },
    { immediate: true },
  )

  onBeforeUnmount(() => {
    clearShow()
    clearHide()
  })

  return visible
}
