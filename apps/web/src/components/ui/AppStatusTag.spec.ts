import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import type { SubmissionStatus } from '@/api/types'
import AppStatusTag from './AppStatusTag.vue'

describe('AppStatusTag', () => {
  it('renders the Chinese label and a status tone', () => {
    const wrapper = mount(AppStatusTag, { props: { status: 'accepted' } })

    expect(wrapper.text()).toBe('通过')
    expect(wrapper.attributes('data-status')).toBe('accepted')
    expect(wrapper.classes()).toContain('app-status-tag--accepted')
  })

  it('uses distinct tones for the judge results', () => {
    const cases: { status: SubmissionStatus; tone: string }[] = [
      { status: 'wrong_answer', tone: 'wrong-answer' },
      { status: 'time_limit_exceeded', tone: 'tle' },
      { status: 'memory_limit_exceeded', tone: 'mle' },
      { status: 'runtime_error', tone: 'runtime-error' },
      { status: 'compile_error', tone: 'compile-error' },
      { status: 'system_error', tone: 'system-error' },
    ]

    for (const { status, tone } of cases) {
      const wrapper = mount(AppStatusTag, { props: { status } })
      expect(wrapper.classes()).toContain(`app-status-tag--${tone}`)
    }
  })

  it('marks judging so the dot can pulse', () => {
    const wrapper = mount(AppStatusTag, { props: { status: 'judging' } })

    expect(wrapper.text()).toBe('评测中')
    expect(wrapper.classes()).toContain('app-status-tag--judging')
    expect(wrapper.find('.app-status-tag__dot').exists()).toBe(true)
  })

  it('accepts a custom label', () => {
    const wrapper = mount(AppStatusTag, { props: { status: 'pending', label: '排队中' } })

    expect(wrapper.text()).toBe('排队中')
  })
})
