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

  it('ships the real art pack with all three poses', () => {
    expect(COMPANION_PACK_AVAILABLE).toBe(true)
    expect(Object.keys(COMPANION_ASSETS).sort()).toEqual(['bust', 'chibi', 'full'])
    expect(resolveCompanionAsset('chibi', 'neutral')).toMatchObject({
      src: '/mascot/chibi/neutral@2x.webp',
      width: 512,
      height: 512,
    })
    expect(resolveCompanionAsset('bust', 'pout')?.src).toBe('/mascot/bust/neutral@2x.webp')
    expect(resolveCompanionAsset('full', 'neutral')?.src).toBe('/mascot/full/neutral@2x.webp')
  })
})
