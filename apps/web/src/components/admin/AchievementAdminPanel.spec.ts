import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import InputNumber from 'primevue/inputnumber'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  createAchievement,
  deleteAchievement,
  listAchievements,
  listBadges,
  updateAchievement,
} from '@/api/gamification'
import type { AchievementItem, BadgeItem } from '@/api/types'
import AchievementAdminPanel from './AchievementAdminPanel.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/gamification', () => ({
  listAchievements: vi.fn(),
  listBadges: vi.fn(),
  createAchievement: vi.fn(),
  updateAchievement: vi.fn(),
  deleteAchievement: vi.fn(),
}))

function achievement(overrides: Partial<AchievementItem> = {}): AchievementItem {
  return {
    id: 1,
    code: 'ac_100',
    name: '百题斩',
    description: '累计通过 100 道题',
    iconUrl: '',
    conditionType: 'ac_count',
    conditionConfig: { threshold: 100 },
    rewardCoins: 50,
    rewardBadgeId: 1,
    sortOrder: 1,
    isActive: true,
    unlocked: false,
    unlockedAt: null,
    progress: [],
    ...overrides,
  }
}

function badge(overrides: Partial<BadgeItem> = {}): BadgeItem {
  return {
    id: 1,
    name: '先锋徽章',
    description: '',
    imageUrl: '',
    category: '竞赛',
    priceCoins: 0,
    isLimited: false,
    stock: 0,
    sortOrder: 1,
    isActive: true,
    owned: false,
    equipped: false,
    purchasedAt: null,
    ...overrides,
  }
}

async function mountPanel() {
  const wrapper = mount(AchievementAdminPanel, {
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
  vi.mocked(listAchievements).mockReset()
  vi.mocked(listBadges).mockReset()
  vi.mocked(createAchievement).mockReset()
  vi.mocked(updateAchievement).mockReset()
  vi.mocked(deleteAchievement).mockReset()

  vi.mocked(listAchievements).mockResolvedValue([
    achievement(),
    achievement({
      id: 2,
      code: 'all_rounder',
      name: '全能选手',
      conditionType: 'all_of',
      conditionConfig: {
        conditions: [
          { type: 'ac_count', threshold: 50 },
          { type: 'contest_count', threshold: 5 },
        ],
      },
      rewardCoins: 0,
      rewardBadgeId: null,
      isActive: false,
    }),
  ])
  vi.mocked(listBadges).mockResolvedValue([badge(), badge({ id: 2, name: '元老徽章' })])
  vi.mocked(createAchievement).mockResolvedValue(achievement({ id: 3 }))
  vi.mocked(updateAchievement).mockResolvedValue(achievement())
  vi.mocked(deleteAchievement).mockResolvedValue(null)
})

describe('AchievementAdminPanel', () => {
  it('renders achievements, conditions, rewards and status', async () => {
    const wrapper = await mountPanel()

    expect(listAchievements).toHaveBeenCalledWith({ includeInactive: true })
    expect(listBadges).toHaveBeenCalledWith({ includeInactive: true })
    expect(wrapper.text()).toContain('共 2 个成就')
    expect(wrapper.text()).toContain('百题斩')
    expect(wrapper.text()).toContain('ac_100')
    expect(wrapper.text()).toContain('AC 题目数 ≥ 100')
    expect(wrapper.text()).toContain('全能选手')
    expect(wrapper.text()).toContain('AC 题目数 ≥ 50 且 参赛数 ≥ 5')
    expect(wrapper.text()).toContain('金币 50')
    expect(wrapper.text()).toContain('徽章 先锋徽章')
    expect(wrapper.text()).toContain('已上架')
    expect(wrapper.text()).toContain('已下线')
    expect(wrapper.findAll('.p-datatable-tbody > tr')).toHaveLength(2)
    wrapper.unmount()
  })

  it('creates an achievement', async () => {
    const wrapper = await mountPanel()

    await actionButton(wrapper, '新建成就')!.trigger('click')
    await flushPromises()
    setField(dialog().querySelector('.code-input')!, 'ac_200')
    setField(dialog().querySelector('.name-input')!, '新成就')
    setField(dialog().querySelector('.description-input')!, '新描述')
    const numbers = wrapper.findAllComponents(InputNumber)
    numbers[1]!.vm.$emit('update:modelValue', 50)
    dialogButton('保存')!.click()
    await flushPromises()

    expect(createAchievement).toHaveBeenCalledWith({
      code: 'ac_200',
      name: '新成就',
      description: '新描述',
      iconUrl: '',
      conditionType: 'ac_count',
      conditionConfig: { threshold: 1 },
      rewardCoins: 50,
      rewardBadgeId: null,
      sortOrder: 0,
      isActive: true,
    })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '成就已创建' }),
    )
    expect(listAchievements).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('edits an all_of achievement and preserves conditions', async () => {
    const wrapper = await mountPanel()

    await actionButton(wrapper, '编辑', 1)!.trigger('click')
    await flushPromises()
    expect(dialog().querySelector('.code-input')?.hasAttribute('disabled')).toBe(true)
    expect(dialog().querySelectorAll('.condition-row')).toHaveLength(2)

    dialogButton('添加条件（最多 8 项）')!.click()
    await flushPromises()
    expect(dialog().querySelectorAll('.condition-row')).toHaveLength(3)

    setField(dialog().querySelector('.name-input')!, '全能选手改')
    dialogButton('保存')!.click()
    await flushPromises()

    const input = vi.mocked(updateAchievement).mock.calls[0]![1]
    expect(updateAchievement).toHaveBeenCalledWith(
      2,
      expect.objectContaining({ name: '全能选手改' }),
    )
    expect(input.conditionConfig.conditions).toHaveLength(3)
    wrapper.unmount()
  })

  it('validates required fields', async () => {
    const wrapper = await mountPanel()

    await actionButton(wrapper, '新建成就')!.trigger('click')
    await flushPromises()
    dialogButton('保存')!.click()
    await flushPromises()

    expect(createAchievement).not.toHaveBeenCalled()
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'warn', summary: '请填写成就名称' }),
    )
    wrapper.unmount()
  })

  it('confirms deletion with Chinese labels', async () => {
    const wrapper = await mountPanel()

    await actionButton(wrapper, '删除')!.trigger('click')
    expect(confirm.require).toHaveBeenCalled()
    const options = confirm.require.mock.calls[0]![0] as {
      header?: string
      message?: string
      acceptLabel?: string
      rejectLabel?: string
      accept?: () => void
    }
    expect(options.header).toBe('删除成就')
    expect(options.message).toContain('百题斩')
    expect(options.acceptLabel).toBe('删除')
    expect(options.rejectLabel).toBe('取消')

    options.accept?.()
    await flushPromises()
    expect(deleteAchievement).toHaveBeenCalledWith(1)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '成就已删除' }),
    )
    expect(listAchievements).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
})
