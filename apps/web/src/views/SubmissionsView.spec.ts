import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import DataTable from 'primevue/datatable'
import Paginator from 'primevue/paginator'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import { ApiError } from '@/api/http'
import { getSubmission, listSubmissions } from '@/api/submissions'
import type { SubmissionDetail, SubmissionListItem } from '@/api/types'
import SubmissionsView from './SubmissionsView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))

vi.mock('primevue/usetoast', () => ({
  useToast: () => toast,
}))
vi.mock('@/api/submissions', () => ({
  listSubmissions: vi.fn(),
  getSubmission: vi.fn(),
}))

class FakeEventSource {
  static instances: FakeEventSource[] = []

  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  onmessage: (() => void) | null = null
  close = vi.fn()

  constructor(readonly url: string) {
    FakeEventSource.instances.push(this)
  }
}

function item(overrides: Partial<SubmissionListItem> = {}): SubmissionListItem {
  return {
    id: 1,
    userId: 7,
    username: 'alice',
    nickname: 'Alice',
    equippedBadges: [],
    equippedTitle: null,
    problemId: 3,
    problemTitle: 'Two Sum',
    problemSlug: 'two-sum',
    language: 'cpp',
    status: 'accepted',
    timeUsedMs: 100,
    memoryUsedKb: 1024,
    score: 100,
    createdAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function detail(overrides: Partial<SubmissionDetail> = {}): SubmissionDetail {
  return {
    ...item(),
    code: 'int main() {}',
    codeHidden: false,
    judgeResult: [{ testcaseId: 1, status: 'accepted', timeMs: 10, memoryKb: 512, message: 'ok' }],
    ...overrides,
  }
}

const routes = [
  { path: '/submissions', name: 'submissions', component: { template: '<div />' } },
  { path: '/users/:id', name: 'user-profile', component: { template: '<div />' } },
  { path: '/problems/:id', name: 'problem-detail', component: { template: '<div />' } },
]

async function mountPage(path = '/submissions') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(SubmissionsView, { global: { plugins: [router, PrimeVue] } })
  await flushPromises()
  return { wrapper, router }
}

function buttons(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper']) {
  return wrapper.findAll('button')
}

function findButton(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper'], label: string) {
  return buttons(wrapper).find((button) => button.text().trim() === label)
}

beforeEach(() => {
  FakeEventSource.instances = []
  vi.stubGlobal('EventSource', FakeEventSource)
  toast.add.mockReset()
  vi.mocked(listSubmissions).mockReset()
  vi.mocked(getSubmission).mockReset()
  vi.mocked(listSubmissions).mockResolvedValue({ total: 45, items: [item()] })
  vi.mocked(getSubmission).mockResolvedValue(detail())
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('SubmissionsView', () => {
  it('loads from the route query and renders a submission row', async () => {
    const { wrapper } = await mountPage(
      '/submissions?problemId=1&language=cpp&status=accepted&page=2',
    )

    expect(listSubmissions).toHaveBeenCalledWith({
      problemId: 1,
      contestId: undefined,
      userId: undefined,
      language: 'cpp',
      status: 'accepted',
      nickname: '',
      page: 2,
      pageSize: 20,
    })
    expect(wrapper.text()).toContain('Alice')
    expect(wrapper.text()).toContain('Two Sum')
    expect(wrapper.text()).toContain('通过')
    expect(wrapper.text()).toContain('C++')
    expect(wrapper.text()).toContain('100 ms')
    expect(wrapper.text()).toContain('1024 KB')
    expect(wrapper.find('a[href="/users/7"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/problems/3"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('applies nickname filters and page changes through the URL', async () => {
    const { wrapper, router } = await mountPage()

    await wrapper.find('#filter-nickname').setValue(' alice ')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({ nickname: 'alice' })
    expect(listSubmissions).toHaveBeenLastCalledWith(
      expect.objectContaining({ nickname: 'alice', page: 1 }),
    )

    wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ nickname: 'alice', page: '2' })
    expect(listSubmissions).toHaveBeenLastCalledWith(
      expect.objectContaining({ nickname: 'alice', page: 2 }),
    )

    await findButton(wrapper, '重置')!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({})
    wrapper.unmount()
  })

  it('shows live state and reloads on SSE events', async () => {
    vi.useFakeTimers()
    const { wrapper } = await mountPage()
    const source = FakeEventSource.instances[0]!

    expect(source.url).toBe('/api/submissions/stream')
    expect(listSubmissions).toHaveBeenCalledTimes(1)

    source.onopen?.()
    await nextTick()
    expect(wrapper.text()).toContain('实时刷新')

    vi.advanceTimersByTime(300)
    await flushPromises()
    expect(listSubmissions).toHaveBeenCalledTimes(2)

    source.onmessage?.()
    vi.advanceTimersByTime(300)
    await flushPromises()
    expect(listSubmissions).toHaveBeenCalledTimes(3)

    source.onerror?.()
    await nextTick()
    expect(wrapper.text()).toContain('连接中断，正在重连')

    wrapper.unmount()
    expect(source.close).toHaveBeenCalled()
  })

  it('opens the detail drawer with code and judge results', async () => {
    const { wrapper } = await mountPage()

    wrapper.findComponent(DataTable).vm.$emit('row-click', { data: item() })
    await flushPromises()
    await nextTick()

    expect(getSubmission).toHaveBeenCalledWith(1)
    const body = document.body.textContent ?? ''
    expect(body).toContain('测评详情')
    expect(body).toContain('int main() {}')
    expect(body).toContain('测试点')
    expect(body).toContain('通过')
    wrapper.unmount()
  })

  it('hides code and test details when the server marks them hidden', async () => {
    vi.mocked(getSubmission).mockResolvedValue(
      detail({ codeHidden: true, code: undefined, judgeResult: [] }),
    )
    const { wrapper } = await mountPage()

    wrapper.findComponent(DataTable).vm.$emit('row-click', { data: item() })
    await flushPromises()
    await nextTick()

    const body = document.body.textContent ?? ''
    expect(body).toContain('比赛进行中，其他选手的代码与测评详情暂不可见。')
    expect(body).not.toContain('int main() {}')
    wrapper.unmount()
  })

  it('reports detail loading failures and keeps the page usable', async () => {
    vi.mocked(getSubmission).mockRejectedValue(new ApiError(404, 40400, '记录不存在'))
    const { wrapper } = await mountPage()

    wrapper.findComponent(DataTable).vm.$emit('row-click', { data: item() })
    await flushPromises()

    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'error', detail: '记录不存在' }),
    )
    expect(listSubmissions).toHaveBeenCalled()
    wrapper.unmount()
  })
})
