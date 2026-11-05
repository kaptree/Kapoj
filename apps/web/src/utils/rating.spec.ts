import { describe, expect, it } from 'vitest'

import type { RatingLevel } from '@/api/types'
import { formatDelta, isBlackRed, ratingLevelStyle, splitBlackRedName } from './rating'

function level(overrides: Partial<RatingLevel> = {}): RatingLevel {
  return {
    key: 'cyan',
    name: '青名',
    color: '#03A89E',
    accentColor: '',
    ...overrides,
  }
}

describe('rating utils', () => {
  it('detects black-red level and splits first character', () => {
    const blackRed = level({
      key: 'black_red',
      name: '黑红名',
      color: '#000000',
      accentColor: '#FF0000',
    })

    expect(isBlackRed(blackRed)).toBe(true)
    expect(isBlackRed(level())).toBe(false)

    expect(splitBlackRedName('黑红名')).toEqual({ first: '黑', rest: '红名' })
    expect(splitBlackRedName('😀红名')).toEqual({ first: '😀', rest: '红名' })
    expect(splitBlackRedName('')).toEqual({ first: '', rest: '' })
  })

  it('formats rating deltas with sign', () => {
    expect(formatDelta(16)).toBe('+16')
    expect(formatDelta(-5)).toBe('-5')
    expect(formatDelta(0)).toBe('0')
  })

  it('uses regular level color for non-black-red', () => {
    expect(ratingLevelStyle(level())).toEqual({ color: '#03A89E' })
  })
})
