import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { getActivitySummary, signInActivity } from '@/api/leaderboard'
import { useCompanionStore } from '@/stores/companion'
import CompanionDock from './CompanionDock.vue'

const auth = vi.hoisted(() => ({ isLoggedIn: true, user: { id: 7 } }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/leaderboard', () => ({
  getActivitySummary: vi.fn().mockResolvedValue({ signedInToday: true }),
  signInActivity: vi.fn(),
}))

const routes = [
  { path: '/', name: 'home', component: { template: '<div />' } },
  { path: '/login', name: 'login', component: { template: '<div />' } },
]

async function mountDock(path = '/') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const pinia = createPinia()
  setActivePinia(pinia)
  return mount(CompanionDock, {
    global: { plugins: [router, PrimeVue, pinia] },
    attachTo: document.body,
  })
}

beforeEach(() => {
  localStorage.clear()
  auth.isLoggedIn = true
  vi.mocked(getActivitySummary).mockResolvedValue({ signedInToday: true } as never)
})

afterEach(() => {
  document.body.innerHTML = ''
})

describe('CompanionDock', () => {
  it('hides for guests', async () => {
    auth.isLoggedIn = false
    const wrapper = await mountDock()
    expect(wrapper.find('.companion-dock').exists()).toBe(false)
  })

  it('hides on auth routes even when a session exists', async () => {
    const wrapper = await mountDock('/login')
    expect(wrapper.find('.companion-dock').exists()).toBe(false)
  })

  it('opens and closes with Escape', async () => {
    const wrapper = await mountDock()
    await flushPromises()
    await wrapper.get('.companion-dock__toggle').trigger('click')
    expect(wrapper.get('.companion-dock__toggle').attributes('aria-expanded')).toBe('true')
    await wrapper.get('.companion-dock__panel').trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('.companion-dock__panel').exists()).toBe(false)
  })

  it('offers sign-in when today is not signed', async () => {
    vi.mocked(getActivitySummary).mockResolvedValue({ signedInToday: false } as never)
    const wrapper = await mountDock()
    await flushPromises()
    expect(wrapper.text()).toContain('签到')
    await wrapper.get('.companion-dock__signin').trigger('click')
    expect(signInActivity).toHaveBeenCalled()
  })

  it('can switch to quiet mode from the panel', async () => {
    const wrapper = await mountDock()
    await flushPromises()
    await wrapper.get('.companion-dock__toggle').trigger('click')
    await wrapper.get('.companion-dock__quiet').trigger('click')
    expect(useCompanionStore().activityLevel).toBe('quiet')
  })

  it('disappears when the companion is disabled', async () => {
    const wrapper = await mountDock()
    const store = useCompanionStore()
    store.load()
    store.setEnabled(false)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.companion-dock').exists()).toBe(false)
  })
})
