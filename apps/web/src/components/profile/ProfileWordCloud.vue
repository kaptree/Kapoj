<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

import type { EvaluationCloudItem } from '@/api/types'
import {
  buildProfileWordCloudLayout,
  type ProfileWordCloudPlacement,
} from '@/utils/profileWordCloudLayout'

const props = withDefaults(
  defineProps<{
    cloud: EvaluationCloudItem[]
    total: number
    loading?: boolean
    error?: string
  }>(),
  {
    loading: false,
    error: '',
  },
)

const emit = defineEmits<{
  open: []
  retry: []
}>()

const panel = ref<HTMLElement | null>(null)
const size = ref({ width: 640, height: 220 })
let observer: ResizeObserver | undefined

function updateSize() {
  if (!panel.value) return
  const width = panel.value.clientWidth
  const height = panel.value.clientHeight
  if (width > 0 && height > 0) size.value = { width, height }
}

onMounted(() => {
  updateSize()
  if (typeof ResizeObserver !== 'undefined' && panel.value) {
    observer = new ResizeObserver(updateSize)
    observer.observe(panel.value)
  }
})

onBeforeUnmount(() => observer?.disconnect())

const maxWords = computed(() => (size.value.width < 480 ? 16 : 24))

const layout = computed(() =>
  buildProfileWordCloudLayout(props.cloud, size.value.width, size.value.height, {
    maxWords: maxWords.value,
  }),
)

function wordStyle(placement: ProfileWordCloudPlacement) {
  return {
    left: `${placement.x}px`,
    top: `${placement.y}px`,
    maxWidth: `${placement.width}px`,
    fontSize: `${placement.fontSize}px`,
    transform: `translate(-50%, -50%) rotate(${placement.rotation}deg)`,
  }
}
</script>

<template>
  <div
    ref="panel"
    class="profile-word-cloud"
    data-test="profile-word-cloud"
    role="group"
    :aria-label="`评价词云，共 ${total} 条评价`"
    @click="emit('open')"
  >
    <div class="profile-word-cloud__top">
      <span class="profile-word-cloud__total">{{ total }} 条评价</span>
      <button
        type="button"
        class="profile-word-cloud__cta"
        aria-label="写评价"
        @click.stop="emit('open')"
      >
        <i class="pi pi-pencil" aria-hidden="true" />
        写评价
      </button>
    </div>

    <div v-if="loading" class="profile-word-cloud__state">正在加载评价</div>

    <div v-else-if="error" class="profile-word-cloud__state profile-word-cloud__state--error">
      <span>评价加载失败</span>
      <button type="button" class="profile-word-cloud__retry" @click.stop="emit('retry')">
        重试
      </button>
    </div>

    <div v-else-if="cloud.length === 0" class="profile-word-cloud__state">
      <span>还没有评价</span>
      <small>点击写下第一条评价</small>
    </div>

    <template v-else>
      <span
        v-for="placement in layout.placements"
        :key="placement.text"
        class="profile-word-cloud__word profile-word-cloud__word--truncate"
        :class="`profile-word-cloud__word--${placement.colorIndex}`"
        :style="wordStyle(placement)"
        :title="placement.text"
      >
        {{ placement.text }}
      </span>
      <span v-if="layout.overflow > 0" class="profile-word-cloud__overflow">
        +{{ layout.overflow }}
      </span>
    </template>

    <span v-if="!loading && !error" class="profile-word-cloud__hint"> 点击词云，写一句评价 </span>
  </div>
</template>

<style scoped>
.profile-word-cloud {
  position: relative;
  height: 220px;
  min-width: 0;
  overflow: hidden;
  cursor: pointer;
  background:
    radial-gradient(ellipse at 50% 35%, var(--kapoj-primary-soft), transparent 62%),
    var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-lg);
  transition:
    border-color var(--kapoj-transition),
    box-shadow var(--kapoj-transition);
}

.profile-word-cloud:hover {
  border-color: color-mix(in srgb, var(--kapoj-primary) 45%, var(--kapoj-border));
  box-shadow: var(--kapoj-glow-primary);
}

.profile-word-cloud__top {
  position: absolute;
  z-index: 3;
  top: var(--kapoj-space-3);
  right: var(--kapoj-space-3);
  left: var(--kapoj-space-3);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
  pointer-events: none;
}

.profile-word-cloud__total {
  padding: 2px 8px;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  background: color-mix(in srgb, var(--kapoj-bg) 72%, transparent);
  border: 1px solid var(--kapoj-border);
  border-radius: 999px;
}

.profile-word-cloud__cta {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 4px 9px;
  color: var(--kapoj-primary);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  cursor: pointer;
  pointer-events: auto;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 35%, transparent);
  border-radius: 999px;
}

.profile-word-cloud__cta:hover {
  border-color: var(--kapoj-primary);
}

.profile-word-cloud__cta i {
  font-size: 9px;
}

.profile-word-cloud__word {
  position: absolute;
  z-index: 2;
  display: block;
  overflow: hidden;
  font-weight: 600;
  line-height: 1.2;
  text-align: center;
  text-overflow: ellipsis;
  white-space: nowrap;
  pointer-events: none;
}

.profile-word-cloud__word--truncate {
  overflow: hidden;
  text-overflow: ellipsis;
}

.profile-word-cloud__word--0 {
  color: var(--kapoj-primary);
}

.profile-word-cloud__word--1 {
  color: var(--kapoj-success);
}

.profile-word-cloud__word--2 {
  color: var(--kapoj-warning);
}

.profile-word-cloud__word--3 {
  color: var(--kapoj-special);
}

.profile-word-cloud__word--4 {
  color: var(--kapoj-re);
}

.profile-word-cloud__state {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--kapoj-space-1);
  color: var(--kapoj-text-muted);
  font-size: 0.875rem;
  text-align: center;
}

.profile-word-cloud__state small {
  font-size: 0.75rem;
}

.profile-word-cloud__state--error {
  color: var(--kapoj-danger);
}

.profile-word-cloud__retry {
  padding: 4px 10px;
  color: var(--kapoj-danger);
  font-size: 0.75rem;
  cursor: pointer;
  background: transparent;
  border: 1px solid color-mix(in srgb, var(--kapoj-danger) 40%, transparent);
  border-radius: var(--kapoj-radius-sm);
}

.profile-word-cloud__hint {
  position: absolute;
  z-index: 3;
  bottom: var(--kapoj-space-3);
  left: var(--kapoj-space-3);
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  pointer-events: none;
}

.profile-word-cloud__overflow {
  position: absolute;
  z-index: 3;
  right: var(--kapoj-space-3);
  bottom: var(--kapoj-space-3);
  padding: 1px 7px;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  background: var(--kapoj-surface);
  border: 1px solid var(--kapoj-border);
  border-radius: 999px;
}

@media (max-width: 640px) {
  .profile-word-cloud {
    height: 180px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .profile-word-cloud {
    transition: none;
  }
}
</style>
