import { mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'

import type { LearningPathNodeItem, LearningProblemSummary } from '@/api/types'
import LearningNodePanel from './LearningNodePanel.vue'

vi.mock('@/components/MarkdownView.vue', () => ({
  default: {
    props: ['source', 'theme'],
    template: '<div class="markdown-stub">{{ source }}</div>',
  },
}))

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
    title: '基础语法',
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

async function mountPanel(
  item: LearningPathNodeItem | null,
  props: { index?: number | null; dismissible?: boolean; emptyLabel?: string } = {},
) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/problems/:id', component: { template: '<div />' } },
    ],
  })
  await router.push('/')
  await router.isReady()
  const wrapper = mount(LearningNodePanel, {
    props: { node: item, ...props },
    global: { plugins: [router, PrimeVue] },
  })
  return wrapper
}

describe('LearningNodePanel', () => {
  it('shows the default empty hint when no node is selected', async () => {
    const wrapper = await mountPanel(null)

    expect(wrapper.get('.panel-empty').text()).toBe('选择树上的节点查看详情。')
  })

  it('renders title, status, progress, description, and problem rows', async () => {
    const item = node({
      title: '动态规划入门',
      descriptionMd: '先理解状态转移。',
      solvedCount: 1,
      requiredCount: 2,
      problems: [problem(101, true), problem(102, false)],
      problemIds: [101, 102],
    })
    const wrapper = await mountPanel(item, { index: 3 })

    expect(wrapper.get('.panel-eyebrow').text()).toBe('NODE // 03')
    expect(wrapper.get('.panel-title').text()).toBe('动态规划入门')
    expect(wrapper.get('.panel-status').text()).toBe('进行中')
    expect(wrapper.get('.panel-progress-count').text()).toBe('1 / 2')
    expect(wrapper.get('.markdown-stub').text()).toBe('先理解状态转移。')

    const rows = wrapper.findAll('.panel-problem')
    expect(rows).toHaveLength(2)
    expect(rows.map((row) => row.attributes('href'))).toEqual(['/problems/101', '/problems/102'])
    expect(rows[0]!.classes()).toContain('solved')
    expect(rows[0]!.text()).toContain('已通过')
    expect(rows[1]!.classes()).not.toContain('solved')
    expect(rows[1]!.text()).toContain('未开始')
  })

  it('links the action to the first unsolved problem', async () => {
    const item = node({
      solvedCount: 2,
      requiredCount: 3,
      problems: [problem(101, true), problem(102, false), problem(103, true)],
      problemIds: [101, 102, 103],
    })
    const wrapper = await mountPanel(item)
    const action = wrapper.get('.panel-action')

    expect(action.text()).toBe('继续挑战')
    expect(action.attributes('href')).toBe('/problems/102')
  })

  it('offers a review action pointing at the first problem when all are solved', async () => {
    const item = node({
      status: 'completed',
      solvedCount: 2,
      requiredCount: 2,
      problems: [problem(101, true), problem(102, true)],
      problemIds: [101, 102],
    })
    const wrapper = await mountPanel(item)
    const action = wrapper.get('.panel-action')

    expect(action.text()).toBe('复习题目')
    expect(action.attributes('href')).toBe('/problems/101')
  })

  it('disables the action when the node has no problems', async () => {
    const wrapper = await mountPanel(node())
    const action = wrapper.get('.panel-action')

    expect(action.text()).toBe('暂无题目')
    expect(action.attributes('disabled')).toBeDefined()
  })

  it('keeps per-problem solved state visible for a locked node', async () => {
    const item = node({
      status: 'locked',
      solvedCount: 1,
      requiredCount: 2,
      problems: [problem(101, true), problem(102, false)],
      problemIds: [101, 102],
    })
    const wrapper = await mountPanel(item)

    expect(wrapper.get('.panel-status').text()).toBe('未解锁')
    expect(wrapper.get('.panel-progress-count').text()).toBe('1 / 2')
    expect(wrapper.text()).toContain('完成前置节点后推进进度；题目仍可提前挑战。')

    const rows = wrapper.findAll('.panel-problem')
    expect(rows[0]!.classes()).toContain('solved')
    expect(rows[1]!.classes()).not.toContain('solved')
  })

  it('emits close from the dismiss button only when dismissible', async () => {
    const dismissible = await mountPanel(node(), { dismissible: true })
    await dismissible.get('.panel-close').trigger('click')

    expect(dismissible.emitted('close')).toEqual([[]])

    const plain = await mountPanel(node())
    expect(plain.find('.panel-close').exists()).toBe(false)
  })

  it('uses a custom empty label', async () => {
    const wrapper = await mountPanel(null, { emptyLabel: '点选一个节点' })

    expect(wrapper.get('.panel-empty').text()).toBe('点选一个节点')
  })
})
