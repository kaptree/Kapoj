import { beforeEach, describe, expect, it, vi } from 'vitest'

const http = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('./client', () => ({ http }))

import { listWealthLeaderboard } from './leaderboard'

describe('leaderboard api', () => {
  beforeEach(() => {
    http.get.mockReset()
    http.post.mockReset()
  })

  it('requests the wealth leaderboard with page params', async () => {
    http.get.mockResolvedValue({ total: 0, items: [] })
    await expect(listWealthLeaderboard({ page: 2, pageSize: 10 })).resolves.toEqual({
      total: 0,
      items: [],
    })
    expect(http.get).toHaveBeenCalledWith('/leaderboard/wealth', {
      params: { page: 2, page_size: 10 },
    })
  })
})
