import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'

import { listTestcases } from '@/api/problems'
import TestcaseManager from './TestcaseManager.vue'

const push = vi.hoisted(() => vi.fn())

vi.mock('vue-router', () => ({
  useRouter: () => ({ push }),
}))
vi.mock('@/api/problems', () => ({
  listTestcases: vi.fn(),
  deleteTestcase: vi.fn(),
  reorderTestcases: vi.fn(),
  updateTestcase: vi.fn(),
  uploadChecker: vi.fn(),
  uploadTestcase: vi.fn(),
}))

function mountManager(type: 'traditional' | 'special') {
  return mount(TestcaseManager, {
    props: { problemId: 1, type, checkerUploaded: false },
    global: { plugins: [ElementPlus] },
  })
}

describe('TestcaseManager', () => {
  it('loads the testcases of the new problem when problemId changes', async () => {
    vi.mocked(listTestcases).mockResolvedValue([])
    const wrapper = mountManager('traditional')

    await vi.waitFor(() => expect(listTestcases).toHaveBeenCalledWith(1), { timeout: 2000 })

    await wrapper.setProps({ problemId: 2 })
    await vi.waitFor(() => expect(listTestcases).toHaveBeenCalledWith(2), { timeout: 2000 })

    wrapper.unmount()
  })

  it('shows the AI 造数据 button for traditional problems and routes to the generator', async () => {
    vi.mocked(listTestcases).mockResolvedValue([])
    push.mockReset()
    const wrapper = mountManager('traditional')

    const button = wrapper.findAll('button').find((b) => b.text().trim() === 'AI 造数据')
    expect(button).toBeTruthy()

    await button!.trigger('click')
    expect(push).toHaveBeenCalledWith({
      name: 'problem-testcase-generator',
      params: { id: 1 },
    })

    wrapper.unmount()
  })

  it('does not show the AI 造数据 button for special problems', async () => {
    vi.mocked(listTestcases).mockResolvedValue([])
    const wrapper = mountManager('special')

    expect(wrapper.findAll('button').find((b) => b.text().trim() === 'AI 造数据')).toBeUndefined()

    wrapper.unmount()
  })
})
