import { describe, expect, it } from 'vitest'

import { parseProblemQuery, toRouteQuery } from './problemQuery'

describe('parseProblemQuery', () => {
  it('returns defaults for an empty query', () => {
    expect(parseProblemQuery({})).toEqual({ keyword: '', tag: '', difficulty: null, page: 1 })
  })

  it('reads valid values', () => {
    expect(parseProblemQuery({ keyword: 'sum', tag: 'math', difficulty: '3', page: '4' })).toEqual({
      keyword: 'sum',
      tag: 'math',
      difficulty: 3,
      page: 4,
    })
  })

  it('falls back on invalid numbers', () => {
    for (const page of ['abc', '0', '-3', '1.5', '']) {
      expect(parseProblemQuery({ page }).page).toBe(1)
    }
    for (const difficulty of ['9', '0', 'abc', '2.5', '']) {
      expect(parseProblemQuery({ difficulty }).difficulty).toBeNull()
    }
  })

  it('takes the first value of repeated parameters', () => {
    expect(parseProblemQuery({ keyword: ['a', 'b'], page: ['2', '3'] })).toMatchObject({
      keyword: 'a',
      page: 2,
    })
  })

  it('ignores null and non-string values', () => {
    expect(parseProblemQuery({ keyword: null, tag: 5, page: null })).toEqual({
      keyword: '',
      tag: '',
      difficulty: null,
      page: 1,
    })
  })
})

describe('toRouteQuery', () => {
  it('omits default values', () => {
    expect(toRouteQuery({ keyword: '', tag: '', difficulty: null, page: 1 })).toEqual({})
  })

  it('writes everything else as strings', () => {
    expect(toRouteQuery({ keyword: 'sum', tag: 'math', difficulty: 3, page: 2 })).toEqual({
      keyword: 'sum',
      tag: 'math',
      difficulty: '3',
      page: '2',
    })
  })

  it('omits a cleared difficulty even when it arrives as undefined or empty', () => {
    const base = { keyword: '', tag: '', page: 1 }
    expect(toRouteQuery({ ...base, difficulty: undefined as unknown as null })).toEqual({})
    expect(toRouteQuery({ ...base, difficulty: '' as unknown as null })).toEqual({})
  })

  it('round-trips through parse', () => {
    const q = { keyword: 'a b', tag: '图论', difficulty: 4, page: 7 }
    expect(parseProblemQuery(toRouteQuery(q))).toEqual(q)
  })
})
