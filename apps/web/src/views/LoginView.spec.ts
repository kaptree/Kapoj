import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/http'
import LoginView from './LoginView.vue'

const auth = vi.hoisted(() => ({
  login: vi.fn(),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => auth,
}))

const routes = [
  { path: '/', name: 'home', component: { template: '<div>home</div>' } },
  { path: '/login', name: 'login', component: { template: '<div>login</div>' } },
  { path: '/register', name: 'register', component: { template: '<div>register</div>' } },
  { path: '/forgot-password', name: 'forgot-password', component: { template: '<div>forgot</div>' } },
  { path: '/verify-email', name: 'verify-email', component: { template: '<div>verify</div>' } },
  { path: '/problems', name: 'problems', component: { template: '<div>problems</div>' } },
]

async function mountLogin(path = '/login') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(LoginView, {
    global: { plugins: [router, PrimeVue, createPinia()] },
  })
  return { wrapper, router }
}

function fillLogin(wrapper: Awaited<ReturnType<typeof mountLogin>>['wrapper']) {
  const inputs = wrapper.findAll('input')
  return Promise.all([inputs[0]!.setValue('alice'), inputs[1]!.setValue('secret123')])
}

beforeEach(() => {
  auth.login.mockReset()
})

describe('LoginView', () => {
  it('blocks submission and shows messages when required fields are empty', async () => {
    const { wrapper } = await mountLogin()

    await wrapper.find('form').trigger('submit')

    expect(wrapper.text()).toContain('请输入用户名或邮箱')
    expect(wrapper.text()).toContain('请输入密码')
    expect(auth.login).not.toHaveBeenCalled()
  })

  it('logs in and follows a safe redirect', async () => {
    auth.login.mockResolvedValue(undefined)
    const { wrapper, router } = await mountLogin('/login?redirect=/problems')

    await fillLogin(wrapper)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(auth.login).toHaveBeenCalledWith({ account: 'alice', password: 'secret123' })
    expect(router.currentRoute.value.fullPath).toBe('/problems')
  })

  it('keeps the user on the page and shows the API error', async () => {
    auth.login.mockRejectedValue(new ApiError(401, 40101, '账号或密码错误'))
    const { wrapper, router } = await mountLogin()

    await fillLogin(wrapper)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('账号或密码错误')
    expect(router.currentRoute.value.fullPath).toBe('/login')
  })

  it('shows an inline unverified message and verification link on 40303', async () => {
    auth.login.mockRejectedValue(new ApiError(403, 40303, '邮箱尚未验证，请先完成邮箱验证'))
    const { wrapper, router } = await mountLogin()

    await wrapper.find('input[autocomplete="username"]').setValue('alice@example.com')
    await wrapper.find('input[autocomplete="current-password"]').setValue('secret123')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('邮箱尚未验证')
    const link = wrapper.find('a[href*="/verify-email"]')
    expect(link.exists()).toBe(true)
    expect(link.attributes('href')).toContain('alice@example.com')
    expect(router.currentRoute.value.fullPath).toBe('/login')
  })

  it('links to the forgot password page', async () => {
    const { wrapper } = await mountLogin()
    const link = wrapper.find('a[href="/forgot-password"]')
    expect(link.exists()).toBe(true)
  })
})
