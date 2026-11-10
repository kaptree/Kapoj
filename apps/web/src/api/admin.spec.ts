import { beforeEach, describe, expect, it, vi } from 'vitest'

const http = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  delete: vi.fn(),
  postForm: vi.fn(),
  putForm: vi.fn(),
}))

vi.mock('./client', () => ({ http }))

import { resetAdminAIKey } from './admin'

describe('admin api', () => {
  beforeEach(() => {
    Object.values(http).forEach((fn) => fn.mockReset())
  })

  it('posts to the AI key reset endpoint', async () => {
    http.post.mockResolvedValue({
      configured: true,
      source: 'file',
      masked: 'abc…wxyz',
      fingerprint: 'fp',
      key: 'BASE64SECRET',
    })

    await expect(resetAdminAIKey()).resolves.toMatchObject({ key: 'BASE64SECRET' })
    expect(http.post).toHaveBeenCalledWith('/admin/ai/key/reset')
  })
})
