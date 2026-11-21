<script setup lang="ts">
import { computed } from 'vue'

import type { RatingLevel } from '@/api/types'
import {
  BLACK_RED_ACCENT_COLOR,
  isBlackRed,
  ratingLevelStyle,
  splitBlackRedName,
} from '@/utils/rating'

const props = defineProps<{
  name: string
  level?: RatingLevel | null
  inheritTypography?: boolean
}>()

const displayName = computed(() => props.name.trim() || '?')
const parts = computed(() => splitBlackRedName(displayName.value))
const hasLevel = computed(() => Boolean(props.level?.key && props.level?.color))
const blackRed = computed(() => hasLevel.value && isBlackRed(props.level as RatingLevel))
const levelStyle = computed(() =>
  hasLevel.value ? ratingLevelStyle(props.level as RatingLevel) : {},
)
const accessibleName = computed(() =>
  hasLevel.value
    ? `${displayName.value}（${(props.level as RatingLevel).name}）`
    : displayName.value,
)
const firstStyle = computed(() => ({ color: (props.level as RatingLevel).color }))
const restStyle = computed(() => ({
  color: (props.level as RatingLevel).accentColor || BLACK_RED_ACCENT_COLOR,
}))
</script>

<template>
  <span
    class="rating-level-name"
    :class="{
      'rating-level-name--black-red': blackRed,
      'rating-level-name--inherit': inheritTypography,
    }"
    :style="levelStyle"
    :title="accessibleName"
    :aria-label="accessibleName"
  >
    <template v-if="blackRed">
      <span class="rating-level-name__first" :style="firstStyle">{{ parts.first }}</span>
      <span v-if="parts.rest" class="rating-level-name__rest" :style="restStyle">
        {{ parts.rest }}
      </span>
    </template>
    <template v-else>{{ displayName }}</template>
  </span>
</template>

<style scoped>
.rating-level-name {
  font-weight: 600;
  white-space: nowrap;
}

.rating-level-name--inherit {
  font-weight: inherit;
  white-space: normal;
}
</style>
