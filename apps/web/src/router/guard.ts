import type { RouteLocationRaw } from 'vue-router'

interface GuardTarget {
  fullPath: string
  meta: {
    requiresAuth?: unknown
    guestOnly?: unknown
    permission?: unknown
    anyPermission?: unknown
    guestPermission?: unknown
  }
}

export interface AuthState {
  isLoggedIn: boolean
  permissions: string[]
  isSuperAdmin: boolean
  /** Permissions granted to anonymous visitors; absent means none. */
  guestPermissions?: string[]
}

/** Super admin passes, otherwise the code must be present in the permission set. */
export function canPermission(state: AuthState, code: string): boolean {
  if (state.isSuperAdmin) return true
  return state.permissions.includes(code)
}

/** Super admin passes, otherwise any listed code may be present. */
export function canAnyPermission(state: AuthState, codes: readonly string[]): boolean {
  if (state.isSuperAdmin) return true
  return codes.some((code) => state.permissions.includes(code))
}

export function authGuard(to: GuardTarget, state: AuthState): true | RouteLocationRaw {
  const permission = typeof to.meta.permission === 'string' ? to.meta.permission : ''
  const anyPermission = Array.isArray(to.meta.anyPermission)
    ? (to.meta.anyPermission as string[])
    : []
  const restricted = permission !== '' || anyPermission.length > 0
  if ((to.meta.requiresAuth || restricted) && !state.isLoggedIn) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.meta.guestOnly && state.isLoggedIn) {
    return { name: 'home' }
  }
  const guestPermission = typeof to.meta.guestPermission === 'string' ? to.meta.guestPermission : ''
  if (
    !state.isLoggedIn &&
    guestPermission !== '' &&
    !(state.guestPermissions ?? []).includes(guestPermission)
  ) {
    return { name: 'forbidden' }
  }
  if (permission !== '' && !canPermission(state, permission)) {
    return { name: 'forbidden' }
  }
  if (anyPermission.length > 0 && !canAnyPermission(state, anyPermission)) {
    return { name: 'forbidden' }
  }
  return true
}

/** Returns the redirect target if it stays on this site, otherwise "/". */
export function safeRedirect(value: unknown): string {
  if (typeof value !== 'string') return '/'
  if (!value.startsWith('/') || value.startsWith('//') || value.startsWith('/\\')) return '/'
  return value
}
