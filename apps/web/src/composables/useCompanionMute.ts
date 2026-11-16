import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import { useRoute } from 'vue-router'

import { getContest } from '@/api/contests'

const CACHE_TTL_MS = 15_000

interface CachedContestStatus {
  status: string
  at: number
}

const POSITIVE_INTEGER = /^\d+$/

/**
 * Route params and query values arrive as strings, so only a plain positive
 * integer is accepted. This keeps `7.5`, `1e2` or whitespace-padded values
 * from turning into a contest request.
 */
function parsePositiveId(value: unknown): number {
  if (typeof value !== 'string' || !POSITIVE_INTEGER.test(value)) return 0
  const id = Number(value)
  return Number.isSafeInteger(id) && id > 0 ? id : 0
}

const contestCache = new Map<number, CachedContestStatus>()

/** Drops cached contest statuses; tests call this between cases. */
export function clearCompanionMuteCache(): void {
  contestCache.clear()
}

export interface CompanionMute {
  /** True while the page belongs to a contest that is currently running. */
  muted: ComputedRef<boolean>
  /** True while a contest status request is in flight. */
  checking: Ref<boolean>
}

/**
 * Decides whether the companion must stay silent because a contest is running.
 *
 * Contest detail pages read the contest id from the route params; problem pages
 * only consult a contest when they were opened from one through `?contestId=`.
 * Ordinary practice pages never request a contest. A failed request counts as
 * "not muted" so the companion never disappears because of a network error.
 */
export function useCompanionMute(): CompanionMute {
  const route = useRoute()
  const status = ref<string | null>(null)
  const checking = ref(false)

  const contestId = computed(() => {
    if (route.name === 'contest-detail') {
      return parsePositiveId(route.params.id)
    }
    if (route.name === 'problem-detail' || route.name === 'problem-submit') {
      return parsePositiveId(route.query.contestId)
    }
    return 0
  })

  watch(
    contestId,
    async (id) => {
      status.value = null

      if (id <= 0) {
        checking.value = false
        return
      }

      const cached = contestCache.get(id)
      if (cached && Date.now() - cached.at < CACHE_TTL_MS) {
        status.value = cached.status
        checking.value = false
        return
      }

      checking.value = true
      try {
        const contest = await getContest(id)
        // Ignore a response that arrives after the route moved elsewhere.
        if (contestId.value !== id) return
        status.value = contest.status
        contestCache.set(id, { status: contest.status, at: Date.now() })
      } catch {
        if (contestId.value === id) status.value = null
      } finally {
        if (contestId.value === id) checking.value = false
      }
    },
    { immediate: true },
  )

  const muted = computed(() => contestId.value > 0 && status.value === 'running')

  return { muted, checking }
}
