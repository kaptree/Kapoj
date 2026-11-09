import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { SiteFooterView, SitePublicView } from '@/api/types'
import { DEFAULT_FOOTER, DEFAULT_HOME, useSiteStore } from '@/stores/site'
import SiteFooter from './SiteFooter.vue'

const siteApi = vi.hoisted(() => ({
  getSite: vi.fn(),
  recordSiteVisit: vi.fn(),
}))

vi.mock('@/api/site', () => siteApi)

function siteView(overrides: {
  footer?: Partial<SiteFooterView>
  runningDays?: number
  visitCount?: number
}): SitePublicView {
  return {
    logoUrl: '',
    faviconUrl: '',
    home: DEFAULT_HOME,
    footer: { ...DEFAULT_FOOTER, ...overrides.footer },
    startedAt: '2026-01-01T00:00:00Z',
    runningDays: overrides.runningDays ?? 0,
    visitCount: overrides.visitCount ?? 0,
  }
}

async function mountFooter(overrides: Parameters<typeof siteView>[0] = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  siteApi.getSite.mockResolvedValue(siteView(overrides))
  const store = useSiteStore()
  await store.ensureLoaded()
  return mount(SiteFooter, { global: { plugins: [pinia] } })
}

describe('SiteFooter', () => {
  beforeEach(() => {
    siteApi.getSite.mockReset()
    siteApi.recordSiteVisit.mockReset()
  })

  it('renders the copyright, filing links, custom text and live counters', async () => {
    const wrapper = await mountFooter({
      footer: {
        icpNumber: '京ICP备12345678号',
        policeNumber: '京公网安备11010102000001号',
        policeUrl: 'https://beian.mps.gov.cn/#/query',
        copyright: '© 2026 kapOJ',
        text: '联系邮箱：hi@example.com\n工作日 9:00-18:00',
        showVisits: true,
      },
      runningDays: 123,
      visitCount: 1234,
    })

    expect(wrapper.text()).toContain('© 2026 kapOJ')
    expect(wrapper.text()).toContain('京ICP备12345678号')
    expect(wrapper.text()).toContain('京公网安备11010102000001号')
    expect(wrapper.find('a.site-footer__icp').attributes('href')).toBe('https://beian.miit.gov.cn/')
    expect(wrapper.find('a.site-footer__police').attributes('href')).toBe(
      'https://beian.mps.gov.cn/#/query',
    )
    expect(wrapper.find('.site-footer__text').text()).toContain('联系邮箱：hi@example.com')
    expect(wrapper.find('.site-footer__text').text()).toContain('工作日 9:00-18:00')
    expect(wrapper.text()).toContain('本站已运行 123 天')
    expect(wrapper.text()).toContain('总访问量 1,234')
  })

  it('hides the visit count when showVisits is false', async () => {
    const wrapper = await mountFooter({
      footer: { showVisits: false },
      runningDays: 12,
      visitCount: 9876,
    })

    expect(wrapper.text()).toContain('本站已运行 12 天')
    expect(wrapper.text()).not.toContain('总访问量')
    expect(wrapper.text()).not.toContain('9,876')
  })

  it('renders a police number without a URL as plain text', async () => {
    const wrapper = await mountFooter({
      footer: { policeNumber: '京公网安备11010102000001号', policeUrl: '' },
    })

    expect(wrapper.find('a.site-footer__police').exists()).toBe(false)
    expect(wrapper.text()).toContain('京公网安备11010102000001号')
  })

  it('does not render separators for empty fields', async () => {
    const wrapper = await mountFooter({
      footer: {
        icpNumber: '',
        policeNumber: '',
        policeUrl: '',
        copyright: '© kapOJ',
        text: '',
        showVisits: false,
      },
    })

    expect(wrapper.text()).toContain('© kapOJ')
    expect(wrapper.text()).not.toContain('·')
    expect(wrapper.find('.site-footer__filing').exists()).toBe(false)
    expect(wrapper.find('.site-footer__text').exists()).toBe(false)
  })

  it('renders exactly one separator between ICP and police filings', async () => {
    const wrapper = await mountFooter({
      footer: {
        icpNumber: '京ICP备12345678号',
        policeNumber: '',
        showVisits: false,
      },
    })

    const filing = wrapper.find('.site-footer__filing')
    expect(filing.text()).toBe('京ICP备12345678号')
    expect(filing.text()).not.toContain('·')
  })
})
