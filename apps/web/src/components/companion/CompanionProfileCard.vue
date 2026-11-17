<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'

import type { UserCompanion } from '@/api/types'
import CompanionAvatar from './CompanionAvatar.vue'

const props = defineProps<{
  state: UserCompanion | null
  isSelf: boolean
  name: string
}>()

/** A missing or zero threshold keeps the track empty instead of dividing by it. */
const fillPercent = computed(() => {
  if (!props.state) return 0
  const threshold = props.state.nextThreshold
  if (!Number.isFinite(threshold) || threshold <= 0) return 0
  return Math.min(100, Math.max(0, (props.state.bond / threshold) * 100))
})

const maxThreshold = computed(() => Math.max(0, props.state?.nextThreshold ?? 0))
const showPrivateHint = computed(() =>
  Boolean(props.state && props.isSelf && !props.state.publicProfile),
)
</script>

<template>
  <section
    v-if="state"
    class="companion-profile-card"
    data-test="companion-profile-card"
    aria-labelledby="companion-profile-name"
  >
    <div class="companion-profile-card__portrait">
      <CompanionAvatar pose="bust" size="md" :alt="`${name} 的伙伴头像`" />
    </div>

    <div class="companion-profile-card__body">
      <p class="companion-profile-card__eyebrow">COMPANION // PROFILE</p>

      <div class="companion-profile-card__head">
        <h2 id="companion-profile-name" class="companion-profile-card__name">{{ name }}</h2>
        <span class="companion-profile-card__level"
          >Lv.{{ state.level }} {{ state.levelName }}</span
        >
      </div>

      <div class="companion-profile-card__bond">
        <p class="companion-profile-card__bond-text">
          <span class="num">{{ state.bond }}</span> / {{ state.nextThreshold }}
        </p>
        <div
          class="companion-profile-card__track"
          role="progressbar"
          :aria-label="`与${name}的羁绊`"
          aria-valuemin="0"
          :aria-valuemax="maxThreshold"
          :aria-valuenow="state.bond"
        >
          <span class="companion-profile-card__fill" :style="{ width: `${fillPercent}%` }" />
        </div>
      </div>

      <p class="companion-profile-card__days">相伴 {{ state.daysTogether }} 天</p>

      <div class="companion-profile-card__skins">
        <span class="companion-profile-card__skins-label">换装</span>
        <ul v-if="state.unlockedSkins.length" class="companion-profile-card__skin-list">
          <li v-for="skin in state.unlockedSkins" :key="skin" class="companion-profile-card__skin">
            {{ skin }}
          </li>
        </ul>
        <span v-else class="companion-profile-card__skins-empty">还没有换装</span>
      </div>

      <RouterLink v-if="showPrivateHint" to="/appearance" class="companion-profile-card__private">
        <i class="pi pi-eye-slash" aria-hidden="true" />
        仅自己可见 · 前往外观设置
      </RouterLink>
    </div>
  </section>
</template>

<style scoped>
.companion-profile-card {
  display: flex;
  align-items: center;
  gap: clamp(16px, 2vw, 24px);
  min-width: 0;
  padding: clamp(16px, 1.8vw, 22px) clamp(18px, 2.2vw, 28px);
  background:
    radial-gradient(
      120% 160% at 0% 0%,
      color-mix(in srgb, var(--kapoj-kapo) 7%, transparent),
      transparent 60%
    ),
    var(--kapoj-surface);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
  box-shadow: var(--kapoj-shadow-sm);
}

.companion-profile-card__portrait {
  display: grid;
  flex: 0 0 auto;
  width: 84px;
  height: 98px;
  background: color-mix(in srgb, var(--kapoj-kapo) 8%, var(--kapoj-surface));
  border: 1px solid color-mix(in srgb, var(--kapoj-kapo) 22%, var(--kapoj-border));
  border-radius: 14px;
  place-items: center;
}

.companion-profile-card__portrait :deep(.companion-avatar) {
  width: 68px;
}

.companion-profile-card__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
  gap: 6px;
}

.companion-profile-card__eyebrow {
  margin: 0;
  color: var(--kapoj-kapo);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.14em;
}

.companion-profile-card__head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 6px 10px;
  min-width: 0;
}

.companion-profile-card__name {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.15rem;
  font-weight: 700;
  letter-spacing: -0.02em;
  overflow-wrap: anywhere;
}

.companion-profile-card__level {
  padding: 2px 9px;
  color: var(--kapoj-kapo);
  font-size: 12px;
  font-weight: 600;
  background: color-mix(in srgb, var(--kapoj-kapo) 9%, var(--kapoj-surface));
  border: 1px solid color-mix(in srgb, var(--kapoj-kapo) 26%, var(--kapoj-border));
  border-radius: 999px;
}

.companion-profile-card__bond {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-width: 320px;
}

.companion-profile-card__bond-text {
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 12px;
  font-weight: 600;
}

.companion-profile-card__track {
  height: 6px;
  overflow: hidden;
  background: color-mix(in srgb, var(--kapoj-kapo) 12%, var(--kapoj-border));
  border-radius: 999px;
}

.companion-profile-card__fill {
  display: block;
  height: 100%;
  background: var(--kapoj-kapo);
  border-radius: inherit;
  transition: width 200ms ease;
}

.companion-profile-card__days {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.companion-profile-card__skins {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.companion-profile-card__skins-label,
.companion-profile-card__skins-empty {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.companion-profile-card__skins-label {
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  letter-spacing: 0.08em;
}

.companion-profile-card__skin-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 0;
  margin: 0;
  list-style: none;
}

.companion-profile-card__skin {
  padding: 2px 8px;
  color: var(--kapoj-text-secondary);
  font-size: 11px;
  background: var(
    --kapoj-surface-muted,
    color-mix(in srgb, var(--kapoj-text) 5%, var(--kapoj-surface))
  );
  border: 1px solid var(--kapoj-border);
  border-radius: 999px;
}

.companion-profile-card__private {
  display: inline-flex;
  align-items: center;
  align-self: flex-start;
  gap: 6px;
  margin-top: 2px;
  color: var(--kapoj-kapo);
  font-size: 12px;
  font-weight: 600;
  text-decoration: none;
}

.companion-profile-card__private:hover {
  text-decoration: underline;
}

.companion-profile-card__private i {
  font-size: 11px;
}

@media (max-width: 640px) {
  .companion-profile-card {
    align-items: flex-start;
  }

  .companion-profile-card__portrait {
    width: 72px;
    height: 84px;
  }

  .companion-profile-card__portrait :deep(.companion-avatar) {
    width: 58px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .companion-profile-card__fill {
    transition: none;
  }
}
</style>
