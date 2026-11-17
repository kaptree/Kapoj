import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import InputChips from 'primevue/inputchips'
import InputNumber from 'primevue/inputnumber'
import SelectButton from 'primevue/selectbutton'
import ToggleSwitch from 'primevue/toggleswitch'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import { ApiError } from '@/api/http'
import {
  assessProblemDifficulty,
  createProblem,
  deleteProblem,
  getProblem,
  getProblemDifficultyHistory,
  updateProblem,
} from '@/api/problems'
import type { DifficultyHistoryItem, ProblemDetail } from '@/api/types'
import ProblemEditView from './ProblemEditView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/problems', () => ({
  getProblem: vi.fn(),
  createProblem: vi.fn(),
  updateProblem: vi.fn(),
  deleteProblem: vi.fn(),
  assessProblemDifficulty: vi.fn(),
  getProblemDifficultyHistory: vi.fn(),
}))
vi.mock('@/components/MarkdownEditor.vue', () => ({
  default: {
    name: 'MarkdownEditor',
    props: ['modelValue', 'height', 'theme'],
    template: '<div class="md-stub" :data-theme="theme">{{ modelValue }}</div>',
  },
}))
vi.mock('@/components/SampleEditor.vue', () => ({
  default: {
    name: 'SampleEditor',
    props: ['modelValue'],
    template: '<div class="sample-stub">samples {{ modelValue.length }}</div>',
  },
}))
vi.mock('@/components/TestcaseManager.vue', () => ({
  default: {
    name: 'TestcaseManager',
    props: ['problemId', 'type', 'checkerUploaded'],
    emits: ['changed'],
    template:
      '<div class="tc-stub" :data-problem-id="problemId" :data-type="type" :data-checker="String(checkerUploaded)" />',
  },
}))

function detail(overrides: Partial<ProblemDetail> = {}): ProblemDetail {
  return {
    id: 1,
    title: 'A+B',
    slug: 'a-plus-b',
    type: 'traditional',
    tags: ['math', 'beginner'],
    difficulty: 62,
    timeLimitMs: 2000,
    memoryLimitMb: 512,
    submitCount: 10,
    acceptCount: 5,
    isPublic: true,
    source: 'luogu',
    createdAt: '2026-11-01T00:00:00Z',
    statementMd: '# 题目',
    inputFormatMd: '输入',
    outputFormatMd: '输出',
    samples: [
      { input: '1 2', output: '3' },
      { input: '2 3', output: '5' },
    ],
    hintMd: '提示',
    solutionPriceCoins: 0,
    checkerUploaded: false,
    canEdit: true,
    ...overrides,
  }
}

function historyItem(overrides: Partial<DifficultyHistoryItem> = {}): DifficultyHistoryItem {
  return {
    oldScore: 50,
    newScore: 72,
    prior: 50,
    source: 'ai_reassess',
    sampleUsers: 10,
    acceptedUsers: 4,
    passRate: 0.4,
    smoothedPassRate: 0.35,
    targetScore: 68,
    confidence: 0.8,
    reason: '题目偏难',
    runId: null,
    createdBy: 1,
    createdAt: '2026-11-10T10:00:00Z',
    ...overrides,
  }
}

const routes = [
  { path: '/problems', name: 'problems', component: { template: '<div />' } },
  { path: '/problems/new', name: 'problem-create', component: { template: '<div />' } },
  { path: '/problems/:id(\\d+)', name: 'problem-detail', component: { template: '<div />' } },
  { path: '/problems/:id(\\d+)/edit', name: 'problem-edit', component: { template: '<div />' } },
]

async function mountView(path: string) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(ProblemEditView, { global: { plugins: [router, PrimeVue] } })
  await flushPromises()
  return { wrapper, router }
}

function button(wrapper: VueWrapper, label: string) {
  const found = wrapper.findAll('button').find((b) => b.text().includes(label))
  expect(found, `button "${label}"`).toBeTruthy()
  return found!
}

// The form renders difficulty first, then the time/memory limits and the
// public switch, so component order identifies each control reliably.
function difficultyInput(wrapper: VueWrapper) {
  return wrapper.findAllComponents(InputNumber)[0]!
}

function difficultyAuto(wrapper: VueWrapper) {
  return wrapper.findAllComponents(ToggleSwitch)[0]!
}

function publicToggle(wrapper: VueWrapper) {
  return wrapper.findAllComponents(ToggleSwitch)[1]!
}

/** Runs a reassessment that leaves the manual lock on and raises the warning. */
async function reassessLocked(wrapper: VueWrapper) {
  vi.mocked(assessProblemDifficulty).mockResolvedValue(
    detail({
      difficulty: 72,
      difficultyAuto: false,
      difficultySource: 'ai',
      difficultyReason: 'AI 重评',
    }),
  )
  await button(wrapper, 'AI 重新评估').trigger('click')
  await flushPromises()
  const options = confirm.require.mock.calls[0]![0] as { accept: () => Promise<void> }
  await options.accept()
  await flushPromises()
}

async function openTab(wrapper: VueWrapper, label: string) {
  const tab = wrapper.findAll('[role="tab"]').find((t) => t.text() === label)
  expect(tab, `tab "${label}"`).toBeTruthy()
  await tab!.trigger('click')
  await flushPromises()
}

beforeEach(() => {
  document.body.innerHTML = ''
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(getProblem).mockReset()
  vi.mocked(createProblem).mockReset()
  vi.mocked(updateProblem).mockReset()
  vi.mocked(deleteProblem).mockReset()
  vi.mocked(assessProblemDifficulty).mockReset()
  vi.mocked(getProblemDifficultyHistory).mockReset()
  vi.mocked(getProblem).mockResolvedValue(detail())
})

describe('ProblemEditView', () => {
  it('loads and fills the edit form', async () => {
    const { wrapper } = await mountView('/problems/1/edit')

    expect(getProblem).toHaveBeenCalledWith(1)
    expect(wrapper.text()).toContain('编辑题目')

    expect((wrapper.find('input[name="title"]').element as HTMLInputElement).value).toBe('A+B')
    expect((wrapper.find('input[name="slug"]').element as HTMLInputElement).value).toBe('a-plus-b')
    expect((wrapper.find('input[name="source"]').element as HTMLInputElement).value).toBe('luogu')

    expect(wrapper.findComponent(SelectButton).props('modelValue')).toBe('traditional')
    expect(wrapper.findComponent(InputChips).props('modelValue')).toEqual(['math', 'beginner'])
    expect(wrapper.findAllComponents(InputNumber).map((n) => n.props('modelValue'))).toEqual([
      62, 2000, 512,
    ])
    expect(difficultyAuto(wrapper).props('modelValue')).toBe(true)
    expect(publicToggle(wrapper).props('modelValue')).toBe(true)

    await openTab(wrapper, '样例')
    expect(wrapper.find('.sample-stub').text()).toBe('samples 2')

    const tc = wrapper.findComponent({ name: 'TestcaseManager' })
    expect(tc.exists()).toBe(true)
    expect(tc.props()).toEqual({ problemId: 1, type: 'traditional', checkerUploaded: false })
    wrapper.unmount()
  })

  it('shows 404 empty state for a missing problem', async () => {
    vi.mocked(getProblem).mockRejectedValue(new ApiError(404, 40400, '题目不存在'))
    const { wrapper } = await mountView('/problems/1/edit')

    expect(wrapper.text()).toContain('题目不存在')
    expect(wrapper.find('a[href="/problems"]').exists()).toBe(true)
    expect(wrapper.find('.tc-stub').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows a generic load error and retries', async () => {
    vi.mocked(getProblem).mockRejectedValueOnce(new ApiError(500, 50000, '服务器错误'))
    const { wrapper } = await mountView('/problems/1/edit')

    expect(wrapper.text()).toContain('加载题目失败，请稍后再试')
    expect(wrapper.find('a[href="/problems"]').exists()).toBe(true)

    vi.mocked(getProblem).mockResolvedValue(detail())
    await button(wrapper, '重试').trigger('click')
    await flushPromises()

    expect(getProblem).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).not.toContain('加载题目失败，请稍后再试')
    expect((wrapper.find('input[name="title"]').element as HTMLInputElement).value).toBe('A+B')
    wrapper.unmount()
  })

  it('saves edits via updateProblem and shows a toast', async () => {
    vi.mocked(updateProblem).mockResolvedValue(detail())
    const { wrapper } = await mountView('/problems/1/edit')

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateProblem).toHaveBeenCalledWith(
      1,
      expect.objectContaining({ title: 'A+B', slug: 'a-plus-b', difficulty: 62 }),
    )
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '已保存' }),
    )
    wrapper.unmount()
  })

  it('creates a new problem and navigates to edit', async () => {
    vi.mocked(createProblem).mockResolvedValue(detail({ id: 9 }))
    const { wrapper, router } = await mountView('/problems/new')

    expect(getProblem).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('新建题目')

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(createProblem).toHaveBeenCalledTimes(1)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'success',
        summary: '题目已创建。请上传测试点，再回到这里发布。',
      }),
    )
    expect(router.currentRoute.value.name).toBe('problem-edit')
    expect(router.currentRoute.value.params.id).toBe('9')
    wrapper.unmount()
  })

  it('deletes after confirm and navigates to problems', async () => {
    vi.mocked(deleteProblem).mockResolvedValue({ offlined: false })
    const { wrapper, router } = await mountView('/problems/1/edit')

    await button(wrapper, '删除题目').trigger('click')
    await flushPromises()

    expect(confirm.require).toHaveBeenCalledTimes(1)
    const options = confirm.require.mock.calls[0]![0] as {
      header: string
      message: string
      acceptLabel: string
      rejectLabel: string
      accept: () => void
    }
    expect(options.header).toBe('删除题目')
    expect(options.message).toBe('确定删除这道题目吗？已有提交记录的题目只会被下线。')
    expect(options.acceptLabel).toBe('删除')
    expect(options.rejectLabel).toBe('取消')

    options.accept()
    await flushPromises()

    expect(deleteProblem).toHaveBeenCalledWith(1)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '题目已删除' }),
    )
    expect(router.currentRoute.value.name).toBe('problems')
    wrapper.unmount()
  })

  it('shows the offlined message when delete falls back to offline', async () => {
    vi.mocked(deleteProblem).mockResolvedValue({ offlined: true })
    const { wrapper, router } = await mountView('/problems/1/edit')

    await button(wrapper, '删除题目').trigger('click')
    const options = confirm.require.mock.calls[0]![0] as { accept: () => void }
    options.accept()
    await flushPromises()

    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'success',
        summary: '该题已有提交记录，已下线而不是删除',
      }),
    )
    expect(router.currentRoute.value.name).toBe('problems')
    wrapper.unmount()
  })

  it('does not delete when confirm is cancelled', async () => {
    const { wrapper } = await mountView('/problems/1/edit')

    await button(wrapper, '删除题目').trigger('click')
    await flushPromises()

    expect(confirm.require).toHaveBeenCalledTimes(1)
    const options = confirm.require.mock.calls[0]![0] as { reject?: () => void }
    options.reject?.()
    await flushPromises()

    expect(deleteProblem).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('restores isPublic when save fails', async () => {
    vi.mocked(getProblem).mockResolvedValue(detail({ isPublic: false }))
    vi.mocked(updateProblem).mockRejectedValue(new ApiError(500, 50000, '保存失败'))
    const { wrapper } = await mountView('/problems/1/edit')

    const toggle = publicToggle(wrapper)
    expect(toggle.props('modelValue')).toBe(false)

    toggle.vm.$emit('update:modelValue', true)
    await flushPromises()
    expect(toggle.props('modelValue')).toBe(true)

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(toggle.props('modelValue')).toBe(false)
    expect(wrapper.text()).toContain('保存失败')
    wrapper.unmount()
  })

  it('passes the dark theme to MarkdownEditor', async () => {
    const { wrapper } = await mountView('/problems/1/edit')

    const md = wrapper.find('.md-stub')
    expect(md.exists()).toBe(true)
    expect(md.attributes('data-theme')).toBe('dark')
    wrapper.unmount()
  })

  it('caps tags at ten via InputChips', async () => {
    const { wrapper } = await mountView('/problems/1/edit')

    const chips = wrapper.findComponent(InputChips)
    expect(chips.props('max')).toBe(10)
    expect(chips.props('allowDuplicate')).toBe(false)
    expect(chips.props('addOnBlur')).toBe(true)
    wrapper.unmount()
  })

  it('normalizes tags on save (trim, drop empty, cap 10)', async () => {
    vi.mocked(getProblem).mockResolvedValue(
      detail({ tags: ['  math ', 'beginner', '', '   ', 'x', 'y', 'z', 'a', 'b', 'c', 'd'] }),
    )
    vi.mocked(updateProblem).mockResolvedValue(detail())
    const { wrapper } = await mountView('/problems/1/edit')

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateProblem).toHaveBeenCalledWith(
      1,
      expect.objectContaining({
        tags: ['math', 'beginner', 'x', 'y', 'z', 'a', 'b', 'c', 'd'],
      }),
    )
    wrapper.unmount()
  })

  it('supports objective answers and hides testcase manager', async () => {
    vi.mocked(getProblem).mockResolvedValue(detail({ type: 'objective', answers: ['A', 'B'] }))
    const { wrapper } = await mountView('/problems/1/edit')

    const answers = wrapper.find('[data-test="objective-answers"]')
    expect(answers.exists()).toBe(true)
    expect((answers.element as HTMLTextAreaElement).value).toBe('A\nB')
    expect(wrapper.find('.testcases').exists()).toBe(false)
    expect(wrapper.findAllComponents(InputNumber)).toHaveLength(1)
    expect(wrapper.text()).toContain('客观题无需测试点；发布前请确认至少 1 个正确答案。')
    wrapper.unmount()
  })

  it('saves objective answers trimmed as a list', async () => {
    vi.mocked(getProblem).mockResolvedValue(detail({ type: 'objective', answers: ['A', 'B'] }))
    vi.mocked(updateProblem).mockResolvedValue(detail({ type: 'objective' }))
    const { wrapper } = await mountView('/problems/1/edit')

    await wrapper.find('[data-test="objective-answers"]').setValue('  A  \n\n b \n')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateProblem).toHaveBeenCalledWith(
      1,
      expect.objectContaining({ type: 'objective', answers: ['A', 'b'] }),
    )
    wrapper.unmount()
  })

  it('sends an empty answers list for non-objective problems', async () => {
    vi.mocked(updateProblem).mockResolvedValue(detail())
    const { wrapper } = await mountView('/problems/1/edit')

    expect(wrapper.text()).toContain('发布需要至少 1 个测试点；特判题还需要 checker。')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateProblem).toHaveBeenCalledWith(1, expect.objectContaining({ answers: [] }))
    wrapper.unmount()
  })

  it('clears answers when switching away from objective', async () => {
    vi.mocked(getProblem).mockResolvedValue(detail({ type: 'objective', answers: ['A'] }))
    vi.mocked(updateProblem).mockResolvedValue(detail())
    const { wrapper } = await mountView('/problems/1/edit')

    wrapper.findComponent(SelectButton).vm.$emit('update:modelValue', 'traditional')
    await flushPromises()

    expect(wrapper.find('[data-test="objective-answers"]').exists()).toBe(false)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateProblem).toHaveBeenCalledWith(
      1,
      expect.objectContaining({ type: 'traditional', answers: [] }),
    )
    wrapper.unmount()
  })

  it('resets stale load flags on the create path', async () => {
    // Create path never calls getProblem and clears any leftover load flags.
    const { wrapper } = await mountView('/problems/new')

    expect(getProblem).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('加载题目失败')
    expect(wrapper.text()).not.toContain('题目不存在')
    expect(wrapper.find('.tc-stub').exists()).toBe(false)
    wrapper.unmount()
  })

  it('disables difficulty input while auto assessment is on', async () => {
    const { wrapper } = await mountView('/problems/new')

    expect(
      wrapper.find('[data-test="difficulty-input"] input').attributes('disabled'),
    ).toBeDefined()
    expect(difficultyAuto(wrapper).props('modelValue')).toBe(true)
    wrapper.unmount()
  })

  it('unlocks difficulty when auto adjustment is turned off and saves the lock', async () => {
    vi.mocked(updateProblem).mockResolvedValue(detail())
    const { wrapper } = await mountView('/problems/1/edit')

    const auto = difficultyAuto(wrapper)
    expect(
      wrapper.find('[data-test="difficulty-input"] input').attributes('disabled'),
    ).toBeDefined()

    auto.vm.$emit('update:modelValue', false)
    await flushPromises()

    expect(auto.props('modelValue')).toBe(false)
    expect(
      wrapper.find('[data-test="difficulty-input"] input').attributes('disabled'),
    ).toBeUndefined()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateProblem).toHaveBeenCalledWith(
      1,
      expect.objectContaining({ difficulty: 62, difficultyAuto: false }),
    )
    wrapper.unmount()
  })

  it('treats a manual score change as a manual lock', async () => {
    vi.mocked(updateProblem).mockResolvedValue(detail({ difficulty: 72 }))
    const { wrapper } = await mountView('/problems/1/edit')

    difficultyInput(wrapper).vm.$emit('update:modelValue', 72)
    await flushPromises()

    expect(difficultyInput(wrapper).props('modelValue')).toBe(72)
    expect(difficultyAuto(wrapper).props('modelValue')).toBe(false)

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateProblem).toHaveBeenCalledWith(
      1,
      expect.objectContaining({ difficulty: 72, difficultyAuto: false }),
    )
    wrapper.unmount()
  })

  it('keeps the current score when auto adjustment is turned back on', async () => {
    vi.mocked(updateProblem).mockResolvedValue(detail({ difficulty: 72 }))
    const { wrapper } = await mountView('/problems/1/edit')

    const auto = difficultyAuto(wrapper)
    auto.vm.$emit('update:modelValue', false)
    await flushPromises()
    difficultyInput(wrapper).vm.$emit('update:modelValue', 72)
    await flushPromises()
    auto.vm.$emit('update:modelValue', true)
    await flushPromises()

    expect(auto.props('modelValue')).toBe(true)
    expect(difficultyInput(wrapper).props('modelValue')).toBe(72)
    expect(
      wrapper.find('[data-test="difficulty-input"] input').attributes('disabled'),
    ).toBeDefined()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateProblem).toHaveBeenCalledWith(
      1,
      expect.objectContaining({ difficulty: 72, difficultyAuto: true }),
    )
    wrapper.unmount()
  })

  it('shows the difficulty source, last update and reason in edit mode', async () => {
    vi.mocked(getProblem).mockResolvedValue(
      detail({
        difficultyAuto: false,
        difficultySource: 'manual',
        difficultyUpdatedAt: '2026-11-10T10:00:00Z',
        difficultyReason: '人工设定',
      }),
    )
    const { wrapper } = await mountView('/problems/1/edit')

    expect(wrapper.find('[data-test="difficulty-source"]').text()).toBe('手动')
    expect(wrapper.text()).toContain('最近更新')
    expect(wrapper.text()).toContain('人工设定')
    wrapper.unmount()
  })

  it('hides reassessment and history controls on the create page', async () => {
    const { wrapper } = await mountView('/problems/new')

    expect(wrapper.findAll('button').some((b) => b.text().includes('AI 重新评估'))).toBe(false)
    expect(wrapper.findAll('button').some((b) => b.text().includes('查看难度历史'))).toBe(false)
    wrapper.unmount()
  })

  it('calls AI reassessment, updates the score and warns about the lock', async () => {
    vi.mocked(assessProblemDifficulty).mockResolvedValue(
      detail({
        difficulty: 72,
        difficultyAuto: false,
        difficultySource: 'ai',
        difficultyReason: 'AI 重评',
      }),
    )
    const { wrapper } = await mountView('/problems/1/edit')

    await button(wrapper, 'AI 重新评估').trigger('click')
    await flushPromises()

    expect(confirm.require).toHaveBeenCalledTimes(1)
    const options = confirm.require.mock.calls[0]![0] as {
      header: string
      message: string
      acceptLabel: string
      accept: () => Promise<void>
    }
    expect(options.header).toBe('AI 重新评估')
    expect(options.acceptLabel).toBe('重新评估')
    expect(assessProblemDifficulty).not.toHaveBeenCalled()

    await options.accept()
    await flushPromises()

    expect(assessProblemDifficulty).toHaveBeenCalledWith(1)
    expect(
      (wrapper.find('[data-test="difficulty-input"] input').element as HTMLInputElement).value,
    ).toBe('72')
    expect(difficultyAuto(wrapper).props('modelValue')).toBe(false)
    expect(wrapper.text()).toContain('AI 评分已更新，自动调整仍关闭')
    wrapper.unmount()
  })

  it('clears the locked reassessment warning when auto adjustment is turned back on', async () => {
    const { wrapper } = await mountView('/problems/1/edit')
    await reassessLocked(wrapper)
    expect(wrapper.text()).toContain('AI 评分已更新，自动调整仍关闭')

    difficultyAuto(wrapper).vm.$emit('update:modelValue', true)
    await flushPromises()

    expect(wrapper.text()).not.toContain('AI 评分已更新，自动调整仍关闭')
    wrapper.unmount()
  })

  it('clears the locked reassessment warning when another problem loads', async () => {
    const { wrapper, router } = await mountView('/problems/1/edit')
    await reassessLocked(wrapper)
    expect(wrapper.text()).toContain('AI 评分已更新，自动调整仍关闭')

    vi.mocked(getProblem).mockResolvedValue(detail({ id: 2, title: 'C+D' }))
    await router.push('/problems/2/edit')
    await flushPromises()

    expect(getProblem).toHaveBeenCalledWith(2)
    expect(wrapper.text()).not.toContain('AI 评分已更新，自动调整仍关闭')
    wrapper.unmount()
  })

  it('shows the difficulty history in a dialog', async () => {
    vi.mocked(getProblemDifficultyHistory).mockResolvedValue({
      total: 1,
      items: [historyItem()],
    })
    const { wrapper } = await mountView('/problems/1/edit')

    await button(wrapper, '查看难度历史').trigger('click')
    await flushPromises()

    expect(getProblemDifficultyHistory).toHaveBeenCalledWith(1, 1, 10)
    const dialog = document.body.querySelector('.p-dialog')
    expect(dialog).not.toBeNull()
    expect(dialog!.textContent).toContain('难度历史')
    expect(dialog!.textContent).toContain('50 → 72')
    expect(dialog!.textContent).toContain('题目偏难')
    wrapper.unmount()
  })
})
