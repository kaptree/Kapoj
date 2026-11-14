import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it } from 'vitest'

import type { EquippedBadge, EquippedTitle, RatingLevel } from '@/api/types'
import UserIdentityName from './UserIdentityName.vue'

const title: EquippedTitle = { id: 3, name: '卷王', color: '#7C3AED' }
const badges: EquippedBadge[] = [
  { id: 1, name: '早鸟', imageUrl: '' },
  { id: 2, name: '探索者', imageUrl: '/api/badges/2/image' },
]
const level: RatingLevel = {
  key: 'cyan',
  name: '青名',
  color: '#03A89E',
  accentColor: '',
}

function mountName(props: {
  nickname?: string
  username?: string
  title?: EquippedTitle | null
  badges?: EquippedBadge[]
  size?: 'small' | 'default'
  ratingLevel?: RatingLevel | null
  linkTo?: string
}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  return mount(UserIdentityName, {
    props: {
      nickname: 'Alice',
      title,
      badges,
      ...props,
    },
    global: {
      plugins: [pinia],
      stubs: {
        RouterLink: {
          props: ['to'],
          template: '<a :href="String(to)"><slot /></a>',
        },
      },
    },
  })
}

describe('UserIdentityName', () => {
  it('puts the bracketed title before the nickname and badges after it', () => {
    const wrapper = mountName({})

    expect(wrapper.find('.user-identity-name__title').text()).toBe('【卷王】')
    expect(wrapper.find('.user-identity-name__name').text()).toBe('Alice')
    expect((wrapper.find('.user-identity-name__title').element as HTMLElement).style.color).toBe(
      'rgb(124, 58, 237)',
    )
    expect(wrapper.findAll('.identity-badge')).toHaveLength(2)
  })

  it('omits the brackets when no title is equipped', () => {
    const wrapper = mountName({ title: null })

    expect(wrapper.text()).not.toContain('【】')
    expect(wrapper.text()).toContain('Alice')
  })

  it('keeps the rating level color on the nickname while the title uses its own color', () => {
    const wrapper = mountName({ ratingLevel: level })

    expect(wrapper.find('.rating-level-name').exists()).toBe(true)
    expect((wrapper.find('.rating-level-name').element as HTMLElement).style.color).toBe(
      'rgb(3, 168, 158)',
    )
    expect((wrapper.find('.user-identity-name__title').element as HTMLElement).style.color).toBe(
      'rgb(124, 58, 237)',
    )
  })

  it('links the nickname to the user profile', () => {
    const wrapper = mountName({ linkTo: '/users/2' })

    expect(wrapper.find('a.user-identity-name__link').attributes('href')).toBe('/users/2')
  })

  it('falls back to the username and supports compact size', () => {
    const wrapper = mountName({ nickname: '', username: 'alice', size: 'small' })

    expect(wrapper.text()).toContain('alice')
    expect(wrapper.find('.identity-showcase').classes()).toContain('identity-showcase--small')
  })
})
