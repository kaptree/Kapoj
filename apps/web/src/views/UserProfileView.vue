<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Avatar from 'primevue/avatar'

import { ApiError } from '@/api/http'
import { getProfile } from '@/api/users'
import type { Profile } from '@/api/types'
import IdentityShowcase from '@/components/IdentityShowcase.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'

const route = useRoute()
const profile = ref<Profile | null>(null)
const error = ref('')
const loading = ref(false)
const avatarFailed = ref(false)

const initial = computed(() => {
  const name = profile.value?.nickname || profile.value?.username || '?'
  return name.slice(0, 1)
})
const registrationDate = computed(() =>
  profile.value ? new Date(profile.value.createdAt).toLocaleDateString('zh-CN') : '',
)
const description = computed(() =>
  profile.value ? `@${profile.value.username}` : '查看选手公开资料、身份与成长数据。',
)

async function loadProfile(id: string) {
  loading.value = true
  error.value = ''
  profile.value = null
  avatarFailed.value = false
  try {
    profile.value = await getProfile(id)
  } catch (e) {
    error.value = e instanceof ApiError && e.code === 40400 ? '用户不存在' : '加载失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

function retry() {
  const id = route.params.id
  if (typeof id === 'string') void loadProfile(id)
}

watch(
  () => route.params.id,
  (id) => {
    if (typeof id === 'string') void loadProfile(id)
  },
  { immediate: true },
)
</script>

<template>
  <div class="profile-page">
    <AppPageHeader
      eyebrow="USER // PROFILE"
      :title="profile?.nickname ?? '用户主页'"
      :description="description"
    />

    <AppLoadingState v-if="loading" label="正在加载用户主页" />

    <AppErrorState v-else-if="error" :description="error" @retry="retry" />

    <AppSurface v-else-if="profile" padding="lg" class="profile-surface">
      <div class="profile-head">
        <Avatar
          :image="!avatarFailed && profile.avatarUrl ? profile.avatarUrl : undefined"
          :label="!avatarFailed && profile.avatarUrl ? undefined : initial"
          size="xlarge"
          shape="circle"
          class="profile-avatar"
          :aria-label="`用户头像：${profile.nickname}`"
          @error="avatarFailed = true"
        />
        <div class="profile-identity">
          <IdentityShowcase
            :badges="profile.equippedBadges"
            :title="profile.equippedTitle"
            size="small"
          />
          <p class="profile-bio" :class="{ muted: !profile.bio }">
            {{ profile.bio || '这个人还没有写简介。' }}
          </p>
        </div>
      </div>

      <dl class="profile-stats">
        <div>
          <dt>Rating</dt>
          <dd class="num">{{ profile.rating }}</dd>
        </div>
        <div>
          <dt>金币</dt>
          <dd class="num">{{ profile.coins }}</dd>
        </div>
        <div>
          <dt>注册时间</dt>
          <dd>{{ registrationDate }}</dd>
        </div>
      </dl>
    </AppSurface>
  </div>
</template>

<style scoped>
.profile-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: 720px;
}

.profile-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
}

.profile-head {
  display: flex;
  align-items: flex-start;
  gap: var(--kapoj-space-4);
}

.profile-avatar {
  flex: 0 0 auto;
  color: var(--kapoj-primary);
  font-size: 1.35rem;
  font-weight: 700;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 30%, transparent);
  box-shadow: var(--kapoj-glow-primary);
}

.profile-identity {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  min-width: 0;
}

.profile-bio {
  max-width: 58ch;
  margin: 0;
  color: var(--kapoj-text);
  font-size: 0.9rem;
  line-height: 1.65;
  white-space: pre-wrap;
}

.profile-bio.muted {
  color: var(--kapoj-text-muted);
}

.profile-stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--kapoj-space-3);
  padding-top: var(--kapoj-space-4);
  margin: 0;
  border-top: 1px solid var(--kapoj-border);
}

.profile-stats > div {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
  min-width: 0;
  padding: var(--kapoj-space-3);
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.profile-stats dt {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.profile-stats dd {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.15rem;
  font-weight: 600;
}

@media (max-width: 520px) {
  .profile-head {
    flex-direction: column;
  }

  .profile-stats {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
