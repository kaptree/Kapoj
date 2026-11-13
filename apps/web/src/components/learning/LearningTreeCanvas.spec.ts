import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it } from 'vitest'

import type { LearningPathNodeItem, LearningProblemSummary } from '@/api/types'
import LearningTreeCanvas from './LearningTreeCanvas.vue'

function problem(problemId: number, solved: boolean): LearningProblemSummary {
  return {
    problemId,
    title: `题目 ${problemId}`,
    slug: `problem-${problemId}`,
    difficulty: 2,
    solved,
  }
}

function node(overrides: Partial<LearningPathNodeItem> = {}): LearningPathNodeItem {
  return {
    id: 1,
    parentId: null,
    title: '根节点',
    descriptionMd: '',
    problemIds: [],
    problems: [],
    sortOrder: 0,
    requiredCount: 1,
    solvedCount: 0,
    status: 'active',
    children: [],
    ...overrides,
  }
}

function mountCanvas(nodes: LearningPathNodeItem[], selectedId: number | null = null) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/problems/:id', component: { template: '<div />' } },
    ],
  })
  const wrapper = mount(LearningTreeCanvas, {
    props: { nodes, pathId: 5, selectedId },
    global: { plugins: [router] },
  })

  return { wrapper, router }
}

describe('LearningTreeCanvas', () => {
  it('renders each node in a tree with multiple roots', () => {
    const firstRoot = node({ id: 1, title: '第一棵树' })
    const child = node({ id: 2, parentId: 1, title: '子节点' })
    firstRoot.children = [child]
    const secondRoot = node({ id: 3, title: '第二棵树', status: 'locked' })
    const { wrapper } = mountCanvas([firstRoot, secondRoot])

    expect(wrapper.findAll('.tree-node')).toHaveLength(3)
    expect(wrapper.find('[aria-label="学习路线树"]').exists()).toBe(true)
  })

  it('renders links for selected leaf problems and shows each solved state', () => {
    const item = node({
      id: 2,
      parentId: 1,
      title: '题目节点',
      status: 'locked',
      problems: [problem(101, true), problem(102, false)],
      problemIds: [101, 102],
    })
    const { wrapper } = mountCanvas([node({ children: [item] })], item.id)
    const leaves = wrapper.findAll('.tree-leaf')

    expect(leaves).toHaveLength(2)
    expect(leaves.map((leaf) => leaf.attributes('href'))).toEqual([
      '/problems/101',
      '/problems/102',
    ])
    expect(leaves[0]!.classes()).toContain('solved')
    expect(leaves[1]!.classes()).not.toContain('solved')
  })

  it('shows the remaining problem count and emits more from the +N button', async () => {
    const problems = Array.from({ length: 6 }, (_, index) => problem(201 + index, false))
    const item = node({
      id: 22,
      parentId: 1,
      title: '更多题目',
      problems,
      problemIds: problems.map(({ problemId }) => problemId),
    })
    const { wrapper } = mountCanvas([node({ children: [item] })], item.id)
    const moreButton = wrapper.get('button.tree-leaf--more')

    expect(moreButton.text()).toBe('+2')
    expect(moreButton.attributes('aria-label')).toBe('查看其余 2 道题目')
    expect(wrapper.findAll('a.tree-leaf')).toHaveLength(4)

    await moreButton.trigger('click')

    expect(wrapper.emitted('more')).toEqual([[item]])
  })

  it('emits select when a node is clicked', async () => {
    const item = node({ id: 8, title: '点击节点' })
    const { wrapper } = mountCanvas([item])

    await wrapper.find('.tree-node').trigger('click')

    expect(wrapper.emitted('select')).toEqual([[item]])
  })

  it('shows a branch problem badge without rendering its own problem leaves', () => {
    const child = node({ id: 2, parentId: 1, title: '子节点' })
    const root = node({
      problems: [problem(11, false), problem(12, false)],
      problemIds: [11, 12],
      children: [child],
    })
    const { wrapper } = mountCanvas([root], root.id)

    expect(wrapper.find('.tree-node__problem-count').text()).toBe('2')
    expect(wrapper.findAll('.tree-leaf')).toHaveLength(0)
  })
})
