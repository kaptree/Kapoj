<script setup lang="ts">
import { computed } from 'vue'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'

import type { Me, UserActivity } from '@/api/types'
import RatingLevelName from '@/components/RatingLevelName.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import type { HomeDataState } from '@/composables/useHomeData'
import { resolveAssetUrl } from '@/utils/asset'
import { formatCoins } from '@/utils/gamification'

const props = defineProps<{
  user: Me
  activity: UserActivity | null
  activityState: HomeDataState
  presence: 'online' | 'away' | 'offline'
}>()

const initial = computed(() => Array.from(props.user.nickname.trim())[0] ?? '?')

const presenceMeta = computed(() => {
  switch (props.presence) {
    case 'offline':
      return { label: '离线', tone: 'offline', hint: '当前浏览器网络已断开' }
    case 'away':
      return { label: '离开', tone: 'away', hint: '页面当前不在前台' }
    default:
      return { label: '在线', tone: 'online', hint: '基于当前浏览器连接与页面可见状态' }
  }
})

const activityPending = computed(
  () => props.activityState === 'idle' || props.activityState === 'loading',
)
const activityFailed = computed(() => props.activityState === 'error')

const solved = computed(() =>
  activityFailed.value ? null : (props.activity?.acceptedProblemCount ?? null),
)
const streak = computed(() =>
  activityFailed.value ? null : (props.activity?.currentStreak ?? null),
)
</script>

<template>
  <AppSurface as="section" padding="none" class="profile-card" aria-labelledby="home-profile-title">
    <div class="profile-card__identity">
      <div class="profile-card__avatar-wrap">
        <Avatar
          :image="user.avatarUrl ? resolveAssetUrl(user.avatarUrl) : undefined"
          :label="initial"
          shape="square"
          class="profile-card__avatar"
        />
        <span
          class="profile-card__presence-dot"
          :class="`is-${presenceMeta.tone}`"
          :title="presenceMeta.hint"
          aria-hidden="true"
        />
      </div>

      <div class="profile-card__main">
        <p class="profile-card__eyebrow">PROFILE</p>
        <h2 id="home-profile-title" class="profile-card__name">
          <RatingLevelName :name="user.nickname" :level="user.level" inherit-typography />
        </h2>
        <div class="profile-card__meta">
          <span class="profile-card__username">@{{ user.username }}</span>
          <span
            class="profile-card__presence"
            :class="`is-${presenceMeta.tone}`"
            :title="presenceMeta.hint"
          >
            <span class="profile-card__presence-pulse" aria-hidden="true" />
            {{ presenceMeta.label }}
          </span>
        </div>

        <div class="profile-card__actions">
          <Button
            as="router-link"
            to="/settings/profile"
            label="编辑资料"
            icon="pi pi-user-edit"
            severity="secondary"
            outlined
            size="small"
          />
        </div>
      </div>
    </div>

    <dl class="profile-card__stats" :aria-busy="activityPending || undefined">
      <div class="profile-card__stat profile-card__stat--rating">
        <dt><i class="pi pi-chart-line" aria-hidden="true" /> Rating</dt>
        <dd class="num">{{ user.rating }}</dd>
      </div>

      <div class="profile-card__stat profile-card__stat--coins">
        <dt><i class="pi pi-wallet" aria-hidden="true" /> 金币</dt>
        <dd class="num">{{ formatCoins(user.coins) }}</dd>
      </div>

      <div class="profile-card__stat profile-card__stat--solved">
        <dt><i class="pi pi-check-circle" aria-hidden="true" /> 通过题目</dt>
        <dd v-if="activityPending" class="profile-card__skeleton" aria-label="正在加载通过题数">
          <span aria-hidden="true" />
        </dd>
        <dd v-else-if="activityFailed" class="num is-muted" title="通过题数暂时无法获取">—</dd>
        <dd v-else class="num">{{ solved ?? 0 }}</dd>
      </div>

      <div class="profile-card__stat profile-card__stat--streak">
        <dt><i class="pi pi-calendar" aria-hidden="true" /> 连续练习</dt>
        <dd v-if="activityPending" class="profile-card__skeleton" aria-label="正在加载连续练习天数">
          <span aria-hidden="true" />
        </dd>
        <dd v-else-if="activityFailed" class="num is-muted" title="连续练习天数暂时无法获取">—</dd>
        <dd v-else class="num">{{ streak ?? 0 }}<span class="profile-card__unit">天</span></dd>
      </div>
    </dl>
  </AppSurface>
</template>

<style scoped>
.profile-card {
  display: grid;
  grid-template-columns: minmax(260px, 0.9fr) minmax(0, 1.6fr);
  overflow: hidden;
}

.profile-card__identity {
  display: flex;
  gap: var(--kapoj-space-4);
  align-items: flex-start;
  min-width: 0;
  padding: var(--kapoj-space-5);
}

.profile-card__avatar-wrap {
  position: relative;
  flex: none;
}

.profile-card__avatar {
  width: 64px;
  height: 64px;
  color: var(--kapoj-brand);
  font-size: 24px;
  font-weight: 700;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-brand) 24%, var(--kapoj-border));
  border-radius: 18px;
}

.profile-card__presence-dot {
  position: absolute;
  right: -3px;
  bottom: -3px;
  width: 14px;
  height: 14px;
  background: var(--kapoj-text-muted);
  border: 3px solid var(--kapoj-surface);
  border-radius: 50%;
}

.profile-card__presence-dot.is-online {
  background: var(--kapoj-success);
}

.profile-card__presence-dot.is-away {
  background: var(--kapoj-warning);
}

.profile-card__presence-dot.is-offline {
  background: var(--kapoj-danger);
}

.profile-card__main {
  min-width: 0;
}

.profile-card__eyebrow {
  margin: 0;
  color: var(--kapoj-primary-strong);
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  font-weight: 600;
  letter-spacing: 0.14em;
}

.profile-card__name {
  margin: 3px 0 0;
  overflow: hidden;
  color: var(--kapoj-text);
  font-size: 1.3rem;
  font-weight: 650;
  line-height: 1.25;
  letter-spacing: -0.02em;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.profile-card__meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
  align-items: center;
  margin-top: 3px;
}

.profile-card__username {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
}

.profile-card__presence {
  display: inline-flex;
  gap: 5px;
  align-items: center;
  padding: 1px var(--kapoj-space-2);
  color: var(--kapoj-text-secondary);
  font-size: 10px;
  font-weight: 500;
  background: var(--kapoj-surface-raised);
  border: 1px solid var(--kapoj-border);
  border-radius: 999px;
}

.profile-card__presence-pulse {
  width: 5px;
  height: 5px;
  background: currentcolor;
  border-radius: 50%;
}

.profile-card__presence.is-online {
  color: var(--kapoj-success);
  background: color-mix(in srgb, var(--kapoj-success) 9%, transparent);
  border-color: color-mix(in srgb, var(--kapoj-success) 24%, transparent);
}

.profile-card__presence.is-away {
  color: var(--kapoj-warning);
  background: color-mix(in srgb, var(--kapoj-warning) 9%, transparent);
  border-color: color-mix(in srgb, var(--kapoj-warning) 24%, transparent);
}

.profile-card__presence.is-offline {
  color: var(--kapoj-danger);
  background: color-mix(in srgb, var(--kapoj-danger) 9%, transparent);
  border-color: color-mix(in srgb, var(--kapoj-danger) 24%, transparent);
}

.profile-card__actions {
  margin-top: var(--kapoj-space-3);
}

.profile-card__stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  align-items: stretch;
  padding: var(--kapoj-space-3) 0;
  margin: 0;
  border-left: 1px solid var(--kapoj-border);
}

.profile-card__stat {
  --stat-color: var(--kapoj-brand);

  display: flex;
  flex-direction: column;
  justify-content: center;
  min-width: 0;
  padding: var(--kapoj-space-2) var(--kapoj-space-5);
  border-left: 1px solid var(--kapoj-border);
}

.profile-card__stat:first-child {
  border-left: 0;
}

.profile-card__stat--coins {
  --stat-color: var(--kapoj-warning);
}

.profile-card__stat--solved {
  --stat-color: var(--kapoj-success);
}

.profile-card__stat--streak {
  --stat-color: var(--kapoj-special);
}

.profile-card__stat dt {
  display: flex;
  gap: 6px;
  align-items: center;
  overflow: hidden;
  color: var(--kapoj-text-muted);
  font-size: 11px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.profile-card__stat dt i {
  color: var(--stat-color);
  font-size: 11px;
}

.profile-card__stat dd {
  margin: var(--kapoj-space-1) 0 0;
  overflow: hidden;
  color: var(--kapoj-text);
  font-size: clamp(1.45rem, 1.2rem + 0.55vw, 1.9rem);
  font-weight: 700;
  line-height: 1.1;
  letter-spacing: -0.03em;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.profile-card__stat dd.is-muted {
  color: var(--kapoj-text-muted);
}

.profile-card__unit {
  margin-left: 3px;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-sans);
  font-size: 11px;
  font-weight: 500;
  letter-spacing: 0;
}

.profile-card__skeleton {
  width: min(72%, 3.4rem);
  height: 1.6rem;
  margin-top: 6px !important;
  overflow: hidden;
  background: var(--kapoj-surface-hover);
  border-radius: var(--kapoj-radius-xs);
}

.profile-card__skeleton span {
  display: block;
  width: 100%;
  height: 100%;
  background: linear-gradient(
    90deg,
    transparent,
    color-mix(in srgb, var(--kapoj-text-muted) 12%, transparent),
    transparent
  );
  transform: translateX(-100%);
  animation: profile-skeleton 1.3s ease-in-out infinite;
}

@keyframes profile-skeleton {
  to {
    transform: translateX(100%);
  }
}

@media (max-width: 1000px) {
  .profile-card {
    grid-template-columns: minmax(0, 1fr);
  }

  .profile-card__identity {
    padding-bottom: var(--kapoj-space-4);
  }

  .profile-card__stats {
    padding: 0;
    border-top: 1px solid var(--kapoj-border);
    border-left: 0;
  }

  .profile-card__stat {
    padding: var(--kapoj-space-4) var(--kapoj-space-5);
  }
}

@media (max-width: 640px) {
  .profile-card__identity {
    gap: var(--kapoj-space-3);
    padding: var(--kapoj-space-4);
  }

  .profile-card__avatar {
    width: 56px;
    height: 56px;
    font-size: 20px;
    border-radius: 16px;
  }

  .profile-card__stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .profile-card__stat {
    padding: var(--kapoj-space-4);
    border-top: 1px solid var(--kapoj-border);
    border-left: 0;
  }

  .profile-card__stat:nth-child(1),
  .profile-card__stat:nth-child(2) {
    border-top: 0;
  }

  .profile-card__stat:nth-child(even) {
    border-left: 1px solid var(--kapoj-border);
  }
}

@media (prefers-reduced-motion: reduce) {
  .profile-card__skeleton span {
    animation: none;
  }
}
</style>
