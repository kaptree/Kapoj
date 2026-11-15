<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import { followUser, getUserSocial, listUserEvaluations, unfollowUser } from '@/api/social'
import { getProfile, getUserActivity } from '@/api/users'
import type { EvaluationPage, Profile, UserActivity } from '@/api/types'
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
import { resolveAssetUrl } from '@/utils/asset'
import { safeTitleColor } from '@/utils/gamification'

type ProfileSocial = {
  followersCount: number | null
  followingCount: number | null
  isFollowing: boolean
}

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const toast = useToast()
const profile = ref<Profile | null>(null)
const social = ref<ProfileSocial | null>(null)
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

const displayName = computed(() => profile.value?.nickname.trim() || profile.value?.username || '?')
const initial = computed(() => displayName.value.slice(0, 1))
const registrationDate = computed(() =>
  profile.value ? new Date(profile.value.createdAt).toLocaleDateString('zh-CN') : '',
)
const evaluationTotal = computed(() => evaluationPage.value?.total ?? null)
const canEvaluate = computed(() => evaluationPage.value?.canEvaluate ?? false)
const myEvaluation = computed(() => evaluationPage.value?.myEvaluation ?? null)
const followersCount = computed(() => social.value?.followersCount)
const followingCount = computed(() => social.value?.followingCount)
const isSelf = computed(() => {
  const meId = auth.user?.id
  return meId != null && profile.value?.id === meId
})
const isLoggedIn = computed(() => auth.isLoggedIn)
const loginTarget = computed(() => ({ name: 'login', query: { redirect: route.fullPath } }))
const followLabel = computed(() => (social.value?.isFollowing ? '已关注' : '关注'))

const titleStyle = computed<Record<string, string>>((): Record<string, string> => {
  const title = profile.value?.equippedTitle
  return title ? { '--profile-title-color': safeTitleColor(title.color, '#1595a8') } : {}
})

const stats = computed(() => [
  {
    key: 'submissions',
    label: '提交总数',
    value: activity.value?.submissionCount ?? '—',
    icon: 'pi pi-send',
    tone: 'teal',
  },
  {
    key: 'accepted',
    label: '通过题数',
    value: activity.value?.acceptedProblemCount ?? '—',
    icon: 'pi pi-check-circle',
    tone: 'green',
  },
  {
    key: 'rating',
    label: 'Rating',
    value: profile.value?.rating ?? '—',
    icon: 'pi pi-chart-line',
    tone: 'blue',
  },
  {
    key: 'coins',
    label: '金币',
    value: profile.value?.coins ?? '—',
    icon: 'pi pi-wallet',
    tone: 'amber',
  },
])

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
      followingCount: social.value?.followingCount ?? null,
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
      <section class="profile-hero" aria-labelledby="profile-name">
        <article class="profile-hero__card">
          <div class="profile-hero__decor" aria-hidden="true">
            <span class="profile-hero__code profile-hero__code--angle">&lt;/&gt;</span>
            <span class="profile-hero__code profile-hero__code--braces">{ }</span>
            <span class="profile-hero__code profile-hero__code--const">const user</span>
            <span class="profile-hero__circuit profile-hero__circuit--a" />
            <span class="profile-hero__circuit profile-hero__circuit--b" />
          </div>

          <div class="profile-hero__content">
            <div class="profile-hero__main">
              <div class="profile-avatar-wrap">
                <Avatar
                  :image="
                    !avatarFailed && profile.avatarUrl
                      ? resolveAssetUrl(profile.avatarUrl)
                      : undefined
                  "
                  :label="!avatarFailed && profile.avatarUrl ? undefined : initial"
                  shape="circle"
                  class="profile-avatar"
                  :aria-label="`用户头像：${displayName}`"
                  @error="avatarFailed = true"
                />
              </div>

              <div class="profile-hero__identity">
                <p class="profile-hero__eyebrow">
                  <span class="profile-hero__eyebrow-dot" aria-hidden="true" />
                  USER // PROFILE
                </p>

                <div class="profile-hero__name-row">
                  <h1 id="profile-name" class="profile-hero__title">{{ displayName }}</h1>
                  <span
                    v-if="profile.equippedTitle"
                    class="profile-title-badge"
                    :style="titleStyle"
                  >
                    <i class="pi pi-star-fill" aria-hidden="true" />
                    {{ profile.equippedTitle.name }}
                  </span>
                </div>

                <p class="profile-hero__handle">
                  <span>@{{ profile.username }}</span>
                  <span class="profile-hero__sep" aria-hidden="true">·</span>
                  <span>UID {{ profile.id }}</span>
                </p>
              </div>
            </div>

            <p
              class="profile-bio"
              :class="{ 'is-empty': !profile.bio }"
              :title="profile.bio || undefined"
            >
              {{ profile.bio || '这个人还没有写简介。' }}
            </p>

            <div v-if="profile.equippedBadges.length" class="profile-hero__badges">
              <span class="profile-hero__badges-label">社区徽章</span>
              <IdentityShowcase :badges="profile.equippedBadges" :title="null" size="small" />
            </div>

            <div class="profile-hero__meta">
              <span class="profile-hero__meta-item">
                <i class="pi pi-calendar" aria-hidden="true" />
                注册时间 {{ registrationDate }}
              </span>
            </div>

            <div class="profile-hero__footer">
              <div class="social-row">
                <button
                  type="button"
                  class="social-stat social-stat--followers"
                  :aria-label="
                    followersCount == null ? '查看粉丝列表' : `查看粉丝列表，${followersCount} 人`
                  "
                  @click="openFollowList('followers')"
                >
                  <strong class="social-stat__num">{{ followersCount ?? '—' }}</strong>
                  <span class="social-stat__label">粉丝</span>
                </button>
                <span class="social-row__divider" aria-hidden="true" />
                <button
                  type="button"
                  class="social-stat social-stat--following"
                  :aria-label="
                    followingCount == null ? '查看关注列表' : `查看关注列表，${followingCount} 人`
                  "
                  @click="openFollowList('following')"
                >
                  <strong class="social-stat__num">{{ followingCount ?? '—' }}</strong>
                  <span class="social-stat__label">关注</span>
                </button>
              </div>

              <div class="profile-hero__actions">
                <RouterLink
                  v-if="isSelf"
                  :to="{ name: 'profile-edit' }"
                  class="profile-action profile-action--ghost"
                >
                  <i class="pi pi-pen-to-square" aria-hidden="true" />
                  编辑资料
                </RouterLink>
                <template v-else-if="isLoggedIn">
                  <Button
                    :label="followLabel"
                    :icon="social?.isFollowing ? 'pi pi-user-minus' : 'pi pi-user-plus'"
                    :severity="social?.isFollowing ? 'secondary' : 'primary'"
                    :outlined="social?.isFollowing"
                    :loading="followBusy"
                    class="follow-button"
                    @click="toggleFollow"
                  />
                </template>
                <Button
                  v-else
                  label="登录后关注"
                  icon="pi pi-sign-in"
                  class="follow-button"
                  @click="goLogin"
                />
              </div>
            </div>
          </div>
        </article>

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

      <dl
        class="profile-stats"
        data-test="career-card"
        aria-label="个人数据统计"
        :aria-busy="activityLoading"
      >
        <div
          v-for="(stat, index) in stats"
          :key="stat.key"
          class="stat-card"
          :class="`stat-card--${stat.tone}`"
          :style="{ '--stat-delay': `${index * 60}ms` }"
        >
          <span class="stat-card__icon" aria-hidden="true">
            <i :class="stat.icon" />
          </span>
          <div class="stat-card__body">
            <dt class="stat-card__label">{{ stat.label }}</dt>
            <dd class="stat-card__value num">{{ stat.value }}</dd>
          </div>
        </div>
      </dl>

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
  --profile-primary: var(--kapoj-primary);
  --profile-primary-text: var(--kapoj-primary);
  --profile-primary-soft: var(--kapoj-primary-soft);
  --profile-success: var(--kapoj-success);
  --profile-success-text: var(--kapoj-success);
  --profile-success-soft: color-mix(in srgb, var(--profile-success) 10%, transparent);
  --profile-text: var(--kapoj-text);
  --profile-text-secondary: var(--kapoj-text-secondary);
  --profile-text-muted: #7c8ca1;
  --profile-text-faint: #8a9aaf;
  --profile-surface: var(--kapoj-surface);
  --profile-surface-muted: color-mix(in srgb, var(--kapoj-primary) 4%, var(--kapoj-surface));
  --profile-border: var(--kapoj-border);
  --profile-border-strong: var(--kapoj-border-strong);
  --profile-shadow: var(--kapoj-shadow-sm);
  --profile-shadow-hover: var(--kapoj-shadow-md);
  --profile-radius: 14px;
  --profile-gap: 20px;

  /* Keep every AppSurface on this page on the same radius and shadow scale. */
  --kapoj-radius-md: var(--profile-radius);
  --kapoj-text-muted: var(--profile-text-muted);
  --kapoj-border-focus: var(--kapoj-primary);

  display: flex;
  flex-direction: column;
  gap: var(--profile-gap);
  max-width: var(--kapoj-content-max);
}

:root:not(.app-dark) .profile-page {
  --profile-primary: #1595a8;
  --profile-success: #19a463;
  --kapoj-primary-soft: rgb(21 149 168 / 9%);
  --kapoj-border: #e3e9f0;
  --kapoj-border-strong: #d2dde9;
  --kapoj-text: #172338;
  --kapoj-text-secondary: #536780;
  --kapoj-text-muted: #5f7086;
  --profile-text-muted: #5f7086;
  --kapoj-border-focus: #0e7490;
  --kapoj-shadow-sm: 0 1px 2px rgb(23 35 56 / 3%), 0 8px 24px rgb(23 35 56 / 5%);
  --kapoj-shadow-md: 0 2px 4px rgb(23 35 56 / 4%), 0 18px 40px rgb(23 35 56 / 9%);
  --profile-primary-text: #0e7490;
  --profile-success-text: #047857;
}

.profile-hero {
  display: grid;
  grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
  gap: var(--profile-gap);
  align-items: stretch;
}

.profile-hero__card {
  position: relative;
  display: flex;
  min-width: 0;
  padding: clamp(20px, 2.1vw, 28px);
  overflow: hidden;
  background:
    radial-gradient(
      120% 150% at 100% 0%,
      color-mix(in srgb, var(--profile-primary) 9%, transparent),
      transparent 58%
    ),
    linear-gradient(
      135deg,
      color-mix(in srgb, var(--profile-primary) 4%, var(--profile-surface)),
      var(--profile-surface) 58%
    );
  border: 1px solid var(--profile-border);
  border-radius: var(--profile-radius);
  box-shadow: var(--profile-shadow);
  transition:
    transform var(--kapoj-transition-slow),
    border-color var(--kapoj-transition-slow),
    box-shadow var(--kapoj-transition-slow);
}

.profile-hero__card:hover {
  border-color: color-mix(in srgb, var(--profile-primary) 28%, var(--profile-border));
  box-shadow: var(--profile-shadow-hover);
  transform: translateY(-2px);
}

.profile-hero__decor {
  position: absolute;
  inset: 0;
  overflow: hidden;
  pointer-events: none;
}

.profile-hero__decor::before {
  position: absolute;
  inset: 0;
  content: '';
  background-image:
    linear-gradient(var(--profile-border) 1px, transparent 1px),
    linear-gradient(90deg, var(--profile-border) 1px, transparent 1px);
  background-size: 32px 32px;
  opacity: 0.38;
  -webkit-mask-image: linear-gradient(140deg, transparent 20%, black 70%, transparent 100%);
  mask-image: linear-gradient(140deg, transparent 20%, black 70%, transparent 100%);
}

.profile-hero__code {
  position: absolute;
  color: var(--profile-primary);
  font-family: var(--kapoj-font-mono);
  font-weight: 700;
  line-height: 1;
  user-select: none;
}

.profile-hero__code--angle {
  top: 18px;
  right: 22px;
  font-size: clamp(42px, 5.4vw, 72px);
  opacity: 0.07;
  transform: rotate(-5deg);
}

.profile-hero__code--braces {
  right: 12%;
  bottom: 18px;
  font-size: clamp(28px, 3.6vw, 48px);
  opacity: 0.06;
}

.profile-hero__code--const {
  top: 24%;
  right: 8%;
  font-size: 11px;
  letter-spacing: 0.14em;
  opacity: 0.22;
}

.profile-hero__circuit {
  position: absolute;
  border: 1px solid color-mix(in srgb, var(--profile-primary) 22%, transparent);
  border-radius: 999px;
  opacity: 0.5;
}

.profile-hero__circuit--a {
  top: -92px;
  right: -50px;
  width: 230px;
  height: 230px;
}

.profile-hero__circuit--b {
  right: 18%;
  bottom: -130px;
  width: 240px;
  height: 240px;
}

.profile-hero__content {
  position: relative;
  z-index: 1;
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
  gap: var(--kapoj-space-4);
}

.profile-hero__main {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-5);
  min-width: 0;
}

.profile-avatar-wrap {
  position: relative;
  flex: 0 0 auto;
}

.profile-avatar-wrap::after {
  position: absolute;
  inset: -7px;
  pointer-events: none;
  content: '';
  background: radial-gradient(circle, var(--profile-primary-soft), transparent 70%);
  border-radius: 50%;
}

.profile-avatar {
  position: relative;
  z-index: 1;
  width: 96px;
  height: 96px;
  color: var(--profile-primary);
  font-size: 2rem;
  font-weight: 700;
  background: var(--profile-primary-soft);
  border: 3px solid color-mix(in srgb, var(--profile-primary) 42%, var(--profile-surface));
  box-shadow:
    0 0 0 5px color-mix(in srgb, var(--profile-primary) 7%, transparent),
    0 12px 26px color-mix(in srgb, var(--profile-primary) 16%, transparent);
}

.profile-avatar :deep(img) {
  border-radius: 50%;
}

.profile-hero__identity {
  min-width: 0;
}

.profile-hero__eyebrow {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0 0 7px;
  color: var(--profile-primary-text);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.profile-hero__eyebrow-dot {
  width: 5px;
  height: 5px;
  background: var(--profile-primary);
  border-radius: 50%;
  box-shadow: 0 0 0 3px var(--profile-primary-soft);
}

.profile-hero__name-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  min-width: 0;
}

.profile-hero__title {
  min-width: 0;
  margin: 0;
  color: var(--profile-text);
  font-size: clamp(1.875rem, 1.5rem + 1.2vw, 2.125rem);
  font-weight: 800;
  line-height: 1.08;
  letter-spacing: -0.035em;
  overflow-wrap: anywhere;
}

.profile-title-badge {
  display: inline-flex;
  flex: 0 1 auto;
  align-items: center;
  gap: 6px;
  max-width: 100%;
  padding: 4px 10px;
  color: var(--profile-text);
  font-size: 12px;
  font-weight: 700;
  line-height: 1.4;
  background: color-mix(in srgb, var(--profile-title-color, #1595a8) 10%, var(--profile-surface));
  border: 1px solid
    color-mix(in srgb, var(--profile-title-color, #1595a8) 34%, var(--profile-border));
  border-radius: 999px;
  box-shadow: 0 3px 10px color-mix(in srgb, var(--profile-title-color, #1595a8) 12%, transparent);
}

.profile-title-badge i {
  color: var(--profile-title-color, #1595a8);
  font-size: 10px;
}

.profile-hero__handle {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  margin: 7px 0 0;
  color: var(--profile-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.profile-hero__sep {
  opacity: 0.6;
}

.profile-bio {
  display: -webkit-box;
  max-width: 68ch;
  margin: 0;
  overflow: hidden;
  color: var(--profile-text-secondary);
  font-size: 14px;
  line-height: 1.7;
  overflow-wrap: anywhere;
  white-space: pre-line;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  line-clamp: 2;
}

.profile-bio.is-empty {
  color: var(--profile-text-muted);
}

.profile-hero__badges {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.profile-hero__badges-label {
  color: var(--profile-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  letter-spacing: 0.08em;
}

.profile-hero__meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2) var(--kapoj-space-4);
  color: var(--profile-text-muted);
  font-size: 12px;
}

.profile-hero__meta-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.profile-hero__meta-item i {
  color: var(--profile-primary);
  font-size: 11px;
}

.profile-hero__footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
  padding-top: var(--kapoj-space-4);
  margin-top: auto;
  border-top: 1px solid var(--profile-border);
}

.social-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-3);
}

.social-row__divider {
  width: 1px;
  height: 22px;
  background: var(--profile-border);
}

.social-stat {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  padding: 2px 0;
  color: var(--profile-text);
  cursor: pointer;
  background: none;
  border: none;
  border-radius: var(--kapoj-radius-sm);
  transition: color var(--kapoj-transition);
}

.social-stat:hover .social-stat__num {
  color: var(--profile-primary-text);
}

.social-stat__num {
  font-family: var(--kapoj-font-mono);
  font-size: 20px;
  font-weight: 700;
  line-height: 1;
  letter-spacing: -0.02em;
}

.social-stat__label {
  color: var(--profile-text-muted);
  font-size: 12px;
}

.profile-hero__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
  margin-left: auto;
}

.profile-action {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  min-height: 34px;
  padding: 7px 14px;
  font-size: 13px;
  font-weight: 600;
  line-height: 1.2;
  cursor: pointer;
  border-radius: var(--kapoj-radius-sm);
  transition:
    color var(--kapoj-transition),
    background-color var(--kapoj-transition),
    border-color var(--kapoj-transition),
    box-shadow var(--kapoj-transition);
}

.profile-action--ghost {
  color: var(--profile-primary-text);
  text-decoration: none;
  background: var(--profile-primary-soft);
  border: 1px solid color-mix(in srgb, var(--profile-primary) 30%, var(--profile-border));
}

.profile-action--ghost:hover {
  color: var(--profile-primary-text);
  text-decoration: none;
  background: color-mix(in srgb, var(--profile-primary) 14%, var(--profile-surface));
  border-color: color-mix(in srgb, var(--profile-primary) 55%, var(--profile-border));
}

.follow-button {
  min-height: 34px;
}

.profile-hero__cloud {
  min-width: 0;
}

.profile-stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--profile-gap);
  margin: 0;
}

.stat-card {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-3);
  min-width: 0;
  min-height: 92px;
  padding: 18px 20px;
  background: var(--profile-surface);
  border: 1px solid var(--profile-border);
  border-radius: var(--profile-radius);
  box-shadow: var(--profile-shadow);
  animation: profile-stat-in 420ms ease-out backwards;
  animation-delay: var(--stat-delay, 0ms);
  transition:
    transform var(--kapoj-transition-slow),
    border-color var(--kapoj-transition-slow),
    box-shadow var(--kapoj-transition-slow);
}

.stat-card:hover {
  border-color: color-mix(in srgb, var(--stat-accent) 38%, var(--profile-border));
  box-shadow: var(--profile-shadow-hover);
  transform: translateY(-2px);
}

.stat-card--teal {
  --stat-accent: var(--profile-primary);
  --stat-tint: color-mix(in srgb, var(--profile-primary) 10%, var(--profile-surface));
}

.stat-card--green {
  --stat-accent: var(--profile-success);
  --stat-tint: color-mix(in srgb, var(--profile-success) 10%, var(--profile-surface));
}

.stat-card--blue {
  --stat-accent: #3b82f6;
  --stat-tint: rgb(59 130 246 / 9%);
}

.stat-card--amber {
  --stat-accent: #d97706;
  --stat-tint: rgb(217 119 6 / 9%);
}

.stat-card__icon {
  display: grid;
  flex: 0 0 auto;
  width: 42px;
  height: 42px;
  color: var(--stat-accent);
  font-size: 18px;
  background: var(--stat-tint);
  border: 1px solid color-mix(in srgb, var(--stat-accent) 18%, transparent);
  border-radius: 12px;
  place-items: center;
}

.stat-card__body {
  display: flex;
  flex-direction: column;
  min-width: 0;
  gap: 3px;
}

.stat-card__label {
  color: var(--profile-text-muted);
  font-size: 12px;
  font-weight: 500;
  line-height: 1.2;
}

.stat-card__value {
  margin: 0;
  color: var(--profile-text);
  font-size: clamp(1.5rem, 1.25rem + 0.55vw, 1.85rem);
  font-weight: 700;
  line-height: 1.05;
  letter-spacing: -0.04em;
}

@keyframes profile-stat-in {
  from {
    opacity: 0;
    transform: translateY(8px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.profile-body {
  display: grid;
  grid-template-columns: minmax(0, 7fr) minmax(300px, 3fr);
  gap: var(--profile-gap);
  align-items: start;
}

.profile-main,
.profile-aside {
  display: flex;
  flex-direction: column;
  gap: var(--profile-gap);
  min-width: 0;
}

.profile-card {
  min-width: 0;
}

@media (max-width: 1320px) {
  .profile-body {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 1024px) {
  .profile-hero {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  }
}

@media (max-width: 900px) {
  .profile-hero {
    grid-template-columns: minmax(0, 1fr);
  }

  .profile-stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .profile-page {
    --profile-gap: 16px;
  }

  .profile-hero__card {
    padding: var(--kapoj-space-4);
  }

  .profile-hero__main {
    align-items: flex-start;
    gap: var(--kapoj-space-4);
  }

  .profile-avatar {
    width: 88px;
    height: 88px;
    font-size: 1.8rem;
  }

  .profile-hero__footer {
    align-items: flex-start;
  }

  .profile-hero__actions {
    width: 100%;
    margin-left: 0;
  }

  .stat-card {
    min-height: 84px;
    padding: var(--kapoj-space-3) var(--kapoj-space-4);
  }

  .stat-card__icon {
    width: 38px;
    height: 38px;
    font-size: 16px;
  }
}

@media (max-width: 420px) {
  .profile-hero__main {
    flex-direction: column;
  }

  .profile-hero__name-row {
    gap: 8px;
  }

  .profile-hero__title {
    font-size: 1.875rem;
  }
}

@media (prefers-reduced-motion: reduce) {
  .profile-hero__card,
  .stat-card,
  .social-stat,
  .profile-action {
    transition: none;
  }

  .profile-hero__card:hover,
  .stat-card:hover {
    transform: none;
  }

  .stat-card {
    animation: none;
  }
}
</style>
