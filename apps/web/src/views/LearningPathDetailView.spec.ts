import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import ProgressBar from 'primevue/progressbar'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick, reactive } from 'vue'

import { ApiError } from '@/api/http'
import { getLearningTree } from '@/api/learning'
import type { LearningPathItem, LearningPathNodeItem, LearningTree } from '@/api/types'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import LearningPathDetailView from './LearningPathDetailView.vue'

const route = reactive({ params: { id: '3' } })

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return { ...actual, useRoute: () => route }
})
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
    descriptionMd: '从零开始学习。',
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

function problem(problemId: number, title = `题目 ${problemId}`) {
  return { problemId, title, slug: `problem-${problemId}`, difficulty: 1 }
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

beforeEach(() => {
  route.params.id = '3'
  vi.mocked(getLearningTree).mockReset()
})

describe('LearningPathDetailView', () => {
  it('renders path header, tree statuses and progress', async () => {
    const nodes: LearningPathNodeItem[] = [
      node({ id: 1, title: '基础语法', status: 'completed', solvedCount: 2, requiredCount: 2 }),
      node({
        id: 2,
        parentId: 1,
        title: '数组',
        status: 'active',
        solvedCount: 1,
        requiredCount: 3,
      }),
      node({
        id: 3,
        parentId: 1,
        title: '链表',
        status: 'locked',
        solvedCount: 0,
        requiredCount: 2,
      }),
    ]
    vi.mocked(getLearningTree).mockResolvedValue({ path: path(), nodes })
    const wrapper = mountView()
    await flushPromises()

    expect(getLearningTree).toHaveBeenCalledWith('3')
    expect(wrapper.text()).toContain('算法入门')
    expect(wrapper.text()).toContain('管理员')
    expect(wrapper.text()).toContain('官方')
    expect(wrapper.text()).toContain('已完成 1 / 3 节点')
    expect(wrapper.text()).toContain('基础语法')
    expect(wrapper.text()).toContain('数组')
    expect(wrapper.text()).toContain('链表')
    expect(wrapper.text()).toContain('已完成')
    expect(wrapper.text()).toContain('进行中')
    expect(wrapper.text()).toContain('未解锁')
    expect(wrapper.text()).toContain('2 / 2')
    expect(wrapper.text()).toContain('1 / 3')
    expect(wrapper.text()).toContain('0 / 2')

    const bars = wrapper.findAllComponents(ProgressBar)
    expect(bars.map((bar) => bar.props('value'))).toEqual([100, 33, 0])
    wrapper.unmount()
  })

  it('selects a node and shows its problems with links', async () => {
    const nodes: LearningPathNodeItem[] = [
      node({
        id: 1,
        title: '基础语法',
        status: 'completed',
        descriptionMd: '语法描述',
        problemIds: [101, 102],
        problems: [problem(101), problem(102)],
      }),
      node({
        id: 2,
        parentId: 1,
        title: '数组',
        status: 'active',
        descriptionMd: '数组描述',
        problemIds: [103],
        problems: [problem(103)],
      }),
    ]
    vi.mocked(getLearningTree).mockResolvedValue({ path: path(), nodes })
    const wrapper = mountView()
    await flushPromises()

    // Default-selects the first active node (id 2).
    expect(wrapper.text()).toContain('数组描述')
    expect(wrapper.find('a[href="/problems/103"]').exists()).toBe(true)

    // Click the 查看 button on the completed node row.
    const row = wrapper.findAll('tr').find((tr) => tr.text().includes('基础语法'))!
    const viewButton = row.findAll('button').find((button) => button.text().trim() === '查看')!
    await viewButton.trigger('click')

    expect(wrapper.text()).toContain('语法描述')
    expect(wrapper.find('a[href="/problems/101"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/problems/102"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('renders guest locked tree from API', async () => {
    const nodes: LearningPathNodeItem[] = [
      node({ id: 1, title: '根节点', status: 'active', solvedCount: 0, requiredCount: 2 }),
      node({
        id: 2,
        parentId: 1,
        title: '子节点',
        status: 'locked',
        solvedCount: 0,
        requiredCount: 2,
      }),
    ]
    vi.mocked(getLearningTree).mockResolvedValue({
      path: path({ isOfficial: false, isPublic: true, nodeCount: 2, myCompletedCount: 0 }),
      nodes,
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('公开')
    expect(wrapper.text()).toContain('进行中')
    expect(wrapper.text()).toContain('未解锁')
    expect(wrapper.text()).toContain('0 / 2')
    expect(wrapper.findAllComponents(ProgressBar).map((bar) => bar.props('value'))).toEqual([0, 0])
    wrapper.unmount()
  })

  it('shows not-found/forbidden error state for private path', async () => {
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
    wrapper.unmount()
  })

  it('reloads on route id change and ignores stale responses', async () => {
    vi.mocked(getLearningTree).mockResolvedValueOnce({
      path: path({ id: 3, title: '旧路线' }),
      nodes: [node({ id: 1, title: '旧节点' })],
    })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('旧路线')

    let resolveSecond!: (value: LearningTree) => void
    vi.mocked(getLearningTree).mockImplementationOnce(
      () => new Promise((resolve) => (resolveSecond = resolve)),
    )
    route.params.id = '4'
    await nextTick()

    expect(wrapper.text()).not.toContain('旧路线')
    expect(wrapper.text()).toContain('正在加载学习路线')

    resolveSecond({
      path: path({ id: 4, title: '新路线' }),
      nodes: [node({ id: 2, title: '新节点' })],
    })
    await flushPromises()
    expect(wrapper.text()).toContain('新路线')
    wrapper.unmount()
  })
})
