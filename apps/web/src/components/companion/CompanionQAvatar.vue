<script setup lang="ts">
import { computed, ref, watch } from 'vue'

import { COMPANION_Q } from '../../content/companion/manifest'
import type { CompanionQLayer, CompanionQExpression } from '../../content/companion/types'

type AvatarFrame = 'dock' | 'full'

const props = withDefaults(
  defineProps<{
    expression?: CompanionQExpression
    lookX?: number
    lookY?: number
    blinking?: boolean
    frame?: AvatarFrame
    size?: number
    reducedMotion?: boolean
    alt?: string
  }>(),
  {
    expression: 'neutral',
    lookX: 0,
    lookY: 0,
    blinking: false,
    frame: 'full',
    size: 0,
    reducedMotion: false,
    alt: '',
  },
)

const irisFailed = ref(false)
const eyeFallback = ref<CompanionQLayer | null>(null)
const eyeOpenFailed = ref(false)
const mouthFailed = ref(false)
const blushFailed = ref(false)
const fxFailed = ref(false)

watch(
  () => props.expression,
  () => {
    irisFailed.value = false
    eyeFallback.value = null
    eyeOpenFailed.value = false
    mouthFailed.value = false
    blushFailed.value = false
    fxFailed.value = false
  },
)

function onEyeError(layer: CompanionQLayer | null): void {
  if (!layer) return
  if (layer.src === COMPANION_Q.layers.eyeOpen.src) {
    eyeOpenFailed.value = true
    eyeFallback.value = null
    return
  }
  eyeFallback.value = COMPANION_Q.layers.eyeOpen
}

function onIrisError(): void {
  irisFailed.value = true
}

const preset = computed(
  () => COMPANION_Q.expressions[props.expression] ?? COMPANION_Q.expressions.neutral,
)

const showClosed = computed(
  () => props.blinking && !props.reducedMotion && preset.value.eye === 'open',
)

const showIrises = computed(
  () => !showClosed.value && preset.value.eye === 'open' && !irisFailed.value && !eyeFallback.value,
)

const eyeLayer = computed<CompanionQLayer | null>(() => {
  if (eyeFallback.value) return eyeOpenFailed.value ? null : eyeFallback.value
  if (showClosed.value) return eyeOpenFailed.value ? null : COMPANION_Q.layers.eyeClosed
  if (preset.value.eye === 'open') {
    return irisFailed.value && !eyeOpenFailed.value ? COMPANION_Q.layers.eyeOpen : null
  }
  if (eyeOpenFailed.value) return null
  if (preset.value.eye === 'closed') return COMPANION_Q.layers.eyeClosed
  if (preset.value.eye === 'happy') return COMPANION_Q.layers.eyes.happy
  return COMPANION_Q.layers.eyes[preset.value.eye]
})

const mouth = computed(() => COMPANION_Q.layers.mouths[preset.value.mouth])
const blush = computed(() =>
  preset.value.blush === 'none' ? null : COMPANION_Q.layers.blush[preset.value.blush],
)
const fx = computed(() =>
  preset.value.fx === 'none' ? null : COMPANION_Q.layers.fx[preset.value.fx],
)

const viewportStyle = computed(() => {
  if (props.frame !== 'dock') {
    return { width: '100%', height: '100%', left: '0', top: '0' }
  }
  const viewport = COMPANION_Q.dockViewport
  const width = COMPANION_Q.canvas.width
  const height = COMPANION_Q.canvas.height
  return {
    width: `${(width / viewport.width) * 100}%`,
    height: `${(height / viewport.height) * 100}%`,
    left: `${(-viewport.x / viewport.width) * 100}%`,
    top: `${(-viewport.y / viewport.height) * 100}%`,
  }
})

const rootStyle = computed(() => ({
  width: props.size ? `${props.size}px` : '100%',
  height: props.size ? `${props.size}px` : '100%',
}))

const irisStyle = computed(() => {
  if (props.reducedMotion) return { transform: 'none' }
  // The iris layer is the full 768 canvas. 1% of its width is ~1% of the eye
  // width times 6, matching the spec's ±6% horizontal / ±4% vertical budget at
  // both dock and full-frame sizes.
  const x = Math.round(Math.max(-1, Math.min(1, props.lookX)) * 100) / 100
  const y = Math.round(Math.max(-1, Math.min(1, props.lookY)) * 70) / 100
  return { transform: `translate(${x}%, ${y}%)` }
})

const accessibleLabel = computed(() => props.alt.trim())
</script>

<template>
  <div
    class="companion-q-avatar"
    :data-expression="expression"
    :style="rootStyle"
    role="img"
    :aria-label="accessibleLabel || undefined"
    :aria-hidden="accessibleLabel ? undefined : 'true'"
  >
    <div class="companion-q-avatar__viewport" :style="viewportStyle">
      <img class="companion-q-avatar__layer" :src="COMPANION_Q.layers.base.src" alt="" />
      <template v-if="showIrises">
        <img
          class="companion-q-avatar__layer companion-q-avatar__iris"
          :src="COMPANION_Q.layers.irisLeft.src"
          :style="irisStyle"
          alt=""
          @error="onIrisError"
        />
        <img
          class="companion-q-avatar__layer companion-q-avatar__iris"
          :src="COMPANION_Q.layers.irisRight.src"
          :style="irisStyle"
          alt=""
          @error="onIrisError"
        />
      </template>
      <img
        v-else-if="eyeLayer"
        class="companion-q-avatar__layer"
        :src="eyeLayer.src"
        alt=""
        @error="onEyeError(eyeLayer)"
      />
      <img
        v-if="!mouthFailed"
        class="companion-q-avatar__layer"
        :src="mouth.src"
        alt=""
        @error="mouthFailed = true"
      />
      <img
        v-if="blush && !blushFailed"
        class="companion-q-avatar__layer"
        :src="blush.src"
        alt=""
        @error="blushFailed = true"
      />
      <img
        v-if="fx && !fxFailed"
        class="companion-q-avatar__layer"
        :src="fx.src"
        alt=""
        @error="fxFailed = true"
      />
    </div>
  </div>
</template>

<style scoped>
.companion-q-avatar {
  position: relative;
  display: block;
  aspect-ratio: 1;
  overflow: hidden;
}

.companion-q-avatar__viewport {
  position: absolute;
}

.companion-q-avatar__layer {
  position: absolute;
  inset: 0;
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
  pointer-events: none;
  user-select: none;
  -webkit-user-drag: none;
}

.companion-q-avatar__iris {
  will-change: transform;
}
</style>
