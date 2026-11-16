import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import CompanionNote from './CompanionNote.vue'

describe('CompanionNote', () => {
  it('renders a chibi, line and close button', async () => {
    const wrapper = mount(CompanionNote, { props: { line: '先读题。别急着写。' } })
    expect(wrapper.text()).toContain('先读题')
    expect(wrapper.find('[data-testid="companion-placeholder"]').exists()).toBe(true)
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('dismiss')).toHaveLength(1)
  })

  it('can hide the close button', () => {
    const wrapper = mount(CompanionNote, { props: { line: '专注点。', dismissible: false } })
    expect(wrapper.find('button').exists()).toBe(false)
  })
})
