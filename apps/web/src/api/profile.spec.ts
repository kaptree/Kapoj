import { beforeEach, describe, expect, it, vi } from 'vitest'

const http = vi.hoisted(() => ({
  get: vi.fn(),
  put: vi.fn(),
  delete: vi.fn(),
}))

vi.mock('./client', () => ({ http }))

import { listUserCertificates } from './certificates'
import {
  deleteMyEvaluation,
  deleteUserEvaluation,
  listUserEvaluations,
  saveMyEvaluation,
} from './social'
import { getUserActivity } from './users'

describe('profile api', () => {
  beforeEach(() => {
    Object.values(http).forEach((fn) => fn.mockReset())
  })

  it('requests a user activity calendar', async () => {
    http.get.mockResolvedValue({ days: [] })
    await expect(getUserActivity(7)).resolves.toEqual({ days: [] })
    expect(http.get).toHaveBeenCalledWith('/users/7/activity')
  })

  it('requests a user evaluation page', async () => {
    http.get.mockResolvedValue({ total: 0, items: [], cloud: [] })
    await expect(listUserEvaluations(7, 2, 10)).resolves.toEqual({
      total: 0,
      items: [],
      cloud: [],
    })
    expect(http.get).toHaveBeenCalledWith('/users/7/evaluations', {
      params: { page: 2, page_size: 10 },
    })
  })

  it('saves the current user evaluation with the content body', async () => {
    http.put.mockResolvedValue({ id: 5, content: '大佬' })
    await expect(saveMyEvaluation(7, '大佬')).resolves.toEqual({ id: 5, content: '大佬' })
    expect(http.put).toHaveBeenCalledWith('/users/7/evaluations/mine', { content: '大佬' })
  })

  it('deletes the current user evaluation', async () => {
    http.delete.mockResolvedValue(null)
    await expect(deleteMyEvaluation(7)).resolves.toBeNull()
    expect(http.delete).toHaveBeenCalledWith('/users/7/evaluations/mine')
  })

  it('deletes an evaluation as the profile owner', async () => {
    http.delete.mockResolvedValue(null)
    await expect(deleteUserEvaluation(7, 5)).resolves.toBeNull()
    expect(http.delete).toHaveBeenCalledWith('/users/7/evaluations/5')
  })

  it('requests a user approved certificate page', async () => {
    http.get.mockResolvedValue({ total: 0, items: [] })
    await expect(listUserCertificates(7)).resolves.toEqual({ total: 0, items: [] })
    expect(http.get).toHaveBeenCalledWith('/users/7/certificates')
  })
})
