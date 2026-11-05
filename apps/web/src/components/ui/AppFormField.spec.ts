import { mount } from '@vue/test-utils'
import { h } from 'vue'
import { describe, expect, it } from 'vitest'

import AppFormField from './AppFormField.vue'

interface FieldSlotProps {
  id: string
  invalid: boolean
  describedby: string | undefined
}

function renderInput(slot: FieldSlotProps) {
  return h('input', {
    id: slot.id,
    'aria-invalid': slot.invalid,
    'aria-describedby': slot.describedby,
  })
}

describe('AppFormField', () => {
  it('links the label and an error message to the control', () => {
    const wrapper = mount(AppFormField, {
      props: { label: '用户名', error: '请输入用户名', required: true },
      slots: { default: renderInput },
    })

    const input = wrapper.find('input')
    const error = wrapper.find('[role="alert"]')

    expect(wrapper.find('label').attributes('for')).toBe(input.attributes('id'))
    expect(wrapper.find('label').text()).toContain('用户名')
    expect(wrapper.find('label').text()).toContain('*')
    expect(input.attributes('aria-invalid')).toBe('true')
    expect(input.attributes('aria-describedby')).toBe(error.attributes('id'))
    expect(error.text()).toBe('请输入用户名')
  })

  it('shows an optional hint when there is no error', () => {
    const wrapper = mount(AppFormField, {
      props: { label: '昵称', hint: '可选，默认与用户名相同', optional: true },
      slots: { default: renderInput },
    })

    const input = wrapper.find('input')
    const hint = wrapper.find('.app-field__hint')

    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.find('label').text()).toContain('可选')
    expect(input.attributes('aria-invalid')).toBe('false')
    expect(input.attributes('aria-describedby')).toBe(hint.attributes('id'))
    expect(hint.text()).toBe('可选，默认与用户名相同')
  })
})
