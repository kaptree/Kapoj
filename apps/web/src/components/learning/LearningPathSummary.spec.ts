import { mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { describe, expect, it } from 'vitest'

import type { LearningPathItem, LearningPathNodeItem, LearningProblemSummary } from '@/api/types'
import LearningPathSummary from './LearningPathSummary.vue'

function path(overrides: Partial<LearningPathItem> = {}): LearningPathItem {
  return {
    id: 3,
    title: '算法入门',
    descriptionMd: '',
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

function problem(problemId: number, solved: boolean): LearningProblemSummary {
  return {
    problemId,
    title: `题目 ${problemId}`,
    slug: `problem-${problemId}`,
    difficulty: 1,
    solved,
  }
}

function node(overrides: Partial<LearningPathNodeItem> = {}): LearningPathNodeItem {
  return {
    id: 1,
    parentId: null,
    title: '节点',
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

function mountSummary(
  props: {
    path?: LearningPathItem
    nodes?: LearningPathNodeItem[]
    showSolved?: boolean
    compact?: boolean
  } = {},
) {
  return mount(LearningPathSummary, {
    props: {
      path: path(),
      nodes: [],
      showSolved: true,
      ...props,
    },
    global: { plugins: [PrimeVue] },
  })
}

describe('LearningPathSummary', () => {
  it('counts a shared problem id only once across nodes', () => {
    const wrapper = mountSummary({
      nodes: [
        node({ id: 1, problems: [problem(101, true)] }),
        node({ id: 2, parentId: 1, problems: [problem(101, true), problem(102, false)] }),
      ],
    })

    expect(wrapper.get('.summary-problem-progress').text()).toBe('已完成 1 / 2 题')
  })

  it('shows only the total problem count for guests', () => {
    const wrapper = mountSummary({
      showSolved: false,
      nodes: [node({ problems: [problem(101, false), problem(102, false)] })],
    })

    expect(wrapper.get('.summary-problem-progress').text()).toBe('2 题')
    expect(wrapper.text()).not.toContain('0 / 2')
  })

  it('renders only progress in compact mode', () => {
    const wrapper = mountSummary({ compact: true })

    expect(wrapper.find('.summary-author').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('管理员')
    expect(wrapper.text()).not.toContain('个节点')
    expect(wrapper.find('.summary-progress-bar').exists()).toBe(true)
    expect(wrapper.get('.summary-progress-label').text()).toBe('已完成 1 / 3 节点')
  })

  it('renders author, node count, problem progress, and completion ring by default', () => {
    const wrapper = mountSummary()

    expect(wrapper.get('.summary-author').text()).toBe('作者 管理员')
    expect(wrapper.get('.summary-node-count').text()).toBe('3 个节点')
    expect(wrapper.get('.summary-problem-progress').text()).toBe('已完成 0 / 0 题')
    expect(wrapper.get('.summary-ring__value').text()).toBe('33%')
  })

  it('falls back to the username when the nickname is empty', () => {
    const wrapper = mountSummary({
      path: path({ author: { id: 1, username: 'admin', nickname: '' } }),
    })

    expect(wrapper.get('.summary-author').text()).toBe('作者 admin')
  })

  it('shows zero percent without crashing when there are no nodes', () => {
    const wrapper = mountSummary({
      path: path({ nodeCount: 0, myCompletedCount: 0 }),
      nodes: [],
    })

    expect(wrapper.get('.summary-ring__value').text()).toBe('0%')
    expect(wrapper.text()).toContain('已完成 0 / 0 节点')
  })
})
