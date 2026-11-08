import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Password from 'primevue/password'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  generateAdminAIKey,
  getAdminAIKey,
  getAdminAISettings,
  setAdminAIKey,
  updateAdminAISettings,
} from '@/api/admin'
import { ApiError } from '@/api/http'
import type { AIKeyGenerateResult, AIKeyView, AdminAISettingsView } from '@/api/types'
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
}))

function settingsView(overrides: Partial<AdminAISettingsView> = {}): AdminAISettingsView {
  return {
    enabled: true,
    keyConfigured: true,
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
  vi.mocked(getAdminAISettings).mockResolvedValue(settingsView())
  vi.mocked(getAdminAIKey).mockResolvedValue(keyView())
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

    expect(wrapper.text()).toContain('由环境变量管理')
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
})
