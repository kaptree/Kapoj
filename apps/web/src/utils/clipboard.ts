/**
 * Copy text with a fallback for non-secure contexts. `navigator.clipboard` is
 * only available on HTTPS/localhost; the platform is frequently opened over a
 * plain HTTP LAN address where it is undefined.
 */
export async function copyText(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return
    } catch {
      // Fall through to the legacy path (permission denial, iframe policy,
      // document not focused, etc.).
    }
  }
  copyWithTextarea(text)
}

function copyWithTextarea(text: string): void {
  if (typeof document === 'undefined' || !document.body) {
    throw new Error('clipboard unavailable')
  }

  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', '')
  textarea.setAttribute('aria-hidden', 'true')
  textarea.style.position = 'fixed'
  textarea.style.top = '0'
  textarea.style.left = '-9999px'
  textarea.style.opacity = '0'

  const active = document.activeElement as HTMLElement | null
  const selection = document.getSelection()
  const ranges: Range[] = []
  if (selection) {
    for (let i = 0; i < selection.rangeCount; i += 1) {
      ranges.push(selection.getRangeAt(i).cloneRange())
    }
  }

  document.body.appendChild(textarea)
  try {
    textarea.focus()
    textarea.select()
    textarea.setSelectionRange(0, text.length)
    const ok = typeof document.execCommand === 'function' && document.execCommand('copy')
    if (!ok) throw new Error('copy command was rejected')
  } finally {
    document.body.removeChild(textarea)
    if (selection) {
      selection.removeAllRanges()
      for (const range of ranges) selection.addRange(range)
    }
    active?.focus?.()
  }
}
