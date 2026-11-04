import sanitize from 'sanitize-html'

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
    'svg',
    'path',
  ],
  allowedAttributes: {
    a: ['href', 'title', 'target', 'rel', 'id'],
    img: ['src', 'alt', 'title', 'width', 'height'],
    code: ['class'],
    pre: ['class'],
    span: ['class', 'style', 'aria-hidden'],
    div: ['class', 'id'],
    th: ['align'],
    td: ['align'],
    h1: ['id'],
    h2: ['id'],
    h3: ['id'],
    h4: ['id'],
    h5: ['id'],
    h6: ['id'],
    svg: ['width', 'height', 'viewBox', 'preserveAspectRatio'],
    path: ['d'],
  },
  allowedStyles: { span: katexStyles },
  allowedSchemes: ['http', 'https', 'mailto'],
  allowedSchemesByTag: { img: ['http', 'https'] },
  allowProtocolRelative: false,
  transformTags: {
    a: sanitize.simpleTransform('a', { target: '_blank', rel: 'noopener noreferrer' }, true),
  },
  // KaTeX also emits a hidden MathML copy of every formula; keep only the HTML one.
  exclusiveFilter: (frame) =>
    frame.tag === 'span' && /(^|\s)katex-mathml(\s|$)/.test(frame.attribs.class ?? ''),
  // SVG attributes such as viewBox are case sensitive.
  parser: { lowerCaseTags: false, lowerCaseAttributeNames: false },
}

/** Cleans rendered Markdown HTML before it is shown to anyone. */
export function sanitizeHtml(html: string): string {
  return sanitize(html, options)
}
