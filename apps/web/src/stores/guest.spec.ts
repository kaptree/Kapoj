import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { GUEST_PERM } from '@/permissions'

import { useGuestStore } from './guest'

const guestApi = vi.hoisted(() => ({
  getGuestPermissions: vi.fn(),
}))

vi.mock('@/api/guest', () => guestApi)

describe('guest store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    guestApi.getGuestPermissions.mockReset()
  })

  it('starts empty and unloaded so growth stays hidden by default', () => {
    const store = useGuestStore()

    expect(store.permissions).toEqual([])
    expect(store.loaded).toBe(false)
    expect(store.can(GUEST_PERM.growth)).toBe(false)
  })

  it('loads permissions once and shares the in-flight request', async () => {
    let resolve!: (value: { permissions: string[] }) => void
    guestApi.getGuestPermissions.mockReturnValue(
      new Promise((r) => {
        resolve = r
      }),
    )
    const store = useGuestStore()

    const first = store.ensureLoaded()
    const second = store.ensureLoaded()

    expect(guestApi.getGuestPermissions).toHaveBeenCalledTimes(1)
    resolve({ permissions: [GUEST_PERM.growth] })
    await Promise.all([first, second])

    expect(store.permissions).toEqual([GUEST_PERM.growth])
    expect(store.loaded).toBe(true)

    await store.ensureLoaded()
    expect(guestApi.getGuestPermissions).toHaveBeenCalledTimes(1)
  })

  it('saves the returned permissions', async () => {
    guestApi.getGuestPermissions.mockResolvedValue({ permissions: [GUEST_PERM.growth] })
    const store = useGuestStore()

    await store.ensureLoaded()

    expect(store.permissions).toEqual([GUEST_PERM.growth])
    expect(store.loaded).toBe(true)
    expect(store.can(GUEST_PERM.growth)).toBe(true)
  })

  it('marks loaded and keeps permissions empty when the request fails', async () => {
    guestApi.getGuestPermissions.mockRejectedValue(new Error('network'))
    const store = useGuestStore()

    await store.ensureLoaded()

    expect(store.loaded).toBe(true)
    expect(store.permissions).toEqual([])
    expect(store.can(GUEST_PERM.growth)).toBe(false)

    await store.ensureLoaded()
    expect(guestApi.getGuestPermissions).toHaveBeenCalledTimes(1)
  })
})
