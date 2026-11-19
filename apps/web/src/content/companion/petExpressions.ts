import type { CompanionQExpression } from './types'

/** Weighted pet reactions; neutral is deliberately absent. */
export const PET_EXPRESSION_WEIGHTS: ReadonlyArray<{
  id: CompanionQExpression
  weight: number
}> = [
  { id: 'happy', weight: 30 },
  { id: 'shy', weight: 25 },
  { id: 'pout', weight: 15 },
  { id: 'surprised', weight: 10 },
  { id: 'sleepy', weight: 10 },
  { id: 'smug', weight: 10 },
]

/** Pick a weighted pet expression, never repeating the excluded id. */
export function pickPetExpression(
  random: () => number = Math.random,
  exclude?: CompanionQExpression,
): CompanionQExpression {
  const pool = exclude
    ? PET_EXPRESSION_WEIGHTS.filter((entry) => entry.id !== exclude)
    : PET_EXPRESSION_WEIGHTS
  const total = pool.reduce((sum, entry) => sum + entry.weight, 0)
  let roll = Math.max(0, Math.min(1, random())) * total
  for (const entry of pool) {
    if (roll < entry.weight) return entry.id
    roll -= entry.weight
  }
  return pool[pool.length - 1].id
}
