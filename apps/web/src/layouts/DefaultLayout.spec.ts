import { mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'
import DefaultLayout from './DefaultLayout.vue'

const mute = vi.hoisted(() => ({ muted: { value: false } }))

vi.mock('@/stores/site', () => ({
  useSiteStore: () => ({ recordVisit: vi.fn().mockResolvedValue(undefined) }),
}))
vi.mock('@/composables/useCompanionMute', () => ({
  useCompanionMute: () => ({ muted: mute.muted, checking: { value: false } }),
}))

const routes = [
  {
    path: '/',
    component: DefaultLayout,
    children: [{ path: '', component: { template: '<div />' } }],
  },
]

describe('DefaultLayout', () => {
  it('mounts the dock and page note for default pages', async () => {
    const router = createRouter({ history: createMemoryHistory(), routes })
    await router.push('/')
    await router.isReady()
    const wrapper = mount(DefaultLayout, {
      global: {
        plugins: [router, PrimeVue, createPinia()],
        stubs: {
          AppShell: { template: '<div><slot name="nav" /><slot /></div>' },
          AppNav: true,
          SiteFooter: true,
          CompanionDock: true,
          CompanionPageNote: true,
        },
      },
    })
    expect(wrapper.find('companion-dock-stub').exists()).toBe(true)
    expect(wrapper.find('companion-page-note-stub').exists()).toBe(true)
  })

  it('shows a static focus chip while a contest is running', async () => {
    mute.muted.value = true
    const router = createRouter({ history: createMemoryHistory(), routes })
    await router.push('/')
    await router.isReady()
    const wrapper = mount(DefaultLayout, {
      global: {
        plugins: [router, PrimeVue, createPinia()],
        stubs: {
          AppShell: { template: '<div><slot name="nav" /><slot /></div>' },
          AppNav: true,
          SiteFooter: true,
          CompanionDock: true,
          CompanionPageNote: true,
        },
      },
    })
    expect(wrapper.text()).toContain('专注模式')
    mute.muted.value = false
  })
})
