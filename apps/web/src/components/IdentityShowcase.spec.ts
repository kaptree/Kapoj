import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus, { ElAvatar, ElTooltip } from 'element-plus'

import type { EquippedBadge, EquippedTitle } from '@/api/types'
import IdentityShowcase from './IdentityShowcase.vue'

function mountShowcase(props: {
  badges: EquippedBadge[]
  title: EquippedTitle | null
  size?: 'small' | 'default'
}) {
  return mount(IdentityShowcase, {
    props,
    global: { plugins: [ElementPlus] },
  })
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

    expect(wrapper.find('.identity-title').text()).toBe('卷王')
    expect((wrapper.find('.identity-title').element as HTMLElement).style.color).toBe(
      'rgb(124, 58, 237)',
    )

    const badges = wrapper.findAll('.identity-badge')
    expect(badges).toHaveLength(2)
    expect(badges[0]!.text()).toContain('早')
    expect(badges[1]!.find('img').attributes('src')).toBe('/api/badges/2/image')

    const tooltips = wrapper.findAllComponents(ElTooltip)
    expect(tooltips.map((tooltip) => tooltip.props('content'))).toEqual(['早鸟', '探索者'])
  })

  it('renders nothing for empty badges and null title', () => {
    const wrapper = mountShowcase({ badges: [], title: null })

    expect(wrapper.find('.identity-showcase').exists()).toBe(false)
    expect(wrapper.findAll('.identity-badge')).toHaveLength(0)
    expect(wrapper.text()).toBe('')
    expect(wrapper.findAll('span')).toHaveLength(0)
  })

  it('uses fallback color for invalid title color', () => {
    const wrapper = mountShowcase({
      badges: [],
      title: { id: 1, name: '头衔', color: 'red' },
    })

    expect((wrapper.find('.identity-title').element as HTMLElement).style.color).toBe(
      'rgb(48, 49, 51)',
    )
  })

  it('renders small badges in compact size', () => {
    const wrapper = mountShowcase({
      badges: [{ id: 1, name: '早鸟', imageUrl: '' }],
      title: null,
      size: 'small',
    })

    expect(wrapper.findComponent(ElAvatar).props('size')).toBe(18)
  })
})
