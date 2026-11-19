<script setup lang="ts">
import { computed, ref } from 'vue'

import { updateCompanionSettings } from '@/api/companion'
import type { CompanionSettingsPatch } from '@/api/types'
import AppSurface from '@/components/ui/AppSurface.vue'
import CompanionPersonalityPanel from './CompanionPersonalityPanel.vue'
import { useAuthStore } from '@/stores/auth'
import { useCompanionStore } from '@/stores/companion'
import type { CompanionActivityLevel } from '@/stores/companion'

const auth = useAuthStore()
const store = useCompanionStore()

// Preferences live in this browser; loading once keeps every surface in sync.
store.load()

const syncError = ref('')

/** Activity and dock stay local-first; only the profile is server-owned state. */
const LOCAL_SYNC_ERROR = '未能同步到服务器，本地偏好已保存。'
const PROFILE_SYNC_ERROR = '公开设置同步失败，已恢复原值。'
const PERSONALITY_SYNC_ERROR = '性格设置同步失败，已恢复原值。'

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

/** An admin can forbid users from switching the companion off themselves. */
const userDisableLocked = computed(() => !store.allowUserDisable)

/**
 * Signed-in visitors sync the server-supported fields. Guests and network
 * failures keep the local preference: this is a best-effort mirror, never a
 * gate on the Phase 0–1 behaviour.
 */
async function persistSettings(patch: CompanionSettingsPatch): Promise<void> {
  if (!auth.isLoggedIn) return

  syncError.value = ''
  try {
    const state = await updateCompanionSettings(patch)
    store.applyRemoteState(state)
  } catch {
    // The local preference is authoritative for these fields, so keep it.
    syncError.value = LOCAL_SYNC_ERROR
  }
}

function toggleEnabled(): void {
  const next = !store.enabled
  store.setEnabled(next)
  void persistSettings({ dockEnabled: next })
}

function selectActivity(value: CompanionActivityLevel): void {
  store.setActivityLevel(value)
  void persistSettings({ activityLevel: value })
}

function toggleMotion(): void {
  store.setMotion(store.motion === 'reduce' ? 'system' : 'reduce')
}

async function togglePublicProfile(): Promise<void> {
  if (!auth.isLoggedIn) return

  const previous = store.publicProfile
  const next = !previous
  store.publicProfile = next
  syncError.value = ''
  try {
    const state = await updateCompanionSettings({ publicProfile: next })
    store.applyRemoteState(state)
  } catch {
    // The profile flag is server-only: revert instead of showing a value that
    // was never persisted.
    store.publicProfile = previous
    syncError.value = PROFILE_SYNC_ERROR
  }
}

async function savePersonality(patch: {
  personalityPrompt: string
  aiDialogueEnabled: boolean
}): Promise<void> {
  if (!auth.isLoggedIn) return

  const previous = {
    personalityPrompt: store.personalityPrompt,
    aiDialogueEnabled: store.aiDialogueEnabled,
  }
  store.personalityPrompt = patch.personalityPrompt
  store.aiDialogueEnabled = patch.aiDialogueEnabled
  syncError.value = ''
  try {
    const state = await updateCompanionSettings(patch)
    store.applyRemoteState(state)
  } catch {
    store.personalityPrompt = previous.personalityPrompt
    store.aiDialogueEnabled = previous.aiDialogueEnabled
    syncError.value = PERSONALITY_SYNC_ERROR
  }
}
</script>

<template>
  <AppSurface padding="lg" class="companion-settings">
    <div class="companion-settings__head">
      <span class="companion-settings__title">{{ store.name }}</span>
      <span class="companion-settings__desc">
        她的出现频率、动效、性格与 AI 台词。登录后会同步到账号，未登录时仅本浏览器生效。
      </span>
    </div>

    <p v-if="!store.globalEnabled" class="companion-settings__notice">
      站点已关闭伙伴功能，这里的偏好会保留，重新开启后继续生效。
    </p>

    <div class="companion-settings__rows">
      <div class="companion-settings__row">
        <div class="companion-settings__copy">
          <span class="companion-settings__label">显示伙伴</span>
          <span class="companion-settings__hint">
            {{
              userDisableLocked
                ? '管理员已关闭此选项'
                : store.enabled
                  ? '已开启，她会在站点里偶尔出现。'
                  : '已关闭，需要时可以随时重新开启。'
            }}
          </span>
        </div>
        <button
          type="button"
          class="companion-settings__toggle"
          data-test="companion-enabled"
          :aria-pressed="store.enabled"
          :disabled="userDisableLocked"
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
            @click="selectActivity(option.value)"
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

      <div v-if="auth.isLoggedIn" class="companion-settings__row">
        <div class="companion-settings__copy">
          <span class="companion-settings__label">公开资料展示</span>
          <span class="companion-settings__hint">
            {{
              store.publicProfile
                ? '其他访客可以在你的公开资料里看到她。'
                : '伙伴不会出现在你的公开资料里。'
            }}
          </span>
        </div>
        <button
          type="button"
          class="companion-settings__toggle"
          data-test="public-profile"
          :aria-pressed="store.publicProfile"
          @click="togglePublicProfile"
        >
          公开伙伴
        </button>
      </div>

      <CompanionPersonalityPanel
        v-if="auth.isLoggedIn"
        :personality-prompt="store.personalityPrompt"
        :ai-dialogue-enabled="store.aiDialogueEnabled"
        :ai-ready="store.aiReady"
        :remaining-today="store.aiRemainingToday"
        @save="savePersonality"
      />

      <p v-if="syncError" class="companion-settings__hint" role="status">
        {{ syncError }}
      </p>

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

.companion-settings__notice {
  margin: 0 0 var(--kapoj-space-3);
  padding: 8px 10px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--kapoj-kapo-text);
  background: var(--kapoj-kapo-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-kapo) 30%, var(--kapoj-border));
  border-radius: var(--kapoj-radius-sm);
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

.companion-settings__toggle:disabled {
  cursor: default;
  opacity: 0.55;
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
