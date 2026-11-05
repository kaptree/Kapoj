import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import ColorPicker from 'primevue/colorpicker'
import InputNumber from 'primevue/inputnumber'
import Select from 'primevue/select'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import {
  createTitle,
  deleteTitle,
  listAchievements,
  listTitles,
  updateTitle,
} from '@/api/gamification'
import type { AchievementItem, TitleItem } from '@/api/types'
import TitleAdminPanel from './TitleAdminPanel.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/gamification', () => ({
  listTitles: vi.fn(),
  listAchievements: vi.fn(),
  createTitle: vi.fn(),
  updateTitle: vi.fn(),
  deleteTitle: vi.fn(),
}))

function title(overrides: Partial<TitleItem> = {}): TitleItem {
  return {
    id: 1,
    name: '荣耀王者',
    color: '#7C3AED',
    unlockConditionType: 'rating',
    unlockConditionConfig: { rating: 1600 },
    priceCoins: 100,
    sortOrder: 1,
    isActive: true,
    unlocked: false,
    unlockedAt: null,
    purchased: false,
    purchasedAt: null,
    equipped: false,
    progress: { type: 'rating', current: 0, target: 1600, done: false, achievementCode: '' },
    ...overrides,
  }
}

function achievement(): AchievementItem {
  return {
    id: 1,
    code: 'ac_100',
    name: '百题斩',
    description: '',
    iconUrl: '',
    conditionType: 'ac_count',
    conditionConfig: { threshold: 100 },
    rewardCoins: 0,
    rewardBadgeId: null,
    sortOrder: 1,
    isActive: true,
    unlocked: false,
    unlockedAt: null,
    progress: [],
  }
}

async function mountPanel() {
  const wrapper = mount(TitleAdminPanel, {
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
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(element, value)
  element.dispatchEvent(new Event('input', { bubbles: true }))
}

beforeEach(() => {
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(listTitles).mockReset()
  vi.mocked(listAchievements).mockReset()
  vi.mocked(createTitle).mockReset()
  vi.mocked(updateTitle).mockReset()
  vi.mocked(deleteTitle).mockReset()

  vi.mocked(listTitles).mockResolvedValue([
    title(),
    title({
      id: 2,
      name: '题海行者',
      color: '#2563EB',
      unlockConditionType: 'ac_count',
      unlockConditionConfig: { threshold: 100 },
      priceCoins: 0,
      sortOrder: 2,
      isActive: false,
    }),
    title({
      id: 3,
      name: '成就收藏家',
      color: '#9333EA',
      unlockConditionType: 'achievement',
      unlockConditionConfig: { achievementCode: 'ac_100' },
      priceCoins: 50,
      sortOrder: 3,
    }),
  ])
  vi.mocked(listAchievements).mockResolvedValue([achievement()])
  vi.mocked(createTitle).mockResolvedValue(title({ id: 99 }))
  vi.mocked(updateTitle).mockResolvedValue(title())
  vi.mocked(deleteTitle).mockResolvedValue(null)
})

describe('TitleAdminPanel', () => {
  it('renders titles, conditions, prices and status', async () => {
    const wrapper = await mountPanel()

    expect(listTitles).toHaveBeenCalledWith({ includeInactive: true })
    expect(listAchievements).toHaveBeenCalledWith({ includeInactive: true })
    expect(wrapper.text()).toContain('共 3 个称号')
    expect(wrapper.text()).toContain('荣耀王者')
    expect(wrapper.text()).toContain('Rating ≥ 1600')
    expect(wrapper.text()).toContain('题海行者')
    expect(wrapper.text()).toContain('AC 题目数 ≥ 100')
    expect(wrapper.text()).toContain('成就收藏家')
    expect(wrapper.text()).toContain('成就 ac_100（百题斩）')
    expect(wrapper.text()).toContain('100 金币')
    expect(wrapper.text()).toContain('已上架')
    expect(wrapper.text()).toContain('已下线')
    expect(wrapper.findAll('.p-datatable-tbody > tr')).toHaveLength(3)
    wrapper.unmount()
  })

  it('creates a rating title', async () => {
    const wrapper = await mountPanel()
    await actionButton(wrapper, '新建称号')!.trigger('click')
    await flushPromises()

    setField(dialog().querySelector('#title-name')!, '新称号')
    wrapper.findComponent(ColorPicker).vm.$emit('update:modelValue', '#112233')
    wrapper.findAllComponents(Select)[0]!.vm.$emit('update:modelValue', 'rating')
    await nextTick()
    wrapper
      .findAllComponents(InputNumber)
      .find((item) => item.props('inputId') === 'title-rating')!
      .vm.$emit('update:modelValue', 1700)
    wrapper
      .findAllComponents(InputNumber)
      .find((item) => item.props('inputId') === 'title-price')!
      .vm.$emit('update:modelValue', 80)
    dialogButton('保存')!.click()
    await flushPromises()

    expect(createTitle).toHaveBeenCalledWith({
      name: '新称号',
      color: '#112233',
      unlockConditionType: 'rating',
      unlockConditionConfig: { rating: 1700 },
      priceCoins: 80,
      sortOrder: 0,
      isActive: true,
    })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '称号已创建' }),
    )
    wrapper.unmount()
  })

  it('edits an achievement-unlocked title', async () => {
    const wrapper = await mountPanel()
    await actionButton(wrapper, '编辑', 2)!.trigger('click')
    await flushPromises()

    expect(dialog().textContent).toContain('解锁类型')
    dialogButton('保存')!.click()
    await flushPromises()

    expect(updateTitle).toHaveBeenCalledWith(
      3,
      expect.objectContaining({
        unlockConditionType: 'achievement',
        unlockConditionConfig: { achievementCode: 'ac_100' },
      }),
    )
    wrapper.unmount()
  })

  it('validates required fields', async () => {
    const wrapper = await mountPanel()
    await actionButton(wrapper, '新建称号')!.trigger('click')
    await flushPromises()
    dialogButton('保存')!.click()
    await flushPromises()

    expect(createTitle).not.toHaveBeenCalled()
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'warn', summary: '请填写称号名称' }),
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
    expect(options.header).toBe('删除称号')
    expect(options.message).toContain('荣耀王者')
    expect(options.acceptLabel).toBe('删除')
    expect(options.rejectLabel).toBe('取消')

    options.accept?.()
    await flushPromises()
    expect(deleteTitle).toHaveBeenCalledWith(1)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '称号已删除' }),
    )
    wrapper.unmount()
  })
})
