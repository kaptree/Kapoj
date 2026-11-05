import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import type { RatingLevel } from '@/api/types'
import RatingLevelName from './RatingLevelName.vue'

const blackRed: RatingLevel = {
  key: 'black_red',
  name: '黑红名',
  color: '#000000',
  accentColor: '#FF0000',
}

const cyan: RatingLevel = {
  key: 'cyan',
  name: '青名',
  color: '#03A89E',
  accentColor: '',
}

describe('RatingLevelName', () => {
  it('renders first char black and rest red for black_red', () => {
    const wrapper = mount(RatingLevelName, { props: { name: '黑红名', level: blackRed } })

    expect(wrapper.text()).toBe('黑红名')
    expect(wrapper.classes()).toContain('rating-level-name--black-red')
    expect((wrapper.find('.rating-level-name__first').element as HTMLElement).style.color).toBe(
      'rgb(0, 0, 0)',
    )
    expect((wrapper.find('.rating-level-name__rest').element as HTMLElement).style.color).toBe(
      'rgb(255, 0, 0)',
    )
    expect(wrapper.attributes('title')).toBe('黑红名')
    expect(wrapper.attributes('aria-label')).toContain('黑红名')
  })

  it('renders regular level color', () => {
    const wrapper = mount(RatingLevelName, { props: { name: '青名', level: cyan } })

    expect(wrapper.text()).toBe('青名')
    expect((wrapper.element as HTMLElement).style.color).toBe('rgb(3, 168, 158)')
    expect(wrapper.find('.rating-level-name__first').exists()).toBe(false)
    expect(wrapper.attributes('title')).toBe('青名')
    expect(wrapper.attributes('aria-label')).toContain('青名')
  })

  it('falls back to ? for an empty name', () => {
    const wrapper = mount(RatingLevelName, { props: { name: '', level: blackRed } })

    expect(wrapper.text()).toBe('?')
    expect(wrapper.attributes('title')).toBe('黑红名')
  })
})
