export interface ProblemQuery {
  keyword: string
  tag: string
  difficulty: number | null
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

/** Reads the problem list filters from a route query, falling back to defaults. */
export function parseProblemQuery(q: Record<string, unknown>): ProblemQuery {
  const page = integer(q.page)
  const difficulty = integer(q.difficulty)
  return {
    keyword: first(q.keyword),
    tag: first(q.tag),
    difficulty: difficulty !== null && difficulty >= 1 && difficulty <= 5 ? difficulty : null,
    page: page !== null && page >= 1 ? page : 1,
  }
}

/** Builds a route query that leaves out default values. */
export function toRouteQuery(q: ProblemQuery): Record<string, string> {
  const out: Record<string, string> = {}
  if (q.keyword) out.keyword = q.keyword
  if (q.tag) out.tag = q.tag
  if (typeof q.difficulty === 'number') out.difficulty = String(q.difficulty)
  if (q.page > 1) out.page = String(q.page)
  return out
}
