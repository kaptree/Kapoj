import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/http'
import ResetPasswordView from './ResetPasswordView.vue'

const authApi = vi.hoisted(() => ({
  resetPassword: vi.fn(),
}))

vi.mock('@/api/auth', () => authApi)

const routes = [
  { path: '/reset-password', name: 'reset-password', component: { template: '<div>reset</div>' } },
  {
    path: '/forgot-password',
    name: 'forgot-password',
    component: { template: '<div>forgot</div>' },
  },
  { path: '/login', name: 'login', component: { template: '<div>login</div>' } },
]

async function mountReset(path = '/reset-password?token=tok') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(ResetPasswordView, {
    global: { plugins: [router, PrimeVue, createPinia()] },
  })
  return { wrapper, router }
}

function passwordInputs(wrapper: Awaited<ReturnType<typeof mountReset>>['wrapper']) {
  return wrapper.findAll('input[type="password"]')
}

beforeEach(() => {
  authApi.resetPassword.mockReset()
})

describe('ResetPasswordView', () => {
  it('shows an invalid state when the token is missing', async () => {
    const { wrapper } = await mountReset('/reset-password')
    expect(wrapper.text()).toContain('链接无效')
    expect(wrapper.find('a[href="/forgot-password"]').exists()).toBe(true)
  })

  it('rejects mismatched passwords', async () => {
    const { wrapper } = await mountReset()
    const inputs = passwordInputs(wrapper)
    await inputs[0]!.setValue('new-password-1')
    await inputs[1]!.setValue('new-password-2')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('两次输入的密码不一致')
    expect(authApi.resetPassword).not.toHaveBeenCalled()
  })

  it('resets the password and links to login', async () => {
    authApi.resetPassword.mockResolvedValue(null)
    const { wrapper } = await mountReset()
    const inputs = passwordInputs(wrapper)
    await inputs[0]!.setValue('new-password-1')
    await inputs[1]!.setValue('new-password-1')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(authApi.resetPassword).toHaveBeenCalledWith({
      token: 'tok',
      newPassword: 'new-password-1',
    })
    expect(wrapper.text()).toContain('密码已重置')
    expect(wrapper.find('a[href="/login"]').exists()).toBe(true)
  })

  it('shows the expired state when the token was already used', async () => {
    authApi.resetPassword.mockRejectedValue(new ApiError(400, 40003, '链接无效或已过期'))
    const { wrapper } = await mountReset()
    const inputs = passwordInputs(wrapper)
    await inputs[0]!.setValue('new-password-1')
    await inputs[1]!.setValue('new-password-1')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('链接无效或已过期')
    expect(wrapper.find('a[href="/forgot-password"]').exists()).toBe(true)
  })
})
