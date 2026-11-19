<script setup lang="ts">
import { computed } from 'vue'

import { resolveCompanionOutfit } from '@/content/companion/manifest'
import type { CompanionOutfit } from '@/content/companion/types'

const props = defineProps<{
  outfits: readonly CompanionOutfit[]
  equippedSkin: string
  unlockedSkins: readonly string[]
  level: number
}>()

const emit = defineEmits<{ equip: [outfitId: string] }>()

const current = computed(() => resolveCompanionOutfit(props.equippedSkin))

function isUnlocked(outfit: CompanionOutfit): boolean {
  return (
    outfit.id === 'outfit-1' ||
    props.unlockedSkins.includes(outfit.id) ||
    props.level >= outfit.level
  )
}

function select(outfit: CompanionOutfit): void {
  if (!isUnlocked(outfit)) return
  emit('equip', outfit.id)
}
</script>

<template>
  <section class="companion-wardrobe" aria-labelledby="companion-wardrobe-title">
    <div class="companion-wardrobe__head">
      <span id="companion-wardrobe-title" class="companion-wardrobe__title">衣橱</span>
      <span class="companion-wardrobe__hint">按好感等级解锁；当前穿戴会展示在伙伴档案里。</span>
    </div>

    <div class="companion-wardrobe__preview">
      <img
        class="companion-wardrobe__preview-image"
        :src="current.src"
        :alt="current.name"
        loading="lazy"
        decoding="async"
      />
      <span class="companion-wardrobe__preview-name">{{ current.name }}</span>
    </div>

    <div class="companion-wardrobe__grid" role="list">
      <button
        v-for="outfit in outfits"
        :key="outfit.id"
        type="button"
        class="companion-wardrobe__card"
        :class="{
          'companion-wardrobe__card--locked': !isUnlocked(outfit),
          'companion-wardrobe__card--equipped': outfit.id === current.id,
        }"
        :data-test="`wardrobe-outfit-${outfit.id}`"
        :disabled="!isUnlocked(outfit)"
        :aria-current="outfit.id === current.id ? 'true' : undefined"
        role="listitem"
        @click="select(outfit)"
      >
        <img
          class="companion-wardrobe__thumb"
          :src="outfit.card"
          :alt="outfit.name"
          loading="lazy"
          decoding="async"
        />
        <span class="companion-wardrobe__name">{{ outfit.name }}</span>
        <span v-if="!isUnlocked(outfit)" class="companion-wardrobe__lock">
          Lv.{{ outfit.level }} 解锁
        </span>
        <span v-else-if="outfit.id === current.id" class="companion-wardrobe__equipped">
          已穿戴
        </span>
        <span v-else class="companion-wardrobe__wear">穿戴</span>
      </button>
    </div>
  </section>
</template>

<style scoped>
.companion-wardrobe {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding-top: var(--kapoj-space-3);
  border-top: 1px solid var(--kapoj-border);
}

.companion-wardrobe__head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: var(--kapoj-space-2);
}

.companion-wardrobe__title {
  color: var(--kapoj-text);
  font-size: 0.95rem;
  font-weight: 600;
}

.companion-wardrobe__hint {
  color: var(--kapoj-text-muted);
  font-size: 12px;
  line-height: 1.5;
}

.companion-wardrobe__preview {
  position: relative;
  display: grid;
  width: min(220px, 60%);
  aspect-ratio: 3 / 4;
  overflow: hidden;
  background:
    linear-gradient(
      45deg,
      color-mix(in srgb, var(--kapoj-border) 45%, transparent) 25%,
      transparent 25%
    ),
    linear-gradient(
      -45deg,
      color-mix(in srgb, var(--kapoj-border) 45%, transparent) 25%,
      transparent 25%
    ),
    linear-gradient(
      45deg,
      transparent 75%,
      color-mix(in srgb, var(--kapoj-border) 45%, transparent) 75%
    ),
    linear-gradient(
      -45deg,
      transparent 75%,
      color-mix(in srgb, var(--kapoj-border) 45%, transparent) 75%
    ),
    var(--kapoj-bg-elevated);
  background-position:
    0 0,
    0 8px,
    8px -8px,
    -8px 0;
  background-size: 16px 16px;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
  place-items: center;
}

.companion-wardrobe__preview-image {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.companion-wardrobe__preview-name {
  position: absolute;
  right: 8px;
  bottom: 8px;
  padding: 3px 8px;
  font-size: 12px;
  color: var(--kapoj-text);
  background: color-mix(in srgb, var(--kapoj-surface) 88%, transparent);
  border: 1px solid var(--kapoj-border);
  border-radius: 999px;
}

.companion-wardrobe__grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 8px;
}

.companion-wardrobe__card {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
  padding: 6px;
  overflow: hidden;
  font: inherit;
  text-align: left;
  color: var(--kapoj-text);
  cursor: pointer;
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.companion-wardrobe__card:hover:not(:disabled) {
  border-color: var(--kapoj-kapo);
}

.companion-wardrobe__card--equipped {
  border-color: color-mix(in srgb, var(--kapoj-kapo) 55%, var(--kapoj-border));
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--kapoj-kapo) 25%, transparent);
}

.companion-wardrobe__card--locked {
  cursor: default;
  opacity: 0.55;
}

.companion-wardrobe__thumb {
  width: 100%;
  aspect-ratio: 3 / 4;
  object-fit: contain;
  background: color-mix(in srgb, var(--kapoj-kapo) 6%, transparent);
  border-radius: 4px;
}

.companion-wardrobe__name {
  overflow: hidden;
  font-size: 11px;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.companion-wardrobe__lock,
.companion-wardrobe__wear,
.companion-wardrobe__equipped {
  font-size: 10px;
  color: var(--kapoj-text-muted);
}

.companion-wardrobe__equipped {
  color: var(--kapoj-kapo-text);
}

.companion-wardrobe__card:focus-visible {
  outline: 2px solid var(--kapoj-kapo);
  outline-offset: 1px;
}

@media (max-width: 640px) {
  .companion-wardrobe__preview {
    width: min(200px, 80%);
  }

  .companion-wardrobe__grid {
    display: flex;
    padding-bottom: 4px;
    overflow-x: auto;
  }

  .companion-wardrobe__card {
    flex: 0 0 92px;
  }
}
</style>
