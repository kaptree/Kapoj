import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import type { UserCompanion } from '@/api/types'
import { pickCompanionLine } from '@/content/companion/lines.zh-CN'
import CompanionAvatar from './CompanionAvatar.vue'
import CompanionProfileCard from './CompanionProfileCard.vue'

const base: UserCompanion = {
  userId: 7,
  bond: 120,
  level: 3,
  levelName: '搭档',
  nextThreshold: 170,
  equippedSkin: 'culture-festival',
  unlockedSkins: ['culture-festival', 'midnight'],
  publicProfile: true,
  daysTogether: 42,
}

const RouterLinkStub = {
  name: 'RouterLink',
  props: ['to'],
  template: '<a :href="String(to)"><slot /></a>',
}

function mountCard(overrides: Partial<typeof base> = {}, isSelf = false) {
  return mount(CompanionProfileCard, {
    props: { state: { ...base, ...overrides }, isSelf, name: 'Kapo' },
    global: { stubs: { RouterLink: RouterLinkStub } },
  })
}

describe('CompanionProfileCard', () => {
  it('renders the companion name, level, bond progress, days and skins', () => {
    const wrapper = mountCard()

    expect(wrapper.find('[data-test="companion-profile-card"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Kapo')
    expect(wrapper.text()).toContain('Lv.3 搭档')
    expect(wrapper.text()).toContain('120 / 170')
    expect(wrapper.text()).toContain('相伴 42 天')
    expect(wrapper.text()).toContain('culture-festival')
    expect(wrapper.text()).toContain('midnight')
    expect(wrapper.text()).not.toContain('还没有换装')

    const avatar = wrapper.getComponent(CompanionAvatar)
    expect(avatar.props('pose')).toBe('bust')
    expect(avatar.props('size')).toBe('md')
    expect(avatar.props('alt')).toBe('')

    const bar = wrapper.get('[role="progressbar"]')
    expect(bar.attributes('aria-valuenow')).toBe('120')
    expect(bar.attributes('aria-valuemax')).toBe('170')
    expect(wrapper.get('.companion-profile-card__fill').attributes('style')).toContain(
      'width: 70.588',
    )
  })

  it('shows an empty skin state', () => {
    const wrapper = mountCard({ unlockedSkins: [] })

    expect(wrapper.text()).toContain('还没有换装')
  })

  it('renders the most frequent event as a quote', () => {
    const wrapper = mountCard({ favoriteEventType: 'ac_first' })
    const expected = pickCompanionLine('ac', '7:ac_first')

    expect(wrapper.get('.companion-profile-card__quote').text()).toContain(expected)
  })

  it('shows a max-level label instead of an overshooting fraction', () => {
    const wrapper = mountCard({ level: 6, bond: 400, nextThreshold: 380 })

    expect(wrapper.text()).toContain('已满级')
    expect(wrapper.text()).not.toContain('400 / 380')
    const bar = wrapper.get('[role="progressbar"]')
    expect(bar.attributes('aria-valuenow')).toBe('380')
    expect(bar.attributes('aria-valuemax')).toBe('380')
  })

  it('shows the private hint and appearance link only on your own hidden profile', () => {
    const privateCard = mountCard({ publicProfile: false }, true)
    expect(privateCard.text()).toContain('仅自己可见')
    expect(privateCard.find('a[href="/appearance"]').exists()).toBe(true)

    const publicCard = mountCard({ publicProfile: true }, true)
    expect(publicCard.text()).not.toContain('仅自己可见')
    expect(publicCard.find('a[href="/appearance"]').exists()).toBe(false)

    const visitorCard = mountCard({ publicProfile: false }, false)
    expect(visitorCard.text()).not.toContain('仅自己可见')
  })

  it('never divides by a zero threshold', () => {
    const wrapper = mountCard({ bond: 0, nextThreshold: 0 })

    expect(wrapper.get('.companion-profile-card__fill').attributes('style')).toContain('width: 0%')
    const bar = wrapper.get('[role="progressbar"]')
    expect(bar.attributes('aria-valuemax')).toBe('0')
    expect(wrapper.text()).toContain('0 / 0')
  })

  it('renders nothing without a companion state', () => {
    const wrapper = mount(CompanionProfileCard, {
      props: { state: null, isSelf: true, name: 'Kapo' },
      global: { stubs: { RouterLink: RouterLinkStub } },
    })

    expect(wrapper.find('[data-test="companion-profile-card"]').exists()).toBe(false)
    expect(wrapper.text()).toBe('')
  })
})
