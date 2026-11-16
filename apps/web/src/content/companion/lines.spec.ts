import { describe, expect, it } from 'vitest'

import { COMPANION_LINES, pickCompanionLine } from './lines.zh-CN'

describe('companion lines', () => {
  it('is deterministic for the same key and day', () => {
    const first = pickCompanionLine('greeting', '2026-11-16')
    const second = pickCompanionLine('greeting', '2026-11-16')
    expect(first).toBe(second)
    expect(COMPANION_LINES.greeting).toContain(first)
  })

  it('keeps every line short enough for a bubble', () => {
    for (const lines of Object.values(COMPANION_LINES)) {
      for (const line of lines) expect(Array.from(line).length).toBeLessThanOrEqual(40)
    }
  })
})
