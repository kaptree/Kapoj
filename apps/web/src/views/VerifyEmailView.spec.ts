import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/http'
import type { Session } from '@/api/types'
import VerifyEmailView from './VerifyEmailView.vue'

const auth = vi.hoisted(() => ({
  setSession: vi.fn(),
}))

const authApi = vi.hoisted(() => ({
  verifyEmail: vi.fn(),
  resendVerification: vi.fn(),
}))

const toast = vi.hoisted(() => ({
  add: vi.fn(),
}))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/auth', () => authApi)
vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))

const session: Session = {
  accessToken: 'a',
  refreshToken: 'r',
  sessionId: 's1',
  user: {
    id: 1,
    username: 'alice',
    nickname: 'Alice',
    email: 'alice@example.com',
    emailVerified: true,
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
    createdAt: '2026-01-01T00:00:00Z',
  },
}

const routes = [
  { path: '/', name: 'home', component: { template: '<div>home</div>' } },
  { path: '/login', name: 'login', component: { template: '<div>login</div>' } },
  { path: '/verify-email', name: 'verify-email', component: { template: '<div>verify</div>' } },
]

async function mountVerify(path = '/verify-email') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(VerifyEmailView, {
    global: { plugins: [router, PrimeVue, createPinia()] },
  })
  return { wrapper, router }
}

beforeEach(() => {
  auth.setSession.mockReset()
  authApi.verifyEmail.mockReset()
  authApi.resendVerification.mockReset()
  toast.add.mockReset()
})

describe('VerifyEmailView', () => {
  it('verifies the token, logs in and navigates home', async () => {
    authApi.verifyEmail.mockResolvedValue(session)
    const { router } = await mountVerify('/verify-email?token=tok')
    await flushPromises()

    expect(authApi.verifyEmail).toHaveBeenCalledWith('tok')
    expect(auth.setSession).toHaveBeenCalledWith(session)
    expect(router.currentRoute.value.name).toBe('home')
  })

  it('shows the expired state and a resend form', async () => {
    authApi.verifyEmail.mockRejectedValue(new ApiError(400, 40003, '链接无效或已过期'))
    const { wrapper } = await mountVerify('/verify-email?token=bad')
    await flushPromises()

    expect(wrapper.text()).toContain('链接无效或已过期')
    expect(wrapper.find('input[type="email"]').exists()).toBe(true)
  })

  it('prefills the email and resends a verification link', async () => {
    authApi.resendVerification.mockResolvedValue({ message: 'ok' })
    const { wrapper } = await mountVerify('/verify-email?email=alice@example.com')

    const email = wrapper.find('input[type="email"]')
    expect((email.element as HTMLInputElement).value).toBe('alice@example.com')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(authApi.resendVerification).toHaveBeenCalledWith('alice@example.com')
    expect(wrapper.text()).toContain('如果该邮箱已注册')
  })
})
