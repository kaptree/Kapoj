import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import InputNumber from 'primevue/inputnumber'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  getAdminDifficultySettings,
  getAdminJobRun,
  listAdminJobRuns,
  listAdminJobs,
  runAdminJob,
  updateAdminDifficultySettings,
  updateAdminJob,
} from '@/api/admin'
import type { DifficultySettingsView, JobRunView, JobView } from '@/api/types'
import AdminJobsView from './AdminJobsView.vue'

const DIFFICULTY_JOB_KEY = 'problem.difficulty.recalculate'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/admin', () => ({
  listAdminJobs: vi.fn(),
  updateAdminJob: vi.fn(),
  runAdminJob: vi.fn(),
  listAdminJobRuns: vi.fn(),
  getAdminJobRun: vi.fn(),
  getAdminDifficultySettings: vi.fn(),
  updateAdminDifficultySettings: vi.fn(),
}))

function jobView(overrides: Partial<JobView> = {}): JobView {
  return {
    key: DIFFICULTY_JOB_KEY,
    name: '全站题目难度重算',
    description: '按统计窗口内的去重用户通过率重算自动调整题目的难度。',
    cronExpr: '0 3 * * *',
    timezone: 'Asia/Shanghai',
    enabled: true,
    nextRunAt: '2026-11-19T19:00:00Z',
    lastRunAt: '2026-11-18T19:00:00Z',
    lastStatus: 'succeeded',
    lastError: '',
    activeRun: null,
    updatedAt: '2026-11-18T19:00:00Z',
    ...overrides,
  }
}

function runView(overrides: Partial<JobRunView> = {}): JobRunView {
  return {
    id: 12,
    jobKey: DIFFICULTY_JOB_KEY,
    trigger: 'manual',
    status: 'queued',
    progressCurrent: 0,
    progressTotal: 0,
    stats: {},
    errorMessage: '',
    createdBy: 1,
    createdAt: '2026-11-18T10:00:00Z',
    startedAt: null,
    finishedAt: null,
    ...overrides,
  }
}

function settingsView(overrides: Partial<DifficultySettingsView> = {}): DifficultySettingsView {
  return {
    enabled: true,
    defaultScore: 50,
    minSample: 10,
    priorStrength: 10,
    maxStep: 5,
    windowDays: 365,
    includeContest: false,
    curveGamma: 1.5,
    aiAssessEnabled: true,
    aiTimeoutSeconds: 15,
    updatedAt: '2026-11-18T00:00:00Z',
    ...overrides,
  }
}

let wrapper: ReturnType<typeof mount> | undefined

async function mountPage() {
  wrapper = mount(AdminJobsView, { global: { plugins: [PrimeVue] } })
  await flushPromises()
  return wrapper
}

function tab(target: ReturnType<typeof mount>, label: string) {
  const found = target.findAll('[role="tab"]').find((item) => item.text() === label)
  expect(found, `tab "${label}"`).toBeTruthy()
  return found!
}

function card(target: ReturnType<typeof mount>, key: string) {
  return target.find(`[data-test="job-card-${key}"]`)
}

function cardButton(target: ReturnType<typeof mount>, key: string, label: string) {
  const found = card(target, key)
    .findAll('button')
    .find((item) => item.text().trim() === label)
  expect(found, `button "${label}"`).toBeTruthy()
  return found!
}

function dialog(): HTMLElement {
  return document.querySelector('.p-dialog') as HTMLElement
}

function dialogButton(label: string): HTMLButtonElement {
  return Array.from(dialog().querySelectorAll('button')).find(
    (item) => item.textContent?.trim() === label,
  ) as HTMLButtonElement
}

function setField(element: Element, value: string) {
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(element, value)
  element.dispatchEvent(new Event('input', { bubbles: true }))
}

beforeEach(() => {
  document.body.innerHTML = ''
  toast.add.mockReset()
  confirm.require.mockReset()
  confirm.require.mockImplementation((options: { accept?: () => void }) => options.accept?.())

  vi.mocked(listAdminJobs).mockReset()
  vi.mocked(updateAdminJob).mockReset()
  vi.mocked(runAdminJob).mockReset()
  vi.mocked(listAdminJobRuns).mockReset()
  vi.mocked(getAdminJobRun).mockReset()
  vi.mocked(getAdminDifficultySettings).mockReset()
  vi.mocked(updateAdminDifficultySettings).mockReset()

  vi.mocked(listAdminJobs).mockResolvedValue([jobView()])
  vi.mocked(listAdminJobRuns).mockResolvedValue({ total: 0, items: [] })
  vi.mocked(getAdminDifficultySettings).mockResolvedValue(settingsView())
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.useRealTimers()
  document.body.innerHTML = ''
})

describe('AdminJobsView', () => {
  it('renders the three job tabs and the difficulty job', async () => {
    const page = await mountPage()

    expect(listAdminJobs).toHaveBeenCalledTimes(1)
    expect(page.text()).toContain('定时任务')
    expect(page.text()).toContain('任务列表')
    expect(page.text()).toContain('题目难度')
    expect(page.text()).toContain('运行历史')
    expect(page.text()).toContain('全站题目难度重算')
    expect(page.text()).toContain(DIFFICULTY_JOB_KEY)
    expect(page.text()).toContain('0 3 * * *')
    expect(page.text()).toContain('Asia/Shanghai')
    expect(page.text()).toContain('已启用')
    expect(page.text()).toContain('成功')
  })

  it('shows the rating placeholder', async () => {
    const page = await mountPage()

    const placeholder = page.find('[data-test="job-rating-placeholder"]')
    expect(placeholder.exists()).toBe(true)
    expect(placeholder.text()).toContain('规划中')
    expect(placeholder.text()).toContain('全站用户 Rating 重算')
    expect(placeholder.find('button').attributes('disabled')).toBeDefined()
  })

  it('runs the difficulty job and shows the queued run status', async () => {
    vi.mocked(runAdminJob).mockResolvedValue(runView({ status: 'queued' }))
    const page = await mountPage()

    await cardButton(page, DIFFICULTY_JOB_KEY, '立即执行').trigger('click')
    await flushPromises()

    expect(confirm.require).toHaveBeenCalledWith(
      expect.objectContaining({ header: '立即执行任务' }),
    )
    expect(runAdminJob).toHaveBeenCalledWith(DIFFICULTY_JOB_KEY)
    const banner = page.find('[data-test="latest-run"]')
    expect(banner.exists()).toBe(true)
    expect(banner.text()).toContain('#12')
    expect(banner.text()).toContain('排队中')
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '任务已入队' }),
    )
  })

  it('refreshes the queued run to its final status while polling', async () => {
    vi.useFakeTimers()
    vi.mocked(runAdminJob).mockResolvedValue(runView({ status: 'queued' }))
    vi.mocked(getAdminJobRun).mockResolvedValue(
      runView({
        status: 'succeeded',
        progressCurrent: 1200,
        progressTotal: 1200,
        startedAt: '2026-11-18T10:00:00Z',
        finishedAt: '2026-11-18T10:02:00Z',
      }),
    )
    const page = await mountPage()

    await cardButton(page, DIFFICULTY_JOB_KEY, '立即执行').trigger('click')
    await flushPromises()
    expect(page.find('[data-test="latest-run"]').text()).toContain('排队中')

    vi.mocked(listAdminJobs).mockResolvedValue([jobView({ lastStatus: 'succeeded' })])
    await vi.advanceTimersByTimeAsync(2500)
    await flushPromises()

    expect(getAdminJobRun).toHaveBeenCalledWith(12)
    expect(page.find('[data-test="latest-run"]').text()).toContain('成功')
  })

  it('updates the cron schedule through the edit dialog', async () => {
    vi.mocked(updateAdminJob).mockResolvedValue(jobView({ cronExpr: '0 4 * * *' }))
    const page = await mountPage()

    await cardButton(page, DIFFICULTY_JOB_KEY, '编辑计划').trigger('click')
    await flushPromises()

    expect(dialog().textContent).toContain('编辑任务计划')
    setField(dialog().querySelector('#job-cron')!, '0 4 * * *')
    await flushPromises()
    dialogButton('保存计划').click()
    await flushPromises()

    expect(updateAdminJob).toHaveBeenCalledWith(DIFFICULTY_JOB_KEY, {
      enabled: true,
      cronExpr: '0 4 * * *',
      timezone: 'Asia/Shanghai',
    })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '计划已更新' }),
    )
    expect(card(page, DIFFICULTY_JOB_KEY).text()).toContain('0 4 * * *')
  })

  it('rejects an invalid cron expression before calling the API', async () => {
    const page = await mountPage()

    await cardButton(page, DIFFICULTY_JOB_KEY, '编辑计划').trigger('click')
    await flushPromises()
    setField(dialog().querySelector('#job-cron')!, 'not-a-cron')
    await flushPromises()
    dialogButton('保存计划').click()
    await flushPromises()

    expect(updateAdminJob).not.toHaveBeenCalled()
    expect(dialog().textContent).toContain('cron 表达式需为 5 段')
  })

  it('saves the difficulty settings and shows the formula', async () => {
    vi.mocked(updateAdminDifficultySettings).mockResolvedValue(settingsView({ defaultScore: 60 }))
    const page = await mountPage()

    await tab(page, '题目难度').trigger('click')
    await flushPromises()

    expect(getAdminDifficultySettings).toHaveBeenCalledTimes(1)
    expect(page.text()).toContain('target = round(1 + 99')
    expect(page.text()).toContain('目标难度约为 31 分')
    expect(page.text()).not.toContain('目标难度约为 60 分')

    page.findAllComponents(InputNumber)[0]!.vm.$emit('update:modelValue', 60)
    await flushPromises()
    await page.find('form.difficulty-form').trigger('submit')
    await flushPromises()

    expect(updateAdminDifficultySettings).toHaveBeenCalledWith({
      enabled: true,
      defaultScore: 60,
      minSample: 10,
      priorStrength: 10,
      maxStep: 5,
      windowDays: 365,
      includeContest: false,
      curveGamma: 1.5,
      aiAssessEnabled: true,
      aiTimeoutSeconds: 15,
    })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '设置已保存' }),
    )
  })

  it('renders run history and stops polling when the run finishes', async () => {
    vi.useFakeTimers()
    const running = runView({
      status: 'running',
      progressCurrent: 400,
      progressTotal: 1200,
      stats: { total: 1200, processed: 400, changed: 23 },
      startedAt: '2026-11-18T10:00:00Z',
    })
    vi.mocked(listAdminJobs).mockResolvedValue([
      jobView({ lastStatus: 'running', activeRun: running }),
    ])
    vi.mocked(listAdminJobRuns).mockResolvedValue({ total: 1, items: [running] })

    const page = await mountPage()
    await tab(page, '运行历史').trigger('click')
    await flushPromises()

    expect(listAdminJobRuns).toHaveBeenCalledWith(
      expect.objectContaining({ jobKey: undefined, status: undefined, page: 1, pageSize: 20 }),
    )
    const table = page.find('[data-test="runs-table"]')
    expect(table.text()).toContain('全站题目难度重算')
    expect(table.text()).toContain('手动')
    expect(table.text()).toContain('运行中')
    expect(table.text()).toContain('400 / 1200')
    expect(table.text()).toContain('total=1200')

    const finished = runView({
      status: 'succeeded',
      progressCurrent: 1200,
      progressTotal: 1200,
      startedAt: '2026-11-18T10:00:00Z',
      finishedAt: '2026-11-18T10:02:00Z',
    })
    vi.mocked(listAdminJobs).mockResolvedValue([jobView({ lastStatus: 'succeeded' })])
    vi.mocked(listAdminJobRuns).mockResolvedValue({ total: 1, items: [finished] })

    await vi.advanceTimersByTimeAsync(2500)
    await flushPromises()

    expect(listAdminJobs).toHaveBeenCalledTimes(2)
    expect(listAdminJobRuns).toHaveBeenCalledTimes(2)
    expect(page.find('[data-test="runs-table"]').text()).toContain('成功')

    await vi.advanceTimersByTimeAsync(10000)
    expect(listAdminJobs).toHaveBeenCalledTimes(2)
  })

  it('opens history filtered to the selected job', async () => {
    const page = await mountPage()

    await cardButton(page, DIFFICULTY_JOB_KEY, '运行历史').trigger('click')
    await flushPromises()

    expect(listAdminJobRuns).toHaveBeenCalledWith(
      expect.objectContaining({ jobKey: DIFFICULTY_JOB_KEY, page: 1, pageSize: 20 }),
    )
  })

  it('keeps polling after a transient refresh failure and terminalizes the active run', async () => {
    vi.useFakeTimers()
    const running = runView({
      status: 'running',
      progressCurrent: 400,
      progressTotal: 1200,
      startedAt: '2026-11-18T10:00:00Z',
    })
    vi.mocked(listAdminJobs).mockResolvedValue([
      jobView({ lastStatus: 'running', activeRun: running }),
    ])
    const page = await mountPage()
    expect(listAdminJobs).toHaveBeenCalledTimes(1)

    vi.mocked(listAdminJobs).mockRejectedValueOnce(new Error('network down'))
    await vi.advanceTimersByTimeAsync(2500)
    await flushPromises()

    expect(listAdminJobs).toHaveBeenCalledTimes(2)
    expect(page.text()).toContain('刷新任务状态失败')
    expect(page.text()).toContain('当前运行')

    vi.mocked(listAdminJobs).mockResolvedValue([jobView({ lastStatus: 'succeeded' })])
    await vi.advanceTimersByTimeAsync(5000)
    await flushPromises()

    expect(listAdminJobs).toHaveBeenCalledTimes(3)
    expect(page.text()).not.toContain('当前运行')
    expect(page.text()).not.toContain('刷新任务状态失败')
    const runButton = card(page, DIFFICULTY_JOB_KEY)
      .findAll('button')
      .find((item) => item.text().trim() === '立即执行')
    expect(runButton?.attributes('disabled')).toBeUndefined()

    await vi.advanceTimersByTimeAsync(10000)
    expect(listAdminJobs).toHaveBeenCalledTimes(3)
  })

  it('clears the poll timer on unmount', async () => {
    vi.useFakeTimers()
    vi.mocked(listAdminJobs).mockResolvedValue([
      jobView({ lastStatus: 'running', activeRun: runView({ status: 'running' }) }),
    ])
    const page = await mountPage()

    page.unmount()
    wrapper = undefined

    await vi.advanceTimersByTimeAsync(10000)
    expect(listAdminJobs).toHaveBeenCalledTimes(1)
  })
})
