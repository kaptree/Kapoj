import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { importAdminUsers, previewAdminUsersImport } from '@/api/admin'
import { ApiError } from '@/api/http'
import type { AdminUserImportResult } from '@/api/types'
import AdminUserImportDialog from './AdminUserImportDialog.vue'

const api = vi.hoisted(() => ({
  previewAdminUsersImport: vi.fn(),
  importAdminUsers: vi.fn(),
}))
vi.mock('@/api/admin', () => api)

function fixture(overrides: Partial<AdminUserImportResult> = {}): AdminUserImportResult {
  return {
    dryRun: true,
    summary: {
      total: 3,
      valid: 2,
      conflicts: 1,
      invalid: 0,
      generatedUsernames: 1,
      imported: 0,
      skipped: 0,
      failed: 0,
    },
    issues: [
      {
        line: 2,
        legacyId: 2,
        uname: 'Kaptree',
        email: 'kaptree@example.com',
        status: 'conflict',
        reason: '用户名已存在',
      },
    ],
    issuesTruncated: false,
    mapping: [
      {
        legacyId: 6,
        uname: '让我睡觉',
        email: 'me@example.com',
        username: 'hydro_6',
      },
    ],
    samples: [
      {
        line: 3,
        legacyId: 6,
        uname: '让我睡觉',
        email: 'me@example.com',
        username: 'hydro_6',
        generated: true,
      },
    ],
    ...overrides,
  }
}

async function mountDialog() {
  const wrapper = mount(AdminUserImportDialog, {
    props: { visible: false },
    global: { plugins: [PrimeVue] },
  })
  // PrimeVue Dialog only renders on a false→true transition.
  await wrapper.setProps({ visible: true })
  await flushPromises()
  return wrapper
}

function dialog(): HTMLElement {
  const element = document.body.querySelector('.p-dialog')
  if (!element) throw new Error('dialog not found')
  return element as HTMLElement
}

function q<T extends Element>(selector: string): T {
  const element = dialog().querySelector(selector)
  if (!element) throw new Error(`missing ${selector}`)
  return element as T
}

async function click(testid: string): Promise<void> {
  q<HTMLButtonElement>(`[data-test="${testid}"]`).click()
  await flushPromises()
}

async function selectFile(file: File): Promise<void> {
  const input = q<HTMLInputElement>('[data-test="import-file"]')
  Object.defineProperty(input, 'files', { value: [file], configurable: true })
  input.dispatchEvent(new Event('change', { bubbles: true }))
  await flushPromises()
}

beforeEach(() => {
  document.body.innerHTML = ''
  api.previewAdminUsersImport.mockReset()
  api.importAdminUsers.mockReset()
})

describe('AdminUserImportDialog', () => {
  it('shows CSV fields, limits and the sample from the help button', async () => {
    const wrapper = await mountDialog()
    expect(q('[data-test="import-help-toggle"]').getAttribute('aria-expanded')).toBe('false')
    expect(dialog().querySelector('[data-test="import-help"]')).toBeNull()

    await click('import-help-toggle')
    expect(q('[data-test="import-help-toggle"]').getAttribute('aria-expanded')).toBe('true')
    const help = q('[data-test="import-help"]').textContent ?? ''
    expect(help).toContain('_id')
    expect(help).toContain('uname')
    expect(help).toContain('hashType')
    expect(help).toContain('hydro')
    expect(help).toContain('1001,alice')

    await click('import-help-toggle')
    expect(q('[data-test="import-help-toggle"]').getAttribute('aria-expanded')).toBe('false')
    expect(dialog().querySelector('[data-test="import-help"]')).toBeNull()
    wrapper.unmount()
  })

  it('previews a selected CSV and renders summary, issues and samples', async () => {
    api.previewAdminUsersImport.mockResolvedValue(fixture())
    const wrapper = await mountDialog()
    const file = new File(['_id,uname,mail,hash,salt,hashType\n'], 'users.csv', {
      type: 'text/csv',
    })

    await selectFile(file)
    await click('import-preview')

    expect(previewAdminUsersImport).toHaveBeenCalledWith(file)
    expect(q('[data-test="summary-total"]').textContent).toContain('3')
    expect(q('[data-test="summary-valid"]').textContent).toContain('2')
    expect(q('[data-test="summary-conflicts"]').textContent).toContain('1')
    expect(q('[data-test="summary-generated"]').textContent).toContain('1')
    expect(dialog().textContent).toContain('Kaptree')
    expect(dialog().textContent).toContain('用户名已存在')
    expect(dialog().textContent).toContain('hydro_6')
    expect(dialog().querySelector('[data-test="import-mapping"]')).not.toBeNull()
    wrapper.unmount()
  })

  it('hides the mapping download when no username was generated', async () => {
    api.previewAdminUsersImport.mockResolvedValue(
      fixture({
        summary: {
          total: 1,
          valid: 1,
          conflicts: 0,
          invalid: 0,
          generatedUsernames: 0,
          imported: 0,
          skipped: 0,
          failed: 0,
        },
        mapping: [],
      }),
    )
    const wrapper = await mountDialog()
    await selectFile(new File(['x'], 'users.csv', { type: 'text/csv' }))
    await click('import-preview')

    expect(dialog().querySelector('[data-test="import-mapping"]')).toBeNull()
    wrapper.unmount()
  })

  it('commits after preview and emits imported', async () => {
    api.previewAdminUsersImport.mockResolvedValue(fixture())
    api.importAdminUsers.mockResolvedValue(
      fixture({
        dryRun: false,
        summary: {
          total: 3,
          valid: 2,
          conflicts: 1,
          invalid: 0,
          generatedUsernames: 1,
          imported: 2,
          skipped: 1,
          failed: 0,
        },
        issues: [],
        samples: [],
      }),
    )
    const wrapper = await mountDialog()
    const file = new File(['x'], 'users.csv', { type: 'text/csv' })

    await selectFile(file)
    await click('import-preview')
    await click('import-confirm')

    expect(importAdminUsers).toHaveBeenCalledWith(file)
    expect(wrapper.emitted('imported')).toHaveLength(1)
    expect(dialog().textContent).toContain('成功 2')
    wrapper.unmount()
  })

  it('shows a Chinese error when the request fails', async () => {
    api.previewAdminUsersImport.mockRejectedValue(new ApiError(400, 40001, 'CSV 表头错误'))
    const wrapper = await mountDialog()
    await selectFile(new File(['x'], 'users.csv', { type: 'text/csv' }))
    await click('import-preview')

    expect(q('[role="alert"]').textContent).toContain('CSV 表头错误')
    wrapper.unmount()
  })

  it('rejects non-csv and oversized files before calling the API', async () => {
    const wrapper = await mountDialog()
    await selectFile(new File(['x'], 'users.txt', { type: 'text/plain' }))
    expect(q('[role="alert"]').textContent).toContain('CSV')

    const oversized = new File(['x'], 'users.csv', { type: 'text/csv' })
    Object.defineProperty(oversized, 'size', { value: 5 * 1024 * 1024 + 1 })
    await selectFile(oversized)
    expect(q('[role="alert"]').textContent).toContain('5 MiB')
    expect(previewAdminUsersImport).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
