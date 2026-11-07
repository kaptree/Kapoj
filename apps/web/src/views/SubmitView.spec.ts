import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Select from 'primevue/select'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import { ApiError } from '@/api/http'
import { getContest } from '@/api/contests'
import { getProblem } from '@/api/problems'
import { createSubmission } from '@/api/submissions'
import type { ContestDetail, ProblemDetail } from '@/api/types'
import SubmitView from './SubmitView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))

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
vi.mock('@/components/CodeEditor.vue', () => ({
  default: {
    name: 'CodeEditor',
    props: ['modelValue', 'language', 'height'],
    emits: ['update:modelValue'],
    template: '<div class="code-editor-stub">{{ modelValue }}</div>',
  },
}))

function detail(id: number): ProblemDetail {
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
  } as ProblemDetail
}

const routes = [
  { path: '/problems/:id/submit', name: 'problem-submit', component: { template: '<div />' } },
  { path: '/submissions', name: 'submissions', component: { template: '<div />' } },
  { path: '/problems', name: 'problems', component: { template: '<div />' } },
  { path: '/problems/:id', name: 'problem-detail', component: { template: '<div />' } },
  { path: '/contests/:id', name: 'contest-detail', component: { template: '<div />' } },
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

beforeEach(() => {
  localStorage.clear()
  toast.add.mockReset()
  vi.mocked(getProblem).mockReset()
  vi.mocked(getContest).mockReset()
  vi.mocked(createSubmission).mockReset()
  vi.mocked(getProblem).mockResolvedValue(detail(1))
  vi.mocked(getContest).mockResolvedValue({ title: 'Weekly' } as unknown as ContestDetail)
  vi.mocked(createSubmission).mockResolvedValue({ id: 42, status: 'pending' })
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
})
