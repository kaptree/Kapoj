import { describe, expect, it } from 'vitest'

import {
  difficultyBand,
  difficultyLabel,
  difficultyText,
  DIFFICULTIES,
  DIFFICULTY_BANDS,
} from './difficulty'

describe('difficultyBand', () => {
  it('maps 1-100 to five bands', () => {
    expect(difficultyBand(1)).toBe(1)
    expect(difficultyBand(20)).toBe(1)
    expect(difficultyBand(21)).toBe(2)
    expect(difficultyBand(62)).toBe(4)
    expect(difficultyBand(100)).toBe(5)
    expect(difficultyText(62)).toBe('62 · 省选')
  })

  it('returns 0 for anything outside an integer 1-100', () => {
    for (const d of [0, 101, -1, 2.5, Number.NaN]) expect(difficultyBand(d)).toBe(0)
  })
})

describe('difficultyLabel', () => {
  it('names bands 1 to 5', () => {
    expect([1, 2, 3, 4, 5].map(difficultyLabel)).toEqual(['入门', '普及', '提高', '省选', 'NOI'])
  })

  it('falls back for anything else', () => {
    for (const d of [0, 6, -1, 2.5, 62, NaN]) expect(difficultyLabel(d)).toBe('未知')
  })

  it('lists every band for selects', () => {
    expect(DIFFICULTIES).toEqual([
      { value: 1, label: '入门' },
      { value: 2, label: '普及' },
      { value: 3, label: '提高' },
      { value: 4, label: '省选' },
      { value: 5, label: 'NOI' },
    ])
  })
})

describe('DIFFICULTY_BANDS', () => {
  it('covers 1-100 without gaps', () => {
    expect(DIFFICULTY_BANDS).toEqual([
      { value: 1, label: '入门', min: 1, max: 20 },
      { value: 2, label: '普及', min: 21, max: 40 },
      { value: 3, label: '提高', min: 41, max: 60 },
      { value: 4, label: '省选', min: 61, max: 80 },
      { value: 5, label: 'NOI', min: 81, max: 100 },
    ])
    for (let i = 1; i < DIFFICULTY_BANDS.length; i += 1) {
      expect(DIFFICULTY_BANDS[i]!.min).toBe(DIFFICULTY_BANDS[i - 1]!.max + 1)
    }
  })
})

describe('difficultyText', () => {
  it('shows the exact score with its band label', () => {
    expect(difficultyText(1)).toBe('1 · 入门')
    expect(difficultyText(40)).toBe('40 · 普及')
    expect(difficultyText(62)).toBe('62 · 省选')
    expect(difficultyText(100)).toBe('100 · NOI')
  })

  it('falls back for invalid scores', () => {
    for (const d of [0, 101, -1, 2.5, Number.NaN]) {
      expect(difficultyText(d)).toBe('未知')
    }
  })
})
