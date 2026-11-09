import { computed, ref, watch } from 'vue'
import { defineStore } from 'pinia'

import { getSite, recordSiteVisit } from '@/api/site'
import type { SiteFooterView, SiteHomeContent, SitePublicView } from '@/api/types'

/** Session marker so one browser session only reports a single visit. */
export const SITE_VISITED_KEY = 'kapoj:site-visited'

const DEFAULT_PRIMARY_CTA = {
  icon: 'pi pi-arrow-right',
  iconPos: 'right' as const,
  title: '开始刷题',
  description: '',
  to: '/problems',
}

const DEFAULT_SECONDARY_CTA = {
  icon: 'pi pi-trophy',
  iconPos: 'left' as const,
  title: '浏览比赛',
  description: '',
  to: '/contests',
}

/** Mirrors the server-side defaults in apps/server/internal/site/defaults.go. */
export const DEFAULT_HOME: SiteHomeContent = {
  eyebrow: 'KAPOJ // ONLINE JUDGE',
  title: '欢迎来到',
  titleHighlight: 'kapOJ',
  description:
    '题库、比赛、排行榜、成就与任务已经上线。完成成就与任务赚取金币，再用金币购买并装扮徽章和称号。每日签到还能领取活跃度与金币。',
  primaryCta: DEFAULT_PRIMARY_CTA,
  secondaryCta: DEFAULT_SECONDARY_CTA,
  quickTitle: '快速开始',
  quickLinks: [
    {
      icon: 'pi pi-book',
      iconPos: 'left',
      title: '题库',
      description: '按标签与难度筛选，开始一次练习。',
      to: '/problems',
    },
    {
      icon: 'pi pi-trophy',
      iconPos: 'left',
      title: '比赛',
      description: '参加 ICPC / IOI / OI 赛制，挑战实时榜单。',
      to: '/contests',
    },
    {
      icon: 'pi pi-chart-line',
      iconPos: 'left',
      title: '排行榜',
      description: '查看 Rating、活跃度与共享榜，每日签到领金币。',
      to: '/leaderboard',
    },
    {
      icon: 'pi pi-list-check',
      iconPos: 'left',
      title: '测评记录',
      description: '查看提交状态、耗时与内存。',
      to: '/submissions',
    },
  ],
  growthTitle: '成长中心',
  growthLinks: [
    {
      icon: 'pi pi-star',
      iconPos: 'left',
      title: '成就墙',
      description: '完成条件，解锁成就与奖励。',
      to: '/achievements',
    },
    {
      icon: 'pi pi-check-square',
      iconPos: 'left',
      title: '任务大厅',
      description: '领取任务，AC 全部题目赢取金币。',
      to: '/tasks',
    },
    {
      icon: 'pi pi-shop',
      iconPos: 'left',
      title: '徽章商城',
      description: '用金币购买并装备专属徽章。',
      to: '/shop/badges',
    },
    {
      icon: 'pi pi-crown',
      iconPos: 'left',
      title: '称号商店',
      description: '解锁称号，展示你的竞赛身份。',
      to: '/shop/titles',
    },
  ],
  guestTitle: '登录后记录你的成长',
  guestDescription: '保存提交、领取任务、解锁成就，并用金币装扮徽章与称号。',
}

/** Mirrors DefaultFooterInput on the server. */
export const DEFAULT_FOOTER: SiteFooterView = {
  icpNumber: '',
  policeNumber: '',
  policeUrl: '',
  copyright: '© kapOJ',
  text: '',
  showVisits: true,
}

/**
 * Old or abnormally written rows may contain null link arrays. Normalize on the
 * way out so templates can always iterate over an array.
 */
function normalizeHome(home: SiteHomeContent): SiteHomeContent {
  return {
    ...home,
    quickLinks: home.quickLinks ?? [],
    growthLinks: home.growthLinks ?? [],
  }
}

function applyFavicon(href: string) {
  if (typeof document === 'undefined') return
  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }
  link.href = href
}

export const useSiteStore = defineStore('site', () => {
  const config = ref<SitePublicView | null>(null)
  const latestVisitCount = ref<number | null>(null)

  const home = computed<SiteHomeContent>(() =>
    config.value ? normalizeHome(config.value.home) : DEFAULT_HOME,
  )
  const footer = computed<SiteFooterView>(() => config.value?.footer ?? DEFAULT_FOOTER)
  const logoUrl = computed(() => config.value?.logoUrl ?? '')
  const faviconUrl = computed(() => config.value?.faviconUrl ?? '')
  const runningDays = computed(() => config.value?.runningDays ?? 0)
  const visitCount = computed(() => latestVisitCount.value ?? config.value?.visitCount ?? 0)

  let inFlight: Promise<void> | null = null

  async function load() {
    try {
      config.value = await getSite()
    } catch {
      // Keep `config` null so the caller can retry later; the UI falls back to
      // the same copy the backend ships by default.
    }
  }

  /** Loads the public config once; concurrent callers share the same request. */
  function ensureLoaded(): Promise<void> {
    if (config.value) return Promise.resolve()
    if (!inFlight) {
      inFlight = load().finally(() => {
        inFlight = null
      })
    }
    return inFlight
  }

  let visitInFlight: Promise<void> | null = null

  /**
   * Records one visit per browser session. The marker is written in `finally`
   * so a failed request never blocks the page or retries on every mount.
   */
  function recordVisit(): Promise<void> {
    if (typeof sessionStorage === 'undefined') return Promise.resolve()
    if (sessionStorage.getItem(SITE_VISITED_KEY)) return Promise.resolve()
    if (visitInFlight) return visitInFlight

    sessionStorage.setItem(SITE_VISITED_KEY, '1')
    visitInFlight = recordSiteVisit()
      .then((result) => {
        latestVisitCount.value = result.visitCount
        if (config.value) config.value.visitCount = result.visitCount
      })
      .catch(() => undefined)
      .finally(() => {
        visitInFlight = null
      })
    return visitInFlight
  }

  watch(faviconUrl, (value) => applyFavicon(value || '/favicon.svg'), { immediate: true })

  return {
    config,
    home,
    footer,
    logoUrl,
    faviconUrl,
    runningDays,
    visitCount,
    ensureLoaded,
    recordVisit,
  }
})
