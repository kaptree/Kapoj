import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import { COMPANION_OUTFITS } from '@/content/companion/manifest'
import CompanionWardrobe from './CompanionWardrobe.vue'

function mountWardrobe(overrides: Record<string, unknown> = {}) {
  return mount(CompanionWardrobe, {
    props: {
      outfits: COMPANION_OUTFITS,
      equippedSkin: 'outfit-1',
      unlockedSkins: ['outfit-1', 'outfit-2', 'outfit-3'],
      level: 2,
      ...overrides,
    },
  })
}

describe('CompanionWardrobe', () => {
  it('renders five outfits in order and locks the rest', () => {
    const wrapper = mountWardrobe()
    const cards = wrapper.findAll('[data-test^="wardrobe-outfit-"]')
    expect(cards).toHaveLength(5)
    expect(cards.map((card) => card.attributes('data-test'))).toEqual(
      COMPANION_OUTFITS.map((outfit) => `wardrobe-outfit-${outfit.id}`),
    )
    expect(
      wrapper.get('[data-test="wardrobe-outfit-outfit-1"]').attributes('disabled'),
    ).toBeUndefined()
    expect(wrapper.get('[data-test="wardrobe-outfit-outfit-4"]').text()).toContain('Lv.4 解锁')
    expect(
      wrapper.get('[data-test="wardrobe-outfit-outfit-4"]').attributes('disabled'),
    ).toBeDefined()
  })

  it('emits equip for an unlocked outfit', async () => {
    const wrapper = mountWardrobe()
    await wrapper.get('[data-test="wardrobe-outfit-outfit-3"]').trigger('click')
    expect(wrapper.emitted('equip')?.[0]).toEqual(['outfit-3'])
  })

  it('marks the equipped outfit and previews it', () => {
    const wrapper = mountWardrobe({ equippedSkin: 'outfit-3' })
    const card = wrapper.get('[data-test="wardrobe-outfit-outfit-3"]')
    expect(card.attributes('aria-current')).toBe('true')
    expect(card.text()).toContain('已穿戴')
    expect(wrapper.get('.companion-wardrobe__preview-image').attributes('src')).toBe(
      '/companion/outfits/outfit-3.webp',
    )
  })

  it('falls back to outfit-1 for an unknown equipped id', () => {
    const wrapper = mountWardrobe({ equippedSkin: 'missing' })
    expect(wrapper.get('.companion-wardrobe__preview-image').attributes('src')).toBe(
      '/companion/outfits/outfit-1.webp',
    )
    expect(wrapper.get('[data-test="wardrobe-outfit-outfit-1"]').attributes('aria-current')).toBe(
      'true',
    )
  })
})

describe('CompanionWardrobe image fallbacks', () => {
  it('falls back to outfit-1 when the preview 404s', async () => {
    const wrapper = mountWardrobe({ equippedSkin: 'outfit-3' })
    const preview = wrapper.get('.companion-wardrobe__preview-image')
    await preview.trigger('error')
    expect(wrapper.get('.companion-wardrobe__preview-image').attributes('src')).toBe(
      '/companion/outfits/outfit-1.webp',
    )
  })

  it('falls back to the outfit-1 card when a thumbnail 404s', async () => {
    const wrapper = mountWardrobe()
    const card = wrapper.get('[data-test="wardrobe-outfit-outfit-3"]')
    const thumb = card.get('img')
    await thumb.trigger('error')
    expect(card.get('img').attributes('src')).toBe('/companion/outfits/outfit-1-card.webp')
  })
})
