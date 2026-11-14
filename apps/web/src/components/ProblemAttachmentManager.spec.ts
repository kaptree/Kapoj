import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  deleteProblemAttachment,
  listProblemAttachments,
  uploadProblemAttachment,
} from '@/api/problems'
import type { ProblemAttachment } from '@/api/types'
import ProblemAttachmentManager from './ProblemAttachmentManager.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/problems', () => ({
  listProblemAttachments: vi.fn(),
  uploadProblemAttachment: vi.fn(),
  deleteProblemAttachment: vi.fn(),
}))

function attachment(overrides: Partial<ProblemAttachment> = {}): ProblemAttachment {
  return {
    id: 1,
    filename: 'sample.zip',
    contentType: 'application/zip',
    size: 2048,
    createdAt: '2026-11-14T00:00:00Z',
    url: '/api/problems/1/attachments/1',
    ...overrides,
  }
}

function mountManager() {
  return mount(ProblemAttachmentManager, {
    props: { problemId: 1 },
    global: { plugins: [PrimeVue] },
  })
}

beforeEach(() => {
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(listProblemAttachments).mockReset()
  vi.mocked(uploadProblemAttachment).mockReset()
  vi.mocked(deleteProblemAttachment).mockReset()
  vi.mocked(listProblemAttachments).mockResolvedValue([attachment()])
})

describe('ProblemAttachmentManager', () => {
  it('lists attachments with size and download link', async () => {
    const wrapper = mountManager()
    await flushPromises()

    expect(listProblemAttachments).toHaveBeenCalledWith(1)
    expect(wrapper.text()).toContain('sample.zip')
    expect(wrapper.text()).toContain('2.0 KiB')
    expect(wrapper.find('a[href="/api/problems/1/attachments/1"]').exists()).toBe(true)
  })

  it('uploads selected files and reloads the list', async () => {
    vi.mocked(uploadProblemAttachment).mockResolvedValue(attachment({ id: 2 }))
    const wrapper = mountManager()
    await flushPromises()

    const input = wrapper.find('input[data-test="attachment-input"]')
    const file = new File(['hello'], 'hello.txt', { type: 'text/plain' })
    Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
    await input.trigger('change')
    await flushPromises()

    expect(uploadProblemAttachment).toHaveBeenCalledWith(1, file)
    expect(listProblemAttachments).toHaveBeenCalledTimes(2)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '附件已上传' }),
    )
  })

  it('deletes after confirmation and removes the row', async () => {
    vi.mocked(deleteProblemAttachment).mockResolvedValue(null)
    const wrapper = mountManager()
    await flushPromises()

    await wrapper.findAll('button').find((b) => b.text().includes('删除'))!.trigger('click')
    expect(confirm.require).toHaveBeenCalledWith(expect.objectContaining({ header: '删除附件' }))
    const options = confirm.require.mock.calls[0]![0] as { accept: () => Promise<void> }
    await options.accept()
    await flushPromises()

    expect(deleteProblemAttachment).toHaveBeenCalledWith(1, 1)
    expect(wrapper.text()).not.toContain('sample.zip')
  })
})
