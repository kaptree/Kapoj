import { http } from './client'
import type {
  ChangePasswordInput,
  LoginRequest,
  RegisterRequest,
  Session,
  SessionView,
} from './types'

export const register = (input: RegisterRequest) => http.post<Session>('/auth/register', input)

export const login = (input: LoginRequest) => http.post<Session>('/auth/login', input)

export const logout = (refreshToken: string) => http.post<null>('/auth/logout', { refreshToken })

export const changePassword = (input: ChangePasswordInput) =>
  http.put<null>('/users/me/password', input)

export const listSessions = () => http.get<SessionView[]>('/users/me/sessions')

export const revokeSession = (id: string) =>
  http.delete<null>(`/users/me/sessions/${encodeURIComponent(id)}`)

export const logoutOthers = (sessionId: string) =>
  http.post<null>('/users/me/sessions/logout-others', { sessionId })

export const logoutAll = () => http.post<null>('/users/me/sessions/logout-all')
