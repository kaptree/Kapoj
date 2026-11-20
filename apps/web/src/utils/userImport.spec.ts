import { describe, expect, it } from 'vitest'

import { mappingCsv } from './userImport'

describe('mappingCsv', () => {
  it('builds the mapping csv with header and escaping', () => {
    expect(
      mappingCsv([
        {
          legacyId: 6,
          uname: '让,我',
          email: 'a@example.com',
          username: 'hydro_6',
        },
      ]),
    ).toBe('_id,uname,mail,username\n6,"让,我",a@example.com,hydro_6\n')
  })

  it('escapes double quotes', () => {
    expect(
      mappingCsv([
        {
          legacyId: 7,
          uname: 'a"b',
          email: 'b@example.com',
          username: 'hydro_7',
        },
      ]),
    ).toBe('_id,uname,mail,username\n7,"a""b",b@example.com,hydro_7\n')
  })

  it('neutralizes spreadsheet formulas in uname', () => {
    const csv = mappingCsv([
      {
        legacyId: 1,
        uname: '=HYPERLINK("http://evil")',
        email: 'a@example.com',
        username: 'hydro_1',
      },
      { legacyId: 2, uname: '+cmd', email: 'b@example.com', username: 'hydro_2' },
      { legacyId: 3, uname: '-1+1', email: 'c@example.com', username: 'hydro_3' },
      { legacyId: 4, uname: '@SUM(A1)', email: 'd@example.com', username: 'hydro_4' },
    ])
    const lines = csv.split('\n')
    expect(lines[1]).toContain("'=HYPERLINK")
    expect(lines[2]).toContain("'+cmd")
    expect(lines[3]).toContain("'-1+1")
    expect(lines[4]).toContain("'@SUM(A1)")
  })

  it('returns only the header for an empty mapping', () => {
    expect(mappingCsv([])).toBe('_id,uname,mail,username\n')
  })
})
