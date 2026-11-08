import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

import { AUTH_STORAGE_KEY, useAuthStore } from './auth'
import type { Me } from '@/api/types'

const authApi = vi.hoisted(() => ({
  login: vi.fn(),
  register: vi.fn(),
  logout: vi.fn(),
}))

const usersApi = vi.hoisted(() => ({
  getMe: vi.fn(),
  updateMe: vi.fn(),
}))

vi.mock('@/api/auth', () => authApi)
vi.mock('@/api/users', () => usersApi)

const me: Me = {
  id: 1,
  username: 'alice',
  nickname: 'Alice',
  email: 'alice@example.com',
  avatarUrl: '',
  avatarUploaded: false,
  role: 'user',
  roles: [],
  permissions: [],
  isSuperAdmin: false,
  rating: 1500,
  coins: 0,
  bio: '',
  status: 'active',
  createdAt: '2026-11-04T00:00:00Z',
}

describe('auth store', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
    authApi.login.mockReset()
    authApi.register.mockReset()
    authApi.logout.mockReset()
    usersApi.getMe.mockReset()
    usersApi.updateMe.mockReset()
  })

  it('starts logged out', () => {
    const store = useAuthStore()
    expect(store.isLoggedIn).toBe(false)
    expect(store.user).toBeNull()
  })

  it('persists a session and restores it in a new store', () => {
    useAuthStore().setSession({ accessToken: 'a', refreshToken: 'r', user: me })

    setActivePinia(createPinia())
    const restored = useAuthStore()
    expect(restored.isLoggedIn).toBe(true)
    expect(restored.accessToken).toBe('a')
    expect(restored.refreshToken).toBe('r')
    expect(restored.user?.username).toBe('alice')
  })

  it('setTokens keeps the user', () => {
    const store = useAuthStore()
    store.setSession({ accessToken: 'a', refreshToken: 'r', user: me })
    store.setTokens({ accessToken: 'a2', refreshToken: 'r2' })
    expect(store.accessToken).toBe('a2')
    expect(store.user?.id).toBe(1)
    expect(JSON.parse(localStorage.getItem(AUTH_STORAGE_KEY)!).refreshToken).toBe('r2')
  })

  it('clear removes everything', () => {
    const store = useAuthStore()
    store.setSession({ accessToken: 'a', refreshToken: 'r', user: me })
    store.clear()
    expect(store.isLoggedIn).toBe(false)
    expect(localStorage.getItem(AUTH_STORAGE_KEY)).toBeNull()
  })

  it('ignores corrupt storage', () => {
    localStorage.setItem(AUTH_STORAGE_KEY, '{not json')
    const store = useAuthStore()
    expect(store.isLoggedIn).toBe(false)
  })

  it('follows changes made by another tab', () => {
    const store = useAuthStore()
    store.setSession({ accessToken: 'a', refreshToken: 'r', user: me })

    const rotated = JSON.stringify({ accessToken: 'a2', refreshToken: 'r2', user: me })
    localStorage.setItem(AUTH_STORAGE_KEY, rotated)
    window.dispatchEvent(new StorageEvent('storage', { key: AUTH_STORAGE_KEY, newValue: rotated }))
    expect(store.refreshToken).toBe('r2')

    localStorage.removeItem(AUTH_STORAGE_KEY)
    window.dispatchEvent(new StorageEvent('storage', { key: AUTH_STORAGE_KEY, newValue: null }))
    expect(store.isLoggedIn).toBe(false)
  })

  it('can/canAny respects permissions and super admin', () => {
    const store = useAuthStore()
    store.setSession({
      accessToken: 'a',
      refreshToken: 'r',
      user: { ...me, permissions: ['problem.update', 'contest.create'] },
    })

    expect(store.isSuperAdmin).toBe(false)
    expect(store.can('problem.update')).toBe(true)
    expect(store.can('problem.create')).toBe(false)
    expect(store.canAny(['problem.create', 'contest.create'])).toBe(true)
    expect(store.canAny(['problem.create', 'contest.delete'])).toBe(false)

    store.setUser({ ...me, isSuperAdmin: true, permissions: [] })
    expect(store.isSuperAdmin).toBe(true)
    expect(store.can('anything.at.all')).toBe(true)
    expect(store.canAny(['anything.at.all'])).toBe(true)
  })

  it('treats a stale cache without permissions as empty', () => {
    const store = useAuthStore()
    const stale = { ...me, permissions: undefined as unknown as string[] }
    store.setSession({ accessToken: 'a', refreshToken: 'r', user: stale })
    expect(store.can('problem.update')).toBe(false)
    expect(store.canAny(['problem.update'])).toBe(false)
  })

  it('login refreshes Me after establishing the session', async () => {
    const store = useAuthStore()
    authApi.login.mockResolvedValue({ accessToken: 'a', refreshToken: 'r', user: me })
    usersApi.getMe.mockResolvedValue({ ...me, permissions: ['problem.create'] })

    await store.login({ account: 'alice', password: 'secret' })

    expect(authApi.login).toHaveBeenCalledWith({ account: 'alice', password: 'secret' })
    expect(usersApi.getMe).toHaveBeenCalledTimes(1)
    expect(store.isLoggedIn).toBe(true)
    expect(store.user?.permissions).toEqual(['problem.create'])
  })

  it('register refreshes Me after establishing the session', async () => {
    const store = useAuthStore()
    authApi.register.mockResolvedValue({ accessToken: 'a', refreshToken: 'r', user: me })
    usersApi.getMe.mockResolvedValue({ ...me, isSuperAdmin: true })

    await store.register({ username: 'alice', email: 'a@b.c', password: 'secret' })

    expect(usersApi.getMe).toHaveBeenCalledTimes(1)
    expect(store.user?.isSuperAdmin).toBe(true)
  })

  it('keeps the session when the Me refresh fails', async () => {
    const store = useAuthStore()
    authApi.login.mockResolvedValue({ accessToken: 'a', refreshToken: 'r', user: me })
    usersApi.getMe.mockRejectedValue(new Error('boom'))

    await store.login({ account: 'alice', password: 'secret' })

    expect(store.isLoggedIn).toBe(true)
    expect(store.user?.username).toBe('alice')
  })

  it('updateProfile preserves roles/permissions/isSuperAdmin', async () => {
    const store = useAuthStore()
    store.setSession({
      accessToken: 'a',
      refreshToken: 'r',
      user: {
        ...me,
        roles: [{ id: 1, code: 'content_admin', name: '内容管理员' }],
        permissions: ['problem.update'],
        isSuperAdmin: false,
      },
    })
    usersApi.updateMe.mockResolvedValue({ ...me, nickname: 'Alice2', bio: 'New bio' })

    await store.updateProfile({ nickname: 'Alice2', bio: 'New bio' })

    expect(usersApi.updateMe).toHaveBeenCalledWith({ nickname: 'Alice2', bio: 'New bio' })
    expect(store.user?.nickname).toBe('Alice2')
    expect(store.user?.bio).toBe('New bio')
    expect(store.user?.roles).toEqual([{ id: 1, code: 'content_admin', name: '内容管理员' }])
    expect(store.user?.permissions).toEqual(['problem.update'])
    expect(store.user?.isSuperAdmin).toBe(false)
  })
})
