import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import CompanionPersonalityPanel from './CompanionPersonalityPanel.vue'

function mountPanel(overrides: Record<string, unknown> = {}) {
  return mount(CompanionPersonalityPanel, {
    props: {
      personalityPrompt: '再温柔一点',
      aiDialogueEnabled: true,
      aiReady: true,
      remainingToday: 27,
      ...overrides,
    },
    global: {
      stubs: {
        RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
      },
    },
  })
}

describe('CompanionPersonalityPanel', () => {
  it('renders the current prompt, count and AI quota', () => {
    const wrapper = mountPanel()
    expect((wrapper.get('[data-test="personality-prompt"]').element as HTMLTextAreaElement).value).toBe(
      '再温柔一点',
    )
    expect(wrapper.get('[data-test="personality-count"]').text()).toBe('5/500')
    expect(wrapper.text()).toContain('今日剩余 27 次')
  })

  it('restores the default prompt and emits save', async () => {
    const wrapper = mountPanel()
    await wrapper.get('[data-test="personality-reset"]').trigger('click')

    expect((wrapper.get('[data-test="personality-prompt"]').element as HTMLTextAreaElement).value).toBe('')
    expect(wrapper.emitted('reset')).toHaveLength(1)
    expect(wrapper.emitted('save')?.[0]).toEqual([
      { personalityPrompt: '', aiDialogueEnabled: true },
    ])
  })

  it('trims the prompt and emits the AI toggle value', async () => {
    const wrapper = mountPanel()
    const textarea = wrapper.get('[data-test="personality-prompt"]')
    await textarea.setValue('  更温柔一点  ')
    await wrapper.get('[data-test="ai-dialogue-toggle"]').trigger('click')
    await wrapper.get('[data-test="personality-save"]').trigger('click')

    expect(wrapper.emitted('save')?.[0]).toEqual([
      { personalityPrompt: '更温柔一点', aiDialogueEnabled: false },
    ])
  })

  it('disables the AI toggle and links to AI settings when no model is configured', () => {
    const wrapper = mountPanel({ aiReady: false, aiDialogueEnabled: false })
    expect(wrapper.get('[data-test="ai-dialogue-toggle"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('a[href="/settings/ai"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('尚未接入 AI')
  })

  it('rejects a prompt over 500 characters instead of emitting save', async () => {
    const wrapper = mountPanel()
    const textarea = wrapper.get('[data-test="personality-prompt"]')
    ;(textarea.element as HTMLTextAreaElement).value = '字'.repeat(501)
    await textarea.trigger('input')
    await wrapper.get('[data-test="personality-save"]').trigger('click')

    expect(wrapper.text()).toContain('不能超过 500 个字符')
    expect(wrapper.emitted('save')).toBeUndefined()
  })
})
