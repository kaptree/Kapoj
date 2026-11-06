import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { getAdminStats } from '@/api/admin'
import type { AdminAuditItem, AdminStats } from '@/api/types'
import { PERM } from '@/permissions'
import AdminDashboardView from './AdminDashboardView.vue'

const auth = vi.hoisted(() => ({
  can: vi.fn<(code: string) => boolean>(() => false),
  canAny: vi.fn(() => false),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => auth,
}))

vi.mock('@/api/admin', () => ({
  getAdminStats: vi.fn(),
}))

const routes = [
  { path: '/admin', component: { template: '<div />' } },
  { path: '/admin/gamification', component: { template: '<div />' } },
  { path: '/admin/users', component: { template: '<div />' } },
  { path: '/admin/roles', component: { template: '<div />' } },
  { path: '/problems', component: { template: '<div />' } },
  { path: '/contests', component: { template: '<div />' } },
]

function audit(overrides: Partial<AdminAuditItem> = {}): AdminAuditItem {
  return {
    id: 1,
    actorId: 2,
    action: 'user.ban',
    targetType: 'user',
    targetId: 3,
    summary: '封禁用户 alice',
    ip: '127.0.0.1',
    userAgent: 'vitest',
    createdAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function stats(overrides: Partial<AdminStats> = {}): AdminStats {
  return {
    users: 100,
    activeUsers: 80,
    bannedUsers: 5,
    problems: 42,
    publicProblems: 30,
    contests: 7,
    submissions: 1200,
    todaySubmissions: 55,
    roles: 6,
    recentAudit: [],
    ...overrides,
  }
}

async function mountPage() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/admin')
  await router.isReady()
  const wrapper = mount(AdminDashboardView, { global: { plugins: [router, PrimeVue] } })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  auth.can.mockReset()
  auth.can.mockImplementation(() => false)
  auth.canAny.mockReset()
  auth.canAny.mockReturnValue(false)
  vi.mocked(getAdminStats).mockReset()
})

describe('AdminDashboardView', () => {
  it('loads stats and recent audit when both permissions are present', async () => {
    auth.can.mockImplementation((code) =>
      [PERM.admin.dashboard.view, PERM.admin.audit.view].some((p) => p === code),
    )
    vi.mocked(getAdminStats).mockResolvedValue(stats({ recentAudit: [audit()] }))
    const wrapper = await mountPage()

    expect(getAdminStats).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('用户')
    expect(wrapper.text()).toContain('100')
    expect(wrapper.text()).toContain('活跃 80')
    expect(wrapper.text()).toContain('封禁 5')
    expect(wrapper.text()).toContain('题目')
    expect(wrapper.text()).toContain('42')
    expect(wrapper.text()).toContain('公开 30')
    expect(wrapper.text()).toContain('比赛')
    expect(wrapper.text()).toContain('7')
    expect(wrapper.text()).toContain('提交')
    expect(wrapper.text()).toContain('1200')
    expect(wrapper.text()).toContain('今日 55')
    expect(wrapper.text()).toContain('角色')
    expect(wrapper.text()).toContain('6')
    expect(wrapper.text()).toContain('最近审计')
    expect(wrapper.text()).toContain('封禁用户 alice')
  })

  it('without dashboard permission shows quick links and does not call stats', async () => {
    auth.can.mockImplementation(() => false)
    auth.canAny.mockReturnValue(true)
    const wrapper = await mountPage()

    expect(getAdminStats).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('管理后台')
    expect(wrapper.text()).toContain('游戏化管理')
    expect(wrapper.text()).toContain('题库管理')
    expect(wrapper.text()).toContain('比赛管理')
    expect(wrapper.find('a[href="/admin/gamification"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/problems"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/contests"]').exists()).toBe(true)
  })

  it('hides recent audit without the audit permission', async () => {
    auth.can.mockImplementation((code) => code === PERM.admin.dashboard.view)
    vi.mocked(getAdminStats).mockResolvedValue(stats({ recentAudit: [audit()] }))
    const wrapper = await mountPage()

    expect(getAdminStats).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('用户')
    expect(wrapper.text()).not.toContain('最近审计')
    expect(wrapper.text()).not.toContain('封禁用户 alice')
  })

  it('shows the users quick link only with user.read', async () => {
    auth.can.mockImplementation((code) => code === PERM.user.read)
    auth.canAny.mockReturnValue(false)
    const wrapper = await mountPage()

    expect(getAdminStats).not.toHaveBeenCalled()
    expect(wrapper.find('a[href="/admin/users"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('用户管理')
    expect(wrapper.text()).not.toContain('游戏化管理')
  })

  it('shows the roles quick link only with role.read', async () => {
    auth.can.mockImplementation((code) => code === PERM.role.read)
    auth.canAny.mockReturnValue(false)
    const wrapper = await mountPage()

    expect(getAdminStats).not.toHaveBeenCalled()
    expect(wrapper.find('a[href="/admin/roles"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('角色权限')
    expect(wrapper.text()).not.toContain('用户管理')
  })

  it('shows the management entry row to dashboard viewers', async () => {
    auth.can.mockImplementation((code) =>
      [PERM.admin.dashboard.view, PERM.user.read, PERM.role.read].some((p) => p === code),
    )
    auth.canAny.mockReturnValue(false)
    vi.mocked(getAdminStats).mockResolvedValue(stats())
    const wrapper = await mountPage()

    expect(getAdminStats).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('管理快捷入口')
    expect(wrapper.find('a[href="/admin/users"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/admin/roles"]').exists()).toBe(true)
  })
})
