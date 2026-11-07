import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import DatePicker from 'primevue/datepicker'
import InputNumber from 'primevue/inputnumber'
import MultiSelect from 'primevue/multiselect'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import { createTask, deleteTask, listTasks, updateTask } from '@/api/gamification'
import { getProblem, listProblems } from '@/api/problems'
import type { ProblemListItem, TaskItem } from '@/api/types'
import TaskAdminPanel from './TaskAdminPanel.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/gamification', () => ({
  listTasks: vi.fn(),
  createTask: vi.fn(),
  updateTask: vi.fn(),
  deleteTask: vi.fn(),
}))
vi.mock('@/api/problems', () => ({
  listProblems: vi.fn(),
  getProblem: vi.fn(),
}))
vi.mock('@/components/MarkdownEditor.vue', () => ({
  default: {
    name: 'MarkdownEditor',
    props: ['modelValue', 'height', 'theme'],
    emits: ['update:modelValue'],
    template:
      '<textarea class="md-editor-stub" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
  },
}))

function task(overrides: Partial<TaskItem> = {}): TaskItem {
  return {
    id: 1,
    title: '新手任务',
    descriptionMd: '完成 5 道题',
    problemIds: [1, 2],
    rewardCoins: 100,
    startTime: '2026-01-01T00:00:00Z',
    endTime: '2026-01-02T00:00:00Z',
    isActive: true,
    status: 'active',
    claimedAt: null,
    completedAt: null,
    progress: 0,
    problemCount: 2,
    hasRecords: false,
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

async function mountPanel() {
  const wrapper = mount(TaskAdminPanel, {
    global: { plugins: [PrimeVue] },
  })
  await flushPromises()
  return wrapper
}

function actionButton(wrapper: Awaited<ReturnType<typeof mountPanel>>, label: string, index = 0) {
  return wrapper.findAll('button').filter((item) => item.text().trim() === label)[index]
}

function dialog(): HTMLElement {
  return document.querySelector('.p-dialog') as HTMLElement
}

function dialogButton(label: string) {
  return Array.from(dialog().querySelectorAll('button')).find(
    (item) => item.textContent?.trim() === label,
  )
}

function setField(element: Element, value: string) {
  const proto =
    element instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype
  Object.getOwnPropertyDescriptor(proto, 'value')!.set!.call(element, value)
  element.dispatchEvent(new Event('input', { bubbles: true }))
}

beforeEach(() => {
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(listTasks).mockReset()
  vi.mocked(createTask).mockReset()
  vi.mocked(updateTask).mockReset()
  vi.mocked(deleteTask).mockReset()
  vi.mocked(listProblems).mockReset()
  vi.mocked(getProblem).mockReset()

  vi.mocked(listTasks).mockResolvedValue([
    task(),
    task({
      id: 2,
      title: '已冻结任务',
      status: 'claimed',
      claimedAt: '2026-01-01T01:00:00Z',
      hasRecords: true,
    }),
  ])
  vi.mocked(listProblems).mockResolvedValue({
    total: 2,
    items: [problem(1, 'Two Sum'), problem(2, 'Add Two Numbers')],
  })
  vi.mocked(getProblem).mockImplementation(async (id) => ({
    ...problem(Number(id), `Problem ${id}`),
    statementMd: '',
    inputFormatMd: '',
    outputFormatMd: '',
    samples: [],
    hintMd: '',
    solutionPriceCoins: 0,
  }))
  vi.mocked(createTask).mockResolvedValue(task({ id: 99 }))
  vi.mocked(updateTask).mockResolvedValue(task())
  vi.mocked(deleteTask).mockResolvedValue(null)
})

describe('TaskAdminPanel', () => {
  it('renders tasks with times, rewards, status and records', async () => {
    const wrapper = await mountPanel()

    expect(listTasks).toHaveBeenCalledWith({ includeInactive: true })
    expect(wrapper.text()).toContain('共 2 个任务')
    expect(wrapper.text()).toContain('新手任务')
    expect(wrapper.text()).toContain('已冻结任务')
    expect(wrapper.text()).toContain('开始 2026/1/1 08:00:00')
    expect(wrapper.text()).toContain('截止 2026/1/2 08:00:00')
    expect(wrapper.text()).toContain('金币 100')
    expect(wrapper.text()).toContain('可领取')
    expect(wrapper.text()).toContain('已有领取')
    expect(wrapper.findAll('.p-datatable-tbody > tr')).toHaveLength(2)
    wrapper.unmount()
  })

  it('creates a task with remotely searched problems', async () => {
    const wrapper = await mountPanel()
    await actionButton(wrapper, '新建任务')!.trigger('click')
    await flushPromises()
    expect(listProblems).toHaveBeenCalledWith({ keyword: undefined, pageSize: 20 })

    setField(dialog().querySelector('.task-title-input')!, '新任务')
    const multi = wrapper.findComponent(MultiSelect)
    multi.vm.$emit('update:modelValue', [1, 2])
    await nextTick()
    multi.vm.$emit('filter', { value: 'two' })
    await flushPromises()
    expect(listProblems).toHaveBeenLastCalledWith({ keyword: 'two', pageSize: 20 })

    wrapper
      .findAllComponents(InputNumber)
      .find((item) => item.props('inputId') === 'task-reward')!
      .vm.$emit('update:modelValue', 50)
    const pickers = wrapper.findAllComponents(DatePicker)
    pickers[0]!.vm.$emit('update:modelValue', new Date('2026-02-01T00:00:00Z'))
    pickers[1]!.vm.$emit('update:modelValue', new Date('2026-02-02T00:00:00Z'))
    await nextTick()
    dialogButton('保存')!.click()
    await flushPromises()

    expect(createTask).toHaveBeenCalledWith({
      title: '新任务',
      descriptionMd: '',
      problemIds: [1, 2],
      rewardCoins: 50,
      startTime: '2026-02-01T00:00:00.000Z',
      endTime: '2026-02-02T00:00:00.000Z',
      isActive: true,
    })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '任务已创建' }),
    )
    wrapper.unmount()
  })

  it('freezes problem and reward fields when the task has records', async () => {
    const wrapper = await mountPanel()
    await actionButton(wrapper, '编辑', 1)!.trigger('click')
    await flushPromises()

    expect(wrapper.findComponent(MultiSelect).props('disabled')).toBe(true)
    expect(
      wrapper
        .findAllComponents(InputNumber)
        .find((item) => item.props('inputId') === 'task-reward')!
        .props('disabled'),
    ).toBe(true)
    expect(dialog().textContent).toContain('已有领取记录，题目与奖励已冻结')
    wrapper.unmount()
  })

  it('validates required fields', async () => {
    const wrapper = await mountPanel()
    await actionButton(wrapper, '新建任务')!.trigger('click')
    await flushPromises()
    dialogButton('保存')!.click()
    await flushPromises()

    expect(createTask).not.toHaveBeenCalled()
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'warn', summary: '请填写任务标题' }),
    )
    wrapper.unmount()
  })

  it('confirms deletion with Chinese labels', async () => {
    const wrapper = await mountPanel()

    await actionButton(wrapper, '删除')!.trigger('click')
    const options = confirm.require.mock.calls[0]![0] as {
      header?: string
      message?: string
      acceptLabel?: string
      rejectLabel?: string
      accept?: () => void
    }
    expect(options.header).toBe('删除任务')
    expect(options.message).toContain('新手任务')
    expect(options.acceptLabel).toBe('删除')
    expect(options.rejectLabel).toBe('取消')

    options.accept?.()
    await flushPromises()
    expect(deleteTask).toHaveBeenCalledWith(1)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '任务已删除' }),
    )
    wrapper.unmount()
  })
})
