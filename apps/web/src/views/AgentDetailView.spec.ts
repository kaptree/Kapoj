import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Paginator from 'primevue/paginator'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { getAgent, getAgentRatingHistory } from '@/api/agents'
import { ApiError } from '@/api/http'
import type { AgentRatingRow, AgentView, Page } from '@/api/types'
import { formatDateTime } from '@/utils/format'
import AgentDetailView from './AgentDetailView.vue'

vi.mock('@/api/agents', () => ({
  getAgent: vi.fn(),
  getAgentRatingHistory: vi.fn(),
}))

function agentView(overrides: Partial<AgentView> = {}): AgentView {
  return {
    id: 11,
    userId: 7,
    name: 'Arena Agent',
    modelName: 'deepseek-chat',
    modelIntro: '擅长图论与动态规划的外部 Agent',
    status: 'active',
    rating: 1532,
    passedCount: 4,
    attemptedCount: 7,
    avgSolveTimeMs: 125_000,
    author: { id: 7, username: 'alice', nickname: 'Alice' },
    createdAt: '2026-11-01T08:00:00Z',
    updatedAt: '2026-11-02T08:00:00Z',
    ...overrides,
  }
}

function ratingRow(overrides: Partial<AgentRatingRow> = {}): AgentRatingRow {
  return {
    id: 1,
    problemId: 3,
    problemTitle: '两数之和',
    problemSlug: 'two-sum',
    kind: 'solve',
    difficulty: 2,
    ratingBefore: 1500,
    ratingAfter: 1512,
    delta: 12,
    solveTimeMs: 65_000,
    createdAt: '2026-11-04T08:00:00Z',
    ...overrides,
  }
}

const routes = [
  { path: '/agents', name: 'agents', component: { template: '<div />' } },
  { path: '/agents/:id', name: 'agent-detail', component: { template: '<div />' } },
]

async function mountPage(id = '11') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(`/agents/${id}`)
  await router.isReady()
  const wrapper = mount(AgentDetailView, { global: { plugins: [router, PrimeVue] } })
  await flushPromises()
  return { wrapper, router }
}

function historyPage(items: AgentRatingRow[], total = items.length): Page<AgentRatingRow> {
  return { total, items }
}

beforeEach(() => {
  vi.mocked(getAgent).mockReset()
  vi.mocked(getAgentRatingHistory).mockReset()
  vi.mocked(getAgent).mockResolvedValue({ agent: agentView() })
  vi.mocked(getAgentRatingHistory).mockResolvedValue(historyPage([ratingRow()]))
})

describe('AgentDetailView', () => {
  it('shows the public profile and rating history without private data', async () => {
    const { wrapper } = await mountPage()

    expect(getAgent).toHaveBeenCalledWith('11')
    expect(getAgentRatingHistory).toHaveBeenCalledWith('11', { page: 1, pageSize: 10 })

    const text = wrapper.text()
    expect(text).toContain('Arena Agent')
    expect(text).toContain('deepseek-chat')
    expect(text).toContain('擅长图论与动态规划的外部 Agent')
    expect(text).toContain('Alice')
    expect(text).toContain('@alice')
    expect(text).toContain('正常')
    expect(text).toContain('1532')
    expect(text).toContain('4')
    expect(text).toContain('7')
    expect(text).toContain('02:05')

    expect(text).toContain('两数之和')
    expect(text).toContain('two-sum')
    expect(text).toContain('2')
    expect(text).toContain('+12')
    expect(text).toContain('1500 → 1512')
    expect(text).toContain('01:05')
    expect(text).toContain(formatDateTime('2026-11-04T08:00:00Z'))

    for (const forbidden of ['Token', '当前题目', '测试数据', '题解', '提交代码', '截止时间']) {
      expect(text).not.toContain(forbidden)
    }
  })

  it('renders reset entries without a problem title', async () => {
    vi.mocked(getAgentRatingHistory).mockResolvedValue(
      historyPage([
        ratingRow({
          id: 2,
          problemId: 0,
          problemTitle: '',
          problemSlug: '',
          kind: 'reset',
          difficulty: 0,
          ratingBefore: 1532,
          ratingAfter: 1500,
          delta: -32,
          solveTimeMs: 0,
        }),
      ]),
    )
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('Rating 重置')
    expect(wrapper.text()).toContain('重置')
    expect(wrapper.text()).toContain('-32')
    expect(wrapper.text()).toContain('1532 → 1500')
  })

  it('paginates the rating history', async () => {
    vi.mocked(getAgentRatingHistory).mockResolvedValue(historyPage([ratingRow()], 25))
    const { wrapper } = await mountPage()

    const pager = wrapper.findComponent(Paginator)
    expect(pager.exists()).toBe(true)
    pager.vm.$emit('page', { page: 1 })
    await flushPromises()

    expect(getAgentRatingHistory).toHaveBeenLastCalledWith('11', { page: 2, pageSize: 10 })
  })

  it('shows an empty history state while keeping the profile', async () => {
    vi.mocked(getAgentRatingHistory).mockResolvedValue(historyPage([], 0))
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('Arena Agent')
    expect(wrapper.text()).toContain('暂无 Rating 记录')
    expect(wrapper.findComponent(Paginator).exists()).toBe(false)
  })

  it('shows a Chinese not-found message for a missing agent', async () => {
    vi.mocked(getAgent).mockRejectedValue(new ApiError(404, 40400, '记录不存在'))
    vi.mocked(getAgentRatingHistory).mockRejectedValue(new ApiError(404, 40400, '记录不存在'))

    const { wrapper } = await mountPage('999')

    expect(wrapper.text()).toContain('Agent 不存在或已删除')
    expect(wrapper.text()).not.toContain('记录不存在')
    expect(wrapper.text()).not.toContain('Token')
  })

  it('hides internal error details and retries', async () => {
    vi.mocked(getAgent)
      .mockRejectedValueOnce(new ApiError(500, 50000, '数据库连接失败'))
      .mockResolvedValue({ agent: agentView() })

    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('加载 Agent 详情失败，请稍后再试')
    expect(wrapper.text()).not.toContain('数据库连接失败')

    await wrapper.find('.app-error button').trigger('click')
    await flushPromises()

    expect(getAgent).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('Arena Agent')
  })

  it('keeps the profile visible when rating history fails and retries it', async () => {
    vi.mocked(getAgentRatingHistory)
      .mockRejectedValueOnce(new ApiError(500, 50000, '历史服务不可用'))
      .mockResolvedValue(historyPage([ratingRow()]))

    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('Arena Agent')
    expect(wrapper.text()).toContain('加载 Rating 历史失败，请稍后再试')
    expect(wrapper.text()).not.toContain('历史服务不可用')

    await wrapper.find('[data-test="history-retry"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('两数之和')
  })

  it('clears stale rows when a later history request fails', async () => {
    vi.mocked(getAgentRatingHistory)
      .mockResolvedValueOnce(historyPage([ratingRow()], 25))
      .mockRejectedValueOnce(new ApiError(500, 50000, '历史服务不可用'))

    const { wrapper } = await mountPage()
    expect(wrapper.findAll('[data-test="agent-rating-history"] tbody tr')).toHaveLength(1)

    wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()

    expect(getAgentRatingHistory).toHaveBeenLastCalledWith('11', { page: 2, pageSize: 10 })
    expect(wrapper.text()).toContain('加载 Rating 历史失败，请稍后再试')
    expect(wrapper.find('[data-test="agent-rating-history"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('两数之和')
    expect(wrapper.text()).not.toContain('历史服务不可用')
  })

  it('shows the disabled status for a disabled agent', async () => {
    vi.mocked(getAgent).mockResolvedValue({ agent: agentView({ status: 'disabled' }) })
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('已停用')
    expect(wrapper.text()).not.toContain('正常')
  })
})
