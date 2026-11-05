<script setup lang="ts">
import { computed, ref } from 'vue'
import Avatar from 'primevue/avatar'
import vTooltip from 'primevue/tooltip'

import type { EquippedBadge, EquippedTitle } from '@/api/types'
import { badgeImageSrc, badgeInitial, safeTitleColor } from '@/utils/gamification'

const props = withDefaults(
  defineProps<{
    badges: EquippedBadge[]
    title: EquippedTitle | null
    size?: 'small' | 'default'
  }>(),
  { size: 'default' },
)

const avatarSize = computed(() => (props.size === 'small' ? 18 : 22))

const avatarStyle = computed(() => ({
  width: `${avatarSize.value}px`,
  height: `${avatarSize.value}px`,
  fontSize: `${Math.max(8, Math.round(avatarSize.value * 0.5))}px`,
}))

const titleColor = computed(() =>
  props.title ? safeTitleColor(props.title.color, 'var(--kapoj-text)') : undefined,
)

const failedImageIds = ref<number[]>([])

function onBadgeError(id: number) {
  if (!failedImageIds.value.includes(id)) failedImageIds.value.push(id)
}

function badgeImage(badge: EquippedBadge): string | undefined {
  return failedImageIds.value.includes(badge.id) ? undefined : badgeImageSrc(badge.imageUrl)
}

function avatarLabel(badge: EquippedBadge): string | undefined {
  return badgeImage(badge) ? undefined : badgeInitial(badge.name)
}
</script>

<template>
  <span
    v-if="title || badges.length"
    class="identity-showcase"
    :class="`identity-showcase--${size}`"
  >
    <span v-if="title" class="identity-title" :style="{ color: titleColor }">
      {{ title.name }}
    </span>
    <span v-for="badge in badges" :key="badge.id" class="identity-badge">
      <Avatar
        v-tooltip.top="badge.name"
        :image="badgeImage(badge)"
        :label="avatarLabel(badge)"
        shape="square"
        class="identity-badge__avatar"
        :style="avatarStyle"
        :aria-label="badge.name"
        @error="onBadgeError(badge.id)"
      />
    </span>
  </span>
</template>

<style scoped>
.identity-showcase {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-1);
  align-items: center;
  vertical-align: middle;
}

.identity-title {
  font-weight: 600;
  line-height: 1.2;
  text-shadow: 0 0 12px color-mix(in srgb, currentColor 18%, transparent);
}

.identity-badge {
  display: inline-flex;
}

.identity-badge__avatar {
  color: var(--kapoj-primary);
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 28%, transparent);
  border-radius: var(--kapoj-radius-xs);
}

.identity-badge__avatar :deep(img) {
  width: 100%;
  height: 100%;
  object-fit: cover;
  border-radius: inherit;
}
</style>
