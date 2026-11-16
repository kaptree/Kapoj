import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import CompanionBubble from './CompanionBubble.vue'

describe('CompanionBubble', () => {
  it('renders the line and tone', () => {
    const wrapper = mount(CompanionBubble, { props: { text: '又 WA 了？', tone: 'tsun' } })
    expect(wrapper.text()).toContain('又 WA 了？')
    expect(wrapper.classes()).toContain('companion-bubble--tsun')
  })

  it('defaults to the tsun tone', () => {
    const wrapper = mount(CompanionBubble, { props: { text: '再来一次。' } })
    expect(wrapper.classes()).toContain('companion-bubble--tsun')
  })

  it('supports a neutral tone', () => {
    const wrapper = mount(CompanionBubble, { props: { text: '记录好了。', tone: 'neutral' } })
    expect(wrapper.classes()).toContain('companion-bubble--neutral')
    expect(wrapper.classes()).not.toContain('companion-bubble--tsun')
  })
})
