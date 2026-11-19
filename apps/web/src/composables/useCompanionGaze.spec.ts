import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { gazeVector, useCompanionGaze } from './useCompanionGaze'

afterEach(() => {
  vi.restoreAllMocks()
})

describe('gazeVector', () => {
  it('returns 0,0 at the centre', () => {
    expect(gazeVector(500, 500, { left: 0, top: 0, width: 1000, height: 1000 })).toEqual({
      x: 0,
      y: 0,
    })
  })

  it('clamps to -1..1', () => {
    const vector = gazeVector(2000, -100, { left: 0, top: 0, width: 1000, height: 1000 })
    expect(vector.x).toBe(1)
    expect(vector.y).toBe(-1)
  })
})

describe('useCompanionGaze', () => {
  it('tracks the pointer, resets on leave and removes listeners on unmount', async () => {
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      callback(0)
      return 1
    })
    vi.stubGlobal('cancelAnimationFrame', () => undefined)

    const target = ref<HTMLElement | null>(document.createElement('div'))
    target.value!.getBoundingClientRect = () =>
      ({
        left: 0,
        top: 0,
        width: 1000,
        height: 1000,
        right: 1000,
        bottom: 1000,
        x: 0,
        y: 0,
        toJSON: () => ({}),
      }) as DOMRect
    const enabled = ref(true)

    const Comp = defineComponent({
      setup() {
        const { lookX, lookY } = useCompanionGaze({ target, enabled })
        return () => h('div', `${lookX.value.toFixed(2)},${lookY.value.toFixed(2)}`)
      },
    })
    const wrapper = mount(Comp)
    expect(wrapper.text()).toBe('0.00,0.00')

    window.dispatchEvent(new MouseEvent('pointermove', { clientX: 900, clientY: 100 }))
    await nextTick()
    expect(wrapper.text()).toBe('0.80,-0.80')

    window.dispatchEvent(new MouseEvent('pointerleave'))
    await nextTick()
    expect(wrapper.text()).toBe('0.00,0.00')

    const remove = vi.spyOn(window, 'removeEventListener')
    wrapper.unmount()
    expect(remove).toHaveBeenCalledWith('pointermove', expect.any(Function))
  })
})
