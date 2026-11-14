<script setup lang="ts">
import { computed } from 'vue'
import type { RouteLocationRaw } from 'vue-router'

import type { EquippedBadge, EquippedTitle, RatingLevel } from '@/api/types'
import { safeTitleColor } from '@/utils/gamification'
import IdentityShowcase from './IdentityShowcase.vue'
import RatingLevelName from './RatingLevelName.vue'

const props = withDefaults(
  defineProps<{
    nickname: string
    username?: string
    title: EquippedTitle | null
    badges: EquippedBadge[]
    size?: 'small' | 'default'
    ratingLevel?: RatingLevel | null
    linkTo?: RouteLocationRaw
  }>(),
  { username: '', size: 'small', ratingLevel: null, linkTo: undefined },
)

const displayName = computed(() => props.nickname.trim() || props.username || '?')
const titleColor = computed(() =>
  props.title ? safeTitleColor(props.title.color, 'var(--kapoj-text)') : undefined,
)
</script>

<template>
  <span class="user-identity-name">
    <span v-if="title" class="user-identity-name__title" :style="{ color: titleColor }">
      【{{ title.name }}】
    </span>

    <RouterLink v-if="linkTo" :to="linkTo" class="user-identity-name__link" @click.stop>
      <RatingLevelName v-if="ratingLevel" :name="displayName" :level="ratingLevel" />
      <span v-else class="user-identity-name__name">{{ displayName }}</span>
    </RouterLink>
    <template v-else>
      <RatingLevelName v-if="ratingLevel" :name="displayName" :level="ratingLevel" />
      <span v-else class="user-identity-name__name">{{ displayName }}</span>
    </template>

    <IdentityShowcase
      v-if="badges.length"
      :badges="badges"
      :title="null"
      :size="size"
      class="user-identity-name__badges"
    />
  </span>
</template>

<style scoped>
.user-identity-name {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 0;
  align-items: center;
  min-width: 0;
  max-width: 100%;
  vertical-align: middle;
}

.user-identity-name__title {
  font-weight: 600;
  line-height: 1.2;
  white-space: nowrap;
  text-shadow: 0 0 12px color-mix(in srgb, currentColor 18%, transparent);
}

.user-identity-name__link {
  color: inherit;
  text-decoration: none;
}

.user-identity-name__link:hover {
  text-decoration: underline;
}

.user-identity-name__name {
  overflow-wrap: anywhere;
}

.user-identity-name__badges {
  min-width: 0;
  margin-left: 0.25em;
}
</style>
