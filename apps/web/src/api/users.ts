import { http } from './client'
import type { Me, Profile, UpdateMeRequest } from './types'

export const getMe = () => http.get<Me>('/users/me')

export const updateMe = (input: UpdateMeRequest) => http.put<Me>('/users/me', input)

export const getProfile = (id: number | string) => http.get<Profile>(`/users/${id}/profile`)
