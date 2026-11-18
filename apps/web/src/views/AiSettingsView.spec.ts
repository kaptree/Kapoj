import { flushPromises, mount } from '@vue/test-utils'
import Checkbox from 'primevue/checkbox'
import PrimeVue from 'primevue/config'
import Select from 'primevue/select'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  createAIUserModel,
  deleteAIUserModel,
  listAIUserModels,
  setActiveAIUserModel,
  testAIUserModel,
  updateAIUserModel,
} from '@/api/ai'
import { ApiError } from '@/api/http'
import type { AIUserModelsView, AIUserModelView } from '@/api/types'
import AiSettingsView from './AiSettingsView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/ai', () => ({
  listAIUserModels: vi.fn(),
  createAIUserModel: vi.fn(),
  updateAIUserModel: vi.fn(),
  deleteAIUserModel: vi.fn(),
  setActiveAIUserModel: vi.fn(),
  testAIUserModel: vi.fn(),
}))

function userModel(overrides: Partial<AIUserModelView> = {}): AIUserModelView {
  return {
    id: 1,
    name: '我的模型',
    protocol: 'openai',
    baseUrl: 'https://api.example.com/v1',
    modelName: 'gpt-4o-mini',
    apiKeyMasked: 'sk-****abcd',
    isActive: true,
    lastTestAt: null,
    lastTestOk: false,
    lastTestError: '',
    lastTestLatencyMs: 0,
    ...overrides,
  }
}

function modelsView(overrides: Partial<AIUserModelsView> = {}): AIUserModelsView {
  return {
    keyConfigured: true,
    activeModelId: 1,
    models: [userModel()],
    providers: [
      {
        code: 'deepseek',
        name: 'DeepSeek',
        defaultModel: 'deepseek-chat',
        baseUrl: 'https://api.deepseek.com/v1',
        protocol: 'openai',
      },
      {
        code: 'openai',
        name: 'OpenAI',
        defaultModel: 'gpt-4o-mini',
        baseUrl: 'https://api.openai.com/v1',
        protocol: 'openai',
      },
    ],
    ...overrides,
  }
}

async function mountPage() {
  const wrapper = mount(AiSettingsView, { global: { plugins: [PrimeVue] } })
  await flushPromises()
  return wrapper
}

function dialog(): HTMLElement {
  return document.querySelector('.p-dialog') as HTMLElement
}

function dialogButton(label: string): HTMLButtonElement {
  return Array.from(dialog().querySelectorAll('button')).find(
    (item) => item.textContent?.trim() === label,
  ) as HTMLButtonElement
}

function setField(element: Element, value: string) {
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(element, value)
  element.dispatchEvent(new Event('input', { bubbles: true }))
}

function dialogSelect(wrapper: Awaited<ReturnType<typeof mountPage>>, inputId: string) {
  const found = wrapper.findAllComponents(Select).find((item) => item.props('inputId') === inputId)
  if (!found) throw new Error(`Select ${inputId} not found`)
  return found
}

beforeEach(() => {
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(listAIUserModels).mockReset()
  vi.mocked(createAIUserModel).mockReset()
  vi.mocked(updateAIUserModel).mockReset()
  vi.mocked(deleteAIUserModel).mockReset()
  vi.mocked(setActiveAIUserModel).mockReset()
  vi.mocked(testAIUserModel).mockReset()
  vi.mocked(listAIUserModels).mockResolvedValue(modelsView())
  document.body.innerHTML = ''
})

describe('AiSettingsView', () => {
  it('lists saved models and marks the active one', async () => {
    vi.mocked(listAIUserModels).mockResolvedValue(
      modelsView({
        activeModelId: 2,
        models: [
          userModel({
            id: 2,
            name: 'DeepSeek 主力',
            protocol: 'openai',
            baseUrl: 'https://api.deepseek.com/v1',
            modelName: 'deepseek-chat',
            apiKeyMasked: 'sk-****1111',
            isActive: true,
            lastTestAt: '2026-11-18T01:00:00Z',
            lastTestOk: true,
            lastTestLatencyMs: 321,
          }),
          userModel({
            id: 3,
            name: 'Anthropic 备用',
            protocol: 'anthropic',
            baseUrl: 'https://api.anthropic.com/v1',
            modelName: 'claude-sonnet-4-5',
            apiKeyMasked: 'sk-ant-****2222',
            isActive: false,
          }),
        ],
      }),
    )
    const wrapper = await mountPage()

    expect(listAIUserModels).toHaveBeenCalledTimes(1)
    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)

    expect(rows[0]!.text()).toContain('DeepSeek 主力')
    expect(rows[0]!.text()).toContain('OpenAI')
    expect(rows[0]!.text()).toContain('https://api.deepseek.com/v1')
    expect(rows[0]!.text()).toContain('deepseek-chat')
    expect(rows[0]!.text()).toContain('sk-****1111')
    expect(rows[0]!.findAll('.p-tag').map((tag) => tag.text())).toContain('当前')
    expect(rows[0]!.text()).toContain('成功')

    expect(rows[1]!.text()).toContain('Anthropic 备用')
    expect(rows[1]!.text()).toContain('Anthropic')
    expect(rows[1]!.text()).toContain('https://api.anthropic.com/v1')
    expect(rows[1]!.text()).toContain('claude-sonnet-4-5')
    expect(rows[1]!.text()).toContain('sk-ant-****2222')
    expect(rows[1]!.findAll('.p-tag').map((tag) => tag.text())).not.toContain('当前')

    expect(wrapper.findAll('.p-tag').map((tag) => tag.text())).toContain('当前')
    wrapper.unmount()
  })

  it('creates a custom Anthropic model from the dialog', async () => {
    vi.mocked(listAIUserModels).mockResolvedValue(
      modelsView({ keyConfigured: true, activeModelId: null, models: [] }),
    )
    vi.mocked(createAIUserModel).mockResolvedValue(
      userModel({
        id: 9,
        name: '自定义 Anthropic',
        protocol: 'anthropic',
        baseUrl: 'https://api.anthropic.com/v1',
        modelName: 'claude-sonnet-4-5',
        apiKeyMasked: 'sk-ant-****9999',
        isActive: true,
      }),
    )
    const wrapper = await mountPage()

    await wrapper.find('[data-test="add-user-model"]').trigger('click')
    await flushPromises()
    expect(dialog()).toBeTruthy()

    setField(dialog().querySelector('#user-model-name')!, '自定义 Anthropic')
    dialogSelect(wrapper, 'user-model-protocol').vm.$emit('update:modelValue', 'anthropic')
    await flushPromises()
    setField(dialog().querySelector('#user-model-base-url')!, 'https://api.anthropic.com/v1')
    setField(dialog().querySelector('#user-model-model-name')!, 'claude-sonnet-4-5')
    setField(dialog().querySelector('#user-model-api-key')!, 'sk-ant-live-secret')
    wrapper.findComponent(Checkbox).vm.$emit('update:modelValue', true)
    await flushPromises()

    dialogButton('创建').click()
    await flushPromises()

    expect(createAIUserModel).toHaveBeenCalledWith({
      name: '自定义 Anthropic',
      protocol: 'anthropic',
      baseUrl: 'https://api.anthropic.com/v1',
      modelName: 'claude-sonnet-4-5',
      apiKey: 'sk-ant-live-secret',
      isActive: true,
    })
    expect(listAIUserModels).toHaveBeenCalledTimes(2)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '模型已创建' }),
    )
    // The raw token is never rendered or echoed by the page.
    expect(wrapper.text()).not.toContain('sk-ant-live-secret')
    wrapper.unmount()
  })

  it('fills the base URL, default model and protocol from a provider preset', async () => {
    vi.mocked(listAIUserModels).mockResolvedValue(modelsView({ activeModelId: null, models: [] }))
    const wrapper = await mountPage()

    await wrapper.find('[data-test="add-user-model"]').trigger('click')
    await flushPromises()

    dialogSelect(wrapper, 'user-model-provider').vm.$emit('update:modelValue', 'deepseek')
    await flushPromises()

    expect((dialog().querySelector('#user-model-base-url') as HTMLInputElement).value).toBe(
      'https://api.deepseek.com/v1',
    )
    expect((dialog().querySelector('#user-model-model-name') as HTMLInputElement).value).toBe(
      'deepseek-chat',
    )
    expect(dialogSelect(wrapper, 'user-model-protocol').props('modelValue')).toBe('openai')
    wrapper.unmount()
  })

  it('edits a model and keeps the token when the API Key is blank', async () => {
    vi.mocked(listAIUserModels).mockResolvedValue(
      modelsView({
        activeModelId: 5,
        models: [
          userModel({
            id: 5,
            name: '旧名称',
            protocol: 'openai',
            baseUrl: 'https://old.example/v1',
            modelName: 'old-model',
            apiKeyMasked: 'sk-****wxyz',
            isActive: true,
          }),
        ],
      }),
    )
    vi.mocked(updateAIUserModel).mockResolvedValue(userModel({ id: 5, name: '新名称' }))
    const wrapper = await mountPage()

    await wrapper.find('[data-test="edit-user-model-5"]').trigger('click')
    await flushPromises()

    expect((dialog().querySelector('#user-model-name') as HTMLInputElement).value).toBe('旧名称')
    expect(dialog().textContent).toContain('sk-****wxyz')

    setField(dialog().querySelector('#user-model-name')!, '新名称')
    dialogSelect(wrapper, 'user-model-protocol').vm.$emit('update:modelValue', 'anthropic')
    await flushPromises()
    setField(dialog().querySelector('#user-model-base-url')!, 'https://api.anthropic.com/v1')
    setField(dialog().querySelector('#user-model-model-name')!, 'claude-sonnet-4-5')
    dialogButton('保存').click()
    await flushPromises()

    const input = vi.mocked(updateAIUserModel).mock.calls[0]![1]
    expect(updateAIUserModel).toHaveBeenCalledWith(5, {
      name: '新名称',
      protocol: 'anthropic',
      baseUrl: 'https://api.anthropic.com/v1',
      modelName: 'claude-sonnet-4-5',
    })
    expect(input).not.toHaveProperty('apiKey')
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '模型已保存' }),
    )
    wrapper.unmount()
  })

  it('validates required fields before creating a model', async () => {
    vi.mocked(listAIUserModels).mockResolvedValue(modelsView({ activeModelId: null, models: [] }))
    const wrapper = await mountPage()

    await wrapper.find('[data-test="add-user-model"]').trigger('click')
    await flushPromises()
    dialogButton('创建').click()
    await flushPromises()

    expect(createAIUserModel).not.toHaveBeenCalled()
    expect(dialog().textContent).toContain('请填写模型名称')
    expect(dialog().textContent).toContain('请填写接口地址')
    expect(dialog().textContent).toContain('请填写模型名')
    expect(dialog().textContent).toContain('请填写 API Key')
    wrapper.unmount()
  })

  it('sets a different model active', async () => {
    vi.mocked(listAIUserModels)
      .mockResolvedValueOnce(
        modelsView({
          activeModelId: 1,
          models: [
            userModel({ id: 1, name: '模型 A', isActive: true }),
            userModel({ id: 2, name: '模型 B', isActive: false }),
          ],
        }),
      )
      .mockResolvedValue(
        modelsView({
          activeModelId: 2,
          models: [
            userModel({ id: 1, name: '模型 A', isActive: false }),
            userModel({ id: 2, name: '模型 B', isActive: true }),
          ],
        }),
      )
    vi.mocked(setActiveAIUserModel).mockResolvedValue(userModel({ id: 2, isActive: true }))
    const wrapper = await mountPage()

    await wrapper.find('[data-test="set-active-model-2"]').trigger('click')
    await flushPromises()

    expect(setActiveAIUserModel).toHaveBeenCalledWith(2)
    expect(listAIUserModels).toHaveBeenCalledTimes(2)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '已设为当前模型' }),
    )
    const rows = wrapper.findAll('tbody tr')
    expect(rows[0]!.findAll('.p-tag').map((tag) => tag.text())).not.toContain('当前')
    expect(rows[1]!.findAll('.p-tag').map((tag) => tag.text())).toContain('当前')
    wrapper.unmount()
  })

  it('disables deleting the active model and shows a switch hint', async () => {
    vi.mocked(listAIUserModels).mockResolvedValue(
      modelsView({
        activeModelId: 1,
        models: [
          userModel({ id: 1, name: '当前模型', isActive: true }),
          userModel({ id: 2, name: '备用模型', isActive: false }),
        ],
      }),
    )
    const wrapper = await mountPage()

    const activeDelete = wrapper.find('[data-test="delete-user-model-1"]')
    expect((activeDelete.element as HTMLButtonElement).disabled).toBe(true)
    expect(wrapper.text()).toContain('先切换')
    expect(
      (wrapper.find('[data-test="delete-user-model-2"]').element as HTMLButtonElement).disabled,
    ).toBe(false)

    await activeDelete.trigger('click')
    await flushPromises()
    expect(confirm.require).not.toHaveBeenCalled()
    expect(deleteAIUserModel).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('deletes a non-active model after confirmation', async () => {
    vi.mocked(listAIUserModels)
      .mockResolvedValueOnce(
        modelsView({
          activeModelId: 1,
          models: [
            userModel({ id: 1, name: '当前模型', isActive: true }),
            userModel({ id: 2, name: '可删除模型', isActive: false }),
          ],
        }),
      )
      .mockResolvedValue(
        modelsView({
          activeModelId: 1,
          models: [userModel({ id: 1, name: '当前模型', isActive: true })],
        }),
      )
    vi.mocked(deleteAIUserModel).mockResolvedValue(undefined)
    const wrapper = await mountPage()

    await wrapper.find('[data-test="delete-user-model-2"]').trigger('click')
    await flushPromises()
    expect(confirm.require).toHaveBeenCalledTimes(1)
    expect(deleteAIUserModel).not.toHaveBeenCalled()

    const options = confirm.require.mock.calls[0]![0] as { message: string; accept?: () => void }
    expect(options.message).toContain('可删除模型')
    options.accept?.()
    await flushPromises()

    expect(deleteAIUserModel).toHaveBeenCalledWith(2)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '模型已删除' }),
    )
    expect(wrapper.text()).not.toContain('可删除模型')
    wrapper.unmount()
  })

  it('tests a model, shows loading and a sanitized success result', async () => {
    const model = userModel({ id: 3, name: '测试模型', isActive: true })
    vi.mocked(listAIUserModels)
      .mockResolvedValueOnce(modelsView({ activeModelId: 3, models: [model] }))
      .mockResolvedValue(
        modelsView({
          activeModelId: 3,
          models: [
            userModel({
              ...model,
              lastTestAt: '2026-11-18T01:00:00Z',
              lastTestOk: true,
              lastTestLatencyMs: 812,
            }),
          ],
        }),
      )

    let resolveTest!: (result: { ok: boolean; latencyMs: number; message: string }) => void
    vi.mocked(testAIUserModel).mockReturnValue(
      new Promise((resolve) => {
        resolveTest = resolve
      }),
    )

    const wrapper = await mountPage()
    const testButton = wrapper.find('[data-test="test-user-model-3"]')
    await testButton.trigger('click')
    await flushPromises()

    expect(testAIUserModel).toHaveBeenCalledWith(3)
    expect(testButton.classes()).toContain('p-button-loading')

    resolveTest({ ok: true, latencyMs: 812, message: '连接成功' })
    await flushPromises()

    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'success',
        summary: '测试通过',
        detail: expect.stringContaining('连接成功'),
      }),
    )
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ detail: expect.stringContaining('812') }),
    )
    expect(listAIUserModels).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('成功')
    expect(wrapper.text()).toContain('812 ms')
    wrapper.unmount()
  })

  it('shows a failure result when the model test fails without leaking tokens', async () => {
    const model = userModel({
      id: 6,
      name: '故障模型',
      apiKeyMasked: 'sk-****fail',
      isActive: true,
    })
    vi.mocked(listAIUserModels)
      .mockResolvedValueOnce(modelsView({ activeModelId: 6, models: [model] }))
      .mockResolvedValue(
        modelsView({
          activeModelId: 6,
          models: [
            userModel({
              ...model,
              lastTestAt: '2026-11-18T01:00:00Z',
              lastTestOk: false,
              lastTestError: '连接超时',
              lastTestLatencyMs: 10000,
            }),
          ],
        }),
      )
    vi.mocked(testAIUserModel).mockResolvedValue({
      ok: false,
      latencyMs: 10000,
      message: '连接超时',
    })
    const wrapper = await mountPage()

    await wrapper.find('[data-test="test-user-model-6"]').trigger('click')
    await flushPromises()

    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'error',
        summary: '测试失败',
        detail: expect.stringContaining('连接超时'),
      }),
    )
    expect(wrapper.text()).toContain('失败')
    expect(wrapper.text()).toContain('连接超时')
    expect(JSON.stringify(toast.add.mock.calls)).not.toContain('sk-****fail')
    wrapper.unmount()
  })

  it('shows the key-missing notice and no model actions when the platform key is not configured', async () => {
    vi.mocked(listAIUserModels).mockResolvedValue(
      modelsView({ keyConfigured: false, activeModelId: null, models: [] }),
    )
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('管理员尚未配置 AI 加密密钥，请联系管理员')
    expect(wrapper.find('[data-test="add-user-model"]').exists()).toBe(false)
    expect(createAIUserModel).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('shows the disabled notice when AI is not enabled (404)', async () => {
    vi.mocked(listAIUserModels).mockRejectedValue(new ApiError(404, 40400, 'not found'))
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('AI 功能未启用，请联系管理员')
    expect(wrapper.find('[data-test="add-user-model"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows the empty state with an add action when there are no models', async () => {
    vi.mocked(listAIUserModels).mockResolvedValue(modelsView({ activeModelId: null, models: [] }))
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('暂无 AI 模型')
    const emptyAdd = wrapper.find('[data-test="add-user-model-empty"]')
    expect(emptyAdd.exists()).toBe(true)
    expect(emptyAdd.text()).toContain('新增模型')

    await emptyAdd.trigger('click')
    await flushPromises()
    expect(dialog()).toBeTruthy()
    wrapper.unmount()
  })

  it('shows a load error and retries', async () => {
    vi.mocked(listAIUserModels).mockRejectedValueOnce(new ApiError(500, 50000, '模型加载失败'))
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('模型加载失败')

    vi.mocked(listAIUserModels).mockResolvedValue(modelsView())
    await wrapper
      .findAll('button')
      .find((item) => item.text().trim() === '重试')!
      .trigger('click')
    await flushPromises()

    expect(listAIUserModels).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('我的模型')
    wrapper.unmount()
  })
})
