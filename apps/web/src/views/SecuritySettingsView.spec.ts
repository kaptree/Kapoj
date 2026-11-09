import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Tag from 'primevue/tag'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { changePassword, listSessions, logoutAll, logoutOthers, revokeSession } from '@/api/auth'
import { ApiError } from '@/api/http'
import type { Me, SessionView } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import SecuritySettingsView from './SecuritySettingsView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/auth', () => ({
  changePassword: vi.fn(),
  listSessions: vi.fn(),
  revokeSession: vi.fn(),
  logoutOthers: vi.fn(),
  logoutAll: vi.fn(),
}))

function me(): Me {
  return {
    id: 1,
    username: 'alice',
    nickname: 'Alice',
    email: 'alice@example.com',
    avatarUrl: '',
    avatarUploaded: false,
    role: 'user',
    roles: [],
    permissions: [],
    isSuperAdmin: false,
    rating: 1500,
    coins: 0,
    bio: '',
    status: 'active',
    createdAt: '2026-11-04T00:00:00Z',
  }
}

function sessionView(overrides: Partial<SessionView> = {}): SessionView {
  return {
    id: 's1',
    ip: '203.0.113.1',
    userAgent:
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36',
    createdAt: '2026-11-01T08:00:00Z',
    lastUsedAt: '2026-11-09T10:00:00Z',
    ...overrides,
  }
}

const routes = [
  { path: '/settings/security', name: 'security-settings', component: { template: '<div />' } },
  { path: '/login', name: 'login', component: { template: '<div />' } },
]

async function mountPage(sessionId: string | null = 's1') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/settings/security')
  await router.isReady()
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  if (sessionId) {
    auth.setSession({ accessToken: 'a', refreshToken: 'r', sessionId, user: me() })
  }
  const wrapper = mount(SecuritySettingsView, { global: { plugins: [router, PrimeVue, pinia] } })
  await flushPromises()
  return { wrapper, router, auth }
}

function button(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper'], label: string) {
  const found = wrapper.findAll('button').find((b) => b.text().includes(label))
  expect(found, `button "${label}"`).toBeTruthy()
  return found!
}

function confirmOptions(): {
  header?: string
  message?: string
  accept?: () => void
  reject?: () => void
} {
  const call = confirm.require.mock.calls.at(-1)
  expect(call).toBeTruthy()
  return call![0] as never
}

async function setPassword(
  wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper'],
  index: number,
  value: string,
) {
  const inputs = wrapper.findAll('input[type="password"]')
  await inputs[index]!.setValue(value)
}

beforeEach(() => {
  document.body.innerHTML = ''
  localStorage.clear()
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(changePassword).mockReset()
  vi.mocked(listSessions).mockReset()
  vi.mocked(revokeSession).mockReset()
  vi.mocked(logoutOthers).mockReset()
  vi.mocked(logoutAll).mockReset()
  vi.mocked(listSessions).mockResolvedValue([sessionView()])
})

describe('SecuritySettingsView', () => {
  it('loads sessions and renders device/IP with the current-device tag', async () => {
    vi.mocked(listSessions).mockResolvedValue([
      sessionView({ id: 's1' }),
      sessionView({
        id: 's2',
        ip: '198.51.100.2',
        userAgent:
          'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:119.0) Gecko/20100101 Firefox/119.0',
      }),
    ])
    const { wrapper } = await mountPage()

    expect(listSessions).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('macOS · Chrome')
    expect(wrapper.text()).toContain('Windows · Firefox')
    expect(wrapper.text()).toContain('203.0.113.1')
    expect(wrapper.text()).toContain('198.51.100.2')

    const tags = wrapper.findAllComponents(Tag)
    expect(tags.map((t) => t.props('value'))).toEqual(['当前设备'])
    expect(tags.map((t) => t.props('severity'))).toEqual(['success'])
  })

  it('shows the empty state when there are no sessions', async () => {
    vi.mocked(listSessions).mockResolvedValue([])
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('暂无活跃会话')
  })

  it('shows an error state and retries', async () => {
    vi.mocked(listSessions).mockRejectedValue(new ApiError(500, 50000, '加载失败'))
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('加载失败')

    vi.mocked(listSessions).mockResolvedValue([sessionView()])
    await button(wrapper, '重试').trigger('click')
    await flushPromises()

    expect(listSessions).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('macOS · Chrome')
  })

  it('omits the current tag and disables logout-others when sessionId is missing', async () => {
    const { wrapper } = await mountPage(null)

    expect(wrapper.findAllComponents(Tag)).toHaveLength(0)
    const othersButton = button(wrapper, '退出其他设备')
    expect(othersButton.attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('当前会话缺少 sessionId')
  })

  it('validates the password form before submitting', async () => {
    vi.mocked(changePassword).mockResolvedValue(null)
    const { wrapper } = await mountPage()

    await setPassword(wrapper, 0, 'old-password')
    await setPassword(wrapper, 1, 'short')
    await setPassword(wrapper, 2, 'short')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('密码长度需为 8–72 个字节')
    expect(changePassword).not.toHaveBeenCalled()
  })

  it('rejects a mismatched confirmation', async () => {
    vi.mocked(changePassword).mockResolvedValue(null)
    const { wrapper } = await mountPage()

    await setPassword(wrapper, 0, 'old-password')
    await setPassword(wrapper, 1, 'new-password-1')
    await setPassword(wrapper, 2, 'different')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('两次输入的密码不一致')
    expect(changePassword).not.toHaveBeenCalled()
  })

  it('rejects a new password identical to the current one', async () => {
    vi.mocked(changePassword).mockResolvedValue(null)
    const { wrapper } = await mountPage()

    await setPassword(wrapper, 0, 'secret123')
    await setPassword(wrapper, 1, 'secret123')
    await setPassword(wrapper, 2, 'secret123')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('新密码不能与当前密码相同')
    expect(changePassword).not.toHaveBeenCalled()
  })

  it('changes the password, clears the session and redirects to login', async () => {
    vi.mocked(changePassword).mockResolvedValue(null)
    const { wrapper, router, auth } = await mountPage()

    await setPassword(wrapper, 0, 'old-password')
    await setPassword(wrapper, 1, 'new-password-1')
    await setPassword(wrapper, 2, 'new-password-1')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(changePassword).toHaveBeenCalledWith({
      currentPassword: 'old-password',
      newPassword: 'new-password-1',
    })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '密码已修改，请重新登录' }),
    )
    expect(auth.sessionId).toBeNull()
    expect(auth.isLoggedIn).toBe(false)
    expect(router.currentRoute.value.name).toBe('login')
  })

  it('surfaces a wrong-current-password error (40001)', async () => {
    vi.mocked(changePassword).mockRejectedValue(new ApiError(400, 40001, '当前密码错误'))
    const { wrapper, router } = await mountPage()

    await setPassword(wrapper, 0, 'wrong')
    await setPassword(wrapper, 1, 'new-password-1')
    await setPassword(wrapper, 2, 'new-password-1')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('当前密码错误')
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'error', summary: '修改失败' }),
    )
    expect(router.currentRoute.value.name).toBe('security-settings')
  })

  it('surfaces a rate-limit error (42904)', async () => {
    vi.mocked(changePassword).mockRejectedValue(
      new ApiError(429, 42904, '修改密码尝试次数过多，请 5 分钟后再试'),
    )
    const { wrapper } = await mountPage()

    await setPassword(wrapper, 0, 'old-password')
    await setPassword(wrapper, 1, 'new-password-1')
    await setPassword(wrapper, 2, 'new-password-1')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('修改密码尝试次数过多')
  })

  it('revokes the current session, clears auth and redirects to login', async () => {
    vi.mocked(listSessions).mockResolvedValue([sessionView({ id: 's1' })])
    vi.mocked(revokeSession).mockResolvedValue(null)
    const { wrapper, router, auth } = await mountPage('s1')

    await button(wrapper, '退出此设备').trigger('click')
    expect(confirm.require).toHaveBeenCalledTimes(1)
    confirmOptions().accept?.()
    await flushPromises()

    expect(revokeSession).toHaveBeenCalledWith('s1')
    expect(auth.sessionId).toBeNull()
    expect(router.currentRoute.value.name).toBe('login')
  })

  it('revokes another session and reloads the list', async () => {
    vi.mocked(listSessions).mockResolvedValue([sessionView({ id: 's2' })])
    vi.mocked(revokeSession).mockResolvedValue(null)
    const { wrapper, router } = await mountPage('s1')

    await button(wrapper, '退出此设备').trigger('click')
    confirmOptions().accept?.()
    await flushPromises()

    expect(revokeSession).toHaveBeenCalledWith('s2')
    expect(listSessions).toHaveBeenCalledTimes(2)
    expect(router.currentRoute.value.name).toBe('security-settings')
  })

  it('does not revoke when the confirmation is rejected', async () => {
    vi.mocked(revokeSession).mockResolvedValue(null)
    const { wrapper } = await mountPage('s1')

    await button(wrapper, '退出此设备').trigger('click')
    confirmOptions().reject?.()
    await flushPromises()

    expect(revokeSession).not.toHaveBeenCalled()
  })

  it('logs out other devices with the current session id and reloads', async () => {
    vi.mocked(logoutOthers).mockResolvedValue(null)
    const { wrapper } = await mountPage('s1')

    await button(wrapper, '退出其他设备').trigger('click')
    expect(confirm.require).toHaveBeenCalledTimes(1)
    confirmOptions().accept?.()
    await flushPromises()

    expect(logoutOthers).toHaveBeenCalledWith('s1')
    expect(listSessions).toHaveBeenCalledTimes(2)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '已退出其他设备' }),
    )
  })

  it('logs out all devices, clears auth and redirects to login', async () => {
    vi.mocked(logoutAll).mockResolvedValue(null)
    const { wrapper, router, auth } = await mountPage('s1')

    await button(wrapper, '退出所有设备').trigger('click')
    confirmOptions().accept?.()
    await flushPromises()

    expect(logoutAll).toHaveBeenCalledTimes(1)
    expect(auth.sessionId).toBeNull()
    expect(router.currentRoute.value.name).toBe('login')
  })
})
