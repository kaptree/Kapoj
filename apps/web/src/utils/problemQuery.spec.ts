import { describe, expect, it } from 'vitest'

import { parseProblemQuery, toRouteQuery } from './problemQuery'

describe('parseProblemQuery', () => {
  it('returns defaults for an empty query', () => {
    expect(parseProblemQuery({})).toEqual({
      keyword: '',
      tag: '',
      difficultyMin: null,
      difficultyMax: null,
      scope: '',
      page: 1,
    })
  })

  it('reads valid values', () => {
    expect(
      parseProblemQuery({
        keyword: 'sum',
        tag: 'math',
        difficultyMin: '40',
        difficultyMax: '80',
        scope: 'mine',
        page: '4',
      }),
    ).toEqual({
      keyword: 'sum',
      tag: 'math',
      difficultyMin: 40,
      difficultyMax: 80,
      scope: 'mine',
      page: 4,
    })
  })

  it('parses a range query', () => {
    expect(parseProblemQuery({ difficultyMin: '40', difficultyMax: '80' })).toMatchObject({
      difficultyMin: 40,
      difficultyMax: 80,
    })
  })

  it('parses one-sided ranges', () => {
    expect(parseProblemQuery({ difficultyMin: '40' })).toMatchObject({
      difficultyMin: 40,
      difficultyMax: null,
    })
    expect(parseProblemQuery({ difficultyMax: '40' })).toMatchObject({
      difficultyMin: null,
      difficultyMax: 40,
    })
  })

  it('swaps a reversed range', () => {
    expect(parseProblemQuery({ difficultyMin: '80', difficultyMax: '40' })).toMatchObject({
      difficultyMin: 40,
      difficultyMax: 80,
    })
  })

  it('falls back on invalid numbers', () => {
    for (const page of ['abc', '0', '-3', '1.5', '']) {
      expect(parseProblemQuery({ page }).page).toBe(1)
    }
    for (const difficulty of ['0', '101', 'abc', '2.5', '']) {
      expect(parseProblemQuery({ difficultyMin: difficulty }).difficultyMin).toBeNull()
      expect(parseProblemQuery({ difficultyMax: difficulty }).difficultyMax).toBeNull()
    }
  })

  it('takes the first value of repeated parameters', () => {
    expect(
      parseProblemQuery({
        keyword: ['a', 'b'],
        difficultyMin: ['20', '40'],
        page: ['2', '3'],
      }),
    ).toMatchObject({
      keyword: 'a',
      difficultyMin: 20,
      page: 2,
    })
  })

  it('ignores null and non-string values', () => {
    expect(parseProblemQuery({ keyword: null, tag: 5, difficultyMin: null })).toEqual({
      keyword: '',
      tag: '',
      difficultyMin: null,
      difficultyMax: null,
      scope: '',
      page: 1,
    })
  })
})

describe('toRouteQuery', () => {
  it('omits default values', () => {
    expect(
      toRouteQuery({
        keyword: '',
        tag: '',
        difficultyMin: null,
        difficultyMax: null,
        scope: '',
        page: 1,
      }),
    ).toEqual({})
  })

  it('writes everything else as strings', () => {
    expect(
      toRouteQuery({
        keyword: 'sum',
        tag: 'math',
        difficultyMin: 40,
        difficultyMax: 80,
        scope: 'all',
        page: 2,
      }),
    ).toEqual({
      keyword: 'sum',
      tag: 'math',
      difficultyMin: '40',
      difficultyMax: '80',
      scope: 'all',
      page: '2',
    })
  })

  it('writes reversed ranges in ascending order', () => {
    const base = { keyword: '', tag: '', scope: '' as const, page: 1 }
    expect(toRouteQuery({ ...base, difficultyMin: 80, difficultyMax: 40 })).toEqual({
      difficultyMin: '40',
      difficultyMax: '80',
    })
  })

  it('omits a cleared range even when it arrives as undefined or empty', () => {
    const base = { keyword: '', tag: '', scope: '' as const, page: 1 }
    expect(
      toRouteQuery({
        ...base,
        difficultyMin: undefined as unknown as null,
        difficultyMax: '' as unknown as null,
      }),
    ).toEqual({})
    expect(
      toRouteQuery({
        ...base,
        difficultyMin: 0 as unknown as null,
        difficultyMax: 101 as unknown as null,
      }),
    ).toEqual({})
  })

  it('round-trips through parse', () => {
    const q = {
      keyword: 'a b',
      tag: '图论',
      difficultyMin: 40,
      difficultyMax: 80,
      scope: 'mine' as const,
      page: 7,
    }
    expect(parseProblemQuery(toRouteQuery(q))).toEqual(q)
  })
})
