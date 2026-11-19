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
    reducedMotion: false,
    alt: '',
  },
)

const irisFailed = ref(false)

watch(
  () => props.expression,
  () => {
    irisFailed.value = false
  },
)

const preset = computed(
  () => COMPANION_Q.expressions[props.expression] ?? COMPANION_Q.expressions.neutral,
)

const showClosed = computed(
  () => props.blinking && !props.reducedMotion && preset.value.eye === 'open',
)

const showIrises = computed(
  () => !showClosed.value && preset.value.eye === 'open' && !irisFailed.value,
)

const eyeLayer = computed<CompanionQLayer | null>(() => {
  if (showClosed.value) return COMPANION_Q.layers.eyeClosed
  if (preset.value.eye === 'open') {
    return irisFailed.value ? COMPANION_Q.layers.eyeOpen : null
  }
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
  const x = Math.round(Math.max(-1, Math.min(1, props.lookX)) * 6 * 100) / 100
  const y = Math.round(Math.max(-1, Math.min(1, props.lookY)) * 4 * 100) / 100
  return { transform: `translate(${x}px, ${y}px)` }
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
          @error="irisFailed = true"
        />
        <img
          class="companion-q-avatar__layer companion-q-avatar__iris"
          :src="COMPANION_Q.layers.irisRight.src"
          :style="irisStyle"
          alt=""
          @error="irisFailed = true"
        />
      </template>
      <img
        v-else-if="eyeLayer"
        class="companion-q-avatar__layer"
        :src="eyeLayer.src"
        alt=""
      />
      <img class="companion-q-avatar__layer" :src="mouth.src" alt="" />
      <img
        v-if="blush"
        class="companion-q-avatar__layer"
        :src="blush.src"
        alt=""
      />
      <img v-if="fx" class="companion-q-avatar__layer" :src="fx.src" alt="" />
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
