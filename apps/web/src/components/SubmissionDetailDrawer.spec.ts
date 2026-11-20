import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { getSubmission } from '@/api/submissions'
import type { SubmissionDetail } from '@/api/types'
import SubmissionDetailDrawer from './SubmissionDetailDrawer.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('@/api/submissions', () => ({
  getSubmission: vi.fn(),
  rejudgeSubmission: vi.fn(),
}))

function detail(overrides: Partial<SubmissionDetail> = {}): SubmissionDetail {
  return {
    id: 1,
    userId: 7,
    username: 'alice',
    nickname: 'Alice',
    agentId: null,
    agentName: '',
    equippedBadges: [],
    equippedTitle: null,
    problemId: 3,
    problemTitle: 'Two Sum',
    problemSlug: 'two-sum',
    language: 'cpp',
    status: 'accepted',
    timeUsedMs: 100,
    cpuUsedMs: 80,
    memoryUsedKb: 1024,
    score: 100,
    codeLength: 13,
    createdAt: '2026-01-01T00:00:00Z',
    code: 'int main() {}',
    codeHidden: false,
    answerHidden: false,
    judgeResult: [],
    ...overrides,
  }
}

const routes = [
  { path: '/', component: { template: '<div />' } },
  { path: '/users/:id', name: 'user-profile', component: { template: '<div />' } },
  { path: '/problems/:id', name: 'problem-detail', component: { template: '<div />' } },
]

async function mountDrawer(props: Record<string, unknown> = {}) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/')
  await router.isReady()
  const wrapper = mount(SubmissionDetailDrawer, {
    attachTo: document.body,
    props: { visible: true, submissionId: 1, canRejudge: false, ...props },
    global: { plugins: [router, PrimeVue] },
  })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  document.body.innerHTML = ''
  toast.add.mockReset()
  vi.mocked(getSubmission).mockReset()
  vi.mocked(getSubmission).mockResolvedValue(detail())
})

describe('SubmissionDetailDrawer', () => {
  it('loads the submission and renders code', async () => {
    const wrapper = await mountDrawer()

    expect(getSubmission).toHaveBeenCalledWith(1)
    expect(document.body.textContent).toContain('int main() {}')
    wrapper.unmount()
  })

  it('shows the hidden-code banner', async () => {
    vi.mocked(getSubmission).mockResolvedValue(detail({ codeHidden: true, code: undefined }))
    const wrapper = await mountDrawer()

    expect(document.body.textContent).toContain('该提交的代码暂不可见。')
    wrapper.unmount()
  })

  it('shows rejudge only when allowed', async () => {
    const allowed = await mountDrawer({ canRejudge: true })
    expect(document.querySelector('[data-test="rejudge"]')).toBeTruthy()
    allowed.unmount()

    document.body.innerHTML = ''
    const hidden = await mountDrawer({ canRejudge: false })
    expect(document.querySelector('[data-test="rejudge"]')).toBeNull()
    hidden.unmount()
  })
})
