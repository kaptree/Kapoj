<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Tag from 'primevue/tag'

import type { SiteLink } from '@/api/types'
import CompanionAvatar from '@/components/companion/CompanionAvatar.vue'
import CompanionBubble from '@/components/companion/CompanionBubble.vue'
import MangaLabel from '@/components/companion/MangaLabel.vue'
import HomeLinkCard from '@/components/home/HomeLinkCard.vue'
import HomeProfileCard from '@/components/home/HomeProfileCard.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppStatusTag from '@/components/ui/AppStatusTag.vue'
import RatingLevelName from '@/components/RatingLevelName.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { pickCompanionLine } from '@/content/companion/lines.zh-CN'
import { useHomeData } from '@/composables/useHomeData'
import { usePlatformHealth } from '@/composables/usePlatformHealth'
import { GUEST_PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'
import { useCompanionStore } from '@/stores/companion'
import { useGuestStore } from '@/stores/guest'
import { useSiteStore } from '@/stores/site'
import { localDateKey } from '@/utils/date'
import { formatDateTime } from '@/utils/format'
import { formatDelta } from '@/utils/rating'

type CardTone = 'teal' | 'amber' | 'blue' | 'violet' | 'green' | 'rose'
type PillTone = 'success' | 'warning' | 'neutral'

interface StatusItem {
  icon: string
  label: string
  pill: string
  tone: PillTone
}

interface StatusGroup {
  key: string
  label: string
  items: StatusItem[]
}

const CHART_WIDTH = 240
const CHART_HEIGHT = 56
const CHART_PADDING = 6

const auth = useAuthStore()
const guest = useGuestStore()
const site = useSiteStore()
const companion = useCompanionStore()
const homeData = useHomeData()
const health = usePlatformHealth()

companion.load()

const now = ref(new Date())
const networkOnline = ref(typeof navigator === 'undefined' ? true : navigator.onLine)
const pageVisible = ref(typeof document === 'undefined' ? true : !document.hidden)
let clockTimer: number | undefined

const appearanceLink: SiteLink = {
  to: '/appearance',
  icon: 'pi pi-palette',
  iconPos: 'left',
  title: '装扮管理',
  description: '调整徽章、称号与展示偏好。',
}

const quickMeta: Record<string, string> = {
  '/problems': 'Practice',
  '/contests': 'Contest',
  '/leaderboard': 'Ranking',
  '/submissions': 'Records',
}

const growthMeta: Record<string, string> = {
  '/achievements': 'Honor',
  '/tasks': 'Quests',
  '/shop/badges': 'Collect',
  '/shop/titles': 'Identity',
  '/appearance': 'Style',
}

const linkTones: Record<string, CardTone> = {
  '/problems': 'teal',
  '/contests': 'amber',
  '/leaderboard': 'blue',
  '/submissions': 'violet',
  '/achievements': 'amber',
  '/tasks': 'green',
  '/shop/badges': 'rose',
  '/shop/titles': 'blue',
  '/appearance': 'violet',
}

const statusGroups: StatusGroup[] = [
  {
    key: 'judge',
    label: '评测与竞赛',
    items: [
      { icon: 'pi pi-bolt', label: '多语言实时评测', pill: '已开放', tone: 'success' },
      { icon: 'pi pi-flag', label: 'ICPC / IOI / OI 赛制', pill: '已开放', tone: 'success' },
    ],
  },
  {
    key: 'growth',
    label: '成长与社区',
    items: [
      { icon: 'pi pi-chart-line', label: 'Rating 结算与等级', pill: '已开放', tone: 'success' },
      {
        icon: 'pi pi-calendar',
        label: '排行榜、签到与 Rating 历史',
        pill: '已开放',
        tone: 'success',
      },
      { icon: 'pi pi-sitemap', label: '学习路线', pill: '已开放', tone: 'success' },
    ],
  },
  {
    key: 'extended',
    label: '扩展能力',
    items: [{ icon: 'pi pi-sparkles', label: 'AI 代码助手', pill: '按配置', tone: 'warning' }],
  },
]

const guestFeatures = ['保存提交与 Rating 变化', '领取任务与成就奖励', '收集并装扮徽章称号']

const visibleGrowthLinks = computed(() => {
  if (auth.isLoggedIn) return [...site.home.growthLinks, appearanceLink]
  return guest.can(GUEST_PERM.growth) ? site.home.growthLinks : []
})

const greeting = computed(() => {
  const hour = now.value.getHours()
  if (hour < 5 || hour >= 23) return '夜深了'
  if (hour < 9) return '早上好'
  if (hour < 12) return '上午好'
  if (hour < 14) return '中午好'
  if (hour < 18) return '下午好'
  return '晚上好'
})

const greetingName = computed(() => auth.user?.nickname.trim() || '同学')
const companionGreeting = computed(() => pickCompanionLine('greeting', localDateKey()))
const presence = computed<'online' | 'away' | 'offline'>(() => {
  if (!networkOnline.value) return 'offline'
  return pageVisible.value ? 'online' : 'away'
})

const healthMeta = computed(() => {
  switch (health.state.value) {
    case 'ok':
      return { label: '平台运行正常', tone: 'success' }
    case 'degraded':
      return { label: '部分服务波动', tone: 'warning' }
    case 'offline':
      return { label: '网络已断开', tone: 'danger' }
    case 'unreachable':
      return { label: '状态暂时不可用', tone: 'neutral' }
    default:
      return { label: '正在检测平台状态', tone: 'checking' }
  }
})

const taskBadge = computed(() => {
  const count = homeData.claimableTaskCount.value
  return count > 0 ? `${count} 个可领取` : ''
})

function toneFor(to: string): CardTone {
  return linkTones[to] ?? 'teal'
}

function metaFor(to: string, section: 'quick' | 'growth'): string {
  return (section === 'quick' ? quickMeta : growthMeta)[to] ?? ''
}

function badgeFor(to: string): string {
  return to === '/tasks' ? taskBadge.value : ''
}

const goalSteps = computed(() => [
  {
    key: 'signin',
    label: '每日签到',
    done: homeData.summary.value?.signedInToday === true,
    known: homeData.summaryState.value === 'ready',
  },
  {
    key: 'practice',
    label: '完成 1 次代码提交',
    done: (homeData.todaySubmissionCount.value ?? 0) > 0,
    known: homeData.activityState.value === 'ready',
  },
])

const goalReady = computed(() => goalSteps.value.every((step) => step.known))
const goalDone = computed(() => goalSteps.value.filter((step) => step.done).length)
const goalPercent = computed(() =>
  goalReady.value ? Math.round((goalDone.value / goalSteps.value.length) * 100) : 0,
)
const goalLoading = computed(
  () =>
    homeData.summaryState.value === 'idle' ||
    homeData.summaryState.value === 'loading' ||
    homeData.activityState.value === 'idle' ||
    homeData.activityState.value === 'loading',
)
const goalFailed = computed(
  () => homeData.summaryState.value === 'error' || homeData.activityState.value === 'error',
)

const recentLoading = computed(
  () => homeData.submissionsState.value === 'idle' || homeData.submissionsState.value === 'loading',
)
const recentFailed = computed(() => homeData.submissionsState.value === 'error')
const ratingLoading = computed(
  () =>
    homeData.ratingHistoryState.value === 'idle' || homeData.ratingHistoryState.value === 'loading',
)
const ratingFailed = computed(() => homeData.ratingHistoryState.value === 'error')

const ratingChart = computed(() => {
  const entries = [...homeData.ratingHistory.value].reverse()
  if (entries.length === 0) return null

  const ratings = entries.map((entry) => entry.ratingAfter)
  const min = Math.min(...ratings)
  const max = Math.max(...ratings)
  const span = max - min
  const innerWidth = CHART_WIDTH - CHART_PADDING * 2
  const innerHeight = CHART_HEIGHT - CHART_PADDING * 2

  const points = entries.map((entry, index) => ({
    x:
      entries.length === 1
        ? CHART_WIDTH / 2
        : CHART_PADDING + (index / (entries.length - 1)) * innerWidth,
    y:
      span === 0
        ? CHART_HEIGHT / 2
        : CHART_PADDING + (1 - (entry.ratingAfter - min) / span) * innerHeight,
  }))

  const first = entries[0].ratingAfter
  const last = entries[entries.length - 1].ratingAfter
  return {
    points,
    polyline: points.map((point) => `${point.x.toFixed(2)},${point.y.toFixed(2)}`).join(' '),
    lastPoint: points[points.length - 1],
    label: `Rating 趋势：最近 ${entries.length} 次变化，从 ${first} 到 ${last}。`,
  }
})

const profileLink = computed(() => {
  const id = auth.user?.id
  return id ? { name: 'user-profile', params: { id } } : { name: 'leaderboard' }
})

function deltaTone(delta: number): string {
  if (delta > 0) return 'is-positive'
  if (delta < 0) return 'is-negative'
  return 'is-neutral'
}

function submissionRoute(problemId: number) {
  return { name: 'problem-detail', params: { id: problemId } }
}

function updateClock() {
  now.value = new Date()
}

function handleOnline() {
  networkOnline.value = true
}

function handleOffline() {
  networkOnline.value = false
}

function handleVisibility() {
  pageVisible.value = !document.hidden
}

onMounted(() => {
  updateClock()
  clockTimer = window.setInterval(updateClock, 60_000)
  window.addEventListener('online', handleOnline)
  window.addEventListener('offline', handleOffline)
  document.addEventListener('visibilitychange', handleVisibility)
})

onBeforeUnmount(() => {
  if (clockTimer !== undefined) window.clearInterval(clockTimer)
  window.removeEventListener('online', handleOnline)
  window.removeEventListener('offline', handleOffline)
  document.removeEventListener('visibilitychange', handleVisibility)
})
</script>

<template>
  <div class="home">
    <AppSurface
      as="section"
      tone="default"
      padding="none"
      class="hero companion-sticker"
      aria-labelledby="home-title"
    >
      <div class="hero__copy">
        <div class="hero__meta">
          <span class="hero__greeting">
            <span class="hero__greeting-dot" aria-hidden="true" />
            {{ greeting }}，<RatingLevelName
              v-if="auth.user"
              :name="greetingName"
              :level="auth.user.level"
              inherit-typography
            /><span v-else>同学</span>
          </span>
          <span v-if="site.home.eyebrow" class="hero__eyebrow">{{ site.home.eyebrow }}</span>
        </div>

        <h1 id="home-title" class="hero__title">
          {{ site.home.title
          }}<span v-if="site.home.titleHighlight" class="hero__title-accent">
            {{ ' ' + site.home.titleHighlight }}
          </span>
        </h1>

        <p v-if="site.home.tagline" class="hero__tagline">{{ site.home.tagline }}</p>
        <p v-if="site.home.description" class="hero__description">
          {{ site.home.description }}
        </p>

        <div class="hero__actions">
          <Button
            v-if="site.home.primaryCta.title"
            as="router-link"
            :to="site.home.primaryCta.to"
            :label="site.home.primaryCta.title"
            :icon="site.home.primaryCta.icon || undefined"
            :icon-pos="site.home.primaryCta.iconPos"
            size="large"
          />
          <Button
            v-if="site.home.secondaryCta.title"
            as="router-link"
            :to="site.home.secondaryCta.to"
            :label="site.home.secondaryCta.title"
            :icon="site.home.secondaryCta.icon || undefined"
            :icon-pos="site.home.secondaryCta.iconPos"
            size="large"
            severity="secondary"
            outlined
          />
        </div>
      </div>

      <div v-if="companion.effectiveEnabled" class="hero__companion" aria-hidden="false">
        <CompanionAvatar pose="bust" expression="smug" size="lg" />
        <CompanionBubble :text="companionGreeting" />
      </div>
      <div v-else class="hero__signal" aria-hidden="true">
        <span class="hero__signal-ring" />
        <span class="hero__signal-ring" />
        <span class="hero__signal-mark">K</span>
      </div>

      <aside class="hero__status" aria-label="平台状态">
        <div class="status__head">
          <span class="status__title">
            <span class="status__live-dot" :class="`is-${healthMeta.tone}`" aria-hidden="true" />
            平台状态
          </span>
          <span class="status__version">M4-C</span>
        </div>

        <div v-for="group in statusGroups" :key="group.key" class="status__group">
          <p class="status__group-label">{{ group.label }}</p>
          <ul class="status__list">
            <li v-for="item in group.items" :key="item.label" class="status__item">
              <span class="status__icon" aria-hidden="true">
                <i :class="item.icon" />
              </span>
              <span class="status__item-label">{{ item.label }}</span>
              <span class="status__pill" :class="`is-${item.tone}`">{{ item.pill }}</span>
            </li>
          </ul>
        </div>

        <div class="status__foot" aria-live="polite">
          <span class="status__health-dot" :class="`is-${healthMeta.tone}`" aria-hidden="true" />
          <span>{{ healthMeta.label }}</span>
          <span class="status__days">已运行 {{ site.runningDays }} 天</span>
        </div>
      </aside>
    </AppSurface>

    <HomeProfileCard
      v-if="auth.isLoggedIn && auth.user"
      :user="auth.user"
      :activity="homeData.activity.value"
      :activity-state="homeData.activityState.value"
      :presence="presence"
    />

    <AppSurface
      v-else-if="auth.isLoggedIn"
      tone="default"
      padding="lg"
      class="profile-loading"
      aria-label="档案加载中"
    >
      <AppLoadingState label="正在加载你的档案" />
    </AppSurface>

    <AppSurface
      v-else
      as="section"
      tone="default"
      padding="none"
      class="guest-strip"
      aria-labelledby="home-guest-title"
    >
      <div class="guest-strip__copy">
        <p class="section-eyebrow">GET STARTED</p>
        <h2 id="home-guest-title" class="guest-strip__title">{{ site.home.guestTitle }}</h2>
        <p v-if="site.home.guestDescription" class="guest-strip__description">
          {{ site.home.guestDescription }}
        </p>
        <ul class="guest-strip__features">
          <li v-for="feature in guestFeatures" :key="feature">
            <i class="pi pi-check" aria-hidden="true" />
            {{ feature }}
          </li>
        </ul>
      </div>
      <div class="guest-strip__actions">
        <Button as="router-link" to="/login" label="登录" icon="pi pi-sign-in" />
        <Button
          as="router-link"
          to="/register"
          label="注册账号"
          icon="pi pi-user-plus"
          severity="secondary"
          outlined
        />
        <p class="guest-strip__hint">注册后自动保存练习数据与成长进度。</p>
      </div>
    </AppSurface>

    <section
      v-if="auth.isLoggedIn && auth.user"
      class="home-section learning-overview"
      aria-labelledby="home-overview-title"
    >
      <div class="section-head">
        <MangaLabel index="01" label="LEARNING OVERVIEW" kana="学習記録" />
        <h2 id="home-overview-title" class="section-title">学习概览</h2>
        <p class="section-subtitle">根据你的真实练习数据生成，帮助你决定下一步。</p>
      </div>

      <div class="overview-grid">
        <AppSurface
          padding="none"
          class="overview-card goal-card"
          :aria-busy="goalLoading || undefined"
        >
          <header class="overview-card__head">
            <span class="overview-card__icon is-teal">
              <i class="pi pi-flag" aria-hidden="true" />
            </span>
            <div class="overview-card__heading">
              <p class="overview-card__eyebrow">DAILY GOAL</p>
              <h3>每日目标</h3>
            </div>
            <span class="goal-card__count">
              {{ goalReady ? `${goalDone} / ${goalSteps.length}` : goalLoading ? '同步中' : '—' }}
            </span>
          </header>

          <div
            class="goal-progress"
            :class="{ 'is-pending': !goalReady }"
            role="progressbar"
            :aria-valuenow="goalPercent"
            aria-valuemin="0"
            aria-valuemax="100"
            :aria-label="`每日目标完成度 ${goalPercent}%`"
            :aria-valuetext="
              goalReady
                ? `${goalDone} / ${goalSteps.length} 已完成`
                : goalFailed
                  ? '暂时无法同步'
                  : '正在同步'
            "
          >
            <span :style="{ width: `${goalPercent}%` }" />
          </div>

          <ul class="goal-list">
            <li
              v-for="step in goalSteps"
              :key="step.key"
              :class="{ 'is-done': step.done, 'is-unknown': !step.known }"
            >
              <span class="goal-list__check" aria-hidden="true">
                <i v-if="step.done" class="pi pi-check" />
                <i v-else-if="!step.known" class="pi pi-minus" />
              </span>
              <span class="goal-list__label">{{ step.label }}</span>
              <span class="goal-list__status">
                {{ step.done ? '已完成' : step.known ? '待完成' : '—' }}
              </span>
            </li>
          </ul>

          <footer class="overview-card__foot goal-card__foot">
            <span v-if="goalFailed" class="overview-state">今日目标暂时无法同步</span>
            <span v-else-if="homeData.summaryState.value === 'ready'">
              今日活跃度
              <strong class="num">+{{ homeData.summary.value?.todayPoints ?? 0 }}</strong>
            </span>
            <span v-else class="overview-state">正在同步今日活跃度</span>
            <RouterLink to="/problems" class="overview-link">去练习</RouterLink>
          </footer>
        </AppSurface>

        <AppSurface
          padding="none"
          class="overview-card recent-card"
          :aria-busy="recentLoading || undefined"
        >
          <header class="overview-card__head">
            <span class="overview-card__icon is-blue">
              <i class="pi pi-history" aria-hidden="true" />
            </span>
            <div class="overview-card__heading">
              <p class="overview-card__eyebrow">RECENT PRACTICE</p>
              <h3>最近练习</h3>
            </div>
            <RouterLink to="/submissions" class="overview-link">全部记录</RouterLink>
          </header>

          <div v-if="recentLoading" class="recent-skeleton" aria-label="正在加载最近练习">
            <span v-for="index in 3" :key="index" />
          </div>

          <p v-else-if="recentFailed" class="overview-state">最近练习暂时无法加载</p>

          <div v-else-if="homeData.submissions.value.length === 0" class="recent-empty">
            <p>还没有提交记录。</p>
            <RouterLink to="/problems" class="overview-link">去题库开始</RouterLink>
          </div>

          <ul v-else class="recent-list">
            <li v-for="submission in homeData.submissions.value" :key="submission.id">
              <RouterLink :to="submissionRoute(submission.problemId)" class="recent-list__row">
                <span class="recent-list__main">
                  <span class="recent-list__title-row">
                    <span class="recent-list__title">{{ submission.problemTitle }}</span>
                    <Tag
                      v-if="submission.agentId != null"
                      value="Agent"
                      severity="info"
                      class="recent-list__agent"
                    />
                    <RatingLevelName
                      v-if="submission.agentName"
                      class="recent-list__agent-name"
                      :name="submission.agentName"
                      :level="submission.agentLevel"
                      inherit-typography
                    />
                  </span>
                  <span class="recent-list__time">{{ formatDateTime(submission.createdAt) }}</span>
                </span>
                <AppStatusTag :status="submission.status" />
              </RouterLink>
            </li>
          </ul>
        </AppSurface>

        <AppSurface
          padding="none"
          class="overview-card rating-card"
          :aria-busy="ratingLoading || undefined"
        >
          <header class="overview-card__head">
            <span class="overview-card__icon is-amber">
              <i class="pi pi-chart-line" aria-hidden="true" />
            </span>
            <div class="overview-card__heading">
              <p class="overview-card__eyebrow">RATING TREND</p>
              <h3>Rating 趋势</h3>
            </div>
            <RouterLink :to="profileLink" class="overview-link">完整历史</RouterLink>
          </header>

          <div v-if="ratingLoading" class="rating-skeleton" aria-label="正在加载 Rating 趋势">
            <span class="rating-skeleton__value" />
            <span class="rating-skeleton__chart" />
          </div>

          <p v-else-if="ratingFailed" class="overview-state">Rating 趋势暂时无法加载</p>

          <template v-else>
            <div class="rating-card__value">
              <strong class="num">{{ auth.user.rating }}</strong>
              <span
                v-if="homeData.ratingDelta.value !== null"
                class="rating-card__delta"
                :class="deltaTone(homeData.ratingDelta.value)"
              >
                {{ formatDelta(homeData.ratingDelta.value) }}
              </span>
              <span class="rating-card__unit">当前 Rating</span>
            </div>

            <figure v-if="ratingChart" class="rating-card__chart">
              <svg
                :viewBox="`0 0 ${CHART_WIDTH} ${CHART_HEIGHT}`"
                role="img"
                :aria-label="ratingChart.label"
              >
                <title>{{ ratingChart.label }}</title>
                <polyline
                  v-if="ratingChart.points.length > 1"
                  :points="ratingChart.polyline"
                  fill="none"
                />
                <circle :cx="ratingChart.lastPoint.x" :cy="ratingChart.lastPoint.y" r="3" />
              </svg>
            </figure>

            <div v-if="homeData.latestRatingEntry.value" class="rating-card__latest">
              <span class="rating-card__latest-title">
                {{ homeData.latestRatingEntry.value.contestTitle }}
              </span>
              <span class="rating-card__latest-time">
                {{ formatDateTime(homeData.latestRatingEntry.value.createdAt) }}
              </span>
            </div>
            <div v-else class="rating-card__empty">
              <p>还没有 Rating 变化记录。</p>
              <RouterLink to="/contests" class="overview-link">去参加比赛</RouterLink>
            </div>
          </template>
        </AppSurface>
      </div>
    </section>

    <section
      v-if="site.home.quickLinks.length"
      class="home-section"
      aria-labelledby="home-quick-title"
    >
      <div class="section-head">
        <MangaLabel index="02" label="QUICK START" kana="練習開始" />
        <h2 id="home-quick-title" class="section-title">{{ site.home.quickTitle }}</h2>
      </div>
      <div class="quick-grid">
        <HomeLinkCard
          v-for="link in site.home.quickLinks"
          :key="link.to"
          :link="link"
          :tone="toneFor(link.to)"
          :meta="metaFor(link.to, 'quick')"
          :featured="link.to === '/problems'"
        />
      </div>
    </section>

    <section
      v-if="visibleGrowthLinks.length"
      class="home-section"
      aria-labelledby="home-growth-title"
    >
      <div class="section-head">
        <MangaLabel index="03" label="GROWTH" kana="成長記録" />
        <h2 id="home-growth-title" class="section-title">{{ site.home.growthTitle }}</h2>
      </div>
      <div class="growth-grid">
        <HomeLinkCard
          v-for="link in visibleGrowthLinks"
          :key="link.to"
          :link="link"
          :tone="toneFor(link.to)"
          :meta="metaFor(link.to, 'growth')"
          :badge="badgeFor(link.to)"
        />
      </div>
    </section>
  </div>
</template>

<style scoped>
.home {
  display: grid;
  gap: var(--kapoj-space-6);
  width: 100%;
  max-width: var(--kapoj-content-max);
  margin: 0 auto;
}

.hero {
  position: relative;
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(220px, 0.55fr) minmax(300px, 0.8fr);
  gap: var(--kapoj-space-5);
  overflow: hidden;
}

.hero::before {
  position: absolute;
  top: -40%;
  right: -8%;
  width: 520px;
  height: 520px;
  pointer-events: none;
  content: '';
  background: radial-gradient(
    circle,
    color-mix(in srgb, var(--kapoj-brand) 10%, transparent),
    transparent 68%
  );
}

.hero__copy,
.hero__companion,
.hero__signal,
.hero__status {
  position: relative;
  z-index: 1;
  min-width: 0;
}

.hero__companion {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  align-items: center;
  align-self: center;
  justify-content: center;
  min-height: 300px;
  padding: var(--kapoj-space-4) 0;
}

.hero__companion :deep(.companion-avatar) {
  width: min(190px, 30vw);
}

.hero__companion :deep(.companion-bubble) {
  align-self: flex-end;
  max-width: 100%;
}

.hero__signal {
  display: grid;
  place-items: center;
  align-self: center;
  min-height: 300px;
  padding: var(--kapoj-space-4) 0;
}

.hero__signal-ring {
  position: absolute;
  top: 50%;
  left: 50%;
  border: 1px solid var(--kapoj-manga-line);
  border-radius: 50%;
  transform: translate(-50%, -50%);
}

.hero__signal-ring:first-child {
  width: 150px;
  height: 150px;
  background: var(--kapoj-kapo-soft);
}

.hero__signal-ring:nth-child(2) {
  width: 210px;
  height: 210px;
}

.hero__signal-mark {
  position: relative;
  display: grid;
  place-items: center;
  width: 56px;
  height: 56px;
  color: var(--kapoj-kapo-text);
  font-family: var(--kapoj-font-mono);
  font-size: 1.5rem;
  font-weight: 700;
  border: 1px solid var(--kapoj-manga-line);
  border-radius: 50%;
}

.hero__copy {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-6);
}

.hero__meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2) var(--kapoj-space-3);
  align-items: center;
}

.hero__greeting {
  display: inline-flex;
  gap: 7px;
  align-items: center;
  padding: 3px var(--kapoj-space-2);
  color: var(--kapoj-primary-strong);
  font-size: 11px;
  font-weight: 600;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-brand) 22%, transparent);
  border-radius: 999px;
}

.hero__greeting-dot {
  width: 6px;
  height: 6px;
  background: currentcolor;
  border-radius: 50%;
}

.hero__eyebrow,
.section-eyebrow {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.hero__title {
  max-width: 17ch;
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text);
  font-size: clamp(2.15rem, 1.55rem + 2.2vw, 3.4rem);
  font-weight: 750;
  line-height: 1.05;
  letter-spacing: -0.045em;
  text-wrap: balance;
}

.hero__title-accent {
  color: var(--kapoj-brand);
}

.hero__tagline {
  max-width: 38ch;
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text);
  font-size: clamp(1rem, 0.9rem + 0.35vw, 1.2rem);
  font-weight: 600;
  line-height: 1.5;
  letter-spacing: -0.01em;
  text-wrap: pretty;
}

.hero__description {
  max-width: 66ch;
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 13.5px;
  line-height: 1.65;
}

.hero__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
  margin-top: var(--kapoj-space-2);
}

.hero__status {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  align-self: center;
  padding: var(--kapoj-space-4);
  margin-right: var(--kapoj-space-5);
  background: var(--kapoj-surface-raised);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.status__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
  padding-bottom: var(--kapoj-space-3);
  border-bottom: 1px solid var(--kapoj-border);
}

.status__title {
  display: inline-flex;
  gap: var(--kapoj-space-2);
  align-items: center;
  color: var(--kapoj-text);
  font-size: 12px;
  font-weight: 650;
}

.status__live-dot,
.status__health-dot {
  width: 7px;
  height: 7px;
  background: var(--kapoj-text-muted);
  border-radius: 50%;
}

.status__live-dot.is-success,
.status__health-dot.is-success {
  background: var(--kapoj-success);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--kapoj-success) 12%, transparent);
}

.status__live-dot.is-warning,
.status__health-dot.is-warning {
  background: var(--kapoj-warning);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--kapoj-warning) 12%, transparent);
}

.status__live-dot.is-danger,
.status__health-dot.is-danger {
  background: var(--kapoj-danger);
}

.status__version {
  padding: 1px 6px;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  letter-spacing: 0.08em;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-xs);
}

.status__group {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
}

.status__group + .status__group {
  padding-top: var(--kapoj-space-3);
  border-top: 1px solid var(--kapoj-border);
}

.status__group-label {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.status__list {
  display: grid;
  gap: 2px;
  padding: 0;
  margin: 0;
  list-style: none;
}

.status__item {
  display: grid;
  grid-template-columns: 24px minmax(0, 1fr) auto;
  gap: var(--kapoj-space-2);
  align-items: center;
  min-height: 30px;
}

.status__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  color: var(--kapoj-brand);
  font-size: 11px;
  background: var(--kapoj-primary-soft);
  border-radius: var(--kapoj-radius-xs);
}

.status__item-label {
  overflow: hidden;
  color: var(--kapoj-text-secondary);
  font-size: 11.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.status__pill {
  padding: 1px 6px;
  font-size: 9px;
  font-weight: 600;
  line-height: 1.5;
  white-space: nowrap;
  border-radius: 999px;
}

.status__pill.is-success {
  color: var(--kapoj-success);
  background: color-mix(in srgb, var(--kapoj-success) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--kapoj-success) 24%, transparent);
}

.status__pill.is-warning {
  color: var(--kapoj-warning);
  background: color-mix(in srgb, var(--kapoj-warning) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--kapoj-warning) 24%, transparent);
}

.status__pill.is-neutral {
  color: var(--kapoj-text-muted);
  background: var(--kapoj-surface-hover);
  border: 1px solid var(--kapoj-border);
}

.status__foot {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
  align-items: center;
  padding-top: var(--kapoj-space-3);
  color: var(--kapoj-text-muted);
  font-size: 10.5px;
  border-top: 1px solid var(--kapoj-border);
}

.status__days {
  margin-left: auto;
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  letter-spacing: 0.04em;
}

.profile-loading {
  min-height: 118px;
}

.guest-strip {
  display: grid;
  grid-template-columns: minmax(0, 1.3fr) minmax(280px, 0.7fr);
  gap: var(--kapoj-space-5);
  align-items: center;
  padding: var(--kapoj-space-5);
}

.guest-strip__copy {
  min-width: 0;
}

.guest-strip__title {
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text);
  font-size: 1.35rem;
  line-height: 1.3;
  letter-spacing: -0.02em;
}

.guest-strip__description {
  max-width: 62ch;
  margin: var(--kapoj-space-2) 0 0;
  color: var(--kapoj-text-secondary);
  font-size: 13px;
  line-height: 1.6;
}

.guest-strip__features {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2) var(--kapoj-space-4);
  padding: 0;
  margin: var(--kapoj-space-3) 0 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
  list-style: none;
}

.guest-strip__features li {
  display: flex;
  gap: 5px;
  align-items: center;
}

.guest-strip__features i {
  color: var(--kapoj-success);
  font-size: 10px;
}

.guest-strip__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
  justify-content: flex-end;
}

.guest-strip__hint {
  flex-basis: 100%;
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 11px;
  text-align: right;
}

.home-section {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.section-head {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.section-title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.25rem;
  font-weight: 650;
  line-height: 1.25;
  letter-spacing: -0.02em;
}

.section-subtitle {
  margin: 2px 0 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.quick-grid,
.growth-grid {
  display: grid;
  gap: var(--kapoj-space-4);
}

.quick-grid {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.growth-grid {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

@media (min-width: 1302px) {
  .growth-grid {
    grid-template-columns: repeat(5, minmax(0, 1fr));
  }
}

.overview-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.15fr) minmax(0, 1fr);
  gap: var(--kapoj-space-4);
}

.overview-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-4);
  min-width: 0;
}

.overview-card__head {
  display: flex;
  gap: var(--kapoj-space-2);
  align-items: center;
  min-width: 0;
}

.overview-card__icon {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  font-size: 13px;
  background: var(--kapoj-primary-soft);
  border-radius: var(--kapoj-radius-sm);
}

.overview-card__icon.is-teal {
  color: var(--kapoj-brand);
}

.overview-card__icon.is-blue {
  color: var(--kapoj-info);
  background: color-mix(in srgb, var(--kapoj-info) 10%, transparent);
}

.overview-card__icon.is-amber {
  color: var(--kapoj-warning);
  background: color-mix(in srgb, var(--kapoj-warning) 10%, transparent);
}

.overview-card__heading {
  min-width: 0;
}

.overview-card__eyebrow {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 8px;
  font-weight: 600;
  letter-spacing: 0.1em;
}

.overview-card__heading h3 {
  margin: 1px 0 0;
  color: var(--kapoj-text);
  font-size: 0.95rem;
  font-weight: 650;
  line-height: 1.3;
}

.overview-card__foot {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
  align-items: center;
  justify-content: space-between;
  padding-top: var(--kapoj-space-3);
  margin-top: auto;
  color: var(--kapoj-text-muted);
  font-size: 11px;
  border-top: 1px solid var(--kapoj-border);
}

.overview-link {
  color: var(--kapoj-primary-strong);
  font-size: 11px;
  font-weight: 600;
  text-decoration: none;
  white-space: nowrap;
}

.overview-link:hover {
  color: var(--kapoj-primary-strong);
  text-decoration: underline;
}

.overview-link:focus-visible {
  outline: 2px solid var(--kapoj-border-focus);
  outline-offset: 2px;
  border-radius: 2px;
}

.goal-card__count {
  margin-left: auto;
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
}

.goal-progress {
  height: 6px;
  overflow: hidden;
  background: var(--kapoj-surface-hover);
  border-radius: 999px;
}

.goal-progress > span {
  display: block;
  height: 100%;
  background: var(--kapoj-brand);
  border-radius: inherit;
  transition: width var(--kapoj-transition-slow);
}

.goal-progress.is-pending > span {
  background: var(--kapoj-text-muted);
}

.goal-list {
  display: grid;
  gap: var(--kapoj-space-2);
  padding: 0;
  margin: 0;
  list-style: none;
}

.goal-list li {
  display: flex;
  gap: var(--kapoj-space-2);
  align-items: center;
  min-height: 24px;
  color: var(--kapoj-text-secondary);
  font-size: 12px;
}

.goal-list li.is-unknown {
  color: var(--kapoj-text-muted);
}

.goal-list__check {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  color: var(--kapoj-text-muted);
  font-size: 8px;
  background: transparent;
  border: 1px solid var(--kapoj-border-strong);
  border-radius: 50%;
}

.goal-list li.is-done .goal-list__check {
  color: var(--kapoj-text-inverted);
  background: var(--kapoj-success);
  border-color: var(--kapoj-success);
}

.goal-list__label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.goal-list__status {
  margin-left: auto;
  color: var(--kapoj-text-muted);
  font-size: 10px;
  white-space: nowrap;
}

.goal-list li.is-done .goal-list__status {
  color: var(--kapoj-success);
}

.overview-state {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.recent-list {
  display: grid;
  gap: 2px;
  padding: 0;
  margin: 0;
  list-style: none;
}

.recent-list__row {
  display: flex;
  gap: var(--kapoj-space-2);
  align-items: center;
  padding: 7px var(--kapoj-space-2);
  color: inherit;
  text-decoration: none;
  border-radius: var(--kapoj-radius-xs);
  transition: background var(--kapoj-transition);
}

.recent-list__row:hover {
  text-decoration: none;
  background: var(--kapoj-surface-hover);
}

.recent-list__row:focus-visible {
  outline: 2px solid var(--kapoj-border-focus);
  outline-offset: -2px;
}

.recent-list__main {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
}

.recent-list__title-row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.recent-list__title {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  color: var(--kapoj-text);
  font-size: 12px;
  font-weight: 500;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.recent-list__agent {
  flex: 0 0 auto;
}

.recent-list__agent-name {
  flex: 0 1 auto;
  max-width: 80px;
  overflow: hidden;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.recent-list__time {
  overflow: hidden;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.recent-list :deep(.app-status-tag) {
  flex: none;
  gap: 4px;
  padding: 0 5px;
  font-size: 9px;
}

.recent-list :deep(.app-status-tag__dot) {
  width: 4px;
  height: 4px;
}

.recent-empty,
.rating-card__empty {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  align-items: flex-start;
  justify-content: center;
  min-height: 88px;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.recent-empty p,
.rating-card__empty p {
  margin: 0;
}

.recent-skeleton,
.rating-skeleton {
  display: grid;
  gap: var(--kapoj-space-2);
}

.recent-skeleton span {
  height: 34px;
  overflow: hidden;
  background: var(--kapoj-surface-hover);
  border-radius: var(--kapoj-radius-xs);
}

.recent-skeleton span::after,
.rating-skeleton span::after {
  display: block;
  width: 100%;
  height: 100%;
  content: '';
  background: linear-gradient(
    90deg,
    transparent,
    color-mix(in srgb, var(--kapoj-text-muted) 10%, transparent),
    transparent
  );
  transform: translateX(-100%);
  animation: overview-skeleton 1.3s ease-in-out infinite;
}

.rating-skeleton__value {
  width: 5rem;
  height: 2rem;
  overflow: hidden;
  background: var(--kapoj-surface-hover);
  border-radius: var(--kapoj-radius-xs);
}

.rating-skeleton__chart {
  height: 56px;
  overflow: hidden;
  background: var(--kapoj-surface-hover);
  border-radius: var(--kapoj-radius-xs);
}

.rating-card__value {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
  align-items: baseline;
}

.rating-card__value strong {
  color: var(--kapoj-text);
  font-size: 2rem;
  font-weight: 700;
  line-height: 1;
  letter-spacing: -0.04em;
}

.rating-card__delta {
  padding: 1px 6px;
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  border-radius: 999px;
}

.rating-card__delta.is-positive {
  color: var(--kapoj-success);
  background: color-mix(in srgb, var(--kapoj-success) 10%, transparent);
}

.rating-card__delta.is-negative {
  color: var(--kapoj-danger);
  background: color-mix(in srgb, var(--kapoj-danger) 10%, transparent);
}

.rating-card__delta.is-neutral {
  color: var(--kapoj-text-muted);
  background: var(--kapoj-surface-hover);
}

.rating-card__unit {
  color: var(--kapoj-text-muted);
  font-size: 10px;
}

.rating-card__chart {
  margin: 0;
  padding: 2px 0;
}

.rating-card__chart svg {
  display: block;
  width: 100%;
  height: auto;
  overflow: visible;
}

.rating-card__chart polyline {
  stroke: var(--kapoj-brand);
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.rating-card__chart circle {
  fill: var(--kapoj-brand);
}

.rating-card__latest {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
  padding-top: var(--kapoj-space-2);
  margin-top: auto;
  border-top: 1px solid var(--kapoj-border);
}

.rating-card__latest-title {
  overflow: hidden;
  color: var(--kapoj-text-secondary);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rating-card__latest-time {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
}

@keyframes overview-skeleton {
  to {
    transform: translateX(100%);
  }
}

.home > * {
  animation: home-enter var(--kapoj-transition-slow) both;
}

.home > *:nth-child(2) {
  animation-delay: 40ms;
}

.home > *:nth-child(3) {
  animation-delay: 80ms;
}

.home > *:nth-child(4) {
  animation-delay: 120ms;
}

.home > *:nth-child(5) {
  animation-delay: 160ms;
}

@keyframes home-enter {
  from {
    opacity: 0;
    transform: translateY(6px);
  }

  to {
    opacity: 1;
    transform: none;
  }
}

@media (max-width: 1100px) {
  .hero {
    grid-template-columns: minmax(0, 1.35fr) minmax(300px, 0.9fr);
  }

  .hero__companion,
  .hero__signal {
    display: none;
  }
}

@media (max-width: 1180px) {
  .quick-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 1180px) and (min-width: 981px) {
  .overview-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .goal-card {
    order: 1;
  }

  .rating-card {
    order: 2;
  }

  .recent-card {
    grid-column: span 2;
    order: 3;
  }
}

@media (max-width: 980px) {
  .hero {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--kapoj-space-4);
  }

  .hero__copy {
    padding-bottom: 0;
  }

  .hero__status {
    align-self: stretch;
    margin: 0 var(--kapoj-space-6) var(--kapoj-space-6);
  }

  .guest-strip {
    grid-template-columns: minmax(0, 1fr);
  }

  .guest-strip__actions {
    justify-content: flex-start;
  }

  .guest-strip__hint {
    text-align: left;
  }

  .growth-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .overview-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 980px) and (min-width: 760px) {
  .hero__status {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--kapoj-space-3) var(--kapoj-space-4);
  }

  .status__head,
  .status__foot {
    grid-column: 1 / -1;
  }

  .status__group {
    min-width: 0;
  }

  .status__group + .status__group {
    padding-top: 0;
    padding-left: var(--kapoj-space-4);
    border-top: 0;
    border-left: 1px solid var(--kapoj-border);
  }

  .status__item-label {
    white-space: normal;
  }
}

@media (max-width: 640px) {
  .home {
    gap: var(--kapoj-space-5);
  }

  .hero__copy {
    padding: var(--kapoj-space-4);
  }

  .hero__title {
    max-width: none;
    font-size: clamp(2rem, 1.7rem + 2vw, 2.6rem);
  }

  .hero__status {
    padding: var(--kapoj-space-3);
    margin: 0 var(--kapoj-space-4) var(--kapoj-space-4);
  }

  .hero__actions {
    width: 100%;
  }

  .hero__actions :deep(.p-button) {
    flex: 1 1 10rem;
  }

  .guest-strip {
    gap: var(--kapoj-space-4);
    padding: var(--kapoj-space-4);
  }

  .guest-strip__actions {
    width: 100%;
  }

  .guest-strip__actions :deep(.p-button) {
    flex: 1 1 8rem;
  }

  .quick-grid,
  .growth-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .overview-card {
    padding: var(--kapoj-space-4);
  }

  .status__item-label {
    white-space: normal;
  }

  .status__days {
    margin-left: 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .home > * {
    animation: none;
  }

  .goal-progress > span,
  .recent-list__row {
    transition: none;
  }

  .recent-skeleton span::after,
  .rating-skeleton span::after {
    animation: none;
  }
}
</style>
