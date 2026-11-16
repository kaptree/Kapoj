<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, RouterLink } from 'vue-router'

import { getActivitySummary, signInActivity } from '../../api/leaderboard'
import type { ActivitySummary } from '../../api/types'
import { useCompanionMute } from '../../composables/useCompanionMute'
import { pickCompanionLine } from '../../content/companion/lines.zh-CN'
import { useAuthStore } from '../../stores/auth'
import { useCompanionStore } from '../../stores/companion'
import { localDateKey } from '../../utils/date'
import CompanionAvatar from './CompanionAvatar.vue'
import CompanionBubble from './CompanionBubble.vue'

const AUTH_ROUTES = new Set([
  'login',
  'register',
  'forgot-password',
  'reset-password',
  'verify-email',
])

const auth = useAuthStore()
const store = useCompanionStore()
const { muted } = useCompanionMute()
const route = useRoute()

store.load()

const open = ref(false)
const signedInToday = ref<boolean | null>(null)
const summary = ref<ActivitySummary | null>(null)
const signingIn = ref(false)
const signInError = ref(false)
const toggleRef = ref<HTMLButtonElement | null>(null)
const panelRef = ref<HTMLElement | null>(null)

const visible = computed(
  () => auth.isLoggedIn && store.enabled && !muted.value && !AUTH_ROUTES.has(String(route.name)),
)

const line = computed(() =>
  pickCompanionLine(signedInToday.value === false ? 'signin' : 'greeting', localDateKey()),
)

async function loadSummary(): Promise<void> {
  if (!visible.value) return

  try {
    const result = await getActivitySummary()
    summary.value = result
    signedInToday.value = result.signedInToday
  } catch {
    // Keep the last known state; the dock stays usable when the request fails.
  }
}

async function handleSignIn(): Promise<void> {
  if (signingIn.value) return

  signingIn.value = true
  signInError.value = false
  try {
    await signInActivity()
    signedInToday.value = true
  } catch {
    signInError.value = true
  } finally {
    signingIn.value = false
  }
}

function close(): void {
  if (!open.value) return

  open.value = false
  void nextTick(() => {
    toggleRef.value?.focus()
  })
}

function focusPanel(): void {
  const target = panelRef.value?.querySelector<HTMLElement>(
    'a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])',
  )
  ;(target ?? panelRef.value)?.focus()
}

function toggle(): void {
  if (open.value) {
    close()
    return
  }

  open.value = true
  void nextTick(focusPanel)
}

function onWindowKeydown(event: KeyboardEvent): void {
  if (open.value && event.key === 'Escape') close()
}

watch(
  visible,
  (value) => {
    if (!value) {
      open.value = false
      return
    }
    void loadSummary()
  },
  { immediate: true },
)

watch(
  () => route.fullPath,
  () => {
    open.value = false
  },
)

onMounted(() => {
  window.addEventListener('keydown', onWindowKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onWindowKeydown)
})
</script>

<template>
  <div v-if="visible" class="companion-dock">
    <div class="companion-dock__anchor">
      <button
        v-if="!open && signedInToday === false"
        class="companion-dock__signin"
        type="button"
        :disabled="signingIn"
        @click="handleSignIn"
      >
        {{ signingIn ? '签到中…' : '签到' }}
      </button>
      <button
        ref="toggleRef"
        class="companion-dock__toggle"
        type="button"
        :aria-expanded="open ? 'true' : 'false'"
        aria-controls="companion-dock-panel"
        :aria-label="open ? '收起伙伴面板' : '打开伙伴面板'"
        @click="toggle"
      >
        <CompanionAvatar pose="chibi" size="sm" eager />
        <span class="companion-dock__dot" aria-hidden="true" />
      </button>
    </div>

    <p v-if="signInError" class="companion-dock__error">……线路断了。</p>

    <div
      v-if="open"
      id="companion-dock-panel"
      ref="panelRef"
      class="companion-dock__panel"
      role="region"
      aria-label="KapOJ 伙伴"
      tabindex="-1"
    >
      <div class="companion-dock__intro">
        <CompanionAvatar pose="bust" size="sm" eager />
        <CompanionBubble :text="line" />
      </div>

      <p v-if="summary" class="companion-dock__activity">
        <span>今日活跃 {{ summary.todayPoints }}</span>
        <span>总活跃 {{ summary.totalPoints }}</span>
      </p>

      <button
        v-if="signedInToday === false"
        class="companion-dock__signin"
        type="button"
        :disabled="signingIn"
        @click="handleSignIn"
      >
        {{ signingIn ? '签到中…' : '签到' }}
      </button>

      <div class="companion-dock__links">
        <RouterLink class="companion-dock__link" to="/problems">去刷题</RouterLink>
        <RouterLink class="companion-dock__link" to="/appearance">伙伴设置</RouterLink>
      </div>

      <div class="companion-dock__actions">
        <button
          class="companion-dock__quiet"
          type="button"
          :disabled="store.activityLevel === 'quiet'"
          @click="store.setActivityLevel('quiet')"
        >
          安静
        </button>
        <button
          class="companion-dock__close"
          type="button"
          aria-label="收起伙伴面板"
          @click="close"
        >
          <span aria-hidden="true">×</span>
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.companion-dock {
  position: fixed;
  right: var(--kapoj-space-5);
  bottom: var(--kapoj-space-5);
  z-index: 25;
  display: flex;
  flex-direction: column-reverse;
  align-items: flex-end;
  gap: 10px;
}

.companion-dock__anchor {
  display: flex;
  gap: 8px;
  align-items: center;
}

.companion-dock__toggle {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  padding: 0;
  cursor: pointer;
  background: var(--kapoj-surface);
  border: 1px solid var(--kapoj-border-strong);
  border-radius: 50%;
  box-shadow: var(--kapoj-shadow-md);
}

.companion-dock__toggle:hover {
  border-color: var(--kapoj-kapo);
}

.companion-dock__toggle:focus-visible {
  outline: 2px solid var(--kapoj-kapo);
  outline-offset: 2px;
}

.companion-dock__toggle :deep(.companion-avatar) {
  width: 42px;
}

.companion-dock__dot {
  position: absolute;
  top: 6px;
  right: 6px;
  width: 8px;
  height: 8px;
  background: var(--kapoj-kapo);
  border-radius: 50%;
}

.companion-dock__panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
  box-sizing: border-box;
  width: min(320px, calc(100vw - 32px));
  max-height: 60vh;
  padding: 14px;
  overflow-y: auto;
  background: var(--kapoj-surface);
  border: 1px solid var(--kapoj-border-strong);
  border-radius: var(--kapoj-radius-md);
  box-shadow: var(--kapoj-shadow-md);
}

.companion-dock__intro {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}

.companion-dock__intro :deep(.companion-avatar) {
  width: 48px;
}

.companion-dock__intro :deep(.companion-bubble) {
  flex: 1;
  min-width: 0;
  margin-bottom: 6px;
}

.companion-dock__activity {
  display: flex;
  gap: 12px;
  margin: 0;
  font-size: 0.78rem;
  color: var(--kapoj-text-secondary);
}

.companion-dock__signin,
.companion-dock__quiet {
  padding: 6px 12px;
  font-size: 0.82rem;
  color: var(--kapoj-text);
  cursor: pointer;
  background: var(--kapoj-kapo-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-kapo) 45%, var(--kapoj-border));
  border-radius: var(--kapoj-radius-sm);
}

.companion-dock__signin:hover:not(:disabled),
.companion-dock__quiet:hover:not(:disabled) {
  border-color: var(--kapoj-kapo);
}

.companion-dock__signin:disabled,
.companion-dock__quiet:disabled {
  cursor: default;
  opacity: 0.55;
}

.companion-dock__signin:focus-visible,
.companion-dock__quiet:focus-visible,
.companion-dock__link:focus-visible,
.companion-dock__close:focus-visible {
  outline: 2px solid var(--kapoj-kapo);
  outline-offset: 1px;
}

.companion-dock__error {
  margin: 0;
  font-size: 0.78rem;
  color: var(--kapoj-kapo-text);
}

.companion-dock__links {
  display: flex;
  gap: 12px;
}

.companion-dock__link {
  font-size: 0.82rem;
  color: var(--kapoj-text-secondary);
  text-decoration: none;
}

.companion-dock__link:hover {
  color: var(--kapoj-kapo-text);
  text-decoration: underline;
}

.companion-dock__actions {
  display: flex;
  gap: 8px;
  align-items: center;
  justify-content: space-between;
}

.companion-dock__close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  font-size: 1rem;
  line-height: 1;
  color: var(--kapoj-text-secondary);
  cursor: pointer;
  background: transparent;
  border: 0;
  border-radius: 4px;
}

.companion-dock__close:hover {
  color: var(--kapoj-text);
  background: color-mix(in srgb, var(--kapoj-border) 50%, transparent);
}

@media (max-width: 640px) {
  .companion-dock {
    right: 16px;
    bottom: 16px;
  }
}
</style>
