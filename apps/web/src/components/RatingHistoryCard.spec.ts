import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/http'
import { getRatingHistory } from '@/api/leaderboard'
import type { RatingHistoryItem, RatingHistoryPage } from '@/api/types'
import RatingHistoryCard from './RatingHistoryCard.vue'

vi.mock('@/api/leaderboard', () => ({
  getRatingHistory: vi.fn(),
}))

function historyItem(overrides: Partial<RatingHistoryItem> = {}): RatingHistoryItem {
  return {
    contestId: 5,
    contestTitle: '春季公开赛',
    rank: 3,
    ratingBefore: 1500,
    ratingAfter: 1524,
    delta: 24,
    createdAt: '2026-01-01T12:00:00Z',
    ...overrides,
  }
}

function historyPage(items: RatingHistoryItem[] = []): RatingHistoryPage {
  return { total: items.length, items }
}

const routes = [
  {
    path: '/contests/:id(\\d+)',
    name: 'contest-detail',
    component: { template: '<div />' },
  },
]

async function mountCard(props: { userId?: number; rating?: number } = {}) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/')
  await router.isReady()
  const wrapper = mount(RatingHistoryCard, {
    props: { userId: 7, ...props },
    global: { plugins: [router, PrimeVue] },
  })
  await flushPromises()
  return wrapper
}

function polylinePoints(wrapper: Awaited<ReturnType<typeof mountCard>>) {
  const raw = wrapper.find('polyline').attributes('points') ?? ''
  return raw
    .trim()
    .split(/\s+/)
    .map((pair) => pair.split(',').map(Number))
}

beforeEach(() => {
  vi.mocked(getRatingHistory).mockReset()
  vi.mocked(getRatingHistory).mockResolvedValue(historyPage())
})

describe('RatingHistoryCard', () => {
  it('loads the latest 20 entries', async () => {
    await mountCard()

    expect(getRatingHistory).toHaveBeenCalledWith(7, { page: 1, pageSize: 20 })
  })

  it('shows a loading state while the history request is pending', async () => {
    let resolve!: (value: RatingHistoryPage) => void
    vi.mocked(getRatingHistory).mockReturnValue(
      new Promise<RatingHistoryPage>((r) => {
        resolve = r
      }),
    )
    const wrapper = await mountCard()

    expect(wrapper.text()).toContain('正在加载 Rating 历史')
    resolve(historyPage([historyItem()]))
    await flushPromises()

    expect(wrapper.text()).not.toContain('正在加载 Rating 历史')
    expect(wrapper.text()).toContain('春季公开赛')
  })

  it('renders empty state when no history', async () => {
    const wrapper = await mountCard()

    expect(wrapper.text()).toContain('Rating 历史')
    expect(wrapper.text()).toContain('暂无 Rating 记录')
    expect(wrapper.find('table').exists()).toBe(false)
  })

  it('renders positive, negative and zero delta colors', async () => {
    vi.mocked(getRatingHistory).mockResolvedValue(
      historyPage([
        historyItem({ contestId: 3, delta: 24, ratingAfter: 1524 }),
        historyItem({ contestId: 2, delta: -18, ratingAfter: 1500 }),
        historyItem({ contestId: 1, delta: 0, ratingAfter: 1518 }),
      ]),
    )
    const wrapper = await mountCard()

    const positive = wrapper.find('.delta--positive')
    const negative = wrapper.find('.delta--negative')
    const neutral = wrapper.find('.delta--neutral')
    expect(positive.text()).toBe('+24')
    expect(negative.text()).toBe('-18')
    expect(neutral.text()).toBe('0')
  })

  it('does not link masked private contests', async () => {
    vi.mocked(getRatingHistory).mockResolvedValue(
      historyPage([
        historyItem({ contestId: 0, contestTitle: '私密比赛' }),
        historyItem({ contestId: 0, contestTitle: '私密比赛' }),
      ]),
    )
    const wrapper = await mountCard()

    expect(wrapper.text()).toContain('私密比赛')
    expect(wrapper.find('a[href^="/contests/"]').exists()).toBe(false)
  })

  it('links public contests to their detail page', async () => {
    vi.mocked(getRatingHistory).mockResolvedValue(
      historyPage([historyItem({ contestId: 42, contestTitle: '秋季公开赛' })]),
    )
    const wrapper = await mountCard()

    const link = wrapper.find('a[href="/contests/42"]')
    expect(link.exists()).toBe(true)
    expect(link.text()).toBe('秋季公开赛')
  })

  it('renders an accessible SVG chart for two or more points in chronological order', async () => {
    vi.mocked(getRatingHistory).mockResolvedValue(
      historyPage([
        historyItem({
          contestId: 2,
          contestTitle: '第二场',
          ratingBefore: 1400,
          ratingAfter: 1600,
          delta: 200,
          createdAt: '2026-01-02T12:00:00Z',
        }),
        historyItem({
          contestId: 1,
          contestTitle: '第一场',
          ratingBefore: 1400,
          ratingAfter: 1400,
          delta: 0,
          createdAt: '2026-01-01T12:00:00Z',
        }),
      ]),
    )
    const wrapper = await mountCard()

    const chart = wrapper.find('svg.rating-chart')
    expect(chart.exists()).toBe(true)
    expect(chart.attributes('viewBox')).toBe('0 0 320 80')
    expect(chart.attributes('role')).toBe('img')
    expect(chart.attributes('aria-label')).toContain('Rating 历史曲线')
    expect(chart.attributes('aria-label')).toContain('2 场')

    const points = polylinePoints(wrapper)
    expect(points).toHaveLength(2)
    // Chronological order: the older 1400 point sits left of the newer 1600 point.
    expect(points[0][0]).toBeLessThan(points[1][0])
    // SVG y grows downward, so the older 1400 rating is drawn lower.
    expect(points[0][1]).toBeGreaterThan(points[1][1])
  })

  it('does not render the SVG chart for a single point', async () => {
    vi.mocked(getRatingHistory).mockResolvedValue(historyPage([historyItem()]))
    const wrapper = await mountCard()

    expect(wrapper.find('svg.rating-chart').exists()).toBe(false)
    expect(wrapper.find('table').exists()).toBe(true)
    expect(wrapper.text()).toContain('春季公开赛')
  })

  it('shows the current rating in the header when provided', async () => {
    const wrapper = await mountCard({ rating: 1680 })

    expect(wrapper.text()).toContain('当前 Rating')
    expect(wrapper.text()).toContain('1680')
  })

  it('shows an error state and retries', async () => {
    vi.mocked(getRatingHistory)
      .mockRejectedValueOnce(new ApiError(500, 50000, '服务器错误'))
      .mockResolvedValueOnce(historyPage([historyItem()]))
    const wrapper = await mountCard()

    expect(wrapper.text()).toContain('服务器错误')
    const retry = wrapper.findAll('button').find((button) => button.text().trim() === '重试')
    expect(retry).toBeTruthy()
    await retry!.trigger('click')
    await flushPromises()

    expect(getRatingHistory).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('春季公开赛')
  })

  it('reloads when the user id changes', async () => {
    vi.mocked(getRatingHistory).mockImplementation(async (userId) =>
      historyPage([
        historyItem({
          contestId: Number(userId),
          contestTitle: `用户 ${userId} 的比赛`,
        }),
      ]),
    )
    const wrapper = await mountCard({ userId: 7 })

    expect(wrapper.text()).toContain('用户 7 的比赛')
    await wrapper.setProps({ userId: 8 })
    await flushPromises()

    expect(getRatingHistory).toHaveBeenLastCalledWith(8, { page: 1, pageSize: 20 })
    expect(wrapper.text()).toContain('用户 8 的比赛')
  })
})
