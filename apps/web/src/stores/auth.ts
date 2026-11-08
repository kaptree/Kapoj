import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import * as authApi from '@/api/auth'
import * as usersApi from '@/api/users'
import type {
  LoginRequest,
  Me,
  RegisterRequest,
  Session,
  TokenPair,
  UpdateMeRequest,
} from '@/api/types'

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
  const isSuperAdmin = computed(() => user.value?.isSuperAdmin === true)

  /** True when the user is super admin or holds the given permission code. */
  function can(code: string): boolean {
    if (isSuperAdmin.value) return true
    return (user.value?.permissions ?? []).includes(code)
  }

  /** True when the user is super admin or holds any of the given codes. */
  function canAny(codes: readonly string[]): boolean {
    if (isSuperAdmin.value) return true
    const permissions = user.value?.permissions ?? []
    return codes.some((code) => permissions.includes(code))
  }

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

  /**
   * Persist a user payload while keeping the RBAC fields that some endpoints
   * (profile update, avatar upload/remove) omit from their responses. Falling
   * back to the incoming payload keeps the first login working when there is
   * no prior user to inherit from.
   */
  function setUserPreservingRbac(me: Me) {
    const current = user.value
    setUser({
      ...me,
      roles: current?.roles ?? me.roles,
      permissions: current?.permissions ?? me.permissions,
      isSuperAdmin: current?.isSuperAdmin ?? me.isSuperAdmin,
    })
  }

  function clear() {
    accessToken.value = null
    refreshToken.value = null
    user.value = null
    localStorage.removeItem(AUTH_STORAGE_KEY)
  }

  async function login(input: LoginRequest) {
    setSession(await authApi.login(input))
    // The session user may carry empty RBAC fields; refresh them, but never
    // let a failed refresh block the login itself.
    await fetchMe().catch(() => undefined)
  }

  async function register(input: RegisterRequest) {
    setSession(await authApi.register(input))
    await fetchMe().catch(() => undefined)
  }

  async function logout() {
    const token = refreshToken.value
    clear()
    if (token) await authApi.logout(token).catch(() => undefined)
  }

  async function fetchMe() {
    setUser(await usersApi.getMe())
  }

  /** Saves the profile while preserving the current RBAC fields. */
  async function updateProfile(input: UpdateMeRequest): Promise<void> {
    setUserPreservingRbac(await usersApi.updateMe(input))
  }

  return {
    accessToken,
    refreshToken,
    user,
    isLoggedIn,
    isSuperAdmin,
    can,
    canAny,
    setTokens,
    setSession,
    setUser,
    setUserPreservingRbac,
    clear,
    login,
    register,
    logout,
    fetchMe,
    updateProfile,
  }
})
