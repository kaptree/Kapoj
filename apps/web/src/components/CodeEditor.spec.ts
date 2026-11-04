import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import CodeEditor from './CodeEditor.vue'

const mocks = vi.hoisted(() => ({
  create: vi.fn(),
  setModelLanguage: vi.fn(),
}))

vi.mock('@/monaco', () => ({
  monaco: {
    editor: {
      create: mocks.create,
      setModelLanguage: mocks.setModelLanguage,
    },
  },
}))

describe('CodeEditor', () => {
  it('creates, updates, changes language and disposes the editor', async () => {
    let changeHandler: (() => void) | undefined
    const getValue = vi.fn(() => 'initial')
    const setValue = vi.fn()
    const dispose = vi.fn()
    const fakeEditor = {
      getValue,
      setValue,
      getModel: vi.fn(() => ({})),
      onDidChangeModelContent: vi.fn((cb: () => void) => {
        changeHandler = cb
      }),
      dispose,
    }
    mocks.create.mockReturnValue(fakeEditor)

    const wrapper = mount(CodeEditor, {
      props: { modelValue: 'initial', language: 'cpp' },
    })
    expect(mocks.create).toHaveBeenCalled()
    const options = mocks.create.mock.calls[0]![1] as Record<string, unknown>
    expect(options.language).toBe('cpp')
    expect(options.readOnly).toBe(false)

    getValue.mockReturnValue('changed')
    changeHandler?.()
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['changed'])

    await wrapper.setProps({ modelValue: 'outside' })
    expect(setValue).toHaveBeenCalledWith('outside')

    await wrapper.setProps({ language: 'python' })
    expect(mocks.setModelLanguage).toHaveBeenCalledWith({}, 'python')

    wrapper.unmount()
    expect(dispose).toHaveBeenCalled()
  })
})
