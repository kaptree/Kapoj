import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import InputNumber from 'primevue/inputnumber'
import Select from 'primevue/select'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import { ApiError } from '@/api/http'
import {
  confirmTestcaseGeneration,
  createTestcaseGeneration,
  discardTestcaseGeneration,
  getTestcaseGeneration,
  previewTestcaseGeneration,
} from '@/api/problems'
import type { TestcaseGenerationView } from '@/api/types'
import TestcaseGeneratorView from './TestcaseGeneratorView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/problems', () => ({
  createTestcaseGeneration: vi.fn(),
  getTestcaseGeneration: vi.fn(),
  previewTestcaseGeneration: vi.fn(),
  confirmTestcaseGeneration: vi.fn(),
  discardTestcaseGeneration: vi.fn(),
}))
vi.mock('@/components/CodeEditor.vue', () => ({
  default: {
    name: 'CodeEditor',
    props: {
      modelValue: { type: String, default: '' },
      language: { type: String },
      height: { type: String },
      readonly: { type: Boolean },
    },
    emits: ['update:modelValue'],
    template: '<div class="code-editor-stub">{{ modelValue }}</div>',
  },
}))

function genView(overrides: Partial<TestcaseGenerationView> = {}): TestcaseGenerationView {
  return {
    id: 1,
    status: 'pending',
    language: 'cpp',
    testcaseCount: 5,
    generatorCode: 'import random\n',
    errorMessage: '',
    createdAt: '2026-11-08T00:00:00Z',
    cases: [],
    ...overrides,
  }
}

const routes = [
  {
    path: '/problems/:id/testcase-generator',
    name: 'problem-testcase-generator',
    component: { template: '<div />' },
  },
  { path: '/problems/:id/edit', name: 'problem-edit', component: { template: '<div />' } },
  { path: '/settings/ai', name: 'ai-settings', component: { template: '<div />' } },
]

async function mountPage() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/problems/1/testcase-generator')
  await router.isReady()
  const wrapper = mount(TestcaseGeneratorView, { global: { plugins: [router, PrimeVue] } })
  await flushPromises()
  return { wrapper, router }
}

function editor(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper']) {
  return wrapper.findComponent({ name: 'CodeEditor' })
}

function setSource(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper'], source: string) {
  editor(wrapper).vm.$emit('update:modelValue', source)
}

function buttonByLabel(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper'], label: string) {
  return wrapper.findAll('button').find((b) => b.text().trim() === label)
}

async function createReady(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper']) {
  setSource(wrapper, 'int main() { return 0; }')
  await nextTick()
  await wrapper.find('form').trigger('submit')
  await flushPromises()
}

beforeEach(() => {
  document.body.innerHTML = ''
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(createTestcaseGeneration).mockReset()
  vi.mocked(getTestcaseGeneration).mockReset()
  vi.mocked(previewTestcaseGeneration).mockReset()
  vi.mocked(confirmTestcaseGeneration).mockReset()
  vi.mocked(discardTestcaseGeneration).mockReset()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('TestcaseGeneratorView', () => {
  it('defaults to cpp language and count 5', async () => {
    const { wrapper } = await mountPage()

    expect(wrapper.findComponent(Select).props('modelValue')).toBe('cpp')
    expect(wrapper.findComponent(InputNumber).props('modelValue')).toBe(5)
    wrapper.unmount()
  })

  it('warns instead of creating when the source is empty', async () => {
    const { wrapper } = await mountPage()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(createTestcaseGeneration).not.toHaveBeenCalled()
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'warn', summary: '源码不能为空' }),
    )
    wrapper.unmount()
  })

  it('warns instead of creating when the source exceeds 64 KiB', async () => {
    const { wrapper } = await mountPage()

    setSource(wrapper, 'a'.repeat(65537))
    await nextTick()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(createTestcaseGeneration).not.toHaveBeenCalled()
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'warn', summary: '源码超过 64 KiB' }),
    )
    wrapper.unmount()
  })

  it('creates a generation and shows the pending status', async () => {
    vi.useFakeTimers()
    vi.mocked(createTestcaseGeneration).mockResolvedValue(genView({ status: 'pending' }))
    vi.mocked(getTestcaseGeneration).mockResolvedValue(genView({ status: 'pending' }))
    const { wrapper } = await mountPage()

    setSource(wrapper, 'int main() { return 0; }')
    await nextTick()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(createTestcaseGeneration).toHaveBeenCalledWith(1, {
      language: 'cpp',
      sourceCode: 'int main() { return 0; }',
      testcaseCount: 5,
    })
    expect(wrapper.text()).toContain('排队中')
    wrapper.unmount()
  })

  it('polls until ready and renders cases and the awaiting-confirmation tag', async () => {
    vi.useFakeTimers()
    vi.mocked(createTestcaseGeneration).mockResolvedValue(genView({ status: 'pending' }))
    vi.mocked(getTestcaseGeneration)
      .mockResolvedValueOnce(genView({ status: 'running' }))
      .mockResolvedValueOnce(
        genView({
          status: 'ready',
          cases: [
            { id: 10, sortOrder: 1, inputSize: 12, outputSize: 6 },
            { id: 11, sortOrder: 2, inputSize: 24, outputSize: 8 },
          ],
        }),
      )
    const { wrapper } = await mountPage()

    setSource(wrapper, 'int main() { return 0; }')
    await nextTick()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('排队中')

    await vi.advanceTimersByTimeAsync(2000)
    expect(wrapper.text()).toContain('生成中')

    await vi.advanceTimersByTimeAsync(2000)
    expect(wrapper.text()).toContain('待确认')
    expect(wrapper.text()).toContain('输入大小')
    expect(wrapper.text()).toContain('输出大小')
    expect(wrapper.text()).toContain('12 B')
    expect(wrapper.text()).toContain('8 B')
    expect(wrapper.text()).toContain('确认入库')

    expect(getTestcaseGeneration).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('maps AI errors: 40002 key, 40001 config link, 42903/50200/50300 message', async () => {
    const { wrapper } = await mountPage()
    setSource(wrapper, 'int main() { return 0; }')
    await nextTick()

    vi.mocked(createTestcaseGeneration).mockRejectedValueOnce(
      new ApiError(400, 40002, 'AI 加密密钥未配置'),
    )
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('管理员尚未配置 AI 加密密钥')

    vi.mocked(createTestcaseGeneration).mockRejectedValueOnce(
      new ApiError(400, 40001, '尚未配置 AI 模型'),
    )
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('尚未配置 AI 模型')
    expect(wrapper.find('a[href="/settings/ai"]').exists()).toBe(true)

    vi.mocked(createTestcaseGeneration).mockRejectedValueOnce(
      new ApiError(429, 42903, 'rate limited'),
    )
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('rate limited')

    vi.mocked(createTestcaseGeneration).mockRejectedValueOnce(
      new ApiError(502, 50200, 'upstream error'),
    )
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('upstream error')

    vi.mocked(createTestcaseGeneration).mockRejectedValueOnce(
      new ApiError(503, 50300, 'unavailable'),
    )
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('unavailable')
    wrapper.unmount()
  })

  it('previews input and output with a truncation note', async () => {
    vi.mocked(createTestcaseGeneration).mockResolvedValue(
      genView({ status: 'ready', cases: [{ id: 10, sortOrder: 1, inputSize: 12, outputSize: 6 }] }),
    )
    vi.mocked(previewTestcaseGeneration).mockResolvedValue({
      kind: 'input',
      content: '1 2 3\n',
      truncated: true,
    })
    const { wrapper } = await mountPage()
    await createReady(wrapper)

    await buttonByLabel(wrapper, '输入')!.trigger('click')
    await flushPromises()

    expect(previewTestcaseGeneration).toHaveBeenCalledWith(1, 1, 10, 'input')
    const dialog = document.querySelector('.p-dialog')!
    expect(dialog.textContent).toContain('1 2 3')
    expect(dialog.textContent).toContain('仅显示前 64 KiB')
    wrapper.unmount()
  })

  it('confirms on ready, toasts and navigates to problem-edit', async () => {
    vi.mocked(createTestcaseGeneration).mockResolvedValue(
      genView({ status: 'ready', cases: [{ id: 10, sortOrder: 1, inputSize: 12, outputSize: 6 }] }),
    )
    vi.mocked(confirmTestcaseGeneration).mockResolvedValue({ count: 1 })
    const { wrapper, router } = await mountPage()
    await createReady(wrapper)

    await buttonByLabel(wrapper, '确认入库')!.trigger('click')
    await flushPromises()

    expect(confirm.require).toHaveBeenCalledTimes(1)
    const options = confirm.require.mock.calls[0]![0] as { accept?: () => void }
    options.accept?.()
    await flushPromises()

    expect(confirmTestcaseGeneration).toHaveBeenCalledWith(1, 1)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '已入库 1 组测试点' }),
    )
    expect(router.currentRoute.value.name).toBe('problem-edit')
    wrapper.unmount()
  })

  it('discards after a second confirmation and clears the generation', async () => {
    vi.mocked(createTestcaseGeneration).mockResolvedValue(genView({ status: 'ready' }))
    vi.mocked(discardTestcaseGeneration).mockResolvedValue(null)
    const { wrapper } = await mountPage()
    await createReady(wrapper)

    await buttonByLabel(wrapper, '丢弃')!.trigger('click')
    await flushPromises()

    expect(confirm.require).toHaveBeenCalledTimes(1)
    const options = confirm.require.mock.calls[0]![0] as { accept?: () => void }
    options.accept?.()
    await flushPromises()

    expect(discardTestcaseGeneration).toHaveBeenCalledWith(1, 1)
    expect(wrapper.text()).not.toContain('生成状态')
    wrapper.unmount()
  })

  it('shows only discard (with errorMessage) when failed', async () => {
    vi.mocked(createTestcaseGeneration).mockResolvedValue(
      genView({ status: 'failed', errorMessage: '编译错误：expected ;' }),
    )
    const { wrapper } = await mountPage()
    await createReady(wrapper)

    expect(wrapper.text()).toContain('生成失败')
    expect(wrapper.text()).toContain('编译错误：expected ;')
    expect(buttonByLabel(wrapper, '丢弃')).toBeTruthy()
    expect(buttonByLabel(wrapper, '确认入库')).toBeUndefined()
    wrapper.unmount()
  })

  it('stops polling after reaching a terminal status', async () => {
    vi.useFakeTimers()
    vi.mocked(createTestcaseGeneration).mockResolvedValue(genView({ status: 'pending' }))
    vi.mocked(getTestcaseGeneration).mockResolvedValue(genView({ status: 'ready' }))
    const { wrapper } = await mountPage()

    setSource(wrapper, 'int main() { return 0; }')
    await nextTick()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    await vi.advanceTimersByTimeAsync(2000)
    expect(getTestcaseGeneration).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('待确认')

    await vi.advanceTimersByTimeAsync(10000)
    expect(getTestcaseGeneration).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('clears the poll timer on unmount', async () => {
    vi.useFakeTimers()
    vi.mocked(createTestcaseGeneration).mockResolvedValue(genView({ status: 'pending' }))
    vi.mocked(getTestcaseGeneration).mockResolvedValue(genView({ status: 'running' }))
    const { wrapper } = await mountPage()

    setSource(wrapper, 'int main() { return 0; }')
    await nextTick()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    wrapper.unmount()

    await vi.advanceTimersByTimeAsync(10000)
    expect(getTestcaseGeneration).not.toHaveBeenCalled()
  })

  it('does not re-arm the poll after unmount during an in-flight GET', async () => {
    vi.useFakeTimers()
    vi.mocked(createTestcaseGeneration).mockResolvedValue(genView({ status: 'pending' }))
    let resolveGet: ((v: TestcaseGenerationView) => void) | undefined
    vi.mocked(getTestcaseGeneration).mockImplementation(
      () => new Promise<TestcaseGenerationView>((resolve) => (resolveGet = resolve)),
    )
    const { wrapper } = await mountPage()

    setSource(wrapper, 'int main() { return 0; }')
    await nextTick()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    await vi.advanceTimersByTimeAsync(2000)
    expect(getTestcaseGeneration).toHaveBeenCalledTimes(1)

    wrapper.unmount()

    resolveGet!(genView({ status: 'running' }))
    await flushPromises()

    await vi.advanceTimersByTimeAsync(10000)
    expect(getTestcaseGeneration).toHaveBeenCalledTimes(1)
  })

  it('times out after 10 minutes and stops polling', async () => {
    vi.useFakeTimers()
    vi.mocked(createTestcaseGeneration).mockResolvedValue(genView({ status: 'pending' }))
    vi.mocked(getTestcaseGeneration).mockResolvedValue(genView({ status: 'pending' }))
    const { wrapper } = await mountPage()

    setSource(wrapper, 'int main() { return 0; }')
    await nextTick()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    await vi.advanceTimersByTimeAsync(10 * 60 * 1000)
    expect(wrapper.text()).toContain('生成超时')

    const calls = vi.mocked(getTestcaseGeneration).mock.calls.length
    await vi.advanceTimersByTimeAsync(10000)
    expect(vi.mocked(getTestcaseGeneration).mock.calls.length).toBe(calls)
    wrapper.unmount()
  })

  it('stops polling and surfaces the error on a terminal GET failure', async () => {
    vi.useFakeTimers()
    vi.mocked(createTestcaseGeneration).mockResolvedValue(genView({ status: 'pending' }))
    vi.mocked(getTestcaseGeneration).mockRejectedValue(
      new ApiError(404, 40400, '题目不存在或没有权限'),
    )
    const { wrapper } = await mountPage()

    setSource(wrapper, 'int main() { return 0; }')
    await nextTick()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    await vi.advanceTimersByTimeAsync(2000)
    expect(getTestcaseGeneration).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('题目不存在或没有权限')

    await vi.advanceTimersByTimeAsync(10000)
    expect(getTestcaseGeneration).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('warns and skips create when count is not an integer 1-10', async () => {
    const { wrapper } = await mountPage()
    setSource(wrapper, 'int main() { return 0; }')
    await nextTick()

    wrapper.findComponent(InputNumber).vm.$emit('update:modelValue', null)
    await nextTick()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(createTestcaseGeneration).not.toHaveBeenCalled()
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'warn',
        summary: '生成组数必须是 1 到 10 之间的整数',
      }),
    )
    wrapper.unmount()
  })

  it('shows a non-config 40001 without the AI settings link', async () => {
    vi.mocked(createTestcaseGeneration).mockRejectedValue(
      new ApiError(400, 40001, '仅传统题支持 AI 造数据'),
    )
    const { wrapper } = await mountPage()
    setSource(wrapper, 'int main() { return 0; }')
    await nextTick()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('仅传统题支持 AI 造数据')
    expect(wrapper.find('a[href="/settings/ai"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows the server message for unmapped API errors', async () => {
    vi.mocked(createTestcaseGeneration).mockRejectedValue(
      new ApiError(500, 50000, '服务器内部错误'),
    )
    const { wrapper } = await mountPage()
    setSource(wrapper, 'int main() { return 0; }')
    await nextTick()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('服务器内部错误')
    wrapper.unmount()
  })
})
