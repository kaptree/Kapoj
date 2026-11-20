import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Paginator from 'primevue/paginator'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  createAgent,
  generateAgentToken,
  getMyAgent,
  getMyAgentRatingHistory,
  revokeAgentToken,
  updateMyAgent,
} from '@/api/agents'
import { ApiError } from '@/api/http'
import { getSubmission } from '@/api/submissions'
import type {
  AgentAttemptView,
  AgentRatingRow,
  AgentTokenView,
  AgentView,
  MyAgentView,
  SubmissionDetail,
} from '@/api/types'
import { formatDateTime } from '@/utils/format'
import AgentSettingsView from './AgentSettingsView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))
const clipboardWrite = vi.hoisted(() => vi.fn())

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/agents', () => ({
  getMyAgent: vi.fn(),
  createAgent: vi.fn(),
  updateMyAgent: vi.fn(),
  generateAgentToken: vi.fn(),
  revokeAgentToken: vi.fn(),
  getMyAgentRatingHistory: vi.fn(),
}))
vi.mock('@/api/submissions', () => ({
  getSubmission: vi.fn(),
  rejudgeSubmission: vi.fn(),
}))
vi.mock('@/components/MarkdownEditor.vue', () => ({
  default: {
    name: 'MarkdownEditor',
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template:
      '<textarea data-test="agent-intro-editor" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
  },
}))

function emptyView(): MyAgentView {
  return { agent: null, token: null, currentAttempt: null }
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

function tokenView(overrides: Partial<AgentTokenView> = {}): AgentTokenView {
  return {
    id: 5,
    tokenPrefix: 'kapoj_agent_abcd1234',
    createdAt: '2026-11-03T08:00:00Z',
    lastUsedAt: null,
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
    submissionId: null,
    createdAt: '2026-11-04T08:00:00Z',
    ...overrides,
  }
}

function attemptView(overrides: Partial<AgentAttemptView> = {}): AgentAttemptView {
  return {
    id: 9,
    problemId: 3,
    problemTitle: 'A + B',
    problemSlug: 'a-plus-b',
    difficulty: 1,
    status: 'active',
    attemptsUsed: 3,
    attemptsRemaining: 17,
    claimedAt: '2026-11-05T08:00:00Z',
    deadlineAt: '2026-11-05T08:30:00Z',
    solvedAt: null,
    solveTimeMs: 0,
    ...overrides,
  }
}

const routes = [
  { path: '/settings/agent', component: { template: '<div />' } },
  { path: '/users/:id', name: 'user-profile', component: { template: '<div />' } },
  { path: '/problems/:id', name: 'problem-detail', component: { template: '<div />' } },
]

async function mountPage() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/settings/agent')
  await router.isReady()
  const wrapper = mount(AgentSettingsView, {
    global: { plugins: [router, PrimeVue] },
  })
  await flushPromises()
  return wrapper
}

function submissionDetail(overrides: Partial<SubmissionDetail> = {}): SubmissionDetail {
  return {
    id: 42,
    userId: 7,
    username: 'alice',
    nickname: 'Alice',
    agentId: null,
    agentName: '',
    equippedBadges: [],
    equippedTitle: null,
    problemId: 3,
    problemTitle: '两数之和',
    problemSlug: 'two-sum',
    language: 'cpp',
    status: 'accepted',
    timeUsedMs: 100,
    cpuUsedMs: 80,
    memoryUsedKb: 1024,
    score: 100,
    codeLength: 13,
    createdAt: '2026-11-04T08:00:00Z',
    code: 'int main() {}',
    codeHidden: false,
    answerHidden: false,
    judgeResult: [],
    ...overrides,
  }
}

function setInput(wrapper: Awaited<ReturnType<typeof mountPage>>, selector: string, value: string) {
  const element = wrapper.find(selector).element
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(element, value)
  element.dispatchEvent(new Event('input', { bubbles: true }))
}

async function submitForm(wrapper: Awaited<ReturnType<typeof mountPage>>) {
  await wrapper.find('[data-test="agent-form"]').trigger('submit')
  await flushPromises()
}

beforeEach(() => {
  toast.add.mockReset()
  confirm.require.mockReset()
  clipboardWrite.mockReset()
  clipboardWrite.mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText: clipboardWrite },
  })
  vi.mocked(getMyAgent).mockReset()
  vi.mocked(createAgent).mockReset()
  vi.mocked(updateMyAgent).mockReset()
  vi.mocked(generateAgentToken).mockReset()
  vi.mocked(revokeAgentToken).mockReset()
  vi.mocked(getMyAgentRatingHistory).mockReset()
  vi.mocked(getSubmission).mockReset()
  vi.mocked(getMyAgent).mockResolvedValue(emptyView())
  vi.mocked(getMyAgentRatingHistory).mockResolvedValue({ total: 0, items: [] })
  vi.mocked(getSubmission).mockResolvedValue(submissionDetail())
  document.body.innerHTML = ''
})

describe('AgentSettingsView', () => {
  it('creates an agent when none exists', async () => {
    vi.mocked(getMyAgent)
      .mockResolvedValueOnce(emptyView())
      .mockResolvedValueOnce({
        agent: agentView({
          id: 21,
          name: '新 Agent',
          modelName: 'gpt-5.1',
          rating: 1500,
          passedCount: 0,
          attemptedCount: 0,
          avgSolveTimeMs: 0,
        }),
        token: null,
        currentAttempt: null,
      })
    vi.mocked(createAgent).mockResolvedValue(
      agentView({
        id: 21,
        name: '新 Agent',
        modelName: 'gpt-5.1',
        rating: 1500,
        passedCount: 0,
        attemptedCount: 0,
        avgSolveTimeMs: 0,
      }),
    )

    const wrapper = await mountPage()

    expect(getMyAgent).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-test="agent-form"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('创建 Agent')

    setInput(wrapper, '#agent-name', '  新 Agent  ')
    setInput(wrapper, '#agent-model-name', 'gpt-5.1')
    await wrapper.find('[data-test="agent-intro-editor"]').setValue('  外部 Agent  ')
    await submitForm(wrapper)

    expect(createAgent).toHaveBeenCalledWith({
      name: '新 Agent',
      modelName: 'gpt-5.1',
      modelIntro: '外部 Agent',
    })
    expect(getMyAgent).toHaveBeenCalledTimes(2)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: 'Agent 已创建' }),
    )
    expect(wrapper.find('[data-test="agent-rating"]').text()).toBe('1500')
    expect(wrapper.text()).toContain('新 Agent')

    wrapper.unmount()
  })

  it('shows stats and a paginated rating history', async () => {
    vi.mocked(getMyAgent).mockResolvedValue({
      agent: agentView(),
      token: tokenView(),
      currentAttempt: null,
    })
    vi.mocked(getMyAgentRatingHistory).mockResolvedValue({
      total: 15,
      items: [
        ratingRow(),
        ratingRow({
          id: 2,
          problemId: 0,
          problemTitle: '',
          problemSlug: '',
          kind: 'reset',
          difficulty: 0,
          ratingBefore: 1512,
          ratingAfter: 1500,
          delta: -12,
          solveTimeMs: 0,
          createdAt: '2026-11-03T08:00:00Z',
        }),
      ],
    })

    const wrapper = await mountPage()

    expect(getMyAgentRatingHistory).toHaveBeenCalledWith({ page: 1, pageSize: 10 })
    expect(wrapper.find('[data-test="agent-author"]').text()).toContain('Alice')
    expect(wrapper.find('[data-test="agent-author"]').text()).toContain('@alice')
    expect(wrapper.find('[data-test="agent-status"]').text()).toContain('正常')
    expect(wrapper.find('[data-test="agent-rating"]').text()).toBe('1532')
    expect(wrapper.find('[data-test="agent-passed"]').text()).toBe('4')
    expect(wrapper.find('[data-test="agent-attempted"]').text()).toBe('7')
    expect(wrapper.find('[data-test="agent-avg-time"]').text()).toBe('02:05')

    const rows = wrapper.findAll('[data-test="rating-history"] tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).toContain('两数之和')
    expect(rows[0]!.text()).toContain('通过')
    expect(rows[0]!.text()).toContain('+12')
    expect(rows[0]!.text()).toContain('1500 → 1512')
    expect(rows[0]!.text()).toContain('01:05')
    expect(rows[1]!.text()).toContain('Rating 重置')
    expect(rows[1]!.text()).toContain('-12')

    await wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()

    expect(getMyAgentRatingHistory).toHaveBeenLastCalledWith({ page: 2, pageSize: 10 })

    wrapper.unmount()
  })

  it('generates a token once and warns that it will not be shown again', async () => {
    const plaintext = 'kapoj_agent_live_secret_value'
    vi.mocked(getMyAgent).mockResolvedValue({
      agent: agentView(),
      token: null,
      currentAttempt: null,
    })
    vi.mocked(generateAgentToken).mockResolvedValue({
      token: plaintext,
      meta: tokenView({ id: 6, tokenPrefix: 'kapoj_agent_live_se' }),
    })

    const storageSpy = vi.spyOn(Storage.prototype, 'setItem')
    const wrapper = await mountPage()

    expect(wrapper.find('[data-test="generate-token"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('尚未生成 Token')

    await wrapper.find('[data-test="generate-token"]').trigger('click')
    await flushPromises()

    expect(generateAgentToken).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-test="agent-token-plaintext"]').text()).toBe(plaintext)
    expect(wrapper.find('[data-test="token-once-warning"]').text()).toContain('只显示一次')
    expect(wrapper.find('[data-test="token-prefix"]').text()).toContain('kapoj_agent_live_se…')

    await wrapper.find('[data-test="copy-token"]').trigger('click')
    await flushPromises()

    expect(clipboardWrite).toHaveBeenCalledWith(plaintext)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '已复制' }),
    )
    expect(storageSpy.mock.calls.flat().join(' ')).not.toContain('kapoj_agent_')

    // A page reload only receives the masked metadata, never the plaintext.
    vi.mocked(getMyAgent).mockResolvedValue({
      agent: agentView(),
      token: tokenView({ id: 6, tokenPrefix: 'kapoj_agent_live_se' }),
      currentAttempt: null,
    })
    wrapper.unmount()
    const reloaded = await mountPage()

    expect(reloaded.text()).not.toContain(plaintext)
    expect(reloaded.find('[data-test="agent-token-plaintext"]').exists()).toBe(false)
    expect(reloaded.find('[data-test="rotate-token"]').exists()).toBe(true)
    expect(reloaded.find('[data-test="token-once-warning"]').exists()).toBe(false)

    storageSpy.mockRestore()
    reloaded.unmount()
  })

  it('hides token actions and explains recovery when the agent is disabled', async () => {
    vi.mocked(getMyAgent).mockResolvedValue({
      agent: agentView({ status: 'disabled' }),
      token: null,
      currentAttempt: null,
    })

    const wrapper = await mountPage()

    expect(wrapper.find('[data-test="agent-status"]').text()).toContain('已停用')
    expect(wrapper.text()).toContain('Agent 已停用，请联系管理员；恢复后需重新生成 Token')
    expect(wrapper.find('[data-test="generate-token"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="rotate-token"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="revoke-token"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('clears stale rating rows when a later history request fails', async () => {
    vi.mocked(getMyAgent).mockResolvedValue({
      agent: agentView(),
      token: null,
      currentAttempt: null,
    })
    vi.mocked(getMyAgentRatingHistory)
      .mockResolvedValueOnce({ total: 25, items: [ratingRow()] })
      .mockRejectedValueOnce(new Error('network down'))

    const wrapper = await mountPage()
    expect(wrapper.findAll('[data-test="rating-history"] tbody tr')).toHaveLength(1)

    await wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()

    expect(getMyAgentRatingHistory).toHaveBeenLastCalledWith({ page: 2, pageSize: 10 })
    expect(wrapper.text()).toContain('加载 Rating 历史失败，请稍后再试')
    expect(wrapper.findAll('[data-test="rating-history"] tbody tr')).toHaveLength(0)
    expect(wrapper.text()).not.toContain('两数之和')
    expect(wrapper.text()).not.toContain('历史服务不可用')

    wrapper.unmount()
  })

  it('rotates and revokes tokens only after confirmation', async () => {
    vi.mocked(getMyAgent).mockResolvedValue({
      agent: agentView(),
      token: tokenView({ tokenPrefix: 'kapoj_agent_old1234', lastUsedAt: null }),
      currentAttempt: null,
    })
    vi.mocked(generateAgentToken).mockResolvedValue({
      token: 'kapoj_agent_new_secret',
      meta: tokenView({ id: 6, tokenPrefix: 'kapoj_agent_new1234' }),
    })
    vi.mocked(revokeAgentToken).mockResolvedValue(null)

    const wrapper = await mountPage()

    expect(wrapper.find('[data-test="token-prefix"]').text()).toContain('kapoj_agent_old1234')
    expect(wrapper.text()).toContain('从未使用')

    await wrapper.find('[data-test="rotate-token"]').trigger('click')
    await flushPromises()

    expect(confirm.require).toHaveBeenCalledTimes(1)
    expect(generateAgentToken).not.toHaveBeenCalled()
    const rotateOptions = confirm.require.mock.calls[0]![0] as {
      header: string
      message: string
      accept?: () => void
    }
    expect(rotateOptions.header).toContain('轮换')
    expect(rotateOptions.message).toContain('旧 Token')

    rotateOptions.accept?.()
    await flushPromises()

    expect(generateAgentToken).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-test="agent-token-plaintext"]').text()).toBe(
      'kapoj_agent_new_secret',
    )
    expect(wrapper.find('[data-test="token-prefix"]').text()).toContain('kapoj_agent_new1234')

    await wrapper.find('[data-test="revoke-token"]').trigger('click')
    await flushPromises()

    expect(confirm.require).toHaveBeenCalledTimes(2)
    expect(revokeAgentToken).not.toHaveBeenCalled()
    const revokeOptions = confirm.require.mock.calls[1]![0] as {
      header: string
      message: string
      accept?: () => void
    }
    expect(revokeOptions.header).toContain('吊销')
    expect(revokeOptions.message).toContain('立即失效')

    revokeOptions.accept?.()
    await flushPromises()

    expect(revokeAgentToken).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-test="generate-token"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="agent-token-plaintext"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('kapoj_agent_new_secret')

    wrapper.unmount()
  })

  it('edits agent metadata', async () => {
    vi.mocked(getMyAgent).mockResolvedValue({
      agent: agentView(),
      token: null,
      currentAttempt: null,
    })
    vi.mocked(updateMyAgent).mockResolvedValue(
      agentView({ name: '改名 Agent', modelName: 'claude-sonnet', modelIntro: '新的简介' }),
    )

    const wrapper = await mountPage()

    expect((wrapper.find('#agent-name').element as HTMLInputElement).value).toBe('Arena Agent')
    expect((wrapper.find('#agent-model-name').element as HTMLInputElement).value).toBe(
      'deepseek-chat',
    )
    expect(
      (wrapper.find('[data-test="agent-intro-editor"]').element as HTMLTextAreaElement).value,
    ).toBe('用于测试的外部 Agent')

    setInput(wrapper, '#agent-name', '改名 Agent')
    setInput(wrapper, '#agent-model-name', 'claude-sonnet')
    await wrapper.find('[data-test="agent-intro-editor"]').setValue('新的简介')
    await submitForm(wrapper)

    expect(updateMyAgent).toHaveBeenCalledWith({
      name: '改名 Agent',
      modelName: 'claude-sonnet',
      modelIntro: '新的简介',
    })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: 'Agent 资料已保存' }),
    )
    expect(wrapper.text()).toContain('改名 Agent')

    wrapper.unmount()
  })

  it('submits markdown intro source unchanged', async () => {
    vi.mocked(getMyAgent).mockResolvedValue({
      agent: agentView(),
      token: null,
      currentAttempt: null,
    })
    vi.mocked(updateMyAgent).mockResolvedValue(agentView({ modelIntro: '## 新简介' }))

    const wrapper = await mountPage()
    await wrapper.find('[data-test="agent-intro-editor"]').setValue('## 新简介\n\n- 列表')
    await submitForm(wrapper)

    expect(updateMyAgent).toHaveBeenCalledWith(
      expect.objectContaining({ modelIntro: '## 新简介\n\n- 列表' }),
    )
    wrapper.unmount()
  })

  it('shows validation and server errors in Chinese', async () => {
    const wrapper = await mountPage()

    await submitForm(wrapper)

    expect(createAgent).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('请填写 Agent 名称')
    expect(wrapper.text()).toContain('请填写模型名称')

    setInput(wrapper, '#agent-name', '重复的 Agent')
    setInput(wrapper, '#agent-model-name', 'gpt-5.1')
    await wrapper.find('[data-test="agent-intro-editor"]').setValue('x'.repeat(2001))
    await submitForm(wrapper)

    expect(createAgent).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('模型简介不能超过 2000 个字符')

    await wrapper.find('[data-test="agent-intro-editor"]').setValue('合法简介')
    vi.mocked(createAgent).mockRejectedValue(new ApiError(400, 40001, 'Agent 名称已存在'))
    await submitForm(wrapper)

    expect(wrapper.text()).toContain('Agent 名称已存在')
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'error',
        summary: '保存失败',
        detail: 'Agent 名称已存在',
      }),
    )

    wrapper.unmount()
  })

  it('shows a Chinese load error and retries', async () => {
    vi.mocked(getMyAgent)
      .mockRejectedValueOnce(new ApiError(500, 50000, '服务暂时不可用'))
      .mockResolvedValueOnce(emptyView())

    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('加载失败')
    expect(wrapper.text()).toContain('服务暂时不可用')

    const retry = wrapper.findAll('button').find((button) => button.text() === '重试')
    expect(retry).toBeDefined()
    await retry!.trigger('click')
    await flushPromises()

    expect(getMyAgent).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-test="agent-form"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('shows the current attempt as read-only information', async () => {
    vi.mocked(getMyAgent).mockResolvedValue({
      agent: agentView(),
      token: tokenView(),
      currentAttempt: attemptView(),
    })

    const wrapper = await mountPage()
    const card = wrapper.find('[data-test="current-attempt"]')

    expect(card.exists()).toBe(true)
    expect(card.find('[data-test="attempt-title"]').text()).toContain('A + B')
    expect(card.find('[data-test="attempt-slug"]').text()).toContain('a-plus-b')
    expect(card.find('[data-test="attempt-difficulty"]').text()).toContain('1')
    expect(card.find('[data-test="attempts-remaining"]').text()).toContain('17')
    expect(card.find('[data-test="attempt-deadline"]').text()).toBe(
      formatDateTime('2026-11-05T08:30:00Z'),
    )
    expect(card.findAll('button')).toHaveLength(0)

    wrapper.unmount()
  })

  it('documents API integration without exposing the token', async () => {
    vi.mocked(getMyAgent).mockResolvedValue({
      agent: agentView(),
      token: tokenView({ tokenPrefix: 'kapoj_agent_prefix_only' }),
      currentAttempt: null,
    })

    const wrapper = await mountPage()
    const instructions = wrapper.find('[data-test="agent-integration"]')

    expect(instructions.exists()).toBe(true)
    expect(instructions.text()).toContain('/api')
    expect(instructions.text()).toContain('Authorization: Bearer')
    expect(instructions.text()).toContain('POST /agent/problems/random')
    expect(instructions.text()).toContain('/agent/attempts/:attemptId/submissions')
    expect(instructions.text()).toContain('/agent/submissions/:submissionId')
    expect(instructions.text()).not.toContain('kapoj_agent_prefix_only')

    wrapper.unmount()
  })

  it('opens the linked submission from the rating history', async () => {
    vi.mocked(getMyAgent).mockResolvedValue({
      agent: agentView(),
      token: null,
      currentAttempt: null,
    })
    vi.mocked(getMyAgentRatingHistory).mockResolvedValue({
      total: 1,
      items: [ratingRow({ submissionId: 42 })],
    })

    const wrapper = await mountPage()
    const link = wrapper.find('[data-test="rating-submission-link"]')
    expect(link.exists()).toBe(true)
    await link.trigger('click')
    await flushPromises()

    expect(getSubmission).toHaveBeenCalledWith(42)
    expect(document.body.textContent).toContain('int main() {}')
    wrapper.unmount()
  })

  it('shows an empty rating history state', async () => {
    vi.mocked(getMyAgent).mockResolvedValue({
      agent: agentView(),
      token: tokenView(),
      currentAttempt: null,
    })

    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('暂无 Rating 记录')
    expect(wrapper.find('[data-test="rating-history"]').exists()).toBe(false)

    wrapper.unmount()
  })
})
