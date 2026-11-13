import { flushPromises, mount } from '@vue/test-utils'
import type { DOMWrapper } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import ProgressBar from 'primevue/progressbar'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { equipTitle, listTitles, purchaseTitle, unequipTitle } from '@/api/gamification'
import { ApiError } from '@/api/http'
import type { Me, TitleItem } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import TitleShopView from './TitleShopView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/gamification', () => ({
  listTitles: vi.fn(),
  purchaseTitle: vi.fn(),
  equipTitle: vi.fn(),
  unequipTitle: vi.fn(),
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
    unlocked: true,
    unlockedAt: '2026-01-01T00:00:00Z',
    purchased: false,
    purchasedAt: null,
    equipped: false,
    progress: { type: 'rating', current: 1700, target: 1600, done: true, achievementCode: '' },
    ...overrides,
  }
}

function titles(): TitleItem[] {
  return [
    title(),
    title({
      id: 2,
      name: '老兵',
      color: '#0F766E',
      priceCoins: 50,
      sortOrder: 2,
      purchased: true,
      purchasedAt: '2026-01-01T00:00:00Z',
    }),
    title({
      id: 3,
      name: '传奇',
      color: '#B45309',
      priceCoins: 0,
      sortOrder: 3,
      purchased: true,
      purchasedAt: '2026-01-01T00:00:00Z',
      equipped: true,
    }),
    title({
      id: 4,
      name: '题海行者',
      color: '#2563EB',
      unlockConditionType: 'ac_count',
      unlockConditionConfig: { threshold: 100 },
      priceCoins: 80,
      sortOrder: 4,
      unlocked: false,
      unlockedAt: null,
      progress: { type: 'ac_count', current: 20, target: 100, done: false, achievementCode: '' },
    }),
    title({
      id: 5,
      name: '至尊',
      color: '#DC2626',
      priceCoins: 1000,
      sortOrder: 5,
    }),
    title({
      id: 6,
      name: '成就收藏家',
      color: '#9333EA',
      unlockConditionType: 'achievement',
      unlockConditionConfig: { achievementCode: 'ac_100' },
      priceCoins: 10,
      sortOrder: 6,
      isActive: false,
      progress: {
        type: 'achievement',
        current: 1,
        target: 1,
        done: true,
        achievementCode: 'ac_100',
      },
    }),
  ]
}

function me(coins = 500): Me {
  return {
    id: 1,
    username: 'alice',
    nickname: 'Alice',
    email: 'alice@example.com',
    emailVerified: true,
    avatarUrl: '',
    avatarUploaded: false,
    bio: '',
    role: 'user',
    roles: [],
    permissions: [],
    isSuperAdmin: false,
    rating: 1600,
    coins,
    status: 'active',
    createdAt: '2025-01-01T00:00:00Z',
  }
}

const routes = [
  { path: '/shop/titles', name: 'title-shop', component: { template: '<div />' } },
  { path: '/login', name: 'login', component: { template: '<div />' } },
]

async function mountPage(loggedIn = true, coins = 500) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/shop/titles')
  await router.isReady()
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  if (loggedIn) {
    auth.setTokens({ accessToken: 'access', refreshToken: 'refresh', sessionId: 's1' })
    auth.setUser(me(coins))
  }
  const wrapper = mount(TitleShopView, {
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
  return wrapper.findAll('.title-card').find((item) => item.text().includes(name))!
}

beforeEach(() => {
  localStorage.clear()
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(listTitles).mockReset()
  vi.mocked(purchaseTitle).mockReset()
  vi.mocked(equipTitle).mockReset()
  vi.mocked(unequipTitle).mockReset()

  vi.mocked(listTitles).mockResolvedValue(titles())
  vi.mocked(purchaseTitle).mockResolvedValue(null)
  vi.mocked(equipTitle).mockResolvedValue(null)
  vi.mocked(unequipTitle).mockResolvedValue(null)
})

describe('TitleShopView', () => {
  it('renders title states, prices, conditions and progress', async () => {
    const { wrapper } = await mountPage()

    expect(listTitles).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('称号商店')
    expect(wrapper.text()).toContain('余额 500 金币')
    expect(wrapper.findAll('.title-card')).toHaveLength(6)
    expect(wrapper.text()).toContain('荣耀王者')
    expect(wrapper.text()).toContain('已解锁')
    expect(wrapper.text()).toContain('已购买')
    expect(wrapper.text()).toContain('装备中')
    expect(wrapper.text()).toContain('未解锁')
    expect(wrapper.text()).toContain('已下线')
    expect(wrapper.text()).toContain('100 金币')
    expect(wrapper.text()).toContain('1,000 金币')
    expect(wrapper.text()).toContain('Rating')
    expect(wrapper.text()).toContain('1700 / 1600')
    expect(wrapper.text()).toContain('AC 题目数')
    expect(wrapper.text()).toContain('20 / 100')
    expect(wrapper.text()).toContain('成就：ac_100')
    expect(wrapper.text()).toContain('该称号正在公开展示。')
    expect(wrapper.text()).toContain('装备后会自动替换当前称号。')
    expect(wrapper.text()).toContain('余额不足，还差 500 金币。')
    expect(wrapper.text()).toContain('该称号已下线，已解锁或已购买的用户仍可继续兑换与使用。')

    expect(button(card(wrapper, '荣耀王者'), '购买')).toBeTruthy()
    expect(button(card(wrapper, '老兵'), '装备')).toBeTruthy()
    expect(button(card(wrapper, '传奇'), '卸下')).toBeTruthy()
    expect(button(card(wrapper, '题海行者'), '未解锁')!.attributes()).toHaveProperty('disabled')
    expect(button(card(wrapper, '至尊'), '余额不足')!.attributes()).toHaveProperty('disabled')

    const progress = wrapper.findAllComponents(ProgressBar)
    expect(progress[0]!.props('value')).toBe(100)
    expect(progress[3]!.props('value')).toBe(20)
    wrapper.unmount()
  })

  it('confirms a purchase and refreshes the user and shop', async () => {
    const { wrapper, auth } = await mountPage()
    const fetchMe = vi.spyOn(auth, 'fetchMe').mockResolvedValue()

    await button(card(wrapper, '荣耀王者'), '购买')!.trigger('click')
    expect(confirm.require).toHaveBeenCalled()
    const options = confirm.require.mock.calls[0]![0] as {
      header?: string
      message?: string
      acceptLabel?: string
      rejectLabel?: string
      accept?: () => void
    }
    expect(options.header).toBe('购买称号')
    expect(options.message).toBe('确定花费 100 金币购买称号「荣耀王者」吗？')
    expect(options.acceptLabel).toBe('购买')
    expect(options.rejectLabel).toBe('取消')

    options.accept?.()
    await flushPromises()
    expect(purchaseTitle).toHaveBeenCalledWith(1)
    expect(fetchMe).toHaveBeenCalled()
    expect(listTitles).toHaveBeenCalledTimes(2)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'success',
        summary: '购买成功，装备后即可展示',
      }),
    )
    wrapper.unmount()
  })

  it('reports purchase failures and resets the button state', async () => {
    vi.mocked(purchaseTitle).mockRejectedValue(new ApiError(409, 40901, '余额不足'))
    const { wrapper, auth } = await mountPage()
    const fetchMe = vi.spyOn(auth, 'fetchMe').mockResolvedValue()

    await button(card(wrapper, '荣耀王者'), '购买')!.trigger('click')
    const options = confirm.require.mock.calls[0]![0] as { accept?: () => void }
    options.accept?.()
    await flushPromises()

    expect(fetchMe).not.toHaveBeenCalled()
    expect(listTitles).toHaveBeenCalledTimes(1)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'error',
        summary: '操作失败',
        detail: '余额不足',
      }),
    )
    expect(button(card(wrapper, '荣耀王者'), '购买')).toBeTruthy()
    wrapper.unmount()
  })

  it('equips and unequips titles', async () => {
    const { wrapper, auth } = await mountPage()
    const fetchMe = vi.spyOn(auth, 'fetchMe').mockResolvedValue()

    await button(card(wrapper, '老兵'), '装备')!.trigger('click')
    await flushPromises()
    expect(equipTitle).toHaveBeenCalledWith(2)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'success',
        summary: '已装备称号，原称号已自动卸下',
      }),
    )

    await button(card(wrapper, '传奇'), '卸下')!.trigger('click')
    await flushPromises()
    expect(unequipTitle).toHaveBeenCalledWith(3)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '已卸下称号' }),
    )
    expect(fetchMe).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('shows guest login calls to action', async () => {
    const { wrapper } = await mountPage(false)

    expect(wrapper.text()).toContain('登录后可查看解锁进度、购买并装备称号。')
    expect(wrapper.text()).not.toContain('余额')
    expect(wrapper.text()).toContain('登录后查看')
    expect(wrapper.text()).toContain('登录后可查看自己的解锁进度。')
    expect(button(wrapper, '购买')).toBeUndefined()
    const link = wrapper.findAll('a').find((item) => item.text().includes('登录 / 注册'))
    expect(link!.attributes('href')).toBe('/login?redirect=/shop/titles')
    wrapper.unmount()
  })

  it('reloads when the login state changes', async () => {
    const { wrapper, auth } = await mountPage()
    expect(listTitles).toHaveBeenCalledTimes(1)

    auth.clear()
    await flushPromises()
    expect(listTitles).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('shows loading, empty and error states', async () => {
    let resolve!: (value: TitleItem[]) => void
    vi.mocked(listTitles).mockReturnValue(
      new Promise<TitleItem[]>((r) => {
        resolve = r
      }),
    )
    const loading = await mountPage()
    expect(loading.wrapper.text()).toContain('正在加载称号商店')
    resolve([])
    await flushPromises()
    expect(loading.wrapper.text()).toContain('暂无称号')
    loading.wrapper.unmount()

    vi.mocked(listTitles)
      .mockRejectedValueOnce(new ApiError(500, 50000, '加载称号商店失败'))
      .mockResolvedValueOnce([title()])
    const failed = await mountPage()
    expect(failed.wrapper.text()).toContain('加载称号商店失败')
    const retry = failed.wrapper.findAll('button').find((item) => item.text().trim() === '重试')
    expect(retry).toBeTruthy()
    await retry!.trigger('click')
    await flushPromises()
    expect(failed.wrapper.text()).toContain('荣耀王者')
    failed.wrapper.unmount()
  })
})
