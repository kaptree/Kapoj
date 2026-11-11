import { flushPromises, mount } from '@vue/test-utils'
import Avatar from 'primevue/avatar'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import { ApiError } from '@/api/http'
import {
  deleteMyEvaluation,
  deleteUserEvaluation,
  listUserEvaluations,
  saveMyEvaluation,
} from '@/api/social'
import type {
  EvaluationAuthor,
  EvaluationCloudItem,
  EvaluationItem,
  EvaluationPage,
} from '@/api/types'
import { formatDateTime } from '@/utils/format'

import ProfileEvaluationCard from './ProfileEvaluationCard.vue'

const auth = vi.hoisted(() => ({ isLoggedIn: false, user: null as { id: number } | null }))
const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/social', () => ({
  listUserEvaluations: vi.fn(),
  saveMyEvaluation: vi.fn(),
  deleteMyEvaluation: vi.fn(),
  deleteUserEvaluation: vi.fn(),
}))

const TARGET_ID = 7

function author(overrides: Partial<EvaluationAuthor> = {}): EvaluationAuthor {
  return {
    id: 9,
    username: 'bob',
    nickname: 'Bob',
    avatarUrl: '',
    rating: 1500,
    ...overrides,
  }
}

function evaluation(overrides: Partial<EvaluationItem> = {}): EvaluationItem {
  return {
    id: 1,
    content: '大佬太强了',
    author: author(),
    createdAt: '2026-10-01T08:00:00Z',
    updatedAt: '2026-10-02T09:30:00Z',
    canDelete: false,
    ...overrides,
  }
}

function cloudItem(text: string, count: number): EvaluationCloudItem {
  return { text, count }
}

function page(overrides: Partial<EvaluationPage> = {}): EvaluationPage {
  return {
    total: 1,
    items: [evaluation()],
    cloud: [cloudItem('大佬太强了', 1)],
    myEvaluation: null,
    canEvaluate: false,
    ...overrides,
  }
}

const routes = [
  { path: '/users/:id', name: 'user-profile', component: { template: '<div />' } },
  { path: '/login', name: 'login', component: { template: '<div />' } },
]

async function mountCard(props: { targetUserId?: number; targetActive?: boolean } = {}) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/users/7')
  await router.isReady()
  const wrapper = mount(ProfileEvaluationCard, {
    props: { targetUserId: TARGET_ID, ...props },
    global: { plugins: [router, PrimeVue] },
  })
  await flushPromises()
  return wrapper
}

function items(wrapper: Awaited<ReturnType<typeof mountCard>>) {
  return wrapper.findAll('[data-test="evaluation-item"]')
}

function button(wrapper: Awaited<ReturnType<typeof mountCard>>, label: string) {
  return wrapper.findAll('button').find((item) => item.text().trim() === label)
}

beforeEach(() => {
  auth.isLoggedIn = false
  auth.user = null
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(listUserEvaluations).mockReset()
  vi.mocked(listUserEvaluations).mockResolvedValue(page())
  vi.mocked(saveMyEvaluation).mockReset()
  vi.mocked(saveMyEvaluation).mockResolvedValue(evaluation())
  vi.mocked(deleteMyEvaluation).mockReset()
  vi.mocked(deleteMyEvaluation).mockResolvedValue(null)
  vi.mocked(deleteUserEvaluation).mockReset()
  vi.mocked(deleteUserEvaluation).mockResolvedValue(null)
})

describe('ProfileEvaluationCard', () => {
  it('shows the word cloud and list read-only to guests with a login CTA', async () => {
    vi.mocked(listUserEvaluations).mockResolvedValue(
      page({
        total: 2,
        items: [
          evaluation({ id: 1, content: '讲题很清楚' }),
          evaluation({
            id: 2,
            content: '代码风格好',
            author: author({ id: 11, nickname: 'Carol' }),
          }),
        ],
        cloud: [cloudItem('讲题很清楚', 2), cloudItem('代码风格好', 1)],
      }),
    )

    const wrapper = await mountCard()

    expect(listUserEvaluations).toHaveBeenCalledWith(TARGET_ID, 1, 10)
    const cloud = wrapper.get('[data-test="evaluation-cloud"]')
    expect(cloud.text()).toContain('讲题很清楚')
    expect(cloud.text()).toContain('代码风格好')
    expect(items(wrapper)).toHaveLength(2)
    expect(wrapper.text()).toContain('Bob')
    expect(wrapper.text()).toContain('Carol')

    expect(wrapper.find('textarea').exists()).toBe(false)
    const login = wrapper.get('.evaluation-form__login')
    expect(login.text()).toBe('登录后评价')
    expect(login.attributes('href')).toBe('/login')

    wrapper.unmount()
  })

  it('tells owners they cannot evaluate themselves', async () => {
    auth.isLoggedIn = true
    auth.user = { id: TARGET_ID }
    vi.mocked(listUserEvaluations).mockResolvedValue(page({ canEvaluate: false }))

    const wrapper = await mountCard()

    expect(wrapper.text()).toContain('不能给自己评价')
    expect(wrapper.find('textarea').exists()).toBe(false)
    expect(wrapper.find('.evaluation-form__submit').exists()).toBe(false)

    wrapper.unmount()
  })

  it('publishes a trimmed evaluation for another logged-in user and reloads', async () => {
    auth.isLoggedIn = true
    auth.user = { id: 9 }
    vi.mocked(listUserEvaluations).mockResolvedValue(page({ canEvaluate: true }))

    const wrapper = await mountCard()

    const textarea = wrapper.get('textarea')
    expect(textarea.attributes('maxlength')).toBe('40')
    expect(wrapper.get('.evaluation-form__counter').text()).toBe('0/40')

    const submit = wrapper.get('.evaluation-form__submit')
    expect(submit.attributes('disabled')).toBeDefined()
    expect(submit.text()).toContain('发布评价')

    await textarea.setValue('  讲题很清楚  ')
    expect(wrapper.get('.evaluation-form__counter').text()).toBe('9/40')
    expect(wrapper.get('.evaluation-form__submit').attributes('disabled')).toBeUndefined()

    await wrapper.get('.evaluation-form__submit').trigger('click')
    await flushPromises()

    expect(saveMyEvaluation).toHaveBeenCalledWith(TARGET_ID, '讲题很清楚')
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '评价已发布' }),
    )
    expect(listUserEvaluations).toHaveBeenCalledTimes(2)
    expect(listUserEvaluations).toHaveBeenLastCalledWith(TARGET_ID, 1, 10)

    wrapper.unmount()
  })

  it('prefills an existing evaluation and labels the action as an update', async () => {
    auth.isLoggedIn = true
    auth.user = { id: 9 }
    vi.mocked(listUserEvaluations).mockResolvedValue(
      page({
        canEvaluate: true,
        myEvaluation: evaluation({ id: 5, author: author({ id: 9 }), content: '之前的评价' }),
      }),
    )

    const wrapper = await mountCard()

    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('之前的评价')
    expect(wrapper.get('.evaluation-form__counter').text()).toBe('5/40')
    const submit = wrapper.get('.evaluation-form__submit')
    expect(submit.text()).toContain('更新评价')

    await submit.trigger('click')
    await flushPromises()

    expect(saveMyEvaluation).toHaveBeenCalledWith(TARGET_ID, '之前的评价')

    wrapper.unmount()
  })

  it('shows a read-only hint for logged-in viewers when the target is inactive', async () => {
    auth.isLoggedIn = true
    auth.user = { id: 9 }
    vi.mocked(listUserEvaluations).mockResolvedValue(page({ canEvaluate: false }))

    const wrapper = await mountCard({ targetActive: false })

    expect(wrapper.text()).toContain('该用户当前不可评价')
    expect(wrapper.find('textarea').exists()).toBe(false)

    wrapper.unmount()
  })

  it('sizes word cloud tags from their counts and colours them by stable hash', async () => {
    vi.mocked(listUserEvaluations).mockResolvedValue(
      page({
        cloud: [cloudItem('重复', 5), cloudItem('常见', 2), cloudItem('少见', 1)],
      }),
    )

    const wrapper = await mountCard()
    const tags = wrapper.findAll('.evaluation-cloud__tag')

    expect(tags).toHaveLength(3)
    expect((tags[0].element as HTMLElement).style.fontSize).toBe('1.67rem')
    expect((tags[1].element as HTMLElement).style.fontSize).toBe('1.13rem')
    expect((tags[2].element as HTMLElement).style.fontSize).toBe('0.95rem')
    expect(tags[0].attributes('title')).toBe('出现 5 次')

    const colours = tags.map((tag) =>
      tag.classes().find((name) => name.startsWith('evaluation-cloud__tag--')),
    )
    expect(colours.every(Boolean)).toBe(true)
    expect(new Set(colours).size).toBeGreaterThan(1)

    wrapper.unmount()
  })

  it('renders author avatar, nickname link, plain text content and update time', async () => {
    auth.isLoggedIn = true
    auth.user = { id: 9 }
    vi.mocked(listUserEvaluations).mockResolvedValue(
      page({
        canEvaluate: true,
        items: [
          evaluation({
            id: 1,
            content: '<b>纯文本</b>',
            canDelete: true,
            author: author({ id: 9, nickname: 'Bob', avatarUrl: '' }),
          }),
          evaluation({
            id: 2,
            content: '第二条',
            author: author({ id: 11, nickname: 'Carol' }),
          }),
        ],
      }),
    )

    const wrapper = await mountCard()
    const rows = items(wrapper)
    expect(rows).toHaveLength(2)

    const first = rows[0]
    expect(first.get('.evaluation-item__content').text()).toBe('<b>纯文本</b>')
    expect(first.find('b').exists()).toBe(false)
    expect(first.get('.evaluation-item__author').text()).toBe('Bob')
    expect(first.get('.evaluation-item__author').attributes('href')).toBe('/users/9')
    expect(first.get('.evaluation-item__time').text()).toBe(formatDateTime('2026-10-02T09:30:00Z'))
    expect(first.getComponent(Avatar).props('label')).toBe('B')

    expect(first.find('.evaluation-item__delete').exists()).toBe(true)
    expect(rows[1].find('.evaluation-item__delete').exists()).toBe(false)

    wrapper.unmount()
  })

  it('falls back to the author initial when the avatar image fails', async () => {
    vi.mocked(listUserEvaluations).mockResolvedValue(
      page({ items: [evaluation({ author: author({ avatarUrl: '/api/avatars/9.png' }) })] }),
    )

    const wrapper = await mountCard()
    const avatar = wrapper.getComponent(Avatar)
    expect(avatar.props('image')).toBe('/api/avatars/9.png')

    avatar.vm.$emit('error')
    await nextTick()

    expect(wrapper.getComponent(Avatar).props('label')).toBe('B')
    expect(wrapper.getComponent(Avatar).props('image')).toBeFalsy()

    wrapper.unmount()
  })

  it('lets the author delete their own evaluation directly', async () => {
    auth.isLoggedIn = true
    auth.user = { id: 9 }
    vi.mocked(listUserEvaluations).mockResolvedValue(
      page({
        canEvaluate: true,
        items: [evaluation({ id: 3, canDelete: true, author: author({ id: 9 }) })],
      }),
    )

    const wrapper = await mountCard()
    await wrapper.get('.evaluation-item__delete').trigger('click')
    await flushPromises()

    expect(confirm.require).not.toHaveBeenCalled()
    expect(deleteMyEvaluation).toHaveBeenCalledWith(TARGET_ID)
    expect(deleteUserEvaluation).not.toHaveBeenCalled()
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '评价已删除' }),
    )

    wrapper.unmount()
  })

  it('asks the profile owner for confirmation before deleting someone else', async () => {
    auth.isLoggedIn = true
    auth.user = { id: TARGET_ID }
    vi.mocked(listUserEvaluations).mockResolvedValue(
      page({
        canEvaluate: false,
        items: [evaluation({ id: 4, canDelete: true, author: author({ id: 9 }) })],
      }),
    )

    const wrapper = await mountCard()
    await wrapper.get('.evaluation-item__delete').trigger('click')
    await flushPromises()

    expect(confirm.require).toHaveBeenCalledTimes(1)
    expect(deleteUserEvaluation).not.toHaveBeenCalled()

    const options = confirm.require.mock.calls[0][0] as { accept: () => void }
    options.accept()
    await flushPromises()

    expect(deleteUserEvaluation).toHaveBeenCalledWith(TARGET_ID, 4)
    expect(deleteMyEvaluation).not.toHaveBeenCalled()
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '评价已删除' }),
    )
    expect(listUserEvaluations).toHaveBeenCalledTimes(2)

    wrapper.unmount()
  })

  it('does not delete anything when the owner cancels the confirmation', async () => {
    auth.isLoggedIn = true
    auth.user = { id: TARGET_ID }
    vi.mocked(listUserEvaluations).mockResolvedValue(
      page({
        canEvaluate: false,
        items: [evaluation({ id: 4, canDelete: true, author: author({ id: 9 }) })],
      }),
    )

    const wrapper = await mountCard()
    await wrapper.get('.evaluation-item__delete').trigger('click')
    await flushPromises()

    expect(confirm.require).toHaveBeenCalledTimes(1)
    expect(deleteUserEvaluation).not.toHaveBeenCalled()
    expect(deleteMyEvaluation).not.toHaveBeenCalled()
    expect(listUserEvaluations).toHaveBeenCalledTimes(1)

    wrapper.unmount()
  })

  it('appends the next page with load more and hides the button at the end', async () => {
    auth.isLoggedIn = true
    auth.user = { id: 9 }
    vi.mocked(listUserEvaluations)
      .mockResolvedValueOnce(
        page({
          total: 3,
          canEvaluate: true,
          items: [evaluation({ id: 1 }), evaluation({ id: 2 })],
        }),
      )
      .mockResolvedValueOnce(page({ total: 3, canEvaluate: true, items: [evaluation({ id: 3 })] }))

    const wrapper = await mountCard()
    expect(items(wrapper)).toHaveLength(2)
    const more = wrapper.get('.evaluation-list__more')
    expect(more.text()).toContain('加载更多')

    await more.trigger('click')
    await flushPromises()

    expect(listUserEvaluations).toHaveBeenLastCalledWith(TARGET_ID, 2, 10)
    expect(items(wrapper)).toHaveLength(3)
    expect(wrapper.find('.evaluation-list__more').exists()).toBe(false)

    wrapper.unmount()
  })

  it('shows a single empty state in the word cloud area', async () => {
    vi.mocked(listUserEvaluations).mockResolvedValue(
      page({ total: 0, items: [], cloud: [], myEvaluation: null }),
    )

    const wrapper = await mountCard()

    expect(wrapper.get('[data-test="evaluation-cloud"]').text()).toContain('还没有评价')
    expect(items(wrapper)).toHaveLength(0)
    expect(wrapper.text().match(/还没有评价/g)).toHaveLength(1)

    wrapper.unmount()
  })

  it('renders loading while the evaluation request is pending', async () => {
    let resolvePage: (value: EvaluationPage) => void = () => undefined
    vi.mocked(listUserEvaluations).mockReturnValue(
      new Promise<EvaluationPage>((resolve) => {
        resolvePage = resolve
      }),
    )

    const wrapper = await mountCard()
    expect(wrapper.text()).toContain('正在加载评价')

    resolvePage(page())
    await flushPromises()
    expect(wrapper.find('[data-test="evaluation-cloud"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('shows an error with retry without breaking the card', async () => {
    vi.mocked(listUserEvaluations)
      .mockRejectedValueOnce(new ApiError(500, 50000, '服务器错误'))
      .mockResolvedValueOnce(page())

    const wrapper = await mountCard()

    expect(wrapper.text()).toContain('评价加载失败')
    expect(wrapper.text()).toContain('服务器错误')
    const retry = button(wrapper, '重试')
    expect(retry).toBeTruthy()

    await retry!.trigger('click')
    await flushPromises()

    expect(listUserEvaluations).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).not.toContain('服务器错误')
    expect(wrapper.get('[data-test="evaluation-cloud"]').text()).toContain('大佬太强了')

    wrapper.unmount()
  })
})
