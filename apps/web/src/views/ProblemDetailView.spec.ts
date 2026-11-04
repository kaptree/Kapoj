import { describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import { createPinia } from 'pinia'

import { getProblem, getStatistics } from '@/api/problems'
import type { ProblemDetail, ProblemStats } from '@/api/types'
import ProblemDetailView from './ProblemDetailView.vue'

const route = reactive({ params: { id: '1' } })

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return { ...actual, useRoute: () => route }
})
vi.mock('@/components/MarkdownView.vue', () => ({
  default: { props: ['source'], template: '<div>{{ source }}</div>' },
}))
vi.mock('@/api/problems', () => ({
  getProblem: vi.fn(),
  getStatistics: vi.fn(),
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

describe('ProblemDetailView', () => {
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
    const wrapper = mount(ProblemDetailView, {
      global: {
        plugins: [ElementPlus, createPinia()],
        stubs: { RouterLink: { template: '<a><slot /></a>' } },
      },
    })

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
    expect(wrapper.text()).toContain('50.0%')

    firstStats.resolve(stats(10, 0))
    await flushPromises()
    expect(wrapper.text()).toContain('50.0%')

    wrapper.unmount()
  })
})
