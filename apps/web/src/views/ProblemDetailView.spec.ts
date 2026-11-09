import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'

import { getAIHint } from '@/api/ai'
import { getContest } from '@/api/contests'
import { ApiError } from '@/api/http'
import { getProblem, getStatistics } from '@/api/problems'
import type { ContestDetail, ProblemDetail, ProblemStats } from '@/api/types'
import ProblemDetailView from './ProblemDetailView.vue'

const route = reactive({ params: { id: '1' }, query: {}, fullPath: '/problems/1' })

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const routerPush = vi.hoisted(() => vi.fn())
const auth = vi.hoisted(() => ({
  isLoggedIn: false as boolean,
  can: vi.fn<(code: string) => boolean>(() => false),
}))

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return { ...actual, useRoute: () => route, useRouter: () => ({ push: routerPush }) }
})
vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('@/components/MarkdownView.vue', () => ({
  default: { props: ['source', 'theme'], template: '<div>{{ source }}</div>' },
}))
vi.mock('@/api/ai', () => ({
  getAIHint: vi.fn(),
}))
vi.mock('@/api/problems', () => ({
  getProblem: vi.fn(),
  getStatistics: vi.fn(),
}))
vi.mock('@/api/contests', () => ({
  getContest: vi.fn(),
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

function problemItem(
  problemId: number,
  displayId: string,
  overrides: Partial<ContestDetail['problems'][number]> = {},
): ContestDetail['problems'][number] {
  return {
    problemId,
    title: `Contest Problem ${displayId}`,
    slug: `contest-problem-${displayId.toLowerCase()}`,
    type: 'traditional',
    displayId,
    sortOrder: displayId.charCodeAt(0) - 64,
    score: 100,
    timeLimitMs: 1000,
    memoryLimitMb: 256,
    isPublic: false,
    locked: true,
    submitCount: 0,
    acceptCount: 0,
    myStatus: '',
    myAttempts: 0,
    ...overrides,
  }
}

function contest(overrides: Partial<ContestDetail> = {}): ContestDetail {
  return {
    id: 5,
    title: 'Weekly Contest',
    descriptionMd: '',
    startTime: '2026-01-01T00:00:00Z',
    endTime: '2026-01-01T02:00:00Z',
    mode: 'icpc',
    isRated: true,
    isPublic: true,
    disableAi: false,
    registerType: 'open',
    status: 'running',
    problemCount: 2,
    participantCount: 10,
    registered: true,
    canManage: false,
    serverTime: '2026-01-01T01:00:00Z',
    createdAt: '2025-12-01T00:00:00Z',
    problems: [problemItem(1, 'A'), problemItem(2, 'B')],
    ...overrides,
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
  auth.isLoggedIn = false
  toast.add.mockReset()
  routerPush.mockReset()
  vi.mocked(getAIHint).mockReset()
  vi.mocked(getContest).mockReset()
  route.params.id = '1'
  route.query = {}
  route.fullPath = '/problems/1'
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
    vi.mocked(getContest).mockResolvedValue(contest({ status: 'ended' }))
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

  it('uses contest problem navigation and hides resources during a running contest', async () => {
    route.query = { contestId: '5' }
    vi.mocked(getProblem).mockResolvedValue(detail(1, { isPublic: false }))
    vi.mocked(getStatistics).mockResolvedValue(stats(10, 5))
    vi.mocked(getContest).mockResolvedValue(
      contest({
        status: 'running',
        disableAi: true,
        problems: [problemItem(1, 'A'), problemItem(2, 'B')],
      }),
    )
    const wrapper = mountView()
    await flushPromises()

    expect(getContest).toHaveBeenCalledWith(5)
    expect(wrapper.find('[data-test="contest-problem-list"]').exists()).toBe(true)
    const links = wrapper
      .find('[data-test="contest-problem-list"]')
      .findAllComponents(RouterLinkStub)
    expect(links).toHaveLength(2)
    expect(links[0].props('to')).toEqual({
      name: 'problem-detail',
      params: { id: 1 },
      query: { contestId: '5' },
    })
    expect(links[0].classes()).toContain('active')
    expect(links[1].classes()).not.toContain('active')
    expect(wrapper.text()).toContain('A')
    expect(wrapper.text()).not.toContain('官方题解')
    expect(wrapper.text()).not.toContain('解题思路')
    expect(wrapper.text()).not.toContain('提交统计')
    expect(wrapper.text()).not.toContain('关联比赛')
    expect(wrapper.find('[data-test="ai-hint-button"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('keeps AI hint for contest admins', async () => {
    auth.can.mockReturnValue(true)
    route.query = { contestId: '5' }
    vi.mocked(getProblem).mockResolvedValue(detail(1, { isPublic: false }))
    vi.mocked(getStatistics).mockResolvedValue(stats(10, 5))
    vi.mocked(getContest).mockResolvedValue(contest({ status: 'running', disableAi: true }))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-test="contest-problem-list"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="ai-hint-button"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('skips the statistics request during a running contest', async () => {
    route.query = { contestId: '5' }
    vi.mocked(getProblem).mockResolvedValue(detail(1, { isPublic: false }))
    vi.mocked(getContest).mockResolvedValue(contest({ status: 'running' }))
    const wrapper = mountView()
    await flushPromises()

    expect(getStatistics).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('keeps resource cards and statistics after the contest has ended', async () => {
    route.query = { contestId: '5' }
    vi.mocked(getProblem).mockResolvedValue(detail(1))
    vi.mocked(getStatistics).mockResolvedValue(stats(10, 5))
    vi.mocked(getContest).mockResolvedValue(contest({ status: 'ended' }))
    const wrapper = mountView()
    await flushPromises()

    expect(getStatistics).toHaveBeenCalledWith('1')
    expect(wrapper.find('[data-test="contest-problem-list"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('官方题解')
    expect(wrapper.text()).toContain('解题思路')
    expect(wrapper.text()).toContain('提交统计')
    expect(wrapper.text()).toContain('关联比赛')
    expect(wrapper.find('[data-test="ai-hint-button"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows the public author with a link to their profile (guest visible)', async () => {
    vi.mocked(getProblem).mockResolvedValue(
      detail(1, { author: { id: 7, username: 'alice', nickname: 'Alice' } }),
    )
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('创建者')
    const link = wrapper.find('a[href="/users/7"]')
    expect(link.exists()).toBe(true)
    expect(link.text()).toContain('Alice')
    wrapper.unmount()
  })

  it('falls back to the username when the author has no nickname', async () => {
    vi.mocked(getProblem).mockResolvedValue(
      detail(1, { author: { id: 7, username: 'alice', nickname: '' } }),
    )
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    const wrapper = mountView()
    await flushPromises()

    const link = wrapper.find('a[href="/users/7"]')
    expect(link.exists()).toBe(true)
    expect(link.text()).toBe('alice')
    wrapper.unmount()
  })

  it('does not show the author when the problem has none', async () => {
    vi.mocked(getProblem).mockResolvedValue(detail(1))
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).not.toContain('创建者')
    expect(wrapper.find('a[href^="/users/"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows AI hint button and sends guests to login', async () => {
    vi.mocked(getProblem).mockResolvedValue(detail(1))
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    const wrapper = mountView()
    await flushPromises()

    const button = wrapper
      .findAllComponents({ name: 'Button' })
      .find((b) => b.text().includes('AI 提示'))
    expect(button).toBeTruthy()

    await button!.trigger('click')
    await flushPromises()

    expect(routerPush).toHaveBeenCalledWith({
      name: 'login',
      query: { redirect: '/problems/1' },
    })
    expect(getAIHint).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('loads and renders AI hint with cached label', async () => {
    auth.isLoggedIn = true
    vi.mocked(getProblem).mockResolvedValue(detail(1))
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    vi.mocked(getAIHint).mockResolvedValue({ hint: 'AI hint text', cached: true })
    const wrapper = mountView()
    await flushPromises()

    const button = wrapper
      .findAllComponents({ name: 'Button' })
      .find((b) => b.text().includes('AI 提示'))
    await button!.trigger('click')
    await flushPromises()

    expect(getAIHint).toHaveBeenCalledWith(1)
    expect(wrapper.text()).toContain('AI hint text')
    expect(wrapper.text()).toContain('来自缓存')
    wrapper.unmount()
  })

  it('shows config link when AI config missing', async () => {
    auth.isLoggedIn = true
    vi.mocked(getProblem).mockResolvedValue(detail(1))
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    vi.mocked(getAIHint).mockRejectedValue(new ApiError(400, 40001, 'ai config not set'))
    const wrapper = mountView()
    await flushPromises()

    const button = wrapper
      .findAllComponents({ name: 'Button' })
      .find((b) => b.text().includes('AI 提示'))
    await button!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('尚未配置 AI 模型')
    expect(wrapper.find('a[href="/settings/ai"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows the admin-key-missing message without a config link on 40002', async () => {
    auth.isLoggedIn = true
    vi.mocked(getProblem).mockResolvedValue(detail(1))
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    vi.mocked(getAIHint).mockRejectedValue(new ApiError(400, 40002, 'AI 加密密钥未配置'))
    const wrapper = mountView()
    await flushPromises()

    const button = wrapper
      .findAllComponents({ name: 'Button' })
      .find((b) => b.text().includes('AI 提示'))
    await button!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('管理员尚未配置 AI 密钥')
    expect(wrapper.find('a[href="/settings/ai"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('maps 429/502/404 to Chinese toast', async () => {
    auth.isLoggedIn = true
    vi.mocked(getProblem).mockResolvedValue(detail(1))
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))

    const cases: Array<[ApiError, string]> = [
      [new ApiError(429, 42903, 'rate limited'), '操作过于频繁，请稍后再试'],
      [new ApiError(502, 50200, 'upstream'), 'AI 服务暂时不可用'],
      [new ApiError(503, 50300, 'redis'), 'AI 服务暂时不可用'],
      [new ApiError(404, 40400, 'not found'), '题目不存在'],
    ]

    for (const [err, expected] of cases) {
      toast.add.mockClear()
      vi.mocked(getAIHint).mockRejectedValue(err)
      const wrapper = mountView()
      await flushPromises()

      const button = wrapper
        .findAllComponents({ name: 'Button' })
        .find((b) => b.text().includes('AI 提示'))
      await button!.trigger('click')
      await flushPromises()

      expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ summary: expected }))
      wrapper.unmount()
    }
  })

  it('clears AI hint when the problem id changes', async () => {
    auth.isLoggedIn = true
    vi.mocked(getProblem).mockResolvedValue(detail(1))
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    vi.mocked(getAIHint).mockResolvedValue({ hint: 'AI hint text', cached: false })
    const wrapper = mountView()
    await flushPromises()

    const button = wrapper
      .findAllComponents({ name: 'Button' })
      .find((b) => b.text().includes('AI 提示'))
    await button!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('AI hint text')

    vi.mocked(getProblem).mockResolvedValue(detail(2))
    route.params.id = '2'
    route.fullPath = '/problems/2'
    await flushPromises()

    expect(wrapper.text()).not.toContain('AI hint text')
    wrapper.unmount()
  })

  it('shows objective label and no time/memory limits', async () => {
    vi.mocked(getProblem).mockResolvedValue(
      detail(1, { type: 'objective', answers: ['secret-a', 'secret-b'] }),
    )
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('客观题')
    expect(wrapper.text()).toContain('直接作答')
    expect(wrapper.text()).toContain('提交答案')
    expect(wrapper.text()).not.toContain('提交代码')
    expect(wrapper.text()).not.toContain('1000 ms')
    expect(wrapper.text()).not.toContain('256 MB')
    expect(wrapper.text()).not.toContain('secret-a')
    expect(wrapper.text()).not.toContain('answers')
    wrapper.unmount()
  })

  it('keeps code submission and limits for traditional problems', async () => {
    vi.mocked(getProblem).mockResolvedValue(detail(1))
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('传统题')
    expect(wrapper.text()).toContain('提交代码')
    expect(wrapper.text()).toContain('1000 ms')
    expect(wrapper.text()).toContain('256 MB')
    expect(wrapper.text()).not.toContain('直接作答')
    wrapper.unmount()
  })

  it('does not request AI hint automatically', async () => {
    auth.isLoggedIn = true
    vi.mocked(getProblem).mockResolvedValue(detail(1))
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    const wrapper = mountView()
    await flushPromises()

    expect(getAIHint).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
