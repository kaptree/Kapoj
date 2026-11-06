import { mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Menu from 'primevue/menu'
import type { MenuItem } from 'primevue/menuitem'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ADMIN_ACCESS_PERMISSIONS } from '@/permissions'
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
  canAny: vi.fn(() => false),
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
  { path: '/admin', name: 'admin-dashboard', component: { template: '<div />' } },
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
  auth.canAny.mockReset()
  auth.canAny.mockReturnValue(false)
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

  it('shows 管理后台 menu item for admins', async () => {
    auth.isLoggedIn = true
    auth.user = {
      id: 7,
      nickname: 'Alice',
      username: 'alice',
      avatarUrl: '',
      coins: 1234,
      role: 'user',
    }
    auth.canAny.mockReturnValue(true)
    const wrapper = await mountMenu()

    expect(auth.canAny).toHaveBeenCalledWith(ADMIN_ACCESS_PERMISSIONS)
    const model = wrapper.findComponent(Menu).props('model') as MenuItem[]
    expect(model.map((item) => item.label)).toContain('管理后台')
  })

  it('does not show the admin entry for normal users', async () => {
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

    const model = wrapper.findComponent(Menu).props('model') as MenuItem[]
    expect(model.map((item) => item.label)).not.toContain('管理后台')
  })
})
