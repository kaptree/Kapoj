import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { getAdminAISettings, updateAdminAISettings } from '@/api/admin'
import { ApiError } from '@/api/http'
import type { AdminAISettingsView } from '@/api/types'
import { PERM } from '@/permissions'
import AdminAiView from './AdminAiView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const auth = vi.hoisted(() => ({
  can: vi.fn<(code: string) => boolean>(() => false),
}))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/admin', () => ({
  getAdminAISettings: vi.fn(),
  updateAdminAISettings: vi.fn(),
}))

function settingsView(overrides: Partial<AdminAISettingsView> = {}): AdminAISettingsView {
  return {
    enabled: true,
    keyConfigured: true,
    updatedAt: '2026-11-08T00:00:00Z',
    ...overrides,
  }
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
  auth.can.mockReset()
  auth.can.mockReturnValue(false)
  vi.mocked(getAdminAISettings).mockReset()
  vi.mocked(updateAdminAISettings).mockReset()
  vi.mocked(getAdminAISettings).mockResolvedValue(settingsView())
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
})
