import { defineStore } from 'pinia'
import { ref } from 'vue'

import { localDateKey } from '@/utils/date'

export const COMPANION_PREFS_KEY = 'kapoj.companion.prefs'
export const COMPANION_NOTES_KEY = 'kapoj.companion.notes'
export const COMPANION_GREETING_KEY = 'kapoj.companion.greeting'

export type CompanionActivityLevel = 'quiet' | 'normal' | 'lively'
export type CompanionMotionPreference = 'system' | 'reduce'

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
    enabled.value = value
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
  }
})
