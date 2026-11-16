import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'

import { COMPANION_NOTES_KEY, COMPANION_PREFS_KEY, useCompanionStore } from './companion'

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
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
})
