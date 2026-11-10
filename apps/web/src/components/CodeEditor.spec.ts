import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import CodeEditor from './CodeEditor.vue'

const editor = vi.hoisted(() => ({ createCodeEditor: vi.fn() }))
vi.mock('@/editor', () => ({ createCodeEditor: editor.createCodeEditor }))

describe('CodeEditor', () => {
  beforeEach(() => {
    editor.createCodeEditor.mockReset()
  })

  it('creates the editor, syncs props and disposes it', async () => {
    const handle = {
      getValue: vi.fn(() => 'initial'),
      setValue: vi.fn(),
      setLanguage: vi.fn(),
      getCursorByteOffset: vi.fn(() => 3),
      insertAtCursor: vi.fn(),
      focus: vi.fn(),
      destroy: vi.fn(),
    }
    editor.createCodeEditor.mockReturnValue(handle)
    const wrapper = mount(CodeEditor, { props: { modelValue: 'initial', language: 'cpp' } })
    const opts = editor.createCodeEditor.mock.calls[0]![1] as {
      value: string
      language: string
      readonly: boolean
      onChange: (value: string) => void
    }
    expect(opts).toMatchObject({ value: 'initial', language: 'cpp', readonly: false })
    opts.onChange('changed')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['changed'])

    await wrapper.setProps({ modelValue: 'outside' })
    expect(handle.setValue).toHaveBeenCalledWith('outside')

    await wrapper.setProps({ language: 'python' })
    expect(handle.setLanguage).toHaveBeenCalledWith('python')

    const vm = wrapper.vm as unknown as {
      getCursorByteOffset: () => number
      insertAtCursor: (t: string) => void
    }
    expect(vm.getCursorByteOffset()).toBe(3)
    vm.insertAtCursor('x')
    expect(handle.insertAtCursor).toHaveBeenCalledWith('x')

    wrapper.unmount()
    expect(handle.destroy).toHaveBeenCalled()
  })

  it('does not push an unchanged modelValue back into the editor', async () => {
    const handle = {
      getValue: vi.fn(() => 'same'),
      setValue: vi.fn(),
      setLanguage: vi.fn(),
      getCursorByteOffset: vi.fn(() => 0),
      insertAtCursor: vi.fn(),
      focus: vi.fn(),
      destroy: vi.fn(),
    }
    editor.createCodeEditor.mockReturnValue(handle)
    const wrapper = mount(CodeEditor, { props: { modelValue: 'same' } })

    await wrapper.setProps({ modelValue: 'same' })

    expect(handle.setValue).not.toHaveBeenCalled()
  })

  it('falls back to the UTF-8 byte length when the editor is unavailable', () => {
    editor.createCodeEditor.mockReturnValue(undefined)
    const wrapper = mount(CodeEditor, { props: { modelValue: '中a' } })

    const vm = wrapper.vm as unknown as { getCursorByteOffset: () => number }
    expect(vm.getCursorByteOffset()).toBe(4)
  })
})
