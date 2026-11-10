import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Paginator from 'primevue/paginator'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/http'
import {
  getRatingSeason,
  listActivityLeaderboard,
  listRatingLeaderboard,
  listSharingLeaderboard,
  listWealthLeaderboard,
} from '@/api/leaderboard'
import type {
  ActivityLeaderboardRow,
  RatingLeaderboardRow,
  SharingLeaderboardRow,
  WealthLeaderboardRow,
} from '@/api/types'
import RatingLevelName from '@/components/RatingLevelName.vue'
import LeaderboardView from './LeaderboardView.vue'

vi.mock('@/api/leaderboard', () => ({
  listRatingLeaderboard: vi.fn(),
  listActivityLeaderboard: vi.fn(),
  listWealthLeaderboard: vi.fn(),
  listSharingLeaderboard: vi.fn(),
  getRatingSeason: vi.fn(),
}))

const ratingRow: RatingLeaderboardRow = {
  rank: 1,
  userId: 2,
  username: 'm2bsolver1',
  nickname: 'm2bsolver1',
  score: 1516,
  level: { key: 'cyan', name: '青名', color: '#03A89E', accentColor: '' },
  equippedBadges: [],
  equippedTitle: null,
}

const activityRow: ActivityLeaderboardRow = {
  rank: 1,
  userId: 2,
  username: 'm2bsolver1',
  nickname: 'm2bsolver1',
  score: 12345,
  equippedBadges: [],
  equippedTitle: null,
}

const wealthRow: WealthLeaderboardRow = {
  rank: 1,
  userId: 7,
  username: 'alice',
  nickname: 'Alice',
  score: 23000,
  equippedBadges: [],
  equippedTitle: null,
}

const wealthFullRow: WealthLeaderboardRow = {
  rank: 21,
  userId: 9,
  username: 'full-wealth',
  nickname: '富豪完整榜用户',
  score: 99,
  equippedBadges: [],
  equippedTitle: null,
}

const sharingRow: SharingLeaderboardRow = {
  rank: 1,
  userId: 1,
  username: 'm2badmin',
  nickname: 'm2badmin',
  score: 120,
  equippedBadges: [],
  equippedTitle: null,
}

const routes = [
  { path: '/leaderboard', name: 'leaderboard', component: { template: '<div />' } },
  { path: '/users/:id', name: 'user-profile', component: { template: '<div />' } },
]

async function mountPage() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/leaderboard')
  await router.isReady()
  const wrapper = mount(LeaderboardView, { global: { plugins: [router, PrimeVue] } })
  await flushPromises()
  return wrapper
}

let scrollIntoView: ReturnType<typeof vi.fn>

beforeEach(() => {
  vi.mocked(listRatingLeaderboard).mockReset()
  vi.mocked(listActivityLeaderboard).mockReset()
  vi.mocked(listWealthLeaderboard).mockReset()
  vi.mocked(listSharingLeaderboard).mockReset()
  vi.mocked(getRatingSeason).mockReset()
  vi.mocked(listRatingLeaderboard).mockResolvedValue({ total: 1, items: [ratingRow] })
  vi.mocked(listActivityLeaderboard).mockResolvedValue({ total: 1, items: [activityRow] })
  vi.mocked(listWealthLeaderboard).mockResolvedValue({ total: 1, items: [wealthRow] })
  vi.mocked(listSharingLeaderboard).mockResolvedValue({ total: 1, items: [sharingRow] })
  vi.mocked(getRatingSeason).mockResolvedValue({
    key: '2026-11-01',
    name: '2026-11-01 ~ 2027-04-30',
    startsAt: '2026-11-01T00:00:00Z',
    endsAt: '2027-05-01T00:00:00Z',
    nextResetAt: '2027-05-01T00:00:00Z',
  })

  // jsdom does not implement scrollIntoView.
  scrollIntoView = vi.fn()
  Element.prototype.scrollIntoView = scrollIntoView as unknown as Element['scrollIntoView']
})

afterEach(() => {
  document.body.innerHTML = ''
})

describe('LeaderboardView', () => {
  it('loads four top10 cards and the rating full table', async () => {
    const wrapper = await mountPage()

    expect(listRatingLeaderboard).toHaveBeenCalledTimes(2)
    expect(listRatingLeaderboard).toHaveBeenCalledWith({ page: 1, pageSize: 10 })
    expect(listRatingLeaderboard).toHaveBeenCalledWith({ page: 1, pageSize: 20 })

    for (const list of [listActivityLeaderboard, listWealthLeaderboard, listSharingLeaderboard]) {
      expect(list).toHaveBeenCalledTimes(1)
      expect(list).toHaveBeenCalledWith({ page: 1, pageSize: 10 })
    }

    const boards = wrapper.findAll('[data-board]')
    expect(boards.map((board) => board.attributes('data-board'))).toEqual([
      'rating',
      'activity',
      'wealth',
      'contribution',
    ])

    const text = wrapper.text()
    expect(text).toContain('Rating 榜')
    expect(text).toContain('活跃榜')
    expect(text).toContain('富豪榜')
    expect(text).toContain('贡献榜')

    const ratingCard = wrapper.find('[data-board="rating"]')
    expect(ratingCard.findComponent(RatingLevelName).props('name')).toBe('m2bsolver1')
    expect(ratingCard.findComponent(RatingLevelName).props('level').name).toBe('青名')
    expect(ratingCard.text()).toContain('1516')

    // Wealth scores use thousands separators in the card.
    expect(wrapper.find('[data-board="wealth"]').text()).toContain('23,000')
  })

  it('switches to a board full table and scrolls when its card is clicked', async () => {
    vi.mocked(listWealthLeaderboard).mockImplementation(async ({ pageSize }) =>
      pageSize === 10 ? { total: 1, items: [wealthRow] } : { total: 21, items: [wealthFullRow] },
    )
    const wrapper = await mountPage()

    await wrapper.find('[data-board="wealth"]').trigger('click')
    await flushPromises()

    expect(listWealthLeaderboard).toHaveBeenLastCalledWith({ page: 1, pageSize: 20 })

    const activeTab = wrapper
      .findAll('[role="tab"]')
      .find((tab) => tab.attributes('aria-selected') === 'true')
    expect(activeTab?.text()).toContain('富豪榜')

    const full = wrapper.find('[data-test="full-rankings"]')
    expect(full.exists()).toBe(true)
    expect(full.text()).toContain('富豪完整榜用户')
    expect(scrollIntoView).toHaveBeenCalled()
  })

  it('does not refetch an already loaded table', async () => {
    const wrapper = await mountPage()
    const card = wrapper.find('[data-board="contribution"]')

    await card.trigger('click')
    await flushPromises()
    await card.trigger('click')
    await flushPromises()

    const pageSizes = vi.mocked(listSharingLeaderboard).mock.calls.map(([query]) => query.pageSize)
    expect(pageSizes).toEqual([10, 20])
  })

  it('keeps cards independent when one board fails', async () => {
    vi.mocked(listWealthLeaderboard)
      .mockRejectedValueOnce(new ApiError(500, 50000, '财富榜挂了'))
      .mockResolvedValue({ total: 1, items: [wealthRow] })

    const wrapper = await mountPage()

    const wealthCard = wrapper.find('[data-board="wealth"]')
    expect(wealthCard.text()).toContain('财富榜挂了')
    expect(wealthCard.text()).toContain('重试')

    // The other three cards still render their data.
    expect(wrapper.find('[data-board="rating"]').text()).toContain('1516')
    expect(wrapper.find('[data-board="activity"]').text()).toContain('12,345')
    expect(wrapper.find('[data-board="contribution"]').text()).toContain('120')

    await wealthCard.find('button').trigger('click')
    await flushPromises()

    expect(listWealthLeaderboard).toHaveBeenLastCalledWith({ page: 1, pageSize: 10 })
    expect(wrapper.find('[data-board="wealth"]').text()).toContain('23,000')
    // Retrying a card must not also open the complete table.
    expect(listWealthLeaderboard).not.toHaveBeenCalledWith({ page: 1, pageSize: 20 })
  })

  it('paginates the complete table per board', async () => {
    vi.mocked(listRatingLeaderboard).mockImplementation(async ({ pageSize }) =>
      pageSize === 10 ? { total: 1, items: [ratingRow] } : { total: 45, items: [ratingRow] },
    )
    const wrapper = await mountPage()

    wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()
    expect(listRatingLeaderboard).toHaveBeenLastCalledWith({ page: 2, pageSize: 20 })

    await wrapper.findAll('[role="tab"]')[1].trigger('click')
    await flushPromises()
    expect(listActivityLeaderboard).toHaveBeenLastCalledWith({ page: 1, pageSize: 20 })
  })

  it('shows the season and next reset', async () => {
    const wrapper = await mountPage()

    expect(getRatingSeason).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('2026-11-01 ~ 2027-04-30')
    expect(wrapper.text()).toContain('下次重置')
    expect(wrapper.text()).toContain('2027-05-01')
  })

  it('ignores a season fetch failure', async () => {
    vi.mocked(getRatingSeason).mockRejectedValue(new ApiError(500, 50000, '服务器错误'))
    const wrapper = await mountPage()

    expect(wrapper.text()).not.toContain('服务器错误')
    expect(wrapper.text()).not.toContain('下次重置')
    expect(wrapper.text()).toContain('m2bsolver1')
  })

  it('does not render a manual sign-in entry', async () => {
    const wrapper = await mountPage()
    const text = wrapper.text()

    expect(text).not.toContain('签到 +5 金币')
    expect(text).not.toContain('今日已签到')
    expect(text).not.toContain('登录后签到领金币')
    expect(text).not.toContain('登录 / 注册')
  })

  it('opens the four-board scoring rules dialog', async () => {
    const wrapper = await mountPage()
    await wrapper.find('[data-test="rules-button"]').trigger('click')
    await flushPromises()

    expect(document.querySelector('[data-test="rules-dialog"]')).not.toBeNull()
    const text = document.body.textContent ?? ''
    expect(text).toContain('四榜计分规则')
    expect(text).toContain('Rating 榜')
    expect(text).toContain('活跃榜')
    expect(text).toContain('富豪榜')
    expect(text).toContain('贡献榜')
    expect(text).toContain('+2')
    expect(text).toContain('+10')
    expect(text).toContain('+3')
    expect(text).toContain('+1')
    expect(text).toContain('+20')
    expect(text).toContain('提交代码 +2')
    expect(text).toContain('每日上限')
    expect(text).not.toContain('累计上限')
    expect(text).toContain('取消点赞')
    expect(text).toContain('点赞者')
  })
})
