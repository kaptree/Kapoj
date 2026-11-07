import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Avatar from 'primevue/avatar'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import { ApiError } from '@/api/http'
import { getRatingHistory } from '@/api/leaderboard'
import { getProfile } from '@/api/users'
import type { Profile } from '@/api/types'
import UserProfileView from './UserProfileView.vue'

vi.mock('@/api/users', () => ({
  getProfile: vi.fn(),
}))

vi.mock('@/api/leaderboard', () => ({
  getRatingHistory: vi.fn(),
}))

function profile(overrides: Partial<Profile> = {}): Profile {
  return {
    id: 7,
    username: 'alice',
    nickname: 'Alice',
    avatarUrl: '',
    bio: '热爱算法与开源。',
    rating: 1680,
    coins: 1234,
    equippedBadges: [{ id: 1, name: '早鸟', imageUrl: '' }],
    equippedTitle: { id: 1, name: '卷王', color: '#7C3AED' },
    createdAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const routes = [
  { path: '/users/:id', name: 'user-profile', component: { template: '<div />' } },
  { path: '/contests/:id(\\d+)', name: 'contest-detail', component: { template: '<div />' } },
]

async function mountPage(path = '/users/7') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(UserProfileView, { global: { plugins: [router, PrimeVue] } })
  await flushPromises()
  return { wrapper, router }
}

beforeEach(() => {
  vi.mocked(getProfile).mockReset()
  vi.mocked(getProfile).mockResolvedValue(profile())
  vi.mocked(getRatingHistory).mockReset()
  vi.mocked(getRatingHistory).mockResolvedValue({ total: 0, items: [] })
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
})
