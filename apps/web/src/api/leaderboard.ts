import { http } from './client'
import type {
  ActivityLeaderboardRow,
  ActivitySignInResult,
  ActivitySummary,
  Page,
  RatingHistoryPage,
  RatingLeaderboardPage,
  RatingSeasonView,
  SharingLeaderboardRow,
  WealthLeaderboardRow,
} from './types'

interface PageQuery {
  page: number
  pageSize: number
}

const pageParams = (q: PageQuery) => ({ page: q.page, page_size: q.pageSize })

export const listRatingLeaderboard = (q: PageQuery) =>
  http.get<RatingLeaderboardPage>('/leaderboard/rating', { params: pageParams(q) })

export const listActivityLeaderboard = (q: PageQuery) =>
  http.get<Page<ActivityLeaderboardRow>>('/leaderboard/activity', { params: pageParams(q) })

export const listSharingLeaderboard = (q: PageQuery) =>
  http.get<Page<SharingLeaderboardRow>>('/leaderboard/sharing', { params: pageParams(q) })

export const listWealthLeaderboard = (q: PageQuery) =>
  http.get<Page<WealthLeaderboardRow>>('/leaderboard/wealth', { params: pageParams(q) })

export const getRatingHistory = (userId: number, q: PageQuery) =>
  http.get<RatingHistoryPage>(`/users/${userId}/rating-history`, { params: pageParams(q) })

/** Current rating season (public). */
export const getRatingSeason = () => http.get<RatingSeasonView>('/rating/season/current')

export const getActivitySummary = () => http.get<ActivitySummary>('/activity/summary')

export const signInActivity = () => http.post<ActivitySignInResult>('/activity/signin')
