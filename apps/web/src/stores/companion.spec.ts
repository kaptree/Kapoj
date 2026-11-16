import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { CompanionState, SiteCompanionSettings } from '@/api/types'
import {
  COMPANION_GREETING_KEY,
  COMPANION_NOTES_KEY,
  COMPANION_PREFS_KEY,
  useCompanionStore,
} from './companion'

function remoteState(overrides: Partial<CompanionState> = {}): CompanionState {
  return {
    bond: 120,
    level: 3,
    levelName: '搭档',
    nextThreshold: 170,
    dailyGain: 4,
    dailyCap: 15,
    activityLevel: 'normal',
    dockEnabled: true,
    publicProfile: false,
    equippedSkin: '',
    unlockedSkins: [],
    unreadCount: 2,
    todayEvents: [],
    ...overrides,
  }
}

function siteSettings(overrides: Partial<SiteCompanionSettings> = {}): SiteCompanionSettings {
  return {
    enabled: true,
    name: 'Kapo',
    defaultActivityLevel: 'normal',
    contestMute: true,
    allowUserDisable: true,
    ...overrides,
  }
}

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

  it('applies remote progress and hydrates server-owned prefs while keeping motion local', () => {
    const store = useCompanionStore()
    store.load()
    store.setEnabled(false)
    store.setActivityLevel('lively')
    store.setMotion('reduce')

    store.applyRemoteState(remoteState())

    expect(store.bond).toBe(120)
    expect(store.level).toBe(3)
    expect(store.levelName).toBe('搭档')
    expect(store.nextThreshold).toBe(170)
    expect(store.dailyGain).toBe(4)
    expect(store.dailyCap).toBe(15)
    expect(store.unreadCount).toBe(2)
    // activityLevel and dockEnabled are server-owned and win on sync.
    expect(store.activityLevel).toBe('normal')
    expect(store.enabled).toBe(true)
    // Motion has no server counterpart and stays local.
    expect(store.motion).toBe('reduce')
  })

  it('keeps the last remote state when a sync returns nothing', () => {
    const store = useCompanionStore()
    store.applyRemoteState(remoteState({ bond: 88, unreadCount: 5 }))

    store.applyRemoteState(null)
    store.applyRemoteState(undefined)

    expect(store.bond).toBe(88)
    expect(store.unreadCount).toBe(5)
  })

  it('ignores malformed remote fields instead of zeroing good state', () => {
    const store = useCompanionStore()
    store.applyRemoteState(remoteState())

    store.applyRemoteState(
      remoteState({
        bond: Number.NaN,
        level: -1,
        levelName: '',
        nextThreshold: -5,
        dailyGain: 2.5,
        dailyCap: -1,
        unreadCount: -3,
      }),
    )

    expect(store.bond).toBe(120)
    expect(store.level).toBe(3)
    expect(store.levelName).toBe('搭档')
    expect(store.nextThreshold).toBe(170)
    expect(store.dailyGain).toBe(4)
    expect(store.dailyCap).toBe(15)
    expect(store.unreadCount).toBe(2)
  })

  it('stores the latest event line', () => {
    const store = useCompanionStore()
    store.setLatestEventLine('过了。下一题，别得意。')
    expect(store.latestEventLine).toBe('过了。下一题，别得意。')
  })

  it('applies the remote public profile flag', () => {
    const store = useCompanionStore()
    expect(store.publicProfile).toBe(false)

    store.applyRemoteState(remoteState({ publicProfile: true }))
    expect(store.publicProfile).toBe(true)

    store.applyRemoteState(remoteState({ publicProfile: 'yes' as never }))
    expect(store.publicProfile).toBe(true)
  })

  it('gates the effective companion on the site-wide switch', () => {
    const store = useCompanionStore()
    store.load()

    store.applyGlobalConfig(siteSettings({ enabled: false }))
    expect(store.globalEnabled).toBe(false)
    expect(store.enabled).toBe(true)
    expect(store.effectiveEnabled).toBe(false)

    store.applyGlobalConfig(siteSettings())
    expect(store.globalEnabled).toBe(true)
    expect(store.effectiveEnabled).toBe(true)

    store.setEnabled(false)
    expect(store.effectiveEnabled).toBe(false)
  })

  it('treats an absent site config as enabled', () => {
    const store = useCompanionStore()
    store.applyGlobalConfig(siteSettings({ enabled: false }))

    store.applyGlobalConfig(null)
    store.applyGlobalConfig(undefined)

    expect(store.globalEnabled).toBe(true)
  })

  it('hydrates server-owned activity and dock prefs and persists them', () => {
    const store = useCompanionStore()
    store.load()
    store.setEnabled(true)
    store.setActivityLevel('lively')

    store.applyRemoteState(remoteState({ activityLevel: 'quiet', dockEnabled: false }))

    expect(store.activityLevel).toBe('quiet')
    expect(store.enabled).toBe(false)
    const saved = JSON.parse(localStorage.getItem(COMPANION_PREFS_KEY)!)
    expect(saved.activityLevel).toBe('quiet')
    expect(saved.enabled).toBe(false)
  })

  it('ignores an invalid remote activity level', () => {
    const store = useCompanionStore()
    store.load()
    store.setActivityLevel('lively')

    store.applyRemoteState(remoteState({ activityLevel: 'chaotic' as never }))

    expect(store.activityLevel).toBe('lively')
  })

  it('applies name, contest mute and user-disable from the global config', () => {
    const store = useCompanionStore()
    store.applyGlobalConfig(
      siteSettings({ name: '小助手', contestMute: false, allowUserDisable: false }),
    )

    expect(store.name).toBe('小助手')
    expect(store.contestMute).toBe(false)
    expect(store.allowUserDisable).toBe(false)
    // Forbidding user-disable keeps the companion switched on and persists it.
    expect(store.enabled).toBe(true)
    expect(JSON.parse(localStorage.getItem(COMPANION_PREFS_KEY)!)).toMatchObject({
      enabled: true,
    })
  })

  it('defaults missing global config fields to Kapo, muted and user-disable allowed', () => {
    const store = useCompanionStore()
    store.applyGlobalConfig(null)

    expect(store.name).toBe('Kapo')
    expect(store.contestMute).toBe(true)
    expect(store.allowUserDisable).toBe(true)

    store.applyGlobalConfig(siteSettings({ name: '   ' }))
    expect(store.name).toBe('Kapo')
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
