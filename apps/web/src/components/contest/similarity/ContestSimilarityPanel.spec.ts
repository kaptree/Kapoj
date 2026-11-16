import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import Select from 'primevue/select'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  downloadSimilarityCSV,
  getSimilarityMatch,
  getSimilarityOverview,
  listSimilarityMatches,
  startSimilarityRuns,
  updateSimilaritySettings,
} from '@/api/similarity'
import type {
  ContestSimilarityMatch,
  ContestSimilarityMatchDetail,
  ContestSimilarityOverview,
  ContestSimilarityProblem,
  ContestSimilarityRun,
} from '@/api/types'
import ContestSimilarityPanel from './ContestSimilarityPanel.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('@/api/similarity', () => ({
  getSimilarityOverview: vi.fn(),
  updateSimilaritySettings: vi.fn(),
  startSimilarityRuns: vi.fn(),
  listSimilarityMatches: vi.fn(),
  getSimilarityMatch: vi.fn(),
  downloadSimilarityCSV: vi.fn(),
}))

function run(overrides: Partial<ContestSimilarityRun> = {}): ContestSimilarityRun {
  return {
    id: 1,
    contestId: 7,
    problemId: 42,
    status: 'queued',
    thresholdPercent: 70,
    ignoreCommonCode: false,
    engine: 'winnowing',
    engineVersion: '1',
    requestedBy: 1,
    submissionCount: 12,
    matchCount: 0,
    errorMessage: '',
    startedAt: null,
    finishedAt: null,
    createdAt: '2026-11-15T10:00:00Z',
    ...overrides,
  }
}

function problem(overrides: Partial<ContestSimilarityProblem> = {}): ContestSimilarityProblem {
  return {
    problemId: 42,
    displayId: 'A',
    title: 'A + B',
    slug: 'a-plus-b',
    thresholdPercent: 70,
    ignoreCommonCode: false,
    baseLanguage: '',
    baseCode: '',
    submissionCount: 12,
    matchCount: 0,
    latestRun: null,
    ...overrides,
  }
}

function overview(problems: ContestSimilarityProblem[]): ContestSimilarityOverview {
  return { contestId: 7, problems }
}

function match(overrides: Partial<ContestSimilarityMatch> = {}): ContestSimilarityMatch {
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
    ...overrides,
  }
}

function matchDetail(
  overrides: Partial<ContestSimilarityMatchDetail> = {},
): ContestSimilarityMatchDetail {
  return {
    ...match(),
    originalCode: 'int main() { return 0; }',
    suspectCode: 'int main() { return 1; }',
    originalStatus: 'accepted',
    suspectStatus: 'wrong_answer',
    originalScore: 100,
    suspectScore: 35,
    ...overrides,
  }
}

function page<T>(items: T[], total = items.length) {
  return { total, items }
}

const wrappers: VueWrapper[] = []
const originalCreateObjectURL = URL.createObjectURL
const originalRevokeObjectURL = URL.revokeObjectURL

async function mountPanel() {
  const wrapper = mount(ContestSimilarityPanel, {
    props: { contestId: 7 },
    global: { plugins: [PrimeVue] },
  })
  wrappers.push(wrapper)
  await flushPromises()
  return wrapper
}

function field(selector: string): HTMLElement {
  const element = document.querySelector(selector) as HTMLElement | null
  if (!element) throw new Error(`missing element: ${selector}`)
  return element
}

function inputOf(selector: string): HTMLInputElement {
  const root = field(selector)
  const input = root.matches('input') ? root : root.querySelector('input')
  if (!input) throw new Error(`missing input inside: ${selector}`)
  return input as HTMLInputElement
}

function textareaOf(selector: string): HTMLTextAreaElement {
  const root = field(selector)
  const textarea = root.matches('textarea') ? root : root.querySelector('textarea')
  if (!textarea) throw new Error(`missing textarea inside: ${selector}`)
  return textarea as HTMLTextAreaElement
}

function setValue(element: Element, value: string) {
  const proto =
    element instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype
  Object.getOwnPropertyDescriptor(proto, 'value')!.set!.call(element, value)
  element.dispatchEvent(new Event('input', { bubbles: true }))
  element.dispatchEvent(new Event('change', { bubbles: true }))
  // PrimeVue InputNumber commits its value on blur.
  element.dispatchEvent(new Event('blur'))
}

function click(selector: string) {
  field(selector).click()
}

beforeEach(() => {
  toast.add.mockReset()
  vi.mocked(getSimilarityOverview).mockReset()
  vi.mocked(updateSimilaritySettings).mockReset()
  vi.mocked(startSimilarityRuns).mockReset()
  vi.mocked(listSimilarityMatches).mockReset()
  vi.mocked(getSimilarityMatch).mockReset()
  vi.mocked(downloadSimilarityCSV).mockReset()
  vi.mocked(getSimilarityOverview).mockResolvedValue(
    overview([problem(), problem({ problemId: 43, displayId: 'B', title: 'B - A' })]),
  )
  vi.mocked(listSimilarityMatches).mockResolvedValue(page([]))
})

afterEach(() => {
  wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  vi.useRealTimers()
  vi.restoreAllMocks()
  if (originalCreateObjectURL) {
    URL.createObjectURL = originalCreateObjectURL
  } else {
    delete (URL as unknown as Record<string, unknown>).createObjectURL
  }
  if (originalRevokeObjectURL) {
    URL.revokeObjectURL = originalRevokeObjectURL
  } else {
    delete (URL as unknown as Record<string, unknown>).revokeObjectURL
  }
  document.body.innerHTML = ''
})

describe('ContestSimilarityPanel', () => {
  it('loads overview and renders problem thresholds', async () => {
    vi.mocked(getSimilarityOverview).mockResolvedValue(
      overview([
        problem({ thresholdPercent: 70 }),
        problem({ problemId: 43, displayId: 'B', title: 'B - A', thresholdPercent: 90 }),
      ]),
    )

    const wrapper = await mountPanel()

    expect(getSimilarityOverview).toHaveBeenCalledWith(7)
    const rows = wrapper.findAll('[data-test="similarity-problem-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).toContain('70%')
    expect(rows[1]!.text()).toContain('90%')
    expect(rows[0]!.text()).toContain('12')
    const headers = wrapper.findAll('.similarity-overview th').map((header) => header.text())
    expect(headers).toEqual(['题号', '题目', '代码提交数', '阈值', '最近状态', '结果数', '操作'])
  })

  it('saves settings through the dialog', async () => {
    vi.mocked(updateSimilaritySettings).mockResolvedValue({
      contestId: 7,
      problemId: 42,
      thresholdPercent: 80,
      ignoreCommonCode: true,
      baseLanguage: '',
      baseCode: '',
    })

    const wrapper = await mountPanel()
    await wrapper.findAll('[data-test="similarity-settings-open"]')[0]!.trigger('click')
    await flushPromises()

    setValue(inputOf('[data-test="similarity-settings-threshold"]'), '80')
    await flushPromises()
    click('[data-test="similarity-settings-save"]')
    await flushPromises()

    expect(updateSimilaritySettings).toHaveBeenCalledWith(7, 42, {
      thresholdPercent: 80,
      ignoreCommonCode: false,
      baseLanguage: '',
      baseCode: '',
    })
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
    expect(wrapper.findAll('[data-test="similarity-problem-row"]')[0]!.text()).toContain('80%')
  })

  it('preserves an existing common-code template when saving other settings', async () => {
    vi.mocked(getSimilarityOverview).mockResolvedValue(
      overview([problem({ thresholdPercent: 70, baseLanguage: 'cpp', baseCode: 'int main() {}' })]),
    )
    vi.mocked(updateSimilaritySettings).mockResolvedValue({
      contestId: 7,
      problemId: 42,
      thresholdPercent: 80,
      ignoreCommonCode: false,
      baseLanguage: 'cpp',
      baseCode: 'int main() {}',
    })

    const wrapper = await mountPanel()
    await wrapper.findAll('[data-test="similarity-settings-open"]')[0]!.trigger('click')
    await flushPromises()

    expect(textareaOf('[data-test="similarity-settings-base-code"]').value).toBe('int main() {}')

    setValue(inputOf('[data-test="similarity-settings-threshold"]'), '80')
    await flushPromises()
    click('[data-test="similarity-settings-save"]')
    await flushPromises()

    expect(updateSimilaritySettings).toHaveBeenCalledWith(7, 42, {
      thresholdPercent: 80,
      ignoreCommonCode: false,
      baseLanguage: 'cpp',
      baseCode: 'int main() {}',
    })
  })

  it('blocks saving a base code without a language', async () => {
    const wrapper = await mountPanel()
    await wrapper.findAll('[data-test="similarity-settings-open"]')[0]!.trigger('click')
    await flushPromises()

    setValue(textareaOf('[data-test="similarity-settings-base-code"]'), 'int main() {}')
    await flushPromises()
    click('[data-test="similarity-settings-save"]')
    await flushPromises()

    expect(updateSimilaritySettings).not.toHaveBeenCalled()
    expect(field('[data-test="similarity-settings-error"]').textContent).toContain('语言')

    setValue(inputOf('[data-test="similarity-settings-threshold"]'), '75')
    await flushPromises()
    expect(document.querySelector('[data-test="similarity-settings-error"]')).toBeNull()
  })

  it('starts a single problem run', async () => {
    vi.mocked(startSimilarityRuns).mockResolvedValue([run()])

    const wrapper = await mountPanel()
    await wrapper.findAll('[data-test="similarity-run-problem"]')[0]!.trigger('click')
    await flushPromises()

    expect(startSimilarityRuns).toHaveBeenCalledWith(7, 42)
    expect(getSimilarityOverview).toHaveBeenCalledTimes(2)
    expect(listSimilarityMatches).toHaveBeenCalledTimes(2)
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
  })

  it('starts all runs', async () => {
    vi.mocked(startSimilarityRuns).mockResolvedValue([run(), run({ id: 2, problemId: 43 })])

    const wrapper = await mountPanel()
    await wrapper.find('[data-test="similarity-run-all"]').trigger('click')
    await flushPromises()

    expect(startSimilarityRuns).toHaveBeenCalledWith(7)
    expect(getSimilarityOverview).toHaveBeenCalledTimes(2)
    expect(listSimilarityMatches).toHaveBeenCalledTimes(2)
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
  })

  it('polls while a run is queued or running', async () => {
    vi.useFakeTimers()
    vi.mocked(getSimilarityOverview).mockResolvedValue(
      overview([problem({ latestRun: run({ status: 'running' }) })]),
    )

    await mountPanel()
    expect(getSimilarityOverview).toHaveBeenCalledTimes(1)

    vi.mocked(getSimilarityOverview).mockResolvedValue(
      overview([problem({ latestRun: run({ status: 'completed', matchCount: 3 }) })]),
    )
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(getSimilarityOverview).toHaveBeenCalledTimes(2)

    await vi.advanceTimersByTimeAsync(4000)
    await flushPromises()
    expect(getSimilarityOverview).toHaveBeenCalledTimes(2)
  })

  it('does not restart polling after unmount', async () => {
    vi.useFakeTimers()
    let resolveOverview!: (value: ContestSimilarityOverview) => void
    vi.mocked(getSimilarityOverview).mockImplementationOnce(
      () =>
        new Promise<ContestSimilarityOverview>((resolve) => {
          resolveOverview = resolve
        }),
    )

    const wrapper = await mountPanel()
    expect(getSimilarityOverview).toHaveBeenCalledTimes(1)

    wrapper.unmount()
    resolveOverview(overview([problem({ latestRun: run({ status: 'running' }) })]))
    await flushPromises()
    await vi.advanceTimersByTimeAsync(6000)
    await flushPromises()

    expect(getSimilarityOverview).toHaveBeenCalledTimes(1)
  })

  it('refreshes after a run start that lands while a poll is in flight', async () => {
    vi.useFakeTimers()
    vi.mocked(getSimilarityOverview).mockResolvedValueOnce(
      overview([problem({ latestRun: run({ status: 'running' }) })]),
    )
    const wrapper = await mountPanel()
    expect(getSimilarityOverview).toHaveBeenCalledTimes(1)

    let resolvePoll!: (value: ContestSimilarityOverview) => void
    vi.mocked(getSimilarityOverview).mockImplementationOnce(
      () =>
        new Promise<ContestSimilarityOverview>((resolve) => {
          resolvePoll = resolve
        }),
    )
    await vi.advanceTimersByTimeAsync(2000)
    expect(getSimilarityOverview).toHaveBeenCalledTimes(2)

    vi.mocked(startSimilarityRuns).mockResolvedValue([run({ status: 'queued' })])
    vi.mocked(getSimilarityOverview).mockResolvedValueOnce(
      overview([problem({ latestRun: run({ status: 'queued' }) })]),
    )
    await wrapper.find('[data-test="similarity-run-all"]').trigger('click')
    await flushPromises()

    resolvePoll(overview([problem({ latestRun: run({ status: 'running' }) })]))
    await flushPromises()

    expect(startSimilarityRuns).toHaveBeenCalledWith(7)
    expect(getSimilarityOverview).toHaveBeenCalledTimes(3)
  })

  it('shows an error toast and keeps the dialog open when saving fails', async () => {
    vi.mocked(updateSimilaritySettings).mockRejectedValue(new Error('save failed'))

    const wrapper = await mountPanel()
    await wrapper.findAll('[data-test="similarity-settings-open"]')[0]!.trigger('click')
    await flushPromises()
    click('[data-test="similarity-settings-save"]')
    await flushPromises()

    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'error' }))
    expect(document.querySelector('[data-test="similarity-settings-save"]')).not.toBeNull()
    expect(wrapper.findAll('[data-test="similarity-problem-row"]')[0]!.text()).toContain('70%')
  })

  it('shows an error toast when starting runs fails', async () => {
    vi.mocked(startSimilarityRuns).mockRejectedValue(new Error('queue down'))

    const wrapper = await mountPanel()
    await wrapper.find('[data-test="similarity-run-all"]').trigger('click')
    await flushPromises()

    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'error' }))
    expect(getSimilarityOverview).toHaveBeenCalledTimes(1)
  })

  it('shows empty and error states', async () => {
    vi.mocked(getSimilarityOverview).mockResolvedValueOnce(overview([]))
    let wrapper = await mountPanel()
    expect(wrapper.text()).toContain('暂无可查重题目')
    wrapper.unmount()

    vi.mocked(getSimilarityOverview).mockRejectedValueOnce(new Error('boom'))
    wrapper = await mountPanel()
    expect(wrapper.text()).toContain('加载查重概览失败')
  })

  it('loads and renders the latest match list', async () => {
    vi.mocked(listSimilarityMatches).mockResolvedValue(
      page([
        match(),
        match({
          id: 12,
          suspectNickname: 'Carol',
          suspectUsername: 'carol',
          similarityPercent: 82,
        }),
      ]),
    )

    const wrapper = await mountPanel()

    expect(listSimilarityMatches).toHaveBeenCalledWith(7, {
      problemId: undefined,
      page: 1,
      pageSize: 20,
    })
    const rows = wrapper.findAll('[data-test="similarity-match-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).toContain('A · A + B')
    expect(rows[0]!.text()).toContain('Alice')
    expect(rows[0]!.text()).toContain('Bob')
    expect(rows[0]!.text()).toContain('95%')
    expect(rows[0]!.find('.p-tag-danger').exists()).toBe(true)
    expect(rows[1]!.find('.p-tag-warn').exists()).toBe(true)
  })

  it('filters matches by problem', async () => {
    const wrapper = await mountPanel()

    const filter = wrapper.findAllComponents(Select)[0]!
    filter.vm.$emit('update:modelValue', 43)
    await flushPromises()

    expect(listSimilarityMatches).toHaveBeenLastCalledWith(7, {
      problemId: 43,
      page: 1,
      pageSize: 20,
    })
  })

  it('opens detail on row click and fetches the match', async () => {
    vi.mocked(listSimilarityMatches).mockResolvedValue(page([match()]))
    vi.mocked(getSimilarityMatch).mockResolvedValue(matchDetail())

    const wrapper = await mountPanel()
    await wrapper.find('[data-test="similarity-match-row"]').trigger('click')
    await flushPromises()

    expect(getSimilarityMatch).toHaveBeenCalledWith(7, 11)
    const dialog = document.querySelector('[data-test="similarity-detail"]')
    expect(dialog).not.toBeNull()
    expect(dialog!.textContent).toContain('Alice')
    expect(dialog!.textContent).toContain('Bob')
  })

  it('downloads CSV with the current problem filter', async () => {
    vi.useFakeTimers()
    const blob = new Blob(['id,similarity'], { type: 'text/csv' })
    vi.mocked(downloadSimilarityCSV).mockResolvedValue(blob)
    const createObjectURL = vi.fn(() => 'blob:mock')
    const revokeObjectURL = vi.fn()
    Object.defineProperty(URL, 'createObjectURL', {
      configurable: true,
      writable: true,
      value: createObjectURL,
    })
    Object.defineProperty(URL, 'revokeObjectURL', {
      configurable: true,
      writable: true,
      value: revokeObjectURL,
    })
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined)

    const wrapper = await mountPanel()
    wrapper.findAllComponents(Select)[0]!.vm.$emit('update:modelValue', 43)
    await flushPromises()
    await wrapper.find('[data-test="similarity-export"]').trigger('click')
    await flushPromises()

    expect(downloadSimilarityCSV).toHaveBeenCalledWith(7, 43)
    expect(createObjectURL).toHaveBeenCalledWith(blob)
    expect(click).toHaveBeenCalled()
    expect(revokeObjectURL).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(0)

    expect(revokeObjectURL).toHaveBeenCalledWith('blob:mock')
  })

  it('shows no-match and failure states', async () => {
    let wrapper = await mountPanel()
    expect(wrapper.text()).toContain('暂无查重结果')
    wrapper.unmount()

    vi.mocked(listSimilarityMatches).mockRejectedValueOnce(new Error('boom'))
    wrapper = await mountPanel()
    expect(wrapper.text()).toContain('加载查重结果失败')
  })
})
