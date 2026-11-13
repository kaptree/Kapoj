import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick, reactive } from 'vue'

import { ApiError } from '@/api/http'
import { getLearningTree } from '@/api/learning'
import type { LearningPathItem, LearningPathNodeItem, LearningTree, Me } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import LearningPathDetailView from './LearningPathDetailView.vue'

const route = reactive({ params: { id: '3' } })
const auth = vi.hoisted(() => ({
  isLoggedIn: true,
  user: null as Me | null,
}))

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return { ...actual, useRoute: () => route }
})
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/learning', () => ({
  getLearningTree: vi.fn(),
}))
vi.mock('@/components/MarkdownView.vue', () => ({
  default: {
    props: ['source', 'theme'],
    template: '<div class="markdown-stub">{{ source }}</div>',
  },
}))

function path(overrides: Partial<LearningPathItem> = {}): LearningPathItem {
  return {
    id: 3,
    title: '算法入门',
    descriptionMd: '从零开始学习。\n\n逐步掌握算法。',
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

function problem(problemId: number, title = `题目 ${problemId}`, solved = false) {
  return { problemId, title, slug: `problem-${problemId}`, difficulty: 1, solved }
}

function node(overrides: Partial<LearningPathNodeItem> = {}): LearningPathNodeItem {
  return {
    id: 1,
    parentId: null,
    title: '基础语法',
    descriptionMd: '',
    problemIds: [],
    problems: [],
    sortOrder: 0,
    requiredCount: 1,
    solvedCount: 0,
    status: 'locked',
    children: [],
    ...overrides,
  }
}

function mountView() {
  return mount(LearningPathDetailView, {
    global: {
      plugins: [PrimeVue],
      stubs: {
        RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
      },
    },
  })
}

function mockMobileMatchMedia(width = 375) {
  const originalMatchMedia = window.matchMedia
  const originalWidth = window.innerWidth
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: width })
  window.matchMedia = vi.fn((query: string) => {
    return {
      matches: query === '(max-width: 899px)',
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(() => false),
    } as MediaQueryList
  }) as typeof window.matchMedia
  return () => {
    window.matchMedia = originalMatchMedia
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: originalWidth })
  }
}

beforeEach(() => {
  route.params.id = '3'
  auth.isLoggedIn = true
  auth.user = null
  vi.mocked(getLearningTree).mockReset()
})

describe('LearningPathDetailView', () => {
  it('renders the mobile tree and compact summary when the viewport matches mobile', async () => {
    const restoreMedia = mockMobileMatchMedia()
    vi.mocked(getLearningTree).mockResolvedValue({
      path: path(),
      nodes: [node({ id: 1, title: '数组', status: 'active' })],
    })
    const wrapper = mountView()
    try {
      await flushPromises()

      expect(wrapper.find('.mobile-tree').exists()).toBe(true)
      expect(wrapper.find('.tree-viewport').exists()).toBe(false)
      expect(wrapper.find('.path-summary--compact').exists()).toBe(true)
      expect(wrapper.find('h1').text()).toBe('算法入门')
    } finally {
      wrapper.unmount()
      restoreMedia()
    }
  })

  it('keeps the mobile detail branch at a 375px viewport without rendering the desktop tree', async () => {
    const restoreMedia = mockMobileMatchMedia(375)
    vi.mocked(getLearningTree).mockResolvedValue({
      path: path(),
      nodes: [node({ id: 1, title: '数组', status: 'active' })],
    })
    const wrapper = mountView()
    try {
      await flushPromises()

      expect(window.innerWidth).toBe(375)
      expect(wrapper.find('.mobile-tree').exists()).toBe(true)
      expect(wrapper.find('.tree-viewport').exists()).toBe(false)
    } finally {
      wrapper.unmount()
      restoreMedia()
    }
  })

  it('renders path metadata, problem summary and completion after loading', async () => {
    const nodes: LearningPathNodeItem[] = [
      node({
        id: 1,
        title: '基础语法',
        status: 'completed',
        solvedCount: 2,
        requiredCount: 2,
        problems: [problem(101, '题目 101', true)],
      }),
      node({
        id: 2,
        parentId: 1,
        title: '数组',
        status: 'active',
        solvedCount: 1,
        requiredCount: 3,
        problems: [problem(102), problem(103)],
      }),
      node({
        id: 3,
        parentId: 1,
        title: '链表',
        status: 'locked',
        solvedCount: 0,
        requiredCount: 2,
        problems: [problem(104)],
      }),
    ]
    vi.mocked(getLearningTree).mockResolvedValue({ path: path(), nodes })
    const wrapper = mountView()
    await flushPromises()

    expect(getLearningTree).toHaveBeenCalledWith('3')
    expect(wrapper.find('h1').text()).toBe('算法入门')
    expect(wrapper.text()).toContain('管理员')
    expect(wrapper.text()).toContain('官方')
    expect(wrapper.text()).toContain('已完成 1 / 3 节点')
    expect(wrapper.text()).toContain('已完成 1 / 4 题')
    expect(wrapper.findAll('.tree-node')).toHaveLength(3)
    expect(wrapper.find('.tree-node[aria-label^="基础语法"]').classes()).toContain(
      'status-completed',
    )
    expect(wrapper.find('.tree-node[aria-label^="数组"]').classes()).toContain('status-active')
    expect(wrapper.find('.tree-node[aria-label^="链表"]').classes()).toContain('status-locked')
    wrapper.unmount()
  })

  it('defaults to the first active node and updates the dossier on tree selection', async () => {
    const nodes: LearningPathNodeItem[] = [
      node({
        id: 1,
        title: '基础语法',
        status: 'completed',
        descriptionMd: '基础语法描述',
        problems: [problem(101)],
        children: [
          node({
            id: 2,
            parentId: 1,
            title: '数组',
            status: 'active',
            descriptionMd: '数组节点描述',
            problems: [problem(102)],
          }),
        ],
      }),
    ]
    vi.mocked(getLearningTree).mockResolvedValue({ path: path(), nodes })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('数组节点描述')
    expect(wrapper.text()).toContain('NODE // 02')
    expect(wrapper.find('a[href="/problems/102"]').exists()).toBe(true)
    expect(wrapper.find('.tree-node[aria-label^="数组"]').classes()).toContain('selected')
    expect(wrapper.find('.sr-only[aria-live="polite"]').text()).toBe('已选择 数组')

    await wrapper.find('.tree-node[aria-label^="基础语法"]').trigger('click')

    expect(wrapper.text()).toContain('基础语法描述')
    expect(wrapper.find('a[href="/problems/101"]').exists()).toBe(true)
    expect(wrapper.find('.tree-node[aria-label^="基础语法"]').classes()).toContain('selected')
    expect(wrapper.find('.sr-only[aria-live="polite"]').text()).toBe('已选择 基础语法')
    wrapper.unmount()
  })

  it('falls back to the first node when no node is active', async () => {
    vi.mocked(getLearningTree).mockResolvedValue({
      path: path(),
      nodes: [
        node({ id: 1, title: '第一个节点', status: 'completed' }),
        node({ id: 2, title: '第二个节点', status: 'locked' }),
      ],
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('.tree-node[aria-label^="第一个节点"]').classes()).toContain('selected')
    expect(wrapper.text()).toContain('NODE // 01')
    expect(wrapper.find('.sr-only[aria-live="polite"]').text()).toBe('已选择 第一个节点')
    wrapper.unmount()
  })

  it('shows selected leaf problem links but no leaves for a node with children', async () => {
    const root = node({
      id: 1,
      title: '根节点',
      status: 'active',
      problems: [problem(101)],
      children: [
        node({
          id: 2,
          parentId: 1,
          title: '叶子节点',
          status: 'locked',
          problems: [problem(202)],
        }),
      ],
    })
    vi.mocked(getLearningTree).mockResolvedValue({ path: path(), nodes: [root] })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('.tree-node[aria-label^="根节点"]').classes()).toContain('selected')
    expect(wrapper.findAll('.tree-leaf')).toHaveLength(0)

    await wrapper.find('.tree-node[aria-label^="叶子节点"]').trigger('click')

    expect(wrapper.find('a.tree-leaf[href="/problems/202"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/problems/202"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('expands and collapses the route description', async () => {
    vi.mocked(getLearningTree).mockResolvedValue({ path: path(), nodes: [node()] })
    const wrapper = mountView()
    await flushPromises()

    const description = wrapper.get('.path-description__content')
    expect(description.classes()).not.toContain('path-description--expanded')
    expect(description.find('.markdown-stub').text()).toContain('逐步掌握算法。')
    expect(wrapper.text()).toContain('展开')
    const toggle = wrapper.get('button.path-description__toggle')
    expect(description.attributes('id')).toBe('path-description-content')
    expect(toggle.attributes('aria-controls')).toBe('path-description-content')

    await toggle.trigger('click')
    expect(description.classes()).toContain('path-description--expanded')
    expect(wrapper.text()).toContain('收起')

    await toggle.trigger('click')
    expect(description.classes()).not.toContain('path-description--expanded')
    expect(wrapper.text()).toContain('展开')
    wrapper.unmount()
  })

  it('omits the route description section when it is empty', async () => {
    vi.mocked(getLearningTree).mockResolvedValue({
      path: path({ descriptionMd: '' }),
      nodes: [node()],
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('.path-description').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows guests only the total problem count and locks non-root nodes', async () => {
    auth.isLoggedIn = false
    const nodes = [
      node({
        id: 1,
        title: '根节点',
        status: 'active',
        problems: [problem(101), problem(102)],
        children: [
          node({
            id: 2,
            parentId: 1,
            title: '子节点',
            status: 'locked',
            problems: [problem(103)],
          }),
        ],
      }),
    ]
    vi.mocked(getLearningTree).mockResolvedValue({
      path: path({ isOfficial: false, isPublic: true, nodeCount: 2, myCompletedCount: 0 }),
      nodes,
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('公开')
    expect(wrapper.get('.summary-problem-progress').text()).toBe('3 题')
    expect(wrapper.text()).not.toContain('已完成 0 / 3 题')
    expect(wrapper.find('.tree-node[aria-label^="根节点"]').classes()).toContain('status-active')
    expect(wrapper.find('.tree-node[aria-label^="子节点"]').classes()).toContain('status-locked')
    wrapper.unmount()
  })

  it('scrolls the dossier problem list into view when a tree leaf shows more problems', async () => {
    const scrollIntoView = vi.fn()
    const problems = Array.from({ length: 6 }, (_, index) => problem(200 + index))
    vi.mocked(getLearningTree).mockResolvedValue({
      path: path(),
      nodes: [node({ id: 1, status: 'active', problems })],
    })
    const wrapper = mountView()
    await flushPromises()
    const problemList = wrapper.get('.panel-problems').element as HTMLUListElement
    problemList.scrollIntoView = scrollIntoView

    await wrapper.get('button.tree-leaf--more').trigger('click')
    await nextTick()

    expect(wrapper.text()).toContain('NODE // 01')
    expect(scrollIntoView).toHaveBeenCalledWith({ behavior: 'smooth', block: 'nearest' })
    wrapper.unmount()
  })

  it('shows an empty state when the path contains no nodes', async () => {
    vi.mocked(getLearningTree).mockResolvedValue({ path: path(), nodes: [] })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findComponent(AppEmptyState).exists()).toBe(true)
    expect(wrapper.findComponent(AppEmptyState).props('title')).toBe('该路线还没有节点')
    expect(wrapper.find('.desktop-layout').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows not-found/forbidden error state for a private path', async () => {
    vi.mocked(getLearningTree).mockRejectedValue(
      new ApiError(404, 40400, '学习路线不存在或无权查看'),
    )
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('学习路线不存在或无权查看')
    expect(wrapper.findComponent(AppErrorState).exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows a generic error for other failures', async () => {
    vi.mocked(getLearningTree).mockRejectedValue(new ApiError(500, 50000, '服务器错误'))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('加载学习路线失败，请稍后再试')
    expect(wrapper.findComponent(AppErrorState).exists()).toBe(true)
    wrapper.unmount()
  })

  it('clears the old route and selection before loading a changed id', async () => {
    vi.mocked(getLearningTree).mockResolvedValueOnce({
      path: path({ id: 3, title: '旧路线' }),
      nodes: [node({ id: 1, title: '旧选中节点', status: 'active' })],
    })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('旧路线')
    expect(wrapper.text()).toContain('旧选中节点')
    await wrapper.get('button.path-description__toggle').trigger('click')
    expect(wrapper.get('.path-description__content').classes()).toContain(
      'path-description--expanded',
    )

    let resolveSecond!: (value: LearningTree) => void
    vi.mocked(getLearningTree).mockImplementationOnce(
      () => new Promise((resolve) => (resolveSecond = resolve)),
    )
    route.params.id = '4'
    await nextTick()

    expect(wrapper.text()).not.toContain('旧路线')
    expect(wrapper.text()).not.toContain('旧选中节点')
    expect(wrapper.find('.desktop-layout').exists()).toBe(false)
    expect(wrapper.text()).toContain('正在加载学习路线')

    resolveSecond({
      path: path({ id: 4, title: '新路线' }),
      nodes: [node({ id: 2, title: '新选中节点', status: 'active' })],
    })
    await flushPromises()

    expect(wrapper.text()).toContain('新路线')
    expect(wrapper.text()).toContain('新选中节点')
    expect(wrapper.text()).not.toContain('旧选中节点')
    expect(wrapper.find('.sr-only[aria-live="polite"]').text()).toBe('已选择 新选中节点')
    expect(wrapper.get('.path-description__content').classes()).not.toContain(
      'path-description--expanded',
    )
    wrapper.unmount()
  })
})
