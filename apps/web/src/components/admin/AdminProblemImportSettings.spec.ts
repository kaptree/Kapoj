import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import InputNumber from 'primevue/inputnumber'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { getAdminProblemImportSettings, updateAdminProblemImportSettings } from '@/api/admin'
import { ApiError } from '@/api/http'
import type { ProblemImportSettings } from '@/api/types'
import AdminProblemImportSettings from './AdminProblemImportSettings.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('@/api/admin', () => ({
  getAdminProblemImportSettings: vi.fn(),
  updateAdminProblemImportSettings: vi.fn(),
}))

function view(overrides: Partial<ProblemImportSettings> = {}): ProblemImportSettings {
  return {
    maxPackageMb: 200,
    maxZipBytes: 200 << 20,
    maxUncompressedBytes: 1 << 30,
    updatedAt: '2026-11-20T00:00:00Z',
    ...overrides,
  }
}

async function mountPanel() {
  const wrapper = mount(AdminProblemImportSettings, { global: { plugins: [PrimeVue] } })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  toast.add.mockReset()
  vi.mocked(getAdminProblemImportSettings).mockReset()
  vi.mocked(updateAdminProblemImportSettings).mockReset()
  vi.mocked(getAdminProblemImportSettings).mockResolvedValue(view())
  vi.mocked(updateAdminProblemImportSettings).mockResolvedValue(view())
})

describe('AdminProblemImportSettings', () => {
  it('loads and displays the current limit and computed unpack limit', async () => {
    const wrapper = await mountPanel()
    expect(getAdminProblemImportSettings).toHaveBeenCalledTimes(1)
    expect(
      (wrapper.get('input[id="problem-import-max-mb"]').element as HTMLInputElement).value,
    ).toContain('200')
    expect(wrapper.text()).toContain('1 GB')
    wrapper.unmount()
  })

  it('saves a valid limit', async () => {
    const wrapper = await mountPanel()
    wrapper.findComponent(InputNumber).vm.$emit('update:modelValue', 2048)
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(updateAdminProblemImportSettings).toHaveBeenCalledWith({ maxPackageMb: 2048 })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ summary: '题包导入设置已保存' }),
    )
    wrapper.unmount()
  })

  it('rejects an out-of-range limit without calling the API', async () => {
    const wrapper = await mountPanel()
    wrapper.findComponent(InputNumber).vm.$emit('update:modelValue', 0)
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(updateAdminProblemImportSettings).not.toHaveBeenCalled()
    expect(wrapper.get('[data-test="problem-import-error"]').text()).toContain('1–2048')
    wrapper.unmount()
  })

  it('shows an API error', async () => {
    vi.mocked(getAdminProblemImportSettings).mockRejectedValue(
      new ApiError(500, 50000, '设置加载失败'),
    )
    const wrapper = await mountPanel()
    expect(wrapper.get('[data-test="problem-import-error"]').text()).toContain('设置加载失败')
    wrapper.unmount()
  })
})
