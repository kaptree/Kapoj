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
  { path: '/admin/audit', component: { template: '<div />' } },
  { path: '/admin/rating', component: { template: '<div />' } },
  { path: '/admin/ai', component: { template: '<div />' } },
  { path: '/admin/agents', component: { template: '<div />' } },
  { path: '/problems', component: { template: '<div />' } },
  { path: '/contests', component: { template: '<div />' } },
  { path: '/admin/learning-paths', component: { template: '<div />' } },
  { path: '/admin/certificates', component: { template: '<div />' } },
  { path: '/admin/site', component: { template: '<div />' } },
  { path: '/admin/jobs', component: { template: '<div />' } },
  { path: '/admin/judge', component: { template: '<div />' } },
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

  it('shows the audit entry only with admin.audit.view', async () => {
    let wrapper = await mountNav()
    expect(wrapper.text()).not.toContain('审计日志')

    auth.can.mockImplementation((code) => code === PERM.admin.audit.view)
    wrapper = await mountNav()
    expect(auth.can).toHaveBeenCalledWith(PERM.admin.audit.view)
    expect(wrapper.find('a[href="/admin/audit"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('审计日志')
  })

  it('shows the rating settings entry only with rating.settings.manage', async () => {
    let wrapper = await mountNav()
    expect(wrapper.text()).not.toContain('Rating 设置')

    auth.can.mockImplementation((code) => code === PERM.rating.settings.manage)
    wrapper = await mountNav()
    expect(auth.can).toHaveBeenCalledWith(PERM.rating.settings.manage)
    expect(wrapper.find('a[href="/admin/rating"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Rating 设置')
  })

  it('shows the AI settings entry only with ai.settings.manage', async () => {
    let wrapper = await mountNav()
    expect(wrapper.text()).not.toContain('AI 设置')

    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    wrapper = await mountNav()
    expect(auth.can).toHaveBeenCalledWith(PERM.ai.settings.manage)
    expect(wrapper.find('a[href="/admin/ai"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('AI 设置')
  })

  it('shows the agent management entry only with agent.manage', async () => {
    let wrapper = await mountNav()
    expect(wrapper.text()).not.toContain('Agent 管理')

    auth.can.mockImplementation((code) => code === PERM.agent.manage)
    wrapper = await mountNav()
    expect(auth.can).toHaveBeenCalledWith(PERM.agent.manage)
    expect(wrapper.find('a[href="/admin/agents"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Agent 管理')
    wrapper.unmount()
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

  it('shows the learning paths entry with official.manage or admin.read', async () => {
    let wrapper = await mountNav()
    expect(wrapper.text()).not.toContain('学习路线')

    // official.manage alone
    auth.canAny.mockImplementation((codes) => codes.includes(PERM.learningPath.official.manage))
    wrapper = await mountNav()
    expect(auth.canAny).toHaveBeenCalledWith([
      PERM.learningPath.official.manage,
      PERM.learningPath.admin.read,
    ])
    expect(wrapper.find('a[href="/admin/learning-paths"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('学习路线')
    wrapper.unmount()

    // admin.read alone
    auth.canAny.mockImplementation((codes) => codes.includes(PERM.learningPath.admin.read))
    wrapper = await mountNav()
    expect(wrapper.find('a[href="/admin/learning-paths"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('学习路线')
    wrapper.unmount()
  })

  it('shows the certificate review entry only with certificate.manage', async () => {
    let wrapper = await mountNav()
    expect(wrapper.text()).not.toContain('证书审核')

    auth.can.mockImplementation((code) => code === PERM.certificate.manage)
    wrapper = await mountNav()
    expect(auth.can).toHaveBeenCalledWith(PERM.certificate.manage)
    expect(wrapper.find('a[href="/admin/certificates"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('证书审核')
    wrapper.unmount()
  })

  it('shows the site settings entry only with site.settings.manage', async () => {
    let wrapper = await mountNav()
    expect(wrapper.text()).not.toContain('站点设置')

    auth.can.mockImplementation((code) => code === PERM.site.settings.manage)
    wrapper = await mountNav()
    expect(auth.can).toHaveBeenCalledWith(PERM.site.settings.manage)
    expect(wrapper.find('a[href="/admin/site"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('站点设置')
    wrapper.unmount()
  })

  it('shows the mail settings entry only with mail.settings.manage', async () => {
    let wrapper = await mountNav()
    expect(wrapper.text()).not.toContain('邮件设置')

    auth.can.mockImplementation((code) => code === PERM.mail.settings.manage)
    wrapper = await mountNav()
    expect(auth.can).toHaveBeenCalledWith(PERM.mail.settings.manage)
    expect(wrapper.find('a[href="/admin/mail"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('邮件设置')
    wrapper.unmount()
  })

  it('shows the backup entry only with system.backup.manage', async () => {
    let wrapper = await mountNav()
    expect(wrapper.text()).not.toContain('数据备份')

    auth.can.mockImplementation((code) => code === PERM.system.backup.manage)
    wrapper = await mountNav()
    expect(auth.can).toHaveBeenCalledWith(PERM.system.backup.manage)
    expect(wrapper.find('a[href="/admin/backups"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('数据备份')
    wrapper.unmount()
  })

  it('shows the scheduled jobs entry only with system.jobs.manage', async () => {
    let wrapper = await mountNav()
    expect(wrapper.text()).not.toContain('定时任务')

    auth.can.mockImplementation((code) => code === PERM.system.jobs.manage)
    wrapper = await mountNav()
    expect(auth.can).toHaveBeenCalledWith(PERM.system.jobs.manage)
    expect(wrapper.find('a[href="/admin/jobs"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('定时任务')
    expect(wrapper.find('a[href="/admin/jobs"] i').classes()).toContain('pi-clock')
    wrapper.unmount()
  })

  it('shows the judge settings entry only with system.settings.manage', async () => {
    let wrapper = await mountNav()
    expect(wrapper.text()).not.toContain('评测设置')

    auth.can.mockImplementation((code) => code === PERM.system.settings.manage)
    wrapper = await mountNav()
    expect(auth.can).toHaveBeenCalledWith(PERM.system.settings.manage)
    expect(wrapper.find('a[href="/admin/judge"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('评测设置')
    expect(wrapper.find('a[href="/admin/judge"] i').classes()).toContain('pi-sliders-h')
    wrapper.unmount()
  })
})
