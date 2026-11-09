import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import type { SitePublicView } from '@/api/types'
import { DEFAULT_FOOTER, DEFAULT_HOME, useSiteStore } from '@/stores/site'
import AppBrand from './AppBrand.vue'

const siteApi = vi.hoisted(() => ({
  getSite: vi.fn(),
  recordSiteVisit: vi.fn(),
}))

vi.mock('@/api/site', () => siteApi)

function siteView(logoUrl: string): SitePublicView {
  return {
    logoUrl,
    faviconUrl: '',
    home: DEFAULT_HOME,
    footer: { ...DEFAULT_FOOTER },
    startedAt: '2026-01-01T00:00:00Z',
    runningDays: 1,
    visitCount: 1,
  }
}

async function mountBrand(logoUrl = '') {
  const pinia = createPinia()
  setActivePinia(pinia)
  siteApi.getSite.mockResolvedValue(siteView(logoUrl))
  await useSiteStore().ensureLoaded()
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div />' } }],
  })
  await router.push('/')
  await router.isReady()
  return mount(AppBrand, {
    props: { badge: '管理后台' },
    global: { plugins: [router, pinia] },
  })
}

describe('AppBrand', () => {
  beforeEach(() => {
    siteApi.getSite.mockReset()
    siteApi.recordSiteVisit.mockReset()
  })

  it('shows the text brand when no logo is configured', async () => {
    const wrapper = await mountBrand()

    expect(wrapper.find('.app-brand__mark').text()).toBe('K')
    expect(wrapper.find('.app-brand__name').text()).toBe('kapOJ')
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.text()).toContain('管理后台')
  })

  it('replaces the whole mark and name with the uploaded logo', async () => {
    const wrapper = await mountBrand('/api/site/logo?v=3')

    const logo = wrapper.find('img.app-brand__logo')
    expect(logo.exists()).toBe(true)
    expect(logo.attributes('src')).toBe('/api/site/logo?v=3')
    expect(logo.attributes('alt')).toBe('kapOJ 首页')
    expect(wrapper.find('.app-brand__mark').exists()).toBe(false)
    expect(wrapper.find('.app-brand__name').exists()).toBe(false)
    expect(wrapper.text()).toContain('管理后台')
  })

  it('falls back to the text brand when the logo image fails to load', async () => {
    const wrapper = await mountBrand('/api/site/logo?v=3')

    await wrapper.find('img.app-brand__logo').trigger('error')

    expect(wrapper.find('img.app-brand__logo').exists()).toBe(false)
    expect(wrapper.find('.app-brand__mark').exists()).toBe(true)
    expect(wrapper.find('.app-brand__name').text()).toBe('kapOJ')
  })
})
