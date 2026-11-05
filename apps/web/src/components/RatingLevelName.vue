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
  level: RatingLevel
}>()

const displayName = computed(() => props.name.trim() || '?')
const parts = computed(() => splitBlackRedName(displayName.value))
const blackRed = computed(() => isBlackRed(props.level))
const levelStyle = computed(() => ratingLevelStyle(props.level))
const firstStyle = computed(() => ({ color: props.level.color }))
const restStyle = computed(() => ({
  color: props.level.accentColor || BLACK_RED_ACCENT_COLOR,
}))
</script>

<template>
  <span
    class="rating-level-name"
    :class="{ 'rating-level-name--black-red': blackRed }"
    :style="levelStyle"
    :title="level.name"
    :aria-label="level.name"
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
</style>
