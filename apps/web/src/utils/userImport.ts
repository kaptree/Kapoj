import type { AdminUserImportMapping } from '@/api/types'

/** Build the downloadable old-ID → new-username CSV for generated users. */
export function mappingCsv(rows: readonly AdminUserImportMapping[]): string {
  const escape = (value: string) =>
    /[",\n\r]/.test(value) ? `"${value.replaceAll('"', '""')}"` : value
  const header = '_id,uname,mail,username\n'
  const body = rows
    .map((row) =>
      [String(row.legacyId), escape(row.uname), escape(row.email), escape(row.username)].join(','),
    )
    .join('\n')
  return header + body + (rows.length > 0 ? '\n' : '')
}
