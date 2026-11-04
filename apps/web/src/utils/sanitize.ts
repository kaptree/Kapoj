import sanitize from 'sanitize-html'

// md-editor-v3 hands its whole rendered preview to this function, including its
// own interface markup: the collapsible code block (details/summary), copy
// buttons, task list checkboxes, KaTeX output and small inline SVG icons. All of
// that has to survive, otherwise code blocks lose their structure and formulas
// stay hidden (the library reveals a formula only once it carries data-processed).
// What must never survive: scripts, frames, plugins, forms, event handlers,
// style tags and non-http(s) URLs. sanitize.spec.ts and sanitize.markdown.spec.ts
// pin both sides.

// Inline styles are only allowed on spans, and only for the numeric box
// properties KaTeX emits. Everything else (colors, positioning, url()) is dropped.
const length = /^-?\d*\.?\d+(em|ex|px|%)?$/
const katexStyles = Object.fromEntries(
  [
    'height',
    'width',
    'min-width',
    'top',
    'left',
    'vertical-align',
    'margin-left',
    'margin-right',
    'padding-left',
    'border-bottom-width',
    'border-right-width',
  ].map((prop) => [prop, [length]]),
)
// KaTeX nudges some glyphs with position:relative; any other position (fixed,
// absolute, sticky) could cover the page, so only relative is allowed.
katexStyles.position = [/^relative$/]
// md-editor sizes the line number gutter of code blocks with a custom property.
katexStyles['--md-code-line-number-width'] = [/^\d+(\.\d+)?ch$/]

const svgAttributes = [
  'width',
  'height',
  'viewBox',
  'preserveAspectRatio',
  'xmlns',
  'fill',
  'stroke',
  'stroke-width',
  'stroke-linecap',
  'stroke-linejoin',
]

const options: sanitize.IOptions = {
  allowedTags: [
    'h1',
    'h2',
    'h3',
    'h4',
    'h5',
    'h6',
    'p',
    'br',
    'hr',
    'blockquote',
    'pre',
    'code',
    'span',
    'div',
    'a',
    'img',
    'b',
    'strong',
    'i',
    'em',
    'u',
    's',
    'del',
    'sub',
    'sup',
    'kbd',
    'mark',
    'ul',
    'ol',
    'li',
    'table',
    'thead',
    'tbody',
    'tr',
    'th',
    'td',
    'details',
    'summary',
    'figure',
    'figcaption',
    'input',
    'svg',
    'path',
    'circle',
    'rect',
    'line',
    'polyline',
    'g',
  ],
  allowedAttributes: {
    '*': ['class', 'id', 'data-*', 'aria-hidden', 'aria-label'],
    a: ['href', 'title', 'target', 'rel'],
    img: ['src', 'alt', 'title', 'width', 'height'],
    code: ['language'],
    details: ['open'],
    th: ['align'],
    td: ['align'],
    input: ['type', 'checked', 'disabled'],
    span: ['style', 'rn-wrapper'],
    svg: svgAttributes,
    path: ['d', 'fill', 'stroke'],
    circle: ['cx', 'cy', 'r', 'fill', 'stroke'],
    rect: ['x', 'y', 'width', 'height', 'rx', 'fill', 'stroke'],
    line: ['x1', 'y1', 'x2', 'y2', 'stroke', 'stroke-width'],
    polyline: ['points', 'fill', 'stroke'],
    g: ['fill', 'stroke'],
  },
  allowedStyles: { span: katexStyles },
  allowedSchemes: ['http', 'https', 'mailto'],
  allowedSchemesByTag: { img: ['http', 'https'] },
  allowProtocolRelative: false,
  transformTags: {
    a: sanitize.simpleTransform('a', { target: '_blank', rel: 'noopener noreferrer' }, true),
  },
  exclusiveFilter: (frame) => {
    // KaTeX also emits a hidden MathML copy of every formula; keep only the HTML one.
    if (frame.tag === 'span' && /(^|\s)katex-mathml(\s|$)/.test(frame.attribs.class ?? ''))
      return true
    // The only form control worth keeping is the read-only task list checkbox.
    if (frame.tag === 'input' && frame.attribs.type !== 'checkbox') return true
    return false
  },
  // SVG attributes such as viewBox are case sensitive.
  parser: { lowerCaseTags: false, lowerCaseAttributeNames: false },
}

/** Cleans rendered Markdown HTML before it is shown to anyone. */
export function sanitizeHtml(html: string): string {
  return sanitize(html, options)
}
