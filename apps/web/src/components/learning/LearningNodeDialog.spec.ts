import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import InputNumber from 'primevue/inputnumber'
import Select from 'primevue/select'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import { ApiError } from '@/api/http'
import { createLearningNode, updateLearningNode } from '@/api/learning'
import { listProblems } from '@/api/problems'
import type { LearningPathNodeItem, ProblemListItem } from '@/api/types'
import LearningNodeDialog from './LearningNodeDialog.vue'

vi.mock('@/api/learning', () => ({
  createLearningNode: vi.fn(),
  updateLearningNode: vi.fn(),
}))
vi.mock('@/api/problems', () => ({
  listProblems: vi.fn(),
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

function summary(problemId: number, title = `题目 ${problemId}`) {
  return { problemId, title, slug: `problem-${problemId}`, difficulty: 1 }
}

function node(overrides: Partial<LearningPathNodeItem> = {}): LearningPathNodeItem {
  return {
    id: 1,
    parentId: null,
    title: '基础语法',
    descriptionMd: '',
    problemIds: [],
    problems: [],
    sortOrder: 0,
    requiredCount: 1,
    solvedCount: 0,
    status: 'locked',
    children: [],
    ...overrides,
  }
}

function mountDialog(props: Record<string, unknown> = {}) {
  return mount(LearningNodeDialog, {
    props: {
      visible: true,
      pathId: 5,
      nodes: [],
      editing: null,
      parentId: null,
      ...props,
    },
    global: { plugins: [PrimeVue] },
  })
}

function dialog(): HTMLElement {
  return document.querySelector('.p-dialog') as HTMLElement
}

function dialogButton(label: string) {
  return Array.from(dialog().querySelectorAll('button')).find(
    (item) => item.textContent?.trim() === label,
  )
}

function setField(element: Element | null, value: string) {
  const el = element as HTMLInputElement | HTMLTextAreaElement
  const proto =
    el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype
  Object.getOwnPropertyDescriptor(proto, 'value')!.set!.call(el, value)
  el.dispatchEvent(new Event('input', { bubbles: true }))
}

function titleInput(): HTMLInputElement {
  return dialog().querySelector('.node-title') as HTMLInputElement
}

beforeEach(() => {
  document.body.innerHTML = ''
  vi.mocked(createLearningNode).mockReset()
  vi.mocked(updateLearningNode).mockReset()
  vi.mocked(listProblems).mockReset()
  vi.mocked(listProblems).mockResolvedValue({
    total: 2,
    items: [problem(101, 'A+B'), problem(102, 'C+D')],
  })
})

describe('LearningNodeDialog', () => {
  it('creates a node with the submitted LearningNodeInput', async () => {
    vi.mocked(createLearningNode).mockResolvedValue(node({ id: 10 }))
    const wrapper = mountDialog()
    await flushPromises()

    setField(titleInput(), '  基础语法  ')
    setField(dialog().querySelector('.md-editor-stub'), '描述文本')

    const selects = wrapper.findAllComponents(Select)
    selects[1]!.vm.$emit('change', { originalEvent: new Event('change'), value: 101 })
    await nextTick()
    selects[1]!.vm.$emit('change', { originalEvent: new Event('change'), value: 102 })
    await nextTick()

    const numbers = wrapper.findAllComponents(InputNumber)
    numbers[0]!.vm.$emit('update:modelValue', 2)
    numbers[1]!.vm.$emit('update:modelValue', 1)
    await nextTick()

    dialogButton('保存')!.click()
    await flushPromises()

    expect(createLearningNode).toHaveBeenCalledTimes(1)
    expect(createLearningNode).toHaveBeenCalledWith(5, {
      title: '基础语法',
      descriptionMd: '描述文本',
      parentId: null,
      problemIds: [101, 102],
      sortOrder: 1,
      requiredCount: 2,
    })
    expect(wrapper.emitted('saved')).toBeTruthy()
    expect(wrapper.emitted('update:visible')?.at(-1)).toEqual([false])
    wrapper.unmount()
  })

  it('preloads problems and searches remotely via listProblems', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    expect(listProblems).toHaveBeenCalledWith({ pageSize: 100 })

    const selects = wrapper.findAllComponents(Select)
    selects[1]!.vm.$emit('filter', { originalEvent: new Event('input'), value: 'two' })
    await flushPromises()

    expect(listProblems).toHaveBeenLastCalledWith({ keyword: 'two', pageSize: 20 })
    wrapper.unmount()
  })

  it('rejects an empty problem list without submitting', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    setField(titleInput(), '节点标题')
    dialogButton('保存')!.click()
    await flushPromises()

    expect(createLearningNode).not.toHaveBeenCalled()
    expect(dialog().textContent).toContain('请至少添加一道题目')
    wrapper.unmount()
  })

  it('rejects more than 50 problems without submitting', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    setField(titleInput(), '节点标题')

    const selects = wrapper.findAllComponents(Select)
    for (let i = 1; i <= 51; i++) {
      selects[1]!.vm.$emit('change', { originalEvent: new Event('change'), value: i })
    }
    await nextTick()

    dialogButton('保存')!.click()
    await flushPromises()

    expect(createLearningNode).not.toHaveBeenCalled()
    expect(dialog().textContent).toContain('题目不能超过 50 道')
    wrapper.unmount()
  })

  it('rejects duplicate problems without submitting', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    setField(titleInput(), '节点标题')

    const selects = wrapper.findAllComponents(Select)
    selects[1]!.vm.$emit('change', { originalEvent: new Event('change'), value: 101 })
    selects[1]!.vm.$emit('change', { originalEvent: new Event('change'), value: 101 })
    await nextTick()

    dialogButton('保存')!.click()
    await flushPromises()

    expect(createLearningNode).not.toHaveBeenCalled()
    expect(dialog().textContent).toContain('题目不能重复')
    wrapper.unmount()
  })

  it('rejects an out-of-range requiredCount without submitting', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    setField(titleInput(), '节点标题')

    const selects = wrapper.findAllComponents(Select)
    selects[1]!.vm.$emit('change', { originalEvent: new Event('change'), value: 101 })
    await nextTick()

    const numbers = wrapper.findAllComponents(InputNumber)
    numbers[0]!.vm.$emit('update:modelValue', 5)
    await nextTick()

    dialogButton('保存')!.click()
    await flushPromises()

    expect(createLearningNode).not.toHaveBeenCalled()
    expect(dialog().textContent).toContain('完成题数需在 1–1 之间')
    wrapper.unmount()
  })

  it('excludes self and descendants from parent options when editing', async () => {
    const grandchild = node({ id: 3, parentId: 2, title: '孙子' })
    const child = node({ id: 2, parentId: 1, title: '子节点', children: [grandchild] })
    const sibling = node({ id: 4, parentId: null, title: '兄弟' })
    const self = node({ id: 1, parentId: null, title: '根节点', children: [child] })

    const wrapper = mountDialog({ nodes: [self, sibling], editing: self })
    await flushPromises()

    const parentSelect = wrapper.findAllComponents(Select)[0]!
    const options = parentSelect.props('options') as Array<{ value: number | null; label: string }>
    const values = options.map((option) => option.value)

    expect(values).toContain(null)
    expect(values).toContain(4)
    expect(values).not.toContain(1)
    expect(values).not.toContain(2)
    expect(values).not.toContain(3)
    wrapper.unmount()
  })

  it('updates an existing node with its id', async () => {
    vi.mocked(updateLearningNode).mockResolvedValue(node({ id: 7 }))
    const existing = node({
      id: 7,
      title: '旧标题',
      descriptionMd: '旧描述',
      parentId: null,
      problemIds: [101],
      problems: [summary(101, 'A+B')],
      sortOrder: 3,
      requiredCount: 1,
    })
    const wrapper = mountDialog({ nodes: [existing], editing: existing })
    await flushPromises()

    expect(titleInput().value).toBe('旧标题')
    setField(titleInput(), '新标题')
    dialogButton('保存')!.click()
    await flushPromises()

    expect(updateLearningNode).toHaveBeenCalledWith(5, 7, {
      title: '新标题',
      descriptionMd: '旧描述',
      parentId: null,
      problemIds: [101],
      sortOrder: 3,
      requiredCount: 1,
    })
    expect(wrapper.emitted('saved')).toBeTruthy()
    wrapper.unmount()
  })

  it('shows the backend Chinese error when saving fails', async () => {
    vi.mocked(createLearningNode).mockRejectedValue(
      new ApiError(400, 40001, '父节点不能选择自身或其子节点'),
    )
    const wrapper = mountDialog()
    await flushPromises()
    setField(titleInput(), '节点标题')

    const selects = wrapper.findAllComponents(Select)
    selects[1]!.vm.$emit('change', { originalEvent: new Event('change'), value: 101 })
    await nextTick()

    dialogButton('保存')!.click()
    await flushPromises()

    expect(dialog().textContent).toContain('父节点不能选择自身或其子节点')
    expect(wrapper.emitted('saved')).toBeFalsy()
    expect(wrapper.emitted('update:visible')).toBeFalsy()
    wrapper.unmount()
  })
})
