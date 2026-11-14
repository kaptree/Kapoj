import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Textarea from 'primevue/textarea'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { createOnlineTestcase, getTestcaseContent, updateTestcaseContent } from '@/api/problems'
import type { Testcase } from '@/api/types'
import TestcaseEditDialog from './TestcaseEditDialog.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/problems', () => ({
  getTestcaseContent: vi.fn(),
  updateTestcaseContent: vi.fn(),
  createOnlineTestcase: vi.fn(),
}))

function testcase(overrides: Partial<Testcase> = {}): Testcase {
  return {
    id: 3,
    sortOrder: 1,
    score: 10,
    isSample: false,
    inputSize: 5,
    outputSize: 3,
    ...overrides,
  }
}

function mountDialog(props: { visible?: boolean; problemId?: number; testcase?: Testcase | null }) {
  return mount(TestcaseEditDialog, {
    props: {
      visible: true,
      problemId: 1,
      testcase: null,
      ...props,
    },
    global: { plugins: [PrimeVue] },
    attachTo: document.body,
  })
}

function textareas(wrapper: ReturnType<typeof mountDialog>) {
  return wrapper.findAllComponents(Textarea)
}

function dialogButton(label: string) {
  const buttons = Array.from(document.querySelectorAll('.p-dialog button'))
  const found = buttons.find((b) => b.textContent?.trim().includes(label))
  if (!found) throw new Error(`dialog button ${label} not found`)
  return found as HTMLElement
}

beforeEach(() => {
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(getTestcaseContent).mockReset()
  vi.mocked(updateTestcaseContent).mockReset()
  vi.mocked(createOnlineTestcase).mockReset()
  vi.mocked(getTestcaseContent).mockResolvedValue({
    input: '1 2',
    output: '3',
    inputSize: 3,
    outputSize: 1,
    score: 10,
    isSample: false,
  })
  vi.mocked(updateTestcaseContent).mockResolvedValue(null)
  vi.mocked(createOnlineTestcase).mockResolvedValue(testcase({ id: 9 }))
})

describe('TestcaseEditDialog', () => {
  it('loads and saves an existing testcase', async () => {
    const wrapper = mountDialog({ testcase: testcase() })
    await flushPromises()

    expect(getTestcaseContent).toHaveBeenCalledWith(1, 3)
    expect(textareas(wrapper)[0]!.props('modelValue')).toBe('1 2')
    textareas(wrapper)[0]!.vm.$emit('update:modelValue', '4 5')
    dialogButton('保存').click()
    await flushPromises()

    expect(updateTestcaseContent).toHaveBeenCalledWith(1, 3, { input: '4 5', output: '3' })
    expect(wrapper.emitted('saved')).toBeTruthy()
    wrapper.unmount()
  })

  it('creates a testcase with score and sample flag', async () => {
    const wrapper = mountDialog({})
    await flushPromises()

    textareas(wrapper)[0]!.vm.$emit('update:modelValue', 'a')
    textareas(wrapper)[1]!.vm.$emit('update:modelValue', 'b')
    dialogButton('创建测试点').click()
    await flushPromises()

    expect(createOnlineTestcase).toHaveBeenCalledWith(1, {
      input: 'a',
      output: 'b',
      score: 10,
      isSample: false,
    })
    wrapper.unmount()
  })

  it('blocks saving when a file exceeds 2 MiB', async () => {
    const wrapper = mountDialog({})
    await flushPromises()

    textareas(wrapper)[0]!.vm.$emit('update:modelValue', 'x'.repeat((2 << 20) + 1))
    await flushPromises()

    expect(document.body.textContent).toContain('单个测试数据不能超过 2 MiB')
    expect(dialogButton('创建测试点').hasAttribute('disabled')).toBe(true)
    wrapper.unmount()
  })

  it('asks before discarding unsaved edits', async () => {
    const wrapper = mountDialog({ testcase: testcase() })
    await flushPromises()
    textareas(wrapper)[0]!.vm.$emit('update:modelValue', 'changed')
    dialogButton('取消').click()
    await flushPromises()

    expect(confirm.require).toHaveBeenCalledWith(expect.objectContaining({ header: '放弃修改' }))
    wrapper.unmount()
  })
})
