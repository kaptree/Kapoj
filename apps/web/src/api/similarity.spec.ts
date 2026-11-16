import { beforeEach, describe, expect, it, vi } from 'vitest'

const http = vi.hoisted(() => ({
  get: vi.fn(),
  put: vi.fn(),
  post: vi.fn(),
  getBlob: vi.fn(),
}))
vi.mock('./client', () => ({ http }))

import {
  downloadSimilarityCSV,
  getSimilarityMatch,
  getSimilarityOverview,
  getSimilarityRun,
  listSimilarityMatches,
  startSimilarityRuns,
  updateSimilaritySettings,
} from './similarity'

describe('similarity api', () => {
  beforeEach(() => {
    http.get.mockReset()
    http.put.mockReset()
    http.post.mockReset()
    http.getBlob.mockReset()
  })

  it('loads the contest overview', async () => {
    const overview = { contestId: 7, problems: [] }
    http.get.mockResolvedValue(overview)

    await expect(getSimilarityOverview(7)).resolves.toEqual(overview)
    expect(http.get).toHaveBeenCalledWith('/contests/7/similarity/overview')
  })

  it('saves problem settings', async () => {
    const input = {
      thresholdPercent: 90,
      ignoreCommonCode: true,
      baseLanguage: 'cpp',
      baseCode: '#include <bits/stdc++.h>',
    }
    const saved = { contestId: 7, problemId: 42, ...input }
    http.put.mockResolvedValue(saved)

    await expect(updateSimilaritySettings(7, 42, input)).resolves.toEqual(saved)
    expect(http.put).toHaveBeenCalledWith('/contests/7/similarity/problems/42', input)
  })

  it('starts a single-problem run', async () => {
    const runs = [{ id: 1, contestId: 7, problemId: 42, status: 'queued' }]
    http.post.mockResolvedValue(runs)

    await expect(startSimilarityRuns(7, 42)).resolves.toEqual(runs)
    expect(http.post).toHaveBeenCalledWith('/contests/7/similarity/runs', { problemId: 42 })
  })

  it('starts runs for every problem without a body', async () => {
    http.post.mockResolvedValue([])

    await expect(startSimilarityRuns(7)).resolves.toEqual([])
    expect(http.post).toHaveBeenCalledWith('/contests/7/similarity/runs', undefined)
  })

  it('fetches one run', async () => {
    const run = { id: 9, contestId: 7, problemId: 42, status: 'running' }
    http.get.mockResolvedValue(run)

    await expect(getSimilarityRun(7, 9)).resolves.toEqual(run)
    expect(http.get).toHaveBeenCalledWith('/contests/7/similarity/runs/9')
  })

  it('lists matches with problem, run and pagination filters', async () => {
    const page = { total: 1, items: [{ id: 3, similarityPercent: 91.5 }] }
    http.get.mockResolvedValue(page)

    await expect(
      listSimilarityMatches(7, { problemId: 42, runId: 9, page: 2, pageSize: 10 }),
    ).resolves.toEqual(page)
    expect(http.get).toHaveBeenCalledWith('/contests/7/similarity/matches', {
      params: { problemId: 42, runId: 9, page: 2, page_size: 10 },
    })
  })

  it('omits zero and empty match filters', async () => {
    const page = { total: 0, items: [] }
    http.get.mockResolvedValue(page)

    await expect(
      listSimilarityMatches(7, { problemId: 0, runId: 0, page: 0, pageSize: 0 }),
    ).resolves.toEqual(page)
    expect(http.get).toHaveBeenCalledWith('/contests/7/similarity/matches', {
      params: { problemId: undefined, runId: undefined, page: undefined, page_size: undefined },
    })
  })

  it('fetches match detail with both code payloads', async () => {
    const detail = { id: 3, originalCode: 'a', suspectCode: 'b', similarityPercent: 88.2 }
    http.get.mockResolvedValue(detail)

    await expect(getSimilarityMatch(7, 3)).resolves.toEqual(detail)
    expect(http.get).toHaveBeenCalledWith('/contests/7/similarity/matches/3')
  })

  it('downloads CSV as a blob with the current problem filter', async () => {
    const blob = new Blob(['original,suspect\n'], { type: 'text/csv' })
    http.getBlob.mockResolvedValue(blob)

    await expect(downloadSimilarityCSV(7, 42)).resolves.toBe(blob)
    expect(http.getBlob).toHaveBeenCalledWith('/contests/7/similarity/export.csv', {
      params: { problemId: 42 },
    })
  })

  it('downloads CSV without a problem filter', async () => {
    const blob = new Blob(['original,suspect\n'], { type: 'text/csv' })
    http.getBlob.mockResolvedValue(blob)

    await expect(downloadSimilarityCSV(7)).resolves.toBe(blob)
    expect(http.getBlob).toHaveBeenCalledWith('/contests/7/similarity/export.csv', {
      params: { problemId: undefined },
    })
  })
})
