export interface DifficultyBand {
  value: number
  label: string
  min: number
  max: number
}

/** The five display bands of the 1-100 difficulty scale. */
export const DIFFICULTY_BANDS: readonly DifficultyBand[] = [
  { value: 1, label: '入门', min: 1, max: 20 },
  { value: 2, label: '普及', min: 21, max: 40 },
  { value: 3, label: '提高', min: 41, max: 60 },
  { value: 4, label: '省选', min: 61, max: 80 },
  { value: 5, label: 'NOI', min: 81, max: 100 },
]

/**
 * Band options for selects.
 * @deprecated difficulty is a 1-100 score; use `DIFFICULTY_BANDS` and an exact input instead.
 */
export const DIFFICULTIES = DIFFICULTY_BANDS.map(({ value, label }) => ({ value, label }))

/** Maps an exact 1-100 score to its band 1-5, or 0 when out of range. */
export function difficultyBand(d: number): number {
  if (!Number.isInteger(d) || d < 1 || d > 100) return 0
  return Math.ceil(d / 20)
}

/** Returns the label of a band 1-5, or 未知 for anything else. */
export function difficultyLabel(d: number): string {
  return DIFFICULTY_BANDS.find((band) => band.value === d)?.label ?? '未知'
}

/** Renders an exact score with its band, e.g. `62 · 省选`. */
export function difficultyText(d: number): string {
  const band = difficultyBand(d)
  return band === 0 ? '未知' : `${d} · ${difficultyLabel(band)}`
}
