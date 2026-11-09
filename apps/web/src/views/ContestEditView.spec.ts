import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import DatePicker from 'primevue/datepicker'
import InputNumber from 'primevue/inputnumber'
import Select from 'primevue/select'
import ToggleSwitch from 'primevue/toggleswitch'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import { createContest, getContest, updateContest } from '@/api/contests'
import { ApiError } from '@/api/http'
import { listProblems } from '@/api/problems'
import type { ContestDetail, ProblemListItem } from '@/api/types'
import ContestEditView from './ContestEditView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('@/api/contests', () => ({
  getContest: vi.fn(),
  createContest: vi.fn(),
  updateContest: vi.fn(),
}))
vi.mock('@/api/problems', () => ({
  listProblems: vi.fn(),
}))
vi.mock('@/components/MarkdownEditor.vue', () => ({
  default: {
    name: 'MarkdownEditor',
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template:
      '<textarea class="md-editor-stub" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
  },
}))

function iso(offsetMs = 0): string {
  return new Date(Date.now() + offsetMs).toISOString()
}

function contest(overrides: Partial<ContestDetail> = {}): ContestDetail {
  return {
    id: 5,
    title: 'Weekly Contest',
    descriptionMd: 'Original description',
    startTime: iso(3_600_000),
    endTime: iso(7_200_000),
    mode: 'ioi',
    isRated: true,
    isPublic: false,
    disableAi: true,
    registerType: 'password',
    status: 'upcoming',
    problemCount: 2,
    participantCount: 10,
    registered: false,
    createdAt: iso(-86_400_000),
    problems: [
      {
        problemId: 1,
        title: 'Two Sum',
        slug: 'two-sum',
        type: 'traditional',
        displayId: 'A',
        sortOrder: 1,
        score: 100,
        timeLimitMs: 1000,
        memoryLimitMb: 128,
        isPublic: true,
        locked: false,
        submitCount: 12,
        acceptCount: 5,
        myStatus: 'accepted',
        myAttempts: 2,
      },
      {
        problemId: 2,
        title: 'Add Two Numbers',
        slug: 'add-two-numbers',
        type: 'traditional',
        displayId: 'B',
        sortOrder: 2,
        score: 150,
        timeLimitMs: 2000,
        memoryLimitMb: 256,
        isPublic: true,
        locked: false,
        submitCount: 7,
        acceptCount: 3,
        myStatus: '',
        myAttempts: 0,
      },
    ],
    canManage: true,
    serverTime: iso(0),
    ...overrides,
  }
}

function problem(id: number, title: string): ProblemListItem {
  return {
    id,
    title,
    slug: `problem-${id}`,
    type: 'traditional',
    tags: [],
    difficulty: 1,
    timeLimitMs: 1000,
    memoryLimitMb: 128,
    submitCount: 0,
    acceptCount: 0,
    isPublic: true,
    source: '',
    createdAt: '2026-01-01T00:00:00Z',
  }
}

const routes = [
  { path: '/contests', name: 'contests', component: { template: '<div />' } },
  { path: '/contests/new', name: 'contest-create', component: { template: '<div />' } },
  { path: '/contests/:id(\\d+)/edit', name: 'contest-edit', component: { template: '<div />' } },
  { path: '/contests/:id(\\d+)', name: 'contest-detail', component: { template: '<div />' } },
]

async function mountPage(path = '/contests/new') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(ContestEditView, { global: { plugins: [router, PrimeVue] } })
  await flushPromises()
  return { wrapper, router }
}

function button(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper'], label: string) {
  return wrapper.findAll('button').find((item) => item.text().trim() === label)
}

function titleInput(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper']) {
  return wrapper.find('input[maxlength="100"]')
}

function displayIds(wrapper: Awaited<ReturnType<typeof mountPage>>['wrapper']) {
  return wrapper.findAll('.display-input').map((input) => (input.element as HTMLInputElement).value)
}

function rowButtons(row: Element, label: string) {
  return Array.from(row.querySelectorAll('button')).find(
    (item) => item.textContent?.trim() === label,
  )
}

beforeEach(() => {
  toast.add.mockReset()
  vi.mocked(getContest).mockReset()
  vi.mocked(createContest).mockReset()
  vi.mocked(updateContest).mockReset()
  vi.mocked(listProblems).mockReset()

  vi.mocked(getContest).mockResolvedValue(contest())
  vi.mocked(createContest).mockResolvedValue(contest({ id: 99, title: 'New Contest' }))
  vi.mocked(updateContest).mockResolvedValue(contest({ id: 5 }))
  vi.mocked(listProblems).mockResolvedValue({
    total: 2,
    items: [problem(1, 'Two Sum'), problem(2, 'Add Two Numbers')],
  })
})

describe('ContestEditView', () => {
  it('creates a contest from the form', async () => {
    const { wrapper, router } = await mountPage('/contests/new')

    expect(listProblems).toHaveBeenCalledWith({ pageSize: 100 })
    await titleInput(wrapper).setValue('  New Contest  ')
    await wrapper.find('.md-editor-stub').setValue('Hello **world**')
    await wrapper.find('#contest-rated').setValue(true)
    await button(wrapper, '保存')!.trigger('click')
    await flushPromises()

    expect(createContest).toHaveBeenCalledTimes(1)
    const input = vi.mocked(createContest).mock.calls[0]![0]
    expect(input).toMatchObject({
      title: 'New Contest',
      descriptionMd: 'Hello **world**',
      mode: 'icpc',
      isRated: true,
      isPublic: true,
      registerType: 'open',
      password: '',
      inviteCode: '',
      problems: [],
    })
    expect(input.startTime).toMatch(/Z$/)
    expect(input.endTime).toMatch(/Z$/)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '比赛已创建' }),
    )
    expect(router.currentRoute.value.fullPath).toBe('/contests/99')
    wrapper.unmount()
  })

  it('defaults disableAi to true and saves the toggle', async () => {
    const { wrapper } = await mountPage('/contests/new')

    const toggle = wrapper.find('[data-test="contest-disable-ai"] input')
    expect((toggle.element as HTMLInputElement).checked).toBe(true)

    await titleInput(wrapper).setValue('AI Contest')
    await toggle.setValue(false)
    await button(wrapper, '保存')!.trigger('click')
    await flushPromises()

    expect(createContest).toHaveBeenCalledWith(expect.objectContaining({ disableAi: false }))
    wrapper.unmount()
  })

  it('loads and saves disableAi=false', async () => {
    vi.mocked(getContest).mockResolvedValue(contest({ disableAi: false }))
    const { wrapper } = await mountPage('/contests/5/edit')

    const toggle = wrapper.find('[data-test="contest-disable-ai"] input')
    expect((toggle.element as HTMLInputElement).checked).toBe(false)

    await button(wrapper, '保存')!.trigger('click')
    await flushPromises()

    expect(updateContest).toHaveBeenCalledWith(5, expect.objectContaining({ disableAi: false }))
    wrapper.unmount()
  })

  it('loads an existing contest and keeps blank secrets on save', async () => {
    const { wrapper, router } = await mountPage('/contests/5/edit')

    expect(getContest).toHaveBeenCalledWith(5)
    expect(titleInput(wrapper).element).toHaveProperty('value', 'Weekly Contest')
    expect(wrapper.find('.md-editor-stub').element).toHaveProperty('value', 'Original description')
    expect(wrapper.findAll('.p-datatable-tbody > tr')).toHaveLength(2)
    expect(wrapper.find('input[type="password"]').attributes('placeholder')).toBe('留空保留原密码')

    await button(wrapper, '保存')!.trigger('click')
    await flushPromises()

    expect(updateContest).toHaveBeenCalledWith(
      5,
      expect.objectContaining({
        title: 'Weekly Contest',
        mode: 'ioi',
        isRated: true,
        isPublic: false,
        registerType: 'password',
        password: '',
        inviteCode: '',
        problems: [
          { problemId: 1, displayId: 'A', score: 100 },
          { problemId: 2, displayId: 'B', score: 150 },
        ],
      }),
    )
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '比赛已更新' }),
    )
    expect(router.currentRoute.value.fullPath).toBe('/contests/5')
    wrapper.unmount()
  })

  it('blocks saving without a title', async () => {
    const { wrapper } = await mountPage()

    await button(wrapper, '保存')!.trigger('click')
    await flushPromises()

    expect(createContest).not.toHaveBeenCalled()
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'warn', summary: '请填写比赛标题' }),
    )
    wrapper.unmount()
  })

  it('rejects an end time that is not after the start time', async () => {
    const { wrapper } = await mountPage()
    await titleInput(wrapper).setValue('Time Check')

    const pickers = wrapper.findAllComponents(DatePicker)
    pickers[0]!.vm.$emit('update:modelValue', new Date('2026-02-02T00:00:00Z'))
    pickers[1]!.vm.$emit('update:modelValue', new Date('2026-02-01T00:00:00Z'))
    await nextTick()
    await button(wrapper, '保存')!.trigger('click')
    await flushPromises()

    expect(createContest).not.toHaveBeenCalled()
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ summary: '结束时间必须晚于开始时间' }),
    )
    wrapper.unmount()
  })

  it('rejects problem rows without a selected problem', async () => {
    vi.mocked(listProblems).mockResolvedValue({ total: 0, items: [] })
    const { wrapper } = await mountPage()
    await titleInput(wrapper).setValue('No Problems')

    await button(wrapper, '添加题目')!.trigger('click')
    await flushPromises()
    await button(wrapper, '保存')!.trigger('click')
    await flushPromises()

    expect(createContest).not.toHaveBeenCalled()
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ summary: '请为每道题选择题目并填写大写编号' }),
    )
    wrapper.unmount()
  })

  it('adds, reorders and removes problem rows', async () => {
    const { wrapper } = await mountPage()
    await titleInput(wrapper).setValue('Ordered Problems')

    await button(wrapper, '添加题目')!.trigger('click')
    await button(wrapper, '添加题目')!.trigger('click')
    await flushPromises()
    expect(displayIds(wrapper)).toEqual(['A', 'B'])

    let rows = wrapper.findAll('.p-datatable-tbody > tr')
    await rowButtons(rows[0]!.element, '下移')!.click()
    await flushPromises()
    expect(displayIds(wrapper)).toEqual(['B', 'A'])

    rows = wrapper.findAll('.p-datatable-tbody > tr')
    await rowButtons(rows[0]!.element, '删除')!.click()
    await flushPromises()
    expect(displayIds(wrapper)).toEqual(['A'])

    await button(wrapper, '保存')!.trigger('click')
    await flushPromises()
    const input = vi.mocked(createContest).mock.calls[0]![0]
    expect(input.problems).toEqual([{ problemId: 1, displayId: 'A', score: 100 }])
    wrapper.unmount()
  })

  it('searches problems remotely', async () => {
    const { wrapper } = await mountPage()
    await button(wrapper, '添加题目')!.trigger('click')
    await flushPromises()

    const problemSelect = wrapper.findAllComponents(Select)[2]!
    problemSelect.vm.$emit('filter', { originalEvent: new Event('input'), value: 'two' })
    await flushPromises()

    expect(listProblems).toHaveBeenLastCalledWith({ keyword: 'two', pageSize: 20 })
    wrapper.unmount()
  })

  it('freezes schedule, mode and problems after the contest starts', async () => {
    vi.mocked(getContest).mockResolvedValue(
      contest({ startTime: iso(-3_600_000), status: 'running' }),
    )
    const { wrapper } = await mountPage('/contests/5/edit')

    expect(wrapper.findAllComponents(DatePicker)[0]!.props('disabled')).toBe(true)
    expect(wrapper.findAllComponents(DatePicker)[1]!.props('disabled')).toBe(false)
    expect(wrapper.findAllComponents(Select)[0]!.props('disabled')).toBe(true)
    expect(wrapper.findAllComponents(ToggleSwitch)[0]!.props('disabled')).toBe(false)
    expect(
      wrapper.find('[data-test="contest-disable-ai"] input').attributes('disabled'),
    ).toBeUndefined()

    expect(button(wrapper, '添加题目')!.attributes()).toHaveProperty('disabled')
    const row = wrapper.find('.p-datatable-tbody > tr')
    expect(row.find('input').attributes()).toHaveProperty('disabled')
    expect(wrapper.findComponent(InputNumber).props('disabled')).toBe(true)
    expect(rowButtons(row.element, '删除')!.hasAttribute('disabled')).toBe(true)
    expect(wrapper.text()).toContain('比赛已开始，开始时间、赛制与题目列表已冻结。')
    wrapper.unmount()
  })

  it('shows a load error with a way back', async () => {
    vi.mocked(getContest).mockRejectedValue(new ApiError(404, 40400, '比赛不存在'))
    const { wrapper } = await mountPage('/contests/5/edit')

    expect(wrapper.text()).toContain('比赛不存在')
    const link = wrapper.findAll('a').find((item) => item.text().includes('返回比赛列表'))
    expect(link).toBeTruthy()
    expect(link!.attributes('href')).toBe('/contests')
    expect(listProblems).toHaveBeenCalled()
    wrapper.unmount()
  })
})
