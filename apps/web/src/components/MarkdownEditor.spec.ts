import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { MdEditor } from 'md-editor-v3'

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

  it('passes the requested theme to the editor', async () => {
    const w = mount(MarkdownEditor, {
      props: { modelValue: 'marker', theme: 'dark' },
      attachTo: document.body,
    })
    await vi.waitFor(() => expect(w.html()).toContain('marker'), { timeout: 3000 })
    expect(w.findComponent(MdEditor).props('theme')).toBe('dark')
  })

  it('never loads third-party scripts or styles', async () => {
    const w = mount(MarkdownEditor, { props: { modelValue: 'marker' }, attachTo: document.body })
    await vi.waitFor(() => expect(w.html()).toContain('marker'), { timeout: 3000 })
    const external = [...document.querySelectorAll('script[src], link[href]')]
      .map((e) => e.getAttribute('src') ?? e.getAttribute('href') ?? '')
      .filter((url) => /^(https?:)?\/\//.test(url))
    expect(external).toEqual([])
  })

  it('disables image upload until the problem has been saved', async () => {
    const w = mount(MarkdownEditor, { props: { modelValue: 'marker' }, attachTo: document.body })
    await vi.waitFor(() => expect(w.html()).toContain('marker'), { timeout: 3000 })

    expect(w.findComponent(MdEditor).props('noUploadImg')).toBe(true)
  })

  it('uploads images through the problem asset callback', async () => {
    const uploadImage = vi.fn().mockResolvedValue('/api/problems/12/assets/1')
    const w = mount(MarkdownEditor, {
      props: { modelValue: 'marker', problemId: 12, uploadImage },
      attachTo: document.body,
    })
    await vi.waitFor(() => expect(w.html()).toContain('marker'), { timeout: 3000 })

    const callback = vi.fn()
    const file = new File(['image'], 'statement.png', { type: 'image/png' })
    const upload = w.findComponent(MdEditor).props('onUploadImg') as (
      files: File[],
      cb: (urls: string[]) => void,
    ) => void
    await upload([file], callback)

    expect(uploadImage).toHaveBeenCalledWith(file)
    expect(callback).toHaveBeenCalledWith(['/api/problems/12/assets/1'])
    expect(w.findComponent(MdEditor).props('noUploadImg')).toBe(false)
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
