import { describe, expect, it } from 'vitest'
import katex from 'katex'

import { sanitizeHtml } from './sanitize'

describe('sanitizeHtml', () => {
  it('removes script tags and their content', () => {
    const out = sanitizeHtml('<p>hi</p><script>alert(1)</script>')
    expect(out).toContain('<p>hi</p>')
    expect(out).not.toContain('script')
    expect(out).not.toContain('alert')
  })

  it('removes event handler attributes', () => {
    const out = sanitizeHtml('<img src="https://a.com/x.png" onerror="alert(1)" onclick="x()">')
    expect(out).toContain('https://a.com/x.png')
    expect(out).not.toContain('onerror')
    expect(out).not.toContain('onclick')
  })

  it('removes javascript urls', () => {
    const out = sanitizeHtml(
      '<a href="javascript:alert(1)">x</a><a href=" JaVaScRiPt:alert(2)">y</a>',
    )
    expect(out.toLowerCase()).not.toContain('javascript')
  })

  it('removes iframes, objects and forms', () => {
    const out = sanitizeHtml(
      '<iframe src="https://evil.com"></iframe><object data="x"></object><form action="https://evil.com"><input></form>',
    )
    for (const tag of ['iframe', 'object', 'form', 'input']) expect(out).not.toContain(`<${tag}`)
  })

  it('keeps code blocks with highlight classes', () => {
    const html =
      '<pre><code class="language-cpp hljs"><span class="hljs-keyword">int</span> x;</code></pre>'
    const out = sanitizeHtml(html)
    expect(out).toContain('class="language-cpp hljs"')
    expect(out).toContain('class="hljs-keyword"')
  })

  it('keeps tables and basic formatting', () => {
    const out = sanitizeHtml(
      '<table><thead><tr><th>a</th></tr></thead><tbody><tr><td><b>1</b></td></tr></tbody></table>',
    )
    expect(out).toContain('<table>')
    expect(out).toContain('<th>a</th>')
    expect(out).toContain('<b>1</b>')
  })

  it('adds rel noopener to links and opens them in a new tab', () => {
    const out = sanitizeHtml('<a href="https://a.com">x</a>')
    expect(out).toContain('rel="noopener noreferrer"')
    expect(out).toContain('target="_blank"')
  })

  it('drops non-http image sources', () => {
    expect(sanitizeHtml('<img src="data:image/png;base64,AAAA">')).not.toContain('data:')
    expect(sanitizeHtml('<img src="javascript:alert(1)">').toLowerCase()).not.toContain(
      'javascript',
    )
    expect(sanitizeHtml('<img src="https://a.com/x.png" alt="ok">')).toContain(
      'src="https://a.com/x.png"',
    )
  })

  it('keeps rendered KaTeX output but not its MathML duplicate', () => {
    const out = sanitizeHtml(katex.renderToString('\\frac{a}{b}+\\sqrt{x}^2'))
    expect(out).toContain('class="katex"')
    expect(out).toContain('katex-html')
    expect(out).toContain('vertical-align')
    expect(out).toContain('<svg')
    expect(out).toContain('viewBox')
    expect(out).not.toContain('katex-mathml')
    expect(out).not.toContain('<math')
  })

  it('only allows harmless inline styles', () => {
    const out = sanitizeHtml(
      '<span style="background:url(https://evil.com/x);position:fixed;top:0;width:100%;height:1.5em;color:red">x</span>',
    )
    expect(out).not.toContain('url(')
    expect(out).not.toContain('position')
    expect(out).not.toContain('color')
    expect(out).toContain('height:1.5em')
  })

  it('removes style attributes outside spans', () => {
    expect(sanitizeHtml('<div style="height:1em">x</div>')).not.toContain('style')
  })
})
