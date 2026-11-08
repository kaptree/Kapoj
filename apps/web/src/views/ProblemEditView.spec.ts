import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import InputChips from 'primevue/inputchips'
import InputNumber from 'primevue/inputnumber'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import ToggleSwitch from 'primevue/toggleswitch'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import { ApiError } from '@/api/http'
import { createProblem, deleteProblem, getProblem, updateProblem } from '@/api/problems'
import type { ProblemDetail } from '@/api/types'
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
    difficulty: 2,
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

async function openTab(wrapper: VueWrapper, label: string) {
  const tab = wrapper.findAll('[role="tab"]').find((t) => t.text() === label)
  expect(tab, `tab "${label}"`).toBeTruthy()
  await tab!.trigger('click')
  await flushPromises()
}

beforeEach(() => {
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(getProblem).mockReset()
  vi.mocked(createProblem).mockReset()
  vi.mocked(updateProblem).mockReset()
  vi.mocked(deleteProblem).mockReset()
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

    expect(wrapper.findComponent(Select).props('modelValue')).toBe(2)
    expect(wrapper.findComponent(SelectButton).props('modelValue')).toBe('traditional')
    expect(wrapper.findComponent(InputChips).props('modelValue')).toEqual(['math', 'beginner'])
    expect(wrapper.findAllComponents(InputNumber).map((n) => n.props('modelValue'))).toEqual([
      2000, 512,
    ])
    expect(wrapper.findComponent(ToggleSwitch).props('modelValue')).toBe(true)

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
      expect.objectContaining({ title: 'A+B', slug: 'a-plus-b', difficulty: 2 }),
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

    const toggle = wrapper.findComponent(ToggleSwitch)
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
})
