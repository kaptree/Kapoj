import type { RouteLocationRaw } from 'vue-router'

interface GuardTarget {
  fullPath: string
  meta: { requiresAuth?: unknown; guestOnly?: unknown }
}

export function authGuard(to: GuardTarget, isLoggedIn: boolean): true | RouteLocationRaw {
  if (to.meta.requiresAuth && !isLoggedIn) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.meta.guestOnly && isLoggedIn) {
    return { name: 'home' }
  }
  return true
}

/** Returns the redirect target if it stays on this site, otherwise "/". */
export function safeRedirect(value: unknown): string {
  if (typeof value !== 'string') return '/'
  if (!value.startsWith('/') || value.startsWith('//') || value.startsWith('/\\')) return '/'
  return value
}
