import { describe, expect, it } from 'vitest'

import { formatBytes, formatDateTime } from './format'

describe('formatBytes', () => {
  it('formats sizes with binary units', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(1023)).toBe('1023 B')
    expect(formatBytes(1024)).toBe('1.0 KiB')
    expect(formatBytes(1536)).toBe('1.5 KiB')
    expect(formatBytes(16 * 1024 * 1024)).toBe('16.0 MiB')
  })

  it('handles invalid input', () => {
    expect(formatBytes(-1)).toBe('-')
    expect(formatBytes(NaN)).toBe('-')
  })
})

describe('formatDateTime', () => {
  it('formats ISO timestamps and handles empty values', () => {
    expect(formatDateTime(null)).toBe('-')
    expect(formatDateTime('not-a-date')).toBe('-')
    expect(formatDateTime('2026-11-04T12:00:00Z')).not.toBe('-')
  })
})
