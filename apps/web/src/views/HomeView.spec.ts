import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { Me, SitePublicView } from '@/api/types'
import { DEFAULT_FOOTER, DEFAULT_HOME, useSiteStore } from '@/stores/site'
import HomeView from './HomeView.vue'

const baseUser: Me = {
  id: 7,
  username: 'alice',
  nickname: 'Alice',
  email: 'alice@example.com',
  avatarUrl: '',
  avatarUploaded: false,
  bio: '',
  role: 'user',
  roles: [],
  permissions: [],
  isSuperAdmin: false,
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

const siteApi = vi.hoisted(() => ({
  getSite: vi.fn(),
  recordSiteVisit: vi.fn(),
}))

vi.mock('@/api/site', () => siteApi)

const routes = [
  { path: '/', name: 'home', component: { template: '<div />' } },
  { path: '/problems', name: 'problems', component: { template: '<div />' } },
  { path: '/contests', name: 'contests', component: { template: '<div />' } },
  { path: '/leaderboard', name: 'leaderboard', component: { template: '<div />' } },
  { path: '/submissions', name: 'submissions', component: { template: '<div />' } },
  { path: '/achievements', name: 'achievements', component: { template: '<div />' } },
  { path: '/tasks', name: 'tasks', component: { template: '<div />' } },
  { path: '/shop/badges', name: 'badge-shop', component: { template: '<div />' } },
  { path: '/shop/titles', name: 'title-shop', component: { template: '<div />' } },
  { path: '/appearance', name: 'appearance', component: { template: '<div />' } },
  { path: '/custom-problem', name: 'custom-problem', component: { template: '<div />' } },
  { path: '/custom-growth', name: 'custom-growth', component: { template: '<div />' } },
  { path: '/login', name: 'login', component: { template: '<div />' } },
  { path: '/register', name: 'register', component: { template: '<div />' } },
  { path: '/settings/profile', name: 'profile-edit', component: { template: '<div />' } },
]

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

async function mountHome(options: { site?: Partial<SitePublicView> } = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  if (options.site) {
    siteApi.getSite.mockResolvedValue(siteView(options.site))
    await useSiteStore().ensureLoaded()
  }
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/')
  await router.isReady()
  const wrapper = mount(HomeView, { global: { plugins: [router, PrimeVue, pinia] } })
  return { wrapper, router }
}

function hrefs(wrapper: Awaited<ReturnType<typeof mountHome>>['wrapper']) {
  return wrapper.findAll('a').map((link) => link.attributes('href'))
}

beforeEach(() => {
  auth.isLoggedIn = false
  auth.user = null
  siteApi.getSite.mockReset()
  siteApi.recordSiteVisit.mockReset()
})

describe('HomeView', () => {
  it('shows the hero, system status and quick links to guests', async () => {
    const { wrapper } = await mountHome()
    const links = hrefs(wrapper)

    expect(wrapper.find('h1').text()).toContain('欢迎来到 kapOJ')
    expect(wrapper.text()).toContain('排行榜、签到与 Rating 历史')
    expect(wrapper.text()).toContain('M4-C')
    expect(links).toEqual(
      expect.arrayContaining([
        '/problems',
        '/contests',
        '/leaderboard',
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

  it('falls back to the default home copy when the site config is unavailable', async () => {
    const { wrapper } = await mountHome()

    expect(wrapper.find('h1').text()).toContain('欢迎来到 kapOJ')
    expect(wrapper.text()).toContain('快速开始')
    expect(wrapper.text()).toContain('成长中心')
    expect(wrapper.text()).toContain('开始刷题')
    expect(wrapper.text()).toContain('浏览比赛')
  })

  it('renders custom hero, quick links, growth links and guest copy from the site config', async () => {
    const { wrapper } = await mountHome({
      site: {
        home: {
          ...DEFAULT_HOME,
          eyebrow: 'CUSTOM // JUDGE',
          title: '自定义欢迎',
          titleHighlight: 'KAP',
          description: '这是自定义的站点说明。',
          primaryCta: {
            icon: 'pi pi-code',
            iconPos: 'right',
            title: '提交代码',
            description: '',
            to: '/custom-problem',
          },
          secondaryCta: {
            icon: 'pi pi-compass',
            iconPos: 'left',
            title: '看看比赛',
            description: '',
            to: '/contests',
          },
          quickTitle: '自定义快速',
          quickLinks: [
            {
              icon: 'pi pi-code',
              iconPos: 'left',
              title: '自定义题库',
              description: '自定义卡片说明。',
              to: '/custom-problem',
            },
          ],
          growthTitle: '自定义成长',
          growthLinks: [
            {
              icon: 'pi pi-star',
              iconPos: 'left',
              title: '自定义成长卡',
              description: '成长说明。',
              to: '/custom-growth',
            },
          ],
          guestTitle: '自定义游客标题',
          guestDescription: '自定义游客描述。',
        },
      },
    })
    const links = hrefs(wrapper)

    expect(wrapper.find('h1').text()).toContain('自定义欢迎 KAP')
    expect(wrapper.text()).toContain('CUSTOM // JUDGE')
    expect(wrapper.text()).toContain('这是自定义的站点说明。')
    expect(wrapper.text()).toContain('自定义快速')
    expect(wrapper.text()).toContain('自定义题库')
    expect(wrapper.text()).toContain('自定义卡片说明。')
    expect(wrapper.text()).toContain('自定义成长')
    expect(wrapper.text()).toContain('自定义成长卡')
    expect(wrapper.text()).toContain('自定义游客标题')
    expect(wrapper.text()).toContain('自定义游客描述。')
    expect(wrapper.text()).toContain('提交代码')
    expect(wrapper.text()).toContain('看看比赛')
    expect(links).toContain('/custom-problem')
    expect(links).toContain('/custom-growth')
    expect(links).not.toContain('/problems')
    expect(links).not.toContain('/achievements')
  })

  it('does not render empty quick or growth sections', async () => {
    const { wrapper } = await mountHome({
      site: {
        home: {
          ...DEFAULT_HOME,
          quickLinks: [],
          growthLinks: [],
        },
      },
    })

    expect(wrapper.findAll('.home-section')).toHaveLength(0)
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

  it('appends the appearance entry to configured growth links for logged-in users', async () => {
    auth.isLoggedIn = true
    auth.user = baseUser
    const { wrapper } = await mountHome({
      site: {
        home: {
          ...DEFAULT_HOME,
          growthLinks: [
            {
              icon: 'pi pi-star',
              iconPos: 'left',
              title: '自定义成长卡',
              description: '成长说明。',
              to: '/custom-growth',
            },
          ],
        },
      },
    })
    const links = hrefs(wrapper)

    expect(links).toContain('/custom-growth')
    expect(links).toContain('/appearance')
    expect(wrapper.text()).toContain('自定义成长卡')
  })

  it('renders a loading strip while a logged-in profile is unavailable', async () => {
    auth.isLoggedIn = true
    auth.user = null
    const { wrapper } = await mountHome()

    expect(wrapper.text()).toContain('正在加载你的档案')
    expect(wrapper.text()).not.toContain('登录后记录你的成长')
  })
})
