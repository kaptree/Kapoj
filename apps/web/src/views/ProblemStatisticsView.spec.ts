import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import Paginator from 'primevue/paginator'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'

import { ApiError } from '@/api/http'
import { getStatistics, getSubmissionStats } from '@/api/problems'
import type { ProblemStats, ProblemSubmissionMetric } from '@/api/types'
import RatingLevelName from '@/components/RatingLevelName.vue'
import ProblemStatisticsView from './ProblemStatisticsView.vue'

const route = reactive({ params: { id: '7' } })

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return { ...actual, useRoute: () => route }
})
vi.mock('@/api/problems', () => ({
  getStatistics: vi.fn(),
  getSubmissionStats: vi.fn(),
}))

function stats(submitCount: number, acceptCount: number): ProblemStats {
  return {
    submitCount,
    acceptCount,
    acceptRate: submitCount === 0 ? 0 : acceptCount / submitCount,
    statusCounts:
      submitCount === 0
        ? []
        : [
            {
              status: 'accepted',
              count: acceptCount,
              percentage: submitCount === 0 ? 0 : acceptCount / submitCount,
            },
            {
              status: 'wrong_answer',
              count: submitCount - acceptCount,
              percentage: submitCount === 0 ? 0 : (submitCount - acceptCount) / submitCount,
            },
          ],
  }
}

function metric(overrides: Partial<ProblemSubmissionMetric> = {}): ProblemSubmissionMetric {
  return {
    userId: 12,
    username: 'metric-user',
    nickname: '指标用户',
    level: { key: 'blue', name: '蓝名', color: '#0000FF', accentColor: '' },
    avatarUrl: 'https://example.com/avatar.png',
    equippedBadges: [],
    equippedTitle: null,
    language: 'cpp',
    status: 'accepted',
    timeUsedMs: 12,
    memoryUsedKb: 345,
    codeLength: 42,
    createdAt: '2026-11-14T10:00:00Z',
    ...overrides,
  }
}

function page(items: ProblemSubmissionMetric[], total = items.length) {
  return { total, items }
}

function mountView() {
  return mount(ProblemStatisticsView, {
    global: {
      plugins: [PrimeVue, ToastService, createPinia()],
      stubs: {
        RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
      },
    },
  })
}

beforeEach(() => {
  vi.mocked(getStatistics).mockReset()
  vi.mocked(getSubmissionStats).mockReset()
  vi.mocked(getStatistics).mockResolvedValue(stats(10, 5))
  vi.mocked(getSubmissionStats).mockResolvedValue(page([metric()]))
})

describe('ProblemStatisticsView', () => {
  it('renders summary, charts, legend and metrics without code entry points', async () => {
    vi.mocked(getSubmissionStats).mockResolvedValue(page([metric()], 21))
    const wrapper = mountView()
    await flushPromises()

    expect(getStatistics).toHaveBeenCalledWith('7')
    expect(getSubmissionStats).toHaveBeenCalledWith('7', {
      sort: 'created_at',
      order: 'desc',
      page: 1,
      pageSize: 20,
    })

    const text = wrapper.text()
    expect(text).toContain('提交数')
    expect(text).toContain('10')
    expect(text).toContain('通过数')
    expect(text).toContain('5')
    expect(text).toContain('通过率')
    expect(text).toContain('50.0%')
    expect(text).toContain('结果分布')
    expect(text).toContain('提交明细')
    expect(wrapper.find('[data-test="status-pie"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="status-bars"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="status-legend"]').text()).toContain('通过')
    expect(wrapper.find('[data-test="status-legend"]').text()).toContain('50.0%')
    expect(text).toContain('指标用户')
    expect(wrapper.findComponent(RatingLevelName).props('level').key).toBe('blue')
    expect(text).toContain('42 B')
    expect(text).not.toContain('查看代码')
    expect(text).not.toContain('SECRET_CODE')
    expect(wrapper.find('a[href="/submissions?problemId=7"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows chart and table empty states', async () => {
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    vi.mocked(getSubmissionStats).mockResolvedValue(page([]))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('暂无可见提交')
    expect(wrapper.text()).toContain('暂无提交明细')
    expect(wrapper.find('[data-test="status-pie"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('reloads metrics when sort or order changes', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('[data-test="sort-field"]').setValue('time')
    await flushPromises()
    await wrapper.find('[data-test="sort-order"]').setValue('asc')
    await flushPromises()

    expect(getSubmissionStats).toHaveBeenLastCalledWith('7', {
      sort: 'time',
      order: 'asc',
      page: 1,
      pageSize: 20,
    })
    wrapper.unmount()
  })

  it('moves to the next page on paginator input', async () => {
    vi.mocked(getSubmissionStats).mockResolvedValue(page([metric()], 25))
    const wrapper = mountView()
    await flushPromises()

    wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()

    expect(getSubmissionStats).toHaveBeenLastCalledWith('7', {
      sort: 'created_at',
      order: 'desc',
      page: 2,
      pageSize: 20,
    })
    wrapper.unmount()
  })

  it('shows statistics and metrics error states', async () => {
    vi.mocked(getStatistics).mockRejectedValue(new ApiError(500, 50000, '加载统计失败'))
    const failed = mountView()
    await flushPromises()
    expect(failed.text()).toContain('加载统计失败')
    expect(failed.text()).toContain('重试')
    failed.unmount()

    vi.mocked(getStatistics).mockResolvedValue(stats(10, 5))
    vi.mocked(getSubmissionStats).mockRejectedValue(new ApiError(500, 50000, '加载明细失败'))
    const failedMetrics = mountView()
    await flushPromises()
    expect(failedMetrics.text()).toContain('加载明细失败')
    failedMetrics.unmount()
  })
})
