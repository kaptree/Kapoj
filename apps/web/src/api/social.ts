import { http } from './client'
import type {
  EvaluationInput,
  EvaluationItem,
  EvaluationPage,
  FollowResult,
  LikeResult,
  NotificationPage,
  SocialUserPage,
  SocialView,
  UnreadCount,
} from './types'

export interface SocialPageQuery {
  page?: number
  pageSize?: number
}

const pageParams = (q: SocialPageQuery) => ({ page: q.page, page_size: q.pageSize })

export const likeSolutionPost = (problemId: number | string, postId: number | string) =>
  http.post<LikeResult>(`/problems/${problemId}/solutions/${postId}/like`)

export const unlikeSolutionPost = (problemId: number | string, postId: number | string) =>
  http.delete<LikeResult>(`/problems/${problemId}/solutions/${postId}/like`)

export const followUser = (id: number | string) => http.post<FollowResult>(`/users/${id}/follow`)

export const unfollowUser = (id: number | string) =>
  http.delete<FollowResult>(`/users/${id}/follow`)

export const getUserSocial = (id: number | string) => http.get<SocialView>(`/users/${id}/social`)

export const listFollowers = (id: number | string, q: SocialPageQuery = {}) =>
  http.get<SocialUserPage>(`/users/${id}/followers`, { params: pageParams(q) })

export const listFollowing = (id: number | string, q: SocialPageQuery = {}) =>
  http.get<SocialUserPage>(`/users/${id}/following`, { params: pageParams(q) })

/** Public user evaluation page, word cloud and viewer permissions. */
export const listUserEvaluations = (userId: number | string, page = 1, pageSize = 20) =>
  http.get<EvaluationPage>(`/users/${userId}/evaluations`, {
    params: { page, page_size: pageSize },
  })

/** Create or update the current user's evaluation of a profile. */
export const saveMyEvaluation = (userId: number | string, content: string) => {
  const input: EvaluationInput = { content }
  return http.put<EvaluationItem>(`/users/${userId}/evaluations/mine`, input)
}

/** Delete the current user's evaluation; idempotent when absent. */
export const deleteMyEvaluation = (userId: number | string) =>
  http.delete<null>(`/users/${userId}/evaluations/mine`)

/** Delete any evaluation on a profile the current user owns. */
export const deleteUserEvaluation = (userId: number | string, evaluationId: number | string) =>
  http.delete<null>(`/users/${userId}/evaluations/${evaluationId}`)

export const listNotifications = (q: SocialPageQuery = {}) =>
  http.get<NotificationPage>('/notifications', { params: pageParams(q) })

export const unreadCount = () => http.get<UnreadCount>('/notifications/unread-count')

export const markRead = (id: number | string) => http.post<null>(`/notifications/${id}/read`)

export const markAllRead = () => http.post<null>('/notifications/read-all')
