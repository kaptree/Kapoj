import { computed, readonly, ref } from 'vue'

export const THEME_STORAGE_KEY = 'kapoj.theme'
export const DARK_THEME_CLASS = 'app-dark'

export type ThemeMode = 'dark' | 'light'

const theme = ref<ThemeMode>('dark')

export function readStoredTheme(): ThemeMode {
  if (typeof localStorage === 'undefined') return 'dark'
  try {
    return localStorage.getItem(THEME_STORAGE_KEY) === 'light' ? 'light' : 'dark'
  } catch {
    return 'dark'
  }
}

export function applyTheme(mode: ThemeMode): void {
  if (typeof document === 'undefined') return

  const root = document.documentElement
  root.classList.toggle(DARK_THEME_CLASS, mode === 'dark')
  root.dataset.theme = mode
  root.style.colorScheme = mode
}

export function setThemeMode(mode: ThemeMode): void {
  theme.value = mode
  applyTheme(mode)

  if (typeof localStorage === 'undefined') return
  try {
    localStorage.setItem(THEME_STORAGE_KEY, mode)
  } catch {
    // Storage may be unavailable in private browsing; the in-memory choice still applies.
  }
}

export function initTheme(): ThemeMode {
  const mode = readStoredTheme()
  theme.value = mode
  applyTheme(mode)
  return mode
}

export function useTheme() {
  return {
    theme: readonly(theme),
    isDark: computed(() => theme.value === 'dark'),
    setTheme: setThemeMode,
    toggleTheme: () => setThemeMode(theme.value === 'dark' ? 'light' : 'dark'),
  }
}
