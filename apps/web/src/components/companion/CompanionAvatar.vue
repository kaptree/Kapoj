<script setup lang="ts">
import { computed, ref, watch } from 'vue'

import { resolveCompanionAsset } from '../../content/companion/manifest'
import type { CompanionExpression, CompanionPose } from '../../content/companion/types'

type AvatarSize = 'xs' | 'sm' | 'md' | 'lg'

const props = withDefaults(
  defineProps<{
    pose?: CompanionPose
    expression?: CompanionExpression
    size?: AvatarSize
    eager?: boolean
    alt?: string
  }>(),
  { pose: 'chibi', expression: 'neutral', size: 'md', eager: false, alt: '' },
)

const ASPECT_RATIOS: Record<CompanionPose, string> = {
  chibi: '1 / 1',
  bust: '6 / 7',
  full: '2 / 3',
}

/**
 * Abstract silhouette, not a character: a silver head, ink shoulders and two
 * crimson eyes. Each pose has its own viewBox so the drawing fills the frame
 * without stretching when real art replaces it.
 */
const PLACEHOLDER_GEOMETRY: Record<
  CompanionPose,
  {
    viewBox: string
    head: { cx: number; cy: number; r: number }
    eyes: { cx: number; cy: number }[]
    eyeRadius: number
    shoulders: string
  }
> = {
  chibi: {
    viewBox: '0 0 100 100',
    head: { cx: 50, cy: 42, r: 26 },
    eyes: [
      { cx: 41, cy: 42 },
      { cx: 59, cy: 42 },
    ],
    eyeRadius: 3,
    shoulders: 'M14 100 C18 72 32 64 50 64 C68 64 82 72 86 100 Z',
  },
  bust: {
    viewBox: '0 0 60 70',
    head: { cx: 30, cy: 26, r: 15 },
    eyes: [
      { cx: 24.5, cy: 26 },
      { cx: 35.5, cy: 26 },
    ],
    eyeRadius: 2,
    shoulders: 'M6 70 C9 50 18 45 30 45 C42 45 51 50 54 70 Z',
  },
  full: {
    viewBox: '0 0 60 90',
    head: { cx: 30, cy: 21, r: 11.5 },
    eyes: [
      { cx: 25.8, cy: 21 },
      { cx: 34.2, cy: 21 },
    ],
    eyeRadius: 1.7,
    shoulders: 'M11 90 C12 52 19 41 30 41 C41 41 48 52 49 90 Z',
  },
}

const asset = computed(() => resolveCompanionAsset(props.pose, props.expression))
const placeholder = computed(() => PLACEHOLDER_GEOMETRY[props.pose])
const aspectRatio = computed(() => ASPECT_RATIOS[props.pose])

const failed = ref(false)

watch(asset, () => {
  failed.value = false
})
</script>

<template>
  <span
    class="companion-avatar"
    :class="[`companion-avatar--${pose}`, `companion-avatar--${size}`]"
    :style="{ aspectRatio }"
  >
    <img
      v-if="asset && !failed"
      class="companion-avatar__image"
      :src="asset.src"
      :width="asset.width"
      :height="asset.height"
      :alt="alt"
      :loading="eager ? 'eager' : 'lazy'"
      decoding="async"
      @error="failed = true"
    />
    <svg
      v-else
      class="companion-avatar__placeholder"
      data-testid="companion-placeholder"
      :viewBox="placeholder.viewBox"
      aria-hidden="true"
      focusable="false"
    >
      <path :d="placeholder.shoulders" fill="var(--kapoj-ink)" />
      <circle
        :cx="placeholder.head.cx"
        :cy="placeholder.head.cy"
        :r="placeholder.head.r"
        fill="var(--kapoj-silver)"
      />
      <circle
        v-for="(eye, index) in placeholder.eyes"
        :key="index"
        :cx="eye.cx"
        :cy="eye.cy"
        :r="placeholder.eyeRadius"
        fill="var(--kapoj-kapo)"
      />
    </svg>
  </span>
</template>

<style scoped>
.companion-avatar {
  display: block;
  flex: none;
  max-width: 100%;
  overflow: hidden;
}

.companion-avatar--xs {
  width: 24px;
}

.companion-avatar--sm {
  width: 44px;
}

.companion-avatar--md {
  width: 96px;
}

.companion-avatar--lg {
  width: min(240px, 40vw);
}

.companion-avatar__image,
.companion-avatar__placeholder {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
}
</style>
