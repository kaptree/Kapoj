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

const route = reactive({ params: { id: '1' }, query: {} })

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

function detail(id: number, overrides: Partial<ProblemDetail> = {}): ProblemDetail {
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
    solutionPriceCoins: 0,
    ...overrides,
  } as ProblemDetail
}

function stats(submitCount: number, acceptCount: number): ProblemStats {
  return {
    submitCount,
    acceptCount,
    acceptRate: submitCount === 0 ? 0 : acceptCount / submitCount,
  }
}

const RouterLinkStub = { props: ['to'], template: '<a :href="to"><slot /></a>' }

function mountView() {
  return mount(ProblemDetailView, {
    global: {
      plugins: [PrimeVue, ToastService, createPinia()],
      stubs: {
        RouterLink: RouterLinkStub,
      },
    },
  })
}

beforeEach(() => {
  auth.can.mockReset()
  auth.can.mockReturnValue(false)
  route.params.id = '1'
  route.query = {}
})

describe('ProblemDetailView', () => {
  it('renders the hint as a card below the statement and four resource links', async () => {
    vi.mocked(getProblem).mockResolvedValue(
      detail(1, { statementMd: 'statement text', hintMd: 'hint text' }),
    )
    vi.mocked(getStatistics).mockResolvedValue(stats(10, 5))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('statement text')
    expect(wrapper.text()).toContain('提示')
    expect(wrapper.text()).toContain('hint text')

    expect(wrapper.text()).toContain('官方题解')
    expect(wrapper.text()).toContain('解题思路')
    expect(wrapper.text()).toContain('提交统计')
    expect(wrapper.text()).toContain('关联比赛')

    expect(wrapper.find('a[href="/problems/1/official-solution"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/problems/1/solutions"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/problems/1/statistics"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/problems/1/contests"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows 暂无提示 when there is no hint', async () => {
    vi.mocked(getProblem).mockResolvedValue(detail(1))
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('暂无提示')
    wrapper.unmount()
  })

  it('shows 免费 for a free solution and 金币 N for a paid one', async () => {
    vi.mocked(getProblem).mockResolvedValue(detail(1, { solutionPriceCoins: 0 }))
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    let wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('免费')
    wrapper.unmount()

    vi.mocked(getProblem).mockResolvedValue(detail(1, { solutionPriceCoins: 100 }))
    wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('金币 100')
    wrapper.unmount()
  })

  it('hides official and discussion links for a hidden problem without view_hidden', async () => {
    vi.mocked(getProblem).mockResolvedValue(detail(1, { isPublic: false, solutionPriceCoins: 100 }))
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).not.toContain('官方题解')
    expect(wrapper.text()).not.toContain('解题思路')
    expect(wrapper.text()).toContain('提交统计')
    expect(wrapper.text()).toContain('关联比赛')
    expect(wrapper.find('a[href="/problems/1/official-solution"]').exists()).toBe(false)
    expect(wrapper.find('a[href="/problems/1/solutions"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('keeps official and discussion links for a hidden problem with view_hidden', async () => {
    auth.can.mockReturnValue(true)
    vi.mocked(getProblem).mockResolvedValue(detail(1, { isPublic: false, solutionPriceCoins: 100 }))
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('a[href="/problems/1/official-solution"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/problems/1/solutions"]').exists()).toBe(true)
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

  it('shows a contest back-link pointing to contest-detail when ?contestId is present', async () => {
    route.query = { contestId: '123' }
    vi.mocked(getProblem).mockResolvedValue(detail(1))
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('返回比赛 #123')
    const link = wrapper
      .findAllComponents(RouterLinkStub)
      .find((w) => w.text().includes('返回比赛'))
    expect(link).toBeTruthy()
    expect(link!.props('to')).toEqual({ name: 'contest-detail', params: { id: 123 } })
    wrapper.unmount()
  })

  it('does not show the contest back-link without ?contestId', async () => {
    vi.mocked(getProblem).mockResolvedValue(detail(1))
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).not.toContain('返回比赛')
    wrapper.unmount()
  })
})
