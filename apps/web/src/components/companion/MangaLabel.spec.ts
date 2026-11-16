import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import MangaLabel from './MangaLabel.vue'

describe('MangaLabel', () => {
  it('renders the index, label and decorative kana', () => {
    const wrapper = mount(MangaLabel, {
      props: { index: '02', label: '学习概览', kana: '学習記録' },
    })
    expect(wrapper.text()).toContain('02')
    expect(wrapper.text()).toContain('学习概览')
    expect(wrapper.find('.manga-label__kana').attributes('aria-hidden')).toBe('true')
  })
})
