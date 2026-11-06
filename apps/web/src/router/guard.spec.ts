import { describe, expect, it } from 'vitest'

import { authGuard, canAnyPermission, canPermission, safeRedirect } from './guard'

const route = (fullPath: string, meta: Record<string, unknown> = {}) => ({ fullPath, meta })
const guest = { isLoggedIn: false, permissions: [], isSuperAdmin: false }
const as = (permissions: string[]) => ({ isLoggedIn: true, permissions, isSuperAdmin: false })
const superUser = { isLoggedIn: true, permissions: [], isSuperAdmin: true }

describe('authGuard', () => {
  it('sends guests on protected routes to login with a redirect', () => {
    expect(authGuard(route('/settings/profile?tab=1', { requiresAuth: true }), guest)).toEqual({
      name: 'login',
      query: { redirect: '/settings/profile?tab=1' },
    })
  })

  it('sends logged-in users away from guest-only routes', () => {
    expect(authGuard(route('/login', { guestOnly: true }), as([]))).toEqual({ name: 'home' })
  })

  it('allows everything else', () => {
    expect(authGuard(route('/settings/profile', { requiresAuth: true }), as([]))).toBe(true)
    expect(authGuard(route('/login', { guestOnly: true }), guest)).toBe(true)
    expect(authGuard(route('/'), guest)).toBe(true)
  })

  it('permission route requires login then checks permission', () => {
    const r = route('/problems/new', { permission: 'problem.create' })
    expect(authGuard(r, guest)).toEqual({ name: 'login', query: { redirect: '/problems/new' } })
    expect(authGuard(r, as(['problem.create']))).toBe(true)
    expect(authGuard(r, as(['contest.create']))).toEqual({ name: 'forbidden' })
  })

  it('anyPermission allows one match; super passes', () => {
    const r = route('/admin/gamification', {
      anyPermission: ['gamification.achievement.manage', 'gamification.task.manage'],
    })
    expect(authGuard(r, as(['gamification.task.manage']))).toBe(true)
    expect(authGuard(r, as(['problem.create']))).toEqual({ name: 'forbidden' })
    expect(authGuard(r, superUser)).toBe(true)
  })

  it('no permission redirects to forbidden', () => {
    const r = route('/admin/gamification', { anyPermission: ['gamification.achievement.manage'] })
    expect(authGuard(r, as([]))).toEqual({ name: 'forbidden' })
  })

  it('removes role-based authorization', () => {
    // A legacy `roles` meta no longer restricts access.
    expect(authGuard(route('/x', { roles: ['problem_admin'] }), as([]))).toBe(true)
  })
})

describe('canPermission', () => {
  it('matches the permission set; super always passes', () => {
    expect(canPermission(as(['problem.create']), 'problem.create')).toBe(true)
    expect(canPermission(as(['problem.create']), 'problem.update')).toBe(false)
    expect(canPermission(as([]), 'problem.create')).toBe(false)
    expect(canPermission(superUser, 'anything')).toBe(true)
    expect(canPermission(guest, 'problem.create')).toBe(false)
  })
})

describe('canAnyPermission', () => {
  it('passes when any code matches; super always passes', () => {
    expect(canAnyPermission(as(['problem.create']), ['problem.create', 'contest.create'])).toBe(
      true,
    )
    expect(canAnyPermission(as(['problem.create']), ['contest.create'])).toBe(false)
    expect(canAnyPermission(as([]), [])).toBe(false)
    expect(canAnyPermission(superUser, ['contest.create'])).toBe(true)
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
