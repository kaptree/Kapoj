<script setup lang="ts">
import { computed } from 'vue'

import AppSurface from '@/components/ui/AppSurface.vue'
import { useCompanionStore } from '@/stores/companion'
import type { CompanionActivityLevel } from '@/stores/companion'

const store = useCompanionStore()

// Preferences live in this browser only; loading once keeps every surface in sync.
store.load()

const ACTIVITY_OPTIONS: ReadonlyArray<{
  value: CompanionActivityLevel
  label: string
  test: string
  hint: string
}> = [
  { value: 'quiet', label: '安静', test: 'activity-quiet', hint: '只在必要时出现。' },
  { value: 'normal', label: '正常', test: 'activity-normal', hint: '保持默认的出场频率。' },
  { value: 'lively', label: '活泼', test: 'activity-lively', hint: '更频繁地露个面。' },
]

const activityHint = computed(
  () => ACTIVITY_OPTIONS.find((option) => option.value === store.activityLevel)?.hint ?? '',
)

function toggleEnabled(): void {
  store.setEnabled(!store.enabled)
}

function toggleMotion(): void {
  store.setMotion(store.motion === 'reduce' ? 'system' : 'reduce')
}
</script>

<template>
  <AppSurface padding="lg" class="companion-settings">
    <div class="companion-settings__head">
      <span class="companion-settings__title">伙伴</span>
      <span class="companion-settings__desc">
        她的出现频率、动效与显示方式。当前为本地偏好，仅本浏览器生效。
      </span>
    </div>

    <div class="companion-settings__rows">
      <div class="companion-settings__row">
        <div class="companion-settings__copy">
          <span class="companion-settings__label">显示伙伴</span>
          <span class="companion-settings__hint">
            {{
              store.enabled ? '已开启，她会在站点里偶尔出现。' : '已关闭，需要时可以随时重新开启。'
            }}
          </span>
        </div>
        <button
          type="button"
          class="companion-settings__toggle"
          data-test="companion-enabled"
          :aria-pressed="store.enabled"
          @click="toggleEnabled"
        >
          启用伙伴
        </button>
      </div>

      <div class="companion-settings__row">
        <div class="companion-settings__copy">
          <span class="companion-settings__label">活跃程度</span>
          <span class="companion-settings__hint">
            {{ activityHint }}
          </span>
        </div>
        <div class="companion-settings__segments" role="group" aria-label="活跃程度">
          <button
            v-for="option in ACTIVITY_OPTIONS"
            :key="option.value"
            type="button"
            class="companion-settings__segment"
            :data-test="option.test"
            :aria-pressed="store.activityLevel === option.value"
            @click="store.setActivityLevel(option.value)"
          >
            {{ option.label }}
          </button>
        </div>
      </div>

      <div class="companion-settings__row">
        <div class="companion-settings__copy">
          <span class="companion-settings__label">动效</span>
          <span class="companion-settings__hint">
            {{
              store.motion === 'reduce'
                ? '已减少动效，减少闪烁与位移动画。'
                : '跟随系统的动效偏好。'
            }}
          </span>
        </div>
        <button
          type="button"
          class="companion-settings__toggle"
          data-test="motion-reduce"
          :aria-pressed="store.motion === 'reduce'"
          @click="toggleMotion"
        >
          减少动效
        </button>
      </div>

      <div class="companion-settings__footer">
        <span class="companion-settings__hint">重置会恢复默认设置并清空本地偏好。</span>
        <button
          type="button"
          class="companion-settings__reset"
          data-test="companion-reset"
          @click="store.reset()"
        >
          重置偏好
        </button>
      </div>
    </div>
  </AppSurface>
</template>

<style scoped>
.companion-settings {
  min-width: 0;
}

.companion-settings__head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: var(--kapoj-space-2);
  margin-bottom: var(--kapoj-space-3);
}

.companion-settings__title {
  color: var(--kapoj-text);
  font-size: 1rem;
  font-weight: 600;
}

.companion-settings__desc {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.companion-settings__rows {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.companion-settings__row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
}

.companion-settings__copy {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.companion-settings__label {
  color: var(--kapoj-text);
  font-size: 0.9rem;
  font-weight: 600;
}

.companion-settings__hint {
  color: var(--kapoj-text-muted);
  font-size: 12px;
  line-height: 1.5;
}

.companion-settings__toggle,
.companion-settings__segment,
.companion-settings__reset {
  padding: 6px 12px;
  font-size: 0.82rem;
  color: var(--kapoj-text);
  cursor: pointer;
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.companion-settings__toggle[aria-pressed='true'] {
  color: var(--kapoj-kapo-text);
  background: var(--kapoj-kapo-soft);
  border-color: color-mix(in srgb, var(--kapoj-kapo) 45%, var(--kapoj-border));
}

.companion-settings__toggle:hover,
.companion-settings__reset:hover {
  border-color: var(--kapoj-border-strong);
}

.companion-settings__segments {
  display: inline-flex;
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.companion-settings__segment {
  border: 0;
  border-radius: 0;
}

.companion-settings__segment + .companion-settings__segment {
  border-left: 1px solid var(--kapoj-border);
}

.companion-settings__segment[aria-pressed='true'] {
  color: var(--kapoj-kapo-text);
  background: var(--kapoj-kapo-soft);
}

.companion-settings__segment:hover[aria-pressed='false'],
.companion-settings__reset:hover {
  background: color-mix(in srgb, var(--kapoj-border) 35%, var(--kapoj-bg-elevated));
}

.companion-settings__toggle:focus-visible,
.companion-settings__segment:focus-visible,
.companion-settings__reset:focus-visible {
  outline: 2px solid var(--kapoj-kapo);
  outline-offset: 1px;
}

.companion-settings__footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
  padding-top: var(--kapoj-space-3);
  border-top: 1px solid var(--kapoj-border);
}

.companion-settings__reset {
  background: transparent;
}

@media (max-width: 640px) {
  .companion-settings__row,
  .companion-settings__footer {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
