import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import InputNumber from 'primevue/inputnumber'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { getAdminJudgeSettings, updateAdminJudgeSettings } from '@/api/admin'
import { ApiError } from '@/api/http'
import type { JudgeSettingsView } from '@/api/types'
import { PERM } from '@/permissions'
import AdminJudgeView from './AdminJudgeView.vue'

const MIB = 1024 * 1024

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const auth = vi.hoisted(() => ({
  can: vi.fn<(code: string) => boolean>(() => false),
}))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/admin', () => ({
  getAdminJudgeSettings: vi.fn(),
  updateAdminJudgeSettings: vi.fn(),
}))

function settingsView(overrides: Partial<JudgeSettingsView> = {}): JudgeSettingsView {
  return {
    outputFloorBytes: 1 * MIB,
    outputSlackBytes: 1 * MIB,
    maxOutputBytes: 16 * MIB,
    maxInputFileBytes: 16 * MIB,
    maxOutputFileBytes: 16 * MIB,
    maxBatchTotalBytes: 200 * MIB,
    maxProblemTotalBytes: 200 * MIB,
    updatedAt: '2026-11-21T00:00:00Z',
    ...overrides,
  }
}

async function mountPage() {
  const wrapper = mount(AdminJudgeView, { global: { plugins: [PrimeVue] } })
  await flushPromises()
  return wrapper
}

function numbers(wrapper: Awaited<ReturnType<typeof mountPage>>) {
  return wrapper.findAllComponents(InputNumber)
}

function buttons(wrapper: Awaited<ReturnType<typeof mountPage>>, label: string) {
  return wrapper.findAll('button').filter((item) => item.text().trim() === label)
}

beforeEach(() => {
  toast.add.mockReset()
  auth.can.mockReset()
  auth.can.mockReturnValue(false)
  vi.mocked(getAdminJudgeSettings).mockReset()
  vi.mocked(updateAdminJudgeSettings).mockReset()
  vi.mocked(getAdminJudgeSettings).mockResolvedValue(settingsView())
})

describe('AdminJudgeView', () => {
  it('loads defaults in MiB and saves bytes, then reloads from the server', async () => {
    auth.can.mockImplementation((code) => code === PERM.system.settings.manage)
    vi.mocked(updateAdminJudgeSettings).mockResolvedValue(settingsView())
    vi.mocked(getAdminJudgeSettings)
      .mockResolvedValueOnce(settingsView())
      .mockResolvedValueOnce(
        settingsView({
          outputFloorBytes: 2 * MIB,
          outputSlackBytes: 1.5 * MIB,
          maxOutputBytes: 64 * MIB,
          maxInputFileBytes: 64 * MIB,
          maxOutputFileBytes: 32 * MIB,
          maxBatchTotalBytes: 512 * MIB,
          maxProblemTotalBytes: 1024 * MIB,
        }),
      )

    const wrapper = await mountPage()

    expect(getAdminJudgeSettings).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('评测设置')
    expect(wrapper.text()).toContain('评测运行')
    expect(wrapper.text()).toContain('测试数据')
    expect(wrapper.text()).toContain('输出下限')
    expect(wrapper.text()).toContain('输出余量')
    expect(wrapper.text()).toContain('最大输出')
    expect(wrapper.text()).toContain('单个输入文件')
    expect(wrapper.text()).toContain('单个输出文件')
    expect(wrapper.text()).toContain('批量上传总量')
    expect(wrapper.text()).toContain('单题导入总量')
    expect(wrapper.text()).toContain('只影响后续提交')

    const inputs = numbers(wrapper)
    // Order matches the template: floor, slack, max output, input, output, batch, problem.
    expect(inputs.map((item) => item.props('modelValue'))).toEqual([1, 1, 16, 16, 16, 200, 200])
    expect(inputs.map((item) => item.props('step'))).toEqual([0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5])

    const edits = [2, 1.5, 32, 64, 32, 512, 1024]
    inputs.forEach((item, index) => item.vm.$emit('update:modelValue', edits[index]))
    await flushPromises()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateAdminJudgeSettings).toHaveBeenCalledWith({
      outputFloorBytes: 2 * MIB,
      outputSlackBytes: Math.round(1.5 * MIB),
      maxOutputBytes: 32 * MIB,
      maxInputFileBytes: 64 * MIB,
      maxOutputFileBytes: 32 * MIB,
      maxBatchTotalBytes: 512 * MIB,
      maxProblemTotalBytes: 1024 * MIB,
    })
    // Saving re-reads the server value so server-side rounding wins.
    expect(getAdminJudgeSettings).toHaveBeenCalledTimes(2)
    expect(numbers(wrapper)[2]!.props('modelValue')).toBe(64)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '评测设置已保存' }),
    )
    wrapper.unmount()
  })

  it('shows validation for floor > max output', async () => {
    auth.can.mockImplementation((code) => code === PERM.system.settings.manage)
    const wrapper = await mountPage()

    numbers(wrapper)[0]!.vm.$emit('update:modelValue', 8)
    numbers(wrapper)[2]!.vm.$emit('update:modelValue', 4)
    await flushPromises()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateAdminJudgeSettings).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('最大输出不能小于输出下限')
    wrapper.unmount()
  })

  it('validates ranges, slack and the output file relation', async () => {
    auth.can.mockImplementation((code) => code === PERM.system.settings.manage)
    const wrapper = await mountPage()

    const inputs = numbers(wrapper)
    inputs[0]!.vm.$emit('update:modelValue', 0.0005) // below 1 KiB
    inputs[1]!.vm.$emit('update:modelValue', 65) // above 64 MiB
    inputs[2]!.vm.$emit('update:modelValue', 16)
    inputs[3]!.vm.$emit('update:modelValue', 257) // above 256 MiB
    inputs[4]!.vm.$emit('update:modelValue', 32) // above max output
    inputs[5]!.vm.$emit('update:modelValue', 2049) // above 2048 MiB
    inputs[6]!.vm.$emit('update:modelValue', 0) // below 1 MiB
    await flushPromises()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateAdminJudgeSettings).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('输出下限必须在 1 KiB 到 64 MiB 之间')
    expect(wrapper.text()).toContain('输出余量必须在 0 到 64 MiB 之间')
    expect(wrapper.text()).toContain('单个输入文件上限必须在 1 到 256 MiB 之间')
    expect(wrapper.text()).toContain('单个输出文件上限不能超过最大输出')
    expect(wrapper.text()).toContain('批量上传总量必须在 1 到 2048 MiB 之间')
    expect(wrapper.text()).toContain('单题导入总量必须在 1 到 2048 MiB 之间')
    wrapper.unmount()
  })

  it('shows the server error message unchanged', async () => {
    auth.can.mockImplementation((code) => code === PERM.system.settings.manage)
    vi.mocked(updateAdminJudgeSettings).mockRejectedValue(
      new ApiError(400, 40001, '最大输出需为 1–64 MiB'),
    )
    const wrapper = await mountPage()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('最大输出需为 1–64 MiB')
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'error',
        summary: '保存失败',
        detail: '最大输出需为 1–64 MiB',
      }),
    )
    wrapper.unmount()
  })

  it('shows a load error and retries', async () => {
    vi.mocked(getAdminJudgeSettings).mockRejectedValueOnce(
      new ApiError(500, 50000, '评测设置服务暂不可用'),
    )
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('评测设置服务暂不可用')
    const retry = buttons(wrapper, '重试')
    expect(retry).toHaveLength(1)

    vi.mocked(getAdminJudgeSettings).mockResolvedValue(settingsView())
    await retry[0]!.trigger('click')
    await flushPromises()

    expect(getAdminJudgeSettings).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('评测运行')
    wrapper.unmount()
  })

  it('only offers the save action with system.settings.manage', async () => {
    let wrapper = await mountPage()
    expect(buttons(wrapper, '保存设置')).toHaveLength(0)
    wrapper.unmount()

    auth.can.mockImplementation((code) => code === PERM.system.settings.manage)
    wrapper = await mountPage()
    expect(auth.can).toHaveBeenCalledWith(PERM.system.settings.manage)
    expect(buttons(wrapper, '保存设置')).toHaveLength(1)
    wrapper.unmount()
  })
})
