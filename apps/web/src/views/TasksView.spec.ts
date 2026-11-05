import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import ProgressBar from 'primevue/progressbar'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { claimTask, listTasks } from '@/api/gamification'
import { ApiError } from '@/api/http'
import type { TaskItem } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import TasksView from './TasksView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/gamification', () => ({
  listTasks: vi.fn(),
  claimTask: vi.fn(),
}))
vi.mock('@/components/MarkdownView.vue', () => ({
  default: {
    name: 'MarkdownView',
    props: ['source', 'theme'],
    template: '<div class="md-stub">{{ source }}</div>',
  },
}))

function iso(offsetMs = 0): string {
  return new Date(Date.now() + offsetMs).toISOString()
}

function task(overrides: Partial<TaskItem> = {}): TaskItem {
  return {
    id: 1,
    title: '新手任务',
    descriptionMd: '完成 **5** 道题',
    problemIds: [1, 2, 3, 4, 5],
    rewardCoins: 100,
    startTime: iso(-86_400_000),
    endTime: iso(86_400_000),
    isActive: true,
    status: 'active',
    claimedAt: null,
    completedAt: null,
    progress: 0,
    problemCount: 5,
    hasRecords: false,
    ...overrides,
  }
}

const routes = [
  { path: '/tasks', name: 'tasks', component: { template: '<div />' } },
  { path: '/login', name: 'login', component: { template: '<div />' } },
]

async function mountPage(path = '/tasks', loggedIn = true) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  if (loggedIn) {
    auth.setTokens({ accessToken: 'access', refreshToken: 'refresh' })
  }
  const wrapper = mount(TasksView, { global: { plugins: [router, PrimeVue, pinia] } })
  await flushPromises()
  return { wrapper, router, auth }
}

function button(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper'], label: string) {
  return wrapper.findAll('button').find((item) => item.text().trim() === label)
}

beforeEach(() => {
  localStorage.clear()
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(listTasks).mockReset()
  vi.mocked(claimTask).mockReset()
  vi.mocked(listTasks).mockResolvedValue([
    task(),
    task({
      id: 2,
      title: '进阶任务',
      status: 'claimed',
      claimedAt: iso(-3_600_000),
      progress: 3,
    }),
    task({
      id: 3,
      title: '已完成任务',
      status: 'completed',
      claimedAt: iso(-7_200_000),
      completedAt: iso(-3_600_000),
      progress: 5,
    }),
    task({ id: 4, title: '已截止任务', status: 'expired' }),
    task({ id: 5, title: '未开始任务', status: 'upcoming' }),
  ])
  vi.mocked(claimTask).mockResolvedValue(task({ status: 'claimed', claimedAt: iso(0) }))
})

describe('TasksView', () => {
  it('renders task states, rewards, times and progress', async () => {
    const { wrapper } = await mountPage()

    expect(listTasks).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('任务大厅')
    expect(wrapper.text()).toContain('进度只统计领取后的 AC')
    expect(wrapper.text()).toContain('新手任务')
    expect(wrapper.text()).toContain('奖励 100 金币')
    expect(wrapper.text()).toContain('可领取')
    expect(wrapper.text()).toContain('完成 **5** 道题')
    expect(wrapper.text()).toContain('开始')
    expect(wrapper.text()).toContain('截止')

    expect(wrapper.text()).toContain('进阶任务')
    expect(wrapper.text()).toContain('完成进度')
    expect(wrapper.text()).toContain('3 / 5')
    expect(wrapper.text()).toContain('完成全部题目后自动发放奖励')

    expect(wrapper.text()).toContain('已完成任务')
    expect(wrapper.text()).toContain('奖励已发放')
    expect(wrapper.text()).toContain('完成')

    expect(wrapper.text()).toContain('任务已截止，未能完成')
    expect(wrapper.text()).toContain('任务尚未开始')
    expect(wrapper.findAll('.task-card')).toHaveLength(5)
    expect(wrapper.findAll('.task-card.is-completed')).toHaveLength(1)

    const progress = wrapper.findComponent(ProgressBar)
    expect(progress.props('value')).toBe(60)
    wrapper.unmount()
  })

  it('confirms a claim and refreshes the list', async () => {
    const { wrapper, auth } = await mountPage()
    const fetchMe = vi.spyOn(auth, 'fetchMe').mockResolvedValue()

    await button(wrapper, '领取任务')!.trigger('click')
    expect(confirm.require).toHaveBeenCalled()
    const options = confirm.require.mock.calls[0]![0] as {
      header?: string
      message?: string
      acceptLabel?: string
      rejectLabel?: string
      accept?: () => void
    }
    expect(options.header).toBe('领取任务')
    expect(options.message).toBe('确定领取任务「新手任务」吗？领取后需在截止时间前完成全部题目。')
    expect(options.acceptLabel).toBe('领取')
    expect(options.rejectLabel).toBe('取消')

    options.accept?.()
    await flushPromises()

    expect(claimTask).toHaveBeenCalledWith(1)
    expect(fetchMe).toHaveBeenCalled()
    expect(listTasks).toHaveBeenCalledTimes(2)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'success',
        summary: '领取成功，完成全部题目即可获得奖励',
      }),
    )
    wrapper.unmount()
  })

  it('reports claim failures and resets the button state', async () => {
    vi.mocked(claimTask).mockRejectedValue(new ApiError(400, 40001, '任务已领取'))
    const { wrapper, auth } = await mountPage()
    const fetchMe = vi.spyOn(auth, 'fetchMe').mockResolvedValue()

    await button(wrapper, '领取任务')!.trigger('click')
    const options = confirm.require.mock.calls[0]![0] as { accept?: () => void }
    options.accept?.()
    await flushPromises()

    expect(fetchMe).not.toHaveBeenCalled()
    expect(listTasks).toHaveBeenCalledTimes(1)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'error',
        summary: '领取失败',
        detail: '任务已领取',
      }),
    )
    expect(button(wrapper, '领取任务')!.attributes()).not.toHaveProperty('disabled')
    wrapper.unmount()
  })

  it('shows guest login calls to action', async () => {
    const { wrapper } = await mountPage('/tasks', false)

    expect(wrapper.text()).toContain('登录后即可领取任务并记录完成进度。')
    const loginLinks = wrapper
      .findAll('a')
      .filter((item) => item.text().includes('登录 / 注册') || item.text().includes('登录后领取'))
    expect(loginLinks.length).toBeGreaterThanOrEqual(1)
    expect(loginLinks[0]!.attributes('href')).toBe('/login?redirect=/tasks')
    expect(button(wrapper, '领取任务')).toBeUndefined()
    expect(wrapper.text()).toContain('登录后领取')
    wrapper.unmount()
  })

  it('reloads when the login state changes', async () => {
    const { wrapper, auth } = await mountPage()
    expect(listTasks).toHaveBeenCalledTimes(1)

    auth.clear()
    await flushPromises()
    expect(listTasks).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('shows an empty state', async () => {
    vi.mocked(listTasks).mockResolvedValue([])
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('暂无任务')
    wrapper.unmount()
  })

  it('shows a loading state', async () => {
    let resolve!: (value: TaskItem[]) => void
    vi.mocked(listTasks).mockReturnValue(
      new Promise<TaskItem[]>((r) => {
        resolve = r
      }),
    )
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('正在加载任务')
    resolve([task()])
    await flushPromises()
    expect(wrapper.text()).toContain('新手任务')
    wrapper.unmount()
  })

  it('shows an error and retries', async () => {
    vi.mocked(listTasks)
      .mockRejectedValueOnce(new ApiError(500, 50000, '加载任务失败'))
      .mockResolvedValueOnce([task()])
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('加载任务失败')
    const retry = wrapper.findAll('button').find((item) => item.text().trim() === '重试')
    expect(retry).toBeTruthy()
    await retry!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('新手任务')
    wrapper.unmount()
  })
})
