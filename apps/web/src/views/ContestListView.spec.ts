import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import DataTable from 'primevue/datatable'
import Paginator from 'primevue/paginator'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/http'
import { cloneContest, deleteContest, listContests } from '@/api/contests'
import type { ContestDetail, ContestListItem } from '@/api/types'
import ContestListView from './ContestListView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))
const auth = vi.hoisted(() => ({
  isLoggedIn: true,
  user: { role: 'contest_admin' } as { role: string } | null,
}))

vi.mock('primevue/usetoast', () => ({
  useToast: () => toast,
}))
vi.mock('primevue/useconfirm', () => ({
  useConfirm: () => confirm,
}))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => auth,
}))
vi.mock('@/api/contests', () => ({
  listContests: vi.fn(),
  cloneContest: vi.fn(),
  deleteContest: vi.fn(),
}))

function contest(overrides: Partial<ContestListItem> = {}): ContestListItem {
  return {
    id: 1,
    title: 'Weekly Contest',
    descriptionMd: '',
    startTime: '2026-01-01T00:00:00Z',
    endTime: '2026-01-02T00:00:00Z',
    mode: 'icpc',
    isRated: true,
    isPublic: true,
    registerType: 'open',
    status: 'running',
    problemCount: 5,
    participantCount: 20,
    registered: true,
    createdAt: '2025-12-01T00:00:00Z',
    ...overrides,
  }
}

const routes = [
  { path: '/contests', name: 'contests', component: { template: '<div />' } },
  { path: '/contests/new', name: 'contest-create', component: { template: '<div />' } },
  { path: '/contests/:id(\\d+)', name: 'contest-detail', component: { template: '<div />' } },
  { path: '/contests/:id(\\d+)/edit', name: 'contest-edit', component: { template: '<div />' } },
]

async function mountPage(path = '/contests') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(ContestListView, { global: { plugins: [router, PrimeVue] } })
  await flushPromises()
  return { wrapper, router }
}

function button(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper'], label: string) {
  return wrapper.findAll('button').find((item) => item.text().trim() === label)
}

beforeEach(() => {
  auth.isLoggedIn = true
  auth.user = { role: 'contest_admin' }
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(listContests).mockReset()
  vi.mocked(cloneContest).mockReset()
  vi.mocked(deleteContest).mockReset()
  vi.mocked(listContests).mockResolvedValue({ total: 45, items: [contest()] })
  vi.mocked(cloneContest).mockResolvedValue({ id: 99 } as ContestDetail)
  vi.mocked(deleteContest).mockResolvedValue(null)
})

describe('ContestListView', () => {
  it('loads from the route query and renders a contest row', async () => {
    const { wrapper } = await mountPage(
      '/contests?keyword=spring&mode=icpc&status=running&mine=true&includeHidden=true&page=2',
    )

    expect(listContests).toHaveBeenCalledWith({
      keyword: 'spring',
      mode: 'icpc',
      status: 'running',
      mine: true,
      includeHidden: true,
      page: 2,
      pageSize: 20,
    })
    expect(wrapper.text()).toContain('Weekly Contest')
    expect(wrapper.text()).toContain('已报名')
    expect(wrapper.text()).toContain('公开报名')
    expect(wrapper.text()).toContain('ICPC')
    expect(wrapper.text()).toContain('进行中')
    expect(wrapper.text()).toContain('5 题 / 20 人')
    expect(wrapper.find('a[href="/contests/1"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('applies keyword filters and page changes through the URL', async () => {
    const { wrapper, router } = await mountPage()

    await wrapper.find('#filter-keyword').setValue(' spring ')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({ keyword: 'spring' })
    expect(listContests).toHaveBeenLastCalledWith(
      expect.objectContaining({ keyword: 'spring', page: 1 }),
    )

    wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ keyword: 'spring', page: '2' })

    await button(wrapper, '重置')!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({})
    wrapper.unmount()
  })

  it('hides my/mine and hidden controls from guests', async () => {
    auth.isLoggedIn = false
    auth.user = null
    const { wrapper } = await mountPage()

    expect(wrapper.find('#filter-mine').exists()).toBe(false)
    expect(wrapper.find('#filter-hidden').exists()).toBe(false)
    expect(button(wrapper, '新建比赛')).toBeUndefined()
    expect(button(wrapper, '编辑')).toBeUndefined()
    wrapper.unmount()
  })

  it('opens the detail page on row click', async () => {
    const { wrapper, router } = await mountPage()

    wrapper.findComponent(DataTable).vm.$emit('row-click', { data: contest() })
    await flushPromises()

    expect(router.currentRoute.value.fullPath).toBe('/contests/1')
    wrapper.unmount()
  })

  it('clones a contest through a dialog and navigates to the editor', async () => {
    const { wrapper, router } = await mountPage()

    await button(wrapper, '克隆')!.trigger('click')
    await flushPromises()
    const dialog = document.querySelector('.p-dialog')
    expect(dialog).toBeTruthy()
    const input = dialog!.querySelector('input') as HTMLInputElement
    input.value = 'Copy Contest'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    ;(dialog!.querySelector('.clone-submit') as HTMLElement).click()
    await flushPromises()

    expect(cloneContest).toHaveBeenCalledWith(1, { title: 'Copy Contest' })
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
    expect(router.currentRoute.value.fullPath).toBe('/contests/99/edit')
    wrapper.unmount()
  })

  it('confirms deletion with Chinese labels and reloads the list', async () => {
    const { wrapper } = await mountPage()

    await button(wrapper, '删除')!.trigger('click')
    expect(confirm.require).toHaveBeenCalled()
    const options = confirm.require.mock.calls[0]![0] as {
      header?: string
      message?: string
      acceptLabel?: string
      rejectLabel?: string
      accept?: () => void
    }
    expect(options.header).toBe('删除比赛')
    expect(options.message).toContain('Weekly Contest')
    expect(options.acceptLabel).toBe('删除')
    expect(options.rejectLabel).toBe('取消')

    options.accept?.()
    await flushPromises()
    expect(deleteContest).toHaveBeenCalledWith(1)
    expect(listContests).toHaveBeenCalledTimes(2)
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
    wrapper.unmount()
  })

  it('shows empty and error states', async () => {
    vi.mocked(listContests).mockResolvedValueOnce({ total: 0, items: [] })
    const empty = await mountPage()
    expect(empty.wrapper.text()).toContain('暂无比赛')
    empty.wrapper.unmount()

    vi.mocked(listContests).mockRejectedValueOnce(new ApiError(500, 50000, '加载比赛失败'))
    const failed = await mountPage()
    expect(failed.wrapper.text()).toContain('加载比赛失败')
    expect(button(failed.wrapper, '重试')).toBeTruthy()
    failed.wrapper.unmount()
  })
})
