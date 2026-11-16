import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getContest } from '@/api/contests'
import { useCompanionStore } from '@/stores/companion'
import { clearCompanionMuteCache, useCompanionMute } from './useCompanionMute'

vi.mock('@/api/contests', () => ({ getContest: vi.fn() }))

const routes = [
  { path: '/contests/:id(\\d+)', name: 'contest-detail', component: { template: '<div />' } },
  { path: '/problems/:id(\\d+)', name: 'problem-detail', component: { template: '<div />' } },
]

async function mountAt(
  path: string,
  before?: (store: ReturnType<typeof useCompanionStore>) => void,
) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const pinia = createPinia()
  setActivePinia(pinia)
  before?.(useCompanionStore())
  let result!: ReturnType<typeof useCompanionMute>
  const Comp = defineComponent({
    setup() {
      result = useCompanionMute()
      return () => null
    },
  })
  mount(Comp, { global: { plugins: [router, pinia] } })
  await flushPromises()
  return result
}

beforeEach(() => {
  clearCompanionMuteCache()
  vi.mocked(getContest).mockReset()
})

describe('useCompanionMute', () => {
  it('mutes a running contest detail', async () => {
    vi.mocked(getContest).mockResolvedValue({ status: 'running' } as never)
    const { muted } = await mountAt('/contests/7')
    expect(muted.value).toBe(true)
  })

  it('mutes a problem opened with a running contest query', async () => {
    vi.mocked(getContest).mockResolvedValue({ status: 'running' } as never)
    const { muted } = await mountAt('/problems/3?contestId=7')
    expect(muted.value).toBe(true)
  })

  it('does not stay muted when the contest request fails', async () => {
    vi.mocked(getContest).mockRejectedValue(new Error('offline'))
    const { muted } = await mountAt('/contests/7')
    expect(muted.value).toBe(false)
  })

  it('does not mute ordinary practice pages', async () => {
    const { muted } = await mountAt('/problems/3')
    expect(muted.value).toBe(false)
    expect(getContest).not.toHaveBeenCalled()
  })

  it('stays unmuted when the site config disables contest muting', async () => {
    vi.mocked(getContest).mockResolvedValue({ status: 'running' } as never)
    const { muted } = await mountAt('/contests/7', (store) =>
      store.applyGlobalConfig({
        enabled: true,
        name: 'Kapo',
        defaultActivityLevel: 'normal',
        contestMute: false,
        allowUserDisable: true,
      }),
    )
    expect(muted.value).toBe(false)
  })

  it('keeps checking false when there is no contest id', async () => {
    const { muted, checking } = await mountAt('/problems/3')
    expect(muted.value).toBe(false)
    expect(checking.value).toBe(false)
    expect(getContest).not.toHaveBeenCalled()
  })

  it('exposes checking while a contest status request is pending', async () => {
    let resolveContest!: (value: unknown) => void
    vi.mocked(getContest).mockImplementation(
      () => new Promise((resolve) => (resolveContest = resolve)) as never,
    )

    const { muted, checking } = await mountAt('/contests/7')

    expect(checking.value).toBe(true)
    expect(muted.value).toBe(false)

    resolveContest({ status: 'running' })
    await flushPromises()

    expect(checking.value).toBe(false)
    expect(muted.value).toBe(true)
  })

  it.each(['7.5', '1e2'])('ignores the non-integer contest query %s', async (contestId) => {
    vi.mocked(getContest).mockResolvedValue({ status: 'running' } as never)

    const { muted } = await mountAt(`/problems/3?contestId=${contestId}`)

    expect(muted.value).toBe(false)
    expect(getContest).not.toHaveBeenCalled()
  })

  it('reuses a contest status cached within the TTL', async () => {
    vi.mocked(getContest).mockResolvedValue({ status: 'running' } as never)

    const first = await mountAt('/contests/7')
    const second = await mountAt('/contests/7')

    expect(first.muted.value).toBe(true)
    expect(second.muted.value).toBe(true)
    expect(getContest).toHaveBeenCalledTimes(1)
  })
})
