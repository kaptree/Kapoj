import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import InputNumber from 'primevue/inputnumber'
import ToggleSwitch from 'primevue/toggleswitch'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { getAdminRatingSettings, updateAdminRatingSettings } from '@/api/admin'
import { ApiError } from '@/api/http'
import type { RatingSettingsView } from '@/api/types'
import { PERM } from '@/permissions'
import AdminRatingView from './AdminRatingView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const auth = vi.hoisted(() => ({
  can: vi.fn<(code: string) => boolean>(() => false),
}))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/admin', () => ({
  getAdminRatingSettings: vi.fn(),
  updateAdminRatingSettings: vi.fn(),
}))

function settingsView(overrides: Partial<RatingSettingsView> = {}): RatingSettingsView {
  return {
    decayEnabled: true,
    decayThreshold: 2400,
    decayInactiveDays: 60,
    decayPercent: 3,
    decayMax: 100,
    seasonEnabled: true,
    seasonMonths: 6,
    seasonResetPercent: 30,
    seasonAnchor: '2026-11-01',
    currentSeason: {
      key: '2026-11-01',
      name: '2026-11-01 ~ 2027-04-30',
      startsAt: '2026-11-01T00:00:00Z',
      endsAt: '2027-05-01T00:00:00Z',
      nextResetAt: '2027-05-01T00:00:00Z',
    },
    updatedAt: '2026-11-01T00:00:00Z',
    ...overrides,
  }
}

async function mountPage() {
  const wrapper = mount(AdminRatingView, { global: { plugins: [PrimeVue] } })
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
  vi.mocked(getAdminRatingSettings).mockReset()
  vi.mocked(updateAdminRatingSettings).mockReset()
  vi.mocked(getAdminRatingSettings).mockResolvedValue(settingsView())
})

describe('AdminRatingView', () => {
  it('loads settings and current season', async () => {
    const wrapper = await mountPage()

    expect(getAdminRatingSettings).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('Rating 设置')
    expect(wrapper.text()).toContain('当前赛季')
    expect(wrapper.text()).toContain('2026-11-01 ~ 2027-04-30')
    expect(wrapper.text()).toContain('下次重置')
    expect(wrapper.text()).toContain('2027-05-01')
    wrapper.unmount()
  })

  it('saves valid settings and shows toast', async () => {
    auth.can.mockImplementation((code) => code === PERM.rating.settings.manage)
    vi.mocked(updateAdminRatingSettings).mockResolvedValue(settingsView())
    const wrapper = await mountPage()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateAdminRatingSettings).toHaveBeenCalledWith({
      decayEnabled: true,
      decayThreshold: 2400,
      decayInactiveDays: 60,
      decayPercent: 3,
      decayMax: 100,
      seasonEnabled: true,
      seasonMonths: 6,
      seasonResetPercent: 30,
      seasonAnchor: '2026-11-01',
    })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '设置已保存' }),
    )
    wrapper.unmount()
  })

  it('shows validation errors for out-of-range values', async () => {
    auth.can.mockImplementation((code) => code === PERM.rating.settings.manage)
    const wrapper = await mountPage()

    // Out-of-range threshold and a non-month-first anchor.
    wrapper.findAllComponents(InputNumber)[0]!.vm.$emit('update:modelValue', 100)
    wrapper.find('#season-anchor').setValue('2026-11-15')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateAdminRatingSettings).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('衰减阈值必须在 1500 到 4000 之间')
    expect(wrapper.text()).toContain('赛季锚点必须为某月 1 日')
    wrapper.unmount()
  })

  it('hidden/forbidden without rating.settings.manage', async () => {
    auth.can.mockReturnValue(false)
    const wrapper = await mountPage()

    // The page still loads (read-only) but exposes no save control.
    expect(getAdminRatingSettings).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('当前赛季')
    expect(buttons(wrapper, '保存设置')).toHaveLength(0)

    // With the permission the save control appears.
    wrapper.unmount()
    auth.can.mockImplementation((code) => code === PERM.rating.settings.manage)
    const allowed = await mountPage()
    expect(buttons(allowed, '保存设置')).toHaveLength(1)
    allowed.unmount()
  })

  it('shows a load error and retries', async () => {
    vi.mocked(getAdminRatingSettings).mockRejectedValueOnce(new ApiError(403, 40300, '没有权限'))
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('没有权限')
    const retry = buttons(wrapper, '重试')
    expect(retry).toHaveLength(1)

    vi.mocked(getAdminRatingSettings).mockResolvedValue(settingsView())
    await retry[0]!.trigger('click')
    await flushPromises()
    expect(getAdminRatingSettings).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('当前赛季')
    wrapper.unmount()
  })

  it('keeps the enabled switches wired to the form', async () => {
    auth.can.mockImplementation((code) => code === PERM.rating.settings.manage)
    vi.mocked(updateAdminRatingSettings).mockResolvedValue(settingsView())
    const wrapper = await mountPage()

    const switches = wrapper.findAllComponents(ToggleSwitch)
    // Two switches: decay enabled, season enabled.
    expect(switches).toHaveLength(2)
    switches[0]!.vm.$emit('update:modelValue', false)
    await flushPromises()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateAdminRatingSettings).toHaveBeenCalledWith(
      expect.objectContaining({ decayEnabled: false }),
    )
    wrapper.unmount()
  })

  it('sends numeric edits through the save payload', async () => {
    auth.can.mockImplementation((code) => code === PERM.rating.settings.manage)
    vi.mocked(updateAdminRatingSettings).mockResolvedValue(settingsView())
    const wrapper = await mountPage()

    const numbers = wrapper.findAllComponents(InputNumber)
    // Order matches template: threshold, inactiveDays, percent, max, months, resetPercent.
    numbers[0]!.vm.$emit('update:modelValue', 2600)
    numbers[3]!.vm.$emit('update:modelValue', 200)
    await flushPromises()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateAdminRatingSettings).toHaveBeenCalledWith(
      expect.objectContaining({ decayThreshold: 2600, decayMax: 200 }),
    )
    wrapper.unmount()
  })
})
