import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'

import { ApiError } from '@/api/http'
import { getProblem, getStatistics } from '@/api/problems'
import type { ProblemDetail, ProblemStats } from '@/api/types'
import ProblemDetailView from './ProblemDetailView.vue'

const route = reactive({ params: { id: '1' } })

const auth = vi.hoisted(() => ({
  can: vi.fn<(code: string) => boolean>(() => false),
}))

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return { ...actual, useRoute: () => route }
})
vi.mock('@/components/MarkdownView.vue', () => ({
  default: { props: ['source', 'theme'], template: '<div>{{ source }}</div>' },
}))
vi.mock('@/api/problems', () => ({
  getProblem: vi.fn(),
  getStatistics: vi.fn(),
}))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => auth,
}))

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

function detail(id: number): ProblemDetail {
  return {
    id,
    title: `Problem ${id}`,
    slug: `problem-${id}`,
    statementMd: 'statement',
    inputFormatMd: '',
    outputFormatMd: '',
    samples: [],
    hintMd: '',
    type: 'traditional',
    tags: [],
    difficulty: 1,
    timeLimitMs: 1000,
    memoryLimitMb: 256,
    isPublic: true,
    source: '',
    submitCount: 0,
    acceptCount: 0,
    createdAt: '',
  } as ProblemDetail
}

function stats(submitCount: number, acceptCount: number): ProblemStats {
  return {
    submitCount,
    acceptCount,
    acceptRate: submitCount === 0 ? 0 : acceptCount / submitCount,
  }
}

function mountView() {
  return mount(ProblemDetailView, {
    global: {
      plugins: [PrimeVue, ToastService, createPinia()],
      stubs: { RouterLink: { template: '<a><slot /></a>' } },
    },
  })
}

async function openStatsTab(wrapper: ReturnType<typeof mountView>) {
  const tab = wrapper.findAll('[role="tab"]').find((el) => el.text() === '提交统计')
  expect(tab).toBeTruthy()
  await tab!.trigger('click')
  await flushPromises()
}

beforeEach(() => {
  auth.can.mockReset()
  auth.can.mockReturnValue(false)
})

describe('ProblemDetailView', () => {
  it('renders the statement, meta and samples', async () => {
    vi.mocked(getProblem).mockResolvedValue({
      ...detail(1),
      title: 'Two Sum',
      slug: 'two-sum',
      statementMd: 'statement text',
      inputFormatMd: 'input format',
      outputFormatMd: 'output format',
      samples: [{ input: '1 2', output: '3' }],
      hintMd: 'hint text',
      tags: ['math'],
      source: '测试来源',
      difficulty: 3,
    })
    vi.mocked(getStatistics).mockResolvedValue(stats(10, 5))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Two Sum')
    expect(wrapper.text()).toContain('提高')
    expect(wrapper.text()).toContain('传统题')
    expect(wrapper.text()).toContain('1000 ms')
    expect(wrapper.text()).toContain('256 MB')
    expect(wrapper.text()).toContain('来源：测试来源')
    expect(wrapper.text()).toContain('math')
    expect(wrapper.text()).toContain('statement text')
    expect(wrapper.text()).toContain('input format')
    expect(wrapper.text()).toContain('output format')
    expect(wrapper.text()).toContain('输入 1')
    expect(wrapper.text()).toContain('1 2')
    expect(wrapper.text()).toContain('hint text')
    expect(wrapper.text()).toContain('提交代码')
    wrapper.unmount()
  })

  it('ignores statistics that arrive after the route moved to another problem', async () => {
    const firstDetail = deferred<ProblemDetail>()
    const firstStats = deferred<ProblemStats>()
    const secondDetail = deferred<ProblemDetail>()
    const secondStats = deferred<ProblemStats>()
    vi.mocked(getProblem).mockImplementation((id) =>
      String(id) === '1' ? firstDetail.promise : secondDetail.promise,
    )
    vi.mocked(getStatistics).mockImplementation((id) =>
      String(id) === '1' ? firstStats.promise : secondStats.promise,
    )

    route.params.id = '1'
    const wrapper = mountView()

    firstDetail.resolve(detail(1))
    await flushPromises()
    expect(getStatistics).toHaveBeenCalledWith('1')

    route.params.id = '2'
    await flushPromises()
    secondDetail.resolve(detail(2))
    await flushPromises()
    expect(getStatistics).toHaveBeenCalledWith('2')

    secondStats.resolve(stats(10, 5))
    await flushPromises()
    await openStatsTab(wrapper)
    expect(wrapper.text()).toContain('50.0%')

    firstStats.resolve(stats(10, 0))
    await flushPromises()
    expect(wrapper.text()).toContain('50.0%')

    wrapper.unmount()
  })

  it('shows a not-found error and a return link', async () => {
    vi.mocked(getProblem).mockRejectedValue(new ApiError(404, 40400, 'not found'))
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('题目不存在')
    expect(wrapper.text()).toContain('重试')
    expect(wrapper.text()).toContain('返回题库')
    wrapper.unmount()
  })

  it('shows the edit button only with problem.update permission', async () => {
    vi.mocked(getProblem).mockResolvedValue(detail(1))
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))

    let wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).not.toContain('编辑')
    wrapper.unmount()

    auth.can.mockReturnValue(true)
    wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('编辑')
    wrapper.unmount()
  })
})
