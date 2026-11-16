import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  COMPANION_GREETING_KEY,
  COMPANION_NOTES_KEY,
  COMPANION_PREFS_KEY,
  useCompanionStore,
} from './companion'

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('companion store', () => {
  it('defaults to enabled, normal activity and system motion', () => {
    const store = useCompanionStore()
    store.load()
    expect(store.enabled).toBe(true)
    expect(store.activityLevel).toBe('normal')
    expect(store.motion).toBe('system')
  })

  it('persists preferences and tolerates corrupt storage', () => {
    localStorage.setItem(COMPANION_PREFS_KEY, '{not json')
    const store = useCompanionStore()
    store.load()
    store.setEnabled(false)
    store.setActivityLevel('quiet')
    store.setMotion('reduce')
    const saved = JSON.parse(localStorage.getItem(COMPANION_PREFS_KEY)!)
    expect(saved).toEqual({ enabled: false, activityLevel: 'quiet', motion: 'reduce' })
  })

  it('dismisses a note only for the given local day', () => {
    const store = useCompanionStore()
    store.load()
    store.dismissNote('problems', '2026-11-16')
    expect(store.isNoteDismissed('problems', '2026-11-16')).toBe(true)
    expect(store.isNoteDismissed('problems', '2026-11-17')).toBe(false)
    expect(localStorage.getItem(COMPANION_NOTES_KEY)).toContain('problems')
  })

  it('greets once per local day', () => {
    const store = useCompanionStore()
    store.load()
    expect(store.shouldGreet('2026-11-16')).toBe(true)
    store.markGreeted('2026-11-16')
    expect(store.shouldGreet('2026-11-16')).toBe(false)
    expect(store.shouldGreet('2026-11-17')).toBe(true)
  })

  it('loads with defaults and still updates memory when storage reads throw', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('storage blocked')
    })
    expect(() => localStorage.getItem('probe')).toThrow('storage blocked')

    const store = useCompanionStore()

    expect(() => store.load()).not.toThrow()
    expect(store.enabled).toBe(true)
    expect(store.activityLevel).toBe('normal')
    expect(store.motion).toBe('system')

    expect(() => store.markGreeted('2026-11-16')).not.toThrow()
    expect(store.shouldGreet('2026-11-16')).toBe(false)
  })

  it('keeps updating memory when storage writes throw', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('storage full')
    })
    expect(() => localStorage.setItem('probe', '1')).toThrow('storage full')

    const store = useCompanionStore()
    store.load()

    expect(() => store.setEnabled(false)).not.toThrow()
    expect(store.enabled).toBe(false)

    expect(() => store.dismissNote('problems', '2026-11-16')).not.toThrow()
    expect(store.isNoteDismissed('problems', '2026-11-16')).toBe(true)

    expect(() => store.markGreeted('2026-11-16')).not.toThrow()
    expect(store.shouldGreet('2026-11-16')).toBe(false)
  })

  it('does not re-read storage on a second load', () => {
    const store = useCompanionStore()
    store.load()
    store.setEnabled(false)

    // Simulate another tab changing storage: a second load must not clobber
    // the in-memory preference with these values.
    localStorage.setItem(
      COMPANION_PREFS_KEY,
      JSON.stringify({ enabled: true, activityLevel: 'lively', motion: 'reduce' }),
    )
    store.load()

    expect(store.enabled).toBe(false)
    expect(store.activityLevel).toBe('normal')
    expect(store.motion).toBe('system')
  })

  it('reset restores defaults and clears all three storage keys', () => {
    const store = useCompanionStore()
    store.load()
    store.setEnabled(false)
    store.setActivityLevel('lively')
    store.setMotion('reduce')
    store.dismissNote('problems', '2026-11-16')
    store.markGreeted('2026-11-16')

    store.reset()

    expect(store.enabled).toBe(true)
    expect(store.activityLevel).toBe('normal')
    expect(store.motion).toBe('system')
    expect(store.shouldGreet('2026-11-16')).toBe(true)
    expect(store.isNoteDismissed('problems', '2026-11-16')).toBe(false)
    expect(localStorage.getItem(COMPANION_PREFS_KEY)).toBeNull()
    expect(localStorage.getItem(COMPANION_NOTES_KEY)).toBeNull()
    expect(localStorage.getItem(COMPANION_GREETING_KEY)).toBeNull()
  })
})
