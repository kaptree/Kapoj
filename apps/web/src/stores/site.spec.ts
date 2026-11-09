import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'

import type { SitePublicView } from '@/api/types'
import { DEFAULT_FOOTER, DEFAULT_HOME, SITE_VISITED_KEY, useSiteStore } from './site'

const siteApi = vi.hoisted(() => ({
  getSite: vi.fn(),
  recordSiteVisit: vi.fn(),
}))

vi.mock('@/api/site', () => siteApi)

function siteView(overrides: Partial<SitePublicView> = {}): SitePublicView {
  return {
    logoUrl: '',
    faviconUrl: '',
    home: {
      ...DEFAULT_HOME,
      primaryCta: { ...DEFAULT_HOME.primaryCta },
      secondaryCta: { ...DEFAULT_HOME.secondaryCta },
      quickLinks: DEFAULT_HOME.quickLinks.map((link) => ({ ...link })),
      growthLinks: DEFAULT_HOME.growthLinks.map((link) => ({ ...link })),
    },
    footer: { ...DEFAULT_FOOTER },
    startedAt: '2026-01-01T00:00:00Z',
    runningDays: 312,
    visitCount: 42,
    ...overrides,
  }
}

describe('site store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    sessionStorage.clear()
    document.head.innerHTML = ''
    siteApi.getSite.mockReset()
    siteApi.recordSiteVisit.mockReset()
  })

  it('loads public config once and applies favicon', async () => {
    siteApi.getSite.mockResolvedValue(siteView({ faviconUrl: '/api/site/favicon?v=1' }))
    const store = useSiteStore()
    await store.ensureLoaded()
    await store.ensureLoaded()

    expect(siteApi.getSite).toHaveBeenCalledTimes(1)
    expect(document.querySelector('link[rel="icon"]')?.getAttribute('href')).toBe(
      '/api/site/favicon?v=1',
    )
    expect(store.config?.runningDays).toBe(312)
    expect(store.visitCount).toBe(42)
  })

  it('falls back to defaults when the request fails', async () => {
    siteApi.getSite.mockRejectedValue(new Error('network'))
    const store = useSiteStore()
    await store.ensureLoaded()

    expect(store.config).toBeNull()
    expect(store.home.title).toBe('欢迎来到')
    expect(store.home.titleHighlight).toBe('kapOJ')
    expect(store.footer.copyright).toContain('kapOJ')
    expect(store.logoUrl).toBe('')
    expect(store.faviconUrl).toBe('')
    expect(store.runningDays).toBe(0)
    expect(store.visitCount).toBe(0)
    expect(document.querySelector('link[rel="icon"]')?.getAttribute('href')).toBe('/favicon.svg')
  })

  it('retries after a failed load', async () => {
    siteApi.getSite.mockRejectedValueOnce(new Error('network'))
    const store = useSiteStore()
    await store.ensureLoaded()
    expect(store.config).toBeNull()

    siteApi.getSite.mockResolvedValueOnce(siteView())
    await store.ensureLoaded()
    expect(siteApi.getSite).toHaveBeenCalledTimes(2)
    expect(store.config?.runningDays).toBe(312)
  })

  it('normalizes missing quick and growth links from abnormal old data', async () => {
    siteApi.getSite.mockResolvedValue(
      siteView({
        home: {
          ...DEFAULT_HOME,
          quickLinks: null as unknown as SitePublicView['home']['quickLinks'],
          growthLinks: null as unknown as SitePublicView['home']['growthLinks'],
        },
      }),
    )
    const store = useSiteStore()
    await store.ensureLoaded()

    expect(store.home.quickLinks).toEqual([])
    expect(store.home.growthLinks).toEqual([])
  })

  it('records a visit once per session', async () => {
    sessionStorage.clear()
    siteApi.getSite.mockResolvedValue(siteView())
    siteApi.recordSiteVisit.mockResolvedValue({ visitCount: 7 })
    const store = useSiteStore()
    await store.ensureLoaded()
    await store.recordVisit()
    await store.recordVisit()

    expect(siteApi.recordSiteVisit).toHaveBeenCalledTimes(1)
    expect(store.visitCount).toBe(7)
    expect(sessionStorage.getItem(SITE_VISITED_KEY)).toBe('1')
  })

  it('retries after a failed visit report and marks the session only on success', async () => {
    siteApi.recordSiteVisit.mockRejectedValueOnce(new Error('offline'))
    const store = useSiteStore()
    await store.recordVisit()

    expect(siteApi.recordSiteVisit).toHaveBeenCalledTimes(1)
    expect(sessionStorage.getItem(SITE_VISITED_KEY)).toBeNull()
    expect(store.visitCount).toBe(0)

    siteApi.recordSiteVisit.mockResolvedValueOnce({ visitCount: 5 })
    await store.recordVisit()
    await store.recordVisit()

    expect(siteApi.recordSiteVisit).toHaveBeenCalledTimes(2)
    expect(sessionStorage.getItem(SITE_VISITED_KEY)).toBe('1')
    expect(store.visitCount).toBe(5)
  })

  it('deduplicates concurrent visit reports', async () => {
    siteApi.recordSiteVisit.mockResolvedValue({ visitCount: 3 })
    const store = useSiteStore()
    await Promise.all([store.recordVisit(), store.recordVisit()])

    expect(siteApi.recordSiteVisit).toHaveBeenCalledTimes(1)
    expect(sessionStorage.getItem(SITE_VISITED_KEY)).toBe('1')
    expect(store.visitCount).toBe(3)
  })

  it('does not throw when session storage is blocked', async () => {
    const getItem = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    const setItem = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    siteApi.recordSiteVisit.mockResolvedValue({ visitCount: 9 })
    const store = useSiteStore()

    try {
      await expect(store.recordVisit()).resolves.toBeUndefined()
      expect(siteApi.recordSiteVisit).toHaveBeenCalledTimes(1)
      expect(store.visitCount).toBe(9)
    } finally {
      getItem.mockRestore()
      setItem.mockRestore()
    }
  })

  it('removes the svg type for uploaded favicons and restores it on fallback', async () => {
    document.head.innerHTML = '<link rel="icon" type="image/svg+xml" href="/favicon.svg" />'
    siteApi.getSite.mockResolvedValue(siteView({ faviconUrl: '/api/site/favicon?v=3' }))
    const store = useSiteStore()
    await store.ensureLoaded()

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')!
    expect(link.getAttribute('href')).toBe('/api/site/favicon?v=3')
    expect(link.hasAttribute('type')).toBe(false)

    store.setConfig(siteView({ faviconUrl: '' }))
    await nextTick()

    expect(link.getAttribute('href')).toBe('/favicon.svg')
    expect(link.getAttribute('type')).toBe('image/svg+xml')
  })
})
