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
