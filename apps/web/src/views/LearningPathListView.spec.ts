import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import SelectButton from 'primevue/selectbutton'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/http'
import { listLearningPaths } from '@/api/learning'
import type { LearningPathItem, Me } from '@/api/types'
import LearningPathListView from './LearningPathListView.vue'

const auth = vi.hoisted(() => ({
  isLoggedIn: true,
  can: vi.fn<(code: string) => boolean>(() => false),
  user: null as Me | null,
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => auth,
}))
vi.mock('@/api/learning', () => ({
  listLearningPaths: vi.fn(),
}))

function me(overrides: Partial<Me> = {}): Me {
  return {
    id: 1,
    username: 'alice',
    nickname: 'Alice',
    email: 'alice@example.com',
    avatarUrl: '',
    bio: '',
    role: 'user',
    roles: [],
    permissions: [],
    isSuperAdmin: false,
    rating: 1600,
    coins: 0,
    status: 'active',
    createdAt: '2025-01-01T00:00:00Z',
    ...overrides,
  }
}

function path(overrides: Partial<LearningPathItem> = {}): LearningPathItem {
  return {
    id: 1,
    title: '算法入门',
    descriptionMd: '从零开始学习基础算法。',
    coverUrl: '',
    isOfficial: true,
    isPublic: true,
    isActive: true,
    author: { id: 1, username: 'admin', nickname: '管理员' },
    nodeCount: 3,
    myCompletedCount: 1,
    sortOrder: 0,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function mountView() {
  return mount(LearningPathListView, {
    global: {
      plugins: [PrimeVue],
      stubs: {
        RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
      },
    },
  })
}

beforeEach(() => {
  auth.isLoggedIn = true
  auth.user = me()
  auth.can.mockReset()
  auth.can.mockReturnValue(false)
  vi.mocked(listLearningPaths).mockReset()
})

describe('LearningPathListView', () => {
  it('loads paths and renders card title, author, progress and tags', async () => {
    vi.mocked(listLearningPaths).mockResolvedValue({
      total: 1,
      items: [path()],
    })
    const wrapper = mountView()
    await flushPromises()

    expect(listLearningPaths).toHaveBeenCalledWith({})
    expect(wrapper.text()).toContain('算法入门')
    expect(wrapper.text()).toContain('管理员')
    expect(wrapper.text()).toContain('已完成 1 / 3 节点')
    expect(wrapper.text()).toContain('官方')
    expect(wrapper.find('a[href="/learning-paths/1"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows 公开/私有 tags for custom paths', async () => {
    vi.mocked(listLearningPaths).mockResolvedValue({
      total: 2,
      items: [
        path({ id: 2, isOfficial: false, isPublic: true, title: '公开路线' }),
        path({ id: 3, isOfficial: false, isPublic: false, title: '私有路线' }),
      ],
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('公开路线')
    expect(wrapper.text()).toContain('私有路线')
    expect(wrapper.text()).toContain('公开')
    expect(wrapper.text()).toContain('私有')
    wrapper.unmount()
  })

  it('filters 官方/自定义 client-side and reloads with mine=true for 我创建的', async () => {
    vi.mocked(listLearningPaths).mockResolvedValue({
      total: 2,
      items: [
        path({ id: 1, isOfficial: true, title: '官方路线' }),
        path({ id: 2, isOfficial: false, isPublic: true, title: '自定义路线' }),
      ],
    })
    const wrapper = mountView()
    await flushPromises()

    wrapper.findComponent(SelectButton).vm.$emit('update:modelValue', 'official')
    await flushPromises()
    expect(wrapper.text()).toContain('官方路线')
    expect(wrapper.text()).not.toContain('自定义路线')

    wrapper.findComponent(SelectButton).vm.$emit('update:modelValue', 'custom')
    await flushPromises()
    expect(wrapper.text()).toContain('自定义路线')
    expect(wrapper.text()).not.toContain('官方路线')

    vi.mocked(listLearningPaths).mockResolvedValue({
      total: 1,
      items: [path({ id: 9, isOfficial: false, isPublic: false, title: '我的路线' })],
    })
    wrapper.findComponent(SelectButton).vm.$emit('update:modelValue', 'mine')
    await flushPromises()
    expect(listLearningPaths).toHaveBeenLastCalledWith({ mine: true })
    expect(wrapper.text()).toContain('我的路线')
    wrapper.unmount()
  })

  it('hides 我创建的 filter and 新建路线 for guests', async () => {
    auth.isLoggedIn = false
    auth.user = null
    vi.mocked(listLearningPaths).mockResolvedValue({ total: 1, items: [path()] })
    const wrapper = mountView()
    await flushPromises()

    const button = wrapper.findComponent(SelectButton)
    expect(button.exists()).toBe(true)
    expect(JSON.stringify(button.props('options'))).not.toContain('我创建的')
    expect(wrapper.find('a[href="/learning-paths/new"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows 新建路线 for logged-in users linking to /learning-paths/new', async () => {
    vi.mocked(listLearningPaths).mockResolvedValue({ total: 1, items: [path()] })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('a[href="/learning-paths/new"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows 编辑 on a card for the author, and on any card for an official manager', async () => {
    const items = [
      path({ id: 1, author: { id: 1, username: 'alice', nickname: 'Alice' } }),
      path({ id: 2, author: { id: 99, username: 'other', nickname: 'Other' } }),
    ]

    vi.mocked(listLearningPaths).mockResolvedValue({ total: 2, items })
    let wrapper = mountView()
    await flushPromises()
    expect(wrapper.findAll('a[href="/learning-paths/1/edit"]').length).toBe(1)
    expect(wrapper.findAll('a[href="/learning-paths/2/edit"]').length).toBe(0)
    wrapper.unmount()

    auth.can.mockReturnValue(true)
    vi.mocked(listLearningPaths).mockResolvedValue({ total: 2, items })
    wrapper = mountView()
    await flushPromises()
    expect(wrapper.findAll('a[href="/learning-paths/1/edit"]').length).toBe(1)
    expect(wrapper.findAll('a[href="/learning-paths/2/edit"]').length).toBe(1)
    wrapper.unmount()
  })

  it('shows empty, error and retry states', async () => {
    vi.mocked(listLearningPaths).mockResolvedValueOnce({ total: 0, items: [] })
    const empty = mountView()
    await flushPromises()
    expect(empty.text()).toContain('暂无学习路线')
    empty.unmount()

    vi.mocked(listLearningPaths).mockRejectedValueOnce(new ApiError(500, 50000, '加载学习路线失败'))
    const failed = mountView()
    await flushPromises()
    expect(failed.text()).toContain('加载学习路线失败')
    expect(failed.text()).toContain('重试')
    failed.unmount()
  })

  it('uses myCompletedCount for the completion label', async () => {
    vi.mocked(listLearningPaths).mockResolvedValue({
      total: 1,
      items: [path({ nodeCount: 7, myCompletedCount: 5 })],
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('已完成 5 / 7 节点')
    wrapper.unmount()
  })
})
