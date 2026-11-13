import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/http'
import { deleteMyEvaluation, saveMyEvaluation } from '@/api/social'
import type { EvaluationItem } from '@/api/types'

import ProfileEvaluationDialog from './ProfileEvaluationDialog.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('@/api/social', () => ({
  saveMyEvaluation: vi.fn(),
  deleteMyEvaluation: vi.fn(),
}))
vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))

function evaluation(overrides: Partial<EvaluationItem> = {}): EvaluationItem {
  return {
    id: 1,
    content: '旧评价',
    author: { id: 9, username: 'bob', nickname: 'Bob', avatarUrl: '', rating: 1500 },
    createdAt: '2026-10-01T08:00:00Z',
    updatedAt: '2026-10-02T09:30:00Z',
    canDelete: true,
    ...overrides,
  }
}

function mountDialog(props: Record<string, unknown> = {}) {
  return mount(ProfileEvaluationDialog, {
    props: {
      visible: true,
      targetUserId: 7,
      targetActive: true,
      myEvaluation: null,
      canEvaluate: true,
      isSelf: false,
      isLoggedIn: true,
      ...props,
    },
    global: {
      plugins: [PrimeVue],
      stubs: {
        RouterLink: { props: ['to'], template: '<a :href="String(to)"><slot /></a>' },
      },
    },
  })
}

function dialog(): HTMLElement {
  return document.querySelector('.p-dialog') as HTMLElement
}

function dialogButton(label: string): HTMLButtonElement | undefined {
  return Array.from(dialog().querySelectorAll('button')).find(
    (button) => button.textContent?.trim() === label,
  ) as HTMLButtonElement | undefined
}

function setTextarea(value: string) {
  const textarea = dialog().querySelector('textarea') as HTMLTextAreaElement
  Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(
    textarea,
    value,
  )
  textarea.dispatchEvent(new Event('input', { bubbles: true }))
}

beforeEach(() => {
  document.body.innerHTML = ''
  toast.add.mockReset()
  confirm.require.mockReset()
  confirm.require.mockImplementation((options: { accept?: () => void }) => options.accept?.())
  vi.mocked(saveMyEvaluation).mockReset()
  vi.mocked(saveMyEvaluation).mockResolvedValue(evaluation())
  vi.mocked(deleteMyEvaluation).mockReset()
  vi.mocked(deleteMyEvaluation).mockResolvedValue(null)
})

describe('ProfileEvaluationDialog', () => {
  it('shows a login prompt to guests without a textarea', async () => {
    const wrapper = mountDialog({ isLoggedIn: false, canEvaluate: false })
    await flushPromises()

    expect(dialog().textContent).toContain('登录后可以给 TA 写评价')
    expect(dialog().querySelector('textarea')).toBeNull()
    expect(dialog().textContent).toContain('登录后评价')

    wrapper.unmount()
  })

  it('tells the owner they cannot evaluate themselves', async () => {
    const wrapper = mountDialog({ isSelf: true, canEvaluate: false })
    await flushPromises()

    expect(dialog().textContent).toContain('不能给自己评价')
    expect(dialog().querySelector('textarea')).toBeNull()

    wrapper.unmount()
  })

  it('tells the viewer when the target cannot be evaluated', async () => {
    const wrapper = mountDialog({ canEvaluate: false, targetActive: false })
    await flushPromises()

    expect(dialog().textContent).toContain('该用户当前不可评价')
    expect(dialog().querySelector('textarea')).toBeNull()

    wrapper.unmount()
  })

  it('submits a trimmed evaluation and closes', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    const textarea = dialog().querySelector('textarea') as HTMLTextAreaElement
    expect(textarea).not.toBeNull()
    expect(dialog().textContent).toContain('0/40')
    expect(dialogButton('发布评价')?.disabled).toBe(true)

    setTextarea('  讲题很清楚  ')
    await flushPromises()

    expect(dialog().textContent).toContain('5/40')
    expect(dialogButton('发布评价')?.disabled).toBe(false)

    dialogButton('发布评价')!.click()
    await flushPromises()

    expect(saveMyEvaluation).toHaveBeenCalledWith(7, '讲题很清楚')
    expect(wrapper.emitted('saved')).toHaveLength(1)
    expect(wrapper.emitted('update:visible')).toEqual([[false]])
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))

    wrapper.unmount()
  })

  it('normalizes newlines and tabs before submitting', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    setTextarea('a\nb\tc')
    await flushPromises()
    dialogButton('发布评价')!.click()
    await flushPromises()

    expect(saveMyEvaluation).toHaveBeenCalledWith(7, 'a b c')
    wrapper.unmount()
  })

  it('prefills an existing evaluation and deletes it after confirmation', async () => {
    const wrapper = mountDialog({ myEvaluation: evaluation() })
    await flushPromises()

    expect((dialog().querySelector('textarea') as HTMLTextAreaElement).value).toBe('旧评价')
    expect(dialogButton('更新评价')).toBeTruthy()
    expect(dialogButton('删除我的评价')).toBeTruthy()

    dialogButton('删除我的评价')!.click()
    await flushPromises()

    expect(confirm.require).toHaveBeenCalledTimes(1)
    expect(deleteMyEvaluation).toHaveBeenCalledWith(7)
    expect(wrapper.emitted('deleted')).toHaveLength(1)
    expect(wrapper.emitted('update:visible')).toEqual([[false]])
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))

    wrapper.unmount()
  })

  it('keeps the dialog open and the input when saving fails', async () => {
    vi.mocked(saveMyEvaluation).mockRejectedValue(new ApiError(500, 50000, '保存失败'))
    const wrapper = mountDialog()
    await flushPromises()

    setTextarea('不会丢失')
    await flushPromises()
    dialogButton('发布评价')!.click()
    await flushPromises()

    expect(dialog()).not.toBeNull()
    expect(dialog().textContent).toContain('保存失败')
    expect((dialog().querySelector('textarea') as HTMLTextAreaElement).value).toBe('不会丢失')
    expect(wrapper.emitted('saved')).toBeUndefined()
    expect(wrapper.emitted('update:visible')).toBeUndefined()

    wrapper.unmount()
  })
})
