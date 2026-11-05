<script setup lang="ts">
import { computed } from 'vue'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'

import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useAuthStore } from '@/stores/auth'
import { formatCoins } from '@/utils/gamification'

interface HomeLink {
  to: string
  icon: string
  title: string
  description: string
}

const auth = useAuthStore()

const quickLinks: HomeLink[] = [
  {
    to: '/problems',
    icon: 'pi pi-book',
    title: '题库',
    description: '按标签与难度筛选，开始一次练习。',
  },
  {
    to: '/contests',
    icon: 'pi pi-trophy',
    title: '比赛',
    description: '参加 ICPC / IOI / OI 赛制，挑战实时榜单。',
  },
  {
    to: '/leaderboard',
    icon: 'pi pi-chart-line',
    title: '排行榜',
    description: '查看 Rating、活跃度与共享榜，每日签到领金币。',
  },
  {
    to: '/submissions',
    icon: 'pi pi-list-check',
    title: '测评记录',
    description: '查看提交状态、耗时与内存。',
  },
]

const growthLinks: HomeLink[] = [
  {
    to: '/achievements',
    icon: 'pi pi-star',
    title: '成就墙',
    description: '完成条件，解锁成就与奖励。',
  },
  {
    to: '/tasks',
    icon: 'pi pi-check-square',
    title: '任务大厅',
    description: '领取任务，AC 全部题目赢取金币。',
  },
  {
    to: '/shop/badges',
    icon: 'pi pi-shop',
    title: '徽章商城',
    description: '用金币购买并装备专属徽章。',
  },
  {
    to: '/shop/titles',
    icon: 'pi pi-crown',
    title: '称号商店',
    description: '解锁称号，展示你的竞赛身份。',
  },
]

const appearanceLink: HomeLink = {
  to: '/appearance',
  icon: 'pi pi-palette',
  title: '装扮管理',
  description: '管理已拥有的徽章与称号。',
}

const visibleGrowthLinks = computed(() =>
  auth.isLoggedIn ? [...growthLinks, appearanceLink] : growthLinks,
)
</script>

<template>
  <div class="home">
    <AppSurface as="section" tone="accent" padding="lg" class="hero" aria-labelledby="home-title">
      <div class="hero__copy">
        <p class="hero__eyebrow">KAPOJ // ONLINE JUDGE</p>
        <h1 id="home-title" class="hero__title">欢迎来到 <span>kapOJ</span></h1>
        <p class="hero__description">
          题库、比赛、排行榜、成就与任务已经上线。完成成就与任务赚取金币，再用金币购买并装扮徽章和称号。每日签到还能领取活跃度与金币。
        </p>
        <div class="hero__actions">
          <Button
            as="router-link"
            to="/problems"
            label="开始刷题"
            icon="pi pi-arrow-right"
            icon-pos="right"
          />
          <Button
            as="router-link"
            to="/contests"
            label="浏览比赛"
            icon="pi pi-trophy"
            severity="secondary"
            outlined
          />
        </div>
      </div>

      <aside class="hero__status" aria-label="系统状态">
        <div class="hero__status-head">
          <span class="hero__status-dots" aria-hidden="true">
            <i />
            <i />
            <i />
          </span>
          <span>KAPOJ // STATUS</span>
          <span class="hero__status-version">M4-C</span>
        </div>
        <ul class="hero__status-list">
          <li><i class="pi pi-check-circle" aria-hidden="true" /> 多语言实时评测</li>
          <li><i class="pi pi-check-circle" aria-hidden="true" /> ICPC / IOI / OI 赛制</li>
          <li><i class="pi pi-check-circle" aria-hidden="true" /> Rating 结算与 8 级等级</li>
          <li><i class="pi pi-check-circle" aria-hidden="true" /> 排行榜、签到与 Rating 历史</li>
          <li class="is-pending">
            <i class="pi pi-clock" aria-hidden="true" /> 学习路线 · 即将开放
          </li>
        </ul>
        <div class="hero__status-foot">
          <span class="hero__pulse" aria-hidden="true" />
          系统就绪
        </div>
      </aside>
    </AppSurface>

    <AppSurface
      v-if="auth.isLoggedIn && auth.user"
      as="section"
      tone="raised"
      padding="lg"
      class="profile-strip"
      aria-labelledby="home-profile-title"
    >
      <div class="profile-strip__identity">
        <Avatar
          :image="auth.user.avatarUrl || undefined"
          :label="auth.user.nickname.slice(0, 1)"
          shape="square"
          size="xlarge"
          class="profile-strip__avatar"
        />
        <div>
          <p class="section-eyebrow">WELCOME BACK</p>
          <h2 id="home-profile-title" class="profile-strip__name">{{ auth.user.nickname }}</h2>
          <p class="profile-strip__username">@{{ auth.user.username }}</p>
        </div>
      </div>
      <dl class="profile-strip__stats">
        <div>
          <dt>Rating</dt>
          <dd class="num">{{ auth.user.rating }}</dd>
        </div>
        <div>
          <dt>金币</dt>
          <dd class="num">{{ formatCoins(auth.user.coins) }}</dd>
        </div>
      </dl>
      <Button
        as="router-link"
        to="/settings/profile"
        label="编辑资料"
        icon="pi pi-user-edit"
        severity="secondary"
        outlined
      />
    </AppSurface>

    <AppSurface
      v-else-if="auth.isLoggedIn"
      tone="raised"
      padding="lg"
      class="profile-strip"
      aria-label="档案加载中"
    >
      <AppLoadingState label="正在加载你的档案" />
    </AppSurface>

    <AppSurface
      v-else
      as="section"
      tone="default"
      padding="lg"
      class="guest-strip"
      aria-labelledby="home-guest-title"
    >
      <div>
        <p class="section-eyebrow">GUEST ACCESS</p>
        <h2 id="home-guest-title" class="guest-strip__title">登录后记录你的成长</h2>
        <p class="guest-strip__description">
          保存提交、领取任务、解锁成就，并用金币装扮徽章与称号。
        </p>
      </div>
      <div class="guest-strip__actions">
        <Button as="router-link" to="/login" label="登录" />
        <Button as="router-link" to="/register" label="注册" severity="secondary" outlined />
      </div>
    </AppSurface>

    <section class="home-section" aria-labelledby="home-quick-title">
      <div class="section-head">
        <p class="section-eyebrow">QUICK START</p>
        <h2 id="home-quick-title" class="section-title">快速开始</h2>
      </div>
      <div class="card-grid">
        <RouterLink v-for="link in quickLinks" :key="link.to" :to="link.to" class="card-link">
          <AppSurface as="article" tone="default" padding="md" interactive class="home-card">
            <span class="home-card__icon"><i :class="link.icon" aria-hidden="true" /></span>
            <h3 class="home-card__title">{{ link.title }}</h3>
            <p class="home-card__description">{{ link.description }}</p>
            <span class="home-card__arrow" aria-hidden="true">
              <i class="pi pi-arrow-up-right" />
            </span>
          </AppSurface>
        </RouterLink>
      </div>
    </section>

    <section class="home-section" aria-labelledby="home-growth-title">
      <div class="section-head">
        <p class="section-eyebrow">GROWTH</p>
        <h2 id="home-growth-title" class="section-title">成长中心</h2>
      </div>
      <div class="card-grid">
        <RouterLink
          v-for="link in visibleGrowthLinks"
          :key="link.to"
          :to="link.to"
          class="card-link"
        >
          <AppSurface as="article" tone="default" padding="md" interactive class="home-card">
            <span class="home-card__icon"><i :class="link.icon" aria-hidden="true" /></span>
            <h3 class="home-card__title">{{ link.title }}</h3>
            <p class="home-card__description">{{ link.description }}</p>
            <span class="home-card__arrow" aria-hidden="true">
              <i class="pi pi-arrow-up-right" />
            </span>
          </AppSurface>
        </RouterLink>
      </div>
    </section>
  </div>
</template>

<style scoped>
.home {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-6);
  max-width: var(--kapoj-content-max);
}

.hero {
  position: relative;
  display: grid;
  grid-template-columns: minmax(0, 1.35fr) minmax(300px, 0.9fr);
  gap: var(--kapoj-space-7);
  overflow: hidden;
}

.hero::after {
  position: absolute;
  top: -45%;
  right: -10%;
  width: 460px;
  height: 460px;
  pointer-events: none;
  content: '';
  background: radial-gradient(circle, var(--kapoj-primary-soft), transparent 68%);
}

.hero__copy,
.hero__status {
  position: relative;
  z-index: 1;
  min-width: 0;
}

.hero__copy {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.hero__eyebrow,
.section-eyebrow {
  margin: 0;
  color: var(--kapoj-primary);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.hero__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: clamp(2rem, 1.5rem + 2vw, 3.1rem);
  line-height: 1.06;
  letter-spacing: -0.045em;
}

.hero__title span {
  color: var(--kapoj-primary);
}

.hero__description {
  max-width: 62ch;
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 14px;
}

.hero__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
  margin-top: var(--kapoj-space-2);
}

.hero__status {
  align-self: center;
  padding: var(--kapoj-space-4);
  background: var(--kapoj-bg);
  border: 1px solid var(--kapoj-border-strong);
  border-radius: var(--kapoj-radius-md);
  box-shadow: inset 0 0 40px var(--kapoj-primary-soft);
}

.hero__status-head,
.hero__status-foot {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  letter-spacing: 0.08em;
}

.hero__status-head {
  padding-bottom: var(--kapoj-space-3);
  border-bottom: 1px solid var(--kapoj-border);
}

.hero__status-dots {
  display: inline-flex;
  gap: 4px;
}

.hero__status-dots i {
  width: 6px;
  height: 6px;
  background: var(--kapoj-text-muted);
  border-radius: 50%;
}

.hero__status-dots i:first-child {
  background: var(--kapoj-danger);
}

.hero__status-dots i:nth-child(2) {
  background: var(--kapoj-warning);
}

.hero__status-dots i:last-child {
  background: var(--kapoj-success);
}

.hero__status-version {
  margin-left: auto;
  color: var(--kapoj-text-secondary);
}

.hero__status-list {
  display: grid;
  gap: var(--kapoj-space-2);
  padding: 0;
  margin: var(--kapoj-space-4) 0;
  list-style: none;
}

.hero__status-list li {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  color: var(--kapoj-text-secondary);
  font-size: 12px;
}

.hero__status-list i {
  width: 14px;
  color: var(--kapoj-success);
  font-size: 11px;
  text-align: center;
}

.hero__status-list li.is-pending {
  color: var(--kapoj-warning);
}

.hero__status-list li.is-pending i {
  color: var(--kapoj-warning);
}

.hero__status-foot {
  padding-top: var(--kapoj-space-3);
  border-top: 1px solid var(--kapoj-border);
}

.hero__pulse {
  width: 6px;
  height: 6px;
  background: var(--kapoj-success);
  border-radius: 50%;
  box-shadow: 0 0 8px var(--kapoj-success);
}

.profile-strip,
.guest-strip {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-4);
}

.profile-strip__identity {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-3);
  min-width: 0;
}

.profile-strip__avatar {
  flex: none;
  width: 56px;
  height: 56px;
  color: var(--kapoj-primary);
  font-size: 22px;
  font-weight: 700;
  background: var(--kapoj-primary-soft);
  border: 1px solid var(--kapoj-border-strong);
  border-radius: var(--kapoj-radius-md);
}

.profile-strip__name {
  margin: 2px 0 0;
  color: var(--kapoj-text);
  font-size: 1.2rem;
  line-height: 1.2;
}

.profile-strip__username {
  margin: 2px 0 0;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
}

.profile-strip__stats {
  display: flex;
  gap: var(--kapoj-space-5);
  margin: 0;
}

.profile-strip__stats div {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.profile-strip__stats dt {
  color: var(--kapoj-text-muted);
  font-size: 10px;
  text-transform: uppercase;
}

.profile-strip__stats dd {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.1rem;
  font-weight: 600;
}

.guest-strip > div:first-child {
  min-width: 0;
}

.guest-strip__title {
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text);
  font-size: 1.35rem;
  line-height: 1.2;
}

.guest-strip__description {
  max-width: 58ch;
  margin: var(--kapoj-space-2) 0 0;
  color: var(--kapoj-text-secondary);
  font-size: 13px;
}

.guest-strip__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
}

.home-section {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.section-head {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
}

.section-title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.2rem;
  line-height: 1.2;
  letter-spacing: -0.02em;
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--kapoj-space-4);
}

.card-link {
  color: inherit;
  text-decoration: none;
}

.card-link:hover {
  text-decoration: none;
}

.home-card {
  position: relative;
  height: 100%;
}

.home-card__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  color: var(--kapoj-primary);
  font-size: 15px;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 28%, transparent);
  border-radius: var(--kapoj-radius-sm);
}

.home-card__title {
  margin: var(--kapoj-space-3) 0 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  line-height: 1.3;
}

.home-card__description {
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text-secondary);
  font-size: 13px;
}

.home-card__arrow {
  position: absolute;
  top: var(--kapoj-space-4);
  right: var(--kapoj-space-4);
  color: var(--kapoj-text-muted);
  font-size: 12px;
  transition:
    color var(--kapoj-transition),
    transform var(--kapoj-transition);
}

.card-link:hover .home-card__arrow {
  color: var(--kapoj-primary);
  transform: translate(2px, -2px);
}

@media (max-width: 900px) {
  .hero {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--kapoj-space-5);
  }

  .hero__status {
    align-self: stretch;
  }

  .profile-strip,
  .guest-strip {
    align-items: flex-start;
  }

  .guest-strip__actions {
    width: 100%;
  }
}

@media (max-width: 600px) {
  .home {
    gap: var(--kapoj-space-5);
  }

  .hero__actions,
  .guest-strip__actions {
    width: 100%;
  }

  .hero__actions :deep(.p-button),
  .guest-strip__actions :deep(.p-button) {
    flex: 1 1 auto;
  }

  .profile-strip__stats {
    width: 100%;
    justify-content: space-between;
  }

  .card-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (prefers-reduced-motion: reduce) {
  .home-card__arrow {
    transition: none;
  }

  .card-link:hover .home-card__arrow {
    transform: none;
  }
}
</style>
