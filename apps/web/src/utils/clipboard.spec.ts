import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { copyText } from './clipboard'

const originalClipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
const originalExecCommand = document.execCommand

function setClipboard(value: unknown) {
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value,
  })
}

beforeEach(() => {
  setClipboard(undefined)
  document.execCommand = vi.fn(() => true)
})

afterEach(() => {
  if (originalClipboard) {
    Object.defineProperty(navigator, 'clipboard', originalClipboard)
  } else {
    delete (navigator as { clipboard?: Clipboard }).clipboard
  }
  document.execCommand = originalExecCommand
  document.body.innerHTML = ''
})

describe('copyText', () => {
  it('uses the async clipboard API when available', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    setClipboard({ writeText })

    await copyText('hello')

    expect(writeText).toHaveBeenCalledWith('hello')
    expect(document.execCommand).not.toHaveBeenCalled()
  })

  it('falls back to a hidden textarea without a clipboard API', async () => {
    await copyText('plain http')

    expect(document.execCommand).toHaveBeenCalledWith('copy')
    expect(document.querySelector('textarea')).toBeNull()
  })

  it('falls back when the clipboard API rejects', async () => {
    setClipboard({ writeText: vi.fn().mockRejectedValue(new Error('denied')) })

    await copyText('permission denied')

    expect(document.execCommand).toHaveBeenCalledWith('copy')
  })

  it('throws when the legacy copy command fails', async () => {
    document.execCommand = vi.fn(() => false)

    await expect(copyText('nope')).rejects.toThrow('copy command was rejected')
  })
})
