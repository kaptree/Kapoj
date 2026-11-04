import { http } from './client'
import type { LoginRequest, RegisterRequest, Session } from './types'

export const register = (input: RegisterRequest) => http.post<Session>('/auth/register', input)

export const login = (input: LoginRequest) => http.post<Session>('/auth/login', input)

export const logout = (refreshToken: string) => http.post<null>('/auth/logout', { refreshToken })
