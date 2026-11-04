import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import MarkdownEditor from './MarkdownEditor.vue'

describe('MarkdownEditor', () => {
  it('renders an editor for the given value', async () => {
    const w = mount(MarkdownEditor, {
      props: { modelValue: '# Title marker' },
      attachTo: document.body,
    })
    await vi.waitFor(() => expect(w.html()).toContain('marker'), { timeout: 3000 })
    expect(w.find('.md-editor').exists()).toBe(true)
  })

  it('never loads third-party scripts or styles', async () => {
    const w = mount(MarkdownEditor, { props: { modelValue: 'marker' }, attachTo: document.body })
    await vi.waitFor(() => expect(w.html()).toContain('marker'), { timeout: 3000 })
    const external = [...document.querySelectorAll('script[src], link[href]')]
      .map((e) => e.getAttribute('src') ?? e.getAttribute('href') ?? '')
      .filter((url) => /^(https?:)?\/\//.test(url))
    expect(external).toEqual([])
  })

  it('has no toolbar buttons that need uploads or external libraries', async () => {
    const w = mount(MarkdownEditor, { props: { modelValue: 'marker' }, attachTo: document.body })
    await vi.waitFor(() => expect(w.html()).toContain('marker'), { timeout: 3000 })
    const labels = [...w.findAll('.md-editor-toolbar [aria-label]')].map((e) =>
      e.attributes('aria-label'),
    )
    expect(labels).toContain('katex公式')
    for (const excluded of ['图片', '流程图', '格式化']) expect(labels).not.toContain(excluded)
  })
})
