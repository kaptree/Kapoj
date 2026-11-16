import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { ContestSimilarityMatchDetail } from '@/api/types'
import { formatDateTime } from '@/utils/format'
import ContestSimilarityDetailDialog from './ContestSimilarityDetailDialog.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))

function matchDetail(
  overrides: Partial<ContestSimilarityMatchDetail> = {},
): ContestSimilarityMatchDetail {
  return {
    id: 11,
    runId: 1,
    contestId: 7,
    problemId: 42,
    originalSubmissionId: 101,
    suspectSubmissionId: 102,
    originalUserId: 1,
    originalUsername: 'alice',
    originalNickname: 'Alice',
    suspectUserId: 2,
    suspectUsername: 'bob',
    suspectNickname: 'Bob',
    language: 'cpp',
    similarityPercent: 95,
    dicePercent: 88,
    thresholdPercent: 70,
    originalCreatedAt: '2026-11-15T10:00:00Z',
    suspectCreatedAt: '2026-11-15T11:00:00Z',
    finishedAt: '2026-11-15T12:00:00Z',
    originalCode: 'int main() { return 0; }',
    suspectCode: 'int main() { return 1; }',
    originalStatus: 'accepted',
    suspectStatus: 'wrong_answer',
    originalScore: 100,
    suspectScore: 35,
    ...overrides,
  }
}

const wrappers: VueWrapper[] = []
const originalMatchMedia = window.matchMedia

function mountDialog(detail: ContestSimilarityMatchDetail = matchDetail()) {
  const wrapper = mount(ContestSimilarityDetailDialog, {
    props: { visible: true, detail, loading: false, error: '' },
    global: { plugins: [PrimeVue] },
    attachTo: document.body,
  })
  wrappers.push(wrapper)
  return wrapper
}

function panel(selector: string): HTMLElement {
  const element = document.querySelector(selector) as HTMLElement | null
  if (!element) throw new Error(`missing element: ${selector}`)
  return element
}

afterEach(() => {
  wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  window.matchMedia = originalMatchMedia
  document.body.innerHTML = ''
})

describe('ContestSimilarityDetailDialog', () => {
  it('renders both user identities, times, languages and percentages', async () => {
    mountDialog()
    await flushPromises()

    const text = panel('[data-test="similarity-detail"]').textContent ?? ''

    expect(text).toContain('Alice')
    expect(text).toContain('Bob')
    expect(text).toContain(formatDateTime('2026-11-15T10:00:00Z'))
    expect(text).toContain(formatDateTime('2026-11-15T11:00:00Z'))
    expect(text).toContain('C++')
    expect(text).toContain('95%')
    expect(text).toContain('88%')
    expect(text).toContain('阈值 70%')
    expect(text).toContain('通过')
    expect(text).toContain('答案错误')
    expect(text).toContain('相似度仅代表代码相似，需人工复核。')

    const originalScore = panel('[data-test="similarity-original-score"]')
    const suspectScore = panel('[data-test="similarity-suspect-score"]')
    expect(originalScore.textContent?.trim()).toBe('100')
    expect(suspectScore.textContent?.trim()).toBe('35')
  })

  it('renders both code panes with CodeViewer', async () => {
    mountDialog()
    await flushPromises()

    const original = panel('[data-test="similarity-original-code"]')
    const suspect = panel('[data-test="similarity-suspect-code"]')

    expect(original.querySelectorAll('.code-viewer')).toHaveLength(1)
    expect(suspect.querySelectorAll('.code-viewer')).toHaveLength(1)
    expect(original.textContent).toContain('int main() { return 0; }')
    expect(suspect.textContent).toContain('int main() { return 1; }')
  })

  it('uses a stacked layout on mobile width classes', async () => {
    window.matchMedia = vi.fn((query: string) => ({
      matches: query === '(max-width: 760px)',
      media: query,
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false,
    })) as unknown as typeof window.matchMedia

    mountDialog()
    await flushPromises()

    const codes = panel('[data-test="similarity-detail-codes"]')

    expect(codes.className).toContain('similarity-detail__codes--stacked')
    expect(panel('[data-test="similarity-original-code"]')).not.toBeNull()
    expect(panel('[data-test="similarity-suspect-code"]')).not.toBeNull()
  })
})
