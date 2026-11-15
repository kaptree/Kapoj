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
    total: number | null
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

const CANVAS_INSET = 20

const canvas = ref<HTMLElement | null>(null)
const size = ref({ width: 640, height: 220 })
let observer: ResizeObserver | undefined

function updateSize() {
  if (!canvas.value) return
  const width = canvas.value.clientWidth - CANVAS_INSET * 2
  const height = canvas.value.clientHeight - CANVAS_INSET * 2
  if (width > 0 && height > 0) size.value = { width, height }
}

onMounted(() => {
  updateSize()
  if (typeof ResizeObserver !== 'undefined' && canvas.value) {
    observer = new ResizeObserver(updateSize)
    observer.observe(canvas.value)
  }
})

onBeforeUnmount(() => observer?.disconnect())

const maxWords = computed(() => (size.value.width < 480 ? 16 : 24))
const totalLabel = computed(() => (props.total == null ? '— 条评价' : `${props.total} 条评价`))
const wordCloudLabel = computed(() =>
  props.total == null ? '评价词云' : `评价词云，共 ${props.total} 条评价`,
)

const layout = computed(() =>
  buildProfileWordCloudLayout(props.cloud, size.value.width, size.value.height, {
    maxWords: maxWords.value,
  }),
)

function wordStyle(placement: ProfileWordCloudPlacement): Record<string, string> {
  return {
    left: `${placement.x + CANVAS_INSET}px`,
    top: `${placement.y + CANVAS_INSET}px`,
    maxWidth: `${placement.width}px`,
    fontSize: `${placement.fontSize}px`,
    '--word-rotation': `${placement.rotation}deg`,
  }
}
</script>

<template>
  <div
    class="profile-word-cloud"
    data-test="profile-word-cloud"
    role="group"
    :aria-label="wordCloudLabel"
    @click="emit('open')"
  >
    <header class="profile-word-cloud__head">
      <div class="profile-word-cloud__heading">
        <h2 class="profile-word-cloud__title">社区评价</h2>
        <p class="profile-word-cloud__subtitle">来自社区成员的一句话印象</p>
      </div>
      <div class="profile-word-cloud__head-actions">
        <span class="profile-word-cloud__total">{{ totalLabel }}</span>
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
    </header>

    <div ref="canvas" class="profile-word-cloud__canvas">
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
          class="profile-word-cloud__word"
          :class="`profile-word-cloud__word--${placement.colorIndex}`"
          :style="wordStyle(placement)"
          :title="placement.text"
        >
          <span class="profile-word-cloud__word-text">{{ placement.text }}</span>
          <span class="profile-word-cloud__word-count">{{ placement.count }} 次</span>
        </span>
      </template>
    </div>

    <footer class="profile-word-cloud__footer">
      <span class="profile-word-cloud__hint">点击词云，写一句评价</span>
      <span v-if="layout.overflow > 0" class="profile-word-cloud__overflow">
        +{{ layout.overflow }}
      </span>
    </footer>
  </div>
</template>

<style scoped>
.profile-word-cloud {
  --cloud-0: var(--profile-primary-text, #0e7490);
  --cloud-1: #1d4ed8;
  --cloud-2: var(--profile-success-text, #047857);
  --cloud-3: #6d28d9;
  --cloud-4: #0369a1;

  position: relative;
  display: flex;
  flex-direction: column;
  height: 100%;
  min-width: 0;
  min-height: 360px;
  overflow: hidden;
  cursor: pointer;
  background:
    radial-gradient(
      90% 120% at 78% 8%,
      color-mix(in srgb, var(--profile-primary, #1595a8) 10%, transparent),
      transparent 58%
    ),
    var(--profile-surface, var(--kapoj-surface));
  border: 1px solid var(--profile-border, var(--kapoj-border));
  border-radius: var(--profile-radius, var(--kapoj-radius-lg));
  box-shadow: var(--profile-shadow, var(--kapoj-shadow-sm));
  transition:
    border-color var(--kapoj-transition-slow),
    box-shadow var(--kapoj-transition-slow),
    transform var(--kapoj-transition-slow);
}

.app-dark .profile-word-cloud {
  --cloud-0: #5eead4;
  --cloud-1: #93c5fd;
  --cloud-2: #86efac;
  --cloud-3: #c4b5fd;
  --cloud-4: #67e8f9;
}

.profile-word-cloud:hover {
  border-color: color-mix(
    in srgb,
    var(--profile-primary, var(--kapoj-primary)) 35%,
    var(--profile-border, var(--kapoj-border))
  );
  box-shadow: var(--profile-shadow-hover, var(--kapoj-shadow-md));
  transform: translateY(-1px);
}

.profile-word-cloud:focus-within {
  border-color: var(--profile-primary, var(--kapoj-primary));
  box-shadow: var(--profile-shadow-hover, var(--kapoj-shadow-md));
}

.profile-word-cloud__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--kapoj-space-2) var(--kapoj-space-3);
  padding: 18px 20px 14px;
  border-bottom: 1px solid var(--profile-border, var(--kapoj-border));
}

.profile-word-cloud__heading {
  min-width: 0;
}

.profile-word-cloud__title {
  margin: 0;
  color: var(--profile-text, var(--kapoj-text));
  font-size: 1.05rem;
  font-weight: 600;
  line-height: 1.3;
  letter-spacing: -0.01em;
}

.profile-word-cloud__subtitle {
  margin: 4px 0 0;
  color: var(--profile-text-muted, var(--kapoj-text-muted));
  font-size: 11px;
  line-height: 1.4;
}

.profile-word-cloud__head-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.profile-word-cloud__total {
  padding: 2px 8px;
  color: var(--profile-text-muted, var(--kapoj-text-muted));
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  white-space: nowrap;
  background: color-mix(in srgb, var(--kapoj-bg) 55%, transparent);
  border: 1px solid var(--profile-border, var(--kapoj-border));
  border-radius: 999px;
}

.profile-word-cloud__cta {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 5px 10px;
  color: var(--profile-primary-text, var(--kapoj-primary));
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  white-space: nowrap;
  cursor: pointer;
  background: var(--profile-primary-soft, var(--kapoj-primary-soft));
  border: 1px solid
    color-mix(
      in srgb,
      var(--profile-primary, var(--kapoj-primary)) 32%,
      var(--profile-border, var(--kapoj-border))
    );
  border-radius: 999px;
  transition:
    color var(--kapoj-transition),
    background-color var(--kapoj-transition),
    border-color var(--kapoj-transition);
}

.profile-word-cloud__cta:hover {
  background: color-mix(
    in srgb,
    var(--profile-primary, var(--kapoj-primary)) 14%,
    var(--profile-surface, var(--kapoj-surface))
  );
  border-color: var(--profile-primary, var(--kapoj-primary));
}

.profile-word-cloud__cta i {
  font-size: 9px;
}

.profile-word-cloud__canvas {
  position: relative;
  flex: 1;
  min-height: 210px;
  margin: 0 10px;
  overflow: hidden;
}

.profile-word-cloud__canvas::before {
  position: absolute;
  inset: 12% 18%;
  pointer-events: none;
  content: '';
  background: radial-gradient(
    ellipse at center,
    color-mix(in srgb, var(--profile-primary, #1595a8) 7%, transparent),
    transparent 70%
  );
  border-radius: 50%;
}

.profile-word-cloud__word {
  position: absolute;
  z-index: 2;
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  overflow: visible;
  font-weight: 600;
  line-height: 1.2;
  white-space: nowrap;
  cursor: pointer;
  transform: translate(-50%, -50%) rotate(var(--word-rotation, 0deg));
  transform-origin: center;
  transition:
    transform var(--kapoj-transition),
    filter var(--kapoj-transition);
}

.profile-word-cloud__word:hover {
  z-index: 5;
  filter: saturate(1.08);
  transform: translate(-50%, -50%) rotate(var(--word-rotation, 0deg)) scale(1.07);
}

.profile-word-cloud__word-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.profile-word-cloud__word-count {
  position: absolute;
  top: calc(100% + 2px);
  left: 50%;
  padding: 1px 6px;
  color: var(--profile-text-secondary, var(--kapoj-text-secondary));
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  font-weight: 600;
  line-height: 1.5;
  white-space: nowrap;
  pointer-events: none;
  background: var(--profile-surface, var(--kapoj-surface));
  border: 1px solid var(--profile-border, var(--kapoj-border));
  border-radius: 999px;
  opacity: 0;
  box-shadow: 0 2px 8px rgb(15 23 42 / 10%);
  transform: translate(-50%, 3px) scale(0.94);
  transition:
    opacity var(--kapoj-transition),
    transform var(--kapoj-transition);
}

.profile-word-cloud__word:hover .profile-word-cloud__word-count {
  opacity: 1;
  transform: translate(-50%, 0) scale(1);
}

.profile-word-cloud__word--0 {
  color: var(--cloud-0);
}

.profile-word-cloud__word--1 {
  color: var(--cloud-1);
}

.profile-word-cloud__word--2 {
  color: var(--cloud-2);
}

.profile-word-cloud__word--3 {
  color: var(--cloud-3);
}

.profile-word-cloud__word--4 {
  color: var(--cloud-4);
}

.profile-word-cloud__state {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--kapoj-space-1);
  color: var(--profile-text-muted, var(--kapoj-text-muted));
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

.profile-word-cloud__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
  min-height: 36px;
  padding: 0 20px 12px;
}

.profile-word-cloud__hint {
  overflow: hidden;
  color: var(--profile-text-muted, var(--kapoj-text-muted));
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.profile-word-cloud__overflow {
  flex: 0 0 auto;
  padding: 1px 7px;
  color: var(--profile-text-muted, var(--kapoj-text-muted));
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  background: var(--profile-surface, var(--kapoj-surface));
  border: 1px solid var(--profile-border, var(--kapoj-border));
  border-radius: 999px;
}

@media (max-width: 900px) {
  .profile-word-cloud {
    min-height: 300px;
  }

  .profile-word-cloud__canvas {
    min-height: 190px;
  }
}

@media (max-width: 640px) {
  .profile-word-cloud {
    min-height: 280px;
  }

  .profile-word-cloud__head {
    padding: 16px 16px 12px;
  }

  .profile-word-cloud__subtitle {
    display: none;
  }

  .profile-word-cloud__canvas {
    min-height: 170px;
  }

  .profile-word-cloud__footer {
    padding: 0 16px 10px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .profile-word-cloud,
  .profile-word-cloud__cta,
  .profile-word-cloud__word,
  .profile-word-cloud__word-count {
    transition: none;
  }

  .profile-word-cloud__word:hover {
    filter: none;
    transform: translate(-50%, -50%) rotate(var(--word-rotation, 0deg));
  }

  .profile-word-cloud:hover {
    transform: none;
  }
}
</style>
