import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Select from 'primevue/select'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import { completeCode, formatCode, getAIConfig, optimizeCode } from '@/api/ai'
import { getContest } from '@/api/contests'
import { ApiError } from '@/api/http'
import { getProblem } from '@/api/problems'
import { createSubmission } from '@/api/submissions'
import type { AIConfigView, ContestDetail, ProblemDetail } from '@/api/types'
import SubmitView from './SubmitView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const auth = vi.hoisted(() => ({ can: vi.fn<(code: string) => boolean>(() => false) }))
const editorMock = vi.hoisted(() => ({
  getCursorByteOffset: vi.fn(),
  insertAtCursor: vi.fn(),
}))

vi.mock('primevue/usetoast', () => ({
  useToast: () => toast,
}))
vi.mock('@/api/problems', () => ({
  getProblem: vi.fn(),
}))
vi.mock('@/api/contests', () => ({
  getContest: vi.fn(),
}))
vi.mock('@/api/submissions', () => ({
  createSubmission: vi.fn(),
}))
vi.mock('@/api/ai', () => ({
  getAIConfig: vi.fn(),
  completeCode: vi.fn(),
  formatCode: vi.fn(),
  optimizeCode: vi.fn(),
}))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => auth,
}))
vi.mock('@/components/MarkdownView.vue', () => ({
  default: {
    name: 'MarkdownView',
    props: ['source', 'theme'],
    template: '<div class="md-stub">{{ source }}</div>',
  },
}))
vi.mock('@/components/CodeEditor.vue', () => ({
  default: {
    name: 'CodeEditor',
    props: {
      modelValue: { type: String, default: '' },
      language: { type: String },
      height: { type: String },
      readonly: { type: Boolean },
    },
    emits: ['update:modelValue'],
    setup(
      props: { modelValue: string },
      {
        emit,
        expose,
      }: {
        emit: (event: 'update:modelValue', value: string) => void
        expose: (exposed: Record<string, unknown>) => void
      },
    ) {
      expose({
        getCursorByteOffset: () => editorMock.getCursorByteOffset(),
        insertAtCursor: (text: string) => {
          editorMock.insertAtCursor(text)
          emit('update:modelValue', props.modelValue + text)
        },
      })
    },
    template: '<div class="code-editor-stub">{{ modelValue }}</div>',
  },
}))

function detail(id: number, overrides: Partial<ProblemDetail> = {}): ProblemDetail {
  return {
    id,
    title: `Problem ${id}`,
    slug: `problem-${id}`,
    statementMd: '',
    inputFormatMd: '',
    outputFormatMd: '',
    samples: [],
    hintMd: '',
    type: 'traditional',
    tags: [],
    difficulty: 3,
    timeLimitMs: 1000,
    memoryLimitMb: 256,
    isPublic: true,
    source: '',
    submitCount: 0,
    acceptCount: 0,
    createdAt: '',
    solutionPriceCoins: 0,
    ...overrides,
  } as ProblemDetail
}

function aiConfig(overrides: Partial<AIConfigView> = {}): AIConfigView {
  return {
    configured: true,
    keyConfigured: true,
    provider: 'deepseek',
    modelName: 'deepseek-chat',
    apiKeyMasked: 'sk-***',
    providers: [{ code: 'deepseek', name: 'DeepSeek', defaultModel: 'deepseek-chat' }],
    ...overrides,
  }
}

function contestDetail(overrides: Partial<ContestDetail> = {}): ContestDetail {
  return {
    id: 5,
    title: 'Weekly',
    descriptionMd: '',
    startTime: '2026-01-01T00:00:00Z',
    endTime: '2026-01-01T02:00:00Z',
    mode: 'icpc',
    isRated: false,
    isPublic: true,
    disableAi: false,
    registerType: 'open',
    status: 'ended',
    problemCount: 0,
    participantCount: 0,
    registered: true,
    canManage: false,
    serverTime: '2026-01-01T03:00:00Z',
    createdAt: '2025-12-01T00:00:00Z',
    problems: [],
    ...overrides,
  }
}

const routes = [
  { path: '/problems/:id/submit', name: 'problem-submit', component: { template: '<div />' } },
  { path: '/submissions', name: 'submissions', component: { template: '<div />' } },
  { path: '/problems', name: 'problems', component: { template: '<div />' } },
  { path: '/problems/:id', name: 'problem-detail', component: { template: '<div />' } },
  { path: '/contests/:id', name: 'contest-detail', component: { template: '<div />' } },
  { path: '/settings/ai', name: 'ai-settings', component: { template: '<div />' } },
]

async function mountPage(path = '/problems/1/submit') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(SubmitView, { global: { plugins: [router, PrimeVue] } })
  await flushPromises()
  return { wrapper, router }
}

function editor(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper']) {
  return wrapper.findComponent({ name: 'CodeEditor' })
}

function submitButton(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper']) {
  return wrapper.findAll('button').find((button) => button.text().includes('提交'))
}

function aiButton(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper'], label: string) {
  return wrapper.findAll('button').find((button) => button.text().includes(label))
}

function dialogButton(label: string): HTMLButtonElement {
  const dialog = document.querySelector('.p-dialog')!
  return [...dialog.querySelectorAll('button')].find((b) =>
    b.textContent!.includes(label),
  ) as HTMLButtonElement
}

beforeEach(() => {
  localStorage.clear()
  document.body.innerHTML = ''
  toast.add.mockReset()
  auth.can.mockReset()
  auth.can.mockReturnValue(false)
  vi.mocked(getProblem).mockReset()
  vi.mocked(getContest).mockReset()
  vi.mocked(createSubmission).mockReset()
  vi.mocked(getAIConfig).mockReset()
  vi.mocked(completeCode).mockReset()
  vi.mocked(formatCode).mockReset()
  vi.mocked(optimizeCode).mockReset()
  editorMock.getCursorByteOffset.mockReset()
  editorMock.insertAtCursor.mockReset()
  vi.mocked(getProblem).mockResolvedValue(detail(1))
  vi.mocked(getContest).mockResolvedValue(contestDetail())
  vi.mocked(createSubmission).mockResolvedValue({ id: 42, status: 'pending' })
  vi.mocked(getAIConfig).mockResolvedValue(aiConfig())
})

describe('SubmitView', () => {
  it('loads language drafts and switches the editor language', async () => {
    localStorage.setItem('kapoj:code:practice:1:cpp', 'int main() {}')
    localStorage.setItem('kapoj:code:practice:1:python3', 'print(1)')
    const { wrapper } = await mountPage()

    expect(editor(wrapper).props('modelValue')).toBe('int main() {}')
    expect(editor(wrapper).props('language')).toBe('cpp')

    wrapper.findComponent(Select).vm.$emit('update:modelValue', 'python3')
    await nextTick()

    expect(editor(wrapper).props('language')).toBe('python')
    expect(editor(wrapper).props('modelValue')).toBe('print(1)')
    wrapper.unmount()
  })

  it('warns instead of submitting when the code is empty', async () => {
    localStorage.setItem('kapoj:code:practice:1:cpp', '   ')
    const { wrapper } = await mountPage()

    await submitButton(wrapper)!.trigger('click')
    await flushPromises()

    expect(createSubmission).not.toHaveBeenCalled()
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'warn', summary: '代码不能为空' }),
    )
    wrapper.unmount()
  })

  it('renders the objective answer form without code tools', async () => {
    vi.mocked(getProblem).mockResolvedValue(detail(1, { type: 'objective' }))
    const { wrapper } = await mountPage()

    expect(wrapper.find('[data-test="objective-answer"]').exists()).toBe(true)
    expect(wrapper.find('.code-editor-stub').exists()).toBe(false)
    expect(wrapper.findAllComponents(Select)).toHaveLength(0)
    expect(wrapper.find('[data-test="ai-complete-button"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('客观题')
    wrapper.unmount()
  })

  it('submits objective answers without code or language', async () => {
    vi.mocked(getProblem).mockResolvedValue(detail(1, { type: 'objective' }))
    const { wrapper, router } = await mountPage()

    await wrapper.find('[data-test="objective-answer"]').setValue('  A  ')
    await submitButton(wrapper)!.trigger('click')
    await flushPromises()

    expect(createSubmission).toHaveBeenCalledWith({ problemId: 1, answer: 'A' })
    expect(router.currentRoute.value.fullPath).toBe('/submissions?problemId=1')
    wrapper.unmount()
  })

  it('warns instead of submitting an empty objective answer', async () => {
    vi.mocked(getProblem).mockResolvedValue(detail(1, { type: 'objective' }))
    const { wrapper } = await mountPage()

    await wrapper.find('[data-test="objective-answer"]').setValue('   ')
    await submitButton(wrapper)!.trigger('click')
    await flushPromises()

    expect(createSubmission).not.toHaveBeenCalled()
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'warn', summary: '答案不能为空' }),
    )
    wrapper.unmount()
  })

  it('keeps the contest context when submitting an objective answer', async () => {
    vi.mocked(getProblem).mockResolvedValue(detail(1, { type: 'objective' }))
    const { wrapper, router } = await mountPage('/problems/1/submit?contestId=5')

    await wrapper.find('[data-test="objective-answer"]').setValue('B')
    await submitButton(wrapper)!.trigger('click')
    await flushPromises()

    expect(createSubmission).toHaveBeenCalledWith({ problemId: 1, contestId: 5, answer: 'B' })
    expect(router.currentRoute.value.fullPath).toBe('/submissions?problemId=1&contestId=5')
    wrapper.unmount()
  })

  it('does not show the contest AI banner for objective problems', async () => {
    vi.mocked(getProblem).mockResolvedValue(detail(1, { type: 'objective' }))
    vi.mocked(getContest).mockResolvedValue(contestDetail({ status: 'running', disableAi: true }))
    const { wrapper } = await mountPage('/problems/1/submit?contestId=5')

    expect(wrapper.find('[data-test="contest-ai-banner"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('does not load AI config for objective problems', async () => {
    vi.mocked(getProblem).mockResolvedValue(detail(1, { type: 'objective' }))
    await mountPage()

    expect(getAIConfig).not.toHaveBeenCalled()
  })

  it('preserves saved code drafts when unmounted before the problem loads', async () => {
    localStorage.setItem('kapoj:code:practice:1:cpp', 'saved draft')
    let resolveProblem!: (value: ProblemDetail) => void
    vi.mocked(getProblem).mockReturnValue(
      new Promise<ProblemDetail>((resolve) => {
        resolveProblem = resolve
      }),
    )
    const { wrapper } = await mountPage()

    wrapper.unmount()

    expect(localStorage.getItem('kapoj:code:practice:1:cpp')).toBe('saved draft')
    resolveProblem(detail(1))
    await flushPromises()
  })

  it('submits the current draft and navigates to submissions', async () => {
    localStorage.setItem('kapoj:code:practice:1:cpp', 'int main() { return 0; }')
    const { wrapper, router } = await mountPage()

    await submitButton(wrapper)!.trigger('click')
    await flushPromises()

    expect(createSubmission).toHaveBeenCalledWith({
      problemId: 1,
      contestId: undefined,
      language: 'cpp',
      code: 'int main() { return 0; }',
    })
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
    expect(router.currentRoute.value.fullPath).toBe('/submissions?problemId=1')
    wrapper.unmount()
  })

  it('keeps the contest context when submitting', async () => {
    localStorage.setItem('kapoj:code:contest:5:1:cpp', 'int main() {}')
    const { wrapper, router } = await mountPage('/problems/1/submit?contestId=5')

    expect(wrapper.text()).toContain('比赛提交')
    expect(wrapper.text()).toContain('Weekly')
    expect(getContest).toHaveBeenCalledWith(5)

    await submitButton(wrapper)!.trigger('click')
    await flushPromises()

    expect(createSubmission).toHaveBeenCalledWith({
      problemId: 1,
      contestId: 5,
      language: 'cpp',
      code: 'int main() {}',
    })
    expect(router.currentRoute.value.fullPath).toBe('/submissions?problemId=1&contestId=5')
    wrapper.unmount()
  })

  it('warns when the contest query is invalid and falls back to practice', async () => {
    const { wrapper } = await mountPage('/problems/1/submit?contestId=abc')

    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'warn', summary: '比赛参数无效' }),
    )
    expect(getContest).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('shows the not-found error and a return link', async () => {
    vi.mocked(getProblem).mockRejectedValue(new ApiError(404, 40400, 'not found'))
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('题目不存在')
    expect(wrapper.text()).toContain('返回题库')
    expect(wrapper.findComponent({ name: 'CodeEditor' }).exists()).toBe(false)
    wrapper.unmount()
  })

  it('flushes the current draft when unmounted', async () => {
    const { wrapper } = await mountPage()
    editor(wrapper).vm.$emit('update:modelValue', 'draft on unmount')
    await nextTick()
    expect(editor(wrapper).props('modelValue')).toBe('draft on unmount')

    wrapper.unmount()

    expect(localStorage.getItem('kapoj:code:practice:1:cpp')).toBe('draft on unmount')
  })

  it('shows config banner and disables AI buttons when not configured', async () => {
    vi.mocked(getAIConfig).mockResolvedValue(aiConfig({ configured: false }))
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('尚未配置 AI 模型')
    expect(wrapper.find('a[href="/settings/ai"]').exists()).toBe(true)
    expect((aiButton(wrapper, 'AI 补全')!.element as HTMLButtonElement).disabled).toBe(true)
    expect((aiButton(wrapper, 'AI 格式化')!.element as HTMLButtonElement).disabled).toBe(true)
    expect((aiButton(wrapper, 'AI 优化')!.element as HTMLButtonElement).disabled).toBe(true)
    wrapper.unmount()
  })

  it('disables AI when GET /ai/configs returns 404', async () => {
    vi.mocked(getAIConfig).mockRejectedValue(new ApiError(404, 40400, 'not found'))
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('AI 功能未启用')
    expect((aiButton(wrapper, 'AI 补全')!.element as HTMLButtonElement).disabled).toBe(true)
    expect((aiButton(wrapper, 'AI 格式化')!.element as HTMLButtonElement).disabled).toBe(true)
    expect((aiButton(wrapper, 'AI 优化')!.element as HTMLButtonElement).disabled).toBe(true)
    wrapper.unmount()
  })

  it('shows the key-missing banner (no config link) when keyConfigured is false', async () => {
    vi.mocked(getAIConfig).mockResolvedValue(aiConfig({ configured: false, keyConfigured: false }))
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('管理员尚未配置 AI 密钥')
    expect(wrapper.text()).not.toContain('尚未配置 AI 模型')
    expect(wrapper.find('a[href="/settings/ai"]').exists()).toBe(false)
    expect((aiButton(wrapper, 'AI 补全')!.element as HTMLButtonElement).disabled).toBe(true)
    expect((aiButton(wrapper, 'AI 格式化')!.element as HTMLButtonElement).disabled).toBe(true)
    expect((aiButton(wrapper, 'AI 优化')!.element as HTMLButtonElement).disabled).toBe(true)
    wrapper.unmount()
  })

  it('completes at cursor and inserts returned fragment', async () => {
    localStorage.setItem('kapoj:code:practice:1:cpp', 'int main() {}')
    editorMock.getCursorByteOffset.mockReturnValue(7)
    vi.mocked(completeCode).mockResolvedValue({ completion: ' return 0;' })
    const { wrapper } = await mountPage()

    await aiButton(wrapper, 'AI 补全')!.trigger('click')
    await flushPromises()

    expect(completeCode).toHaveBeenCalledWith({
      code: 'int main() {}',
      language: 'cpp',
      cursor: 7,
      problemId: 1,
    })
    expect(editorMock.insertAtCursor).toHaveBeenCalledWith(' return 0;')
    expect(editor(wrapper).props('modelValue')).toBe('int main() {} return 0;')
    wrapper.unmount()
  })

  it('previews and applies formatted code only after confirm', async () => {
    localStorage.setItem('kapoj:code:practice:1:cpp', 'int  main()  {}')
    vi.mocked(formatCode).mockResolvedValue({ code: 'int main() {}\n' })
    const { wrapper } = await mountPage()

    await aiButton(wrapper, 'AI 格式化')!.trigger('click')
    await flushPromises()

    expect(formatCode).toHaveBeenCalledWith({
      code: 'int  main()  {}',
      language: 'cpp',
      problemId: 1,
    })
    // Preview only — the draft must not change yet.
    expect(editor(wrapper).props('modelValue')).toBe('int  main()  {}')
    expect(document.querySelector('.p-dialog')!.textContent).toContain('int main() {}')

    dialogButton('应用格式化').click()
    await flushPromises()

    expect(editor(wrapper).props('modelValue')).toBe('int main() {}\n')
    wrapper.unmount()
  })

  it('previews and applies optimized code/suggestions', async () => {
    localStorage.setItem('kapoj:code:practice:1:cpp', 'int main() {}')
    vi.mocked(optimizeCode).mockResolvedValue({
      suggestions: '# 优化建议\n- 使用 const',
      code: 'int main() { return 0; }',
    })
    const { wrapper } = await mountPage()

    await aiButton(wrapper, 'AI 优化')!.trigger('click')
    await flushPromises()

    expect(optimizeCode).toHaveBeenCalledWith({
      code: 'int main() {}',
      language: 'cpp',
      problemId: 1,
    })
    expect(document.querySelector('.p-dialog')!.textContent).toContain('使用 const')
    expect(document.querySelector('.p-dialog')!.textContent).toContain('int main() { return 0; }')
    expect(editor(wrapper).props('modelValue')).toBe('int main() {}')

    dialogButton('应用优化代码').click()
    await flushPromises()

    expect(editor(wrapper).props('modelValue')).toBe('int main() { return 0; }')
    wrapper.unmount()
  })

  it('disables AI during a running contest that has disableAi set', async () => {
    vi.mocked(getContest).mockResolvedValue(contestDetail({ status: 'running', disableAi: true }))
    const { wrapper } = await mountPage('/problems/1/submit?contestId=5')

    expect(getContest).toHaveBeenCalledWith(5)
    expect(wrapper.find('[data-test="contest-ai-banner"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('比赛期间已禁用 AI，管理员可在比赛设置中调整。')
    expect(wrapper.find('[data-test="ai-complete-button"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-test="ai-format-button"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-test="ai-optimize-button"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('keeps AI enabled for contest admins when disableAi is set', async () => {
    auth.can.mockReturnValue(true)
    vi.mocked(getContest).mockResolvedValue(contestDetail({ status: 'running', disableAi: true }))
    const { wrapper } = await mountPage('/problems/1/submit?contestId=5')

    expect(wrapper.find('[data-test="contest-ai-banner"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="ai-format-button"]').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('keeps AI enabled after the contest has ended', async () => {
    vi.mocked(getContest).mockResolvedValue(contestDetail({ status: 'ended', disableAi: true }))
    const { wrapper } = await mountPage('/problems/1/submit?contestId=5')

    expect(wrapper.find('[data-test="contest-ai-banner"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="ai-format-button"]').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('sends problemId to AI endpoints', async () => {
    localStorage.setItem('kapoj:code:practice:1:cpp', 'int main() {}')
    vi.mocked(completeCode).mockResolvedValue({ completion: 'x' })
    vi.mocked(formatCode).mockResolvedValue({ code: 'int main() {}\n' })
    vi.mocked(optimizeCode).mockResolvedValue({
      suggestions: 'ok',
      code: 'int main() { return 0; }',
    })
    const { wrapper } = await mountPage()

    await aiButton(wrapper, 'AI 补全')!.trigger('click')
    await flushPromises()
    await aiButton(wrapper, 'AI 格式化')!.trigger('click')
    await flushPromises()
    await aiButton(wrapper, 'AI 优化')!.trigger('click')
    await flushPromises()

    expect(completeCode).toHaveBeenCalledWith(expect.objectContaining({ problemId: 1 }))
    expect(formatCode).toHaveBeenCalledWith(expect.objectContaining({ problemId: 1 }))
    expect(optimizeCode).toHaveBeenCalledWith(expect.objectContaining({ problemId: 1 }))
    wrapper.unmount()
  })

  it('maps 40302 to the contest AI disabled toast', async () => {
    localStorage.setItem('kapoj:code:practice:1:cpp', 'int main() {}')
    vi.mocked(completeCode).mockRejectedValueOnce(new ApiError(403, 40302, '比赛期间已禁用 AI'))
    const { wrapper } = await mountPage()

    await aiButton(wrapper, 'AI 补全')!.trigger('click')
    await flushPromises()

    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'warn', summary: '比赛期间已禁用 AI' }),
    )
    wrapper.unmount()
  })

  it('blocks >64KiB with Chinese toast', async () => {
    localStorage.setItem('kapoj:code:practice:1:cpp', 'a'.repeat(65537))
    const { wrapper } = await mountPage()

    await aiButton(wrapper, 'AI 补全')!.trigger('click')
    await flushPromises()

    expect(completeCode).not.toHaveBeenCalled()
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ summary: '代码超过 64 KiB，无法使用 AI' }),
    )
    wrapper.unmount()
  })

  it('maps 42903/50200/40001 to Chinese messages and config link', async () => {
    localStorage.setItem('kapoj:code:practice:1:cpp', 'int main() {}')
    const { wrapper } = await mountPage()

    vi.mocked(completeCode).mockRejectedValueOnce(new ApiError(429, 42903, 'rate limited'))
    await aiButton(wrapper, 'AI 补全')!.trigger('click')
    await flushPromises()
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ summary: '操作过于频繁，请稍后再试' }),
    )

    vi.mocked(completeCode).mockRejectedValueOnce(new ApiError(502, 50200, 'upstream'))
    await aiButton(wrapper, 'AI 补全')!.trigger('click')
    await flushPromises()
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ summary: 'AI 服务暂时不可用' }),
    )

    vi.mocked(completeCode).mockRejectedValueOnce(new ApiError(400, 40001, 'config not set'))
    await aiButton(wrapper, 'AI 补全')!.trigger('click')
    await flushPromises()
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ summary: '尚未配置 AI 模型' }))
    expect(wrapper.find('a[href="/settings/ai"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('maps the platform-key-missing 40002 to the key banner without a config link', async () => {
    localStorage.setItem('kapoj:code:practice:1:cpp', 'int main() {}')
    const { wrapper } = await mountPage()

    vi.mocked(completeCode).mockRejectedValueOnce(new ApiError(400, 40002, 'AI 加密密钥未配置'))
    await aiButton(wrapper, 'AI 补全')!.trigger('click')
    await flushPromises()

    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ summary: '管理员尚未配置 AI 密钥' }),
    )
    expect(wrapper.text()).toContain('管理员尚未配置 AI 密钥')
    expect(wrapper.find('a[href="/settings/ai"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
