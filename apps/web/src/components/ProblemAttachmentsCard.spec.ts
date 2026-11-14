import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { listProblemAttachments } from '@/api/problems'
import type { ProblemAttachment } from '@/api/types'
import ProblemAttachmentsCard from './ProblemAttachmentsCard.vue'

vi.mock('@/api/problems', () => ({
  listProblemAttachments: vi.fn(),
}))

function attachment(): ProblemAttachment {
  return {
    id: 2,
    filename: 'statement.pdf',
    contentType: 'application/pdf',
    size: 1024,
    createdAt: '2026-11-14T00:00:00Z',
    url: '/api/problems/1/attachments/2',
  }
}

beforeEach(() => {
  vi.mocked(listProblemAttachments).mockReset()
})

describe('ProblemAttachmentsCard', () => {
  it('renders download links and sizes', async () => {
    vi.mocked(listProblemAttachments).mockResolvedValue([attachment()])
    const wrapper = mount(ProblemAttachmentsCard, { props: { problemId: 1 } })
    await flushPromises()

    expect(wrapper.text()).toContain('题目附件')
    expect(wrapper.text()).toContain('statement.pdf')
    expect(wrapper.text()).toContain('1.0 KiB')
    expect(wrapper.find('a[href="/api/problems/1/attachments/2"]').attributes('download')).toBe('')
  })

  it('renders nothing when the problem has no attachments', async () => {
    vi.mocked(listProblemAttachments).mockResolvedValue([])
    const wrapper = mount(ProblemAttachmentsCard, { props: { problemId: 1 } })
    await flushPromises()

    expect(wrapper.find('.attachment-card').exists()).toBe(false)
  })
})
