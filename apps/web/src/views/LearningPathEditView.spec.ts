import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import ToggleSwitch from 'primevue/toggleswitch'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import { ApiError } from '@/api/http'
import {
  createLearningPath,
  deleteLearningNode,
  deleteLearningPath,
  getLearningTree,
  updateLearningNode,
  updateLearningPath,
} from '@/api/learning'
import type { LearningPathItem, LearningPathNodeItem, Me } from '@/api/types'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import { PERM } from '@/permissions'
import LearningPathEditView from './LearningPathEditView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))
const auth = vi.hoisted(() => ({
  isLoggedIn: true,
  isSuperAdmin: false,
  can: vi.fn<(code: string) => boolean>(() => false),
  user: null as Me | null,
}))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/learning', () => ({
  getLearningTree: vi.fn(),
  createLearningPath: vi.fn(),
  updateLearningPath: vi.fn(),
  deleteLearningPath: vi.fn(),
  createLearningNode: vi.fn(),
  updateLearningNode: vi.fn(),
  deleteLearningNode: vi.fn(),
}))
vi.mock('@/components/MarkdownEditor.vue', () => ({
  default: {
    name: 'MarkdownEditor',
    props: ['modelValue', 'theme'],
    emits: ['update:modelValue'],
    template:
      '<textarea class="md-editor-stub" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
  },
}))
vi.mock('@/components/learning/LearningNodeDialog.vue', () => ({
  default: {
    name: 'LearningNodeDialog',
    props: ['visible', 'pathId', 'nodes', 'editing', 'parentId'],
    emits: ['update:visible', 'saved'],
    template: '<div class="node-dialog-stub" />',
  },
}))

function path(overrides: Partial<LearningPathItem> = {}): LearningPathItem {
  return {
    id: 3,
    title: '算法入门',
    descriptionMd: '从零开始学习。',
    coverUrl: '',
    isOfficial: false,
    isPublic: true,
    isActive: true,
    author: { id: 1, username: 'alice', nickname: 'Alice' },
    nodeCount: 2,
    myCompletedCount: 0,
    sortOrder: 0,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function node(overrides: Partial<LearningPathNodeItem> = {}): LearningPathNodeItem {
  return {
    id: 1,
    parentId: null,
    title: '基础语法',
    descriptionMd: '',
    problemIds: [],
    problems: [],
    sortOrder: 1,
    requiredCount: 1,
    solvedCount: 0,
    status: 'active',
    children: [],
    ...overrides,
  }
}

const routes = [
  { path: '/learning-paths', name: 'learning-paths', component: { template: '<div />' } },
  { path: '/learning-paths/new', name: 'learning-path-create', component: { template: '<div />' } },
  {
    path: '/learning-paths/:id(\\d+)/edit',
    name: 'learning-path-edit',
    component: { template: '<div />' },
  },
]

async function mountPage(route = '/learning-paths/new') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(route)
  await router.isReady()
  const wrapper = mount(LearningPathEditView, {
    global: { plugins: [router, PrimeVue] },
  })
  await flushPromises()
  return { wrapper, router }
}

function button(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper'], label: string) {
  return wrapper.findAll('button').find((item) => item.text().trim() === label)
}

function titleInput(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper']) {
  return wrapper.find('.path-title')
}

function rowButtons(row: Element, label: string) {
  return Array.from(row.querySelectorAll('button')).find(
    (item) => item.textContent?.trim() === label,
  )
}

beforeEach(() => {
  toast.add.mockReset()
  confirm.require.mockReset()
  auth.can.mockReset()
  auth.can.mockReturnValue(false)
  auth.isSuperAdmin = false
  vi.mocked(getLearningTree).mockReset()
  vi.mocked(createLearningPath).mockReset()
  vi.mocked(updateLearningPath).mockReset()
  vi.mocked(deleteLearningPath).mockReset()
  vi.mocked(updateLearningNode).mockReset()
  vi.mocked(deleteLearningNode).mockReset()

  vi.mocked(getLearningTree).mockResolvedValue({ path: path(), nodes: [] })
  vi.mocked(createLearningPath).mockResolvedValue(path({ id: 99 }))
  vi.mocked(updateLearningPath).mockResolvedValue(path())
  vi.mocked(deleteLearningPath).mockResolvedValue({ deleted: true })
})

describe('LearningPathEditView', () => {
  it('shows an empty form on /new and hides the official toggle for authors', async () => {
    const { wrapper } = await mountPage('/learning-paths/new')

    expect((titleInput(wrapper).element as HTMLInputElement).value).toBe('')
    expect(wrapper.find('#path-official').exists()).toBe(false)
    expect(wrapper.find('#path-public').exists()).toBe(true)
    expect(wrapper.text()).toContain('先保存路线，再添加节点')
    wrapper.unmount()
  })

  it('shows the official toggle for managers and forces public when official', async () => {
    auth.can.mockImplementation((code) => code === PERM.learningPath.official.manage)
    const { wrapper } = await mountPage('/learning-paths/new')

    const official = wrapper.find('#path-official')
    expect(official.exists()).toBe(true)

    await official.setValue(true)
    await nextTick()

    const toggles = wrapper.findAllComponents(ToggleSwitch)
    expect(toggles[0]!.props('modelValue')).toBe(true)
    expect(toggles[0]!.props('disabled')).toBe(true)
    wrapper.unmount()
  })

  it('pre-checks the official toggle from ?official=true', async () => {
    auth.can.mockImplementation((code) => code === PERM.learningPath.official.manage)
    const { wrapper } = await mountPage('/learning-paths/new?official=true')

    const official = wrapper.find('#path-official')
    expect((official.element as HTMLInputElement).checked).toBe(true)
    expect(wrapper.findAllComponents(ToggleSwitch)[0]!.props('disabled')).toBe(true)
    wrapper.unmount()
  })

  it('creates a path then redirects to its edit page', async () => {
    const { wrapper, router } = await mountPage('/learning-paths/new')

    await titleInput(wrapper).setValue('  我的路线  ')
    await wrapper.find('.md-editor-stub').setValue('描述 **Markdown**')
    await wrapper.find('.path-cover').setValue('https://example.com/c.png')
    await wrapper.find('#path-public').setValue(true)

    await button(wrapper, '保存')!.trigger('click')
    await flushPromises()

    expect(createLearningPath).toHaveBeenCalledWith({
      title: '我的路线',
      descriptionMd: '描述 **Markdown**',
      coverUrl: 'https://example.com/c.png',
      isPublic: true,
      sortOrder: 0,
    })
    expect(router.currentRoute.value.fullPath).toBe('/learning-paths/99/edit')
    wrapper.unmount()
  })

  it('creates an official path with isOfficial when manager', async () => {
    auth.can.mockImplementation((code) => code === PERM.learningPath.official.manage)
    const { wrapper } = await mountPage('/learning-paths/new')

    await titleInput(wrapper).setValue('官方路线')
    await wrapper.find('#path-official').setValue(true)
    await button(wrapper, '保存')!.trigger('click')
    await flushPromises()

    expect(createLearningPath).toHaveBeenCalledWith(
      expect.objectContaining({ title: '官方路线', isOfficial: true, isPublic: true }),
    )
    wrapper.unmount()
  })

  it('loads an existing path and its node tree, and updates on save', async () => {
    const nodes = [
      node({ id: 1, title: '根节点', sortOrder: 1 }),
      node({ id: 2, parentId: 1, title: '子节点', sortOrder: 1 }),
    ]
    vi.mocked(getLearningTree).mockResolvedValue({ path: path(), nodes })
    const { wrapper, router } = await mountPage('/learning-paths/3/edit')

    expect(getLearningTree).toHaveBeenCalledWith(3)
    expect((titleInput(wrapper).element as HTMLInputElement).value).toBe('算法入门')
    expect(wrapper.text()).toContain('根节点')
    expect(wrapper.text()).toContain('子节点')
    expect(wrapper.find('.node-dialog-stub').exists()).toBe(true)

    await titleInput(wrapper).setValue('新标题')
    await button(wrapper, '保存')!.trigger('click')
    await flushPromises()

    expect(updateLearningPath).toHaveBeenCalledWith(3, {
      title: '新标题',
      descriptionMd: '从零开始学习。',
      coverUrl: '',
      isPublic: true,
      sortOrder: 0,
    })
    expect(router.currentRoute.value.fullPath).toBe('/learning-paths/3/edit')
    wrapper.unmount()
  })

  it('opens the node dialog with parentId for a child and null for a root', async () => {
    const nodes = [node({ id: 1, title: '根节点' })]
    vi.mocked(getLearningTree).mockResolvedValue({ path: path(), nodes })
    const { wrapper } = await mountPage('/learning-paths/3/edit')

    const rootRow = wrapper.findAll('tr').find((tr) => tr.text().includes('根节点'))!
    await rowButtons(rootRow.element, '添加子节点')!.click()
    await flushPromises()
    const dialog = wrapper.findComponent({ name: 'LearningNodeDialog' })
    expect(dialog.props('parentId')).toBe(1)
    expect(dialog.props('editing')).toBeNull()

    await button(wrapper, '添加根节点')!.trigger('click')
    await flushPromises()
    expect(dialog.props('parentId')).toBeNull()
    expect(dialog.props('editing')).toBeNull()
    wrapper.unmount()
  })

  it('disables delete for nodes with children and deletes a leaf via the API', async () => {
    const child = node({ id: 2, parentId: 1, title: '子节点甲' })
    const root = node({ id: 1, title: '根节点', children: [child] })
    vi.mocked(getLearningTree).mockResolvedValue({ path: path(), nodes: [root] })
    vi.mocked(deleteLearningNode).mockResolvedValue({ deleted: true })
    const { wrapper } = await mountPage('/learning-paths/3/edit')

    const rootRow = wrapper.findAll('tr').find((tr) => tr.text().includes('根节点'))!
    expect(rowButtons(rootRow.element, '删除')!.hasAttribute('disabled')).toBe(true)

    const childRow = wrapper.findAll('tr').find((tr) => tr.text().includes('子节点甲'))!
    await rowButtons(childRow.element, '删除')!.click()
    await flushPromises()

    expect(confirm.require).toHaveBeenCalledTimes(1)
    const options = confirm.require.mock.calls[0]![0] as { accept?: () => void }
    options.accept?.()
    await flushPromises()

    expect(deleteLearningNode).toHaveBeenCalledWith(3, 2)
    expect(getLearningTree).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('moves a node up and only reindexes changed siblings', async () => {
    const nodes = [
      node({ id: 1, title: '节点1', sortOrder: 1 }),
      node({ id: 2, title: '节点2', sortOrder: 2 }),
    ]
    vi.mocked(getLearningTree).mockResolvedValue({ path: path(), nodes })
    vi.mocked(updateLearningNode).mockResolvedValue(node())
    const { wrapper } = await mountPage('/learning-paths/3/edit')

    const row = wrapper.findAll('tr').find((tr) => tr.text().includes('节点2'))!
    await rowButtons(row.element, '上移')!.click()
    await flushPromises()

    expect(updateLearningNode).toHaveBeenCalledTimes(2)
    expect(vi.mocked(updateLearningNode).mock.calls[0]).toEqual([
      3,
      2,
      expect.objectContaining({ sortOrder: 1, title: '节点2' }),
    ])
    expect(vi.mocked(updateLearningNode).mock.calls[1]).toEqual([
      3,
      1,
      expect.objectContaining({ sortOrder: 2, title: '节点1' }),
    ])
    expect(getLearningTree).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('deletes the path after confirmation and returns to the list', async () => {
    vi.mocked(getLearningTree).mockResolvedValue({ path: path(), nodes: [] })
    const { wrapper, router } = await mountPage('/learning-paths/3/edit')

    await button(wrapper, '删除路线')!.trigger('click')
    await flushPromises()

    expect(confirm.require).toHaveBeenCalledTimes(1)
    const options = confirm.require.mock.calls[0]![0] as { accept?: () => void }
    options.accept?.()
    await flushPromises()

    expect(deleteLearningPath).toHaveBeenCalledWith(3)
    expect(router.currentRoute.value.fullPath).toBe('/learning-paths')
    wrapper.unmount()
  })

  it('shows a not-found error state for a missing path', async () => {
    vi.mocked(getLearningTree).mockRejectedValue(
      new ApiError(404, 40400, '学习路线不存在或无权查看'),
    )
    const { wrapper } = await mountPage('/learning-paths/3/edit')

    expect(wrapper.text()).toContain('学习路线不存在或无权查看')
    expect(wrapper.findComponent(AppErrorState).exists()).toBe(true)
    wrapper.unmount()
  })
})
