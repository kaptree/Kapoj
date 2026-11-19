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

/** The seven Q-avatar reactions used by the dock. */
export type CompanionQExpression =
  | 'neutral'
  | 'happy'
  | 'shy'
  | 'pout'
  | 'surprised'
  | 'sleepy'
  | 'smug'

export type CompanionEyeState = 'open' | 'closed' | 'happy' | 'wide' | 'half' | 'teary' | 'angry'
export type CompanionMouthState = 'neutral' | 'smile' | 'open' | 'pout' | 'wavy' | 'o'
export type CompanionBlushState = 'none' | 'soft' | 'strong'
export type CompanionFxState = 'none' | 'sparkle' | 'heart' | 'sweat' | 'anger' | 'zzz'

export interface CompanionQLayer {
  src: string
  width: number
  height: number
}

export interface CompanionExpressionPreset {
  eye: CompanionEyeState
  mouth: CompanionMouthState
  blush: CompanionBlushState
  fx: CompanionFxState
}

export interface CompanionQAssets {
  canvas: { width: number; height: number }
  dockViewport: { x: number; y: number; width: number; height: number }
  layers: {
    base: CompanionQLayer
    irisLeft: CompanionQLayer
    irisRight: CompanionQLayer
    eyeOpen: CompanionQLayer
    eyeClosed: CompanionQLayer
    eyes: Record<'happy' | 'wide' | 'half' | 'teary' | 'angry', CompanionQLayer>
    mouths: Record<CompanionMouthState, CompanionQLayer>
    blush: Record<'soft' | 'strong', CompanionQLayer>
    fx: Record<'sparkle' | 'heart' | 'sweat' | 'anger' | 'zzz', CompanionQLayer>
  }
  expressions: Record<CompanionQExpression, CompanionExpressionPreset>
}
