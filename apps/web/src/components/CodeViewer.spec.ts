import { mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import CodeViewer from './CodeViewer.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))

function mountViewer(code = 'int main() { return 0; }', language = 'cpp') {
  return mount(CodeViewer, {
    props: { code, language },
    global: { plugins: [PrimeVue] },
  })
}

beforeEach(() => {
  toast.add.mockReset()
  Object.assign(navigator, {
    clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
  })
})

describe('CodeViewer', () => {
  it('renders line numbers and highlighted code', () => {
    const wrapper = mountViewer('int main() {\n  return 0;\n}', 'cpp')

    expect(wrapper.find('.code-viewer__gutter').text()).toBe('1\n2\n3')
    expect(wrapper.find('code.hljs').html()).toContain('hljs-')
    expect(wrapper.find('[data-test="code-copy"]').exists()).toBe(true)
  })

  it('copies the original code', async () => {
    const wrapper = mountViewer('print(1)', 'python3')
    await wrapper.find('[data-test="code-copy"]').trigger('click')

    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('print(1)')
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '代码已复制' }),
    )
  })

  it('escapes unknown languages instead of injecting HTML', () => {
    const wrapper = mountViewer('<script>alert(1)</script>', 'answer')
    expect(wrapper.find('code.hljs').html()).toContain('&lt;script&gt;')
  })
})
