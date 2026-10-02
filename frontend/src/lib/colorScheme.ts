import { ref } from 'vue'

// Manual light/dark selection. PrimeVue is configured with a class-based dark
// selector (`.p-dark`); we toggle it on <html> and also set `color-scheme` so
// the CSS `light-dark()` tokens resolve. With no stored preference the system
// preference is followed, including later system changes.
const STORAGE_KEY = 'goaudit:color-scheme'
type Scheme = 'light' | 'dark'

export const isDark = ref(false)

function stored(): Scheme | null {
  try {
    const value = localStorage.getItem(STORAGE_KEY)
    return value === 'light' || value === 'dark' ? value : null
  } catch {
    return null
  }
}

function prefersDark(): boolean {
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false
}

function apply(dark: boolean): void {
  isDark.value = dark
  const root = document.documentElement
  root.classList.toggle('p-dark', dark)
  root.style.colorScheme = dark ? 'dark' : 'light'
}

export function initColorScheme(): void {
  const preference = stored()
  apply(preference ? preference === 'dark' : prefersDark())

  window.matchMedia?.('(prefers-color-scheme: dark)').addEventListener('change', (event) => {
    // An explicit choice wins over later system changes.
    if (!stored()) apply(event.matches)
  })
}

export function toggleColorScheme(): void {
  const next = !isDark.value
  try {
    localStorage.setItem(STORAGE_KEY, next ? 'dark' : 'light')
  } catch {
    // Storage can be unavailable; the toggle still applies for this session.
  }
  apply(next)
}
