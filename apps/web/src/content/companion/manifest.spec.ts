import { describe, expect, it } from 'vitest'
import { COMPANION_ASSETS, COMPANION_PACK_AVAILABLE, resolveCompanionAsset } from './manifest'
import type { CompanionAssetPack } from './types'

const pack: CompanionAssetPack = {
  chibi: {
    neutral: { src: '/mascot/chibi/neutral@2x.webp', width: 512, height: 512 },
    pout: { src: '/mascot/chibi/pout@2x.webp', width: 512, height: 512 },
  },
}

describe('resolveCompanionAsset', () => {
  it('falls back to neutral for an unknown expression', () => {
    expect(resolveCompanionAsset('chibi', 'panic', pack)?.src).toContain('neutral')
  })

  it('returns null when the pose is missing', () => {
    expect(resolveCompanionAsset('full', 'neutral', pack)).toBeNull()
  })

  it('ships an empty pack so the placeholder renders until art arrives', () => {
    expect(COMPANION_PACK_AVAILABLE).toBe(false)
    expect(Object.keys(COMPANION_ASSETS)).toHaveLength(0)
    expect(resolveCompanionAsset('chibi', 'neutral')).toBeNull()
  })
})
