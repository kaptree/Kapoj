import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Avatar from 'primevue/avatar'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import { ApiError } from '@/api/http'
import { getRatingHistory } from '@/api/leaderboard'
import { followUser, getUserSocial, listUserEvaluations, unfollowUser } from '@/api/social'
import { getProfile, getUserActivity } from '@/api/users'
import type { EvaluationPage, Profile, UserActivity } from '@/api/types'
import UserProfileView from './UserProfileView.vue'

const auth = vi.hoisted(() => ({ isLoggedIn: true, user: null as { id: number } | null }))
const toast = vi.hoisted(() => ({ add: vi.fn() }))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))

vi.mock('@/api/users', () => ({
  getProfile: vi.fn(),
  getUserActivity: vi.fn(),
}))

vi.mock('@/api/leaderboard', () => ({
  getRatingHistory: vi.fn(),
}))

vi.mock('@/api/social', () => ({
  getUserSocial: vi.fn(),
  followUser: vi.fn(),
  unfollowUser: vi.fn(),
  listFollowers: vi.fn(),
  listFollowing: vi.fn(),
  listUserEvaluations: vi.fn(),
}))

function profile(overrides: Partial<Profile> = {}): Profile {
  return {
    id: 7,
    username: 'alice',
    nickname: 'Alice',
    avatarUrl: '',
    avatarUploaded: false,
    bio: '热爱算法与开源。',
    rating: 1680,
    coins: 1234,
    equippedBadges: [{ id: 1, name: '早鸟', imageUrl: '' }],
    equippedTitle: { id: 1, name: '卷王', color: '#7C3AED' },
    createdAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function activity(overrides: Partial<UserActivity> = {}): UserActivity {
  return {
    submissionCount: 321,
    acceptedProblemCount: 210,
    contributionTotal: 96,
    startDate: '2025-11-13',
    endDate: '2026-11-12',
    currentStreak: 4,
    longestStreak: 18,
    days: [],
    ...overrides,
  }
}

function evaluationPage(overrides: Partial<EvaluationPage> = {}): EvaluationPage {
  return {
    total: 3,
    items: [],
    cloud: [
      { text: '讲题清楚', count: 2 },
      { text: '大佬', count: 1 },
    ],
    myEvaluation: null,
    canEvaluate: true,
    ...overrides,
  }
}

const routes = [
  { path: '/users/:id', name: 'user-profile', component: { template: '<div />' } },
  { path: '/contests/:id(\\d+)', name: 'contest-detail', component: { template: '<div />' } },
  { path: '/login', name: 'login', component: { template: '<div />' } },
]

const WordCloudStub = {
  name: 'ProfileWordCloud',
  props: ['cloud', 'total', 'loading', 'error'],
  emits: ['open', 'retry'],
  template: '<div data-test="word-cloud" @click="$emit(\'open\')">{{ total }} 条评价</div>',
}

const EvaluationDialogStub = {
  name: 'ProfileEvaluationDialog',
  props: [
    'visible',
    'targetUserId',
    'targetActive',
    'myEvaluation',
    'canEvaluate',
    'isSelf',
    'isLoggedIn',
  ],
  emits: ['update:visible', 'saved', 'deleted'],
  template: '<div v-if="visible" data-test="evaluation-dialog">{{ targetUserId }}</div>',
}

const CertificateCardStub = {
  name: 'ProfileCertificatesCard',
  props: ['targetUserId'],
  template: '<div data-test="certificate-slot">证书 {{ targetUserId }}</div>',
}

async function mountPage(path = '/users/7') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(UserProfileView, {
    global: {
      plugins: [router, PrimeVue],
      stubs: {
        FollowListDialog: true,
        ProfileWordCloud: WordCloudStub,
        ProfileEvaluationDialog: EvaluationDialogStub,
        ProfileCertificatesCard: CertificateCardStub,
      },
    },
  })
  await flushPromises()
  return { wrapper, router }
}

function button(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper'], label: string) {
  return wrapper.findAll('button').find((b) => b.text().trim() === label)
}

function wordCloud(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper']) {
  return wrapper.findComponent({ name: 'ProfileWordCloud' })
}

function evaluationDialog(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper']) {
  return wrapper.findComponent({ name: 'ProfileEvaluationDialog' })
}

beforeEach(() => {
  auth.isLoggedIn = true
  auth.user = null
  toast.add.mockReset()
  vi.mocked(getProfile).mockReset()
  vi.mocked(getProfile).mockResolvedValue(profile())
  vi.mocked(getUserActivity).mockReset()
  vi.mocked(getUserActivity).mockResolvedValue(activity())
  vi.mocked(getRatingHistory).mockReset()
  vi.mocked(getRatingHistory).mockResolvedValue({ total: 0, items: [] })
  vi.mocked(getUserSocial).mockReset()
  vi.mocked(getUserSocial).mockResolvedValue({
    followersCount: 0,
    followingCount: 0,
    isFollowing: false,
  })
  vi.mocked(listUserEvaluations).mockReset()
  vi.mocked(listUserEvaluations).mockResolvedValue(evaluationPage())
  vi.mocked(followUser).mockReset()
  vi.mocked(unfollowUser).mockReset()
})

describe('UserProfileView', () => {
  it('renders the profile, identity and stats', async () => {
    const { wrapper } = await mountPage('/users/7')

    expect(getProfile).toHaveBeenCalledWith('7')
    expect(wrapper.text()).toContain('Alice')
    expect(wrapper.text()).toContain('@alice')
    expect(wrapper.text()).toContain('卷王')
    expect(wrapper.text()).toContain('热爱算法与开源。')
    expect(wrapper.text()).toContain('Rating')
    expect(wrapper.text()).toContain('1680')
    expect(wrapper.text()).toContain('金币')
    expect(wrapper.text()).toContain('1234')
    expect(wrapper.text()).toContain('注册时间')
    expect(wrapper.text()).toContain(new Date('2026-01-01T00:00:00Z').toLocaleDateString('zh-CN'))
    expect(wrapper.findComponent(Avatar).props('label')).toBe('A')
    wrapper.unmount()
  })

  it('renders the activity stats before Rating and the contribution grid', async () => {
    const { wrapper } = await mountPage('/users/7')

    expect(getUserActivity).toHaveBeenCalledWith('7')
    const labels = wrapper.findAll('.profile-stats dt').map((label) => label.text())
    expect(labels[0]).toContain('提交')
    expect(labels[1]).toBe('Rating')
    expect(labels[2]).toBe('金币')
    expect(labels[3]).toBe('注册时间')

    const career = wrapper.get('[data-test="career-card"]')
    expect(career.text()).toContain('提交总数')
    expect(career.text()).toContain('321')
    expect(career.text()).toContain('通过题数')
    expect(career.text()).toContain('210')
    expect(wrapper.text()).toContain('代码提交贡献图')
    expect(wrapper.text()).toContain('过去一年 96 次代码提交')
    wrapper.unmount()
  })

  it('renders the hero word cloud above the bio and keeps only certificates in the aside', async () => {
    const { wrapper } = await mountPage('/users/7')

    expect(listUserEvaluations).toHaveBeenCalledWith('7', 1, 1)

    const hero = wrapper.get('.profile-hero')
    const surface = wrapper.get('.profile-surface')
    expect(
      hero.element.compareDocumentPosition(surface.element) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
    expect(hero.find('[data-test="word-cloud"]').exists()).toBe(true)
    expect(surface.text()).toContain('热爱算法与开源。')

    const cloud = wordCloud(wrapper)
    expect(cloud.props('total')).toBe(3)
    expect(cloud.props('cloud')).toEqual(evaluationPage().cloud)

    const body = wrapper.get('.profile-body')
    expect(body.find('.profile-main').text()).toContain('代码提交贡献图')
    const aside = body.get('[data-test="profile-aside"]')
    expect(
      Array.from(aside.element.children).map((child) => child.getAttribute('data-test')),
    ).toEqual(['certificate-slot'])
    expect(wrapper.find('[data-test="evaluation-slot"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('opens the evaluation dialog from the word cloud', async () => {
    const { wrapper } = await mountPage('/users/7')

    expect(evaluationDialog(wrapper).props('visible')).toBe(false)

    wordCloud(wrapper).vm.$emit('open')
    await nextTick()

    const dialog = evaluationDialog(wrapper)
    expect(dialog.props('visible')).toBe(true)
    expect(dialog.props('targetUserId')).toBe(7)
    expect(dialog.props('canEvaluate')).toBe(true)
    expect(dialog.props('isSelf')).toBe(false)
    expect(dialog.props('isLoggedIn')).toBe(true)
    wrapper.unmount()
  })

  it('reloads evaluations after the dialog saves or deletes', async () => {
    const { wrapper } = await mountPage('/users/7')

    evaluationDialog(wrapper).vm.$emit('saved')
    await flushPromises()
    expect(listUserEvaluations).toHaveBeenCalledTimes(2)

    evaluationDialog(wrapper).vm.$emit('deleted')
    await flushPromises()
    expect(listUserEvaluations).toHaveBeenCalledTimes(3)
    wrapper.unmount()
  })

  it('keeps the profile when evaluation loading fails', async () => {
    vi.mocked(listUserEvaluations).mockRejectedValue(new ApiError(500, 50000, '评价加载失败'))
    const { wrapper } = await mountPage('/users/7')

    expect(wrapper.text()).toContain('Alice')
    expect(wrapper.text()).toContain('热爱算法与开源。')
    expect(wrapper.text()).toContain('Rating')
    expect(wordCloud(wrapper).props('error')).toBe('评价加载失败')
    wrapper.unmount()
  })

  it('keeps the profile, social and rating history when activity fails', async () => {
    vi.mocked(getUserActivity).mockRejectedValue(new ApiError(500, 50000, '服务器错误'))
    vi.mocked(getUserSocial).mockResolvedValue({
      followersCount: 12,
      followingCount: 5,
      isFollowing: false,
    })
    const { wrapper } = await mountPage('/users/7')

    expect(wrapper.text()).toContain('Alice')
    expect(wrapper.text()).toContain('12')
    expect(wrapper.text()).toContain('Rating 历史')
    expect(wrapper.find('[data-test="activity-error"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('服务器错误')

    vi.mocked(getUserActivity).mockResolvedValue(activity())
    const retry = wrapper
      .findAll('[data-test="activity-error"] button')
      .find((item) => item.text().trim() === '重试')
    expect(retry).toBeTruthy()
    await retry!.trigger('click')
    await flushPromises()

    expect(getUserActivity).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-test="activity-error"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('过去一年 96 次代码提交')
    wrapper.unmount()
  })

  it('renders the rating history card below the profile', async () => {
    vi.mocked(getRatingHistory).mockResolvedValue({
      total: 1,
      items: [
        {
          kind: 'contest',
          contestId: 5,
          contestTitle: '春季公开赛',
          rank: 2,
          periodKey: '',
          ratingBefore: 1600,
          ratingAfter: 1624,
          delta: 24,
          createdAt: '2026-02-01T10:00:00Z',
        },
      ],
    })
    const { wrapper } = await mountPage('/users/7')

    expect(getRatingHistory).toHaveBeenCalledWith(7, { page: 1, pageSize: 20 })
    expect(wrapper.text()).toContain('Rating 历史')
    expect(wrapper.text()).toContain('春季公开赛')
    expect(wrapper.text()).toContain('+24')
    wrapper.unmount()
  })

  it('shows a loading state before the profile arrives', async () => {
    let resolve!: (value: Profile) => void
    vi.mocked(getProfile).mockReturnValue(
      new Promise<Profile>((r) => {
        resolve = r
      }),
    )
    const { wrapper } = await mountPage('/users/7')

    expect(wrapper.text()).toContain('正在加载用户主页')
    resolve(profile())
    await flushPromises()
    expect(wrapper.text()).toContain('Alice')
    expect(wrapper.text()).not.toContain('正在加载用户主页')
    wrapper.unmount()
  })

  it('falls back when the bio is empty', async () => {
    vi.mocked(getProfile).mockResolvedValue(profile({ bio: '' }))
    const { wrapper } = await mountPage('/users/7')

    expect(wrapper.text()).toContain('这个人还没有写简介。')
    wrapper.unmount()
  })

  it('shows a not-found error and retries', async () => {
    vi.mocked(getProfile)
      .mockRejectedValueOnce(new ApiError(404, 40400, '用户不存在'))
      .mockResolvedValueOnce(profile())
    const { wrapper } = await mountPage('/users/7')

    expect(wrapper.text()).toContain('用户不存在')
    const retry = wrapper.findAll('button').find((item) => item.text().trim() === '重试')
    expect(retry).toBeTruthy()
    await retry!.trigger('click')
    await flushPromises()
    expect(getProfile).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('Alice')
    wrapper.unmount()
  })

  it('shows a generic load error', async () => {
    vi.mocked(getProfile).mockRejectedValue(new ApiError(500, 50000, '服务器错误'))
    const { wrapper } = await mountPage('/users/7')

    expect(wrapper.text()).toContain('加载失败，请稍后再试')
    wrapper.unmount()
  })

  it('reloads when the route id changes', async () => {
    vi.mocked(getProfile).mockImplementation(async (id) =>
      profile({
        id: Number(id),
        username: id === '7' ? 'alice' : 'bob',
        nickname: id === '7' ? 'Alice' : 'Bob',
        bio: id === '7' ? 'Alice bio' : '',
      }),
    )
    const { wrapper, router } = await mountPage('/users/7')
    expect(wrapper.text()).toContain('Alice')

    await router.push('/users/8')
    await flushPromises()
    expect(getProfile).toHaveBeenLastCalledWith('8')
    expect(listUserEvaluations).toHaveBeenLastCalledWith('8', 1, 1)
    expect(wrapper.text()).toContain('Bob')
    expect(wrapper.text()).toContain('@bob')
    wrapper.unmount()
  })

  it('falls back to the initial when the avatar image fails', async () => {
    vi.mocked(getProfile).mockResolvedValue(profile({ avatarUrl: '/avatar.png' }))
    const { wrapper } = await mountPage('/users/7')

    const avatar = wrapper.findComponent(Avatar)
    expect(avatar.props('image')).toBe('/avatar.png')
    expect(avatar.props('label')).toBeFalsy()
    avatar.vm.$emit('error', new Event('error'))
    await nextTick()

    expect(avatar.props('image')).toBeFalsy()
    expect(avatar.props('label')).toBe('A')
    wrapper.unmount()
  })

  it('shows social counts and opens the follower dialog', async () => {
    vi.mocked(getUserSocial).mockResolvedValue({
      followersCount: 12,
      followingCount: 5,
      isFollowing: false,
    })
    const { wrapper } = await mountPage('/users/7')

    expect(getUserSocial).toHaveBeenCalledWith('7')
    expect(wrapper.text()).toContain('12')
    expect(wrapper.text()).toContain('5')

    await wrapper.find('.social-stat--followers').trigger('click')
    await flushPromises()

    const dialog = wrapper.findComponent({ name: 'FollowListDialog' })
    expect(dialog.props('visible')).toBe(true)
    expect(dialog.props('initialTab')).toBe('followers')
    wrapper.unmount()
  })

  it('shows the follow button to a logged-in visitor and follows', async () => {
    auth.user = { id: 99 }
    vi.mocked(getUserSocial).mockResolvedValue({
      followersCount: 12,
      followingCount: 5,
      isFollowing: false,
    })
    vi.mocked(followUser).mockResolvedValue({ following: true, followersCount: 13 })
    const { wrapper } = await mountPage('/users/7')

    await button(wrapper, '关注')!.trigger('click')
    await flushPromises()

    expect(followUser).toHaveBeenCalledWith(7)
    expect(wrapper.text()).toContain('已关注')
    expect(wrapper.text()).toContain('13')
    wrapper.unmount()
  })

  it('hides the follow button on your own profile', async () => {
    auth.user = { id: 7 }
    const { wrapper } = await mountPage('/users/7')

    expect(button(wrapper, '关注')).toBeUndefined()
    expect(button(wrapper, '已关注')).toBeUndefined()
    expect(button(wrapper, '登录后关注')).toBeUndefined()
    wrapper.unmount()
  })

  it('shows a login button to guests and redirects to login', async () => {
    auth.isLoggedIn = false
    auth.user = null
    const { wrapper, router } = await mountPage('/users/7')

    await button(wrapper, '登录后关注')!.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('login')
    expect(router.currentRoute.value.query.redirect).toBe('/users/7')
    wrapper.unmount()
  })
})
