import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Paginator from 'primevue/paginator'
import ToastService from 'primevue/toastservice'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick, reactive } from 'vue'

import { createSolutionPost, listSolutionPosts } from '@/api/discussions'
import { ApiError } from '@/api/http'
import type { SolutionPostDetail, SolutionPostItem } from '@/api/types'
import ProblemSolutionsView from './ProblemSolutionsView.vue'

const route = reactive({ params: { id: '1' }, fullPath: '/problems/1/solutions' })

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const auth = vi.hoisted(() => ({ isLoggedIn: true }))

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return { ...actual, useRoute: () => route }
})
vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/components/MarkdownEditor.vue', () => ({
  default: {
    props: ['modelValue', 'theme', 'height'],
    emits: ['update:modelValue'],
    template:
      '<textarea class="markdown-editor-mock" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
  },
}))
vi.mock('@/api/discussions', () => ({
  listSolutionPosts: vi.fn(),
  createSolutionPost: vi.fn(),
}))

function post(overrides: Partial<SolutionPostItem> = {}): SolutionPostItem {
  return {
    id: 1,
    problemId: 1,
    userId: 2,
    title: '如何 AC 这道题',
    excerpt: '先打表找规律……',
    author: { id: 2, username: 'alice', nickname: 'Alice' },
    replyCount: 3,
    createdAt: '2026-01-02T03:04:05Z',
    updatedAt: '2026-01-02T03:04:05Z',
    canEdit: false,
    canDelete: false,
    ...overrides,
  }
}

function createdDetail(overrides: Partial<SolutionPostDetail> = {}): SolutionPostDetail {
  return { ...post(), contentMd: '正文内容', replies: [], ...overrides }
}

function mountView() {
  return mount(ProblemSolutionsView, {
    global: {
      plugins: [PrimeVue, ToastService, createPinia()],
      stubs: {
        RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
      },
    },
  })
}

function button(wrapper: ReturnType<typeof mountView>, label: string) {
  return wrapper.findAll('button').find((b) => b.text().trim() === label)
}

beforeEach(() => {
  route.params.id = '1'
  auth.isLoggedIn = true
  toast.add.mockReset()
  vi.mocked(listSolutionPosts).mockReset()
  vi.mocked(createSolutionPost).mockReset()
})

describe('ProblemSolutionsView', () => {
  it('lists posts and paginates with author/reply count', async () => {
    vi.mocked(listSolutionPosts).mockResolvedValue({ total: 45, items: [post()] })
    const wrapper = mountView()
    await flushPromises()

    expect(listSolutionPosts).toHaveBeenCalledWith(1, { page: 1, pageSize: 20 })
    expect(wrapper.text()).toContain('如何 AC 这道题')
    expect(wrapper.text()).toContain('Alice')
    expect(wrapper.text()).toContain('3 回复')
    expect(wrapper.find('a[href="/problems/1/solutions/1"]').exists()).toBe(true)

    wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()
    expect(listSolutionPosts).toHaveBeenLastCalledWith(1, { page: 2, pageSize: 20 })
    wrapper.unmount()
  })

  it('creates a post via dialog', async () => {
    vi.mocked(listSolutionPosts).mockResolvedValue({ total: 0, items: [] })
    vi.mocked(createSolutionPost).mockResolvedValue(createdDetail())
    const wrapper = mountView()
    await flushPromises()

    await button(wrapper, '发布思路')!.trigger('click')
    await flushPromises()
    const dialog = document.querySelector('.p-dialog') as HTMLElement
    expect(dialog).toBeTruthy()

    const title = dialog.querySelector('input') as HTMLInputElement
    title.value = '我的思路'
    title.dispatchEvent(new Event('input', { bubbles: true }))
    const md = dialog.querySelector('.markdown-editor-mock') as HTMLTextAreaElement
    md.value = '正文内容'
    md.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()

    ;(dialog.querySelector('.publish-submit') as HTMLElement).click()
    await flushPromises()

    expect(createSolutionPost).toHaveBeenCalledWith(1, { title: '我的思路', contentMd: '正文内容' })
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
    wrapper.unmount()
  })

  it('disables publish when empty and shows Chinese length errors', async () => {
    vi.mocked(listSolutionPosts).mockResolvedValue({ total: 0, items: [] })
    const wrapper = mountView()
    await flushPromises()

    await button(wrapper, '发布思路')!.trigger('click')
    await flushPromises()
    const dialog = document.querySelector('.p-dialog') as HTMLElement
    const submit = dialog.querySelector('.publish-submit') as HTMLButtonElement
    expect(submit.disabled).toBe(true)

    const title = dialog.querySelector('input') as HTMLInputElement
    title.value = 'x'.repeat(101)
    title.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    expect(dialog.textContent).toContain('标题长度需为 1–100 个字符')
    wrapper.unmount()
  })

  it('shows empty and error states', async () => {
    vi.mocked(listSolutionPosts).mockResolvedValueOnce({ total: 0, items: [] })
    const empty = mountView()
    await flushPromises()
    expect(empty.text()).toContain('还没有人分享思路')
    empty.unmount()

    vi.mocked(listSolutionPosts).mockRejectedValueOnce(new ApiError(500, 50000, '加载思路失败'))
    const failed = mountView()
    await flushPromises()
    expect(failed.text()).toContain('加载思路失败')
    expect(button(failed, '重试')).toBeTruthy()
    failed.unmount()
  })

  it('clears the previous problem posts while switching problems', async () => {
    vi.mocked(listSolutionPosts).mockResolvedValueOnce({
      total: 1,
      items: [post({ title: '旧题的思路' })],
    })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('旧题的思路')

    let resolveSecond!: (value: { total: number; items: SolutionPostItem[] }) => void
    vi.mocked(listSolutionPosts).mockImplementationOnce(
      () => new Promise((resolve) => (resolveSecond = resolve)),
    )
    route.params.id = '2'
    await nextTick()

    expect(wrapper.text()).not.toContain('旧题的思路')
    expect(wrapper.text()).toContain('正在加载解题思路')

    resolveSecond({ total: 1, items: [post({ id: 2, problemId: 2, title: '新题的思路' })] })
    await flushPromises()
    expect(wrapper.text()).toContain('新题的思路')
    wrapper.unmount()
  })

  it('shows an error when switching to a problem whose load fails', async () => {
    vi.mocked(listSolutionPosts).mockResolvedValueOnce({
      total: 1,
      items: [post({ title: '旧题的思路' })],
    })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('旧题的思路')

    vi.mocked(listSolutionPosts).mockRejectedValueOnce(new ApiError(500, 50000, '加载思路失败'))
    route.params.id = '2'
    await flushPromises()

    expect(wrapper.text()).not.toContain('旧题的思路')
    expect(wrapper.text()).toContain('加载思路失败')
    wrapper.unmount()
  })

  it('shows an error when a page change fails with existing posts', async () => {
    vi.mocked(listSolutionPosts).mockResolvedValueOnce({ total: 45, items: [post()] })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('如何 AC 这道题')

    vi.mocked(listSolutionPosts).mockRejectedValueOnce(new ApiError(500, 50000, '加载思路失败'))
    wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()

    expect(wrapper.text()).toContain('加载思路失败')
    expect(wrapper.text()).toContain('如何 AC 这道题')
    wrapper.unmount()
  })

  it('shows login action to guests while still listing posts', async () => {
    auth.isLoggedIn = false
    vi.mocked(listSolutionPosts).mockResolvedValue({ total: 1, items: [post()] })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('如何 AC 这道题')
    expect(wrapper.find('a.login-link').exists()).toBe(true)
    expect(button(wrapper, '发布思路')).toBeUndefined()
    wrapper.unmount()
  })
})
