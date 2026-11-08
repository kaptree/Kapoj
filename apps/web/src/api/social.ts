import { http } from './client'
import type {
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

export const listNotifications = (q: SocialPageQuery = {}) =>
  http.get<NotificationPage>('/notifications', { params: pageParams(q) })

export const unreadCount = () => http.get<UnreadCount>('/notifications/unread-count')

export const markRead = (id: number | string) => http.post<null>(`/notifications/${id}/read`)

export const markAllRead = () => http.post<null>('/notifications/read-all')
