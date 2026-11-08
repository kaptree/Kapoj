import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'

import {
  createSolutionReply,
  deleteSolutionPost,
  deleteSolutionReply,
  getSolutionPost,
  updateSolutionPost,
  updateSolutionReply,
} from '@/api/discussions'
import { ApiError } from '@/api/http'
import { likeSolutionPost, unlikeSolutionPost } from '@/api/social'
import type { SolutionPostDetail, SolutionReplyItem } from '@/api/types'
import ProblemSolutionDetailView from './ProblemSolutionDetailView.vue'

const route = reactive({
  params: { id: '1', postId: '5' },
  fullPath: '/problems/1/solutions/5',
})

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))
const routerPush = vi.hoisted(() => vi.fn())
const auth = vi.hoisted(() => ({ isLoggedIn: true }))

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return { ...actual, useRoute: () => route, useRouter: () => ({ push: routerPush }) }
})
vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/components/MarkdownView.vue', () => ({
  default: { props: ['source', 'theme'], template: '<div class="md-stub">{{ source }}</div>' },
}))
vi.mock('@/components/MarkdownEditor.vue', () => ({
  default: {
    props: ['modelValue', 'theme', 'height'],
    emits: ['update:modelValue'],
    template:
      '<textarea class="markdown-editor-mock" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
  },
}))
vi.mock('@/api/discussions', () => ({
  getSolutionPost: vi.fn(),
  createSolutionReply: vi.fn(),
  updateSolutionPost: vi.fn(),
  deleteSolutionPost: vi.fn(),
  updateSolutionReply: vi.fn(),
  deleteSolutionReply: vi.fn(),
}))
vi.mock('@/api/social', () => ({
  likeSolutionPost: vi.fn(),
  unlikeSolutionPost: vi.fn(),
}))

function reply(overrides: Partial<SolutionReplyItem> = {}): SolutionReplyItem {
  return {
    id: 10,
    postId: 5,
    userId: 3,
    contentMd: '回复正文',
    author: { id: 3, username: 'bob', nickname: 'Bob' },
    createdAt: '2026-01-02T04:00:00Z',
    updatedAt: '2026-01-02T04:00:00Z',
    canEdit: false,
    canDelete: false,
    ...overrides,
  }
}

function detail(overrides: Partial<SolutionPostDetail> = {}): SolutionPostDetail {
  return {
    id: 5,
    problemId: 1,
    userId: 2,
    title: '如何 AC 这道题',
    excerpt: '先打表找规律……',
    author: { id: 2, username: 'alice', nickname: 'Alice' },
    replyCount: 1,
    likeCount: 0,
    likedByMe: false,
    createdAt: '2026-01-02T03:04:05Z',
    updatedAt: '2026-01-02T03:04:05Z',
    canEdit: true,
    canDelete: true,
    contentMd: '帖子正文 markdown',
    replies: [reply()],
    ...overrides,
  }
}

function mountView() {
  return mount(ProblemSolutionDetailView, {
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
  auth.isLoggedIn = true
  toast.add.mockReset()
  confirm.require.mockReset()
  routerPush.mockReset()
  vi.mocked(getSolutionPost).mockReset()
  vi.mocked(createSolutionReply).mockReset()
  vi.mocked(updateSolutionPost).mockReset()
  vi.mocked(deleteSolutionPost).mockReset()
  vi.mocked(updateSolutionReply).mockReset()
  vi.mocked(deleteSolutionReply).mockReset()
  vi.mocked(likeSolutionPost).mockReset()
  vi.mocked(unlikeSolutionPost).mockReset()
})

describe('ProblemSolutionDetailView', () => {
  it('renders detail, replies, and reply composer', async () => {
    vi.mocked(getSolutionPost).mockResolvedValue(detail())
    const wrapper = mountView()
    await flushPromises()

    expect(getSolutionPost).toHaveBeenCalledWith(1, 5)
    expect(wrapper.text()).toContain('如何 AC 这道题')
    expect(wrapper.text()).toContain('Alice')
    expect(wrapper.text()).toContain('帖子正文 markdown')
    expect(wrapper.text()).toContain('Bob')
    expect(wrapper.text()).toContain('回复正文')
    expect(wrapper.find('.markdown-editor-mock').exists()).toBe(true)
    wrapper.unmount()
  })

  it('submits a reply', async () => {
    vi.mocked(getSolutionPost).mockResolvedValue(detail({ replies: [] }))
    vi.mocked(createSolutionReply).mockResolvedValue(reply({ canEdit: true, canDelete: true }))
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('.markdown-editor-mock').setValue('我的回复')
    await button(wrapper, '发布回复')!.trigger('click')
    await flushPromises()

    expect(createSolutionReply).toHaveBeenCalledWith(1, 5, { contentMd: '我的回复' })
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
    wrapper.unmount()
  })

  it('edits own post', async () => {
    vi.mocked(getSolutionPost).mockResolvedValue(detail())
    vi.mocked(updateSolutionPost).mockResolvedValue(
      detail({ title: '新标题', contentMd: '新正文' }),
    )
    const wrapper = mountView()
    await flushPromises()

    await button(wrapper, '编辑思路')!.trigger('click')
    await flushPromises()
    const dialog = document.querySelector('.p-dialog') as HTMLElement
    const title = dialog.querySelector('input') as HTMLInputElement
    title.value = '新标题'
    title.dispatchEvent(new Event('input', { bubbles: true }))
    const md = dialog.querySelector('.markdown-editor-mock') as HTMLTextAreaElement
    md.value = '新正文'
    md.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()

    ;(dialog.querySelector('.edit-post-submit') as HTMLElement).click()
    await flushPromises()

    expect(updateSolutionPost).toHaveBeenCalledWith(1, 5, { title: '新标题', contentMd: '新正文' })
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
    wrapper.unmount()
  })

  it('edits and deletes a reply when allowed', async () => {
    vi.mocked(getSolutionPost).mockResolvedValue(
      detail({ replies: [reply({ canEdit: true, canDelete: true })] }),
    )
    vi.mocked(updateSolutionReply).mockResolvedValue(
      reply({ canEdit: true, canDelete: true, contentMd: '回复已编辑' }),
    )
    vi.mocked(deleteSolutionReply).mockResolvedValue(null)
    const wrapper = mountView()
    await flushPromises()

    await button(wrapper, '编辑')!.trigger('click')
    await flushPromises()
    const dialog = document.querySelector('.p-dialog') as HTMLElement
    const md = dialog.querySelector('.markdown-editor-mock') as HTMLTextAreaElement
    md.value = '回复已编辑'
    md.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    ;(dialog.querySelector('.edit-reply-submit') as HTMLElement).click()
    await flushPromises()

    expect(updateSolutionReply).toHaveBeenCalledWith(1, 5, 10, { contentMd: '回复已编辑' })

    await button(wrapper, '删除')!.trigger('click')
    expect(confirm.require).toHaveBeenCalled()
    const options = confirm.require.mock.calls[0]![0] as {
      header?: string
      acceptLabel?: string
      rejectLabel?: string
      accept?: () => void
    }
    expect(options.header).toBe('删除回复')
    expect(options.acceptLabel).toBe('删除')
    expect(options.rejectLabel).toBe('取消')

    options.accept?.()
    await flushPromises()
    expect(deleteSolutionReply).toHaveBeenCalledWith(1, 5, 10)
    wrapper.unmount()
  })

  it('deletes own post and navigates back to list', async () => {
    vi.mocked(getSolutionPost).mockResolvedValue(detail())
    vi.mocked(deleteSolutionPost).mockResolvedValue(null)
    const wrapper = mountView()
    await flushPromises()

    await button(wrapper, '删除思路')!.trigger('click')
    expect(confirm.require).toHaveBeenCalled()
    const options = confirm.require.mock.calls[0]![0] as {
      header?: string
      message?: string
      accept?: () => void
    }
    expect(options.header).toBe('删除思路')
    expect(options.message).toContain('如何 AC 这道题')

    options.accept?.()
    await flushPromises()
    expect(deleteSolutionPost).toHaveBeenCalledWith(1, 5)
    expect(routerPush).toHaveBeenCalledWith('/problems/1/solutions')
    wrapper.unmount()
  })

  it('shows 404 error for a missing post', async () => {
    vi.mocked(getSolutionPost).mockRejectedValue(new ApiError(404, 40400, '帖子不存在'))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('帖子不存在')
    expect(button(wrapper, '重试')).toBeTruthy()
    wrapper.unmount()
  })

  it('shows login action to guests while still reading the post', async () => {
    auth.isLoggedIn = false
    vi.mocked(getSolutionPost).mockResolvedValue(detail({ canEdit: false, canDelete: false }))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('如何 AC 这道题')
    expect(wrapper.text()).toContain('帖子正文 markdown')
    expect(wrapper.find('a.login-link').exists()).toBe(true)
    expect(wrapper.find('.markdown-editor-mock').exists()).toBe(false)
    expect(button(wrapper, '编辑思路')).toBeUndefined()
    expect(button(wrapper, '删除思路')).toBeUndefined()
    wrapper.unmount()
  })

  it('shows the like button and count on the post', async () => {
    vi.mocked(getSolutionPost).mockResolvedValue(detail({ likeCount: 4, likedByMe: true }))
    const wrapper = mountView()
    await flushPromises()

    const like = wrapper.find('button.like-button')
    expect(like.exists()).toBe(true)
    expect(like.attributes('aria-pressed')).toBe('true')
    expect(like.text()).toContain('4')
    wrapper.unmount()
  })

  it('redirects guests to login when they click like', async () => {
    auth.isLoggedIn = false
    vi.mocked(getSolutionPost).mockResolvedValue(detail({ canEdit: false, canDelete: false }))
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('button.like-button').trigger('click')
    await flushPromises()

    expect(routerPush).toHaveBeenCalledWith({
      name: 'login',
      query: { redirect: '/problems/1/solutions/5' },
    })
    expect(likeSolutionPost).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('likes via the API and applies the returned count', async () => {
    vi.mocked(getSolutionPost).mockResolvedValue(detail({ likeCount: 2, likedByMe: false }))
    vi.mocked(likeSolutionPost).mockResolvedValue({ liked: true, likeCount: 3 })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('button.like-button').trigger('click')
    await flushPromises()

    expect(likeSolutionPost).toHaveBeenCalledWith(1, 5)
    const like = wrapper.find('button.like-button')
    expect(like.attributes('aria-pressed')).toBe('true')
    expect(like.text()).toContain('3')
    wrapper.unmount()
  })

  it('rolls back the optimistic like on API failure', async () => {
    vi.mocked(getSolutionPost).mockResolvedValue(detail({ likeCount: 2, likedByMe: false }))
    vi.mocked(likeSolutionPost).mockRejectedValue(new ApiError(500, 50000, '点赞失败'))
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('button.like-button').trigger('click')
    await flushPromises()

    const like = wrapper.find('button.like-button')
    expect(like.attributes('aria-pressed')).toBe('false')
    expect(like.text()).toContain('2')
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'error' }))
    wrapper.unmount()
  })
})
