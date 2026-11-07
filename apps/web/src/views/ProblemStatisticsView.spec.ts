import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'

import { ApiError } from '@/api/http'
import { getStatistics } from '@/api/problems'
import type { ProblemStats } from '@/api/types'
import ProblemStatisticsView from './ProblemStatisticsView.vue'

const route = reactive({ params: { id: '7' } })

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return { ...actual, useRoute: () => route }
})
vi.mock('@/api/problems', () => ({
  getStatistics: vi.fn(),
}))

function stats(submitCount: number, acceptCount: number): ProblemStats {
  return {
    submitCount,
    acceptCount,
    acceptRate: submitCount === 0 ? 0 : acceptCount / submitCount,
  }
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
})

describe('ProblemStatisticsView', () => {
  it('renders stats and a submissions link', async () => {
    vi.mocked(getStatistics).mockResolvedValue(stats(10, 5))
    const wrapper = mountView()
    await flushPromises()

    expect(getStatistics).toHaveBeenCalledWith('7')
    expect(wrapper.text()).toContain('提交数')
    expect(wrapper.text()).toContain('10')
    expect(wrapper.text()).toContain('通过数')
    expect(wrapper.text()).toContain('5')
    expect(wrapper.text()).toContain('通过率')
    expect(wrapper.text()).toContain('50.0%')
    expect(wrapper.text()).toContain('查看提交记录')
    expect(wrapper.find('a[href="/submissions?problemId=7"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows a zero state and an error state', async () => {
    vi.mocked(getStatistics).mockResolvedValue(stats(0, 0))
    const empty = mountView()
    await flushPromises()
    expect(empty.text()).toContain('0.0%')
    empty.unmount()

    vi.mocked(getStatistics).mockRejectedValue(new ApiError(500, 50000, '加载统计失败'))
    const failed = mountView()
    await flushPromises()
    expect(failed.text()).toContain('加载统计失败')
    expect(failed.text()).toContain('重试')
    failed.unmount()
  })
})
