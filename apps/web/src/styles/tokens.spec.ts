/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

// `?raw` imports of CSS are stubbed to an empty string by Vitest's default
// CSS handling, so read the stylesheet directly to assert on its source text.
const tokens = readFileSync(resolve(process.cwd(), 'src/styles/tokens.css'), 'utf8')

const required = [
  '--kapoj-kapo:',
  '--kapoj-kapo-strong:',
  '--kapoj-kapo-text:',
  '--kapoj-kapo-soft:',
  '--kapoj-silver:',
  '--kapoj-silver-strong:',
  '--kapoj-ink:',
  '--kapoj-manga-line:',
  '--kapoj-manga-dot:',
  '--kapoj-sticker-shadow:',
]

describe('companion tokens', () => {
  it('defines every companion token in both themes', () => {
    for (const token of required) {
      expect(tokens.split(token).length - 1).toBeGreaterThanOrEqual(2)
    }
  })
})
