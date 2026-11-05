import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { getContestRanklist } from '@/api/contests'
import { ApiError } from '@/api/http'
import type { ContestRanklist } from '@/api/types'
import ContestRanklistView from './ContestRanklist.vue'

vi.mock('@/api/contests', () => ({
  getContestRanklist: vi.fn(),
}))

function ranklist(overrides: Partial<ContestRanklist> = {}): ContestRanklist {
  return {
    contestId: 5,
    mode: 'icpc',
    status: 'ended',
    generatedAt: '2026-01-01T12:00:00Z',
    selfOnly: false,
    problems: [
      { problemId: 1, displayId: 'A', title: 'Two Sum' },
      { problemId: 2, displayId: 'B', title: 'Add Two Numbers' },
    ],
    rows: [
      {
        rank: 1,
        userId: 7,
        username: 'alice',
        nickname: 'Alice',
        equippedBadges: [],
        equippedTitle: null,
        solved: 2,
        penalty: 120,
        totalScore: 200,
        lastAcceptedAt: '2026-01-01T11:00:00Z',
        problems: [
          {
            status: 'accepted',
            attempts: 2,
            score: 100,
            acceptedAt: '2026-01-01T10:00:00Z',
          },
          { status: 'wrong_answer', attempts: 1, score: 0 },
        ],
      },
      {
        rank: 2,
        userId: 8,
        username: 'bob',
        nickname: 'Bob',
        equippedBadges: [],
        equippedTitle: null,
        solved: 0,
        penalty: 0,
        totalScore: 0,
        problems: [
          { attempts: 0, score: 0 },
          { attempts: 0, score: 0 },
        ],
      },
    ],
    ...overrides,
  }
}

async function mountRanklist(props: { contestId?: number; refreshTick?: number } = {}) {
  const wrapper = mount(ContestRanklistView, {
    props: { contestId: 5, refreshTick: 0, ...props },
    global: { plugins: [PrimeVue] },
  })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  vi.mocked(getContestRanklist).mockReset()
  vi.mocked(getContestRanklist).mockResolvedValue(ranklist())
})

describe('ContestRanklist', () => {
  it('renders ICPC rows with solved/penalty and cell attempts', async () => {
    const wrapper = await mountRanklist()

    expect(getContestRanklist).toHaveBeenCalledWith(5)
    expect(wrapper.text()).toContain('ICPC')
    expect(wrapper.text()).toContain('Alice')
    expect(wrapper.text()).toContain('2 / 120')
    expect(wrapper.text()).toContain('AC (2)')
    expect(wrapper.text()).toContain('-1')
    expect(wrapper.text()).toContain('Bob')
    expect(wrapper.findAll('.accepted')).toHaveLength(1)
    wrapper.unmount()
  })

  it('renders IOI scores and the self-only warning', async () => {
    vi.mocked(getContestRanklist).mockResolvedValue(ranklist({ mode: 'oi', selfOnly: true }))
    const wrapper = await mountRanklist()

    expect(wrapper.text()).toContain('OI')
    expect(wrapper.text()).toContain('总分')
    expect(wrapper.text()).toContain('200')
    expect(wrapper.text()).toContain('OI 赛时仅显示本人成绩，赛后公布完整榜单')
    wrapper.unmount()
  })

  it('reloads when refreshTick changes', async () => {
    const wrapper = await mountRanklist()
    expect(getContestRanklist).toHaveBeenCalledTimes(1)

    await wrapper.setProps({ refreshTick: 1 })
    await flushPromises()
    expect(getContestRanklist).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('shows an error with retry and recovers', async () => {
    vi.mocked(getContestRanklist)
      .mockRejectedValueOnce(new ApiError(500, 50000, '加载榜单失败'))
      .mockResolvedValueOnce(ranklist())
    const wrapper = await mountRanklist()

    expect(wrapper.text()).toContain('加载榜单失败')
    const retry = wrapper.findAll('button').find((item) => item.text().trim() === '重试')
    expect(retry).toBeTruthy()
    await retry!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Alice')
    wrapper.unmount()
  })
})
