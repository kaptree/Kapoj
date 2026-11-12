import { beforeEach, describe, expect, it, vi } from 'vitest'

const http = vi.hoisted(() => ({
  get: vi.fn(),
}))

vi.mock('./client', () => ({ http }))

import { getGuestPermissions } from './guest'

describe('guest api', () => {
  beforeEach(() => {
    Object.values(http).forEach((fn) => fn.mockReset())
  })

  it('requests the public guest permissions', async () => {
    http.get.mockResolvedValue({ permissions: [] })
    await expect(getGuestPermissions()).resolves.toEqual({ permissions: [] })
    expect(http.get).toHaveBeenCalledWith('/guest/permissions')
  })
})
