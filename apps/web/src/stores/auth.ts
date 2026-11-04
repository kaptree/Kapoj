import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import * as authApi from '@/api/auth'
import * as usersApi from '@/api/users'
import type { LoginRequest, Me, RegisterRequest, Session, TokenPair } from '@/api/types'

export const AUTH_STORAGE_KEY = 'kapoj.auth'

interface Persisted {
  accessToken: string | null
  refreshToken: string | null
  user: Me | null
}

function load(): Persisted {
  const empty = { accessToken: null, refreshToken: null, user: null }
  try {
    const raw = localStorage.getItem(AUTH_STORAGE_KEY)
    return raw ? { ...empty, ...(JSON.parse(raw) as Persisted) } : empty
  } catch {
    return empty
  }
}

export const useAuthStore = defineStore('auth', () => {
  const initial = load()
  const accessToken = ref<string | null>(initial.accessToken)
  const refreshToken = ref<string | null>(initial.refreshToken)
  const user = ref<Me | null>(initial.user)

  const isLoggedIn = computed(() => accessToken.value !== null)

  // Keep tabs in sync: another tab may rotate tokens or log out.
  if (typeof window !== 'undefined') {
    window.addEventListener('storage', (e) => {
      if (e.key !== AUTH_STORAGE_KEY) return
      const next = load()
      accessToken.value = next.accessToken
      refreshToken.value = next.refreshToken
      user.value = next.user
    })
  }

  function persist() {
    const data: Persisted = {
      accessToken: accessToken.value,
      refreshToken: refreshToken.value,
      user: user.value,
    }
    localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(data))
  }

  function setTokens(pair: TokenPair) {
    accessToken.value = pair.accessToken
    refreshToken.value = pair.refreshToken
    persist()
  }

  function setSession(session: Session) {
    user.value = session.user
    setTokens(session)
  }

  function setUser(me: Me) {
    user.value = me
    persist()
  }

  function clear() {
    accessToken.value = null
    refreshToken.value = null
    user.value = null
    localStorage.removeItem(AUTH_STORAGE_KEY)
  }

  async function login(input: LoginRequest) {
    setSession(await authApi.login(input))
  }

  async function register(input: RegisterRequest) {
    setSession(await authApi.register(input))
  }

  async function logout() {
    const token = refreshToken.value
    clear()
    if (token) await authApi.logout(token).catch(() => undefined)
  }

  async function fetchMe() {
    setUser(await usersApi.getMe())
  }

  return {
    accessToken,
    refreshToken,
    user,
    isLoggedIn,
    setTokens,
    setSession,
    setUser,
    clear,
    login,
    register,
    logout,
    fetchMe,
  }
})
