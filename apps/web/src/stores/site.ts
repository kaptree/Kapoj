import { computed, ref, watch } from 'vue'
import { defineStore } from 'pinia'

import { getSite, recordSiteVisit } from '@/api/site'
import { resolveAssetUrl, setAssetBase } from '@/utils/asset'
import type {
  SiteFooterView,
  SiteHomeContent,
  SitePublicDeployment,
  SitePublicView,
} from '@/api/types'

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
  tagline: '每一次提交，都是向更高 Rating 迈进一步。',
  description: '从第一道题开始，构建你的竞赛档案。用真实的提交、比赛与成长记录，见证每一次进步。',
  primaryCta: DEFAULT_PRIMARY_CTA,
  secondaryCta: DEFAULT_SECONDARY_CTA,
  quickTitle: '快速开始',
  quickLinks: [
    {
      icon: 'pi pi-book',
      iconPos: 'left',
      title: '题库',
      description: '按难度与标签筛选，从一道适合你的题开始。',
      to: '/problems',
    },
    {
      icon: 'pi pi-trophy',
      iconPos: 'left',
      title: '比赛',
      description: '参加 ICPC / IOI / OI 赛制，在实时榜单中验证实力。',
      to: '/contests',
    },
    {
      icon: 'pi pi-chart-line',
      iconPos: 'left',
      title: '排行榜',
      description: '查看 Rating、活跃度与贡献榜，找到下一个目标。',
      to: '/leaderboard',
    },
    {
      icon: 'pi pi-list-check',
      iconPos: 'left',
      title: '测评记录',
      description: '追踪每次提交的状态、耗时与内存表现。',
      to: '/submissions',
    },
  ],
  growthTitle: '成长中心',
  growthLinks: [
    {
      icon: 'pi pi-star',
      iconPos: 'left',
      title: '成就墙',
      description: '解锁荣誉节点，收藏属于你的高光时刻。',
      to: '/achievements',
    },
    {
      icon: 'pi pi-check-square',
      iconPos: 'left',
      title: '任务大厅',
      description: '领取限时任务，完成挑战赢取金币奖励。',
      to: '/tasks',
    },
    {
      icon: 'pi pi-shop',
      iconPos: 'left',
      title: '徽章商城',
      description: '用金币收藏徽章，组合你的专属展示。',
      to: '/shop/badges',
    },
    {
      icon: 'pi pi-crown',
      iconPos: 'left',
      title: '称号商店',
      description: '解锁称号，展示你的竞赛身份与风格。',
      to: '/shop/titles',
    },
  ],
  guestTitle: '登录后记录你的成长',
  guestDescription: '保存每一次提交，积累 Rating，领取任务奖励，并用金币装扮你的徽章与称号。',
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
    // `tagline` was added after the first site-settings release. Old rows do
    // not contain it, so the public home keeps the designed product line until
    // an administrator saves a custom one.
    tagline: home.tagline || DEFAULT_HOME.tagline,
    quickLinks: home.quickLinks ?? [],
    growthLinks: home.growthLinks ?? [],
  }
}

function applyFavicon(href: string, uploaded: boolean) {
  if (typeof document === 'undefined') return
  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }
  link.href = href
  if (uploaded) {
    // Uploads are PNG/JPEG/WebP/GIF; the static fallback is the only SVG.
    link.removeAttribute('type')
  } else {
    link.setAttribute('type', 'image/svg+xml')
  }
}

/** Session storage can be blocked by privacy settings or sandboxed frames. */
function hasVisited(): boolean {
  try {
    return sessionStorage.getItem(SITE_VISITED_KEY) !== null
  } catch {
    return false
  }
}

function markVisited(): void {
  try {
    sessionStorage.setItem(SITE_VISITED_KEY, '1')
  } catch {
    // The visit still counts for this page load; only cross-reload dedupe is lost.
  }
}

// Module-level so concurrent layout mounts share one in-flight report.
let visitInFlight = false

export const useSiteStore = defineStore('site', () => {
  const config = ref<SitePublicView | null>(null)
  const latestVisitCount = ref<number | null>(null)

  const home = computed<SiteHomeContent>(() =>
    config.value ? normalizeHome(config.value.home) : DEFAULT_HOME,
  )
  const footer = computed<SiteFooterView>(() => config.value?.footer ?? DEFAULT_FOOTER)
  const deployment = computed<SitePublicDeployment>(
    () => config.value?.deployment ?? { cdnDomain: '', assetsDomain: '' },
  )
  const assetsBaseUrl = computed(() => deployment.value.assetsDomain || deployment.value.cdnDomain)

  watch(
    assetsBaseUrl,
    (base) => {
      setAssetBase(base)
    },
    { immediate: true },
  )

  function assetUrl(path: string): string {
    return resolveAssetUrl(path)
  }

  const logoUrl = computed(() => resolveAssetUrl(config.value?.logoUrl ?? ''))
  const faviconUrl = computed(() => resolveAssetUrl(config.value?.faviconUrl ?? ''))
  const badgeSize = computed(() => config.value?.appearance?.badgeSize ?? 22)
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

  /**
   * Records one visit per browser session. The session marker is written only
   * after a successful request, so a failed report retries on the next mount.
   */
  function recordVisit(): Promise<void> {
    if (visitInFlight) return Promise.resolve()
    if (hasVisited()) return Promise.resolve()

    visitInFlight = true
    return recordSiteVisit()
      .then((result) => {
        latestVisitCount.value = result.visitCount
        if (config.value) config.value.visitCount = result.visitCount
        markVisited()
      })
      .catch(() => undefined)
      .finally(() => {
        visitInFlight = false
      })
  }

  /** Replace the cached public config after a successful admin mutation. */
  function setConfig(next: SitePublicView) {
    config.value = next
  }

  watch(faviconUrl, (value) => applyFavicon(value || '/favicon.svg', Boolean(value)), {
    immediate: true,
  })

  return {
    config,
    home,
    footer,
    deployment,
    assetsBaseUrl,
    assetUrl,
    logoUrl,
    faviconUrl,
    badgeSize,
    runningDays,
    visitCount,
    ensureLoaded,
    recordVisit,
    setConfig,
  }
})
