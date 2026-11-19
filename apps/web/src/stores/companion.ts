import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import type { CompanionState, SiteCompanionSettings } from '@/api/types'
import { localDateKey } from '@/utils/date'

export const COMPANION_PREFS_KEY = 'kapoj.companion.prefs'
export const COMPANION_NOTES_KEY = 'kapoj.companion.notes'
export const COMPANION_GREETING_KEY = 'kapoj.companion.greeting'

export type CompanionActivityLevel = 'quiet' | 'normal' | 'lively'
export type CompanionMotionPreference = 'system' | 'reduce'

/**
 * Progress defaults mirror the server's level table at bond 0. They are only
 * placeholders until the first successful sync (or for guests, who never sync).
 */
export const DEFAULT_COMPANION_PROGRESS = {
  bond: 0,
  level: 0,
  levelName: '陌生',
  nextThreshold: 20,
  dailyGain: 0,
  dailyCap: 15,
  unreadCount: 0,
}

interface CompanionPrefs {
  enabled: boolean
  activityLevel: CompanionActivityLevel
  motion: CompanionMotionPreference
}

const DEFAULT_PREFS: CompanionPrefs = {
  enabled: true,
  activityLevel: 'normal',
  motion: 'system',
}

const ACTIVITY_LEVELS: readonly CompanionActivityLevel[] = ['quiet', 'normal', 'lively']
const MOTION_PREFERENCES: readonly CompanionMotionPreference[] = ['system', 'reduce']

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function readJson(key: string): unknown {
  try {
    const raw = localStorage.getItem(key)
    return raw ? JSON.parse(raw) : null
  } catch {
    return null
  }
}

function readString(key: string): string {
  try {
    return localStorage.getItem(key) ?? ''
  } catch {
    return ''
  }
}

function writeJson(key: string, value: unknown): void {
  try {
    localStorage.setItem(key, JSON.stringify(value))
  } catch {
    // Storage can be blocked or full; in-memory preferences still work for the session.
  }
}

function writeString(key: string, value: string): void {
  try {
    localStorage.setItem(key, value)
  } catch {
    // See writeJson: this is a best-effort local cache.
  }
}

function removeKey(key: string): void {
  try {
    localStorage.removeItem(key)
  } catch {
    // Nothing to clean up if storage is unavailable.
  }
}

function readPrefs(): CompanionPrefs {
  const stored = readJson(COMPANION_PREFS_KEY)
  if (!isRecord(stored)) return { ...DEFAULT_PREFS }

  return {
    enabled: typeof stored.enabled === 'boolean' ? stored.enabled : DEFAULT_PREFS.enabled,
    activityLevel: ACTIVITY_LEVELS.includes(stored.activityLevel as CompanionActivityLevel)
      ? (stored.activityLevel as CompanionActivityLevel)
      : DEFAULT_PREFS.activityLevel,
    motion: MOTION_PREFERENCES.includes(stored.motion as CompanionMotionPreference)
      ? (stored.motion as CompanionMotionPreference)
      : DEFAULT_PREFS.motion,
  }
}

function readDismissedNotes(): Record<string, string> {
  const stored = readJson(COMPANION_NOTES_KEY)
  if (!isRecord(stored)) return {}

  const notes: Record<string, string> = {}
  for (const [key, value] of Object.entries(stored)) {
    if (typeof value === 'string') notes[key] = value
  }
  return notes
}

/** Remote progress is written only when the server value is a valid count. */
function nonNegativeInt(value: unknown): number | null {
  return typeof value === 'number' && Number.isInteger(value) && value >= 0 ? value : null
}

/**
 * Frontend-only companion preferences. Phase 0–1 keeps them in localStorage so
 * the avatar, dock and page notes share one source of truth per browser.
 */
export const useCompanionStore = defineStore('companion', () => {
  const enabled = ref(DEFAULT_PREFS.enabled)
  const activityLevel = ref<CompanionActivityLevel>(DEFAULT_PREFS.activityLevel)
  const motion = ref<CompanionMotionPreference>(DEFAULT_PREFS.motion)
  const loaded = ref(false)

  const dismissedNotes = ref<Record<string, string>>({})
  const greetedOn = ref('')

  const bond = ref(DEFAULT_COMPANION_PROGRESS.bond)
  const level = ref(DEFAULT_COMPANION_PROGRESS.level)
  const levelName = ref(DEFAULT_COMPANION_PROGRESS.levelName)
  const nextThreshold = ref(DEFAULT_COMPANION_PROGRESS.nextThreshold)
  const dailyGain = ref(DEFAULT_COMPANION_PROGRESS.dailyGain)
  const dailyCap = ref(DEFAULT_COMPANION_PROGRESS.dailyCap)
  const unreadCount = ref(DEFAULT_COMPANION_PROGRESS.unreadCount)
  const latestEventLine = ref('')
  const globalEnabled = ref(true)
  const publicProfile = ref(false)
  const personalityPrompt = ref('')
  const aiDialogueEnabled = ref(true)
  const aiReady = ref(false)
  const aiRemainingToday = ref(0)
  const name = ref('Koj')
  const contestMute = ref(true)
  const allowUserDisable = ref(true)

  /** Both the site-wide switch and the visitor's own preference must allow it. */
  const effectiveEnabled = computed(() => globalEnabled.value && enabled.value)

  function persistPrefs(): void {
    writeJson(COMPANION_PREFS_KEY, {
      enabled: enabled.value,
      activityLevel: activityLevel.value,
      motion: motion.value,
    })
  }

  /** Reads local preferences once; later calls keep the current in-memory state. */
  function load(): void {
    if (loaded.value) return

    const prefs = readPrefs()
    enabled.value = prefs.enabled
    activityLevel.value = prefs.activityLevel
    motion.value = prefs.motion
    dismissedNotes.value = readDismissedNotes()
    greetedOn.value = readString(COMPANION_GREETING_KEY)
    loaded.value = true

    // Normalizes corrupt or partial storage back to a valid shape.
    persistPrefs()
  }

  function setEnabled(value: boolean): void {
    enabled.value = allowUserDisable.value ? value : true
    persistPrefs()
  }

  function setActivityLevel(value: CompanionActivityLevel): void {
    activityLevel.value = value
    persistPrefs()
  }

  function setMotion(value: CompanionMotionPreference): void {
    motion.value = value
    persistPrefs()
  }

  /** Restores every preference and forgets dismissed notes and the daily greeting. */
  function reset(): void {
    enabled.value = DEFAULT_PREFS.enabled
    activityLevel.value = DEFAULT_PREFS.activityLevel
    motion.value = DEFAULT_PREFS.motion
    dismissedNotes.value = {}
    greetedOn.value = ''
    removeKey(COMPANION_PREFS_KEY)
    removeKey(COMPANION_NOTES_KEY)
    removeKey(COMPANION_GREETING_KEY)
  }

  function dismissNote(key: string, date = localDateKey()): void {
    dismissedNotes.value = { ...dismissedNotes.value, [key]: date }
    writeJson(COMPANION_NOTES_KEY, dismissedNotes.value)
  }

  function isNoteDismissed(key: string, date = localDateKey()): boolean {
    return dismissedNotes.value[key] === date
  }

  function shouldGreet(date = localDateKey()): boolean {
    return greetedOn.value !== date
  }

  function markGreeted(date = localDateKey()): void {
    greetedOn.value = date
    writeString(COMPANION_GREETING_KEY, date)
  }

  /**
   * Merges a server read model into the in-memory progress. `activityLevel` and
   * `dockEnabled` are server-owned: on login the server value wins and is
   * persisted locally. `motion` has no server counterpart and always stays
   * local, and `enabled` only changes through the server's `dockEnabled` flag.
   * The API failing or returning nothing must never disable the local fallback.
   */
  function applyRemoteState(state: CompanionState | null | undefined): void {
    if (!state) return

    const nextBondValue = nonNegativeInt(state.bond)
    if (nextBondValue !== null) bond.value = nextBondValue

    const nextLevelValue = nonNegativeInt(state.level)
    if (nextLevelValue !== null) level.value = nextLevelValue

    const nextThresholdValue = nonNegativeInt(state.nextThreshold)
    if (nextThresholdValue !== null) nextThreshold.value = nextThresholdValue

    const nextDailyGainValue = nonNegativeInt(state.dailyGain)
    if (nextDailyGainValue !== null) dailyGain.value = nextDailyGainValue

    const nextDailyCapValue = nonNegativeInt(state.dailyCap)
    if (nextDailyCapValue !== null) dailyCap.value = nextDailyCapValue

    const nextUnreadValue = nonNegativeInt(state.unreadCount)
    if (nextUnreadValue !== null) unreadCount.value = nextUnreadValue

    if (typeof state.levelName === 'string' && state.levelName.trim() !== '') {
      levelName.value = state.levelName.trim()
    }

    if (typeof state.publicProfile === 'boolean') {
      publicProfile.value = state.publicProfile
    }
    if (typeof state.personalityPrompt === 'string') {
      personalityPrompt.value = state.personalityPrompt
    }
    if (typeof state.aiDialogueEnabled === 'boolean') {
      aiDialogueEnabled.value = state.aiDialogueEnabled
    }
    if (typeof state.aiReady === 'boolean') {
      aiReady.value = state.aiReady
    }
    const nextRemaining = nonNegativeInt(state.aiRemainingToday)
    if (nextRemaining !== null) aiRemainingToday.value = nextRemaining

    let prefsChanged = false
    if (ACTIVITY_LEVELS.includes(state.activityLevel as CompanionActivityLevel)) {
      activityLevel.value = state.activityLevel as CompanionActivityLevel
      prefsChanged = true
    }
    if (typeof state.dockEnabled === 'boolean') {
      // An admin who forbids user control wins over a stale server opt-out.
      enabled.value = allowUserDisable.value ? state.dockEnabled : true
      prefsChanged = true
    }
    if (prefsChanged) persistPrefs()
  }

  /** Applies the site-wide switch; a missing config is treated as enabled. */
  function applyGlobalConfig(settings: SiteCompanionSettings | null | undefined): void {
    globalEnabled.value = settings?.enabled ?? true
    name.value = settings?.name?.trim() || 'Koj'
    contestMute.value = settings?.contestMute ?? true
    const allowDisable = settings?.allowUserDisable ?? true
    allowUserDisable.value = allowDisable
    // An admin who forbids user control keeps the companion switched on.
    if (!allowDisable) {
      enabled.value = true
      persistPrefs()
    }
  }

  function setLatestEventLine(line: string): void {
    latestEventLine.value = typeof line === 'string' ? line : ''
  }

  return {
    enabled,
    activityLevel,
    motion,
    loaded,
    load,
    setEnabled,
    setActivityLevel,
    setMotion,
    reset,
    dismissNote,
    isNoteDismissed,
    shouldGreet,
    markGreeted,
    bond,
    level,
    levelName,
    nextThreshold,
    dailyGain,
    dailyCap,
    unreadCount,
    latestEventLine,
    globalEnabled,
    publicProfile,
    personalityPrompt,
    aiDialogueEnabled,
    aiReady,
    aiRemainingToday,
    name,
    contestMute,
    allowUserDisable,
    effectiveEnabled,
    applyRemoteState,
    applyGlobalConfig,
    setLatestEventLine,
  }
})
