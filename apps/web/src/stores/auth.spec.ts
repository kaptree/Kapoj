import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

import { AUTH_STORAGE_KEY, useAuthStore } from './auth'
import type { Me } from '@/api/types'

const me: Me = {
  id: 1,
  username: 'alice',
  nickname: 'Alice',
  email: 'alice@example.com',
  avatarUrl: '',
  role: 'user',
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
})
