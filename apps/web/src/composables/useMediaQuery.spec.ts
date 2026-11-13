import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick, type Ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'

import { useMediaQuery } from './useMediaQuery'

describe('useMediaQuery', () => {
  it('initializes from matchMedia, updates on change, and removes its listener on unmount', async () => {
    let listener: ((event: MediaQueryListEvent) => void) | undefined
    const mediaQueryList = {
      matches: true,
      media: '(max-width: 899px)',
      onchange: null,
      addEventListener: vi.fn((_type: string, callback: EventListenerOrEventListenerObject) => {
        listener = callback as (event: MediaQueryListEvent) => void
      }),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(() => false),
    } as unknown as MediaQueryList
    vi.spyOn(window, 'matchMedia').mockReturnValue(mediaQueryList)

    let matches: Ref<boolean>
    const Host = defineComponent({
      setup() {
        matches = useMediaQuery('(max-width: 899px)')
        return () => h('output', matches.value ? 'mobile' : 'desktop')
      },
    })
    const wrapper = mount(Host)
    await nextTick()

    expect(wrapper.text()).toBe('mobile')
    expect(mediaQueryList.addEventListener).toHaveBeenCalledWith('change', expect.any(Function))

    listener?.({ matches: false } as MediaQueryListEvent)
    await nextTick()
    expect(wrapper.text()).toBe('desktop')

    wrapper.unmount()
    expect(mediaQueryList.removeEventListener).toHaveBeenCalledWith('change', expect.any(Function))
  })
})
