export type ProblemScope = 'public' | 'mine' | 'all'

export interface ProblemQuery {
  keyword: string
  tag: string
  difficultyMin: number | null
  difficultyMax: number | null
  scope: ProblemScope | ''
  page: number
}

function first(v: unknown): string {
  const x = Array.isArray(v) ? v[0] : v
  return typeof x === 'string' ? x : ''
}

function integer(v: unknown): number | null {
  const s = first(v)
  return /^-?\d+$/.test(s) ? Number(s) : null
}

/** Keeps only integer scores on the 1-100 difficulty scale. */
function difficulty(v: unknown): number | null {
  const n = integer(v)
  return n !== null && n >= 1 && n <= 100 ? n : null
}

/** Keeps only integer scores on the 1-100 difficulty scale. */
function validDifficulty(n: number | null): number | null {
  return typeof n === 'number' && Number.isInteger(n) && n >= 1 && n <= 100 ? n : null
}

function ascendingRange(min: number | null, max: number | null): [number | null, number | null] {
  return min !== null && max !== null && min > max ? [max, min] : [min, max]
}

/** Reads the problem list filters from a route query, falling back to defaults. */
export function parseProblemQuery(q: Record<string, unknown>): ProblemQuery {
  const page = integer(q.page)
  const [difficultyMin, difficultyMax] = ascendingRange(
    difficulty(q.difficultyMin),
    difficulty(q.difficultyMax),
  )
  const rawScope = first(q.scope)
  const scope: ProblemQuery['scope'] =
    rawScope === 'public' || rawScope === 'mine' || rawScope === 'all' ? rawScope : ''
  return {
    keyword: first(q.keyword),
    tag: first(q.tag),
    difficultyMin,
    difficultyMax,
    scope,
    page: page !== null && page >= 1 ? page : 1,
  }
}

/** Builds a route query that leaves out default values. */
export function toRouteQuery(q: ProblemQuery): Record<string, string> {
  const out: Record<string, string> = {}
  const [difficultyMin, difficultyMax] = ascendingRange(
    validDifficulty(q.difficultyMin),
    validDifficulty(q.difficultyMax),
  )
  if (q.keyword) out.keyword = q.keyword
  if (q.tag) out.tag = q.tag
  if (difficultyMin !== null) out.difficultyMin = String(difficultyMin)
  if (difficultyMax !== null) out.difficultyMax = String(difficultyMax)
  if (q.scope) out.scope = q.scope
  if (q.page > 1) out.page = String(q.page)
  return out
}
