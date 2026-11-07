import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Paginator from 'primevue/paginator'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/http'
import {
  getActivitySummary,
  getRatingSeason,
  listActivityLeaderboard,
  listRatingLeaderboard,
  listSharingLeaderboard,
  signInActivity,
} from '@/api/leaderboard'
import type {
  ActivityLeaderboardRow,
  RatingLeaderboardRow,
  SharingLeaderboardRow,
} from '@/api/types'
import RatingLevelName from '@/components/RatingLevelName.vue'
import LeaderboardView from './LeaderboardView.vue'

vi.mock('@/api/leaderboard', () => ({
  listRatingLeaderboard: vi.fn(),
  listActivityLeaderboard: vi.fn(),
  listSharingLeaderboard: vi.fn(),
  getActivitySummary: vi.fn(),
  getRatingSeason: vi.fn(),
  signInActivity: vi.fn(),
}))

const auth = vi.hoisted(() => ({
  isLoggedIn: false,
  user: null as unknown,
  fetchMe: vi.fn(),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => auth,
}))

const toast = vi.hoisted(() => ({ add: vi.fn() }))

vi.mock('primevue/usetoast', () => ({
  useToast: () => toast,
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

const blackRedRow: RatingLeaderboardRow = {
  rank: 2,
  userId: 3,
  username: 'zhangsan',
  nickname: '张三',
  score: 2650,
  level: { key: 'black_red', name: '黑红名', color: '#000000', accentColor: '#FF0000' },
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
  { path: '/login', name: 'login', component: { template: '<div />' } },
]

async function mountPage() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/leaderboard')
  await router.isReady()
  const wrapper = mount(LeaderboardView, { global: { plugins: [router, PrimeVue] } })
  await flushPromises()
  return wrapper
}

async function openTab(
  wrapper: Awaited<ReturnType<typeof mountPage>>,
  index: number,
): Promise<void> {
  await wrapper.findAll('[role="tab"]')[index].trigger('click')
  await flushPromises()
}

beforeEach(() => {
  auth.isLoggedIn = false
  auth.fetchMe.mockReset()
  auth.fetchMe.mockResolvedValue(undefined)
  toast.add.mockReset()
  vi.mocked(listRatingLeaderboard).mockReset()
  vi.mocked(listActivityLeaderboard).mockReset()
  vi.mocked(listSharingLeaderboard).mockReset()
  vi.mocked(getActivitySummary).mockReset()
  vi.mocked(getRatingSeason).mockReset()
  vi.mocked(signInActivity).mockReset()
  vi.mocked(listRatingLeaderboard).mockResolvedValue({ total: 1, items: [ratingRow] })
  vi.mocked(listActivityLeaderboard).mockResolvedValue({ total: 1, items: [activityRow] })
  vi.mocked(listSharingLeaderboard).mockResolvedValue({ total: 1, items: [sharingRow] })
  vi.mocked(getActivitySummary).mockResolvedValue({
    signedInToday: true,
    todayPoints: 17,
    totalPoints: 17,
  })
  vi.mocked(getRatingSeason).mockResolvedValue({
    key: '2026-11-01',
    name: '2026-11-01 ~ 2027-04-30',
    startsAt: '2026-11-01T00:00:00Z',
    endsAt: '2027-05-01T00:00:00Z',
    nextResetAt: '2027-05-01T00:00:00Z',
  })
})

describe('LeaderboardView', () => {
  it('loads the Rating tab by default and renders rating names with levels', async () => {
    const wrapper = await mountPage()

    expect(listRatingLeaderboard).toHaveBeenCalledWith({ page: 1, pageSize: 20 })
    expect(listActivityLeaderboard).not.toHaveBeenCalled()
    expect(listSharingLeaderboard).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('m2bsolver1')
    expect(wrapper.text()).toContain('1516')

    const level = wrapper.findComponent(RatingLevelName)
    expect(level.exists()).toBe(true)
    expect(level.props('name')).toBe('m2bsolver1')
    expect(level.props('level').name).toBe('青名')
    expect(level.attributes('title')).toBe('m2bsolver1（青名）')
    expect(level.attributes('aria-label')).toBe('m2bsolver1（青名）')
  })

  it('shows current season and next reset', async () => {
    const wrapper = await mountPage()

    expect(getRatingSeason).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('赛季')
    expect(wrapper.text()).toContain('2026-11-01 ~ 2027-04-30')
    expect(wrapper.text()).toContain('下次重置')
    expect(wrapper.text()).toContain('2027-05-01')
  })

  it('ignores season fetch failure', async () => {
    vi.mocked(getRatingSeason).mockRejectedValue(new ApiError(500, 50000, '服务器错误'))
    const wrapper = await mountPage()

    // The leaderboard still renders and no error is surfaced for the season.
    expect(wrapper.text()).not.toContain('服务器错误')
    expect(wrapper.text()).not.toContain('赛季')
    expect(listRatingLeaderboard).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('m2bsolver1')
  })

  it('shows the level name as text and keeps the black-red username legible', async () => {
    vi.mocked(listRatingLeaderboard).mockResolvedValue({
      total: 2,
      items: [ratingRow, blackRedRow],
    })
    const wrapper = await mountPage()

    // RatingLevelName lives in the user column, so the nickname is not repeated.
    const names = wrapper.findAllComponents(RatingLevelName)
    expect(names).toHaveLength(2)
    expect(names[1].text()).toBe('张三')
    expect(names[1].attributes('aria-label')).toBe('张三（黑红名）')

    const ratingLink = wrapper.findAll('.user-link--rating')[1]
    expect(ratingLink.text()).toBe('张三')
    expect(ratingLink.classes()).toContain('user-link--rating')
    expect(ratingLink.find('.rating-level-name__first').exists()).toBe(true)

    // The level column stays readable without relying on color alone.
    const badges = wrapper.findAll('.level-badge')
    expect(badges).toHaveLength(2)
    expect(badges[0].text()).toBe('青名')
    expect(badges[1].text()).toBe('黑红名')
    expect(badges[1].find('.level-badge__name').attributes('style')).toBeUndefined()
    expect((badges[1].element as HTMLElement).style.borderColor).toBe('rgb(255, 0, 0)')

    const dot = badges[1].find('.level-badge__dot')
    expect((dot.element as HTMLElement).style.backgroundColor).toBe('rgb(0, 0, 0)')
    expect((dot.element as HTMLElement).style.borderColor).toBe('rgb(255, 0, 0)')
  })

  it('exposes the leaderboard table wrapper as a labeled scroll region', async () => {
    const wrapper = await mountPage()

    const scroll = wrapper.find('.table-scroll')
    expect(scroll.attributes('tabindex')).toBe('0')
    expect(scroll.attributes('role')).toBe('region')
    expect(scroll.attributes('aria-label')).toBe('排行榜表格，可横向滚动')
    expect((scroll.element as HTMLElement).tabIndex).toBe(0)
  })

  it('lazy-loads activity and sharing tabs with formatted scores', async () => {
    auth.isLoggedIn = true
    const wrapper = await mountPage()
    await openTab(wrapper, 1)

    expect(listActivityLeaderboard).toHaveBeenCalledWith({ page: 1, pageSize: 20 })
    expect(wrapper.text()).toContain('12,345')
    expect(wrapper.text()).toContain('今日已签到')
    expect(wrapper.findComponent(RatingLevelName).exists()).toBe(false)

    await openTab(wrapper, 2)

    expect(listSharingLeaderboard).toHaveBeenCalledWith({ page: 1, pageSize: 20 })
    expect(wrapper.text()).toContain('120')
    expect(wrapper.findComponent(RatingLevelName).exists()).toBe(false)
  })

  it('shows the guest login call to action on the activity tab', async () => {
    const wrapper = await mountPage()
    await openTab(wrapper, 1)

    expect(getActivitySummary).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('登录后签到领金币')
    const login = wrapper.findAll('a').find((link) => link.text().includes('登录 / 注册'))
    expect(login).toBeTruthy()
    expect(login!.attributes('href')).toContain('/login?redirect=/leaderboard')
    // The CTA is a single focusable anchor, not a button nested in a router link.
    expect(login!.find('button').exists()).toBe(false)
    expect(wrapper.findAll('a').filter((link) => link.text().includes('登录 / 注册'))).toHaveLength(
      1,
    )
  })

  it('signs in, refreshes the summary and reloads activity page one', async () => {
    auth.isLoggedIn = true
    vi.mocked(getActivitySummary)
      .mockResolvedValueOnce({ signedInToday: false, todayPoints: 12, totalPoints: 12 })
      .mockResolvedValueOnce({ signedInToday: true, todayPoints: 17, totalPoints: 17 })
    vi.mocked(signInActivity).mockResolvedValue({
      awarded: true,
      activityPoints: 5,
      coins: 5,
      date: '2026-11-05',
    })

    const wrapper = await mountPage()
    await openTab(wrapper, 1)

    const signInButton = wrapper
      .findAll('button')
      .find((button) => button.text().trim() === '签到 +5 金币')
    expect(signInButton).toBeTruthy()
    await signInButton!.trigger('click')
    await flushPromises()

    expect(signInActivity).toHaveBeenCalledTimes(1)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'success',
        detail: '+5 活跃度 / +5 金币',
      }),
    )
    expect(auth.fetchMe).toHaveBeenCalledTimes(1)
    expect(getActivitySummary).toHaveBeenCalledTimes(2)
    expect(listActivityLeaderboard).toHaveBeenLastCalledWith({ page: 1, pageSize: 20 })
    expect(wrapper.text()).toContain('今日已签到')
  })

  it('handles a duplicate sign-in response without awarding twice', async () => {
    auth.isLoggedIn = true
    vi.mocked(getActivitySummary).mockResolvedValue({
      signedInToday: false,
      todayPoints: 12,
      totalPoints: 12,
    })
    vi.mocked(signInActivity).mockResolvedValue({
      awarded: false,
      activityPoints: 0,
      coins: 0,
      date: '2026-11-05',
    })

    const wrapper = await mountPage()
    await openTab(wrapper, 1)
    const signInButton = wrapper
      .findAll('button')
      .find((button) => button.text().trim() === '签到 +5 金币')
    await signInButton!.trigger('click')
    await flushPromises()

    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'info', summary: '今日已签到' }),
    )
    expect(auth.fetchMe).toHaveBeenCalledTimes(1)
  })

  it('shows the API error message when signing in fails', async () => {
    auth.isLoggedIn = true
    vi.mocked(getActivitySummary).mockResolvedValue({
      signedInToday: false,
      todayPoints: 0,
      totalPoints: 0,
    })
    vi.mocked(signInActivity).mockRejectedValue(new ApiError(429, 42900, '今日签到过于频繁'))

    const wrapper = await mountPage()
    await openTab(wrapper, 1)
    const signInButton = wrapper
      .findAll('button')
      .find((button) => button.text().trim() === '签到 +5 金币')
    await signInButton!.trigger('click')
    await flushPromises()

    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'error', detail: '今日签到过于频繁' }),
    )
    expect(auth.fetchMe).not.toHaveBeenCalled()
  })

  it('keeps independent pages and reloads only the active tab', async () => {
    vi.mocked(listRatingLeaderboard).mockResolvedValue({
      total: 45,
      items: [ratingRow],
    })
    vi.mocked(listActivityLeaderboard).mockResolvedValue({
      total: 45,
      items: [activityRow],
    })

    const wrapper = await mountPage()
    wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()
    expect(listRatingLeaderboard).toHaveBeenLastCalledWith({ page: 2, pageSize: 20 })
    expect(listActivityLeaderboard).not.toHaveBeenCalled()

    await openTab(wrapper, 1)
    expect(listActivityLeaderboard).toHaveBeenLastCalledWith({ page: 1, pageSize: 20 })

    await openTab(wrapper, 0)
    expect(listRatingLeaderboard).toHaveBeenCalledTimes(2)
  })
})
