import { describe, expect, it } from 'vitest'

import { authGuard, safeRedirect } from './guard'

const route = (fullPath: string, meta: Record<string, unknown> = {}) => ({ fullPath, meta })

describe('authGuard', () => {
  it('sends guests on protected routes to login with a redirect', () => {
    expect(authGuard(route('/settings/profile?tab=1', { requiresAuth: true }), false)).toEqual({
      name: 'login',
      query: { redirect: '/settings/profile?tab=1' },
    })
  })

  it('sends logged-in users away from guest-only routes', () => {
    expect(authGuard(route('/login', { guestOnly: true }), true)).toEqual({ name: 'home' })
  })

  it('allows everything else', () => {
    expect(authGuard(route('/settings/profile', { requiresAuth: true }), true)).toBe(true)
    expect(authGuard(route('/login', { guestOnly: true }), false)).toBe(true)
    expect(authGuard(route('/'), false)).toBe(true)
  })
})

describe('safeRedirect', () => {
  it('accepts in-site paths', () => {
    expect(safeRedirect('/users/1')).toBe('/users/1')
  })

  it('rejects anything that could leave the site', () => {
    for (const bad of ['https://evil.com', '//evil.com', '/\\evil.com', 'javascript:alert(1)', '', undefined, ['/a']]) {
      expect(safeRedirect(bad)).toBe('/')
    }
  })
})
