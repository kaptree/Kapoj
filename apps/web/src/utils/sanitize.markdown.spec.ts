import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { MdPreview } from 'md-editor-v3'

import '@/markdown'
import { sanitizeHtml } from './sanitize'

const document_ = [
  '# Heading marker',
  '',
  '行内公式 $a+b$ 与块公式：',
  '',
  '$$\\sum_{i=1}^{n} \\sqrt{x_i}$$',
  '',
  '```cpp',
  'int main() { return 0; }',
  '```',
  '',
  '```',
  'plain block',
  '```',
  '',
  '- [x] done',
  '- [ ] todo',
  '',
  '1. one',
  '2. two',
  '   - nested',
  '',
  '| a | b |',
  '|---|---|',
  '| 1 | 2 |',
  '',
  '> quote **bold** *italic* ~~strike~~ `inline`',
  '',
  '---',
  '',
  '![alt](https://example.com/x.png "title")',
  '',
  '[link](https://example.com)',
].join('\n')

/** Renders markdown with md-editor and returns the HTML it hands to `sanitize`. */
async function generatedHtml(source: string): Promise<string> {
  let captured = ''
  const w = mount(MdPreview, {
    props: {
      id: 'probe',
      modelValue: source,
      noMermaid: true,
      noEcharts: true,
      sanitize: (html: string) => (captured = html),
    },
    attachTo: document.body,
  })
  await vi.waitFor(() => expect(w.html()).toContain('marker'), { timeout: 3000 })
  await vi.waitFor(() => expect(captured).toContain('katex'), { timeout: 3000 })
  w.unmount()
  return captured
}

function parse(html: string): HTMLElement {
  const root = document.createElement('div')
  root.innerHTML = html
  return root
}

describe('sanitizeHtml on md-editor output', () => {
  it('keeps everything md-editor itself generates, except the hidden MathML copy', async () => {
    const original = parse(await generatedHtml(document_))
    original.querySelectorAll('.katex-mathml').forEach((e) => e.remove())
    const cleaned = parse(sanitizeHtml(original.innerHTML + '')) // sanitize what remains

    // Run the real thing too: sanitizing the unmodified output must give the same result.
    const full = parse(sanitizeHtml(await generatedHtml(document_)))
    expect(full.innerHTML).toBe(cleaned.innerHTML)

    const before = [...original.querySelectorAll('*')]
    const after = [...cleaned.querySelectorAll('*')]
    expect(after.map((e) => e.tagName)).toEqual(before.map((e) => e.tagName))

    const lost: string[] = []
    before.forEach((el, i) => {
      for (const attr of el.getAttributeNames()) {
        if (!after[i].hasAttribute(attr)) lost.push(`${el.tagName.toLowerCase()}[${attr}]`)
      }
      if (el.getAttribute('class') !== after[i].getAttribute('class')) {
        lost.push(`${el.tagName.toLowerCase()}.class`)
      }
    })
    expect([...new Set(lost)]).toEqual([])
    expect(cleaned.textContent).toBe(original.textContent)
  })

  it('still shows formulas: the inline math wrapper keeps its processed marker', async () => {
    const out = parse(sanitizeHtml(await generatedHtml(document_)))
    const wrapper = out.querySelector('.md-editor-katex-inline')
    expect(wrapper?.hasAttribute('data-processed')).toBe(true)
    expect(out.querySelector('.katex-html')).not.toBeNull()
    expect(out.querySelector('.katex-mathml')).toBeNull()
  })

  it('keeps the code block structure that its collapse and copy buttons rely on', async () => {
    const out = parse(sanitizeHtml(await generatedHtml(document_)))
    const block = out.querySelector('details.md-editor-code')
    expect(block).not.toBeNull()
    expect(block?.hasAttribute('open')).toBe(true)
    expect(block?.querySelector('summary')).not.toBeNull()
    expect(block?.querySelector('.md-editor-copy-button')).not.toBeNull()
    expect(block?.querySelector('pre code.language-cpp, pre code[class*="cpp"]')).not.toBeNull()
  })

  it('keeps task list checkboxes but only as disabled checkboxes', async () => {
    const out = parse(sanitizeHtml(await generatedHtml(document_)))
    const boxes = [...out.querySelectorAll('input')]
    expect(boxes.length).toBe(2)
    for (const box of boxes) expect(box.getAttribute('type')).toBe('checkbox')
    expect(sanitizeHtml('<input type="text" value="x"><input type="password">')).not.toContain(
      '<input',
    )
  })
})
