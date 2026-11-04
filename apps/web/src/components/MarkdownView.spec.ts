import { describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'

import MarkdownView from './MarkdownView.vue'

async function render(source: string) {
  const wrapper = mount(MarkdownView, { props: { source }, attachTo: document.body })
  await vi.waitFor(() => expect(wrapper.html()).toContain('marker'), { timeout: 3000 })
  return wrapper
}

describe('MarkdownView', () => {
  it('renders markdown', async () => {
    const w = await render('**bold** marker\n\n- a\n- b')
    expect(w.find('strong').text()).toBe('bold')
    expect(w.findAll('li')).toHaveLength(2)
  })

  it('does not render script tags from markdown', async () => {
    const w = await render('marker\n\n<script>window.__pwned = 1</script>\n\n<p>after</p>')
    expect(w.find('script').exists()).toBe(false)
    expect((window as unknown as Record<string, unknown>).__pwned).toBeUndefined()
  })

  it('does not create elements from raw html or javascript links', async () => {
    const w = await render(
      'marker\n\n<img src="https://a.com/x.png" onerror="window.__pwned=1">\n\n[click](javascript:alert(1))',
    )
    const preview = w.element as HTMLElement
    expect(preview.querySelector('img')).toBeNull()
    expect(preview.querySelector('[onerror]')).toBeNull()
    expect(preview.querySelector('a[href^="javascript" i]')).toBeNull()
    expect((window as unknown as Record<string, unknown>).__pwned).toBeUndefined()
  })

  it('renders math and code blocks', async () => {
    const w = await render('marker $x^2$\n\n```cpp\nint main() {}\n```')
    await vi.waitFor(() => expect(w.html()).toContain('katex'), { timeout: 3000 })
    expect(w.html()).toContain('int')
  })

  it('never loads third-party scripts or styles', async () => {
    await render('marker $x^2$\n\n```cpp\nint main() {}\n```\n\n```echarts\n{}\n```')
    const external = [...document.querySelectorAll('script[src], link[href]')]
      .map((e) => e.getAttribute('src') ?? e.getAttribute('href') ?? '')
      .filter((url) => /^(https?:)?\/\//.test(url))
    expect(external).toEqual([])
  })

  it('gives headings ids that are unique per preview and cannot clash with page ids', async () => {
    // Two previews in one app, like the statement and the hint on a problem page.
    const Page = defineComponent({
      render: () =>
        h('div', [
          h(MarkdownView, { source: '# Same marker\n\n## Same' }),
          h(MarkdownView, { source: '# Same marker\n\n## Same' }),
        ]),
    })
    const w = mount(Page, { attachTo: document.body })
    const headingIds = () => [...w.element.querySelectorAll('h1[id], h2[id]')].map((e) => e.id)
    await vi.waitFor(() => expect(headingIds()).toHaveLength(4), { timeout: 3000 })

    const ids = headingIds()
    expect(new Set(ids).size).toBe(ids.length)
    for (const id of ids) expect(id).toMatch(/^md-/)
    w.unmount()
  })
})
