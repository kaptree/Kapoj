import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AdminNav from '@/components/layout/AdminNav.vue'
import AdminLayout from './AdminLayout.vue'

const auth = vi.hoisted(() => ({
  isLoggedIn: true,
  user: { id: 1, username: 'admin', nickname: '管理员', avatarUrl: '', coins: 0 },
  can: vi.fn(() => false),
  canAny: vi.fn(() => false),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => auth,
}))

const routes = [
  { path: '/', component: { template: '<div />' } },
  {
    path: '/admin',
    component: AdminLayout,
    meta: { title: '概览' },
    children: [
      { path: '', component: { template: '<div class="admin-dashboard-child">仪表盘内容</div>' } },
    ],
  },
  { path: '/admin/gamification', component: { template: '<div />' } },
]

async function mountLayout() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/admin')
  await router.isReady()
  const wrapper = mount(RouterView, {
    global: {
      plugins: [router, PrimeVue],
      stubs: { UserMenu: true, AppThemeToggle: true },
    },
  })
  return { wrapper, router }
}

beforeEach(() => {
  auth.can.mockReset()
  auth.can.mockReturnValue(false)
  auth.canAny.mockReset()
  auth.canAny.mockReturnValue(false)
})

describe('AdminLayout', () => {
  it('renders the admin nav and the route title', async () => {
    const { wrapper } = await mountLayout()

    expect(wrapper.findComponent(AdminNav).exists()).toBe(true)
    expect(wrapper.text()).toContain('概览')
    expect(wrapper.text()).toContain('仪表盘内容')
  })

  it('opens the mobile drawer with the admin nav and closes on navigate', async () => {
    const { wrapper } = await mountLayout()
    const toggle = wrapper.find('.admin-menu-toggle')

    expect(toggle.exists()).toBe(true)
    expect(toggle.attributes('aria-expanded')).toBe('false')

    await toggle.trigger('click')
    await flushPromises()

    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(document.body.querySelector('#admin-mobile-nav')).toBeTruthy()

    // The drawer's admin nav emits navigate, which closes the drawer.
    const drawerNav = document.body.querySelector('#admin-mobile-nav')!
    const link = drawerNav.querySelector('a[href="/admin"]')
    expect(link).toBeTruthy()
    ;(link as HTMLElement).click()
    await flushPromises()

    expect(toggle.attributes('aria-expanded')).toBe('false')
  })
})
