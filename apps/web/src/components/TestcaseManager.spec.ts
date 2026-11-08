import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import { ApiError } from '@/api/http'
import {
  deleteTestcase,
  listTestcases,
  reorderTestcases,
  updateTestcase,
  uploadChecker,
  uploadTestcase,
} from '@/api/problems'
import type { Testcase } from '@/api/types'
import TestcaseManager from './TestcaseManager.vue'

const push = vi.hoisted(() => vi.fn())
const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push }),
}))
vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/problems', () => ({
  listTestcases: vi.fn(),
  deleteTestcase: vi.fn(),
  reorderTestcases: vi.fn(),
  updateTestcase: vi.fn(),
  uploadChecker: vi.fn(),
  uploadTestcase: vi.fn(),
}))

function tc(overrides: Partial<Testcase> = {}): Testcase {
  return {
    id: 1,
    sortOrder: 1,
    score: 100,
    isSample: false,
    inputSize: 2048,
    outputSize: 1024,
    ...overrides,
  }
}

function mountManager(
  type: 'traditional' | 'special' = 'traditional',
  opts: { checkerUploaded?: boolean } = {},
) {
  return mount(TestcaseManager, {
    props: { problemId: 1, type, checkerUploaded: opts.checkerUploaded ?? false },
    global: { plugins: [PrimeVue] },
  })
}

function buttons(wrapper: ReturnType<typeof mountManager>, label: string) {
  return wrapper.findAll('button').filter((b) => b.text().trim() === label)
}

beforeEach(() => {
  push.mockReset()
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(listTestcases).mockReset()
  vi.mocked(deleteTestcase).mockReset()
  vi.mocked(reorderTestcases).mockReset()
  vi.mocked(updateTestcase).mockReset()
  vi.mocked(uploadChecker).mockReset()
  vi.mocked(uploadTestcase).mockReset()
  vi.mocked(listTestcases).mockResolvedValue([])
})

describe('TestcaseManager', () => {
  it('loads testcases and switches problem id', async () => {
    const case1 = tc({ id: 1, inputSize: 1024, outputSize: 512 })
    const case2 = tc({ id: 2, inputSize: 2048, outputSize: 4096 })

    let resolve1!: (v: Testcase[]) => void
    let resolve2!: (v: Testcase[]) => void
    vi.mocked(listTestcases)
      .mockImplementationOnce(() => new Promise<Testcase[]>((r) => (resolve1 = r)))
      .mockImplementationOnce(() => new Promise<Testcase[]>((r) => (resolve2 = r)))

    const wrapper = mountManager('traditional')
    await nextTick()

    await wrapper.setProps({ problemId: 2 })
    await nextTick()

    resolve2([case2])
    await flushPromises()
    expect(wrapper.text()).toContain('2.0 KiB')

    resolve1([case1])
    await flushPromises()
    expect(wrapper.text()).toContain('2.0 KiB')
    expect(wrapper.text()).not.toContain('1.0 KiB')

    expect(listTestcases).toHaveBeenCalledWith(1)
    expect(listTestcases).toHaveBeenCalledWith(2)
    wrapper.unmount()
  })

  it('wraps the DataTable in a scroll container', async () => {
    vi.mocked(listTestcases).mockResolvedValue([tc({ id: 1, sortOrder: 1 })])
    const wrapper = mountManager('traditional')
    await flushPromises()

    // jsdom cannot measure overflow, so assert the structural wrapper exists
    // around the DataTable and owns the scroll/border.
    const wrap = wrapper.find('.tc-table-wrap')
    expect(wrap.exists()).toBe(true)
    expect(wrap.findComponent({ name: 'DataTable' }).exists()).toBe(true)
    expect(wrap.classes()).toContain('tc-table-wrap')
    wrapper.unmount()
  })

  it('shows AI button only for traditional', async () => {
    const wrapper = mountManager('traditional')
    await flushPromises()

    const ai = buttons(wrapper, 'AI 造数据')
    expect(ai).toHaveLength(1)
    await ai[0]!.trigger('click')
    expect(push).toHaveBeenCalledWith({
      name: 'problem-testcase-generator',
      params: { id: 1 },
    })

    const special = mountManager('special')
    await flushPromises()
    expect(buttons(special, 'AI 造数据')).toHaveLength(0)

    wrapper.unmount()
    special.unmount()
  })

  it('moves rows and reorders', async () => {
    vi.mocked(listTestcases).mockResolvedValue([
      tc({ id: 1, sortOrder: 1 }),
      tc({ id: 2, sortOrder: 2 }),
    ])
    vi.mocked(reorderTestcases).mockResolvedValue(null)
    const wrapper = mountManager('traditional')
    await flushPromises()

    const up = buttons(wrapper, '上移')
    expect(up).toHaveLength(2)
    expect((up[0]!.element as HTMLButtonElement).disabled).toBe(true)

    await up[1]!.trigger('click')
    await flushPromises()

    expect(reorderTestcases).toHaveBeenCalledWith(1, [2, 1])
    wrapper.unmount()
  })

  it('updates score and sample flag', async () => {
    vi.mocked(listTestcases).mockResolvedValue([
      tc({ id: 1, sortOrder: 1, score: 10, isSample: false }),
    ])
    vi.mocked(updateTestcase).mockResolvedValue(null)
    const wrapper = mountManager('traditional')
    await flushPromises()

    wrapper.findComponent(InputNumber).vm.$emit('value-change', 20)
    await flushPromises()
    expect(updateTestcase).toHaveBeenCalledWith(1, 1, { score: 20 })

    wrapper.findComponent(ToggleSwitch).vm.$emit('update:modelValue', true)
    await flushPromises()
    expect(updateTestcase).toHaveBeenCalledWith(1, 1, { isSample: true })

    vi.mocked(updateTestcase).mockRejectedValueOnce(new ApiError(500, 50000, '修改失败'))
    wrapper.findComponent(InputNumber).vm.$emit('value-change', 30)
    await flushPromises()
    expect(listTestcases).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('deletes after confirm', async () => {
    vi.mocked(listTestcases).mockResolvedValue([tc({ id: 1, sortOrder: 1 })])
    vi.mocked(deleteTestcase).mockResolvedValue(null)
    const wrapper = mountManager('traditional')
    await flushPromises()

    await buttons(wrapper, '删除')[0]!.trigger('click')
    await flushPromises()

    expect(deleteTestcase).not.toHaveBeenCalled()
    expect(confirm.require).toHaveBeenCalledTimes(1)
    const options = confirm.require.mock.calls[0]![0] as {
      header: string
      message: string
      acceptLabel: string
      rejectLabel: string
      accept: () => void
    }
    expect(options.header).toBe('删除测试点')
    expect(options.message).toBe('确定删除第 1 个测试点吗？')
    expect(options.acceptLabel).toBe('删除')
    expect(options.rejectLabel).toBe('取消')

    options.accept()
    await flushPromises()

    expect(deleteTestcase).toHaveBeenCalledWith(1, 1)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '已删除' }),
    )
    expect(listTestcases).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('changed')).toBeTruthy()
    wrapper.unmount()
  })

  it('does not delete when confirm is cancelled', async () => {
    vi.mocked(listTestcases).mockResolvedValue([tc({ id: 1, sortOrder: 1 })])
    const wrapper = mountManager('traditional')
    await flushPromises()

    await buttons(wrapper, '删除')[0]!.trigger('click')
    await flushPromises()

    expect(confirm.require).toHaveBeenCalledTimes(1)
    const options = confirm.require.mock.calls[0]![0] as { reject?: () => void }
    options.reject?.()
    await flushPromises()

    expect(deleteTestcase).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('uploads testcase', async () => {
    vi.mocked(listTestcases).mockResolvedValue([])
    vi.mocked(uploadTestcase).mockResolvedValue(tc({ id: 9 }))
    const wrapper = mountManager('traditional')
    await flushPromises()

    const uploadBtn = () => buttons(wrapper, '上传测试点')[0]!
    expect((uploadBtn().element as HTMLButtonElement).disabled).toBe(true)

    const fileInputs = wrapper.findAll('input[type="file"]')
    const input = fileInputs[0]!
    const output = fileInputs[1]!
    const inputFile = new File(['in'], 'in.txt')
    const outputFile = new File(['out'], 'out.txt')
    Object.defineProperty(input.element, 'files', { value: [inputFile], configurable: true })
    await input.trigger('change')
    Object.defineProperty(output.element, 'files', { value: [outputFile], configurable: true })
    await output.trigger('change')

    expect(wrapper.text()).toContain('in.txt')
    expect(wrapper.text()).toContain('out.txt')
    expect((uploadBtn().element as HTMLButtonElement).disabled).toBe(false)

    await uploadBtn().trigger('click')
    await flushPromises()

    expect(uploadTestcase).toHaveBeenCalledWith(1, {
      input: inputFile,
      output: outputFile,
      score: 10,
      isSample: false,
    })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '测试点已上传' }),
    )
    expect(listTestcases).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('changed')).toBeTruthy()
    expect(wrapper.text()).not.toContain('in.txt')
    wrapper.unmount()
  })

  it('uploads checker for special', async () => {
    vi.mocked(listTestcases).mockResolvedValue([])
    vi.mocked(uploadChecker).mockResolvedValue(null)
    const wrapper = mountManager('special', { checkerUploaded: false })
    await flushPromises()

    expect(wrapper.findComponent(Tag).props('value')).toBe('未上传')

    const checkerInput = wrapper.find('input[accept=".cpp,.cc,.cxx,.txt"]')
    const file = new File(['code'], 'checker.cpp')
    Object.defineProperty(checkerInput.element, 'files', { value: [file], configurable: true })
    await checkerInput.trigger('change')

    expect(wrapper.text()).toContain('checker.cpp')

    await buttons(wrapper, '上传 checker')[0]!.trigger('click')
    await flushPromises()

    expect(uploadChecker).toHaveBeenCalledWith(1, file)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: 'checker 已上传' }),
    )
    expect(wrapper.emitted('changed')).toBeTruthy()

    await wrapper.setProps({ checkerUploaded: true })
    expect(wrapper.findComponent(Tag).props('value')).toBe('已上传')
    wrapper.unmount()
  })

  it('shows load error with retry', async () => {
    vi.mocked(listTestcases).mockRejectedValueOnce(new Error('boom'))
    const wrapper = mountManager('traditional')
    await flushPromises()

    expect(wrapper.findComponent(Message).exists()).toBe(true)
    expect(wrapper.text()).toContain('加载测试点失败')

    vi.mocked(listTestcases).mockResolvedValue([tc({ id: 1, sortOrder: 1 })])

    await buttons(wrapper, '重试')[0]!.trigger('click')
    await flushPromises()

    expect(listTestcases).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).not.toContain('加载测试点失败')
    wrapper.unmount()
  })
})
