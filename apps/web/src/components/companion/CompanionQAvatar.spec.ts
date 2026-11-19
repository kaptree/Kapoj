import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { describe, expect, it } from 'vitest'

import { COMPANION_Q } from '../../content/companion/manifest'
import CompanionQAvatar from './CompanionQAvatar.vue'

function srcs(wrapper: ReturnType<typeof mount>): string[] {
  return wrapper.findAll('img').map((img) => img.attributes('src') ?? '')
}

describe('CompanionQAvatar', () => {
  it('renders the neutral base, irises and neutral mouth', () => {
    const wrapper = mount(CompanionQAvatar, { props: { expression: 'neutral' } })
    const rendered = srcs(wrapper)
    expect(rendered).toContain(COMPANION_Q.layers.base.src)
    expect(rendered).toContain(COMPANION_Q.layers.irisLeft.src)
    expect(rendered).toContain(COMPANION_Q.layers.irisRight.src)
    expect(rendered).toContain(COMPANION_Q.layers.mouths.neutral.src)
    expect(wrapper.attributes('data-expression')).toBe('neutral')
  })

  it('renders the >< happy patch without the open irises', () => {
    const wrapper = mount(CompanionQAvatar, { props: { expression: 'happy' } })
    const rendered = srcs(wrapper)
    expect(rendered).toContain(COMPANION_Q.layers.eyes.happy.src)
    expect(rendered).toContain(COMPANION_Q.layers.mouths.smile.src)
    expect(rendered).toContain(COMPANION_Q.layers.fx.sparkle.src)
    expect(rendered).not.toContain(COMPANION_Q.layers.irisLeft.src)
    expect(rendered).not.toContain(COMPANION_Q.layers.irisRight.src)
  })

  it('applies the dock viewport and leaves the full frame uncropped', () => {
    const dock = mount(CompanionQAvatar, { props: { frame: 'dock' } })
    const viewport = dock.find('.companion-q-avatar__viewport')
    const expectedWidth = (COMPANION_Q.canvas.width / COMPANION_Q.dockViewport.width) * 100
    expect(Number.parseFloat((viewport.element as HTMLElement).style.width)).toBeCloseTo(expectedWidth, 1)

    const full = mount(CompanionQAvatar, { props: { frame: 'full' } })
    expect((full.find('.companion-q-avatar__viewport').element as HTMLElement).style.width).toBe('100%')
  })

  it('disables iris movement and blinking when motion is reduced', () => {
    const wrapper = mount(CompanionQAvatar, {
      props: { expression: 'neutral', lookX: 1, lookY: -1, blinking: true, reducedMotion: true },
    })
    const irises = wrapper.findAll('.companion-q-avatar__iris')
    expect(irises.length).toBe(2)
    for (const iris of irises) {
      expect(iris.attributes('style') ?? '').not.toContain('translate')
    }
    expect(srcs(wrapper)).not.toContain(COMPANION_Q.layers.eyeClosed.src)
  })

  it('falls back to the open-eye layer when an iris image fails', async () => {
    const wrapper = mount(CompanionQAvatar, { props: { expression: 'neutral' } })
    const iris = wrapper
      .findAll('img')
      .find((img) => img.attributes('src') === COMPANION_Q.layers.irisLeft.src)
    expect(iris).toBeTruthy()
    await iris!.trigger('error')
    await nextTick()
    const rendered = srcs(wrapper)
    expect(rendered).toContain(COMPANION_Q.layers.eyeOpen.src)
    expect(rendered).not.toContain(COMPANION_Q.layers.irisLeft.src)
  })

  it('uses the closed-eye patch while blinking', () => {
    const wrapper = mount(CompanionQAvatar, { props: { expression: 'neutral', blinking: true } })
    const rendered = srcs(wrapper)
    expect(rendered).toContain(COMPANION_Q.layers.eyeClosed.src)
    expect(rendered).not.toContain(COMPANION_Q.layers.irisLeft.src)
  })
})
