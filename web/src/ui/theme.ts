import { ref } from 'vue'

export type ThemeMode = 'light' | 'dark' | 'auto'

const THEME_STORAGE_KEY = 'media-tool:theme-mode:v1'

function getStoredTheme(): ThemeMode {
  try {
    const val = localStorage.getItem(THEME_STORAGE_KEY)
    if (val === 'light' || val === 'dark' || val === 'auto') return val
  } catch {}
  return 'auto'
}

export const themeMode = ref<ThemeMode>(getStoredTheme())

export function applyTheme(mode: ThemeMode, persist = true) {
  themeMode.value = mode
  if (persist && mode !== 'auto') {
    try {
      localStorage.setItem(THEME_STORAGE_KEY, mode)
    } catch {}
  } else if (persist && mode === 'auto') {
    try {
      localStorage.removeItem(THEME_STORAGE_KEY)
    } catch {}
  }

  if (typeof document === 'undefined') return
  const root = document.documentElement
  const isDark = mode === 'dark' || (mode === 'auto' && typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: dark)').matches)

  if (isDark) {
    root.classList.add('dark')
    root.setAttribute('data-theme', 'dark')
  } else {
    root.classList.remove('dark')
    root.setAttribute('data-theme', 'light')
  }
}

export function initTheme() {
  applyTheme(themeMode.value, false)
  if (typeof window !== 'undefined' && window.matchMedia) {
    try {
      window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
        if (themeMode.value === 'auto') {
          applyTheme('auto', false)
        }
      })
    } catch {}
  }
}
