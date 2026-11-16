import { onBeforeUnmount, watch } from 'vue'

import { getCompanion, listCompanionEvents, markCompanionEventsRead } from '@/api/companion'
import type { CompanionLineKey } from '@/content/companion/lines.zh-CN'
import { pickCompanionLine } from '@/content/companion/lines.zh-CN'
import { useAuthStore } from '@/stores/auth'
import { useCompanionStore } from '@/stores/companion'
import { localDateKey } from '@/utils/date'

/** Unread events are polled once a minute while the tab is visible. */
export const COMPANION_EVENT_POLL_MS = 60_000

/** Event types that have a dedicated bubble line. */
const EVENT_LINE_KEYS: Record<string, CompanionLineKey> = {
  signin: 'signin',
  task: 'task',
  achievement: 'achievement',
  contest: 'contest',
  pet: 'pet',
  streak: 'streak',
}

/**
 * The backend emits `ac:daily` and `ac:first` (and may add more AC variants);
 * they all share the accepted line. Unknown events fall back to a greeting so
 * a new event type can never break the bubble.
 */
export function companionEventLineKey(type: string): CompanionLineKey {
  if (Object.hasOwn(EVENT_LINE_KEYS, type)) return EVENT_LINE_KEYS[type]
  if (type.startsWith('ac')) return 'ac'
  return 'greeting'
}

export interface CompanionSync {
  /** Fetches and applies the current remote state; never rejects. */
  syncState(): Promise<void>
  /** Fetches unread events, updates the line and marks them read; never rejects. */
  pollEvents(): Promise<void>
}

/**
 * Keeps the companion store in sync with the authenticated user. Guests never
 * emit a request, a hidden tab never polls, and every network failure is
 * swallowed so the locally cached preferences and lines keep working.
 */
export function useCompanionSync(): CompanionSync {
  const auth = useAuthStore()
  const companion = useCompanionStore()

  let timer: ReturnType<typeof setInterval> | null = null
  let polling = false
  let stopped = false
  // Bumped on every login/logout so responses for a previous user are dropped.
  let generation = 0

  function canSync(): boolean {
    return !stopped && auth.isLoggedIn && auth.user !== null
  }

  function isVisible(): boolean {
    return typeof document === 'undefined' || document.visibilityState === 'visible'
  }

  function stopTimer(): void {
    if (timer === null) return
    clearInterval(timer)
    timer = null
  }

  async function syncState(): Promise<void> {
    if (!canSync()) return
    const run = generation
    try {
      const state = await getCompanion()
      if (run !== generation || stopped) return
      companion.applyRemoteState(state)
    } catch {
      // Keep the last known state; a later poll or login retries.
    }
  }

  async function pollEvents(): Promise<void> {
    if (!canSync() || !isVisible() || polling) return

    const run = generation
    polling = true
    try {
      const events = await listCompanionEvents()
      if (run !== generation || stopped || events.length === 0) return

      const latest = events[events.length - 1]
      companion.setLatestEventLine(
        pickCompanionLine(companionEventLineKey(latest.type), `${localDateKey()}:${latest.id}`),
      )

      await markCompanionEventsRead(events.map((event) => event.id))
      if (run === generation && !stopped) {
        companion.unreadCount = Math.max(0, companion.unreadCount - events.length)
      }
    } catch {
      // Keep the unread count and line; the next tick retries the same events.
    } finally {
      polling = false
    }
  }

  function start(): void {
    generation += 1
    stopTimer()
    if (!canSync()) return

    // Fetch progress before reading events so a slow state response cannot
    // overwrite the unread count that the event poll just consumed.
    void syncState().then(() => pollEvents())
    timer = setInterval(() => {
      void pollEvents()
    }, COMPANION_EVENT_POLL_MS)
  }

  function handleVisibilityChange(): void {
    if (isVisible()) void pollEvents()
  }

  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', handleVisibilityChange)
  }

  watch(
    () => (auth.isLoggedIn ? (auth.user?.id ?? 0) : 0),
    () => {
      start()
    },
    { immediate: true },
  )

  onBeforeUnmount(() => {
    stopped = true
    stopTimer()
    if (typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  })

  return { syncState, pollEvents }
}
