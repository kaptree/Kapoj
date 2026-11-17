import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import DataTable from 'primevue/datatable'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Paginator from 'primevue/paginator'
import Select from 'primevue/select'
import ToggleSwitch from 'primevue/toggleswitch'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'

import { ApiError } from '@/api/http'
import type { ProblemListItem } from '@/api/types'
import ProblemListView from './ProblemListView.vue'

const problemsApi = vi.hoisted(() => ({
  listProblems: vi.fn(),
  bulkProblemAction: vi.fn(),
  exportProblems: vi.fn(),
}))

const auth = vi.hoisted(() => ({
  can: vi.fn<(code: string) => boolean>(() => false),
}))

vi.mock('@/api/problems', () => ({
  listProblems: problemsApi.listProblems,
  bulkProblemAction: problemsApi.bulkProblemAction,
  exportProblems: problemsApi.exportProblems,
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
    difficulty: 62,
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
  { path: '/problems/import', name: 'problem-import', component: { template: '<div />' } },
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
  auth.can.mockReset()
  auth.can.mockReturnValue(false)
  problemsApi.listProblems.mockReset()
  problemsApi.bulkProblemAction.mockReset()
  problemsApi.exportProblems.mockReset()
})

async function selectRows(wrapper: ReturnType<typeof mount>, rows: ProblemListItem[]) {
  wrapper.findComponent(DataTable).vm.$emit('row-select-all', { data: rows })
  await nextTick()
  await flushPromises()
}

function button(wrapper: ReturnType<typeof mount>, label: string) {
  const found = wrapper.findAll('button').find((b) => b.text().includes(label))
  expect(found, `button "${label}"`).toBeTruthy()
  return found!
}

describe('ProblemListView', () => {
  it('loads from the route query and renders a problem row', async () => {
    problemsApi.listProblems.mockResolvedValue({ total: 1, items: [problem()] })
    const { wrapper } = await mountPage(
      '/problems?keyword=sum&tag=math&difficultyMin=40&difficultyMax=80&page=2',
    )

    expect(problemsApi.listProblems).toHaveBeenCalledWith({
      keyword: 'sum',
      tag: 'math',
      difficultyMin: 40,
      difficultyMax: 80,
      scope: 'public',
      page: 2,
      includeHidden: false,
      pageSize: 20,
    })
    expect(wrapper.text()).toContain('两数之和')
    expect(wrapper.text()).toContain('62')
    expect(wrapper.find('.app-difficulty-tag--4').exists()).toBe(true)
    expect(wrapper.text()).toContain('10 / 20')
    expect(wrapper.text()).toContain('50%')
    expect(wrapper.find('a[href="/problems/1"]').exists()).toBe(true)
  })

  it('keeps legacy exact difficulty links filtering a single score', async () => {
    problemsApi.listProblems.mockResolvedValue({
      total: 1,
      items: [problem({ difficulty: 3 })],
    })
    const { wrapper } = await mountPage('/problems?difficulty=3')

    expect(problemsApi.listProblems).toHaveBeenCalledWith({
      keyword: '',
      tag: '',
      difficultyMin: 3,
      difficultyMax: 3,
      scope: 'public',
      page: 1,
      includeHidden: false,
      pageSize: 20,
    })
    expect(wrapper.text()).toContain('两数之和')
    expect(wrapper.find('.app-difficulty-tag').text()).toBe('3')
  })

  it('trims filters and resets the page when searching', async () => {
    problemsApi.listProblems.mockResolvedValue({ total: 0, items: [] })
    const { wrapper, router } = await mountPage()
    const inputs = wrapper.findAllComponents(InputText)
    const numbers = wrapper.findAllComponents(InputNumber)
    expect(numbers).toHaveLength(2)

    await inputs[0]!.setValue('  sum  ')
    await inputs[1]!.setValue(' math ')
    numbers[0]!.vm.$emit('update:modelValue', 40)
    numbers[1]!.vm.$emit('update:modelValue', 80)
    await nextTick()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({
      keyword: 'sum',
      tag: 'math',
      difficultyMin: '40',
      difficultyMax: '80',
    })
    expect(problemsApi.listProblems).toHaveBeenLastCalledWith({
      keyword: 'sum',
      tag: 'math',
      difficultyMin: 40,
      difficultyMax: 80,
      scope: 'public',
      page: 1,
      includeHidden: false,
      pageSize: 20,
    })
  })

  it('swaps a reversed difficulty range before searching', async () => {
    problemsApi.listProblems.mockResolvedValue({ total: 0, items: [] })
    const { wrapper, router } = await mountPage()
    const numbers = wrapper.findAllComponents(InputNumber)

    numbers[0]!.vm.$emit('update:modelValue', 80)
    numbers[1]!.vm.$emit('update:modelValue', 40)
    await nextTick()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({
      difficultyMin: '40',
      difficultyMax: '80',
    })
    expect(problemsApi.listProblems).toHaveBeenLastCalledWith({
      keyword: '',
      tag: '',
      difficultyMin: 40,
      difficultyMax: 80,
      scope: 'public',
      page: 1,
      includeHidden: false,
      pageSize: 20,
    })
  })

  it('keeps difficulty filters integral before navigating', async () => {
    problemsApi.listProblems.mockResolvedValue({ total: 0, items: [] })
    const { wrapper, router } = await mountPage()
    const numbers = wrapper.findAllComponents(InputNumber)

    expect(numbers[0]!.props('maxFractionDigits')).toBe(0)
    expect(numbers[1]!.props('maxFractionDigits')).toBe(0)

    numbers[0]!.vm.$emit('update:modelValue', 40.5)
    numbers[1]!.vm.$emit('update:modelValue', 80.4)
    await nextTick()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({
      difficultyMin: '41',
      difficultyMax: '80',
    })
    expect(problemsApi.listProblems).toHaveBeenLastCalledWith(
      expect.objectContaining({ difficultyMin: 41, difficultyMax: 80 }),
    )
    wrapper.unmount()
  })

  it('resets filters through the URL', async () => {
    problemsApi.listProblems.mockResolvedValue({ total: 0, items: [] })
    const { wrapper, router } = await mountPage(
      '/problems?keyword=sum&tag=math&difficultyMin=40&difficultyMax=80',
    )

    await button(wrapper, '重置').trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({})
    expect(problemsApi.listProblems).toHaveBeenLastCalledWith({
      keyword: '',
      tag: '',
      difficultyMin: null,
      difficultyMax: null,
      scope: 'public',
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
      difficultyMin: null,
      difficultyMax: null,
      scope: 'public',
      page: 2,
      includeHidden: false,
      pageSize: 20,
    })
  })

  it('problem.create shows create; problem.view_hidden shows hidden toggle', async () => {
    auth.can.mockImplementation(
      (code: string) => code === 'problem.create' || code === 'problem.view_hidden',
    )
    problemsApi.listProblems.mockResolvedValue({ total: 0, items: [] })
    const { wrapper } = await mountPage()

    expect(wrapper.find('a[href="/problems/new"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/problems/import"]').exists()).toBe(true)
    expect(wrapper.findComponent(Select).exists()).toBe(true)
    expect(wrapper.find('#show-hidden').exists()).toBe(true)

    wrapper.findComponent(ToggleSwitch).vm.$emit('update:modelValue', true)
    await flushPromises()

    expect(problemsApi.listProblems).toHaveBeenLastCalledWith({
      keyword: '',
      tag: '',
      difficultyMin: null,
      difficultyMax: null,
      scope: 'mine',
      page: 1,
      includeHidden: true,
      pageSize: 20,
    })
  })

  it('a user without problem permissions sees no create button or hidden toggle', async () => {
    problemsApi.listProblems.mockResolvedValue({ total: 0, items: [] })
    const { wrapper } = await mountPage()

    expect(wrapper.find('a[href="/problems/new"]').exists()).toBe(false)
    expect(wrapper.find('a[href="/problems/import"]').exists()).toBe(false)
    expect(wrapper.find('#show-hidden').exists()).toBe(false)
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

  it('shows bulk management only with problem.manage_all', async () => {
    problemsApi.listProblems.mockResolvedValue({ total: 0, items: [] })
    let view = await mountPage()
    expect(view.wrapper.text()).not.toContain('批量管理')
    view.wrapper.unmount()

    auth.can.mockImplementation((code) => code === 'problem.manage_all')
    view = await mountPage()
    expect(view.wrapper.text()).toContain('批量管理')
    view.wrapper.unmount()
  })

  it('publishes selected problems after confirmation and reports per-problem failures', async () => {
    auth.can.mockImplementation((code) => code === 'problem.manage_all')
    problemsApi.listProblems.mockResolvedValue({
      total: 2,
      items: [problem({ id: 1 }), problem({ id: 2, title: 'Another' })],
    })
    problemsApi.bulkProblemAction.mockResolvedValue({
      results: [
        { id: 1, status: 'success', offlined: false, message: '已公开' },
        { id: 2, status: 'failed', offlined: false, error: '题目被学习路线引用' },
      ],
    })
    const { wrapper } = await mountPage()

    await button(wrapper, '批量管理').trigger('click')
    await flushPromises()
    await selectRows(wrapper, [problem({ id: 1 }), problem({ id: 2, title: 'Another' })])
    await button(wrapper, '公开').trigger('click')
    await flushPromises()
    expect(problemsApi.bulkProblemAction).not.toHaveBeenCalled()

    const confirm = document.querySelector('[data-test="bulk-confirm-submit"]') as HTMLButtonElement
    expect(confirm).not.toBeNull()
    confirm.click()
    await flushPromises()

    expect(problemsApi.bulkProblemAction).toHaveBeenCalledWith({
      action: 'publish',
      problemIds: [1, 2],
    })
    expect(wrapper.text()).toContain('成功 1 道')
    expect(wrapper.text()).toContain('失败 1 道')
    expect(wrapper.text()).toContain('已公开')
    expect(wrapper.text()).toContain('题目被学习路线引用')
    expect(problemsApi.listProblems).toHaveBeenCalledTimes(2)
  })

  it('downloads selected problems as one ZIP', async () => {
    auth.can.mockImplementation((code) => code === 'problem.manage_all')
    problemsApi.listProblems.mockResolvedValue({
      total: 2,
      items: [problem({ id: 1 }), problem({ id: 2, title: 'Another' })],
    })
    const blob = new Blob(['zip'], { type: 'application/zip' })
    problemsApi.exportProblems.mockResolvedValue(blob)
    const createObjectURL = vi.fn(() => 'blob:mock')
    Object.defineProperty(URL, 'createObjectURL', {
      configurable: true,
      writable: true,
      value: createObjectURL,
    })
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined)

    const { wrapper } = await mountPage()
    await button(wrapper, '批量管理').trigger('click')
    await flushPromises()
    await selectRows(wrapper, [problem({ id: 1 }), problem({ id: 2, title: 'Another' })])
    await button(wrapper, '下载').trigger('click')
    await flushPromises()

    expect(problemsApi.exportProblems).toHaveBeenCalledWith([1, 2])
    expect(createObjectURL).toHaveBeenCalledWith(blob)
    expect(click).toHaveBeenCalled()
    click.mockRestore()
  })

  it('keeps select-all selections across pages', async () => {
    auth.can.mockImplementation((code) => code === 'problem.manage_all')
    problemsApi.listProblems.mockImplementation((query: { page?: number }) =>
      Promise.resolve(
        query.page === 2
          ? { total: 2, items: [problem({ id: 2, title: 'Second' })] }
          : { total: 2, items: [problem({ id: 1 })] },
      ),
    )
    const blob = new Blob(['zip'], { type: 'application/zip' })
    problemsApi.exportProblems.mockResolvedValue(blob)
    Object.defineProperty(URL, 'createObjectURL', {
      configurable: true,
      writable: true,
      value: vi.fn(() => 'blob:mock'),
    })
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined)

    const { wrapper, router } = await mountPage('/problems?page=1')
    await button(wrapper, '批量管理').trigger('click')
    await flushPromises()
    await selectRows(wrapper, [problem({ id: 1 })])
    expect(wrapper.text()).toContain('已选 1 道')

    await router.push({ name: 'problems', query: { page: '2' } })
    await flushPromises()
    await selectRows(wrapper, [problem({ id: 2, title: 'Second' })])
    expect(wrapper.text()).toContain('已选 2 道')

    await button(wrapper, '下载').trigger('click')
    await flushPromises()
    expect(problemsApi.exportProblems).toHaveBeenCalledWith([1, 2])
    click.mockRestore()
  })

  it('deletes selected problems only after confirmation', async () => {
    auth.can.mockImplementation((code) => code === 'problem.manage_all')
    problemsApi.listProblems.mockResolvedValue({
      total: 1,
      items: [problem({ id: 1 })],
    })
    problemsApi.bulkProblemAction.mockResolvedValue({
      results: [{ id: 1, status: 'success', offlined: true }],
    })
    const { wrapper } = await mountPage()

    await button(wrapper, '批量管理').trigger('click')
    await flushPromises()
    await selectRows(wrapper, [problem({ id: 1 })])
    await button(wrapper, '删除').trigger('click')
    await flushPromises()
    expect(problemsApi.bulkProblemAction).not.toHaveBeenCalled()

    const confirm = document.querySelector('[data-test="bulk-confirm-submit"]') as HTMLButtonElement
    confirm.click()
    await flushPromises()

    expect(problemsApi.bulkProblemAction).toHaveBeenCalledWith({
      action: 'delete',
      problemIds: [1],
    })
    expect(wrapper.text()).toContain('已下线')
  })
})
