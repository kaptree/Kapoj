import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import AppDifficultyTag from './AppDifficultyTag.vue'

describe('AppDifficultyTag', () => {
  it('renders every difficulty level with its own tone', () => {
    for (const [level, label] of ['入门', '普及', '提高', '省选', 'NOI'].entries()) {
      const wrapper = mount(AppDifficultyTag, { props: { difficulty: level + 1 } })

      expect(wrapper.text()).toBe(label)
      expect(wrapper.classes()).toContain(`app-difficulty-tag--${level + 1}`)
    }
  })

  it('falls back to the unknown tone for invalid values', () => {
    for (const difficulty of [0, 6, -1, 2.5, Number.NaN]) {
      const wrapper = mount(AppDifficultyTag, { props: { difficulty } })

      expect(wrapper.text()).toBe('未知')
      expect(wrapper.classes()).toContain('app-difficulty-tag--unknown')
    }
  })
})
