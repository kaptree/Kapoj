import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Paginator from 'primevue/paginator'
import Select from 'primevue/select'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { listAdminAgentSubmissions, listAdminAgents, updateAdminAgent } from '@/api/agents'
import { ApiError } from '@/api/http'
import type { AdminAgentSubmissionView, AgentView, Page } from '@/api/types'
import AdminAgentsView from './AdminAgentsView.vue'

const PAGE_SIZE = 20

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/agents', () => ({
  listAdminAgents: vi.fn(),
  listAdminAgentSubmissions: vi.fn(),
  updateAdminAgent: vi.fn(),
}))

const DialogStub = {
  name: 'Dialog',
  props: ['visible', 'header'],
  template: '<section v-if="visible"><h2>{{ header }}</h2><slot /></section>',
}

function agentView(overrides: Partial<AgentView> = {}): AgentView {
  return {
    id: 11,
    userId: 7,
    name: 'Arena Agent',
    modelName: 'deepseek-chat',
    modelIntro: '用于测试的外部 Agent',
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

const activeAgent = agentView()
const disabledAgent = agentView({
  id: 12,
  userId: 8,
  name: 'Fast Agent',
  modelName: 'gpt-5.1',
  status: 'disabled',
  rating: 1400,
  passedCount: 2,
  attemptedCount: 5,
  avgSolveTimeMs: 65_000,
  author: { id: 8, username: 'bob', nickname: 'Bob' },
})

function pageOf(items: AgentView[], total = items.length): Page<AgentView> {
  return { total, items }
}

function submissionView(
  overrides: Partial<AdminAgentSubmissionView> = {},
): AdminAgentSubmissionView {
  return {
    id: 101,
    problemId: 3,
    problemTitle: '两数之和',
    problemSlug: 'two-sum',
    language: 'cpp',
    status: 'accepted',
    timeUsedMs: 120,
    memoryUsedKb: 3456,
    score: 100,
    code: 'int main() { return 0; }',
    answer: '',
    errorMessage: '',
    createdAt: '2026-11-06T08:00:00Z',
    judgedAt: '2026-11-06T08:01:00Z',
    agentSettled: true,
    ...overrides,
  }
}

async function mountPage() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/admin/agents', component: { template: '<div />' } },
      { path: '/agents/:id', name: 'agent-detail', component: { template: '<div />' } },
    ],
  })
  await router.push('/admin/agents')
  await router.isReady()
  const wrapper = mount(AdminAgentsView, {
    global: { plugins: [router, PrimeVue], stubs: { Dialog: DialogStub } },
  })
  await flushPromises()
  return wrapper
}

type Wrapper = Awaited<ReturnType<typeof mountPage>>

function actionButton(wrapper: Wrapper, label: string) {
  const found = wrapper.findAll('button').find((item) => item.text().trim() === label)
  expect(found, `button "${label}"`).toBeTruthy()
  return found!
}

beforeEach(() => {
  toast.add.mockReset()
  confirm.require.mockReset()

  vi.mocked(listAdminAgents).mockReset()
  vi.mocked(listAdminAgentSubmissions).mockReset()
  vi.mocked(updateAdminAgent).mockReset()

  vi.mocked(listAdminAgents).mockResolvedValue(pageOf([activeAgent, disabledAgent]))
  vi.mocked(listAdminAgentSubmissions).mockResolvedValue({
    total: 1,
    items: [submissionView()],
  })
  vi.mocked(updateAdminAgent).mockResolvedValue(activeAgent)
})

describe('AdminAgentsView', () => {
  it('lists agents with author, model, status, rating and statistics', async () => {
    const wrapper = await mountPage()

    expect(listAdminAgents).toHaveBeenCalledWith({ page: 1, pageSize: PAGE_SIZE })

    const text = wrapper.text()
    expect(text).toContain('Agent 管理')
    expect(text).toContain('Arena Agent')
    expect(text).toContain('deepseek-chat')
    expect(text).toContain('Alice')
    expect(text).toContain('@alice')
    expect(text).toContain('1532')
    expect(text).toContain('4 / 7')
    expect(text).toContain('02:05')
    expect(text).toContain('正常')
    expect(text).toContain('Fast Agent')
    expect(text).toContain('gpt-5.1')
    expect(text).toContain('Bob')
    expect(text).toContain('@bob')
    expect(text).toContain('已停用')
    expect(text).toContain('共 2 个 Agent')

    wrapper.unmount()
  })

  it('filters by status and model and resets to page one', async () => {
    vi.mocked(listAdminAgents).mockResolvedValue(
      pageOf([activeAgent, disabledAgent], PAGE_SIZE * 2 + 5),
    )
    const wrapper = await mountPage()

    wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()
    expect(listAdminAgents).toHaveBeenLastCalledWith({ page: 2, pageSize: PAGE_SIZE })

    wrapper.findComponent(Select).vm.$emit('update:modelValue', 'disabled')
    await flushPromises()
    expect(listAdminAgents).toHaveBeenLastCalledWith({
      page: 1,
      pageSize: PAGE_SIZE,
      status: 'disabled',
    })

    vi.mocked(listAdminAgents).mockResolvedValue(pageOf([disabledAgent]))
    await wrapper.find('[data-test="model-filter"]').setValue('  gpt-5.1  ')
    await wrapper.find('form[role="search"]').trigger('submit')
    await flushPromises()

    expect(listAdminAgents).toHaveBeenLastCalledWith({
      page: 1,
      pageSize: PAGE_SIZE,
      status: 'disabled',
      model: 'gpt-5.1',
    })
    expect(wrapper.text()).toContain('Fast Agent')
    expect(wrapper.text()).not.toContain('Arena Agent')

    await wrapper.find('[data-test="reset-filters"]').trigger('click')
    await flushPromises()
    expect(listAdminAgents).toHaveBeenLastCalledWith({ page: 1, pageSize: PAGE_SIZE })

    wrapper.unmount()
  })

  it('disables an agent after confirmation and warns that tokens are revoked', async () => {
    const wrapper = await mountPage()

    await actionButton(wrapper, '停用').trigger('click')
    await flushPromises()

    expect(updateAdminAgent).not.toHaveBeenCalled()
    expect(confirm.require).toHaveBeenCalledTimes(1)

    const options = confirm.require.mock.calls[0]![0] as {
      message: string
      accept: () => void
    }
    expect(options.message).toContain('Arena Agent')
    expect(options.message).toContain('token')
    expect(options.message).toContain('立即')

    options.accept()
    await flushPromises()

    expect(updateAdminAgent).toHaveBeenCalledWith(activeAgent.id, { status: 'disabled' })
    expect(listAdminAgents).toHaveBeenCalledTimes(2)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: 'Agent 已停用' }),
    )

    wrapper.unmount()
  })

  it('restores an agent without restoring tokens', async () => {
    const wrapper = await mountPage()

    await actionButton(wrapper, '恢复').trigger('click')
    await flushPromises()

    expect(updateAdminAgent).toHaveBeenCalledWith(disabledAgent.id, { status: 'active' })
    expect(vi.mocked(updateAdminAgent).mock.calls[0]![1]).not.toHaveProperty('token')
    expect(listAdminAgents).toHaveBeenCalledTimes(2)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'success',
        summary: 'Agent 已恢复',
        detail: expect.stringContaining('旧 token 不会恢复'),
      }),
    )

    wrapper.unmount()
  })

  it('resets rating with confirmation without changing status', async () => {
    const wrapper = await mountPage()

    await actionButton(wrapper, '重置 Rating').trigger('click')
    await flushPromises()

    expect(updateAdminAgent).not.toHaveBeenCalled()
    expect(confirm.require).toHaveBeenCalledTimes(1)

    const options = confirm.require.mock.calls[0]![0] as {
      message: string
      accept: () => void
    }
    expect(options.message).toContain('1500')

    options.accept()
    await flushPromises()

    expect(updateAdminAgent).toHaveBeenCalledTimes(1)
    expect(updateAdminAgent).toHaveBeenCalledWith(activeAgent.id, { resetRating: true })
    expect(vi.mocked(updateAdminAgent).mock.calls[0]![1]).not.toHaveProperty('status')
    expect(listAdminAgents).toHaveBeenCalledTimes(2)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: 'Rating 已重置' }),
    )

    wrapper.unmount()
  })

  it('opens a paginated submission investigation dialog with code and answers', async () => {
    vi.mocked(listAdminAgentSubmissions).mockResolvedValue({
      total: 25,
      items: [
        submissionView(),
        submissionView({
          id: 102,
          language: 'answer',
          code: '',
          answer: '42',
          status: 'wrong_answer',
          agentSettled: false,
          judgedAt: null,
        }),
      ],
    })
    const wrapper = await mountPage()

    await actionButton(wrapper, '提交').trigger('click')
    await flushPromises()

    expect(listAdminAgentSubmissions).toHaveBeenCalledWith(activeAgent.id, {
      page: 1,
      pageSize: PAGE_SIZE,
    })
    const dialog = wrapper.find('[data-test="agent-submissions-dialog"]')
    expect(dialog.exists()).toBe(true)
    expect(dialog.text()).toContain('Arena Agent')
    expect(dialog.text()).toContain('两数之和')
    expect(dialog.text()).toContain('通过')

    const rows = dialog.findAll('[data-test="agent-submissions-table"] tbody tr')
    expect(rows).toHaveLength(2)

    await rows[0]!.trigger('click')
    await flushPromises()
    expect(dialog.find('[data-test="submission-code"]').text()).toContain(
      'int main() { return 0; }',
    )

    await rows[1]!.trigger('click')
    await flushPromises()
    expect(dialog.find('[data-test="submission-answer"]').text()).toContain('42')
    expect(dialog.find('[data-test="submission-code"]').exists()).toBe(false)

    await dialog.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()
    expect(listAdminAgentSubmissions).toHaveBeenLastCalledWith(activeAgent.id, {
      page: 2,
      pageSize: PAGE_SIZE,
    })

    wrapper.unmount()
  })

  it('shows Chinese loading, empty and error states', async () => {
    let resolve!: (value: Page<AgentView>) => void
    vi.mocked(listAdminAgents).mockReturnValue(
      new Promise<Page<AgentView>>((done) => {
        resolve = done
      }),
    )
    let wrapper = await mountPage()
    expect(wrapper.text()).toContain('正在加载 Agent 管理列表')
    resolve(pageOf([activeAgent]))
    await flushPromises()
    expect(wrapper.text()).toContain('Arena Agent')
    wrapper.unmount()

    vi.mocked(listAdminAgents).mockResolvedValue(pageOf([], 0))
    wrapper = await mountPage()
    expect(wrapper.text()).toContain('暂无 Agent')
    wrapper.unmount()

    vi.mocked(listAdminAgents)
      .mockRejectedValueOnce(new ApiError(500, 50000, '数据库连接失败'))
      .mockResolvedValue(pageOf([activeAgent]))
    wrapper = await mountPage()
    expect(wrapper.text()).toContain('加载 Agent 管理列表失败，请稍后再试')
    expect(wrapper.text()).not.toContain('数据库连接失败')

    const callsBeforeRetry = vi.mocked(listAdminAgents).mock.calls.length
    await wrapper.find('.app-error button').trigger('click')
    await flushPromises()
    expect(listAdminAgents).toHaveBeenCalledTimes(callsBeforeRetry + 1)
    expect(wrapper.text()).toContain('Arena Agent')
    wrapper.unmount()
  })
})
