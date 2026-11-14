import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it } from 'vitest'

import type { EquippedBadge, EquippedTitle, SitePublicView } from '@/api/types'
import { useSiteStore } from '@/stores/site'
import IdentityShowcase from './IdentityShowcase.vue'

function mountShowcase(
  props: {
    badges: EquippedBadge[]
    title: EquippedTitle | null
    size?: 'small' | 'default'
  },
  pinia = createPinia(),
) {
  setActivePinia(pinia)
  return mount(IdentityShowcase, { props, global: { plugins: [pinia] } })
}

describe('IdentityShowcase', () => {
  it('renders equipped badges and title', () => {
    const wrapper = mountShowcase({
      badges: [
        { id: 1, name: '早鸟', imageUrl: '' },
        { id: 2, name: '探索者', imageUrl: '/api/badges/2/image' },
      ],
      title: { id: 3, name: '卷王', color: '#7C3AED' },
    })

    expect(wrapper.find('.identity-title').text()).toBe('【卷王】')
    expect((wrapper.find('.identity-title').element as HTMLElement).style.color).toBe(
      'rgb(124, 58, 237)',
    )

    const badges = wrapper.findAll('.identity-badge')
    expect(badges).toHaveLength(2)
    expect(badges[0]!.text()).toContain('早')
    expect(badges[1]!.find('img').attributes('src')).toBe('/api/badges/2/image')
    expect(badges[0]!.find('.identity-badge__avatar').attributes('aria-label')).toBe('早鸟')
    expect(badges[1]!.find('.identity-badge__avatar').attributes('aria-label')).toBe('探索者')
  })

  it('falls back to the initial when a badge image fails to load', async () => {
    const wrapper = mountShowcase({
      badges: [{ id: 2, name: '探索者', imageUrl: '/api/badges/2/image' }],
      title: null,
    })

    expect(wrapper.find('img').exists()).toBe(true)
    await wrapper.find('img').trigger('error')

    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('.identity-badge').text()).toContain('探')
  })

  it('renders nothing for empty badges and null title', () => {
    const wrapper = mountShowcase({ badges: [], title: null })

    expect(wrapper.find('.identity-showcase').exists()).toBe(false)
    expect(wrapper.findAll('.identity-badge')).toHaveLength(0)
    expect(wrapper.text()).toBe('')
    expect(wrapper.findAll('span')).toHaveLength(0)
  })

  it('uses the design-token fallback for invalid title color', () => {
    const wrapper = mountShowcase({
      badges: [],
      title: { id: 1, name: '头衔', color: 'red' },
    })

    expect((wrapper.find('.identity-title').element as HTMLElement).style.color).toBe(
      'var(--kapoj-text)',
    )
  })

  it('renders small badges in compact size', () => {
    const wrapper = mountShowcase({
      badges: [{ id: 1, name: '早鸟', imageUrl: '' }],
      title: null,
      size: 'small',
    })
    const avatar = wrapper.find('.identity-badge__avatar').element as HTMLElement

    expect(wrapper.find('.identity-showcase').classes()).toContain('identity-showcase--small')
    expect(avatar.style.width).toBe('18px')
    expect(avatar.style.height).toBe('18px')
  })

  it('uses the badge size from the site settings', () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    useSiteStore(pinia).setConfig({
      appearance: { badgeSize: 40 },
    } as unknown as SitePublicView)

    const wrapper = mountShowcase(
      {
        badges: [{ id: 1, name: '早鸟', imageUrl: '' }],
        title: null,
      },
      pinia,
    )
    const avatar = wrapper.find('.identity-badge__avatar').element as HTMLElement
    expect(avatar.style.width).toBe('40px')
    expect(avatar.style.height).toBe('40px')

    const small = mountShowcase(
      {
        badges: [{ id: 1, name: '早鸟', imageUrl: '' }],
        title: null,
        size: 'small',
      },
      pinia,
    )
    const smallAvatar = small.find('.identity-badge__avatar').element as HTMLElement
    expect(smallAvatar.style.width).toBe('36px')
    expect(smallAvatar.style.height).toBe('36px')
  })
})
