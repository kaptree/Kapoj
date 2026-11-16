import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getContest } from '@/api/contests'
import { clearCompanionMuteCache } from '@/composables/useCompanionMute'
import { useCompanionStore } from '@/stores/companion'
import CompanionPageNote from './CompanionPageNote.vue'

vi.mock('@/api/contests', () => ({ getContest: vi.fn() }))

const routes = [
  { path: '/problems', name: 'problems', component: { template: '<div />' } },
  { path: '/privacy', name: 'privacy', component: { template: '<div />' } },
  { path: '/contests/:id(\\d+)', name: 'contest-detail', component: { template: '<div />' } },
]

let pinia: ReturnType<typeof createPinia>

async function mountAt(path: string) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  return mount(CompanionPageNote, { global: { plugins: [router, pinia] } })
}

beforeEach(() => {
  localStorage.clear()
  pinia = createPinia()
  setActivePinia(pinia)
  vi.mocked(getContest).mockReset()
  clearCompanionMuteCache()
})

describe('CompanionPageNote', () => {
  it('renders the mapped note and dismisses it for today', async () => {
    const wrapper = await mountAt('/problems')
    expect(wrapper.text()).toContain('今天的题')
    await wrapper.get('button').trigger('click')
    expect(wrapper.find('.companion-note').exists()).toBe(false)
    const store = useCompanionStore()
    store.load()
    expect(store.isNoteDismissed('problems')).toBe(true)
  })

  it('renders nothing on routes without a note', async () => {
    const wrapper = await mountAt('/privacy')
    expect(wrapper.find('.companion-note').exists()).toBe(false)
  })

  it('renders nothing when the companion is disabled or quiet', async () => {
    const store = useCompanionStore()
    store.load()
    store.setEnabled(false)
    expect((await mountAt('/problems')).find('.companion-note').exists()).toBe(false)
    store.setEnabled(true)
    store.setActivityLevel('quiet')
    expect((await mountAt('/problems')).find('.companion-note').exists()).toBe(false)
  })

  it('stays hidden on a running contest while its status is unknown and after it resolves', async () => {
    let resolveContest!: (value: unknown) => void
    vi.mocked(getContest).mockImplementation(
      () => new Promise((resolve) => (resolveContest = resolve)) as never,
    )

    const wrapper = await mountAt('/contests/7')

    // Status is still pending: the in-flow note must not flash in.
    expect(wrapper.find('.companion-note').exists()).toBe(false)

    resolveContest({ status: 'running' })
    await flushPromises()

    expect(wrapper.find('.companion-note').exists()).toBe(false)
  })

  it('shows the note on a non-running contest once its status resolves', async () => {
    vi.mocked(getContest).mockResolvedValue({ status: 'ended' } as never)

    const wrapper = await mountAt('/contests/7')
    await flushPromises()

    expect(wrapper.find('.companion-note').exists()).toBe(true)
  })
})
