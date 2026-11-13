import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/http'
import ForgotPasswordView from './ForgotPasswordView.vue'

const authApi = vi.hoisted(() => ({
  forgotPassword: vi.fn(),
}))

vi.mock('@/api/auth', () => authApi)

const routes = [
  {
    path: '/forgot-password',
    name: 'forgot-password',
    component: { template: '<div>forgot</div>' },
  },
  { path: '/login', name: 'login', component: { template: '<div>login</div>' } },
]

async function mountForgot() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/forgot-password')
  await router.isReady()
  const wrapper = mount(ForgotPasswordView, {
    global: { plugins: [router, PrimeVue, createPinia()] },
  })
  return { wrapper, router }
}

beforeEach(() => {
  authApi.forgotPassword.mockReset()
})

describe('ForgotPasswordView', () => {
  it('shows a generic success panel after submitting an email', async () => {
    authApi.forgotPassword.mockResolvedValue({ message: 'ok' })
    const { wrapper } = await mountForgot()

    await wrapper.find('input[type="email"]').setValue('alice@example.com')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(authApi.forgotPassword).toHaveBeenCalledWith('alice@example.com')
    expect(wrapper.text()).toContain('如果该邮箱已注册')
    expect(wrapper.find('form').exists()).toBe(false)
  })

  it('shows a generic success panel even for unknown emails', async () => {
    authApi.forgotPassword.mockResolvedValue({ message: 'ok' })
    const { wrapper } = await mountForgot()

    await wrapper.find('input[type="email"]').setValue('nobody@example.com')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('如果该邮箱已注册')
  })

  it('shows the API error and keeps the form', async () => {
    authApi.forgotPassword.mockRejectedValue(new ApiError(429, 42905, '邮件请求过于频繁'))
    const { wrapper } = await mountForgot()

    await wrapper.find('input[type="email"]').setValue('alice@example.com')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('邮件请求过于频繁')
    expect(wrapper.find('form').exists()).toBe(true)
  })
})
