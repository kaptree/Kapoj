import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Password from 'primevue/password'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  createAdminAIPlatformModel,
  deleteAdminAIPlatformModel,
  generateAdminAIKey,
  getAdminAIKey,
  getAdminAISettings,
  listAdminAIPlatformModels,
  resetAdminAIKey,
  selectAdminAIPlatformModel,
  setAdminAIKey,
  testAdminAIPlatformModel,
  updateAdminAIPlatformModel,
  updateAdminAISettings,
} from '@/api/admin'
import { ApiError } from '@/api/http'
import type {
  AIKeyGenerateResult,
  AIKeyView,
  AdminAISettingsView,
  PlatformModelView,
} from '@/api/types'
import { PERM } from '@/permissions'
import AdminAiView from './AdminAiView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))
const clipboardWrite = vi.hoisted(() => vi.fn())
const auth = vi.hoisted(() => ({
  can: vi.fn<(code: string) => boolean>(() => false),
}))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/admin', () => ({
  getAdminAISettings: vi.fn(),
  updateAdminAISettings: vi.fn(),
  getAdminAIKey: vi.fn(),
  generateAdminAIKey: vi.fn(),
  setAdminAIKey: vi.fn(),
  resetAdminAIKey: vi.fn(),
  listAdminAIPlatformModels: vi.fn(),
  createAdminAIPlatformModel: vi.fn(),
  updateAdminAIPlatformModel: vi.fn(),
  deleteAdminAIPlatformModel: vi.fn(),
  testAdminAIPlatformModel: vi.fn(),
  selectAdminAIPlatformModel: vi.fn(),
}))

function settingsView(overrides: Partial<AdminAISettingsView> = {}): AdminAISettingsView {
  return {
    enabled: true,
    keyConfigured: true,
    platformModelId: null,
    platformModelName: '',
    updatedAt: '2026-11-08T00:00:00Z',
    ...overrides,
  }
}

function platformModelView(overrides: Partial<PlatformModelView> = {}): PlatformModelView {
  return {
    id: 1,
    name: '平台模型 A',
    baseUrl: 'https://api.example.com/v1',
    modelName: 'deepseek-chat',
    enabled: true,
    apiKeyMasked: 'sk-…abcd',
    lastTestAt: null,
    lastTestOk: false,
    lastTestError: '',
    lastTestLatencyMs: 0,
    createdAt: '2026-11-08T00:00:00Z',
    updatedAt: '2026-11-08T00:00:00Z',
    ...overrides,
  }
}

function keyView(overrides: Partial<AIKeyView> = {}): AIKeyView {
  return {
    configured: true,
    source: 'file',
    masked: 'abc…wxyz',
    fingerprint: 'deadbeefcafe',
    ...overrides,
  }
}

function generateResult(overrides: Partial<AIKeyGenerateResult> = {}): AIKeyGenerateResult {
  return { ...keyView(), key: 'BASE64SECRET', ...overrides }
}

async function mountPage() {
  const wrapper = mount(AdminAiView, { global: { plugins: [PrimeVue] } })
  await flushPromises()
  return wrapper
}

function buttons(wrapper: Awaited<ReturnType<typeof mountPage>>, label: string) {
  return wrapper.findAll('button').filter((item) => item.text().trim() === label)
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

beforeEach(() => {
  toast.add.mockReset()
  confirm.require.mockReset()
  clipboardWrite.mockReset()
  clipboardWrite.mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText: clipboardWrite },
  })
  auth.can.mockReset()
  auth.can.mockReturnValue(false)
  vi.mocked(getAdminAISettings).mockReset()
  vi.mocked(updateAdminAISettings).mockReset()
  vi.mocked(getAdminAIKey).mockReset()
  vi.mocked(generateAdminAIKey).mockReset()
  vi.mocked(setAdminAIKey).mockReset()
  vi.mocked(resetAdminAIKey).mockReset()
  vi.mocked(listAdminAIPlatformModels).mockReset()
  vi.mocked(createAdminAIPlatformModel).mockReset()
  vi.mocked(updateAdminAIPlatformModel).mockReset()
  vi.mocked(deleteAdminAIPlatformModel).mockReset()
  vi.mocked(testAdminAIPlatformModel).mockReset()
  vi.mocked(selectAdminAIPlatformModel).mockReset()
  vi.mocked(getAdminAISettings).mockResolvedValue(settingsView())
  vi.mocked(getAdminAIKey).mockResolvedValue(keyView())
  vi.mocked(listAdminAIPlatformModels).mockResolvedValue([])
  document.body.innerHTML = ''
})

describe('AdminAiView', () => {
  it('loads settings and renders enabled status', async () => {
    const wrapper = await mountPage()

    expect(getAdminAISettings).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('AI 设置')
    expect(wrapper.text()).toContain('已启用')
    wrapper.unmount()
  })

  it('toggles and saves enabled/disabled with toast', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(updateAdminAISettings).mockResolvedValue(settingsView({ enabled: false }))
    const wrapper = await mountPage()

    wrapper.findAllComponents(ToggleSwitch)[0]!.vm.$emit('update:modelValue', false)
    await flushPromises()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateAdminAISettings).toHaveBeenCalledWith({ enabled: false })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '设置已保存' }),
    )
    expect(wrapper.text()).toContain('已关闭')
    wrapper.unmount()
  })

  it('renders unavailable status when key is not configured', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(getAdminAISettings).mockResolvedValue(
      settingsView({ enabled: true, keyConfigured: false }),
    )
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('未配置 AI 加密密钥，无法启用')
    expect(wrapper.text()).toContain('关闭仍可保存')

    // Effective status is "unavailable" while the toggle stays interactive.
    expect(wrapper.findComponent(Tag).props('value')).toBe('不可用')
    expect(wrapper.findComponent(Tag).props('severity')).toBe('secondary')
    expect(wrapper.findComponent(ToggleSwitch).props('disabled')).toBe(false)
    wrapper.unmount()
  })

  it('allows disabling and saving without a configured key', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(getAdminAISettings).mockResolvedValue(
      settingsView({ enabled: true, keyConfigured: false }),
    )
    vi.mocked(updateAdminAISettings).mockResolvedValue(
      settingsView({ enabled: false, keyConfigured: false }),
    )
    const wrapper = await mountPage()

    wrapper.findAllComponents(ToggleSwitch)[0]!.vm.$emit('update:modelValue', false)
    await flushPromises()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateAdminAISettings).toHaveBeenCalledWith({ enabled: false })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '设置已保存' }),
    )
    wrapper.unmount()
  })

  it('disables save when enabling without a configured key', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(getAdminAISettings).mockResolvedValue(
      settingsView({ enabled: true, keyConfigured: false }),
    )
    const wrapper = await mountPage()

    // enabled=true without a key → save blocked (cannot enable).
    expect((buttons(wrapper, '保存设置')[0]!.element as HTMLButtonElement).disabled).toBe(true)

    // Flip to false → closing is allowed and save re-enables.
    wrapper.findAllComponents(ToggleSwitch)[0]!.vm.$emit('update:modelValue', false)
    await flushPromises()
    expect((buttons(wrapper, '保存设置')[0]!.element as HTMLButtonElement).disabled).toBe(false)
    wrapper.unmount()
  })

  it('shows error state and retry', async () => {
    vi.mocked(getAdminAISettings).mockRejectedValueOnce(new ApiError(403, 40300, '没有权限'))
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('没有权限')
    const retry = buttons(wrapper, '重试')
    expect(retry).toHaveLength(1)

    vi.mocked(getAdminAISettings).mockResolvedValue(settingsView())
    await retry[0]!.trigger('click')
    await flushPromises()
    expect(getAdminAISettings).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('已启用')
    wrapper.unmount()
  })

  it('loads the key view and renders configured file status with masked/fingerprint', async () => {
    vi.mocked(getAdminAIKey).mockResolvedValue(
      keyView({
        configured: true,
        source: 'file',
        masked: 'abc…wxyz',
        fingerprint: 'deadbeefcafe',
      }),
    )
    const wrapper = await mountPage()

    expect(getAdminAIKey).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('加密密钥')
    expect(wrapper.text()).toContain('已配置（文件）')
    expect(wrapper.text()).toContain('abc…wxyz')
    expect(wrapper.text()).toContain('deadbeefcafe')
    wrapper.unmount()
  })

  it('shows generate and manual entry when no key is configured', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(getAdminAIKey).mockResolvedValue(
      keyView({ configured: false, source: 'none', masked: '', fingerprint: '' }),
    )
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('未配置')
    expect(buttons(wrapper, '自动生成密钥')).toHaveLength(1)
    expect(wrapper.text()).toContain('手动填写密钥')
    wrapper.unmount()
  })

  it('renders env source as read-only with no generate/manual actions', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(getAdminAIKey).mockResolvedValue(
      keyView({ configured: true, source: 'env', masked: 'env…1234', fingerprint: 'fp' }),
    )
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('由部署配置管理')
    expect(wrapper.text()).toContain('KAPOJ_AI_ENCRYPTION_KEY')
    expect(buttons(wrapper, '自动生成密钥')).toHaveLength(0)
    expect(buttons(wrapper, '重新生成')).toHaveLength(0)
    expect(wrapper.text()).not.toContain('手动填写密钥')
    wrapper.unmount()
  })

  it('generates a key and shows the one-time key with copy and warning', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(getAdminAIKey).mockResolvedValue(
      keyView({ configured: false, source: 'none', masked: '', fingerprint: '' }),
    )
    vi.mocked(generateAdminAIKey).mockResolvedValue(generateResult({ key: 'BASE64SECRET' }))

    const wrapper = await mountPage()
    await buttons(wrapper, '自动生成密钥')[0]!.trigger('click')
    await flushPromises()

    expect(generateAdminAIKey).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('请立即备份')
    const input = wrapper.find('input[readonly]')
    expect(input.exists()).toBe(true)
    expect((input.element as HTMLInputElement).value).toBe('BASE64SECRET')
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
    wrapper.unmount()
  })

  it('confirms before regenerating an existing file key', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(getAdminAIKey).mockResolvedValue(keyView({ configured: true, source: 'file' }))
    vi.mocked(generateAdminAIKey).mockResolvedValue(generateResult())

    const wrapper = await mountPage()
    await buttons(wrapper, '重新生成')[0]!.trigger('click')
    await flushPromises()

    expect(generateAdminAIKey).not.toHaveBeenCalled()
    expect(confirm.require).toHaveBeenCalledTimes(1)
    const options = confirm.require.mock.calls[0]![0] as { accept?: () => void }
    options.accept?.()
    await flushPromises()
    expect(generateAdminAIKey).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('saves a manually entered key when none exists', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(getAdminAIKey).mockResolvedValue(
      keyView({ configured: false, source: 'none', masked: '', fingerprint: '' }),
    )
    vi.mocked(setAdminAIKey).mockResolvedValue(keyView({ configured: true, source: 'file' }))

    const wrapper = await mountPage()
    await wrapper.findComponent(Password).setValue('BASE64SECRET')
    await wrapper.findAll('form')[1]!.trigger('submit')
    await flushPromises()

    expect(setAdminAIKey).toHaveBeenCalledWith({ key: 'BASE64SECRET' })
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
    wrapper.unmount()
  })

  it('confirms before manually replacing an existing file key', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(getAdminAIKey).mockResolvedValue(keyView({ configured: true, source: 'file' }))
    vi.mocked(setAdminAIKey).mockResolvedValue(keyView({ configured: true, source: 'file' }))

    const wrapper = await mountPage()
    await wrapper.findComponent(Password).setValue('NEWSECRET')
    await wrapper.findAll('form')[1]!.trigger('submit')
    await flushPromises()

    expect(setAdminAIKey).not.toHaveBeenCalled()
    expect(confirm.require).toHaveBeenCalledTimes(1)
    const options = confirm.require.mock.calls[0]![0] as { accept?: () => void }
    options.accept?.()
    await flushPromises()
    expect(setAdminAIKey).toHaveBeenCalledWith({ key: 'NEWSECRET' })
    wrapper.unmount()
  })

  it('copies the generated key to the clipboard and hides it on dismiss', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(getAdminAIKey).mockResolvedValue(
      keyView({ configured: false, source: 'none', masked: '', fingerprint: '' }),
    )
    vi.mocked(generateAdminAIKey).mockResolvedValue(generateResult({ key: 'SECRET' }))

    const wrapper = await mountPage()
    await buttons(wrapper, '自动生成密钥')[0]!.trigger('click')
    await flushPromises()

    await buttons(wrapper, '复制')[0]!.trigger('click')
    await flushPromises()
    expect(clipboardWrite).toHaveBeenCalledWith('SECRET')
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ summary: '已复制' }))

    await buttons(wrapper, '隐藏')[0]!.trigger('click')
    await flushPromises()
    expect(wrapper.find('input[readonly]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows a key load error and retries', async () => {
    vi.mocked(getAdminAIKey).mockRejectedValueOnce(new ApiError(500, 50000, '密钥状态加载失败'))
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('密钥状态加载失败')
    const retry = buttons(wrapper, '重试')
    expect(retry).toHaveLength(1)

    vi.mocked(getAdminAIKey).mockResolvedValue(keyView())
    await retry[0]!.trigger('click')
    await flushPromises()
    expect(getAdminAIKey).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('已配置（文件）')
    wrapper.unmount()
  })

  it('shows the destructive reset action for file/none sources', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)

    const fileWrapper = await mountPage()
    expect(fileWrapper.find('[data-test="reset-ai-key"]').exists()).toBe(true)
    fileWrapper.unmount()

    vi.mocked(getAdminAIKey).mockResolvedValue(
      keyView({ configured: false, source: 'none', masked: '', fingerprint: '' }),
    )
    const noneWrapper = await mountPage()
    expect(noneWrapper.find('[data-test="reset-ai-key"]').exists()).toBe(true)
    noneWrapper.unmount()
  })

  it('hides the reset action for env source', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(getAdminAIKey).mockResolvedValue(
      keyView({ configured: true, source: 'env', masked: 'env…1234', fingerprint: 'fp' }),
    )
    const wrapper = await mountPage()

    expect(wrapper.find('[data-test="reset-ai-key"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('confirms before resetting and shows the one-time key', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(getAdminAIKey).mockResolvedValue(keyView({ configured: true, source: 'file' }))
    vi.mocked(resetAdminAIKey).mockResolvedValue(generateResult({ key: 'NEWSECRET' }))

    const wrapper = await mountPage()
    await wrapper.find('[data-test="reset-ai-key"]').trigger('click')
    await flushPromises()

    expect(confirm.require).toHaveBeenCalledTimes(1)
    expect(resetAdminAIKey).not.toHaveBeenCalled()

    const options = confirm.require.mock.calls[0]![0] as { accept?: () => void }
    options.accept?.()
    await flushPromises()

    expect(resetAdminAIKey).toHaveBeenCalledTimes(1)
    const input = wrapper.find('input[readonly]')
    expect(input.exists()).toBe(true)
    expect((input.element as HTMLInputElement).value).toBe('NEWSECRET')
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '旧密钥已清除' }),
    )
    expect(getAdminAISettings).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('cancels reset without calling the API', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(getAdminAIKey).mockResolvedValue(keyView({ configured: true, source: 'file' }))

    const wrapper = await mountPage()
    await wrapper.find('[data-test="reset-ai-key"]').trigger('click')
    await flushPromises()

    expect(confirm.require).toHaveBeenCalledTimes(1)
    expect(resetAdminAIKey).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('shows an error toast when reset fails', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(getAdminAIKey).mockResolvedValue(keyView({ configured: true, source: 'file' }))
    vi.mocked(resetAdminAIKey).mockRejectedValue(new ApiError(500, 50000, '重置失败'))

    const wrapper = await mountPage()
    await wrapper.find('[data-test="reset-ai-key"]').trigger('click')
    await flushPromises()

    const options = confirm.require.mock.calls[0]![0] as { accept?: () => void }
    options.accept?.()
    await flushPromises()

    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'error',
        summary: '重置失败',
        detail: expect.stringContaining('重置失败'),
      }),
    )
    wrapper.unmount()
  })

  it('clears a stale one-time key when reset fails', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(getAdminAIKey).mockResolvedValue(
      keyView({ configured: false, source: 'none', masked: '', fingerprint: '' }),
    )
    vi.mocked(generateAdminAIKey).mockResolvedValue(generateResult({ key: 'OLDSECRET' }))
    vi.mocked(resetAdminAIKey).mockRejectedValue(new ApiError(500, 50000, '重置失败'))

    const wrapper = await mountPage()
    await buttons(wrapper, '自动生成密钥')[0]!.trigger('click')
    await flushPromises()
    expect((wrapper.find('input[readonly]').element as HTMLInputElement).value).toBe('OLDSECRET')

    await wrapper.find('[data-test="reset-ai-key"]').trigger('click')
    await flushPromises()
    const options = confirm.require.mock.calls[0]![0] as { accept?: () => void }
    options.accept?.()
    await flushPromises()

    expect(wrapper.find('input[readonly]').exists()).toBe(false)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'error', summary: '重置失败' }),
    )
    wrapper.unmount()
  })

  it('lists platform models and saves the current selection', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(listAdminAIPlatformModels).mockResolvedValue([
      platformModelView({
        id: 1,
        name: '模型甲',
        baseUrl: 'https://a.example/v1',
        modelName: 'model-a',
      }),
      platformModelView({
        id: 2,
        name: '模型乙',
        baseUrl: 'https://b.example/v1',
        modelName: 'model-b',
        lastTestAt: '2026-11-10T08:00:00Z',
        lastTestOk: true,
        lastTestLatencyMs: 320,
      }),
    ])
    vi.mocked(selectAdminAIPlatformModel).mockResolvedValue(
      settingsView({ platformModelId: 2, platformModelName: '模型乙' }),
    )

    const wrapper = await mountPage()

    expect(listAdminAIPlatformModels).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('平台模型')
    expect(wrapper.text()).toContain('模型甲')
    expect(wrapper.text()).toContain('https://b.example/v1')
    expect(wrapper.text()).toContain('model-b')
    expect(wrapper.text()).toContain('未测试')

    wrapper.findComponent(Select).vm.$emit('update:modelValue', 2)
    await flushPromises()
    await wrapper.find('[data-test="save-platform-selection"]').trigger('click')
    await flushPromises()

    expect(selectAdminAIPlatformModel).toHaveBeenCalledWith(2)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '当前模型已更新' }),
    )
    wrapper.unmount()
  })

  it('clears the current platform model selection', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(getAdminAISettings).mockResolvedValue(
      settingsView({ platformModelId: 2, platformModelName: '模型乙' }),
    )
    vi.mocked(listAdminAIPlatformModels).mockResolvedValue([
      platformModelView({ id: 2, name: '模型乙' }),
    ])
    vi.mocked(selectAdminAIPlatformModel).mockResolvedValue(settingsView())

    const wrapper = await mountPage()

    wrapper.findComponent(Select).vm.$emit('update:modelValue', null)
    await flushPromises()
    await wrapper.find('[data-test="save-platform-selection"]').trigger('click')
    await flushPromises()

    expect(selectAdminAIPlatformModel).toHaveBeenCalledWith(null)
    wrapper.unmount()
  })

  it('disables adding and editing platform models when the encryption key is missing', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(getAdminAISettings).mockResolvedValue(
      settingsView({ enabled: true, keyConfigured: false }),
    )
    vi.mocked(listAdminAIPlatformModels).mockResolvedValue([
      platformModelView({ id: 4, name: '待编辑模型' }),
    ])

    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('未配置 AI 加密密钥，无法新增或编辑平台模型')
    const add = wrapper.find('[data-test="add-platform-model"]')
    expect(add.exists()).toBe(true)
    expect((add.element as HTMLButtonElement).disabled).toBe(true)
    const edit = wrapper.find('[data-test="edit-platform-model-4"]')
    expect(edit.exists()).toBe(true)
    expect((edit.element as HTMLButtonElement).disabled).toBe(true)

    await add.trigger('click')
    await flushPromises()
    expect(document.querySelector('.p-dialog')).toBeNull()
    wrapper.unmount()
  })

  it('creates a platform model from the dialog', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(createAdminAIPlatformModel).mockResolvedValue(platformModelView({ id: 11 }))

    const wrapper = await mountPage()
    await wrapper.find('[data-test="add-platform-model"]').trigger('click')
    await flushPromises()
    expect(dialog()).toBeTruthy()

    setField(dialog().querySelector('#platform-model-name')!, '新模型')
    setField(dialog().querySelector('#platform-model-base-url')!, 'https://api.example.com/v1')
    setField(dialog().querySelector('#platform-model-model-name')!, 'deepseek-chat')
    setField(dialog().querySelector('#platform-model-api-key')!, 'sk-test')
    dialogButton('创建').click()
    await flushPromises()

    expect(createAdminAIPlatformModel).toHaveBeenCalledWith({
      name: '新模型',
      baseUrl: 'https://api.example.com/v1',
      modelName: 'deepseek-chat',
      apiKey: 'sk-test',
      enabled: true,
    })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '模型已创建' }),
    )
    expect(listAdminAIPlatformModels).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('edits a platform model and keeps the token when the API Key is blank', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(listAdminAIPlatformModels).mockResolvedValue([
      platformModelView({
        id: 5,
        name: '旧名称',
        baseUrl: 'https://old.example/v1',
        modelName: 'old-model',
        apiKeyMasked: 'sk-…wxyz',
      }),
    ])
    vi.mocked(updateAdminAIPlatformModel).mockResolvedValue(
      platformModelView({ id: 5, name: '新名称' }),
    )

    const wrapper = await mountPage()
    await wrapper.find('[data-test="edit-platform-model-5"]').trigger('click')
    await flushPromises()

    expect((dialog().querySelector('#platform-model-name') as HTMLInputElement).value).toBe(
      '旧名称',
    )
    expect(dialog().textContent).toContain('sk-…wxyz')

    setField(dialog().querySelector('#platform-model-name')!, '新名称')
    dialogButton('保存').click()
    await flushPromises()

    expect(updateAdminAIPlatformModel).toHaveBeenCalledWith(5, {
      name: '新名称',
      baseUrl: 'https://old.example/v1',
      modelName: 'old-model',
      enabled: true,
    })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '模型已保存' }),
    )
    wrapper.unmount()
  })

  it('validates the platform model form before saving', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)

    const wrapper = await mountPage()
    await wrapper.find('[data-test="add-platform-model"]').trigger('click')
    await flushPromises()

    dialogButton('创建').click()
    await flushPromises()

    expect(createAdminAIPlatformModel).not.toHaveBeenCalled()
    expect(dialog().textContent).toContain('请填写模型名称')
    expect(dialog().textContent).toContain('请填写接口地址')
    expect(dialog().textContent).toContain('请填写模型名')
    expect(dialog().textContent).toContain('请填写 API Key')

    setField(dialog().querySelector('#platform-model-name')!, '新模型')
    setField(dialog().querySelector('#platform-model-base-url')!, 'ftp://api.example.com')
    setField(dialog().querySelector('#platform-model-model-name')!, 'deepseek-chat')
    setField(dialog().querySelector('#platform-model-api-key')!, 'sk-test')
    dialogButton('创建').click()
    await flushPromises()

    expect(createAdminAIPlatformModel).not.toHaveBeenCalled()
    expect(dialog().textContent).toContain('接口地址必须以 http:// 或 https:// 开头')
    wrapper.unmount()
  })

  it('tests a platform model and shows a success result', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    const model = platformModelView({ id: 3, name: '测试模型' })
    const tested = platformModelView({
      ...model,
      lastTestAt: '2026-11-18T01:00:00Z',
      lastTestOk: true,
      lastTestLatencyMs: 812,
    })
    vi.mocked(listAdminAIPlatformModels).mockResolvedValueOnce([model]).mockResolvedValue([tested])

    let resolveTest!: (result: { ok: boolean; latencyMs: number; message: string }) => void
    vi.mocked(testAdminAIPlatformModel).mockReturnValue(
      new Promise((resolve) => {
        resolveTest = resolve
      }),
    )

    const wrapper = await mountPage()
    const testButton = wrapper.find('[data-test="test-platform-model-3"]')
    await testButton.trigger('click')
    await flushPromises()

    expect(testAdminAIPlatformModel).toHaveBeenCalledWith(3)
    expect(testButton.classes()).toContain('p-button-loading')

    resolveTest({ ok: true, latencyMs: 812, message: '连接成功' })
    await flushPromises()

    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'success',
        summary: '测试通过',
        detail: expect.stringContaining('812'),
      }),
    )
    expect(listAdminAIPlatformModels).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('812 ms')
    expect(wrapper.text()).toContain('成功')
    wrapper.unmount()
  })

  it('shows a failure result when the platform model test fails', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    const model = platformModelView({ id: 6, name: '故障模型' })
    vi.mocked(listAdminAIPlatformModels)
      .mockResolvedValueOnce([model])
      .mockResolvedValue([
        platformModelView({
          ...model,
          lastTestAt: '2026-11-18T01:00:00Z',
          lastTestOk: false,
          lastTestError: '连接超时',
          lastTestLatencyMs: 10000,
        }),
      ])
    vi.mocked(testAdminAIPlatformModel).mockResolvedValue({
      ok: false,
      latencyMs: 10000,
      message: '连接超时',
    })

    const wrapper = await mountPage()
    await wrapper.find('[data-test="test-platform-model-6"]').trigger('click')
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
    wrapper.unmount()
  })

  it('deletes a platform model after confirmation', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(listAdminAIPlatformModels).mockResolvedValue([
      platformModelView({ id: 7, name: '可删除模型' }),
    ])
    vi.mocked(deleteAdminAIPlatformModel).mockResolvedValue(null)

    const wrapper = await mountPage()
    await wrapper.find('[data-test="delete-platform-model-7"]').trigger('click')
    await flushPromises()

    expect(confirm.require).toHaveBeenCalledTimes(1)
    expect(deleteAdminAIPlatformModel).not.toHaveBeenCalled()

    const options = confirm.require.mock.calls[0]![0] as { accept?: () => void }
    options.accept?.()
    await flushPromises()

    expect(deleteAdminAIPlatformModel).toHaveBeenCalledWith(7)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '模型已删除' }),
    )
    expect(listAdminAIPlatformModels).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('disables deleting the currently selected platform model with a hint', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(getAdminAISettings).mockResolvedValue(
      settingsView({ platformModelId: 9, platformModelName: '当前模型' }),
    )
    vi.mocked(listAdminAIPlatformModels).mockResolvedValue([
      platformModelView({ id: 9, name: '当前模型' }),
      platformModelView({ id: 10, name: '备用模型' }),
    ])

    const wrapper = await mountPage()

    const selectedDelete = wrapper.find('[data-test="delete-platform-model-9"]')
    expect((selectedDelete.element as HTMLButtonElement).disabled).toBe(true)
    expect(wrapper.text()).toContain('先切换')
    const otherDelete = wrapper.find('[data-test="delete-platform-model-10"]')
    expect((otherDelete.element as HTMLButtonElement).disabled).toBe(false)

    await selectedDelete.trigger('click')
    await flushPromises()
    expect(confirm.require).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('shows the empty state when there are no platform models', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)

    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('暂无平台模型')
    wrapper.unmount()
  })

  it('shows a platform model load error and retries', async () => {
    auth.can.mockImplementation((code) => code === PERM.ai.settings.manage)
    vi.mocked(listAdminAIPlatformModels).mockRejectedValueOnce(
      new ApiError(500, 50000, '平台模型加载失败'),
    )

    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('平台模型加载失败')
    const retry = buttons(wrapper, '重试')
    expect(retry).toHaveLength(1)

    vi.mocked(listAdminAIPlatformModels).mockResolvedValue([
      platformModelView({ id: 12, name: '恢复模型' }),
    ])
    await retry[0]!.trigger('click')
    await flushPromises()

    expect(listAdminAIPlatformModels).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('恢复模型')
    wrapper.unmount()
  })
})
