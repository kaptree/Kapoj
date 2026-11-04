import { describe, expect, it } from 'vitest'

import {
  contestCountdown,
  contestModeLabel,
  contestStatusLabel,
  contestStatusTagType,
  formatDuration,
  registerTypeLabel,
} from './contest'

describe('contest utils', () => {
  it('maps labels and tag types', () => {
    expect(contestStatusLabel('running')).toBe('进行中')
    expect(contestStatusTagType('running')).toBe('success')
    expect(contestModeLabel('icpc')).toBe('ICPC')
    expect(contestModeLabel('oi')).toBe('OI')
    expect(registerTypeLabel('invite')).toBe('邀请码报名')
  })

  it('formats durations', () => {
    expect(formatDuration(0)).toBe('00:00:00')
    expect(formatDuration(65_000)).toBe('00:01:05')
    expect(formatDuration(90_000_000)).toBe('1天 01:00:00')
  })

  it('counts down against the captured server skew', () => {
    const now = Date.parse('2026-01-01T00:00:00Z')
    const start = '2026-01-01T01:00:00Z'
    const end = '2026-01-01T02:00:00Z'

    expect(contestCountdown(start, end, 0, now)?.phase).toBe('upcoming')
    expect(contestCountdown(start, end, 0, now)?.text).toBe('距开始 01:00:00')
    expect(contestCountdown(start, end, 0, now + 60_000)?.text).toBe('距开始 00:59:00')
    expect(contestCountdown(start, end, 0, now + 90 * 60_000)?.text).toBe('距结束 00:30:00')
    expect(contestCountdown(start, end, 0, now + 120 * 60_000)?.phase).toBe('ended')
  })

  it('applies a server clock skew', () => {
    const now = Date.parse('2026-01-01T00:00:00Z')
    expect(
      contestCountdown('2026-01-01T01:00:00Z', '2026-01-01T02:00:00Z', 30 * 60_000, now)?.text,
    ).toBe('距开始 00:30:00')
  })

  it('returns null for invalid times', () => {
    expect(contestCountdown('bad', '2026-01-01T02:00:00Z', 0, 0)).toBeNull()
  })
})
