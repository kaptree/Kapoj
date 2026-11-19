import { describe, expect, it } from 'vitest'
import {
  COMPANION_ASSETS,
  COMPANION_OUTFITS,
  COMPANION_PACK_AVAILABLE,
  resolveCompanionAsset,
  resolveCompanionOutfit,
} from './manifest'
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

describe('companion outfits', () => {
  it('maps five outfits with 3:4 sources and level order', () => {
    expect(COMPANION_OUTFITS.map((outfit) => outfit.id)).toEqual([
      'outfit-1',
      'outfit-2',
      'outfit-3',
      'outfit-4',
      'outfit-5',
    ])
    expect(COMPANION_OUTFITS.map((outfit) => outfit.level)).toEqual([0, 1, 2, 4, 6])
    for (const outfit of COMPANION_OUTFITS) {
      expect(outfit.src).toBe(`/companion/outfits/${outfit.id}.webp`)
      expect(outfit.card).toBe(`/companion/outfits/${outfit.id}-card.webp`)
      expect(outfit.width).toBe(768)
      expect(outfit.height).toBe(1024)
    }
  })

  it('falls back to outfit-1 for empty or unknown ids', () => {
    expect(resolveCompanionOutfit('')).toBe(COMPANION_OUTFITS[0])
    expect(resolveCompanionOutfit('missing')).toBe(COMPANION_OUTFITS[0])
  })
})
