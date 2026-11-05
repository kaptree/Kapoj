import { flushPromises, mount } from '@vue/test-utils'
import type { DOMWrapper } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Avatar from 'primevue/avatar'
import Paginator from 'primevue/paginator'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import {
  equipBadge,
  equipTitle,
  getGamificationConfig,
  listCoinTransactions,
  listTitles,
  listUserBadges,
  unequipBadge,
  unequipTitle,
} from '@/api/gamification'
import { ApiError } from '@/api/http'
import type {
  BadgeItem,
  CoinTransactionItem,
  CoinTransactionPage,
  Me,
  TitleItem,
} from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import AppearanceView from './AppearanceView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('@/api/gamification', () => ({
  listUserBadges: vi.fn(),
  listTitles: vi.fn(),
  getGamificationConfig: vi.fn(),
  listCoinTransactions: vi.fn(),
  equipBadge: vi.fn(),
  unequipBadge: vi.fn(),
  equipTitle: vi.fn(),
  unequipTitle: vi.fn(),
}))

function badge(overrides: Partial<BadgeItem> = {}): BadgeItem {
  return {
    id: 1,
    name: '竞赛徽章',
    description: '',
    imageUrl: '',
    category: '竞赛',
    priceCoins: 100,
    isLimited: false,
    stock: 0,
    sortOrder: 1,
    isActive: true,
    owned: true,
    equipped: true,
    purchasedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function ownedBadges(): BadgeItem[] {
  return [
    badge(),
    badge({ id: 2, name: '老玩家', sortOrder: 2, equipped: false }),
    badge({ id: 3, name: '绝版徽章', sortOrder: 3, equipped: false, isActive: false }),
  ]
}

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
    unlocked: true,
    unlockedAt: '2026-01-01T00:00:00Z',
    purchased: true,
    purchasedAt: '2026-01-01T00:00:00Z',
    equipped: true,
    progress: { type: 'rating', current: 1700, target: 1600, done: true, achievementCode: '' },
    ...overrides,
  }
}

function ownedTitles(): TitleItem[] {
  return [
    title(),
    title({ id: 2, name: '老兵', sortOrder: 2, equipped: false }),
    title({
      id: 3,
      name: '未购买称号',
      sortOrder: 3,
      purchased: false,
      purchasedAt: null,
      equipped: false,
    }),
  ]
}

function transaction(overrides: Partial<CoinTransactionItem> = {}): CoinTransactionItem {
  return {
    id: 1,
    amount: 100,
    type: 'earn',
    reason: 'achievement',
    refType: 'achievement',
    refId: 1,
    note: '',
    createdAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function txPage(overrides: Partial<CoinTransactionPage> = {}): CoinTransactionPage {
  return {
    balance: 500,
    total: 45,
    items: [
      transaction(),
      transaction({
        id: 2,
        amount: -50,
        type: 'spend',
        reason: 'task',
        note: '任务消费',
      }),
    ],
    ...overrides,
  }
}

function me(coins = 500, id = 1): Me {
  return {
    id,
    username: 'alice',
    nickname: 'Alice',
    email: 'alice@example.com',
    avatarUrl: '',
    bio: '',
    role: 'user',
    rating: 1600,
    coins,
    status: 'active',
    createdAt: '2025-01-01T00:00:00Z',
  }
}

const routes = [
  { path: '/appearance', name: 'appearance', component: { template: '<div />' } },
  { path: '/shop/badges', name: 'badge-shop', component: { template: '<div />' } },
  { path: '/shop/titles', name: 'title-shop', component: { template: '<div />' } },
]

async function mountPage() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/appearance')
  await router.isReady()
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.setTokens({ accessToken: 'access', refreshToken: 'refresh' })
  auth.setUser(me())
  const fetchMe = vi.spyOn(auth, 'fetchMe').mockResolvedValue()
  const wrapper = mount(AppearanceView, {
    global: { plugins: [router, PrimeVue, pinia] },
  })
  await flushPromises()
  return { wrapper, router, auth, fetchMe }
}

function button(
  container: Awaited<ReturnType<typeof mountPage>>['wrapper'] | DOMWrapper<Element>,
  label: string,
) {
  return container.findAll('button').find((item) => item.text().trim() === label)
}

function badgeCard(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper'], name: string) {
  return wrapper.findAll('.owned-card').find((item) => item.text().includes(name))!
}

beforeEach(() => {
  localStorage.clear()
  toast.add.mockReset()
  vi.mocked(listUserBadges).mockReset()
  vi.mocked(listTitles).mockReset()
  vi.mocked(getGamificationConfig).mockReset()
  vi.mocked(listCoinTransactions).mockReset()
  vi.mocked(equipBadge).mockReset()
  vi.mocked(unequipBadge).mockReset()
  vi.mocked(equipTitle).mockReset()
  vi.mocked(unequipTitle).mockReset()

  vi.mocked(listUserBadges).mockResolvedValue(ownedBadges())
  vi.mocked(listTitles).mockResolvedValue(ownedTitles())
  vi.mocked(getGamificationConfig).mockResolvedValue({ maxEquippedBadges: 2 })
  vi.mocked(listCoinTransactions).mockResolvedValue(txPage())
  vi.mocked(equipBadge).mockResolvedValue(null)
  vi.mocked(unequipBadge).mockResolvedValue(null)
  vi.mocked(equipTitle).mockResolvedValue(null)
  vi.mocked(unequipTitle).mockResolvedValue(null)
})

describe('AppearanceView', () => {
  it('renders overview, owned badges, titles and coin transactions', async () => {
    const { wrapper, fetchMe } = await mountPage()

    expect(fetchMe).toHaveBeenCalled()
    expect(listUserBadges).toHaveBeenCalledWith(1)
    expect(listTitles).toHaveBeenCalledTimes(1)
    expect(getGamificationConfig).toHaveBeenCalledTimes(1)
    expect(listCoinTransactions).toHaveBeenCalledWith({ page: 1, pageSize: 20 })

    expect(wrapper.text()).toContain('装扮管理')
    expect(wrapper.text()).toContain('金币余额')
    expect(wrapper.text()).toContain('500')
    expect(wrapper.text()).toContain('荣耀王者')
    expect(wrapper.findAll('.owned-card')).toHaveLength(3)
    expect(wrapper.text()).toContain('装备中')
    expect(wrapper.text()).toContain('已下线')
    expect(wrapper.text()).toContain('我的称号')
    expect(wrapper.text()).toContain('已购买 2 个')
    expect(wrapper.text()).not.toContain('未购买称号')
    expect(wrapper.text()).toContain('金币流水')
    expect(wrapper.text()).toContain('共 45 条')
    expect(wrapper.text()).toContain('成就奖励')
    expect(wrapper.text()).toContain('任务奖励')
    expect(wrapper.text()).toContain('任务消费')
    expect(wrapper.text()).toContain('+100')
    expect(wrapper.text()).toContain('-50')
    expect(wrapper.findAll('.amount.income')).toHaveLength(1)
    expect(wrapper.findAll('.amount.expense')).toHaveLength(1)
    expect(wrapper.findComponent(Paginator).props('totalRecords')).toBe(45)
    wrapper.unmount()
  })

  it('equips and unequips badges and respects the limit', async () => {
    const { wrapper, fetchMe } = await mountPage()

    await button(badgeCard(wrapper, '老玩家'), '装备')!.trigger('click')
    await flushPromises()
    expect(equipBadge).toHaveBeenCalledWith(2)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '已装备徽章' }),
    )
    expect(fetchMe).toHaveBeenCalledTimes(2)
    expect(listUserBadges).toHaveBeenCalledTimes(2)

    await button(badgeCard(wrapper, '竞赛徽章'), '卸下')!.trigger('click')
    await flushPromises()
    expect(unequipBadge).toHaveBeenCalledWith(1)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '已卸下徽章' }),
    )
    wrapper.unmount()
  })

  it('disables badge equipping when the limit is reached', async () => {
    vi.mocked(getGamificationConfig).mockResolvedValue({ maxEquippedBadges: 1 })
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('已装备 1 / 1 个')
    expect(wrapper.text()).toContain('装备已满，请先卸下一个徽章。')
    expect(button(badgeCard(wrapper, '老玩家'), '装备')!.attributes()).toHaveProperty('disabled')
    wrapper.unmount()
  })

  it('equips and unequips titles', async () => {
    const { wrapper, fetchMe } = await mountPage()

    await wrapper.findAll('.owned-title button')[1]!.trigger('click')
    await flushPromises()
    expect(equipTitle).toHaveBeenCalledWith(2)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'success',
        summary: '已装备称号，原称号已自动卸下',
      }),
    )

    await wrapper.findAll('.owned-title button')[0]!.trigger('click')
    await flushPromises()
    expect(unequipTitle).toHaveBeenCalledWith(1)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '已卸下称号' }),
    )
    expect(fetchMe).toHaveBeenCalledTimes(3)
    wrapper.unmount()
  })

  it('loads the next transaction page', async () => {
    const { wrapper } = await mountPage()
    vi.mocked(listCoinTransactions).mockResolvedValue(
      txPage({
        items: [transaction({ id: 3, amount: 30, reason: 'task' })],
      }),
    )

    wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()

    expect(listCoinTransactions).toHaveBeenLastCalledWith({ page: 2, pageSize: 20 })
    expect(wrapper.text()).toContain('+30')
    wrapper.unmount()
  })

  it('shows transaction errors and retries', async () => {
    vi.mocked(listCoinTransactions)
      .mockRejectedValueOnce(new ApiError(500, 50000, '加载金币流水失败'))
      .mockResolvedValueOnce(txPage())
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('加载金币流水失败')
    const retry = wrapper.findAll('button').find((item) => item.text().trim() === '重试')
    expect(retry).toBeTruthy()
    await retry!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('+100')
    wrapper.unmount()
  })

  it('shows page errors and retries the page load', async () => {
    vi.mocked(listUserBadges)
      .mockRejectedValueOnce(new ApiError(500, 50000, '加载装扮信息失败'))
      .mockResolvedValueOnce(ownedBadges())
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('加载装扮信息失败')
    const retry = wrapper.findAll('button').find((item) => item.text().trim() === '重试')
    expect(retry).toBeTruthy()
    await retry!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('竞赛徽章')
    wrapper.unmount()
  })

  it('shows loading and empty states', async () => {
    let resolve!: (value: BadgeItem[]) => void
    vi.mocked(listUserBadges).mockReturnValue(
      new Promise<BadgeItem[]>((r) => {
        resolve = r
      }),
    )
    vi.mocked(listTitles).mockResolvedValue([])
    vi.mocked(listCoinTransactions).mockResolvedValue(txPage({ total: 0, items: [] }))
    const loading = await mountPage()
    expect(loading.wrapper.text()).toContain('正在加载装扮信息')
    resolve([])
    await flushPromises()
    expect(loading.wrapper.text()).toContain('暂无徽章，去商城看看吧')
    expect(loading.wrapper.text()).toContain('暂无称号，去商店看看吧')
    expect(loading.wrapper.text()).toContain('暂无金币流水')
    loading.wrapper.unmount()
  })

  it('falls back to the badge initial when the image fails', async () => {
    vi.mocked(listUserBadges).mockResolvedValue([badge({ imageUrl: '/badge.png' })])
    const { wrapper } = await mountPage()

    const avatar = wrapper
      .findAllComponents(Avatar)
      .find((item) => item.classes().includes('owned-icon'))!
    expect(avatar.props('image')).toBe('/badge.png')
    expect(avatar.props('label')).toBeFalsy()
    avatar.vm.$emit('error', new Event('error'))
    await nextTick()
    expect(avatar.props('image')).toBeFalsy()
    expect(avatar.props('label')).toBe('竞')
    wrapper.unmount()
  })
})
