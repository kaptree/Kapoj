<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, RouterLink } from 'vue-router'

import { getCompanion, petCompanion } from '../../api/companion'
import { getActivitySummary, signInActivity } from '../../api/leaderboard'
import type { ActivitySummary } from '../../api/types'
import { useCompanionMute } from '../../composables/useCompanionMute'
import { useCompanionDialogue, type DialogueGreetingKey } from '../../composables/useCompanionDialogue'
import { useCompanionGaze } from '../../composables/useCompanionGaze'
import type { CompanionLineKey } from '../../content/companion/lines.zh-CN'
import { pickCompanionLine } from '../../content/companion/lines.zh-CN'
import { pickPetExpression } from '../../content/companion/petExpressions'
import type { CompanionQExpression } from '../../content/companion/types'
import { useAuthStore } from '../../stores/auth'
import { useCompanionStore } from '../../stores/companion'
import { localDateKey } from '../../utils/date'
import CompanionBondMeter from './CompanionBondMeter.vue'
import CompanionBubble from './CompanionBubble.vue'
import CompanionQAvatar from './CompanionQAvatar.vue'

const AUTH_ROUTES = new Set([
  'login',
  'register',
  'forgot-password',
  'reset-password',
  'verify-email',
])

const PET_LINE_KEYS: Record<CompanionQExpression, CompanionLineKey> = {
  neutral: 'pet',
  happy: 'pet.happy',
  shy: 'pet.shy',
  pout: 'pet.pout',
  surprised: 'pet.surprised',
  sleepy: 'pet.sleepy',
  smug: 'pet.smug',
}

const IDLE_CHECK_MS = 60_000
const IDLE_AFTER_MS = 4 * 60_000
const IDLE_MAX_PER_HOUR = 3

const auth = useAuthStore()
const store = useCompanionStore()
const { muted, checking } = useCompanionMute()
const route = useRoute()

store.load()

const open = ref(false)
const signedInToday = ref<boolean | null>(null)
const summary = ref<ActivitySummary | null>(null)
const signingIn = ref(false)
const signInError = ref(false)
const petting = ref(false)
const petError = ref(false)
const petLine = ref('')
const greetingLine = ref('')
const idleLine = ref('')
const expression = ref<CompanionQExpression>('neutral')
const blinking = ref(false)
const toggleRef = ref<HTMLButtonElement | null>(null)
const characterRef = ref<HTMLButtonElement | null>(null)
const panelRef = ref<HTMLElement | null>(null)

const { request: requestDialogue } = useCompanionDialogue()

let petCooldownUntil = 0
let lastInteractionAt = Date.now()
let dialogueToken = 0
let greetingToken = 0
let idleToken = 0
let petFallbackTimer: number | undefined
let expressionTimer: number | undefined
let greetingTimer: number | undefined
let blinkOnTimer: number | undefined
let blinkOffTimer: number | undefined
let idleTimer: number | undefined
let idleHour = -1
let idleCount = 0

const visible = computed(
  () =>
    auth.isLoggedIn &&
    store.effectiveEnabled &&
    !muted.value &&
    !checking.value &&
    !AUTH_ROUTES.has(String(route.name)),
)

const reducedMotion = computed(() => {
  if (store.motion === 'reduce') return true
  return typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
})

const gazeEnabled = computed(() => visible.value && !reducedMotion.value)
const { lookX, lookY } = useCompanionGaze({ target: characterRef, enabled: gazeEnabled })

/** A just-petted response wins, then the daily greeting, then the latest event. */
const line = computed(
  () =>
    petLine.value ||
    greetingLine.value ||
    idleLine.value ||
    store.latestEventLine ||
    pickCompanionLine(signedInToday.value === false ? 'signin' : 'greeting', localDateKey()),
)

/** Only spontaneous/pet lines float above the character; the default remains in the panel. */
const characterLine = computed(() => petLine.value || greetingLine.value || idleLine.value)

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

  lastInteractionAt = Date.now()
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

function clearExpressionTimer(): void {
  if (expressionTimer !== undefined) {
    window.clearTimeout(expressionTimer)
    expressionTimer = undefined
  }
}

function scheduleNeutral(): void {
  clearExpressionTimer()
  expressionTimer = window.setTimeout(() => {
    expression.value = 'neutral'
    expressionTimer = undefined
  }, 3000)
}

function clearPetFallback(): void {
  if (petFallbackTimer !== undefined) {
    window.clearTimeout(petFallbackTimer)
    petFallbackTimer = undefined
  }
}

/**
 * Pets once per interaction with an 800ms client-side guard. The server still
 * owns the once-per-day bond award; this guard only prevents a burst of
 * requests and expression flicker.
 */
async function handlePet(): Promise<void> {
  if (petting.value || Date.now() < petCooldownUntil) return

  petting.value = true
  petError.value = false
  lastInteractionAt = Date.now()
  idleLine.value = ''
  idleToken += 1
  cancelGreeting()
  const nextExpression = pickPetExpression(Math.random, expression.value)
  expression.value = nextExpression
  petCooldownUntil = Date.now() + 800
  const seed = String(Date.now())
  try {
    const result = await petCompanion()
    store.bond = result.bond
    store.level = result.level
    const staticLine = result.awarded
      ? pickCompanionLine(PET_LINE_KEYS[nextExpression], seed, petLine.value)
      : pickCompanionLine('pet.already', seed)
    // The pet response omits the level name, next threshold and daily gain, so
    // an awarded pet refreshes the full state to keep the meter honest.
    if (result.awarded) void refreshProgress()

    clearPetFallback()
    if (store.aiReady && store.aiDialogueEnabled) {
      const token = ++dialogueToken
      petLine.value = '……'
      petFallbackTimer = window.setTimeout(() => {
        if (token === dialogueToken) petLine.value = staticLine
      }, 1200)
      void requestDialogue({ trigger: 'pet', expression: nextExpression }).then((line) => {
        if (token !== dialogueToken) return
        clearPetFallback()
        if (petLine.value === '……' || petLine.value === staticLine) {
          petLine.value = line.line
        }
      })
    } else {
      petLine.value = staticLine
    }
  } catch {
    petError.value = true
    petLine.value = pickCompanionLine('pet', seed)
  } finally {
    petting.value = false
    scheduleNeutral()
  }
}

/** Best-effort read model refresh; a failure leaves the pet result in place. */
async function refreshProgress(): Promise<void> {
  try {
    store.applyRemoteState(await getCompanion())
  } catch {
    // The pet itself succeeded; the next sync will fill in the details.
  }
}

function clearGreetingTimer(): void {
  if (greetingTimer !== undefined) {
    window.clearTimeout(greetingTimer)
    greetingTimer = undefined
  }
}

function greetingKeyFor(hour: number): DialogueGreetingKey {
  if (hour >= 5 && hour < 12) return 'greet.morning'
  if (hour >= 12 && hour < 18) return 'greet.afternoon'
  return 'greet.night'
}

function scheduleGreeting(): void {
  clearGreetingTimer()
  if (!visible.value || store.activityLevel === 'quiet' || !store.shouldGreet()) return
  greetingTimer = window.setTimeout(() => {
    const key = greetingKeyFor(new Date().getHours())
    const staticLine = pickCompanionLine(key, localDateKey())
    greetingLine.value = staticLine
    store.markGreeted()
    greetingTimer = undefined
    if (store.aiReady && store.aiDialogueEnabled) {
      const token = ++greetingToken
      void requestDialogue({ trigger: 'greet', greetingKey: key }).then((line) => {
        if (token !== greetingToken) return
        if (greetingLine.value === staticLine) greetingLine.value = line.line
      })
    }
  }, 2000)
}

/** User interaction before the 2s greeting cancels it for today. */
function cancelGreeting(): void {
  clearGreetingTimer()
  greetingToken += 1
  if (!greetingLine.value) return
  greetingLine.value = ''
}

function clearBlink(): void {
  if (blinkOnTimer !== undefined) window.clearTimeout(blinkOnTimer)
  if (blinkOffTimer !== undefined) window.clearTimeout(blinkOffTimer)
  blinkOnTimer = undefined
  blinkOffTimer = undefined
  blinking.value = false
}

function scheduleBlink(): void {
  clearBlink()
  if (!visible.value || reducedMotion.value) return
  blinkOnTimer = window.setTimeout(() => {
    blinking.value = true
    blinkOffTimer = window.setTimeout(() => {
      blinking.value = false
      scheduleBlink()
    }, 120)
  }, 3000 + Math.random() * 4000)
}

function stopIdleTimer(): void {
  if (idleTimer !== undefined) {
    window.clearInterval(idleTimer)
    idleTimer = undefined
  }
}

function tickIdle(): void {
  if (!visible.value || store.activityLevel !== 'lively' || muted.value) return
  const hour = new Date().getHours()
  if (hour !== idleHour) {
    idleHour = hour
    idleCount = 0
  }
  if (Date.now() - lastInteractionAt < IDLE_AFTER_MS || idleCount >= IDLE_MAX_PER_HOUR) return
  idleCount += 1
  const token = ++idleToken
  const staticLine = pickCompanionLine('idle.lively', String(Date.now()))
  idleLine.value = staticLine
  if (!store.aiReady || !store.aiDialogueEnabled) return
  void requestDialogue({ trigger: 'idle' }).then((line) => {
    if (token === idleToken && idleLine.value === staticLine) idleLine.value = line.line
  })
}

function startIdleTimer(): void {
  stopIdleTimer()
  if (!visible.value || store.activityLevel !== 'lively') return
  idleTimer = window.setInterval(tickIdle, IDLE_CHECK_MS)
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
  lastInteractionAt = Date.now()
  idleLine.value = ''
  idleToken += 1
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
      clearGreetingTimer()
      clearBlink()
      stopIdleTimer()
      idleLine.value = ''
      return
    }
    void loadSummary()
    scheduleGreeting()
    scheduleBlink()
    startIdleTimer()
  },
  { immediate: true },
)

watch(
  () => store.activityLevel,
  () => {
    scheduleGreeting()
    startIdleTimer()
    if (store.activityLevel !== 'lively') {
      idleLine.value = ''
      idleToken += 1
    }
  },
)

watch(reducedMotion, () => scheduleBlink())

watch(
  () => route.fullPath,
  () => {
    open.value = false
    cancelGreeting()
    idleLine.value = ''
    idleToken += 1
  },
)

// The pet acknowledgement is transient: once the event poll delivers a newer
// line it takes over the bubble again.
watch(
  () => store.latestEventLine,
  () => {
    petLine.value = ''
  },
)

onMounted(() => {
  window.addEventListener('keydown', onWindowKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onWindowKeydown)
  clearExpressionTimer()
  clearGreetingTimer()
  clearBlink()
  clearPetFallback()
  stopIdleTimer()
  dialogueToken += 1
  greetingToken += 1
  idleToken += 1
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
        ref="characterRef"
        class="companion-dock__character"
        type="button"
        :aria-label="`摸摸 ${store.name}`"
        :disabled="petting"
        @click="handlePet"
      >
        <CompanionQAvatar
          :expression="expression"
          :look-x="lookX"
          :look-y="lookY"
          :blinking="blinking"
          frame="dock"
          :reduced-motion="reducedMotion"
        />
      </button>
      <button
        ref="toggleRef"
        class="companion-dock__toggle companion-dock__menu"
        type="button"
        :aria-expanded="open ? 'true' : 'false'"
        aria-controls="companion-dock-panel"
        :aria-label="open ? '收起伙伴面板' : '打开伙伴面板'"
        @click="toggle"
      >
        <span aria-hidden="true">⋯</span>
        <span class="companion-dock__dot" aria-hidden="true" />
      </button>
    </div>

    <CompanionBubble
      v-if="!open && characterLine"
      :text="characterLine"
      class="companion-dock__floating-bubble"
    />

    <p v-if="signInError" class="companion-dock__error">……线路断了。</p>

    <div
      v-if="open"
      id="companion-dock-panel"
      ref="panelRef"
      class="companion-dock__panel"
      role="region"
      :aria-label="store.name"
      tabindex="-1"
    >
      <div class="companion-dock__intro">
        <CompanionQAvatar
          :expression="expression"
          :size="64"
          :reduced-motion="reducedMotion"
        />
        <CompanionBubble :text="line" />
      </div>

      <CompanionBondMeter
        v-if="store.bond > 0"
        :bond="store.bond"
        :level="store.level"
        :level-name="store.levelName"
        :next-threshold="store.nextThreshold"
        :daily-gain="store.dailyGain"
        :daily-cap="store.dailyCap"
      />

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

      <button class="companion-dock__pet" type="button" :disabled="petting" @click="handlePet">
        {{ petting ? '抚摸中…' : '摸摸她' }}
      </button>

      <p v-if="petError" class="companion-dock__error">……线路断了。</p>

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
  gap: 6px;
  align-items: flex-end;
}

.companion-dock__character {
  display: inline-flex;
  width: 104px;
  height: 104px;
  padding: 0;
  cursor: pointer;
  background: transparent;
  border: 0;
  filter: drop-shadow(0 8px 16px rgb(0 0 0 / 22%));
  transition: transform 120ms ease;
}

.companion-dock__character:hover:not(:disabled) {
  transform: translateY(-2px);
}

.companion-dock__character:focus-visible {
  outline: 2px solid var(--kapoj-kapo);
  outline-offset: 2px;
  border-radius: 18px;
}

.companion-dock__character:disabled {
  cursor: default;
  opacity: 0.8;
}

.companion-dock__toggle {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  padding: 0;
  margin-bottom: 6px;
  font-size: 1.05rem;
  line-height: 1;
  color: var(--kapoj-text-secondary);
  cursor: pointer;
  background: var(--kapoj-surface);
  border: 1px solid var(--kapoj-border-strong);
  border-radius: 50%;
  box-shadow: var(--kapoj-shadow-md);
}

.companion-dock__toggle:hover {
  color: var(--kapoj-kapo-text);
  border-color: var(--kapoj-kapo);
}

.companion-dock__toggle:focus-visible {
  outline: 2px solid var(--kapoj-kapo);
  outline-offset: 2px;
}

.companion-dock__dot {
  position: absolute;
  top: 2px;
  right: 2px;
  width: 6px;
  height: 6px;
  background: var(--kapoj-kapo);
  border-radius: 50%;
}

.companion-dock__floating-bubble {
  max-width: 220px;
  margin: 0 4px 2px;
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

.companion-dock__intro :deep(.companion-q-avatar) {
  flex: none;
  width: 64px;
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
.companion-dock__pet,
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
.companion-dock__pet:hover:not(:disabled),
.companion-dock__quiet:hover:not(:disabled) {
  border-color: var(--kapoj-kapo);
}

.companion-dock__signin:disabled,
.companion-dock__pet:disabled,
.companion-dock__quiet:disabled {
  cursor: default;
  opacity: 0.55;
}

.companion-dock__signin:focus-visible,
.companion-dock__pet:focus-visible,
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

  .companion-dock__character {
    width: 88px;
    height: 88px;
  }

  .companion-dock__toggle {
    width: 28px;
    height: 28px;
  }

  .companion-dock__floating-bubble {
    max-width: 190px;
  }
}
</style>
