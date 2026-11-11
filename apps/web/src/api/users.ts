import { http } from './client'
import type { Me, Profile, UpdateMeRequest, UserActivity } from './types'

export const getMe = () => http.get<Me>('/users/me')

export const updateMe = (input: UpdateMeRequest) => http.put<Me>('/users/me', input)

export const getProfile = (id: number | string) => http.get<Profile>(`/users/${id}/profile`)

/** Public 365-day submission calendar and profile activity totals. */
export const getUserActivity = (id: number | string) =>
  http.get<UserActivity>(`/users/${id}/activity`)

export const uploadAvatar = (file: File) => {
  const form = new FormData()
  form.append('file', file)
  return http.postForm<Me>('/users/me/avatar', form)
}

export const deleteAvatar = () => http.delete<Me>('/users/me/avatar')
