import { describe, expect, it } from 'vitest'

import { localDateKey } from './date'

describe('localDateKey', () => {
  it('uses local calendar fields, not UTC', () => {
    const date = new Date(2026, 10, 16, 23, 30)
    expect(localDateKey(date)).toBe('2026-11-16')
  })
})
