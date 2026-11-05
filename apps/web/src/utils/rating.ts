import type { RatingLevel } from '@/api/types'

/** Fixed accent used when the server does not provide a black-red accent. */
export const BLACK_RED_ACCENT_COLOR = '#FF0000'

export function isBlackRed(level: RatingLevel): boolean {
  return level.key === 'black_red'
}

/** Colors come only from the server-provided level. */
export function ratingLevelStyle(level: RatingLevel): { color: string } {
  return { color: level.color }
}

/** Returns the first code point and the remaining code points of a name. */
export function splitBlackRedName(name: string): { first: string; rest: string } {
  const [first = '', ...rest] = Array.from(name)
  return { first, rest: rest.join('') }
}

export function formatDelta(delta: number): string {
  if (!Number.isFinite(delta)) return '0'
  const value = Math.trunc(delta)
  return value > 0 ? `+${value}` : `${value}`
}
