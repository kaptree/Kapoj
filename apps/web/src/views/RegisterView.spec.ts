import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/http'
import RegisterView from './RegisterView.vue'

const auth = vi.hoisted(() => ({
  register: vi.fn(),
}))

const toast = vi.hoisted(() => ({
  add: vi.fn(),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => auth,
}))

vi.mock('primevue/usetoast', () => ({
  useToast: () => toast,
}))

const routes = [
  { path: '/', name: 'home', component: { template: '<div>home</div>' } },
  { path: '/login', name: 'login', component: { template: '<div>login</div>' } },
  { path: '/register', name: 'register', component: { template: '<div>register</div>' } },
]

async function mountRegister() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/register')
  await router.isReady()
  const wrapper = mount(RegisterView, { global: { plugins: [router, PrimeVue] } })
  return { wrapper, router }
}

beforeEach(() => {
  auth.register.mockReset()
  toast.add.mockReset()
})

describe('RegisterView', () => {
  it('blocks submission and shows required messages', async () => {
    const { wrapper } = await mountRegister()

    await wrapper.find('form').trigger('submit')

    expect(wrapper.text()).toContain('请输入用户名')
    expect(wrapper.text()).toContain('请输入邮箱')
    expect(wrapper.text()).toContain('请输入密码')
    expect(auth.register).not.toHaveBeenCalled()
  })

  it('applies the registration format rules', async () => {
    const { wrapper } = await mountRegister()

    await wrapper.find('input[autocomplete="username"]').setValue('ab')
    await wrapper.find('input[autocomplete="email"]').setValue('not-an-email')
    await wrapper.find('input[autocomplete="new-password"]').setValue('short')
    await wrapper.find('form').trigger('submit')

    expect(wrapper.text()).toContain('用户名需为 3–32 位字母、数字或下划线')
    expect(wrapper.text()).toContain('邮箱格式不正确')
    expect(wrapper.text()).toContain('密码长度需为 8–72 个字节')
    expect(auth.register).not.toHaveBeenCalled()
  })

  it('registers, shows a success toast and navigates home', async () => {
    auth.register.mockResolvedValue(undefined)
    const { wrapper, router } = await mountRegister()

    await wrapper.find('input[autocomplete="username"]').setValue('alice_01')
    await wrapper.find('input[autocomplete="email"]').setValue('alice@example.com')
    await wrapper.find('input[autocomplete="new-password"]').setValue('secret123')
    await wrapper.findAll('input')[3]!.setValue('Alice')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(auth.register).toHaveBeenCalledWith({
      username: 'alice_01',
      email: 'alice@example.com',
      password: 'secret123',
      nickname: 'Alice',
    })
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
    expect(router.currentRoute.value.name).toBe('home')
  })

  it('keeps the user on the page and shows the API error', async () => {
    auth.register.mockRejectedValue(new ApiError(409, 40901, '用户名已被占用'))
    const { wrapper, router } = await mountRegister()

    await wrapper.find('input[autocomplete="username"]').setValue('alice_01')
    await wrapper.find('input[autocomplete="email"]').setValue('alice@example.com')
    await wrapper.find('input[autocomplete="new-password"]').setValue('secret123')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('用户名已被占用')
    expect(router.currentRoute.value.name).toBe('register')
  })
})
