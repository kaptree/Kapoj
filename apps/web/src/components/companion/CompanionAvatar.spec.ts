import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { CompanionAsset } from '../../content/companion/types'
import CompanionAvatar from './CompanionAvatar.vue'

const art = vi.hoisted(() => ({ asset: null as CompanionAsset | null }))

vi.mock('../../content/companion/manifest', () => ({
  resolveCompanionAsset: () => art.asset,
}))

describe('CompanionAvatar', () => {
  beforeEach(() => {
    art.asset = null
  })

  it('renders the inline placeholder when no art pack is available', () => {
    const wrapper = mount(CompanionAvatar, { props: { pose: 'bust', expression: 'pout' } })
    expect(wrapper.find('[data-testid="companion-placeholder"]').exists()).toBe(true)
    expect(wrapper.find('img').exists()).toBe(false)
  })

  it('applies pose and size classes', () => {
    const wrapper = mount(CompanionAvatar, {
      props: { pose: 'full', size: 'lg', expression: 'smug' },
    })
    expect(wrapper.classes()).toContain('companion-avatar--full')
    expect(wrapper.classes()).toContain('companion-avatar--lg')
  })

  it('reserves the aspect ratio of the current pose', () => {
    const wrapper = mount(CompanionAvatar, { props: { pose: 'bust' } })
    expect(wrapper.attributes('style')).toContain('aspect-ratio: 6 / 7')
  })

  it('keeps the placeholder decorative for assistive technology', () => {
    const wrapper = mount(CompanionAvatar)
    const placeholder = wrapper.find('[data-testid="companion-placeholder"]')
    expect(placeholder.attributes('aria-hidden')).toBe('true')
    expect(placeholder.attributes('focusable')).toBe('false')
  })

  it('renders the art with fixed dimensions and lazy loading when available', () => {
    art.asset = { src: '/mascot/chibi/neutral@2x.webp', width: 512, height: 512 }
    const wrapper = mount(CompanionAvatar, { props: { pose: 'chibi', alt: 'Kapo' } })
    const image = wrapper.find('img')
    expect(image.attributes()).toMatchObject({
      src: '/mascot/chibi/neutral@2x.webp',
      width: '512',
      height: '512',
      alt: 'Kapo',
      loading: 'lazy',
      decoding: 'async',
    })
    expect(wrapper.find('[data-testid="companion-placeholder"]').exists()).toBe(false)
  })

  it('loads the art eagerly when asked', () => {
    art.asset = { src: '/mascot/chibi/neutral@2x.webp', width: 512, height: 512 }
    const wrapper = mount(CompanionAvatar, { props: { eager: true } })
    expect(wrapper.find('img').attributes('loading')).toBe('eager')
  })

  it('falls back to the placeholder after a broken image and recovers for new art', async () => {
    art.asset = { src: '/mascot/chibi/neutral@2x.webp', width: 512, height: 512 }
    const wrapper = mount(CompanionAvatar, { props: { expression: 'neutral' } })
    await wrapper.find('img').trigger('error')
    expect(wrapper.find('[data-testid="companion-placeholder"]').exists()).toBe(true)
    expect(wrapper.find('img').exists()).toBe(false)

    art.asset = { src: '/mascot/chibi/pout@2x.webp', width: 512, height: 512 }
    await wrapper.setProps({ expression: 'pout' })
    expect(wrapper.find('img').attributes('src')).toBe('/mascot/chibi/pout@2x.webp')
  })
})
