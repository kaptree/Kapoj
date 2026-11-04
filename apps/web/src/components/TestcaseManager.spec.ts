import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'

import { listTestcases } from '@/api/problems'
import TestcaseManager from './TestcaseManager.vue'

vi.mock('@/api/problems', () => ({
  listTestcases: vi.fn(),
  deleteTestcase: vi.fn(),
  reorderTestcases: vi.fn(),
  updateTestcase: vi.fn(),
  uploadChecker: vi.fn(),
  uploadTestcase: vi.fn(),
}))

describe('TestcaseManager', () => {
  it('loads the testcases of the new problem when problemId changes', async () => {
    vi.mocked(listTestcases).mockResolvedValue([])
    const wrapper = mount(TestcaseManager, {
      props: { problemId: 1, type: 'traditional', checkerUploaded: false },
      global: { plugins: [ElementPlus] },
    })

    await vi.waitFor(() => expect(listTestcases).toHaveBeenCalledWith(1), { timeout: 2000 })

    await wrapper.setProps({ problemId: 2 })
    await vi.waitFor(() => expect(listTestcases).toHaveBeenCalledWith(2), { timeout: 2000 })

    wrapper.unmount()
  })
})
