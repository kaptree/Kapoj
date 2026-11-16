import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useCompanionStore } from '@/stores/companion'
import CompanionPageNote from './CompanionPageNote.vue'

vi.mock('@/composables/useCompanionMute', () => ({
  useCompanionMute: () => ({ muted: { value: false }, checking: { value: false } }),
}))

const routes = [
  { path: '/problems', name: 'problems', component: { template: '<div />' } },
  { path: '/privacy', name: 'privacy', component: { template: '<div />' } },
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
})
