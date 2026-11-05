import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import InputText from 'primevue/inputtext'
import Paginator from 'primevue/paginator'
import Select from 'primevue/select'
import ToggleSwitch from 'primevue/toggleswitch'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import { ApiError } from '@/api/http'
import type { ProblemListItem } from '@/api/types'
import ProblemListView from './ProblemListView.vue'

const problemsApi = vi.hoisted(() => ({
  listProblems: vi.fn(),
}))

const auth = vi.hoisted(() => ({
  user: { role: 'user' } as { role: string } | null,
}))

vi.mock('@/api/problems', () => ({
  listProblems: problemsApi.listProblems,
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => auth,
}))

function problem(overrides: Partial<ProblemListItem> = {}): ProblemListItem {
  return {
    id: 1,
    title: '两数之和',
    slug: 'two-sum',
    type: 'traditional',
    tags: ['math'],
    difficulty: 3,
    timeLimitMs: 1000,
    memoryLimitMb: 128,
    submitCount: 20,
    acceptCount: 10,
    isPublic: true,
    source: '',
    createdAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const routes = [
  { path: '/problems', name: 'problems', component: { template: '<div />' } },
  { path: '/problems/new', name: 'problem-create', component: { template: '<div />' } },
  { path: '/problems/:id(\\d+)', name: 'problem-detail', component: { template: '<div />' } },
]

async function mountPage(path = '/problems') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(ProblemListView, { global: { plugins: [router, PrimeVue] } })
  await flushPromises()
  return { wrapper, router }
}

beforeEach(() => {
  auth.user = { role: 'user' }
  problemsApi.listProblems.mockReset()
})

describe('ProblemListView', () => {
  it('loads from the route query and renders a problem row', async () => {
    problemsApi.listProblems.mockResolvedValue({ total: 1, items: [problem()] })
    const { wrapper } = await mountPage('/problems?keyword=sum&tag=math&difficulty=3&page=2')

    expect(problemsApi.listProblems).toHaveBeenCalledWith({
      keyword: 'sum',
      tag: 'math',
      difficulty: 3,
      page: 2,
      includeHidden: false,
      pageSize: 20,
    })
    expect(wrapper.text()).toContain('两数之和')
    expect(wrapper.text()).toContain('提高')
    expect(wrapper.text()).toContain('10 / 20')
    expect(wrapper.text()).toContain('50%')
    expect(wrapper.find('a[href="/problems/1"]').exists()).toBe(true)
  })

  it('trims filters and resets the page when searching', async () => {
    problemsApi.listProblems.mockResolvedValue({ total: 0, items: [] })
    const { wrapper, router } = await mountPage()
    const inputs = wrapper.findAllComponents(InputText)

    await inputs[0]!.setValue('  sum  ')
    await inputs[1]!.setValue(' math ')
    wrapper.findComponent(Select).vm.$emit('update:modelValue', 3)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({
      keyword: 'sum',
      tag: 'math',
      difficulty: '3',
    })
    expect(problemsApi.listProblems).toHaveBeenLastCalledWith({
      keyword: 'sum',
      tag: 'math',
      difficulty: 3,
      page: 1,
      includeHidden: false,
      pageSize: 20,
    })
  })

  it('filters by a tag and changes pages through the URL', async () => {
    problemsApi.listProblems.mockResolvedValue({ total: 45, items: [problem()] })
    const { wrapper, router } = await mountPage()

    await wrapper.find('.tag-chip').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ tag: 'math' })

    wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({ tag: 'math', page: '2' })
    expect(problemsApi.listProblems).toHaveBeenLastCalledWith({
      keyword: '',
      tag: 'math',
      difficulty: null,
      page: 2,
      includeHidden: false,
      pageSize: 20,
    })
  })

  it('shows administrator controls and requests hidden problems when enabled', async () => {
    auth.user = { role: 'problem_admin' }
    problemsApi.listProblems.mockResolvedValue({ total: 0, items: [] })
    const { wrapper } = await mountPage()

    expect(wrapper.find('a[href="/problems/new"]').exists()).toBe(true)
    wrapper.findComponent(ToggleSwitch).vm.$emit('update:modelValue', true)
    await flushPromises()

    expect(problemsApi.listProblems).toHaveBeenLastCalledWith({
      keyword: '',
      tag: '',
      difficulty: null,
      page: 1,
      includeHidden: true,
      pageSize: 20,
    })
  })

  it('shows the empty state when no problems match', async () => {
    problemsApi.listProblems.mockResolvedValue({ total: 0, items: [] })
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('没有找到符合条件的题目')
    expect(wrapper.text()).toContain('调整筛选条件')
  })

  it('shows an error state and retries the request', async () => {
    problemsApi.listProblems.mockRejectedValue(new ApiError(500, 50000, '加载题目失败'))
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('加载题目失败')
    const retry = wrapper.findAll('button').find((button) => button.text().includes('重试'))
    expect(retry).toBeTruthy()

    await retry!.trigger('click')
    await flushPromises()
    expect(problemsApi.listProblems).toHaveBeenCalledTimes(2)
  })
})
