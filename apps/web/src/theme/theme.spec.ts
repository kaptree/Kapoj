import { beforeEach, describe, expect, it } from 'vitest'

import { applyTheme, initTheme, readStoredTheme, setThemeMode, THEME_STORAGE_KEY } from './theme'

beforeEach(() => {
  localStorage.clear()
  document.documentElement.className = ''
  delete document.documentElement.dataset.theme
  document.documentElement.style.colorScheme = ''
})

describe('theme', () => {
  it('defaults to dark mode', () => {
    expect(readStoredTheme()).toBe('dark')
    expect(initTheme()).toBe('dark')
    expect(document.documentElement.classList.contains('app-dark')).toBe(true)
  })

  it('reads a stored light preference', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'light')

    expect(readStoredTheme()).toBe('light')
    expect(initTheme()).toBe('light')
    expect(document.documentElement.classList.contains('app-dark')).toBe(false)
  })

  it('applies the class, data attribute and color scheme', () => {
    applyTheme('dark')
    expect(document.documentElement.classList.contains('app-dark')).toBe(true)
    expect(document.documentElement.dataset.theme).toBe('dark')
    expect(document.documentElement.style.colorScheme).toBe('dark')

    applyTheme('light')
    expect(document.documentElement.classList.contains('app-dark')).toBe(false)
    expect(document.documentElement.dataset.theme).toBe('light')
    expect(document.documentElement.style.colorScheme).toBe('light')
  })

  it('persists a mode change', () => {
    setThemeMode('light')

    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('light')
    expect(document.documentElement.dataset.theme).toBe('light')
  })
})
