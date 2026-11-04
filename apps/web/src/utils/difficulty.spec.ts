import { describe, expect, it } from 'vitest'

import { difficultyLabel, DIFFICULTIES } from './difficulty'

describe('difficultyLabel', () => {
  it('names levels 1 to 5', () => {
    expect([1, 2, 3, 4, 5].map(difficultyLabel)).toEqual(['入门', '普及', '提高', '省选', 'NOI'])
  })

  it('falls back for anything else', () => {
    for (const d of [0, 6, -1, 2.5, NaN]) expect(difficultyLabel(d)).toBe('未知')
  })

  it('lists every level for selects', () => {
    expect(DIFFICULTIES).toEqual([
      { value: 1, label: '入门' },
      { value: 2, label: '普及' },
      { value: 3, label: '提高' },
      { value: 4, label: '省选' },
      { value: 5, label: 'NOI' },
    ])
  })
})
