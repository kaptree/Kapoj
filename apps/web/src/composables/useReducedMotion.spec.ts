import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useCompanionStore } from '@/stores/companion'
import { useReducedMotion } from './useReducedMotion'

let result: ReturnType<typeof useReducedMotion>

function mountComposable() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const Comp = defineComponent({
    setup() {
      result = useReducedMotion()
      return () => null
    },
  })
  return mount(Comp, { global: { plugins: [pinia] } })
}

beforeEach(() => {
  vi.mocked(window.matchMedia).mockReturnValue({
    matches: false,
    media: '',
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  } as MediaQueryList)
})

describe('useReducedMotion', () => {
  it('honors the store override and the system preference', async () => {
    mountComposable()
    const store = useCompanionStore()
    expect(result.value).toBe(false)
    store.setMotion('reduce')
    await Promise.resolve()
    expect(result.value).toBe(true)
  })

  it('follows the system preference and removes its listener on unmount', () => {
    const removeEventListener = vi.fn()
    vi.mocked(window.matchMedia).mockReturnValue({
      matches: true,
      media: '(prefers-reduced-motion: reduce)',
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
      addEventListener: () => undefined,
      removeEventListener,
      dispatchEvent: () => false,
    } as MediaQueryList)

    const wrapper = mountComposable()
    expect(result.value).toBe(true)

    wrapper.unmount()
    expect(removeEventListener).toHaveBeenCalledWith('change', expect.any(Function))
  })
})
