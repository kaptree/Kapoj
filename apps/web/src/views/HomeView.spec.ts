import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type {
  ActivitySummary,
  Me,
  RatingHistoryItem,
  SitePublicView,
  SubmissionListItem,
  TaskItem,
  UserActivity,
} from '@/api/types'
import { GUEST_PERM } from '@/permissions'
import { useCompanionStore } from '@/stores/companion'
import { DEFAULT_FOOTER, DEFAULT_HOME, useSiteStore } from '@/stores/site'
import HomeView from './HomeView.vue'

const baseUser: Me = {
  id: 7,
  username: 'alice',
  nickname: 'Alice',
  email: 'alice@example.com',
  emailVerified: true,
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

const today = '2026-11-15'

const activity: UserActivity = {
  submissionCount: 34,
  acceptedProblemCount: 12,
  contributionTotal: 20,
  startDate: '2025-11-16',
  endDate: today,
  currentStreak: 4,
  longestStreak: 9,
  days: [{ date: today, count: 2 }],
}

const activitySummary: ActivitySummary = {
  signedInToday: true,
  todayPoints: 5,
  totalPoints: 120,
}

const recentSubmission: SubmissionListItem = {
  id: 101,
  userId: 7,
  username: 'alice',
  nickname: 'Alice',
  equippedBadges: [],
  equippedTitle: null,
  problemId: 42,
  problemTitle: 'A + B',
  problemSlug: 'a-b',
  language: 'cpp',
  status: 'accepted',
  timeUsedMs: 12,
  memoryUsedKb: 1024,
  score: 100,
  codeLength: 128,
  createdAt: '2026-11-15T10:00:00Z',
}

const ratingEntry: RatingHistoryItem = {
  kind: 'contest',
  contestId: 3,
  contestTitle: 'Weekly Round 1',
  rank: 5,
  periodKey: '',
  ratingBefore: 1600,
  ratingAfter: 1680,
  delta: 80,
  createdAt: '2026-11-10T10:00:00Z',
}

const claimableTask: TaskItem = {
  id: 1,
  title: '热身任务',
  descriptionMd: '',
  problemIds: [1, 2],
  rewardCoins: 50,
  startTime: '2026-11-01T00:00:00Z',
  endTime: '2026-12-01T00:00:00Z',
  isActive: true,
  status: 'active',
  claimedAt: null,
  completedAt: null,
  progress: 0,
  problemCount: 2,
  hasRecords: false,
}

const auth = vi.hoisted(() => ({
  isLoggedIn: false,
  user: null as Me | null,
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => auth,
}))

const guest = vi.hoisted(() => ({
  permissions: [] as string[],
  loaded: false,
  ensureLoaded: vi.fn(),
  can: vi.fn<(code: string) => boolean>(() => false),
}))

vi.mock('@/stores/guest', () => ({
  useGuestStore: () => guest,
}))

const siteApi = vi.hoisted(() => ({
  getSite: vi.fn(),
  recordSiteVisit: vi.fn(),
}))

vi.mock('@/api/site', () => siteApi)

const usersApi = vi.hoisted(() => ({
  getUserActivity: vi.fn(),
}))

vi.mock('@/api/users', () => usersApi)

const submissionsApi = vi.hoisted(() => ({
  listSubmissions: vi.fn(),
}))

vi.mock('@/api/submissions', () => submissionsApi)

const leaderboardApi = vi.hoisted(() => ({
  getRatingHistory: vi.fn(),
  getActivitySummary: vi.fn(),
}))

vi.mock('@/api/leaderboard', () => leaderboardApi)

const gamificationApi = vi.hoisted(() => ({
  listTasks: vi.fn(),
}))

vi.mock('@/api/gamification', () => gamificationApi)

const routes = [
  { path: '/', name: 'home', component: { template: '<div />' } },
  { path: '/problems', name: 'problems', component: { template: '<div />' } },
  { path: '/problems/:id', name: 'problem-detail', component: { template: '<div />' } },
  { path: '/contests', name: 'contests', component: { template: '<div />' } },
  { path: '/leaderboard', name: 'leaderboard', component: { template: '<div />' } },
  { path: '/submissions', name: 'submissions', component: { template: '<div />' } },
  { path: '/achievements', name: 'achievements', component: { template: '<div />' } },
  { path: '/tasks', name: 'tasks', component: { template: '<div />' } },
  { path: '/shop/badges', name: 'badge-shop', component: { template: '<div />' } },
  { path: '/shop/titles', name: 'title-shop', component: { template: '<div />' } },
  { path: '/appearance', name: 'appearance', component: { template: '<div />' } },
  { path: '/users/:id', name: 'user-profile', component: { template: '<div />' } },
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
    appearance: { badgeSize: 22 },
    startedAt: '2026-01-01T00:00:00Z',
    runningDays: 312,
    visitCount: 42,
    ...overrides,
  }
}

function mockHealth(status = 'ok') {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({
      ok: status === 'ok',
      json: async () => ({ code: 0, message: 'ok', data: { status } }),
    })),
  )
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
  await flushPromises()
  return { wrapper, router }
}

function hrefs(wrapper: Awaited<ReturnType<typeof mountHome>>['wrapper']) {
  return wrapper.findAll('a').map((link) => link.attributes('href'))
}

beforeEach(() => {
  localStorage.removeItem('kapoj.companion.prefs')
  auth.isLoggedIn = false
  auth.user = null
  guest.permissions = []
  guest.loaded = false
  guest.can.mockReset()
  guest.can.mockReturnValue(false)
  siteApi.getSite.mockReset()
  siteApi.recordSiteVisit.mockReset()
  usersApi.getUserActivity.mockReset()
  submissionsApi.listSubmissions.mockReset()
  leaderboardApi.getRatingHistory.mockReset()
  leaderboardApi.getActivitySummary.mockReset()
  gamificationApi.listTasks.mockReset()

  usersApi.getUserActivity.mockResolvedValue(activity)
  submissionsApi.listSubmissions.mockResolvedValue({ total: 1, items: [recentSubmission] })
  leaderboardApi.getRatingHistory.mockResolvedValue({ total: 1, items: [ratingEntry] })
  leaderboardApi.getActivitySummary.mockResolvedValue(activitySummary)
  gamificationApi.listTasks.mockResolvedValue([])
  mockHealth()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('HomeView', () => {
  it('shows the hero and quick links to guests while hiding growth by default', async () => {
    const { wrapper } = await mountHome()
    const links = hrefs(wrapper)

    expect(wrapper.find('h1').text()).toContain('欢迎来到 kapOJ')
    expect(wrapper.text()).toContain('每一次提交，都是向更高 Rating 迈进一步。')
    expect(wrapper.text()).toContain('排行榜、签到与 Rating 历史')
    expect(wrapper.text()).toContain('M4-C')
    expect(wrapper.text()).toContain('平台运行正常')
    expect(links).toEqual(
      expect.arrayContaining([
        '/problems',
        '/contests',
        '/leaderboard',
        '/submissions',
        '/login',
        '/register',
      ]),
    )
    expect(links).not.toContain('/achievements')
    expect(links).not.toContain('/tasks')
    expect(links).not.toContain('/shop/badges')
    expect(links).not.toContain('/shop/titles')
    expect(links).not.toContain('/appearance')
    expect(wrapper.text()).not.toContain('成长中心')
    expect(wrapper.text()).not.toContain('学习概览')
    expect(wrapper.text()).toContain('登录后记录你的成长')
    expect(usersApi.getUserActivity).not.toHaveBeenCalled()
    expect(submissionsApi.listSubmissions).not.toHaveBeenCalled()
    expect(leaderboardApi.getRatingHistory).not.toHaveBeenCalled()
  })

  it('shows the companion and greeting in the hero', async () => {
    const { wrapper } = await mountHome()
    expect(wrapper.find('.hero__companion').exists()).toBe(true)
    expect(wrapper.find('.companion-bubble').exists()).toBe(true)
    expect(wrapper.find('.hero__companion').text().length).toBeGreaterThan(0)
  })

  it('shows the abstract fallback when the companion is disabled', async () => {
    const { wrapper } = await mountHome()
    const store = useCompanionStore()
    store.load()
    store.setEnabled(false)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.hero__companion').exists()).toBe(false)
    expect(wrapper.find('.hero__signal').exists()).toBe(true)
    store.reset()
  })

  it('falls back to the default home copy when the site config is unavailable', async () => {
    guest.can.mockReturnValue(true)
    const { wrapper } = await mountHome()

    expect(wrapper.find('h1').text()).toContain('欢迎来到 kapOJ')
    expect(wrapper.text()).toContain('快速开始')
    expect(wrapper.text()).toContain('成长中心')
    expect(wrapper.text()).toContain('开始刷题')
    expect(wrapper.text()).toContain('浏览比赛')
  })

  it('renders custom hero, quick links, growth links and guest copy from the site config', async () => {
    guest.can.mockReturnValue(true)
    const { wrapper } = await mountHome({
      site: {
        home: {
          ...DEFAULT_HOME,
          eyebrow: 'CUSTOM // JUDGE',
          title: '自定义欢迎',
          titleHighlight: 'KAP',
          tagline: '自定义品牌宣传语。',
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
    expect(wrapper.text()).toContain('自定义品牌宣传语。')
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
    guest.can.mockReturnValue(true)
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

  it('shows configured growth links without appearance to guests with guest.growth.view', async () => {
    guest.can.mockReturnValue(true)
    const { wrapper } = await mountHome()
    const links = hrefs(wrapper)

    expect(guest.can).toHaveBeenCalledWith(GUEST_PERM.growth)
    expect(wrapper.text()).toContain('成长中心')
    expect(links).toContain('/achievements')
    expect(links).toContain('/tasks')
    expect(links).toContain('/shop/badges')
    expect(links).toContain('/shop/titles')
    expect(links).not.toContain('/appearance')
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
    expect(links).toContain('/achievements')
    expect(links).toContain('/tasks')
    expect(links).toContain('/appearance')
    expect(links).toContain('/settings/profile')
    expect(wrapper.text()).not.toContain('登录后记录你的成长')
  })

  it('renders real profile stats, daily goal, recent practice and rating trend', async () => {
    auth.isLoggedIn = true
    auth.user = baseUser
    gamificationApi.listTasks.mockResolvedValue([claimableTask])
    const { wrapper } = await mountHome()
    const links = hrefs(wrapper)

    expect(wrapper.text()).toContain('通过题目')
    expect(wrapper.text()).toContain('12')
    expect(wrapper.text()).toContain('连续练习')
    expect(wrapper.text()).toContain('4天')
    expect(wrapper.text()).toContain('每日目标')
    expect(wrapper.text()).toContain('2 / 2')
    expect(wrapper.text()).toContain('今日活跃度')
    expect(wrapper.text()).toContain('最近练习')
    expect(wrapper.text()).toContain('A + B')
    expect(wrapper.text()).toContain('通过')
    expect(wrapper.text()).toContain('Rating 趋势')
    expect(wrapper.text()).toContain('+80')
    expect(wrapper.text()).toContain('Weekly Round 1')
    expect(wrapper.text()).toContain('1 个可领取')
    expect(links).toContain('/users/7')
    expect(links).toContain('/problems/42')
    expect(usersApi.getUserActivity).toHaveBeenCalledWith(7)
    expect(submissionsApi.listSubmissions).toHaveBeenCalledWith(
      expect.objectContaining({ userId: 7, page: 1, pageSize: 4 }),
    )
    expect(leaderboardApi.getRatingHistory).toHaveBeenCalledWith(7, { page: 1, pageSize: 8 })
    expect(leaderboardApi.getActivitySummary).toHaveBeenCalled()
  })

  it('renders a fully real daily goal state instead of demo numbers', async () => {
    auth.isLoggedIn = true
    auth.user = baseUser
    leaderboardApi.getActivitySummary.mockResolvedValue({
      signedInToday: false,
      todayPoints: 0,
      totalPoints: 120,
    })
    usersApi.getUserActivity.mockResolvedValue({
      ...activity,
      days: [{ date: today, count: 0 }],
    })

    const { wrapper } = await mountHome()

    expect(wrapper.text()).toContain('0 / 2')
    expect(wrapper.text()).toContain('待完成')
    expect(wrapper.text()).not.toContain('1 个可领取')
  })

  it('does not render overview data or claim badges when the task list is empty', async () => {
    auth.isLoggedIn = true
    auth.user = baseUser
    const { wrapper } = await mountHome()

    expect(wrapper.text()).not.toContain('个可领取')
    expect(usersApi.getUserActivity).toHaveBeenCalledTimes(1)
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
