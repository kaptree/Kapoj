import { computed, onBeforeUnmount, ref, watch } from 'vue'

import { listTasks } from '@/api/gamification'
import { getActivitySummary, getRatingHistory } from '@/api/leaderboard'
import { listSubmissions } from '@/api/submissions'
import { getUserActivity } from '@/api/users'
import type {
  ActivitySummary,
  RatingHistoryItem,
  SubmissionListItem,
  TaskItem,
  UserActivity,
} from '@/api/types'
import { useAuthStore } from '@/stores/auth'

export type HomeDataState = 'idle' | 'loading' | 'ready' | 'error'

const RECENT_SUBMISSION_LIMIT = 4
const RATING_HISTORY_LIMIT = 8

/**
 * Loads the real, signed-in user's home dashboard data. Each slice is tracked
 * independently so one failing endpoint never hides the rest of the page.
 */
export function useHomeData() {
  const auth = useAuthStore()

  const activity = ref<UserActivity | null>(null)
  const activityState = ref<HomeDataState>('idle')
  const tasks = ref<TaskItem[]>([])
  const tasksState = ref<HomeDataState>('idle')
  const submissions = ref<SubmissionListItem[]>([])
  const submissionsState = ref<HomeDataState>('idle')
  const ratingHistory = ref<RatingHistoryItem[]>([])
  const ratingHistoryState = ref<HomeDataState>('idle')
  const summary = ref<ActivitySummary | null>(null)
  const summaryState = ref<HomeDataState>('idle')

  let activitySeq = 0
  let tasksSeq = 0
  let submissionsSeq = 0
  let ratingHistorySeq = 0
  let summarySeq = 0

  const todaySubmissionCount = computed<number | null>(() => {
    const value = activity.value
    if (!value) return null
    return value.days.find((day) => day.date === value.endDate)?.count ?? 0
  })

  const claimableTaskCount = computed(
    () => tasks.value.filter((task) => task.status === 'active').length,
  )

  const latestRatingEntry = computed<RatingHistoryItem | null>(() => ratingHistory.value[0] ?? null)

  const ratingDelta = computed<number | null>(() => latestRatingEntry.value?.delta ?? null)

  async function loadActivity(userId: number) {
    const current = ++activitySeq
    activityState.value = 'loading'
    try {
      const data = await getUserActivity(userId)
      if (current !== activitySeq) return
      activity.value = data
      activityState.value = 'ready'
    } catch {
      if (current !== activitySeq) return
      activityState.value = 'error'
    }
  }

  async function loadTasks() {
    const current = ++tasksSeq
    tasksState.value = 'loading'
    try {
      const data = await listTasks()
      if (current !== tasksSeq) return
      tasks.value = data
      tasksState.value = 'ready'
    } catch {
      if (current !== tasksSeq) return
      tasksState.value = 'error'
    }
  }

  async function loadSubmissions(userId: number) {
    const current = ++submissionsSeq
    submissionsState.value = 'loading'
    try {
      const page = await listSubmissions({
        userId,
        page: 1,
        pageSize: RECENT_SUBMISSION_LIMIT,
      })
      if (current !== submissionsSeq) return
      submissions.value = page.items
      submissionsState.value = 'ready'
    } catch {
      if (current !== submissionsSeq) return
      submissionsState.value = 'error'
    }
  }

  async function loadRatingHistory(userId: number) {
    const current = ++ratingHistorySeq
    ratingHistoryState.value = 'loading'
    try {
      const page = await getRatingHistory(userId, {
        page: 1,
        pageSize: RATING_HISTORY_LIMIT,
      })
      if (current !== ratingHistorySeq) return
      ratingHistory.value = page.items
      ratingHistoryState.value = 'ready'
    } catch {
      if (current !== ratingHistorySeq) return
      ratingHistoryState.value = 'error'
    }
  }

  async function loadSummary(userId: number) {
    const current = ++summarySeq
    summaryState.value = 'loading'
    try {
      const data = await getActivitySummary()
      if (current !== summarySeq || auth.user?.id !== userId) return
      summary.value = data
      summaryState.value = 'ready'
    } catch {
      if (current !== summarySeq) return
      summaryState.value = 'error'
    }
  }

  function reset() {
    activitySeq += 1
    tasksSeq += 1
    submissionsSeq += 1
    ratingHistorySeq += 1
    summarySeq += 1

    activity.value = null
    activityState.value = 'idle'
    tasks.value = []
    tasksState.value = 'idle'
    submissions.value = []
    submissionsState.value = 'idle'
    ratingHistory.value = []
    ratingHistoryState.value = 'idle'
    summary.value = null
    summaryState.value = 'idle'
  }

  function loadAll(userId: number) {
    void loadActivity(userId)
    void loadTasks()
    void loadSubmissions(userId)
    void loadRatingHistory(userId)
    void loadSummary(userId)
  }

  watch(
    () => auth.user?.id,
    (userId) => {
      reset()
      if (userId) loadAll(userId)
    },
    { immediate: true },
  )

  // The app signs in automatically after restoring a session. Refreshing the
  // summary when the coin balance changes keeps "今日签到" truthful even when
  // that sign-in finishes after the first home request.
  watch(
    () => auth.user?.coins,
    (next, previous) => {
      const userId = auth.user?.id
      if (!userId || next === previous) return
      void loadSummary(userId)
    },
  )

  onBeforeUnmount(() => {
    activitySeq += 1
    tasksSeq += 1
    submissionsSeq += 1
    ratingHistorySeq += 1
    summarySeq += 1
  })

  return {
    activity,
    activityState,
    tasks,
    tasksState,
    submissions,
    submissionsState,
    ratingHistory,
    ratingHistoryState,
    summary,
    summaryState,
    todaySubmissionCount,
    claimableTaskCount,
    latestRatingEntry,
    ratingDelta,
    reloadSummary: () => {
      const userId = auth.user?.id
      if (userId) void loadSummary(userId)
    },
  }
}
