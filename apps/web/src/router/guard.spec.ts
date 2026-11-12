import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it } from 'vitest'

import {
  ADMIN_ACCESS_PERMISSIONS,
  GAMIFICATION_MANAGE_PERMISSIONS,
  GUEST_PERM,
  PERM,
} from '@/permissions'
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

  it('sends guests to forbidden when the guest permission is missing', () => {
    const r = route('/achievements', { guestPermission: GUEST_PERM.growth })

    expect(authGuard(r, guest)).toEqual({ name: 'forbidden' })
    expect(authGuard(r, { ...guest, guestPermissions: [] })).toEqual({ name: 'forbidden' })
  })

  it('allows guests holding the guest permission', () => {
    const r = route('/achievements', { guestPermission: GUEST_PERM.growth })

    expect(authGuard(r, { ...guest, guestPermissions: [GUEST_PERM.growth] })).toBe(true)
  })

  it('lets logged-in users bypass the guest permission', () => {
    const r = route('/achievements', { guestPermission: GUEST_PERM.growth })

    expect(authGuard(r, as([]))).toBe(true)
    expect(authGuard(r, superUser)).toBe(true)
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

describe('merged parent/child route meta', () => {
  // Mirrors the real /admin tree: the parent gates access via anyPermission and
  // children layer their own permission/anyPermission on top.
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: '/admin',
        component: { template: '<div />' },
        meta: { requiresAuth: true, anyPermission: ADMIN_ACCESS_PERMISSIONS },
        children: [
          {
            path: 'users',
            name: 'admin-users',
            component: { template: '<div />' },
            meta: { permission: PERM.user.read },
          },
          {
            path: 'gamification',
            name: 'gamification-admin',
            component: { template: '<div />' },
            meta: { anyPermission: GAMIFICATION_MANAGE_PERMISSIONS },
          },
        ],
      },
    ],
  })

  it('lets a user.read-only admin reach /admin/users but not /admin/gamification', () => {
    const state = as([PERM.user.read])
    expect(authGuard(router.resolve('/admin/users'), state)).toBe(true)
    expect(authGuard(router.resolve('/admin/gamification'), state)).toEqual({ name: 'forbidden' })
  })

  it('lets a super admin reach both', () => {
    expect(authGuard(router.resolve('/admin/users'), superUser)).toBe(true)
    expect(authGuard(router.resolve('/admin/gamification'), superUser)).toBe(true)
  })
})
