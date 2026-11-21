import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import DataTable from 'primevue/datatable'
import Paginator from 'primevue/paginator'
import Select from 'primevue/select'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { listAgents } from '@/api/agents'
import { ApiError } from '@/api/http'
import type { AgentView, Page } from '@/api/types'
import AgentsView from './AgentsView.vue'

vi.mock('@/api/agents', () => ({
  listAgents: vi.fn(),
}))

function agentView(overrides: Partial<AgentView> = {}): AgentView {
  return {
    id: 11,
    userId: 7,
    name: 'Arena Agent',
    modelName: 'deepseek-chat',
    modelIntro: '用于测试的外部 Agent',
    status: 'active',
    rating: 1532,
    level: { key: 'cyan', name: '青名', color: '#03A89E', accentColor: '' },
    passedCount: 4,
    attemptedCount: 7,
    avgSolveTimeMs: 125_000,
    author: {
      id: 7,
      username: 'alice',
      nickname: 'Alice',
      level: { key: 'blue', name: '蓝名', color: '#0000FF', accentColor: '' },
    },
    createdAt: '2026-11-01T08:00:00Z',
    updatedAt: '2026-11-02T08:00:00Z',
    ...overrides,
  }
}

const arenaAgent = agentView()
const fastAgent = agentView({
  id: 12,
  userId: 8,
  name: 'Fast Agent',
  modelName: 'gpt-5.1',
  status: 'disabled',
  rating: 1400,
  level: { key: 'cyan', name: '青名', color: '#03A89E', accentColor: '' },
  passedCount: 2,
  attemptedCount: 5,
  avgSolveTimeMs: 65_000,
  author: {
    id: 8,
    username: 'bob',
    nickname: 'Bob',
    level: { key: 'green', name: '绿名', color: '#008000', accentColor: '' },
  },
})

const routes = [
  { path: '/agents', name: 'agents', component: { template: '<div />' } },
  { path: '/agents/:id', name: 'agent-detail', component: { template: '<div />' } },
]

async function mountPage() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/agents')
  await router.isReady()
  const wrapper = mount(AgentsView, { global: { plugins: [router, PrimeVue] } })
  await flushPromises()
  return { wrapper, router }
}

function pageOf(items: AgentView[], total = items.length): Page<AgentView> {
  return { total, items }
}

beforeEach(() => {
  vi.mocked(listAgents).mockReset()
  vi.mocked(listAgents).mockResolvedValue(pageOf([arenaAgent, fastAgent]))
})

describe('AgentsView', () => {
  it('lists agents and filters by model', async () => {
    const { wrapper } = await mountPage()

    expect(listAgents).toHaveBeenCalledWith({ page: 1, pageSize: 20, sort: 'rating' })

    const text = wrapper.text()
    expect(text).toContain('Arena Agent')
    expect(text).toContain('deepseek-chat')
    expect(text).toContain('Alice')
    expect(wrapper.find('.author-name.rating-level-name').attributes('title')).toBe('Alice（蓝名）')
    expect(wrapper.find('.agent-link .rating-level-name').attributes('title')).toBe(
      'Arena Agent（青名）',
    )
    expect(text).toContain('@alice')
    expect(text).toContain('1532')
    expect(text).toContain('4 / 7')
    expect(text).toContain('02:05')
    expect(text).toContain('正常')
    expect(text).toContain('Fast Agent')
    expect(text).toContain('已停用')

    vi.mocked(listAgents).mockResolvedValue(pageOf([fastAgent]))
    await wrapper.find('[data-test="model-filter"]').setValue('gpt-5.1')
    await wrapper.find('form[role="search"]').trigger('submit')
    await flushPromises()

    expect(listAgents).toHaveBeenLastCalledWith({
      page: 1,
      pageSize: 20,
      sort: 'rating',
      model: 'gpt-5.1',
    })
    expect(wrapper.text()).toContain('Fast Agent')
    expect(wrapper.text()).not.toContain('Arena Agent')
  })

  it('sorts by rating, passed count and average time', async () => {
    const { wrapper } = await mountPage()
    const sort = wrapper.findComponent(Select)

    sort.vm.$emit('update:modelValue', 'passed')
    await flushPromises()
    expect(listAgents).toHaveBeenLastCalledWith({ page: 1, pageSize: 20, sort: 'passed' })

    sort.vm.$emit('update:modelValue', 'avg_time')
    await flushPromises()
    expect(listAgents).toHaveBeenLastCalledWith({ page: 1, pageSize: 20, sort: 'avg_time' })

    sort.vm.$emit('update:modelValue', 'rating')
    await flushPromises()
    expect(listAgents).toHaveBeenLastCalledWith({ page: 1, pageSize: 20, sort: 'rating' })
  })

  it('paginates the leaderboard and returns to page one when sorting', async () => {
    vi.mocked(listAgents).mockResolvedValue(pageOf([arenaAgent], 45))
    const { wrapper } = await mountPage()

    wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()
    expect(listAgents).toHaveBeenLastCalledWith({ page: 2, pageSize: 20, sort: 'rating' })

    wrapper.findComponent(Select).vm.$emit('update:modelValue', 'passed')
    await flushPromises()
    expect(listAgents).toHaveBeenLastCalledWith({ page: 1, pageSize: 20, sort: 'passed' })
  })

  it('opens an agent detail by link and by row click', async () => {
    const { wrapper, router } = await mountPage()

    await wrapper.find('[data-test="agent-link"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('agent-detail')
    expect(router.currentRoute.value.params.id).toBe('11')

    await router.push('/agents')
    wrapper.findComponent(DataTable).vm.$emit('row-click', { data: fastAgent })
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('agent-detail')
    expect(router.currentRoute.value.params.id).toBe('12')
  })

  it('shows an empty state when no agents exist', async () => {
    vi.mocked(listAgents).mockResolvedValue(pageOf([], 0))
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('暂无 Agent')
    expect(wrapper.text()).toContain('参与排名')
    expect(wrapper.findComponent(Paginator).exists()).toBe(false)
  })

  it('shows a loading state while the leaderboard loads', async () => {
    let resolve!: (value: Page<AgentView>) => void
    vi.mocked(listAgents).mockReturnValue(
      new Promise<Page<AgentView>>((done) => {
        resolve = done
      }),
    )

    const { wrapper } = await mountPage()
    expect(wrapper.text()).toContain('正在加载 Agent 榜单')

    resolve(pageOf([arenaAgent]))
    await flushPromises()
    expect(wrapper.text()).toContain('Arena Agent')
    expect(wrapper.text()).not.toContain('正在加载 Agent 榜单')
  })

  it('shows a Chinese error without leaking internals and retries', async () => {
    vi.mocked(listAgents)
      .mockRejectedValueOnce(new ApiError(500, 50000, '数据库连接失败'))
      .mockResolvedValue(pageOf([arenaAgent]))

    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('加载 Agent 榜单失败，请稍后再试')
    expect(wrapper.text()).not.toContain('数据库连接失败')
    expect(wrapper.text()).not.toContain('Token')
    expect(wrapper.text()).not.toContain('当前题目')

    await wrapper.find('.app-error button').trigger('click')
    await flushPromises()

    expect(listAgents).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('Arena Agent')
  })
})
