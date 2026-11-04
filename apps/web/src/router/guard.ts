import type { RouteLocationRaw } from 'vue-router'

import type { Role } from '@/api/types'

interface GuardTarget {
  fullPath: string
  meta: { requiresAuth?: unknown; guestOnly?: unknown; roles?: unknown }
}

export interface AuthState {
  isLoggedIn: boolean
  role: Role | null
}

/** Same rule as the server: super_admin always passes, others must be listed. */
export function hasRole(role: Role | null, allowed: Role[]): boolean {
  if (role === null) return false
  return role === 'super_admin' || allowed.includes(role)
}

export function authGuard(to: GuardTarget, state: AuthState): true | RouteLocationRaw {
  const roles = Array.isArray(to.meta.roles) ? (to.meta.roles as Role[]) : []
  if ((to.meta.requiresAuth || roles.length > 0) && !state.isLoggedIn) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.meta.guestOnly && state.isLoggedIn) {
    return { name: 'home' }
  }
  if (roles.length > 0 && !hasRole(state.role, roles)) {
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
