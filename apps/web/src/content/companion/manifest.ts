import type {
  CompanionAsset,
  CompanionAssetPack,
  CompanionExpression,
  CompanionPose,
} from './types'

/** Filled in when the user supplies real art; empty until then. */
export const COMPANION_ASSETS: CompanionAssetPack = {}

/** Keeps the placeholder rendering for as long as the pack has no art. */
export const COMPANION_PACK_AVAILABLE = false

export function resolveCompanionAsset(
  pose: CompanionPose,
  expression: CompanionExpression,
  pack: CompanionAssetPack = COMPANION_ASSETS,
): CompanionAsset | null {
  return pack[pose]?.[expression] ?? pack[pose]?.neutral ?? null
}
