import { describe, expect, it } from 'vitest'

import { COMPANION_LINES, pickCompanionLine } from './lines.zh-CN'
import { pickPetExpression } from './petExpressions'

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

describe('pet expressions', () => {
  it('has 3-5 lines for every new reaction key', () => {
    const keys = [
      'pet.happy',
      'pet.shy',
      'pet.pout',
      'pet.surprised',
      'pet.sleepy',
      'pet.smug',
      'pet.already',
      'greet.morning',
      'greet.afternoon',
      'greet.night',
      'idle.lively',
    ] as const
    for (const key of keys) {
      expect(COMPANION_LINES[key].length).toBeGreaterThanOrEqual(3)
      expect(COMPANION_LINES[key].length).toBeLessThanOrEqual(5)
      for (const line of COMPANION_LINES[key]) expect(line.trim()).not.toBe('')
    }
  })

  it('can exclude the previous line', () => {
    const previous = COMPANION_LINES['pet.happy'][0]
    expect(pickCompanionLine('pet.happy', 'seed', previous)).not.toBe(previous)
  })

  it('picks weighted expressions and avoids the excluded one', () => {
    expect(pickPetExpression(() => 0, 'happy')).not.toBe('happy')
    const seen = new Set<string>()
    for (const roll of [0, 0.3, 0.55, 0.7, 0.8, 0.9, 0.999]) {
      seen.add(pickPetExpression(() => roll))
    }
    expect(seen).toEqual(new Set(['happy', 'shy', 'pout', 'surprised', 'sleepy', 'smug']))
  })
})
