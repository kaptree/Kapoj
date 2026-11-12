import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ADMIN_ACCESS_PERMISSIONS, GUEST_PERM } from '@/permissions'
import AppNav from './AppNav.vue'

const auth = vi.hoisted(() => ({
  isLoggedIn: false,
  canAny: vi.fn(() => false),
}))

const guest = vi.hoisted(() => ({
  permissions: [] as string[],
  loaded: false,
  ensureLoaded: vi.fn(),
  can: vi.fn<(code: string) => boolean>(() => false),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => auth,
}))

vi.mock('@/stores/guest', () => ({
  useGuestStore: () => guest,
}))

const routes = [
  { path: '/', component: { template: '<div />' } },
  { path: '/problems', component: { template: '<div />' } },
  { path: '/problems/:id', component: { template: '<div />' } },
  { path: '/learning-paths', component: { template: '<div />' } },
  { path: '/leaderboard', component: { template: '<div />' } },
  { path: '/achievements', component: { template: '<div />' } },
  { path: '/tasks', component: { template: '<div />' } },
  { path: '/shop/badges', component: { template: '<div />' } },
  { path: '/shop/titles', component: { template: '<div />' } },
  { path: '/appearance', component: { template: '<div />' } },
  { path: '/admin', component: { template: '<div />' } },
]

async function mountNav(path = '/') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  return mount(AppNav, { global: { plugins: [router] } })
}

beforeEach(() => {
  auth.isLoggedIn = false
  auth.canAny.mockReset()
  auth.canAny.mockReturnValue(false)
  guest.permissions = []
  guest.loaded = false
  guest.can.mockReset()
  guest.can.mockReturnValue(false)
})

describe('AppNav', () => {
  it('renders only the core group for guests by default', async () => {
    const wrapper = await mountNav()

    for (const label of ['首页', '题库', '测评记录', '比赛', '学习路线', '排行榜']) {
      expect(wrapper.text()).toContain(label)
    }
    expect(wrapper.text()).not.toContain('成长')
    expect(wrapper.find('a[href="/achievements"]').exists()).toBe(false)
    expect(wrapper.find('nav').attributes('aria-label')).toBe('主导航')
  })

  it('shows the growth group without appearance for guests with guest.growth.view', async () => {
    guest.can.mockReturnValue(true)
    const wrapper = await mountNav()

    expect(wrapper.text()).toContain('成长')
    expect(wrapper.find('a[href="/achievements"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/tasks"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/shop/badges"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/shop/titles"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/appearance"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('管理后台')
    expect(guest.can).toHaveBeenCalledWith(GUEST_PERM.growth)
  })

  it('keeps the full growth group for logged-in users', async () => {
    auth.isLoggedIn = true
    const wrapper = await mountNav()

    expect(wrapper.text()).toContain('成长')
    expect(wrapper.find('a[href="/achievements"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/appearance"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('管理后台')
  })

  it('links the learning path entry for both guests and logged-in users', async () => {
    let wrapper = await mountNav()
    const guestLink = wrapper.find('a[href="/learning-paths"]')
    expect(guestLink.exists()).toBe(true)
    expect(guestLink.text()).toContain('学习路线')

    auth.isLoggedIn = true
    wrapper = await mountNav()
    const loggedInLink = wrapper.find('a[href="/learning-paths"]')
    expect(loggedInLink.exists()).toBe(true)
    expect(loggedInLink.text()).toContain('学习路线')
  })

  it('shows 管理后台 for any admin permission, not for normal users', async () => {
    auth.isLoggedIn = true
    let wrapper = await mountNav()
    expect(wrapper.text()).not.toContain('管理后台')

    auth.canAny.mockReturnValue(true)
    wrapper = await mountNav()
    expect(wrapper.text()).toContain('管理后台')
    expect(auth.canAny).toHaveBeenCalledWith(ADMIN_ACCESS_PERMISSIONS)
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
