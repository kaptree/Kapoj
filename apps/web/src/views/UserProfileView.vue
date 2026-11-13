<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import { followUser, getUserSocial, listUserEvaluations, unfollowUser } from '@/api/social'
import { getProfile, getUserActivity } from '@/api/users'
import type { EvaluationPage, Profile, SocialView, UserActivity } from '@/api/types'
import ContributionGrid from '@/components/profile/ContributionGrid.vue'
import ProfileCertificatesCard from '@/components/profile/ProfileCertificatesCard.vue'
import ProfileEvaluationDialog from '@/components/profile/ProfileEvaluationDialog.vue'
import ProfileWordCloud from '@/components/profile/ProfileWordCloud.vue'
import FollowListDialog from '@/components/social/FollowListDialog.vue'
import IdentityShowcase from '@/components/IdentityShowcase.vue'
import RatingHistoryCard from '@/components/RatingHistoryCard.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const toast = useToast()
const profile = ref<Profile | null>(null)
const social = ref<SocialView | null>(null)
const activity = ref<UserActivity | null>(null)
const error = ref('')
const loading = ref(false)
const activityLoading = ref(false)
const activityError = ref('')
const evaluationPage = ref<EvaluationPage | null>(null)
const evaluationLoading = ref(false)
const evaluationError = ref('')
const evaluationDialogOpen = ref(false)
const avatarFailed = ref(false)
const followBusy = ref(false)
const followDialogOpen = ref(false)
const followDialogTab = ref<'followers' | 'following'>('followers')
let latestLoad = 0

const initial = computed(() => {
  const name = profile.value?.nickname || profile.value?.username || '?'
  return name.slice(0, 1)
})
const registrationDate = computed(() =>
  profile.value ? new Date(profile.value.createdAt).toLocaleDateString('zh-CN') : '',
)
const evaluationTotal = computed(() => evaluationPage.value?.total ?? 0)
const canEvaluate = computed(() => evaluationPage.value?.canEvaluate ?? false)
const myEvaluation = computed(() => evaluationPage.value?.myEvaluation ?? null)
const isSelf = computed(() => {
  const meId = auth.user?.id
  return meId != null && profile.value?.id === meId
})
const isLoggedIn = computed(() => auth.isLoggedIn)
const loginTarget = computed(() => ({ name: 'login', query: { redirect: route.fullPath } }))
const followLabel = computed(() => (social.value?.isFollowing ? '已关注' : '关注'))

async function loadActivity(id: string, seq: number) {
  activityLoading.value = true
  activityError.value = ''
  try {
    const data = await getUserActivity(id)
    if (seq !== latestLoad) return
    activity.value = data
  } catch (e) {
    if (seq !== latestLoad) return
    activity.value = null
    activityError.value = e instanceof ApiError ? e.message : '代码提交贡献加载失败，请稍后再试'
  } finally {
    if (seq === latestLoad) activityLoading.value = false
  }
}

async function loadEvaluations(id: string, seq: number) {
  evaluationLoading.value = true
  evaluationError.value = ''
  try {
    const data = await listUserEvaluations(id, 1, 1)
    if (seq !== latestLoad) return
    evaluationPage.value = data
  } catch (e) {
    if (seq !== latestLoad) return
    evaluationPage.value = null
    evaluationError.value = e instanceof ApiError ? e.message : '评价加载失败，请稍后再试'
  } finally {
    if (seq === latestLoad) evaluationLoading.value = false
  }
}

async function loadProfile(id: string) {
  const seq = ++latestLoad
  loading.value = true
  error.value = ''
  profile.value = null
  social.value = null
  activity.value = null
  activityError.value = ''
  evaluationPage.value = null
  evaluationError.value = ''
  evaluationDialogOpen.value = false
  avatarFailed.value = false
  void loadActivity(id, seq)
  void loadEvaluations(id, seq)
  try {
    const [profileData, socialData] = await Promise.all([
      getProfile(id),
      getUserSocial(id).catch(() => null),
    ])
    if (seq !== latestLoad) return
    profile.value = profileData
    social.value = socialData
  } catch (e) {
    if (seq !== latestLoad) return
    error.value = e instanceof ApiError && e.code === 40400 ? '用户不存在' : '加载失败，请稍后再试'
  } finally {
    if (seq === latestLoad) loading.value = false
  }
}

function retry() {
  const id = route.params.id
  if (typeof id === 'string') void loadProfile(id)
}

function retryActivity() {
  const id = route.params.id
  if (typeof id === 'string') void loadActivity(id, latestLoad)
}

function retryEvaluations() {
  const id = route.params.id
  if (typeof id === 'string') void loadEvaluations(id, latestLoad)
}

function reloadEvaluations() {
  const id = route.params.id
  if (typeof id === 'string') void loadEvaluations(id, latestLoad)
}

function openFollowList(tab: 'followers' | 'following') {
  followDialogTab.value = tab
  followDialogOpen.value = true
}

function goLogin() {
  void router.push(loginTarget.value)
}

async function toggleFollow() {
  if (!profile.value) return
  if (!auth.isLoggedIn) {
    void router.push(loginTarget.value)
    return
  }
  const wasFollowing = social.value?.isFollowing ?? false
  followBusy.value = true
  try {
    const result = wasFollowing
      ? await unfollowUser(profile.value.id)
      : await followUser(profile.value.id)
    social.value = {
      followersCount: result.followersCount,
      followingCount: social.value?.followingCount ?? 0,
      isFollowing: result.following,
    }
    toast.add({
      severity: 'success',
      summary: result.following ? '已关注' : '已取消关注',
      life: 3000,
    })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: e instanceof ApiError ? e.message : '操作失败，请稍后再试',
      life: 3000,
    })
  } finally {
    followBusy.value = false
  }
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
    <AppLoadingState v-if="loading" label="正在加载用户主页" />

    <AppErrorState v-else-if="error" :description="error" @retry="retry" />

    <template v-else-if="profile">
      <section class="profile-hero">
        <div class="profile-hero__identity">
          <Avatar
            :image="!avatarFailed && profile.avatarUrl ? profile.avatarUrl : undefined"
            :label="!avatarFailed && profile.avatarUrl ? undefined : initial"
            size="xlarge"
            shape="circle"
            class="profile-avatar"
            :aria-label="`用户头像：${profile.nickname}`"
            @error="avatarFailed = true"
          />
          <div class="profile-hero__names">
            <p class="profile-hero__eyebrow">USER // PROFILE</p>
            <h1 class="profile-hero__title">{{ profile.nickname }}</h1>
            <p class="profile-hero__handle">@{{ profile.username }}</p>
          </div>
        </div>

        <ProfileWordCloud
          class="profile-hero__cloud"
          :cloud="evaluationPage?.cloud ?? []"
          :total="evaluationTotal"
          :loading="evaluationLoading"
          :error="evaluationError"
          @open="evaluationDialogOpen = true"
          @retry="retryEvaluations"
        />
      </section>

      <AppSurface padding="lg" class="profile-surface">
        <div class="profile-intro">
          <IdentityShowcase
            :badges="profile.equippedBadges"
            :title="profile.equippedTitle"
            size="small"
          />
          <p class="profile-bio" :class="{ muted: !profile.bio }">
            {{ profile.bio || '这个人还没有写简介。' }}
          </p>
        </div>

        <div class="social-row">
          <button
            type="button"
            class="social-stat social-stat--followers"
            @click="openFollowList('followers')"
          >
            <span class="social-stat__num">{{ social?.followersCount ?? 0 }}</span>
            <span class="social-stat__label">粉丝</span>
          </button>
          <button
            type="button"
            class="social-stat social-stat--following"
            @click="openFollowList('following')"
          >
            <span class="social-stat__num">{{ social?.followingCount ?? 0 }}</span>
            <span class="social-stat__label">关注</span>
          </button>
          <template v-if="!isSelf">
            <Button
              v-if="isLoggedIn"
              :label="followLabel"
              :icon="social?.isFollowing ? 'pi pi-user-minus' : 'pi pi-user-plus'"
              :severity="social?.isFollowing ? 'secondary' : 'primary'"
              :outlined="social?.isFollowing"
              :loading="followBusy"
              class="follow-button"
              @click="toggleFollow"
            />
            <Button
              v-else
              label="登录后关注"
              icon="pi pi-sign-in"
              class="follow-button"
              @click="goLogin"
            />
          </template>
        </div>

        <dl class="profile-stats">
          <div class="career-card" data-test="career-card">
            <dt>提交 / 通过</dt>
            <dd class="career-card__values">
              <span class="career-card__metric">
                <strong class="num">{{ activity?.submissionCount ?? '—' }}</strong>
                <span class="career-card__label">提交总数</span>
              </span>
              <span class="career-card__metric">
                <strong class="num">{{ activity?.acceptedProblemCount ?? '—' }}</strong>
                <span class="career-card__label">通过题数</span>
              </span>
            </dd>
          </div>
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

      <div class="profile-body">
        <div class="profile-main">
          <AppSurface padding="lg" class="profile-card">
            <ContributionGrid v-if="activity" :activity="activity" />
            <AppLoadingState v-else-if="activityLoading" label="正在加载代码提交贡献图" />
            <AppErrorState
              v-else-if="activityError"
              data-test="activity-error"
              title="贡献图加载失败"
              :description="activityError"
              @retry="retryActivity"
            />
          </AppSurface>

          <RatingHistoryCard :user-id="profile.id" :rating="profile.rating" />
        </div>

        <aside class="profile-aside" data-test="profile-aside">
          <ProfileCertificatesCard
            :target-user-id="profile.id"
            class="profile-aside__card"
            data-test="certificate-slot"
          />
        </aside>
      </div>

      <ProfileEvaluationDialog
        v-model:visible="evaluationDialogOpen"
        :target-user-id="profile.id"
        :target-active="true"
        :my-evaluation="myEvaluation"
        :can-evaluate="canEvaluate"
        :is-self="isSelf"
        :is-logged-in="isLoggedIn"
        @saved="reloadEvaluations"
        @deleted="reloadEvaluations"
      />
    </template>

    <FollowListDialog
      v-if="profile"
      v-model:visible="followDialogOpen"
      :user-id="profile.id"
      :initial-tab="followDialogTab"
    />
  </div>
</template>

<style scoped>
.profile-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.profile-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.1fr);
  gap: var(--kapoj-space-5);
  align-items: stretch;
}

.profile-hero__identity {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: var(--kapoj-space-4);
  padding: var(--kapoj-space-2) 0;
}

.profile-hero__names {
  min-width: 0;
}

.profile-hero__eyebrow {
  margin: 0 0 var(--kapoj-space-1);
  color: var(--kapoj-primary);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.profile-hero__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: clamp(1.35rem, 1.1rem + 1vw, 1.9rem);
  line-height: 1.2;
  letter-spacing: -0.02em;
}

.profile-hero__handle {
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 0.875rem;
}

.profile-hero__cloud {
  min-width: 0;
}

.profile-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
}

.profile-intro {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: var(--kapoj-space-2);
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

.social-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-3);
}

.social-stat {
  display: inline-flex;
  align-items: baseline;
  gap: var(--kapoj-space-2);
  padding: 0;
  color: var(--kapoj-text);
  cursor: pointer;
  background: none;
  border: none;
}

.social-stat:hover .social-stat__num {
  color: var(--kapoj-primary);
}

.social-stat__num {
  font-size: 1.1rem;
  font-weight: 700;
}

.social-stat__label {
  color: var(--kapoj-text-muted);
  font-size: 0.8125rem;
}

.follow-button {
  margin-left: auto;
}

.profile-stats {
  display: grid;
  grid-template-columns: 1.25fr repeat(3, minmax(0, 1fr));
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

.career-card__values {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2) var(--kapoj-space-4);
  align-items: flex-end;
}

.career-card__metric {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.career-card__metric strong {
  font-size: 1.5rem;
  line-height: 1.1;
}

.career-card__label {
  color: var(--kapoj-text-muted);
  font-size: 0.75rem;
  font-weight: 500;
}

.profile-body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 340px;
  gap: var(--kapoj-space-4);
  align-items: start;
}

.profile-main,
.profile-aside {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
}

.profile-card {
  min-width: 0;
}

@media (max-width: 900px) {
  .profile-body {
    grid-template-columns: minmax(0, 1fr);
  }

  .profile-hero {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 640px) {
  .profile-stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .career-card {
    grid-column: span 2;
  }
}

@media (max-width: 520px) {
  .profile-hero__identity {
    align-items: flex-start;
  }
}

@media (max-width: 420px) {
  .profile-stats {
    grid-template-columns: minmax(0, 1fr);
  }

  .career-card {
    grid-column: auto;
  }
}
</style>
