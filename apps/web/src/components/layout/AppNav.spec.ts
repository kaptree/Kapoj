import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { GAMIFICATION_MANAGE_PERMISSIONS } from '@/permissions'
import AppNav from './AppNav.vue'

const auth = vi.hoisted(() => ({
  canAny: vi.fn(() => false),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => auth,
}))

const routes = [
  { path: '/', component: { template: '<div />' } },
  { path: '/problems', component: { template: '<div />' } },
  { path: '/problems/:id', component: { template: '<div />' } },
  { path: '/leaderboard', component: { template: '<div />' } },
  { path: '/admin/gamification', component: { template: '<div />' } },
]

async function mountNav(path = '/') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  return mount(AppNav, { global: { plugins: [router] } })
}

beforeEach(() => {
  auth.canAny.mockReset()
  auth.canAny.mockReturnValue(false)
})

describe('AppNav', () => {
  it('renders the primary and growth navigation groups', async () => {
    const wrapper = await mountNav()

    for (const label of ['首页', '题库', '测评记录', '比赛', '排行榜', '成长', '成就', '任务']) {
      expect(wrapper.text()).toContain(label)
    }
    expect(wrapper.find('nav').attributes('aria-label')).toBe('主导航')
  })

  it('shows the administration entry to users with any gamification manage permission', async () => {
    let wrapper = await mountNav()
    expect(wrapper.text()).not.toContain('游戏化管理')

    auth.canAny.mockReturnValue(true)
    wrapper = await mountNav()
    expect(wrapper.text()).toContain('游戏化管理')
    expect(auth.canAny).toHaveBeenCalledWith(GAMIFICATION_MANAGE_PERMISSIONS)
  })

  it('links the leaderboard now that it is available', async () => {
    const wrapper = await mountNav()
    const leaderboard = wrapper.find('a[href="/leaderboard"]')

    expect(leaderboard.exists()).toBe(true)
    expect(leaderboard.text()).toContain('排行榜')
    expect(leaderboard.attributes('aria-disabled')).toBeUndefined()
  })

  it('marks prefix routes active and sets aria-current', async () => {
    const wrapper = await mountNav('/problems/42')
    const link = wrapper.find('a[href="/problems"]')

    expect(link.classes()).toContain('is-active')
    expect(link.attributes('aria-current')).toBe('page')
  })
})
