import { mount, type VueWrapper } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { describe, expect, it } from 'vitest'

import type { Sample } from '@/api/types'
import SampleEditor from './SampleEditor.vue'

function mountEditor(modelValue: Sample[]) {
  return mount(SampleEditor, {
    props: { modelValue },
    global: { plugins: [PrimeVue] },
  })
}

function buttonByText(wrapper: VueWrapper, text: string) {
  return wrapper.findAll('button').find((button) => button.text().trim() === text)
}

describe('SampleEditor', () => {
  it('renders empty hint and add button', () => {
    const wrapper = mountEditor([])

    expect(wrapper.text()).toContain('还没有样例。')
    expect(buttonByText(wrapper, '添加样例')).toBeTruthy()
  })

  it('adds a sample and emits update:modelValue', async () => {
    const wrapper = mountEditor([])

    await buttonByText(wrapper, '添加样例')!.trigger('click')

    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual([{ input: '', output: '' }])
  })

  it('edits input and output', async () => {
    const wrapper = mountEditor([{ input: '', output: '' }])

    const textareas = wrapper.findAll('textarea')
    expect(textareas).toHaveLength(2)
    for (const textarea of textareas) expect(textarea.classes()).toContain('mono')

    await textareas[0]!.setValue('1 2')
    await textareas[1]!.setValue('3')

    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual([
      { input: '1 2', output: '3' },
    ])
  })

  it('removes a sample', async () => {
    const wrapper = mountEditor([
      { input: '1', output: '2' },
      { input: '3', output: '4' },
    ])

    const deleteButtons = wrapper.findAll('button').filter((b) => b.text().trim() === '删除')
    await deleteButtons[0]!.trigger('click')

    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual([{ input: '3', output: '4' }])
  })

  it('caps samples at ten', () => {
    const samples = Array.from({ length: 10 }, () => ({ input: '', output: '' }))
    const wrapper = mountEditor(samples)

    const addButton = buttonByText(wrapper, '添加样例')
    expect((addButton!.element as HTMLButtonElement).disabled).toBe(true)
    expect(wrapper.text()).toContain('最多 10 组样例。')
  })
})
