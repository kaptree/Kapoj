<script setup lang="ts">
import { computed } from 'vue'

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
</script>

<template>
  <span v-if="title || badges.length" class="identity-showcase">
    <span v-if="title" class="identity-title" :style="{ color: safeTitleColor(title.color) }">
      {{ title.name }}
    </span>
    <span v-for="badge in badges" :key="badge.id" class="identity-badge">
      <el-tooltip :content="badge.name" placement="top">
        <el-avatar :size="avatarSize" :src="badgeImageSrc(badge.imageUrl)">
          {{ badgeInitial(badge.name) }}
        </el-avatar>
      </el-tooltip>
    </span>
  </span>
</template>

<style scoped>
.identity-showcase {
  display: inline-flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
  vertical-align: middle;
}

.identity-title {
  font-weight: 600;
  line-height: 1.2;
}

.identity-badge {
  display: inline-flex;
}

.identity-badge :deep(.el-avatar) {
  font-size: 12px;
  line-height: 1;
}
</style>
