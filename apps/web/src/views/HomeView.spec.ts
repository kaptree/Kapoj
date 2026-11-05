import { mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { Me } from '@/api/types'
import HomeView from './HomeView.vue'

const baseUser: Me = {
  id: 7,
  username: 'alice',
  nickname: 'Alice',
  email: 'alice@example.com',
  avatarUrl: '',
  bio: '',
  role: 'user',
  rating: 1680,
  coins: 1234,
  status: 'active',
  createdAt: '2026-01-01T00:00:00Z',
}

const auth = vi.hoisted(() => ({
  isLoggedIn: false,
  user: null as Me | null,
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => auth,
}))

const routes = [
  { path: '/', name: 'home', component: { template: '<div />' } },
  { path: '/problems', name: 'problems', component: { template: '<div />' } },
  { path: '/contests', name: 'contests', component: { template: '<div />' } },
  { path: '/submissions', name: 'submissions', component: { template: '<div />' } },
  { path: '/achievements', name: 'achievements', component: { template: '<div />' } },
  { path: '/tasks', name: 'tasks', component: { template: '<div />' } },
  { path: '/shop/badges', name: 'badge-shop', component: { template: '<div />' } },
  { path: '/shop/titles', name: 'title-shop', component: { template: '<div />' } },
  { path: '/appearance', name: 'appearance', component: { template: '<div />' } },
  { path: '/login', name: 'login', component: { template: '<div />' } },
  { path: '/register', name: 'register', component: { template: '<div />' } },
  { path: '/settings/profile', name: 'profile-edit', component: { template: '<div />' } },
]

async function mountHome() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/')
  await router.isReady()
  const wrapper = mount(HomeView, { global: { plugins: [router, PrimeVue] } })
  return { wrapper, router }
}

function hrefs(wrapper: Awaited<ReturnType<typeof mountHome>>['wrapper']) {
  return wrapper.findAll('a').map((link) => link.attributes('href'))
}

beforeEach(() => {
  auth.isLoggedIn = false
  auth.user = null
})

describe('HomeView', () => {
  it('shows the hero, system status and quick links to guests', async () => {
    const { wrapper } = await mountHome()
    const links = hrefs(wrapper)

    expect(wrapper.find('h1').text()).toContain('欢迎来到 kapOJ')
    expect(wrapper.text()).toContain('排行榜前端')
    expect(wrapper.text()).toContain('M4-A')
    expect(links).toEqual(
      expect.arrayContaining([
        '/problems',
        '/contests',
        '/submissions',
        '/achievements',
        '/tasks',
        '/shop/badges',
        '/shop/titles',
        '/login',
        '/register',
      ]),
    )
    expect(links).not.toContain('/appearance')
    expect(wrapper.text()).toContain('登录后记录你的成长')
  })

  it('shows the profile strip and appearance entry to logged-in users', async () => {
    auth.isLoggedIn = true
    auth.user = baseUser
    const { wrapper } = await mountHome()
    const links = hrefs(wrapper)

    expect(wrapper.text()).toContain('Alice')
    expect(wrapper.text()).toContain('@alice')
    expect(wrapper.text()).toContain('1680')
    expect(wrapper.text()).toContain('1,234')
    expect(links).toContain('/appearance')
    expect(links).toContain('/settings/profile')
    expect(wrapper.text()).not.toContain('登录后记录你的成长')
  })

  it('renders a loading strip while a logged-in profile is unavailable', async () => {
    auth.isLoggedIn = true
    auth.user = null
    const { wrapper } = await mountHome()

    expect(wrapper.text()).toContain('正在加载你的档案')
    expect(wrapper.text()).not.toContain('登录后记录你的成长')
  })
})
