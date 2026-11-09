import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Paginator from 'primevue/paginator'
import ToastService from 'primevue/toastservice'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick, reactive } from 'vue'

import { ApiError } from '@/api/http'
import { listContests } from '@/api/contests'
import type { ContestListItem } from '@/api/types'
import ProblemContestsView from './ProblemContestsView.vue'

const route = reactive({ params: { id: '3' } })

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return { ...actual, useRoute: () => route }
})
vi.mock('@/api/contests', () => ({
  listContests: vi.fn(),
}))

function contest(overrides: Partial<ContestListItem> = {}): ContestListItem {
  return {
    id: 1,
    title: 'Weekly Contest',
    descriptionMd: '',
    startTime: '2026-01-01T00:00:00Z',
    endTime: '2026-01-02T00:00:00Z',
    mode: 'icpc',
    isRated: true,
    isPublic: true,
    disableAi: true,
    registerType: 'open',
    status: 'running',
    problemCount: 5,
    participantCount: 20,
    registered: true,
    createdAt: '2025-12-01T00:00:00Z',
    ...overrides,
  }
}

function mountView() {
  return mount(ProblemContestsView, {
    global: {
      plugins: [PrimeVue, ToastService, createPinia()],
      stubs: {
        RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
      },
    },
  })
}

beforeEach(() => {
  route.params.id = '3'
  vi.mocked(listContests).mockReset()
})

describe('ProblemContestsView', () => {
  it('loads contests with problemId and renders a contest card', async () => {
    vi.mocked(listContests).mockResolvedValue({ total: 45, items: [contest()] })
    const wrapper = mountView()
    await flushPromises()

    expect(listContests).toHaveBeenCalledWith({ problemId: '3', page: 1, pageSize: 20 })
    expect(wrapper.text()).toContain('Weekly Contest')
    expect(wrapper.text()).toContain('进行中')
    expect(wrapper.text()).toContain('ICPC')
    expect(wrapper.text()).toContain('公开报名')
    expect(wrapper.text()).toContain('已报名')
    expect(wrapper.text()).toContain('5 题 / 20 人')
    expect(wrapper.find('a[href="/contests/1"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('changes pages through the paginator', async () => {
    vi.mocked(listContests).mockResolvedValue({ total: 45, items: [contest()] })
    const wrapper = mountView()
    await flushPromises()

    wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()
    expect(listContests).toHaveBeenLastCalledWith({ problemId: '3', page: 2, pageSize: 20 })
    wrapper.unmount()
  })

  it('shows empty and error states', async () => {
    vi.mocked(listContests).mockResolvedValueOnce({ total: 0, items: [] })
    const empty = mountView()
    await flushPromises()
    expect(empty.text()).toContain('暂无关联比赛')
    empty.unmount()

    vi.mocked(listContests).mockRejectedValueOnce(new ApiError(500, 50000, '加载比赛失败'))
    const failed = mountView()
    await flushPromises()
    expect(failed.text()).toContain('加载比赛失败')
    expect(failed.text()).toContain('重试')
    failed.unmount()
  })

  it('clears the previous problem contests while switching problems', async () => {
    vi.mocked(listContests).mockResolvedValueOnce({
      total: 1,
      items: [contest({ title: '旧题的比赛' })],
    })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('旧题的比赛')

    let resolveSecond!: (value: { total: number; items: ContestListItem[] }) => void
    vi.mocked(listContests).mockImplementationOnce(
      () => new Promise((resolve) => (resolveSecond = resolve)),
    )
    route.params.id = '4'
    await nextTick()

    expect(wrapper.text()).not.toContain('旧题的比赛')
    expect(wrapper.text()).toContain('正在加载关联比赛')

    resolveSecond({ total: 1, items: [contest({ id: 2, title: '新题的比赛' })] })
    await flushPromises()
    expect(wrapper.text()).toContain('新题的比赛')
    wrapper.unmount()
  })

  it('shows an error when switching to a problem whose load fails', async () => {
    vi.mocked(listContests).mockResolvedValueOnce({
      total: 1,
      items: [contest({ title: '旧题的比赛' })],
    })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('旧题的比赛')

    vi.mocked(listContests).mockRejectedValueOnce(new ApiError(500, 50000, '加载比赛失败'))
    route.params.id = '4'
    await flushPromises()

    expect(wrapper.text()).not.toContain('旧题的比赛')
    expect(wrapper.text()).toContain('加载比赛失败')
    wrapper.unmount()
  })

  it('shows an error when a page change fails with existing contests', async () => {
    vi.mocked(listContests).mockResolvedValueOnce({ total: 45, items: [contest()] })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('Weekly Contest')

    vi.mocked(listContests).mockRejectedValueOnce(new ApiError(500, 50000, '加载比赛失败'))
    wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()

    expect(wrapper.text()).toContain('加载比赛失败')
    expect(wrapper.text()).toContain('Weekly Contest')
    wrapper.unmount()
  })
})
