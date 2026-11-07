import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import SelectButton from 'primevue/selectbutton'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/http'
import { deleteLearningPath, listLearningPaths, updateLearningPath } from '@/api/learning'
import type { LearningPathItem } from '@/api/types'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import { PERM } from '@/permissions'
import AdminLearningPathsView from './AdminLearningPathsView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))
const auth = vi.hoisted(() => ({
  can: vi.fn<(code: string) => boolean>(() => false),
}))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/learning', () => ({
  listLearningPaths: vi.fn(),
  deleteLearningPath: vi.fn(),
  updateLearningPath: vi.fn(),
}))

function path(overrides: Partial<LearningPathItem> = {}): LearningPathItem {
  return {
    id: 1,
    title: '算法入门',
    descriptionMd: '从零开始学习基础算法。',
    coverUrl: '',
    isOfficial: true,
    isPublic: true,
    isActive: true,
    author: { id: 1, username: 'admin', nickname: '管理员' },
    nodeCount: 5,
    myCompletedCount: 2,
    sortOrder: 0,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-02T00:00:00Z',
    ...overrides,
  }
}

const routes = [
  { path: '/', component: { template: '<div />' } },
  { path: '/learning-paths/new', name: 'learning-path-create', component: { template: '<div />' } },
  {
    path: '/learning-paths/:id/edit',
    name: 'learning-path-edit',
    component: { template: '<div />' },
  },
]

async function mountPage() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/')
  await router.isReady()
  const wrapper = mount(AdminLearningPathsView, {
    global: { plugins: [router, PrimeVue] },
  })
  await flushPromises()
  return { wrapper, router }
}

function buttons(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper'], label: string) {
  return wrapper.findAll('button, a').filter((item) => item.text().trim() === label)
}

beforeEach(() => {
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(listLearningPaths).mockReset()
  vi.mocked(deleteLearningPath).mockReset()
  vi.mocked(updateLearningPath).mockReset()
  auth.can.mockReset()
  auth.can.mockReturnValue(false)
})

describe('AdminLearningPathsView', () => {
  it('loads all paths with includeInactive and renders row actions', async () => {
    auth.can.mockImplementation((code) => code === PERM.learningPath.official.manage)
    vi.mocked(listLearningPaths).mockResolvedValue({
      total: 2,
      items: [
        path(),
        path({
          id: 2,
          isOfficial: false,
          isPublic: false,
          isActive: false,
          title: '私有已下架',
          author: { id: 2, username: 'bob', nickname: 'Bob' },
        }),
      ],
    })
    const { wrapper } = await mountPage()

    expect(listLearningPaths).toHaveBeenCalledWith({ includeInactive: true })
    // Column headers.
    for (const header of [
      '标题',
      '类型',
      '可见性',
      '状态',
      '作者',
      '节点数',
      '我的完成',
      '更新时间',
    ]) {
      expect(wrapper.text()).toContain(header)
    }
    // Type / visibility / status tags.
    expect(wrapper.text()).toContain('官方')
    expect(wrapper.text()).toContain('自定义')
    expect(wrapper.text()).toContain('公开')
    expect(wrapper.text()).toContain('私有')
    expect(wrapper.text()).toContain('上架')
    expect(wrapper.text()).toContain('已下架')
    // Author and counts.
    expect(wrapper.text()).toContain('管理员')
    expect(wrapper.text()).toContain('Bob')
    expect(wrapper.text()).toContain('算法入门')
    expect(wrapper.text()).toContain('私有已下架')

    // Write actions present for the official manager.
    expect(buttons(wrapper, '新建官方路线')).toHaveLength(1)
    expect(buttons(wrapper, '编辑')).toHaveLength(2)
    expect(buttons(wrapper, '下架')).toHaveLength(1)
    expect(buttons(wrapper, '恢复')).toHaveLength(1)
    wrapper.unmount()
  })

  it('official manager can create official, down and restore', async () => {
    auth.can.mockImplementation((code) => code === PERM.learningPath.official.manage)
    vi.mocked(listLearningPaths).mockResolvedValue({
      total: 2,
      items: [
        path({ id: 1, title: '上架路线' }),
        path({
          id: 2,
          title: '下架路线',
          isActive: false,
          isOfficial: false,
          isPublic: false,
          sortOrder: 3,
        }),
      ],
    })
    vi.mocked(deleteLearningPath).mockResolvedValue({ deleted: true })
    vi.mocked(updateLearningPath).mockResolvedValue(path())
    const { wrapper, router } = await mountPage()

    // Create official -> /learning-paths/new?official=true
    await buttons(wrapper, '新建官方路线')[0]!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/learning-paths/new?official=true')

    // Edit -> /learning-paths/:id/edit
    await buttons(wrapper, '编辑')[0]!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/learning-paths/1/edit')

    // Down -> deleteLearningPath with confirm, then reload.
    await buttons(wrapper, '下架')[0]!.trigger('click')
    const downOptions = confirm.require.mock.calls[0]![0] as {
      header?: string
      accept?: () => void
    }
    expect(downOptions.header).toBe('下架学习路线')
    downOptions.accept?.()
    await flushPromises()
    expect(deleteLearningPath).toHaveBeenCalledWith(1)
    expect(listLearningPaths).toHaveBeenCalledTimes(2)

    // Restore -> updateLearningPath with full fields + isActive:true, then reload.
    await buttons(wrapper, '恢复')[0]!.trigger('click')
    const restoreOptions = confirm.require.mock.calls[1]![0] as {
      header?: string
      accept?: () => void
    }
    expect(restoreOptions.header).toBe('恢复学习路线')
    restoreOptions.accept?.()
    await flushPromises()
    expect(updateLearningPath).toHaveBeenCalledWith(2, {
      title: '下架路线',
      descriptionMd: '从零开始学习基础算法。',
      coverUrl: '',
      isPublic: false,
      sortOrder: 3,
      isActive: true,
    })
    expect(listLearningPaths).toHaveBeenCalledTimes(3)
    wrapper.unmount()
  })

  it('admin.read sees rows but no write actions', async () => {
    auth.can.mockImplementation((code) => code === PERM.learningPath.admin.read)
    vi.mocked(listLearningPaths).mockResolvedValue({
      total: 2,
      items: [path(), path({ id: 2, isOfficial: false, isActive: false, title: '已下架自定义' })],
    })
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('算法入门')
    expect(wrapper.text()).toContain('已下架自定义')
    for (const label of ['新建官方路线', '编辑', '下架', '恢复']) {
      expect(buttons(wrapper, label)).toHaveLength(0)
    }
    wrapper.unmount()
  })

  it('type/status filters work client-side', async () => {
    vi.mocked(listLearningPaths).mockResolvedValue({
      total: 3,
      items: [
        path({ id: 1, isOfficial: true, isActive: true, title: '甲官方路线' }),
        path({ id: 2, isOfficial: false, isPublic: true, isActive: true, title: '乙自定义路线' }),
        path({ id: 3, isOfficial: false, isPublic: false, isActive: false, title: '丙私有路线' }),
      ],
    })
    const { wrapper } = await mountPage()

    const [typeFilter, statusFilter] = wrapper.findAllComponents(SelectButton)

    // Type filter: 官方
    typeFilter.vm.$emit('update:modelValue', 'official')
    await flushPromises()
    expect(wrapper.text()).toContain('甲官方路线')
    expect(wrapper.text()).not.toContain('乙自定义路线')
    expect(wrapper.text()).not.toContain('丙私有路线')

    // Type filter: 自定义
    typeFilter.vm.$emit('update:modelValue', 'custom')
    await flushPromises()
    expect(wrapper.text()).not.toContain('甲官方路线')
    expect(wrapper.text()).toContain('乙自定义路线')
    expect(wrapper.text()).toContain('丙私有路线')

    // Reset type to 全部, status to 已下架
    typeFilter.vm.$emit('update:modelValue', 'all')
    statusFilter.vm.$emit('update:modelValue', 'inactive')
    await flushPromises()
    expect(wrapper.text()).not.toContain('甲官方路线')
    expect(wrapper.text()).not.toContain('乙自定义路线')
    expect(wrapper.text()).toContain('丙私有路线')

    // Status filter: 上架
    statusFilter.vm.$emit('update:modelValue', 'active')
    await flushPromises()
    expect(wrapper.text()).toContain('甲官方路线')
    expect(wrapper.text()).toContain('乙自定义路线')
    expect(wrapper.text()).not.toContain('丙私有路线')
    wrapper.unmount()
  })

  it('empty/error states', async () => {
    vi.mocked(listLearningPaths).mockResolvedValueOnce({ total: 0, items: [] })
    const empty = await mountPage()
    expect(empty.wrapper.text()).toContain('暂无学习路线')
    empty.wrapper.unmount()

    vi.mocked(listLearningPaths).mockRejectedValueOnce(new ApiError(500, 50000, '加载学习路线失败'))
    const failed = await mountPage()
    expect(failed.wrapper.text()).toContain('加载学习路线失败')
    expect(failed.wrapper.text()).toContain('重试')
    expect(failed.wrapper.findComponent(AppErrorState).exists()).toBe(true)
    failed.wrapper.unmount()
  })
})
