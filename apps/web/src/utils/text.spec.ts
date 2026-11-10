import { describe, expect, it } from 'vitest'

import { utf8Offset } from './text'

describe('utf8Offset', () => {
  it('counts ASCII, Chinese and surrogate pairs as UTF-8 bytes', () => {
    expect(utf8Offset('abc', 2)).toBe(2)
    expect(utf8Offset('中a', 1)).toBe(3)
    expect(utf8Offset('😀a', 2)).toBe(4)
    expect(utf8Offset('a😀b', 3)).toBe(5)
  })
})
