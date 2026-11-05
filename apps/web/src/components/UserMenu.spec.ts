import { mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import UserMenu from './UserMenu.vue'

interface TestUser {
  id: number
  nickname: string
  username: string
  avatarUrl: string
  coins: number
  role: string
}

const auth = vi.hoisted(() => ({
  isLoggedIn: false,
  user: null as TestUser | null,
  fetchMe: vi.fn(),
  logout: vi.fn(),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => auth,
}))

const routes = [
  { path: '/', name: 'home', component: { template: '<div />' } },
  { path: '/login', name: 'login', component: { template: '<div />' } },
  { path: '/register', name: 'register', component: { template: '<div />' } },
  { path: '/users/:id', name: 'user-profile', component: { template: '<div />' } },
  { path: '/appearance', name: 'appearance', component: { template: '<div />' } },
  { path: '/settings/profile', name: 'profile-edit', component: { template: '<div />' } },
]

async function mountMenu() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/')
  await router.isReady()
  return mount(UserMenu, { global: { plugins: [router, PrimeVue] } })
}

beforeEach(() => {
  auth.isLoggedIn = false
  auth.user = null
  auth.fetchMe.mockReset()
  auth.logout.mockReset()
})

describe('UserMenu', () => {
  it('shows login and register links to guests', async () => {
    const wrapper = await mountMenu()

    expect(wrapper.text()).toContain('登录')
    expect(wrapper.text()).toContain('注册')
    expect(wrapper.find('a[href="/login"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/register"]').exists()).toBe(true)
  })

  it('shows the nickname and an accessible menu trigger for logged-in users', async () => {
    auth.isLoggedIn = true
    auth.user = {
      id: 7,
      nickname: 'Alice',
      username: 'alice',
      avatarUrl: '',
      coins: 1234,
      role: 'user',
    }
    const wrapper = await mountMenu()
    const trigger = wrapper.find('.user-menu__trigger')

    expect(trigger.exists()).toBe(true)
    expect(trigger.text()).toContain('Alice')
    expect(trigger.attributes('aria-haspopup')).toBe('menu')
    expect(trigger.attributes('aria-expanded')).toBe('false')
  })
})
