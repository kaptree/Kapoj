import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { clearCompanionMuteCache } from '@/composables/useCompanionMute'
import DefaultLayout from './DefaultLayout.vue'

const contestsApi = vi.hoisted(() => ({ getContest: vi.fn() }))

vi.mock('@/stores/site', () => ({
  useSiteStore: () => ({ recordVisit: vi.fn().mockResolvedValue(undefined) }),
}))
vi.mock('@/api/contests', () => contestsApi)

const routes = [
  {
    path: '/',
    component: DefaultLayout,
    children: [{ path: '', component: { template: '<div />' } }],
  },
  {
    path: '/contests/:id',
    name: 'contest-detail',
    component: { template: '<div />' },
  },
]

const stubs = {
  AppShell: { template: '<div><slot name="nav" /><slot /></div>' },
  AppNav: true,
  SiteFooter: true,
  CompanionDock: true,
  CompanionPageNote: true,
}

async function mountLayout(path = '/') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(DefaultLayout, {
    global: {
      plugins: [router, PrimeVue, createPinia()],
      stubs,
    },
  })
  return { wrapper, router }
}

beforeEach(() => {
  contestsApi.getContest.mockReset()
  clearCompanionMuteCache()
})

describe('DefaultLayout', () => {
  it('mounts the dock and page note for default pages', async () => {
    const { wrapper } = await mountLayout()

    expect(wrapper.find('companion-dock-stub').exists()).toBe(true)
    expect(wrapper.find('companion-page-note-stub').exists()).toBe(true)
    expect(wrapper.find('.companion-focus-chip').exists()).toBe(false)
    expect(contestsApi.getContest).not.toHaveBeenCalled()
  })

  it('shows a static focus chip while a contest is running', async () => {
    contestsApi.getContest.mockResolvedValue({ status: 'running' })

    const { wrapper } = await mountLayout('/contests/7')
    await flushPromises()

    expect(contestsApi.getContest).toHaveBeenCalledWith(7)
    expect(wrapper.find('.companion-focus-chip').exists()).toBe(true)
    expect(wrapper.text()).toContain('专注模式')
  })
})
