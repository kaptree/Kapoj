import { flushPromises, mount } from '@vue/test-utils'
import type { DOMWrapper } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Avatar from 'primevue/avatar'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import {
  equipBadge,
  getGamificationConfig,
  listBadges,
  purchaseBadge,
  unequipBadge,
} from '@/api/gamification'
import { ApiError } from '@/api/http'
import type { BadgeItem, Me } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import BadgeShopView from './BadgeShopView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/gamification', () => ({
  getGamificationConfig: vi.fn(),
  listBadges: vi.fn(),
  purchaseBadge: vi.fn(),
  equipBadge: vi.fn(),
  unequipBadge: vi.fn(),
}))

function badge(overrides: Partial<BadgeItem> = {}): BadgeItem {
  return {
    id: 1,
    name: '竞赛徽章',
    description: '参加比赛获得',
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

function badges(): BadgeItem[] {
  return [
    badge(),
    badge({
      id: 2,
      name: '老玩家',
      description: '早期玩家专属',
      category: '竞赛',
      priceCoins: 50,
      isLimited: false,
      stock: 0,
      sortOrder: 2,
      owned: true,
      purchasedAt: '2026-01-01T00:00:00Z',
    }),
    badge({
      id: 3,
      name: '荣耀勋章',
      description: '荣誉展示',
      category: '成就',
      priceCoins: 0,
      isLimited: false,
      stock: 0,
      sortOrder: 3,
      owned: true,
      equipped: true,
      purchasedAt: '2026-01-01T00:00:00Z',
    }),
    badge({
      id: 4,
      name: '绝版徽章',
      description: '限量已售罄',
      category: '竞赛',
      priceCoins: 80,
      stock: 0,
      sortOrder: 4,
    }),
    badge({
      id: 5,
      name: '天价徽章',
      description: '价格昂贵',
      category: '外观',
      priceCoins: 1000,
      isLimited: false,
      stock: 0,
      sortOrder: 5,
    }),
  ]
}

function me(coins = 500): Me {
  return {
    id: 1,
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
  { path: '/shop/badges', name: 'badge-shop', component: { template: '<div />' } },
  { path: '/login', name: 'login', component: { template: '<div />' } },
]

async function mountPage(loggedIn = true, coins = 500) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/shop/badges')
  await router.isReady()
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  if (loggedIn) {
    auth.setTokens({ accessToken: 'access', refreshToken: 'refresh' })
    auth.setUser(me(coins))
  }
  const wrapper = mount(BadgeShopView, {
    global: { plugins: [router, PrimeVue, pinia] },
  })
  await flushPromises()
  return { wrapper, router, auth }
}

function button(
  container: Awaited<ReturnType<typeof mountPage>>['wrapper'] | DOMWrapper<Element>,
  label: string,
) {
  return container.findAll('button').find((item) => item.text().trim() === label)
}

function card(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper'], name: string) {
  return wrapper.findAll('.badge-card').find((item) => item.text().includes(name))!
}

beforeEach(() => {
  localStorage.clear()
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(getGamificationConfig).mockReset()
  vi.mocked(listBadges).mockReset()
  vi.mocked(purchaseBadge).mockReset()
  vi.mocked(equipBadge).mockReset()
  vi.mocked(unequipBadge).mockReset()

  vi.mocked(getGamificationConfig).mockResolvedValue({ maxEquippedBadges: 1 })
  vi.mocked(listBadges).mockResolvedValue(badges())
  vi.mocked(purchaseBadge).mockResolvedValue(null)
  vi.mocked(equipBadge).mockResolvedValue(null)
  vi.mocked(unequipBadge).mockResolvedValue(null)
})

describe('BadgeShopView', () => {
  it('renders categories, states, inventory, balance and button states', async () => {
    const { wrapper } = await mountPage()

    expect(listBadges).toHaveBeenCalledTimes(1)
    expect(getGamificationConfig).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('徽章商城')
    expect(wrapper.text()).toContain('已装备 1 / 1 个')
    expect(wrapper.text()).toContain('余额 500 金币')
    expect(wrapper.text()).toContain('竞赛')
    expect(wrapper.text()).toContain('成就')
    expect(wrapper.text()).toContain('外观')
    expect(wrapper.findAll('.badge-card')).toHaveLength(5)
    expect(wrapper.text()).toContain('装备中')
    expect(wrapper.text()).toContain('已拥有')
    expect(wrapper.text()).toContain('100 金币')
    expect(wrapper.text()).toContain('限量 · 剩余 5')
    expect(wrapper.text()).toContain('限量 · 已售罄')
    expect(wrapper.text()).toContain('装备已满，请先卸下一个徽章。')
    expect(wrapper.text()).toContain('余额不足，还差 500 金币。')

    expect(button(card(wrapper, '老玩家'), '装备')!.attributes()).toHaveProperty('disabled')
    expect(button(card(wrapper, '荣耀勋章'), '卸下')!.attributes()).not.toHaveProperty('disabled')
    expect(button(card(wrapper, '绝版徽章'), '已售罄')!.attributes()).toHaveProperty('disabled')
    expect(button(card(wrapper, '天价徽章'), '余额不足')!.attributes()).toHaveProperty('disabled')
    expect(button(card(wrapper, '竞赛徽章'), '购买')).toBeTruthy()
    wrapper.unmount()
  })

  it('filters badges by category', async () => {
    const { wrapper } = await mountPage()

    await button(wrapper, '成就')!.trigger('click')
    await nextTick()
    expect(wrapper.findAll('.badge-card')).toHaveLength(1)
    expect(wrapper.text()).toContain('荣耀勋章')
    expect(wrapper.text()).not.toContain('竞赛徽章')

    await button(wrapper, '竞赛')!.trigger('click')
    await nextTick()
    expect(wrapper.findAll('.badge-card')).toHaveLength(3)

    await button(wrapper, '全部')!.trigger('click')
    await nextTick()
    expect(wrapper.findAll('.badge-card')).toHaveLength(5)
    wrapper.unmount()
  })

  it('confirms a purchase and refreshes the user and shop', async () => {
    const { wrapper, auth } = await mountPage()
    const fetchMe = vi.spyOn(auth, 'fetchMe').mockResolvedValue()

    await button(card(wrapper, '竞赛徽章'), '购买')!.trigger('click')
    expect(confirm.require).toHaveBeenCalled()
    const options = confirm.require.mock.calls[0]![0] as {
      header?: string
      message?: string
      acceptLabel?: string
      rejectLabel?: string
      accept?: () => void
    }
    expect(options.header).toBe('购买徽章')
    expect(options.message).toBe('确定花费 100 金币购买徽章「竞赛徽章」吗？')
    expect(options.acceptLabel).toBe('购买')
    expect(options.rejectLabel).toBe('取消')

    options.accept?.()
    await flushPromises()
    expect(purchaseBadge).toHaveBeenCalledWith(1)
    expect(fetchMe).toHaveBeenCalled()
    expect(listBadges).toHaveBeenCalledTimes(2)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'success',
        summary: '购买成功，可在装扮管理中装备',
      }),
    )
    wrapper.unmount()
  })

  it('reports purchase failures and resets the button state', async () => {
    vi.mocked(purchaseBadge).mockRejectedValue(new ApiError(409, 40901, '库存不足'))
    const { wrapper, auth } = await mountPage()
    const fetchMe = vi.spyOn(auth, 'fetchMe').mockResolvedValue()

    await button(card(wrapper, '竞赛徽章'), '购买')!.trigger('click')
    const options = confirm.require.mock.calls[0]![0] as { accept?: () => void }
    options.accept?.()
    await flushPromises()

    expect(fetchMe).not.toHaveBeenCalled()
    expect(listBadges).toHaveBeenCalledTimes(1)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'error',
        summary: '操作失败',
        detail: '库存不足',
      }),
    )
    expect(button(card(wrapper, '竞赛徽章'), '购买')!.attributes()).not.toHaveProperty('disabled')
    wrapper.unmount()
  })

  it('equips and unequips owned badges', async () => {
    vi.mocked(getGamificationConfig).mockResolvedValue({ maxEquippedBadges: 3 })
    const { wrapper, auth } = await mountPage()
    const fetchMe = vi.spyOn(auth, 'fetchMe').mockResolvedValue()

    await button(card(wrapper, '老玩家'), '装备')!.trigger('click')
    await flushPromises()
    expect(equipBadge).toHaveBeenCalledWith(2)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '已装备徽章' }),
    )

    await button(card(wrapper, '荣耀勋章'), '卸下')!.trigger('click')
    await flushPromises()
    expect(unequipBadge).toHaveBeenCalledWith(3)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '已卸下徽章' }),
    )
    expect(fetchMe).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('shows guest login calls to action', async () => {
    vi.mocked(listBadges).mockResolvedValue(
      badges().map((item) => ({ ...item, owned: false, equipped: false })),
    )
    const { wrapper } = await mountPage(false)

    expect(wrapper.text()).toContain('登录后可购买徽章并装备展示。')
    expect(wrapper.text()).not.toContain('余额')
    expect(wrapper.text()).toContain('登录后购买')
    expect(wrapper.text()).toContain('登录后才能购买与装备徽章。')
    expect(button(wrapper, '购买')).toBeUndefined()
    const link = wrapper.findAll('a').find((item) => item.text().includes('登录 / 注册'))
    expect(link!.attributes('href')).toBe('/login?redirect=/shop/badges')
    wrapper.unmount()
  })

  it('reloads when the login state changes', async () => {
    const { wrapper, auth } = await mountPage()
    expect(listBadges).toHaveBeenCalledTimes(1)

    auth.clear()
    await flushPromises()
    expect(listBadges).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('shows loading, empty and error states', async () => {
    let resolve!: (value: BadgeItem[]) => void
    vi.mocked(listBadges).mockReturnValue(
      new Promise<BadgeItem[]>((r) => {
        resolve = r
      }),
    )
    const loading = await mountPage()
    expect(loading.wrapper.text()).toContain('正在加载徽章商城')
    resolve([])
    await flushPromises()
    expect(loading.wrapper.text()).toContain('暂无徽章')
    loading.wrapper.unmount()

    vi.mocked(listBadges)
      .mockRejectedValueOnce(new ApiError(500, 50000, '加载徽章商城失败'))
      .mockResolvedValueOnce([badge()])
    const failed = await mountPage()
    expect(failed.wrapper.text()).toContain('加载徽章商城失败')
    const retry = failed.wrapper.findAll('button').find((item) => item.text().trim() === '重试')
    expect(retry).toBeTruthy()
    await retry!.trigger('click')
    await flushPromises()
    expect(failed.wrapper.text()).toContain('竞赛徽章')
    failed.wrapper.unmount()
  })

  it('falls back to the badge initial when the image fails', async () => {
    vi.mocked(listBadges).mockResolvedValue([badge({ imageUrl: '/badge.png' })])
    const { wrapper } = await mountPage()

    const avatar = wrapper.findComponent(Avatar)
    expect(avatar.props('image')).toBe('/badge.png')
    expect(avatar.props('label')).toBeFalsy()
    avatar.vm.$emit('error', new Event('error'))
    await nextTick()
    expect(avatar.props('image')).toBeFalsy()
    expect(avatar.props('label')).toBe('竞')
    wrapper.unmount()
  })
})
