import assets from './assets.json'
import type {
  CompanionAsset,
  CompanionAssetPack,
  CompanionExpression,
  CompanionOutfit,
  CompanionPose,
} from './types'

/**
 * Derived from `public/sucai.png`: a square head crop, a 6:7 head-to-waist
 * crop, and the full-body 3:4 art. Expressions currently fall back to
 * `neutral`; add per-expression files when the expression pack arrives.
 */
export const COMPANION_ASSETS: CompanionAssetPack = {
  chibi: {
    neutral: { src: '/mascot/chibi/neutral@2x.webp', width: 512, height: 512 },
  },
  bust: {
    neutral: { src: '/mascot/bust/neutral@2x.webp', width: 640, height: 747 },
  },
  full: {
    neutral: { src: '/mascot/full/neutral@2x.webp', width: 1000, height: 1333 },
  },
}

export const COMPANION_PACK_AVAILABLE = true

export function resolveCompanionAsset(
  pose: CompanionPose,
  expression: CompanionExpression,
  pack: CompanionAssetPack = COMPANION_ASSETS,
): CompanionAsset | null {
  if (!COMPANION_PACK_AVAILABLE) return null
  return pack[pose]?.[expression] ?? pack[pose]?.neutral ?? null
}


/** The five wearable portraits, in display/unlock order. */
export const COMPANION_OUTFITS: readonly CompanionOutfit[] = assets.outfits

/** Resolve an outfit id; unknown and empty ids fall back to outfit-1. */
export function resolveCompanionOutfit(id: string): CompanionOutfit {
  return COMPANION_OUTFITS.find((outfit) => outfit.id === id) ?? COMPANION_OUTFITS[0]
}
