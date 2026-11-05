import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Avatar from 'primevue/avatar'
import ProgressBar from 'primevue/progressbar'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import { listAchievements } from '@/api/gamification'
import { ApiError } from '@/api/http'
import type { AchievementItem } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import AchievementsView from './AchievementsView.vue'

vi.mock('@/api/gamification', () => ({
  listAchievements: vi.fn(),
}))

function achievement(overrides: Partial<AchievementItem> = {}): AchievementItem {
  return {
    id: 1,
    code: 'ac_100',
    name: '百题斩',
    description: '累计通过 100 道题',
    iconUrl: '',
    conditionType: 'ac_count',
    conditionConfig: { threshold: 100 },
    rewardCoins: 50,
    rewardBadgeId: 3,
    sortOrder: 1,
    isActive: true,
    unlocked: false,
    unlockedAt: null,
    progress: [{ type: 'ac_count', current: 60, target: 100, done: false }],
    ...overrides,
  }
}

function unlocked(overrides: Partial<AchievementItem> = {}): AchievementItem {
  return achievement({
    id: 2,
    code: 'first_contest',
    name: '首战告捷',
    description: '首次参加比赛',
    conditionType: 'contest_count',
    conditionConfig: { threshold: 1 },
    rewardCoins: 0,
    rewardBadgeId: null,
    sortOrder: 2,
    unlocked: true,
    unlockedAt: '2026-01-01T00:00:00Z',
    progress: [],
    ...overrides,
  })
}

const routes = [
  { path: '/achievements', name: 'achievements', component: { template: '<div />' } },
  { path: '/login', name: 'login', component: { template: '<div />' } },
]

async function mountPage(path = '/achievements', loggedIn = true) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  if (loggedIn) {
    auth.setTokens({ accessToken: 'access', refreshToken: 'refresh' })
  }
  const wrapper = mount(AchievementsView, {
    global: { plugins: [router, PrimeVue, pinia] },
  })
  await flushPromises()
  return { wrapper, router, auth }
}

function filterButton(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper'], label: string) {
  return wrapper.findAll('button').find((item) => item.text().trim() === label)
}

beforeEach(() => {
  localStorage.clear()
  vi.mocked(listAchievements).mockReset()
  vi.mocked(listAchievements).mockResolvedValue([achievement(), unlocked()])
})

describe('AchievementsView', () => {
  it('renders counts, rewards, progress and unlocked time', async () => {
    const { wrapper } = await mountPage()

    expect(listAchievements).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('成就墙')
    expect(wrapper.text()).toContain('已解锁 1 / 2')
    expect(wrapper.findAll('.achievement-card')).toHaveLength(2)
    expect(wrapper.text()).toContain('百题斩')
    expect(wrapper.text()).toContain('累计通过 100 道题')
    expect(wrapper.text()).toContain('金币 +50')
    expect(wrapper.text()).toContain('徽章 #3')
    expect(wrapper.text()).toContain('AC 题目数')
    expect(wrapper.text()).toContain('60 / 100')
    expect(wrapper.text()).toContain('首战告捷')
    expect(wrapper.text()).toContain('解锁时间')
    expect(wrapper.text()).toContain('2026/1/1')

    const progress = wrapper.findComponent(ProgressBar)
    expect(progress.props('value')).toBe(60)
    expect(wrapper.findComponent(Avatar).props('label')).toBe('百')
    wrapper.unmount()
  })

  it('filters by unlocked and locked', async () => {
    const { wrapper } = await mountPage()

    await filterButton(wrapper, '已解锁（1）')!.trigger('click')
    await nextTick()
    expect(wrapper.findAll('.achievement-card')).toHaveLength(1)
    expect(wrapper.text()).toContain('首战告捷')
    expect(wrapper.text()).not.toContain('百题斩')

    await filterButton(wrapper, '未解锁（1）')!.trigger('click')
    await nextTick()
    expect(wrapper.findAll('.achievement-card')).toHaveLength(1)
    expect(wrapper.text()).toContain('百题斩')
    expect(wrapper.text()).not.toContain('首战告捷')

    await filterButton(wrapper, '全部（2）')!.trigger('click')
    await nextTick()
    expect(wrapper.findAll('.achievement-card')).toHaveLength(2)
    wrapper.unmount()
  })

  it('shows the guest login call to action', async () => {
    vi.mocked(listAchievements).mockResolvedValue([
      achievement(),
      unlocked({ unlocked: false, unlockedAt: null }),
    ])
    const { wrapper } = await mountPage('/achievements', false)

    expect(wrapper.text()).toContain('登录后可查看自己的成就解锁进度')
    const link = wrapper.findAll('a').find((item) => item.text().includes('登录 / 注册'))
    expect(link).toBeTruthy()
    expect(link!.attributes('href')).toBe('/login?redirect=/achievements')
    expect(wrapper.text()).toContain('已解锁 0 / 2')
    wrapper.unmount()
  })

  it('reloads when the login state changes', async () => {
    vi.mocked(listAchievements)
      .mockResolvedValueOnce([achievement(), unlocked()])
      .mockResolvedValueOnce([
        achievement({ unlocked: false }),
        unlocked({ unlocked: false, unlockedAt: null }),
      ])
    const { wrapper, auth } = await mountPage()
    expect(wrapper.text()).toContain('已解锁 1 / 2')

    auth.clear()
    await flushPromises()
    expect(listAchievements).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('已解锁 0 / 2')
    wrapper.unmount()
  })

  it('shows an empty state for a filter with no matches', async () => {
    vi.mocked(listAchievements).mockResolvedValue([unlocked()])
    const { wrapper } = await mountPage()

    await filterButton(wrapper, '未解锁（0）')!.trigger('click')
    await nextTick()
    expect(wrapper.text()).toContain('暂无符合条件的成就')
    wrapper.unmount()
  })

  it('shows an error and retries', async () => {
    vi.mocked(listAchievements)
      .mockRejectedValueOnce(new ApiError(500, 50000, '加载成就失败'))
      .mockResolvedValueOnce([achievement()])
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('加载成就失败')
    const retry = wrapper.findAll('button').find((item) => item.text().trim() === '重试')
    expect(retry).toBeTruthy()
    await retry!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('百题斩')
    wrapper.unmount()
  })

  it('falls back to the achievement initial when the icon fails', async () => {
    vi.mocked(listAchievements).mockResolvedValue([achievement({ iconUrl: '/achievement.png' })])
    const { wrapper } = await mountPage()

    const avatar = wrapper.findComponent(Avatar)
    expect(avatar.props('image')).toBe('/achievement.png')
    expect(avatar.props('label')).toBeFalsy()
    avatar.vm.$emit('error', new Event('error'))
    await nextTick()
    expect(avatar.props('image')).toBeFalsy()
    expect(avatar.props('label')).toBe('百')
    wrapper.unmount()
  })
})
