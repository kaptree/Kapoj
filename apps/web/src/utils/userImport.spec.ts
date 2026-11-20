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

  it('returns only the header for an empty mapping', () => {
    expect(mappingCsv([])).toBe('_id,uname,mail,username\n')
  })
})
