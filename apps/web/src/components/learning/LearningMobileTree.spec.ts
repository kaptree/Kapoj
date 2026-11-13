import { mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { describe, expect, it } from 'vitest'

import type { LearningPathNodeItem, LearningProblemSummary } from '@/api/types'
import LearningMobileTree from './LearningMobileTree.vue'

function problem(problemId: number): LearningProblemSummary {
  return {
    problemId,
    title: `题目 ${problemId}`,
    slug: `problem-${problemId}`,
    difficulty: 2,
    solved: false,
  }
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
    requiredCount: 2,
    solvedCount: 1,
    status: 'active',
    children: [],
    ...overrides,
  }
}

function mountTree(nodes: LearningPathNodeItem[], selectedId: number | null) {
  return mount(LearningMobileTree, {
    props: { nodes, selectedId },
    global: {
      plugins: [PrimeVue],
      stubs: {
        RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
      },
    },
  })
}

describe('LearningMobileTree', () => {
  it('renders nodes in pre-order with the correct depth', () => {
    const nodes = [
      node({
        id: 1,
        title: '根节点',
        children: [
          node({
            id: 2,
            parentId: 1,
            title: '子节点',
            children: [node({ id: 3, parentId: 2, title: '孙节点' })],
          }),
          node({ id: 4, parentId: 1, title: '另一个子节点' }),
        ],
      }),
    ]
    const wrapper = mountTree(nodes, null)

    expect(wrapper.findAll('.mobile-node').map((button) => button.text())).toEqual([
      expect.stringContaining('根节点'),
      expect.stringContaining('子节点'),
      expect.stringContaining('孙节点'),
      expect.stringContaining('另一个子节点'),
    ])
    expect(wrapper.findAll('.mobile-row').map((row) => row.attributes('data-depth'))).toEqual([
      '0',
      '1',
      '2',
      '1',
    ])
    expect(wrapper.find('.mobile-row[data-depth="2"]').attributes('style')).toContain(
      '--indent: 44px',
    )
  })

  it('shows status, progress counts, and emits the selected node on card click', async () => {
    const root = node({
      id: 1,
      title: '基础语法',
      children: [node({ id: 2, parentId: 1, title: '数组', status: 'locked' })],
    })
    const wrapper = mountTree([root], 1)

    expect(wrapper.get('.mobile-node[data-node-id="1"]').text()).toContain('进行中')
    expect(wrapper.get('.mobile-node[data-node-id="1"]').text()).toContain('1 / 2')
    expect(wrapper.get('.mobile-node[data-node-id="2"]').text()).toContain('未解锁')

    await wrapper.get('.mobile-node[data-node-id="2"]').trigger('click')
    expect(wrapper.emitted('select')?.[0]?.[0]).toEqual(root.children[0])
  })

  it('shows four selected leaf links and a +N indicator when a leaf has seven problems', () => {
    const problems = Array.from({ length: 7 }, (_, index) => problem(101 + index))
    const wrapper = mountTree([node({ id: 1, title: '数组', problems })], 1)

    expect(wrapper.findAll('.mobile-leaf')).toHaveLength(4)
    expect(wrapper.findAll('.mobile-leaf').map((link) => link.attributes('href'))).toEqual([
      '/problems/101',
      '/problems/102',
      '/problems/103',
      '/problems/104',
    ])
    expect(wrapper.get('.mobile-leaf-more').text()).toBe('+3')
  })

  it('renders the selected node dossier inline below its card', () => {
    const leaf = node({
      id: 2,
      parentId: 1,
      title: '链表',
      problems: [problem(202)],
      problemIds: [202],
    })
    const wrapper = mountTree([node({ id: 1, children: [leaf] })], 2)

    expect(wrapper.get('.mobile-node-panel .panel-title').text()).toBe('链表')
    expect(wrapper.get('.mobile-node-panel .panel-problem').text()).toContain('题目 202')
    expect(wrapper.get('.mobile-node-panel .panel-action').text()).toBe('继续挑战')
  })

  it('shows an internal node problem-count badge without expanding its problems as leaves', () => {
    const root = node({
      id: 1,
      title: '根节点',
      problems: [problem(101), problem(102)],
      children: [node({ id: 2, parentId: 1, title: '子节点' })],
    })
    const wrapper = mountTree([root], 1)

    expect(wrapper.get('.mobile-node__leaf-count').text()).toBe('2 题')
    expect(wrapper.find('.mobile-leaves').exists()).toBe(false)
  })

  it('exposes tree semantics and selected state on each row', () => {
    const wrapper = mountTree(
      [node({ id: 1, children: [node({ id: 2, parentId: 1, title: '子节点' })] })],
      2,
    )

    expect(wrapper.find('ol[role="tree"]').exists()).toBe(true)
    const rows = wrapper.findAll('li[role="treeitem"]')
    expect(rows.map((row) => row.attributes('aria-level'))).toEqual(['1', '2'])
    expect(rows.map((row) => row.attributes('aria-selected'))).toEqual(['false', 'true'])
  })
})
