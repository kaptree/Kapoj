import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import AppDifficultyTag from './AppDifficultyTag.vue'

describe('AppDifficultyTag', () => {
  it('renders the score with its band tone', () => {
    for (const [score, band] of [
      [1, 1],
      [20, 1],
      [21, 2],
      [62, 4],
      [100, 5],
    ] as const) {
      const wrapper = mount(AppDifficultyTag, { props: { difficulty: score } })

      expect(wrapper.text()).toBe(String(score))
      expect(wrapper.classes()).toContain(`app-difficulty-tag--${band}`)
    }
  })

  it('shows the score and band label with showLabel', () => {
    const wrapper = mount(AppDifficultyTag, { props: { difficulty: 62, showLabel: true } })

    expect(wrapper.text()).toBe('62 · 省选')
    expect(wrapper.classes()).toContain('app-difficulty-tag--4')
  })

  it('falls back to the unknown tone for invalid values', () => {
    for (const difficulty of [0, 101, -1, 2.5, Number.NaN]) {
      const wrapper = mount(AppDifficultyTag, { props: { difficulty } })

      expect(wrapper.text()).toBe('未知')
      expect(wrapper.classes()).toContain('app-difficulty-tag--unknown')
    }
  })

  it('falls back to unknown even when showLabel is set', () => {
    const wrapper = mount(AppDifficultyTag, { props: { difficulty: 0, showLabel: true } })

    expect(wrapper.text()).toBe('未知')
    expect(wrapper.classes()).toContain('app-difficulty-tag--unknown')
  })
})
