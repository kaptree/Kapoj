import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import type { LearningPathNodeItem } from '@/api/types'
import { buildLearningTreeLayout } from '@/utils/learningTreeLayout'
import LearningTreeNode from './LearningTreeNode.vue'

function node(overrides: Partial<LearningPathNodeItem> = {}): LearningPathNodeItem {
  return {
    id: 1,
    parentId: null,
    title: '基础语法',
    descriptionMd: '',
    problemIds: [101, 102],
    problems: [],
    sortOrder: 0,
    requiredCount: 2,
    solvedCount: 1,
    status: 'active',
    children: [],
    ...overrides,
  }
}

function mountNode(item: LearningPathNodeItem, selected = false) {
  const layoutNode = buildLearningTreeLayout([item]).nodes[0]!
  return mount(LearningTreeNode, { props: { layoutNode, selected } })
}

describe('LearningTreeNode', () => {
  it.each([
    ['completed', 'status-completed', '已完成', '1 / 2'],
    ['active', 'status-active', '进行中', '1 / 2'],
    ['locked', 'status-locked', '未解锁', '未解锁'],
  ] as const)(
    'renders %s status and accessible progress',
    (status, className, label, statusText) => {
      const wrapper = mountNode(node({ status }))
      const button = wrapper.get('button.tree-node')

      expect(button.classes()).toContain(className)
      expect(button.attributes('aria-label')).toContain('基础语法')
      expect(button.attributes('aria-label')).toContain(label)
      expect(button.attributes('aria-label')).toContain('1 / 2')

      // Visible title below the circle and the native full-title tooltip.
      expect(button.get('.tree-node__label-title').text()).toBe('基础语法')
      expect(button.attributes('title')).toBe('基础语法')
      // Second line: solved / required for completed/active, 未解锁 for locked.
      expect(button.get('.tree-node__label-status').text()).toBe(statusText)
    },
  )

  it('renders the full title in the label and title attribute for a long title', () => {
    const longTitle = '这是一个非常长的节点标题用来验证单行省略与完整卷宗展示'
    const wrapper = mountNode(node({ title: longTitle }))
    const button = wrapper.get('button.tree-node')

    expect(button.get('.tree-node__label-title').text()).toBe(longTitle)
    expect(button.attributes('title')).toBe(longTitle)
    expect(button.attributes('aria-label')).toContain(longTitle)
  })

  it('emits select with the original node when clicked', async () => {
    const item = node()
    const wrapper = mountNode(item, true)

    await wrapper.get('button.tree-node').trigger('click')

    expect(wrapper.emitted('select')).toEqual([[item]])
    expect(wrapper.get('button.tree-node').classes()).toContain('selected')
  })

  it('shows a problem count for branches with problems but not for leaves', () => {
    const child = node({ id: 2, parentId: 1, title: '子节点', children: [] })
    const branch = node({
      problems: [
        { problemId: 101, title: '题目 101', slug: 'problem-101', difficulty: 1, solved: false },
        { problemId: 102, title: '题目 102', slug: 'problem-102', difficulty: 1, solved: false },
      ],
      children: [child],
    })
    const leafWrapper = mountNode(node())
    const branchWrapper = mountNode(branch)

    expect(leafWrapper.find('.tree-node__problem-count').exists()).toBe(false)
    expect(branchWrapper.get('.tree-node__problem-count').text()).toBe('2')
  })
})
