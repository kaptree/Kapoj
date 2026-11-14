import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  createPlatformBackup,
  getBackupOverview,
  restoreAccountBackup,
  restorePlatformBackup,
  updateBackupSchedule,
} from '@/api/backups'
import type { BackupOverview, BackupPackage, BackupSchedule } from '@/api/types'
import AdminBackupView from './AdminBackupView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('@/api/backups', () => ({
  getBackupOverview: vi.fn(),
  createPlatformBackup: vi.fn(),
  createAccountBackup: vi.fn(),
  downloadBackup: vi.fn(),
  restorePlatformBackup: vi.fn(),
  restoreAccountBackup: vi.fn(),
  updateBackupSchedule: vi.fn(),
  getRestoreJob: vi.fn(),
  restartServer: vi.fn(),
}))

function pkg(overrides: Partial<BackupPackage> = {}): BackupPackage {
  return {
    id: 1,
    kind: 'platform',
    filename: '/backups/platform-1.zip',
    sizeBytes: 2048,
    status: 'ready',
    createdBy: 1,
    createdAt: '2026-11-14T10:00:00Z',
    completedAt: '2026-11-14T10:01:00Z',
    ...overrides,
  }
}

function schedule(kind: 'platform' | 'accounts'): BackupSchedule {
  return {
    id: kind === 'platform' ? 1 : 2,
    kind,
    enabled: false,
    frequency: 'daily',
    runHour: 3,
    runMinute: 0,
    weekday: 1,
    dayOfMonth: 1,
    updatedBy: 1,
    updatedAt: '2026-11-14T10:00:00Z',
  }
}

function overview(overrides: Partial<BackupOverview> = {}): BackupOverview {
  return {
    platformPackage: pkg(),
    accountsPackage: pkg({ id: 2, kind: 'accounts', filename: '/backups/accounts-2.zip' }),
    accountPackages: [pkg({ id: 3, kind: 'account', userId: 7 })],
    schedules: [schedule('platform'), schedule('accounts')],
    restores: [],
    ...overrides,
  }
}

function mountView() {
  return mount(AdminBackupView, {
    global: { plugins: [PrimeVue] },
  })
}

beforeEach(() => {
  vi.mocked(getBackupOverview).mockReset()
  vi.mocked(createPlatformBackup).mockReset()
  vi.mocked(updateBackupSchedule).mockReset()
  vi.mocked(restorePlatformBackup).mockReset()
  vi.mocked(restoreAccountBackup).mockReset()
  toast.add.mockReset()
})

describe('AdminBackupView', () => {
  it('renders latest packages and schedules', async () => {
    vi.mocked(getBackupOverview).mockResolvedValue(overview())
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-test="platform-package"]').text()).toContain('#1')
    expect(wrapper.find('[data-test="accounts-package"]').text()).toContain('#2')
    expect(wrapper.text()).toContain('账号备份包')
    expect(wrapper.text()).toContain('用户 7')
    wrapper.unmount()
  })

  it('creates a platform backup and saves a schedule', async () => {
    vi.mocked(getBackupOverview).mockResolvedValue(overview())
    vi.mocked(createPlatformBackup).mockResolvedValue(pkg({ status: 'running' }))
    vi.mocked(updateBackupSchedule).mockResolvedValue(schedule('platform'))
    const wrapper = mountView()
    await flushPromises()

    const create = wrapper.findAll('button').find((b) => b.text().includes('立即备份'))
    await create!.trigger('click')
    await flushPromises()
    expect(createPlatformBackup).toHaveBeenCalled()

    const save = wrapper.findAll('button').filter((b) => b.text().trim() === '保存')[0]
    await save!.trigger('click')
    await flushPromises()
    expect(updateBackupSchedule).toHaveBeenCalledWith(
      'platform',
      expect.objectContaining({ frequency: 'daily', runHour: 3 }),
    )
    wrapper.unmount()
  })

  it('requires the RESTORE phrase before starting a platform restore', async () => {
    vi.mocked(getBackupOverview).mockResolvedValue(overview())
    vi.mocked(restorePlatformBackup).mockResolvedValue({
      id: 9,
      kind: 'platform',
      status: 'running',
      createdBy: 1,
      createdAt: '2026-11-14T10:00:00Z',
      restartRequired: false,
    })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('[data-test="platform-restore"]').trigger('click')
    await flushPromises()
    const submit = document.querySelector(
      '[data-test="platform-restore-submit"]',
    ) as HTMLButtonElement
    submit.click()
    await flushPromises()
    expect(restorePlatformBackup).not.toHaveBeenCalled()

    const phrase = document.querySelector(
      '[data-test="platform-restore-phrase"]',
    ) as HTMLInputElement
    phrase.value = 'RESTORE'
    phrase.dispatchEvent(new Event('input'))
    await flushPromises()
    submit.click()
    await flushPromises()
    expect(restorePlatformBackup).toHaveBeenCalledWith(
      { packageId: 1, confirm: 'RESTORE' },
      undefined,
    )
    wrapper.unmount()
  })
})
