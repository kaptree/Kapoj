export type CompanionPose = 'chibi' | 'bust' | 'full'
export type CompanionExpression =
  | 'neutral'
  | 'smug'
  | 'pout'
  | 'blush'
  | 'surprised'
  | 'annoyed'
  | 'thinking'
  | 'sleepy'
  | 'victory'
  | 'panic'
  | 'lost'

export interface CompanionAsset {
  src: string
  width: number
  height: number
}

export type CompanionAssetPack = Partial<
  Record<CompanionPose, Partial<Record<CompanionExpression, CompanionAsset>>>
>

/** One wearable full-body outfit. Levels match the companion bond table. */
export interface CompanionOutfit {
  id: string
  name: string
  level: number
  src: string
  card: string
  width: number
  height: number
}
