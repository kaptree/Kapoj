import { describe, expect, it } from 'vitest'

import type { Role } from '@/api/types'
import { authGuard, hasRole, safeRedirect } from './guard'

const route = (fullPath: string, meta: Record<string, unknown> = {}) => ({ fullPath, meta })
const guest = { isLoggedIn: false, role: null }
const as = (role: Role) => ({ isLoggedIn: true, role })

describe('authGuard', () => {
  it('sends guests on protected routes to login with a redirect', () => {
    expect(authGuard(route('/settings/profile?tab=1', { requiresAuth: true }), guest)).toEqual({
      name: 'login',
      query: { redirect: '/settings/profile?tab=1' },
    })
  })

  it('sends logged-in users away from guest-only routes', () => {
    expect(authGuard(route('/login', { guestOnly: true }), as('user'))).toEqual({ name: 'home' })
  })

  it('allows everything else', () => {
    expect(authGuard(route('/settings/profile', { requiresAuth: true }), as('user'))).toBe(true)
    expect(authGuard(route('/login', { guestOnly: true }), guest)).toBe(true)
    expect(authGuard(route('/'), guest)).toBe(true)
  })

  it('redirects users lacking the role to forbidden', () => {
    const r = route('/problems/new', { requiresAuth: true, roles: ['problem_admin'] })
    expect(authGuard(r, as('user'))).toEqual({ name: 'forbidden' })
    expect(authGuard(r, as('contest_admin'))).toEqual({ name: 'forbidden' })
    expect(authGuard(r, as('problem_admin'))).toBe(true)
  })

  it('lets super_admin through any role route', () => {
    expect(authGuard(route('/x', { roles: ['problem_admin'] }), as('super_admin'))).toBe(true)
  })

  it('login redirect wins over the role check', () => {
    expect(authGuard(route('/problems/new', { roles: ['problem_admin'] }), guest)).toEqual({
      name: 'login',
      query: { redirect: '/problems/new' },
    })
  })
})

describe('hasRole', () => {
  it('matches the server rule: super_admin always passes, others must be listed', () => {
    expect(hasRole('super_admin', ['problem_admin'])).toBe(true)
    expect(hasRole('problem_admin', ['problem_admin'])).toBe(true)
    expect(hasRole('contest_admin', ['problem_admin'])).toBe(false)
    expect(hasRole('user', ['problem_admin'])).toBe(false)
    expect(hasRole(null, ['problem_admin'])).toBe(false)
    expect(hasRole('user', [])).toBe(false)
  })
})

describe('safeRedirect', () => {
  it('accepts in-site paths', () => {
    expect(safeRedirect('/users/1')).toBe('/users/1')
  })

  it('rejects anything that could leave the site', () => {
    for (const bad of [
      'https://evil.com',
      '//evil.com',
      '/\\evil.com',
      'javascript:alert(1)',
      '',
      undefined,
      ['/a'],
    ]) {
      expect(safeRedirect(bad)).toBe('/')
    }
  })
})
