import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Select from 'primevue/select'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { getAIConfig, saveAIConfig } from '@/api/ai'
import { ApiError } from '@/api/http'
import type { AIConfigView } from '@/api/types'
import AiSettingsView from './AiSettingsView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('@/api/ai', () => ({
  getAIConfig: vi.fn(),
  saveAIConfig: vi.fn(),
}))

function aiConfig(overrides: Partial<AIConfigView> = {}): AIConfigView {
  return {
    configured: true,
    keyConfigured: true,
    provider: 'deepseek',
    modelName: 'deepseek-chat',
    apiKeyMasked: 'sk-****abcd',
    providers: [
      { code: 'deepseek', name: 'DeepSeek', defaultModel: 'deepseek-chat' },
      { code: 'openai', name: 'OpenAI', defaultModel: 'gpt-4o-mini' },
    ],
    ...overrides,
  }
}

async function mountPage() {
  const wrapper = mount(AiSettingsView, { global: { plugins: [PrimeVue] } })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  toast.add.mockReset()
  vi.mocked(getAIConfig).mockReset()
  vi.mocked(saveAIConfig).mockReset()
})

describe('AiSettingsView', () => {
  it('loads the current config into the form', async () => {
    vi.mocked(getAIConfig).mockResolvedValue(aiConfig())
    const wrapper = await mountPage()

    expect(wrapper.findComponent(Select).props('options')).toHaveLength(2)
    expect(wrapper.findComponent(Select).props('modelValue')).toBe('deepseek')
    expect(wrapper.findComponent(InputText).props('modelValue')).toBe('deepseek-chat')
    expect(wrapper.text()).toContain('当前密钥：sk-****abcd')
    expect(wrapper.find('input[placeholder="留空保留现有密钥"]').exists()).toBe(true)
  })

  it('shows the disabled notice and a disabled form when AI is not enabled (404)', async () => {
    vi.mocked(getAIConfig).mockRejectedValue(new ApiError(404, 40400, 'not found'))
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('AI 功能未启用，请联系管理员')
    expect(wrapper.findComponent(Select).exists()).toBe(false)
  })

  it('shows the key-missing notice and no form when the platform key is not configured', async () => {
    vi.mocked(getAIConfig).mockResolvedValue(
      aiConfig({
        configured: false,
        keyConfigured: false,
        provider: '',
        modelName: '',
        apiKeyMasked: '',
      }),
    )
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('管理员尚未配置 AI 加密密钥，请联系管理员')
    expect(wrapper.findComponent(Select).exists()).toBe(false)
    expect(saveAIConfig).not.toHaveBeenCalled()
  })

  it('requires a key on first save', async () => {
    vi.mocked(getAIConfig).mockResolvedValue(
      aiConfig({ configured: false, provider: '', modelName: '', apiKeyMasked: '' }),
    )
    const wrapper = await mountPage()

    expect(wrapper.find('input[placeholder="留空保留现有密钥"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('尚未配置 API Key')

    await wrapper.find('form').trigger('submit')
    expect(wrapper.text()).toContain('请选择模型服务商')
    expect(wrapper.text()).toContain('首次使用需填写 API Key')
    expect(saveAIConfig).not.toHaveBeenCalled()
  })

  it('omits a blank key on save so the existing key is preserved', async () => {
    vi.mocked(getAIConfig).mockResolvedValue(aiConfig())
    vi.mocked(saveAIConfig).mockResolvedValue(aiConfig())
    const wrapper = await mountPage()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(saveAIConfig).toHaveBeenCalledWith({
      provider: 'deepseek',
      modelName: 'deepseek-chat',
    })
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
  })

  it('fills the provider default model when the provider changes', async () => {
    vi.mocked(getAIConfig).mockResolvedValue(
      aiConfig({ configured: false, provider: '', modelName: '', apiKeyMasked: '' }),
    )
    const wrapper = await mountPage()

    wrapper.findComponent(Select).vm.$emit('update:modelValue', 'openai')
    await flushPromises()

    expect(wrapper.findComponent(InputText).props('modelValue')).toBe('gpt-4o-mini')
  })

  it('replaces the old provider default model when switching providers', async () => {
    vi.mocked(getAIConfig).mockResolvedValue(aiConfig())
    const wrapper = await mountPage()

    expect(wrapper.findComponent(InputText).props('modelValue')).toBe('deepseek-chat')

    wrapper.findComponent(Select).vm.$emit('update:modelValue', 'openai')
    await flushPromises()

    expect(wrapper.findComponent(Select).props('modelValue')).toBe('openai')
    expect(wrapper.findComponent(InputText).props('modelValue')).toBe('gpt-4o-mini')
  })

  it('preserves a user-customized model when switching providers', async () => {
    vi.mocked(getAIConfig).mockResolvedValue(aiConfig({ modelName: 'my-custom-model' }))
    const wrapper = await mountPage()

    wrapper.findComponent(Select).vm.$emit('update:modelValue', 'openai')
    await flushPromises()

    expect(wrapper.findComponent(InputText).props('modelValue')).toBe('my-custom-model')
  })

  it('submits the entered model and key and refreshes the view on success', async () => {
    vi.mocked(getAIConfig).mockResolvedValue(
      aiConfig({ configured: false, provider: '', modelName: '', apiKeyMasked: '' }),
    )
    vi.mocked(saveAIConfig).mockResolvedValue(
      aiConfig({
        provider: 'deepseek',
        modelName: 'deepseek-reasoner',
        apiKeyMasked: 'sk-****new',
      }),
    )
    const wrapper = await mountPage()

    wrapper.findComponent(Select).vm.$emit('update:modelValue', 'deepseek')
    await wrapper.findComponent(InputText).setValue('deepseek-reasoner')
    await wrapper.findComponent(Password).setValue('sk-custom-key')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(saveAIConfig).toHaveBeenCalledWith({
      provider: 'deepseek',
      modelName: 'deepseek-reasoner',
      apiKey: 'sk-custom-key',
    })
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
    expect(wrapper.text()).toContain('当前密钥：sk-****new')
  })
})
