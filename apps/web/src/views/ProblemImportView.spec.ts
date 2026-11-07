import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import { ApiError } from '@/api/http'
import type { HydroImportResult } from '@/api/types'
import ProblemImportView from './ProblemImportView.vue'

const problemsApi = vi.hoisted(() => ({
  importHydroProblems: vi.fn(),
}))

const auth = vi.hoisted(() => ({
  can: vi.fn<(code: string) => boolean>(() => false),
}))

vi.mock('@/api/problems', () => ({
  importHydroProblems: problemsApi.importHydroProblems,
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => auth,
}))

const routes = [
  { path: '/problems', name: 'problems', component: { template: '<div />' } },
  { path: '/problems/import', name: 'problem-import', component: { template: '<div />' } },
  { path: '/problems/:id(\\d+)', name: 'problem-detail', component: { template: '<div />' } },
  { path: '/problems/:id(\\d+)/edit', name: 'problem-edit', component: { template: '<div />' } },
]

async function mountView() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/problems/import')
  await router.isReady()
  const wrapper = mount(ProblemImportView, { global: { plugins: [router, PrimeVue] } })
  await flushPromises()
  return { wrapper, router }
}

function result(overrides: Partial<HydroImportResult> = {}): HydroImportResult {
  return { imported: [], failed: [], ...overrides }
}

function fileLike(name: string, size: number): File {
  const f = new File(['x'], name)
  Object.defineProperty(f, 'size', { value: size })
  return f
}

async function setFile(wrapper: VueWrapper, file: File): Promise<void> {
  const input = wrapper.find('input[type="file"]')
  Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
  await input.trigger('change')
  await flushPromises()
}

function button(wrapper: VueWrapper, label: string) {
  const found = wrapper.findAll('button').find((b) => b.text().includes(label))
  expect(found, `button "${label}"`).toBeTruthy()
  return found!
}

beforeEach(() => {
  auth.can.mockReset()
  auth.can.mockReturnValue(false)
  problemsApi.importHydroProblems.mockReset()
})

describe('ProblemImportView', () => {
  it('uploads a zip and renders imported/failed results', async () => {
    problemsApi.importHydroProblems.mockResolvedValue(
      result({
        imported: [{ id: 101, title: '求和', slug: 'hydro-6760', testcaseCount: 10 }],
        failed: [{ folder: '6761', title: '坏题', error: '缺少 problem_zh.md' }],
      }),
    )
    const { wrapper } = await mountView()

    await setFile(wrapper, fileLike('problems.zip', 1024))
    await button(wrapper, '开始导入').trigger('click')
    await flushPromises()

    expect(problemsApi.importHydroProblems).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('导入成功 1 道')
    expect(wrapper.text()).toContain('求和')
    expect(wrapper.text()).toContain('hydro-6760')
    expect(wrapper.text()).toContain('10')
    expect(wrapper.text()).toContain('导入失败 1 道')
    expect(wrapper.text()).toContain('6761')
    expect(wrapper.text()).toContain('缺少 problem_zh.md')
  })

  it('shows edit links only with problem.update', async () => {
    const imported = { id: 101, title: '求和', slug: 'hydro-6760', testcaseCount: 10 }
    problemsApi.importHydroProblems.mockResolvedValue(result({ imported: [imported] }))

    let view = await mountView()
    await setFile(view.wrapper, fileLike('problems.zip', 1024))
    await button(view.wrapper, '开始导入').trigger('click')
    await flushPromises()

    expect(view.wrapper.find('a[href="/problems/101/edit"]').exists()).toBe(false)
    expect(view.wrapper.text()).toContain('已导入（下线）')
    view.wrapper.unmount()

    auth.can.mockReturnValue(true)
    view = await mountView()
    await setFile(view.wrapper, fileLike('problems.zip', 1024))
    await button(view.wrapper, '开始导入').trigger('click')
    await flushPromises()

    expect(view.wrapper.find('a[href="/problems/101/edit"]').exists()).toBe(true)
    view.wrapper.unmount()
  })

  it('shows upload/API errors in Chinese', async () => {
    problemsApi.importHydroProblems.mockRejectedValue(
      new ApiError(400, 40001, '题包文件不能超过 200 MB'),
    )
    const { wrapper } = await mountView()

    await setFile(wrapper, fileLike('problems.zip', 1024))
    await button(wrapper, '开始导入').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('题包文件不能超过 200 MB')
  })

  it('falls back to a Chinese message for unexpected upload failures', async () => {
    problemsApi.importHydroProblems.mockRejectedValue(new Error('boom'))
    const { wrapper } = await mountView()

    await setFile(wrapper, fileLike('problems.zip', 1024))
    await button(wrapper, '开始导入').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('上传失败，请稍后再试')
  })

  it('rejects non-zip and too-large files before upload', async () => {
    const { wrapper } = await mountView()

    await setFile(wrapper, fileLike('notes.txt', 1024))
    expect(wrapper.text()).toContain('请选择 .zip 格式的题包文件')
    expect(problemsApi.importHydroProblems).not.toHaveBeenCalled()

    await setFile(wrapper, fileLike('big.zip', 200 * 1024 * 1024 + 1))
    expect(wrapper.text()).toContain('题包文件不能超过 200 MB')
    expect(problemsApi.importHydroProblems).not.toHaveBeenCalled()
  })
})
