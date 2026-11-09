import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import DataTable from 'primevue/datatable'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import { ApiError } from '@/api/http'
import {
  cloneContest,
  createAnnouncement,
  deleteAnnouncement,
  deleteContest,
  getContest,
  listAnnouncements,
  listContestParticipants,
  registerContest,
  updateAnnouncement,
} from '@/api/contests'
import type { AnnouncementItem, ContestDetail, ContestParticipant } from '@/api/types'
import ContestDetailView from './ContestDetailView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))
const auth = vi.hoisted(() => ({
  isLoggedIn: true,
  can: vi.fn<(code: string) => boolean>(() => false),
}))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/components/MarkdownView.vue', () => ({
  default: {
    name: 'MarkdownView',
    props: ['source', 'theme'],
    template: '<div class="md-stub">{{ source }}</div>',
  },
}))
vi.mock('@/components/ContestRanklist.vue', () => ({
  default: {
    name: 'ContestRanklist',
    props: ['contestId', 'refreshTick'],
    template: '<div class="ranklist-stub">ranklist {{ contestId }} {{ refreshTick }}</div>',
  },
}))
vi.mock('@/api/contests', () => ({
  getContest: vi.fn(),
  listAnnouncements: vi.fn(),
  listContestParticipants: vi.fn(),
  registerContest: vi.fn(),
  cloneContest: vi.fn(),
  deleteContest: vi.fn(),
  createAnnouncement: vi.fn(),
  updateAnnouncement: vi.fn(),
  deleteAnnouncement: vi.fn(),
}))

class FakeEventSource {
  static instances: FakeEventSource[] = []

  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  onmessage: (() => void) | null = null
  close = vi.fn()

  constructor(readonly url: string) {
    FakeEventSource.instances.push(this)
  }
}

function iso(offsetMs = 0): string {
  return new Date(Date.now() + offsetMs).toISOString()
}

function detail(overrides: Partial<ContestDetail> = {}): ContestDetail {
  return {
    id: 5,
    title: 'Weekly Contest',
    descriptionMd: '比赛简介',
    startTime: iso(-60_000),
    endTime: iso(3_600_000),
    mode: 'icpc',
    isRated: true,
    isPublic: true,
    disableAi: true,
    registerType: 'open',
    status: 'running',
    problemCount: 2,
    participantCount: 10,
    registered: true,
    createdAt: iso(-86_400_000),
    problems: [
      {
        problemId: 1,
        title: 'Two Sum',
        slug: 'two-sum',
        displayId: 'A',
        sortOrder: 1,
        score: 100,
        timeLimitMs: 1000,
        memoryLimitMb: 256,
        isPublic: true,
        locked: false,
        submitCount: 0,
        acceptCount: 0,
        myStatus: '',
        myAttempts: 0,
      },
      {
        problemId: 2,
        title: 'Add Two Numbers',
        slug: 'add-two-numbers',
        displayId: 'B',
        sortOrder: 2,
        score: 100,
        timeLimitMs: 1000,
        memoryLimitMb: 256,
        isPublic: true,
        locked: false,
        submitCount: 0,
        acceptCount: 0,
        myStatus: '',
        myAttempts: 0,
      },
    ],
    canManage: false,
    serverTime: iso(0),
    ...overrides,
  }
}

function announcement(overrides: Partial<AnnouncementItem> = {}): AnnouncementItem {
  return {
    id: 1,
    contestId: 5,
    title: '赛前公告',
    contentMd: '请提前十分钟入场。',
    createdAt: iso(-3_600_000),
    ...overrides,
  }
}

function participant(overrides: Partial<ContestParticipant> = {}): ContestParticipant {
  return {
    userId: 7,
    username: 'alice',
    nickname: 'Alice',
    registerTime: iso(-7_200_000),
    ...overrides,
  }
}

function problem(
  overrides: Partial<ContestDetail['problems'][number]> = {},
): ContestDetail['problems'][number] {
  return {
    problemId: 1,
    title: 'Two Sum',
    slug: 'two-sum',
    displayId: 'A',
    sortOrder: 1,
    score: 100,
    timeLimitMs: 1000,
    memoryLimitMb: 256,
    isPublic: true,
    locked: false,
    submitCount: 0,
    acceptCount: 0,
    myStatus: '',
    myAttempts: 0,
    ...overrides,
  }
}

const routes = [
  { path: '/contests', name: 'contests', component: { template: '<div />' } },
  { path: '/contests/new', name: 'contest-create', component: { template: '<div />' } },
  { path: '/contests/:id(\\d+)', name: 'contest-detail', component: { template: '<div />' } },
  { path: '/contests/:id(\\d+)/edit', name: 'contest-edit', component: { template: '<div />' } },
  { path: '/problems/:id(\\d+)', name: 'problem-detail', component: { template: '<div />' } },
  {
    path: '/problems/:id(\\d+)/submit',
    name: 'problem-submit',
    component: { template: '<div />' },
  },
  { path: '/login', name: 'login', component: { template: '<div />' } },
]

async function mountPage(path = '/contests/5') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(ContestDetailView, {
    global: { plugins: [router, PrimeVue] },
  })
  await flushPromises()
  return { wrapper, router }
}

function buttons(container: ParentNode) {
  return Array.from(container.querySelectorAll('button'))
}

function button(container: ParentNode, label: string) {
  return buttons(container).find((item) => item.textContent?.trim() === label)
}

function setField(element: Element, value: string) {
  const proto =
    element instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype
  Object.getOwnPropertyDescriptor(proto, 'value')!.set!.call(element, value)
  element.dispatchEvent(new Event('input', { bubbles: true }))
}

async function openTab(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper'], label: string) {
  const tab = wrapper
    .findAll('[role="tab"]')
    .find((item: { text(): string }) => item.text() === label)
  expect(tab).toBeTruthy()
  await tab!.trigger('click')
  await flushPromises()
}

beforeEach(() => {
  FakeEventSource.instances = []
  vi.stubGlobal('EventSource', FakeEventSource)
  auth.isLoggedIn = true
  auth.can.mockReset()
  auth.can.mockReturnValue(false)
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(getContest).mockReset()
  vi.mocked(listAnnouncements).mockReset()
  vi.mocked(listContestParticipants).mockReset()
  vi.mocked(registerContest).mockReset()
  vi.mocked(cloneContest).mockReset()
  vi.mocked(deleteContest).mockReset()
  vi.mocked(createAnnouncement).mockReset()
  vi.mocked(updateAnnouncement).mockReset()
  vi.mocked(deleteAnnouncement).mockReset()

  vi.mocked(getContest).mockResolvedValue(detail())
  vi.mocked(listAnnouncements).mockResolvedValue([announcement()])
  vi.mocked(listContestParticipants).mockResolvedValue([participant()])
  vi.mocked(registerContest).mockResolvedValue({ registered: true })
  vi.mocked(cloneContest).mockResolvedValue(detail({ id: 99, title: 'Copy' }))
  vi.mocked(deleteContest).mockResolvedValue(null)
  vi.mocked(createAnnouncement).mockResolvedValue(announcement({ id: 2 }))
  vi.mocked(updateAnnouncement).mockResolvedValue(null)
  vi.mocked(deleteAnnouncement).mockResolvedValue(null)
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('ContestDetailView', () => {
  it('loads the contest and renders countdown, problems and announcements', async () => {
    const { wrapper, router } = await mountPage()

    expect(getContest).toHaveBeenCalledWith(5)
    expect(listAnnouncements).toHaveBeenCalledWith(5)
    expect(wrapper.text()).toContain('Weekly Contest')
    expect(wrapper.text()).toContain('进行中')
    expect(wrapper.text()).toContain('ICPC')
    expect(wrapper.text()).toContain('公开报名')
    expect(wrapper.text()).toContain('已报名')
    expect(wrapper.text()).toContain('10 人报名')
    expect(wrapper.text()).toContain('距结束')
    expect(wrapper.text()).toContain('比赛简介')
    expect(wrapper.text()).toContain('A')
    expect(wrapper.text()).toContain('Two Sum')
    expect(wrapper.text()).toContain('100')

    const submit = button(wrapper.element, '提交')
    expect(submit).toBeTruthy()
    submit!.click()
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/problems/1/submit?contestId=5')

    await openTab(wrapper, '公告')
    expect(wrapper.text()).toContain('赛前公告')
    expect(wrapper.text()).toContain('请提前十分钟入场。')

    await openTab(wrapper, '榜单')
    expect(wrapper.text()).toContain('ranklist 5')
    wrapper.unmount()
  })

  it('guides guests to log in and shows the upcoming empty state', async () => {
    auth.isLoggedIn = false
    vi.mocked(getContest).mockResolvedValue(
      detail({ status: 'upcoming', registered: false, problems: [] }),
    )
    const { wrapper, router } = await mountPage()

    expect(wrapper.text()).toContain('登录后报名')
    expect(wrapper.text()).toContain('题目将在比赛开始后公布。')
    const link = wrapper.findAll('a').find((item) => item.text().includes('登录后报名'))
    expect(link).toBeTruthy()
    expect(link!.attributes('href')).toBe(
      router.resolve({ name: 'login', query: { redirect: '/contests/5' } }).href,
    )
    wrapper.unmount()
  })

  it('registers directly for open contests', async () => {
    vi.mocked(getContest).mockResolvedValue(detail({ registered: false }))
    const { wrapper } = await mountPage()

    await button(wrapper.element, '报名')!.click()
    await flushPromises()

    expect(registerContest).toHaveBeenCalledWith(5, {})
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '报名成功' }),
    )
    expect(getContest).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('registers for password contests through a dialog', async () => {
    vi.mocked(getContest).mockResolvedValue(detail({ registered: false, registerType: 'password' }))
    const { wrapper } = await mountPage()

    await button(wrapper.element, '报名')!.click()
    await flushPromises()
    const dialog = document.querySelector('.p-dialog')
    expect(dialog?.textContent).toContain('报名比赛')
    setField(dialog!.querySelector('input')!, 'secret')
    await nextTick()
    ;(button(dialog!, '报名') as HTMLElement).click()
    await flushPromises()

    expect(registerContest).toHaveBeenCalledWith(5, {
      password: 'secret',
      inviteCode: '',
    })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '报名成功' }),
    )
    wrapper.unmount()
  })

  it('participant.read/edit/clone/delete/announcement buttons use separate permissions', async () => {
    auth.can.mockImplementation((code: string) => code === 'contest.update')
    let { wrapper } = await mountPage()
    expect(button(wrapper.element, '编辑')).toBeTruthy()
    expect(button(wrapper.element, '报名用户')).toBeUndefined()
    expect(button(wrapper.element, '克隆')).toBeUndefined()
    expect(button(wrapper.element, '删除')).toBeUndefined()
    wrapper.unmount()

    auth.can.mockImplementation((code: string) => code === 'contest.participant.read')
    ;({ wrapper } = await mountPage())
    expect(button(wrapper.element, '报名用户')).toBeTruthy()
    expect(button(wrapper.element, '编辑')).toBeUndefined()
    wrapper.unmount()

    auth.can.mockImplementation((code: string) => code === 'contest.delete')
    ;({ wrapper } = await mountPage())
    expect(button(wrapper.element, '删除')).toBeTruthy()
    expect(button(wrapper.element, '编辑')).toBeUndefined()
    wrapper.unmount()

    auth.can.mockImplementation((code: string) => code === 'contest.announcement.manage')
    ;({ wrapper } = await mountPage())
    expect(button(wrapper.element, '编辑')).toBeUndefined()
    await openTab(wrapper, '公告')
    expect(button(wrapper.element, '发布公告')).toBeTruthy()
    wrapper.unmount()
  })

  it('contest.update permits unregistered test submit UI; view_hidden does not', async () => {
    vi.mocked(getContest).mockResolvedValue(detail({ registered: false, status: 'running' }))

    auth.can.mockImplementation((code: string) => code === 'contest.view_hidden')
    let { wrapper } = await mountPage()
    expect(button(wrapper.element, '提交')).toBeUndefined()
    expect(button(wrapper.element, '报名')).toBeTruthy()
    wrapper.unmount()

    auth.can.mockImplementation((code: string) => code === 'contest.update')
    ;({ wrapper } = await mountPage())
    expect(button(wrapper.element, '提交')).toBeTruthy()
    expect(button(wrapper.element, '报名')).toBeUndefined()
    wrapper.unmount()
  })

  it('supports participants, editing and cloning for managers', async () => {
    auth.can.mockReturnValue(true)
    vi.mocked(getContest).mockResolvedValue(detail({ registered: false }))
    const { wrapper, router } = await mountPage()

    await button(wrapper.element, '报名用户')!.click()
    await flushPromises()
    expect(listContestParticipants).toHaveBeenCalledWith(5)
    expect(document.body.textContent).toContain('Alice')
    ;(button(document.querySelector('.p-dialog')!, '关闭') as HTMLElement).click()
    await flushPromises()

    await button(wrapper.element, '编辑')!.click()
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/contests/5/edit')

    await button(wrapper.element, '克隆')!.click()
    await flushPromises()
    const dialog = document.querySelector('.p-dialog')
    const input = dialog!.querySelector('input') as HTMLInputElement
    expect(input.value).toBe('Weekly Contest（副本）')
    setField(input, 'Copy Contest')
    await nextTick()
    ;(dialog!.querySelector('.clone-submit') as HTMLElement).click()
    await flushPromises()

    expect(cloneContest).toHaveBeenCalledWith(5, { title: 'Copy Contest' })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '比赛已克隆' }),
    )
    expect(router.currentRoute.value.fullPath).toBe('/contests/99/edit')
    wrapper.unmount()
  })

  it('confirms deletion with Chinese labels and returns to the list', async () => {
    auth.can.mockImplementation((code: string) => code === 'contest.delete')
    vi.mocked(getContest).mockResolvedValue(detail())
    const { wrapper, router } = await mountPage()

    await button(wrapper.element, '删除')!.click()
    expect(confirm.require).toHaveBeenCalled()
    const options = confirm.require.mock.calls[0]![0] as {
      header?: string
      message?: string
      acceptLabel?: string
      rejectLabel?: string
      accept?: () => void
    }
    expect(options.header).toBe('删除比赛')
    expect(options.message).toBe('确定删除这场比赛吗？')
    expect(options.acceptLabel).toBe('删除')
    expect(options.rejectLabel).toBe('取消')

    options.accept?.()
    await flushPromises()
    expect(deleteContest).toHaveBeenCalledWith(5)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '已删除' }),
    )
    expect(router.currentRoute.value.fullPath).toBe('/contests')
    wrapper.unmount()
  })

  it('creates, edits and deletes announcements', async () => {
    auth.can.mockImplementation((code: string) => code === 'contest.announcement.manage')
    vi.mocked(getContest).mockResolvedValue(detail())
    const { wrapper } = await mountPage()
    await openTab(wrapper, '公告')

    await button(wrapper.element, '发布公告')!.click()
    await flushPromises()
    let dialog = document.querySelector('.p-dialog')!
    expect(dialog.textContent).toContain('发布公告')
    setField(dialog.querySelector('input')!, '新公告')
    setField(dialog.querySelector('textarea')!, '公告内容')
    await nextTick()
    ;(dialog.querySelector('.announcement-submit') as HTMLElement).click()
    await flushPromises()
    expect(createAnnouncement).toHaveBeenCalledWith(5, {
      title: '新公告',
      contentMd: '公告内容',
    })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '公告已保存' }),
    )
    expect(listAnnouncements).toHaveBeenCalledTimes(2)

    const announcementRow = wrapper.find('.announcement')
    expect(announcementRow.exists()).toBe(true)
    await button(announcementRow.element, '编辑')!.click()
    await flushPromises()
    dialog = document.querySelector('.p-dialog')!
    expect(dialog.textContent).toContain('编辑公告')
    setField(dialog.querySelector('input')!, '改后的公告')
    await nextTick()
    ;(dialog.querySelector('.announcement-submit') as HTMLElement).click()
    await flushPromises()
    expect(updateAnnouncement).toHaveBeenCalledWith(5, 1, {
      title: '改后的公告',
      contentMd: '请提前十分钟入场。',
    })

    await button(announcementRow.element, '删除')!.click()
    const options = confirm.require.mock.calls[0]![0] as {
      header?: string
      message?: string
      accept?: () => void
    }
    expect(options.header).toBe('删除公告')
    expect(options.message).toBe('删除公告「赛前公告」？')
    options.accept?.()
    await flushPromises()
    expect(deleteAnnouncement).toHaveBeenCalledWith(5, 1)
    wrapper.unmount()
  })

  it('refreshes on SSE events and cleans up on unmount', async () => {
    vi.useFakeTimers()
    const { wrapper } = await mountPage()
    const source = FakeEventSource.instances[0]!
    expect(source.url).toBe('/api/submissions/stream')
    expect(getContest).toHaveBeenCalledTimes(1)

    source.onmessage?.()
    vi.advanceTimersByTime(300)
    await flushPromises()
    expect(getContest).toHaveBeenCalledTimes(2)

    wrapper.unmount()
    expect(source.close).toHaveBeenCalled()
  })

  it('shows limits and post-contest stats with problem-bank links', async () => {
    vi.mocked(getContest).mockResolvedValue(
      detail({
        status: 'ended',
        problems: [
          problem({
            submitCount: 12,
            acceptCount: 3,
            myStatus: 'accepted',
            myAttempts: 2,
          }),
        ],
      }),
    )
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('1000 ms')
    expect(wrapper.text()).toContain('256 MB')
    expect(wrapper.text()).toContain('提交 12 / 通过 3')
    expect(wrapper.text()).toContain('已通过 · 2 次')
    expect(wrapper.find('a.problem-link').attributes('href')).toBe('/problems/1')
    expect(wrapper.find('[data-test="problem-bank-link"]').attributes('href')).toContain(
      '/problems/1',
    )
    wrapper.unmount()
  })

  it('hides post-contest columns and keeps contest links while running', async () => {
    vi.mocked(getContest).mockResolvedValue(detail({ status: 'running' }))
    const { wrapper } = await mountPage()

    expect(wrapper.text()).not.toContain('本场')
    expect(wrapper.text()).not.toContain('我的')
    expect(wrapper.find('a.problem-link').attributes('href')).toContain('contestId=5')
    expect(wrapper.find('[data-test="problem-bank-link"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('disables problem-bank links for hidden or locked problems after the contest', async () => {
    vi.mocked(getContest).mockResolvedValue(
      detail({
        status: 'ended',
        problems: [
          problem({ problemId: 1, isPublic: false }),
          problem({ problemId: 2, title: 'Locked', displayId: 'B', sortOrder: 2, locked: true }),
        ],
      }),
    )
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('未公开')
    expect(wrapper.text()).toContain('比赛中')
    expect(wrapper.find('[data-test="problem-bank-link"]').exists()).toBe(false)
    const hrefs = wrapper.findAll('a').map((link) => link.attributes('href') ?? '')
    expect(hrefs.some((href) => href.includes('/problems/1'))).toBe(false)
    expect(hrefs.some((href) => href.includes('/problems/2'))).toBe(false)
    wrapper.unmount()
  })

  it('shows a not-found state with a way back', async () => {
    vi.mocked(getContest).mockRejectedValue(new ApiError(404, 40400, '比赛不存在'))
    const { wrapper } = await mountPage()

    expect(wrapper.text()).toContain('比赛不存在或无权访问')
    const link = wrapper.findAll('a').find((item) => item.text().includes('返回比赛列表'))
    expect(link).toBeTruthy()
    expect(link!.attributes('href')).toBe('/contests')
    wrapper.unmount()
  })

  it('renders problem rows through the shared data table', async () => {
    const { wrapper } = await mountPage()
    expect(wrapper.findComponent(DataTable).exists()).toBe(true)
    wrapper.unmount()
  })
})
