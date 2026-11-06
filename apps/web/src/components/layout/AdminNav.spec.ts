import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  CONTEST_MANAGE_PERMISSIONS,
  GAMIFICATION_MANAGE_PERMISSIONS,
  PERM,
  PROBLEM_MANAGE_PERMISSIONS,
} from '@/permissions'
import AdminNav from './AdminNav.vue'

const auth = vi.hoisted(() => ({
  can: vi.fn<(code: string) => boolean>(() => false),
  canAny: vi.fn<(codes: readonly string[]) => boolean>(() => false),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => auth,
}))

const routes = [
  { path: '/', component: { template: '<div />' } },
  { path: '/admin', component: { template: '<div />' } },
  { path: '/admin/gamification', component: { template: '<div />' } },
  { path: '/admin/users', component: { template: '<div />' } },
  { path: '/admin/roles', component: { template: '<div />' } },
  { path: '/problems', component: { template: '<div />' } },
  { path: '/contests', component: { template: '<div />' } },
]

async function mountNav(path = '/admin') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  return mount(AdminNav, { global: { plugins: [router] } })
}

beforeEach(() => {
  auth.can.mockReset()
  auth.can.mockReturnValue(false)
  auth.canAny.mockReset()
  auth.canAny.mockReturnValue(false)
})

describe('AdminNav', () => {
  it('always shows the overview and return-to-site links', async () => {
    const wrapper = await mountNav()

    expect(wrapper.find('a[href="/admin"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/admin"]').text()).toContain('概览')
    expect(wrapper.find('a[href="/"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/"]').text()).toContain('返回站点')
  })

  it('shows the gamification link only with a gamification manage permission', async () => {
    let wrapper = await mountNav()
    expect(wrapper.text()).not.toContain('游戏化管理')

    auth.canAny.mockImplementation((codes) => codes === GAMIFICATION_MANAGE_PERMISSIONS)
    wrapper = await mountNav()
    expect(auth.canAny).toHaveBeenCalledWith(GAMIFICATION_MANAGE_PERMISSIONS)
    expect(wrapper.find('a[href="/admin/gamification"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('游戏化管理')
  })

  it('shows the users entry only with user.read', async () => {
    let wrapper = await mountNav()
    expect(wrapper.text()).not.toContain('用户管理')

    auth.can.mockImplementation((code) => code === PERM.user.read)
    wrapper = await mountNav()
    expect(auth.can).toHaveBeenCalledWith(PERM.user.read)
    expect(wrapper.find('a[href="/admin/users"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('用户管理')
  })

  it('shows the roles entry only with role.read', async () => {
    let wrapper = await mountNav()
    expect(wrapper.text()).not.toContain('角色权限')

    auth.can.mockImplementation((code) => code === PERM.role.read)
    wrapper = await mountNav()
    expect(auth.can).toHaveBeenCalledWith(PERM.role.read)
    expect(wrapper.find('a[href="/admin/roles"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('角色权限')
  })

  it('shows the content links by problem/contest permission', async () => {
    auth.canAny.mockImplementation((codes) => codes === PROBLEM_MANAGE_PERMISSIONS)
    const wrapper = await mountNav()

    expect(auth.canAny).toHaveBeenCalledWith(PROBLEM_MANAGE_PERMISSIONS)
    expect(auth.canAny).toHaveBeenCalledWith(CONTEST_MANAGE_PERMISSIONS)
    expect(wrapper.find('a[href="/problems"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/contests"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('题库管理')
    expect(wrapper.text()).not.toContain('比赛管理')
  })

  it('hides content links without problem/contest manage permissions', async () => {
    const wrapper = await mountNav()

    expect(wrapper.text()).not.toContain('题库管理')
    expect(wrapper.text()).not.toContain('比赛管理')
    expect(wrapper.text()).not.toContain('内容')
  })
})
