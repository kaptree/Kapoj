import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Checkbox from 'primevue/checkbox'
import InputNumber from 'primevue/inputnumber'
import ToggleSwitch from 'primevue/toggleswitch'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import {
  createBadge,
  deleteBadge,
  listBadges,
  updateBadge,
  uploadBadgeImage,
} from '@/api/gamification'
import type { BadgeItem } from '@/api/types'
import BadgeAdminPanel from './BadgeAdminPanel.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/gamification', () => ({
  listBadges: vi.fn(),
  createBadge: vi.fn(),
  updateBadge: vi.fn(),
  deleteBadge: vi.fn(),
  uploadBadgeImage: vi.fn(),
}))

function badge(overrides: Partial<BadgeItem> = {}): BadgeItem {
  return {
    id: 1,
    name: '竞赛徽章',
    description: '比赛纪念',
    imageUrl: '',
    category: '竞赛',
    priceCoins: 100,
    isLimited: true,
    stock: 5,
    sortOrder: 1,
    isActive: true,
    owned: false,
    equipped: false,
    purchasedAt: null,
    ...overrides,
  }
}

async function mountPanel() {
  const wrapper = mount(BadgeAdminPanel, {
    global: { plugins: [PrimeVue] },
  })
  await flushPromises()
  return wrapper
}

function actionButton(wrapper: Awaited<ReturnType<typeof mountPanel>>, label: string, index = 0) {
  return wrapper.findAll('button').filter((item) => item.text().trim() === label)[index]
}

function dialog(): HTMLElement {
  return document.querySelector('.p-dialog') as HTMLElement
}

function dialogButton(label: string) {
  return Array.from(dialog().querySelectorAll('button')).find(
    (item) => item.textContent?.trim() === label,
  )
}

function setField(element: Element, value: string) {
  const proto =
    element instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype
  Object.getOwnPropertyDescriptor(proto, 'value')!.set!.call(element, value)
  element.dispatchEvent(new Event('input', { bubbles: true }))
}

beforeEach(() => {
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(listBadges).mockReset()
  vi.mocked(createBadge).mockReset()
  vi.mocked(updateBadge).mockReset()
  vi.mocked(deleteBadge).mockReset()
  vi.mocked(uploadBadgeImage).mockReset()

  Object.defineProperty(URL, 'createObjectURL', {
    value: vi.fn(() => 'blob:preview'),
    configurable: true,
  })
  Object.defineProperty(URL, 'revokeObjectURL', {
    value: vi.fn(),
    configurable: true,
  })

  vi.mocked(listBadges).mockResolvedValue([
    badge(),
    badge({ id: 2, name: '纪念徽章', category: '活动', isLimited: false, stock: 0 }),
  ])
  vi.mocked(createBadge).mockResolvedValue(badge({ id: 99 }))
  vi.mocked(updateBadge).mockResolvedValue(badge())
  vi.mocked(deleteBadge).mockResolvedValue(null)
  vi.mocked(uploadBadgeImage).mockResolvedValue(null)
})

describe('BadgeAdminPanel', () => {
  it('renders badges with stock and status', async () => {
    const wrapper = await mountPanel()

    expect(listBadges).toHaveBeenCalledWith({ includeInactive: true })
    expect(wrapper.text()).toContain('共 2 个徽章')
    expect(wrapper.text()).toContain('竞赛徽章')
    expect(wrapper.text()).toContain('纪念徽章')
    expect(wrapper.text()).toContain('100 金币')
    expect(wrapper.text()).toContain('5')
    expect(wrapper.text()).toContain('不限')
    expect(wrapper.text()).toContain('已上架')
    expect(wrapper.findAll('.p-datatable-tbody > tr')).toHaveLength(2)
    wrapper.unmount()
  })

  it('creates a limited badge and uploads the selected image', async () => {
    const wrapper = await mountPanel()
    await actionButton(wrapper, '新建徽章')!.trigger('click')
    await flushPromises()

    setField(dialog().querySelector('.badge-name-input')!, '新徽章')
    setField(dialog().querySelector('.badge-category-input')!, '活动')
    const numbers = () => wrapper.findAllComponents(InputNumber)
    numbers()
      .find((item) => item.props('inputId') === 'badge-price')!
      .vm.$emit('update:modelValue', 120)
    wrapper.findAllComponents(ToggleSwitch)[0]!.vm.$emit('update:modelValue', true)
    await nextTick()
    numbers()
      .find((item) => item.props('inputId') === 'badge-stock')!
      .vm.$emit('update:modelValue', 5)
    await nextTick()

    const file = new File(['image'], 'badge.png', { type: 'image/png' })
    const input = dialog().querySelector('.badge-file-input') as HTMLInputElement
    Object.defineProperty(input, 'files', { value: [file], configurable: true })
    input.dispatchEvent(new Event('change', { bubbles: true }))
    await nextTick()
    expect(dialog().textContent).toContain('待上传：badge.png')

    dialogButton('保存')!.click()
    await flushPromises()

    expect(createBadge).toHaveBeenCalledWith({
      name: '新徽章',
      description: '',
      category: '活动',
      priceCoins: 120,
      isLimited: true,
      stock: 5,
      sortOrder: 0,
      isActive: true,
    })
    expect(uploadBadgeImage).toHaveBeenCalledWith(99, file)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '徽章已创建' }),
    )
    expect(listBadges).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('keeps stock unchanged unless stock adjustment is enabled', async () => {
    const wrapper = await mountPanel()
    await actionButton(wrapper, '编辑', 0)!.trigger('click')
    await flushPromises()

    dialogButton('保存')!.click()
    await flushPromises()
    expect(updateBadge).toHaveBeenCalledWith(
      1,
      expect.not.objectContaining({ stock: expect.anything() }),
    )

    await actionButton(wrapper, '编辑', 0)!.trigger('click')
    await flushPromises()
    wrapper.findComponent(Checkbox).vm.$emit('update:modelValue', true)
    await nextTick()
    wrapper
      .findAllComponents(InputNumber)
      .find((item) => item.props('inputId') === 'badge-stock')!
      .vm.$emit('update:modelValue', 8)
    await nextTick()
    dialogButton('保存')!.click()
    await flushPromises()

    expect(updateBadge).toHaveBeenLastCalledWith(1, expect.objectContaining({ stock: 8 }))
    wrapper.unmount()
  })

  it('validates required fields', async () => {
    const wrapper = await mountPanel()
    await actionButton(wrapper, '新建徽章')!.trigger('click')
    await flushPromises()
    dialogButton('保存')!.click()
    await flushPromises()

    expect(createBadge).not.toHaveBeenCalled()
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'warn', summary: '请填写徽章名称' }),
    )
    wrapper.unmount()
  })

  it('confirms deletion with Chinese labels', async () => {
    const wrapper = await mountPanel()

    await actionButton(wrapper, '删除')!.trigger('click')
    const options = confirm.require.mock.calls[0]![0] as {
      header?: string
      message?: string
      acceptLabel?: string
      rejectLabel?: string
      accept?: () => void
    }
    expect(options.header).toBe('删除徽章')
    expect(options.message).toContain('竞赛徽章')
    expect(options.acceptLabel).toBe('删除')
    expect(options.rejectLabel).toBe('取消')

    options.accept?.()
    await flushPromises()
    expect(deleteBadge).toHaveBeenCalledWith(1)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '徽章已删除' }),
    )
    wrapper.unmount()
  })
})
