<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import DatePicker from 'primevue/datepicker'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tab from 'primevue/tab'
import Tag from 'primevue/tag'
import TabList from 'primevue/tablist'
import TabPanel from 'primevue/tabpanel'
import TabPanels from 'primevue/tabpanels'
import Tabs from 'primevue/tabs'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import {
  deleteAdminSiteFavicon,
  deleteAdminSiteLogo,
  getAdminSite,
  getAdminSiteCompanion,
  updateAdminSiteAppearance,
  updateAdminSiteCompanion,
  updateAdminSiteDeployment,
  updateAdminSiteFooter,
  updateAdminSiteHome,
  uploadAdminSiteFavicon,
  uploadAdminSiteLogo,
} from '@/api/site'
import type {
  SiteAdminView,
  SiteAppearance,
  SiteCompanionSettings,
  SiteDeployment,
  SiteFooterInput,
  SiteHomeContent,
  SiteLink,
} from '@/api/types'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { DEFAULT_COMPANION, DEFAULT_HOME, useSiteStore } from '@/stores/site'
import { copyText } from '@/utils/clipboard'

const ACCEPT_IMAGES = 'image/png,image/jpeg,image/webp,image/gif'
const ACCEPT_TYPES = ['image/png', 'image/jpeg', 'image/webp', 'image/gif']
const LOGO_MAX_BYTES = 2 * 1024 * 1024
const FAVICON_MAX_BYTES = 1 * 1024 * 1024
const MAX_HOME_LINKS = 8
const DEFAULT_HOME_TAGLINE = DEFAULT_HOME.tagline
const MAX_TAGLINE_LEN = 80
const COMPANION_NAME_MAX = 20

const ACTIVITY_OPTIONS: Array<{
  label: string
  value: SiteCompanionSettings['defaultActivityLevel']
}> = [
  { label: '安静', value: 'quiet' },
  { label: '普通', value: 'normal' },
  { label: '活泼', value: 'lively' },
]

const ICON_OPTIONS: Array<{ label: string; value: string }> = [
  { label: '箭头', value: 'pi pi-arrow-right' },
  { label: '图书', value: 'pi pi-book' },
  { label: '奖杯', value: 'pi pi-trophy' },
  { label: '排行榜', value: 'pi pi-chart-line' },
  { label: '列表', value: 'pi pi-list-check' },
  { label: '成就', value: 'pi pi-star' },
  { label: '任务', value: 'pi pi-check-square' },
  { label: '商城', value: 'pi pi-shop' },
  { label: '皇冠', value: 'pi pi-crown' },
  { label: '装扮', value: 'pi pi-palette' },
  { label: '比赛', value: 'pi pi-flag' },
  { label: '奖励', value: 'pi pi-gift' },
  { label: '身份', value: 'pi pi-id-card' },
  { label: '能量', value: 'pi pi-bolt' },
  { label: '用户', value: 'pi pi-users' },
  { label: '设置', value: 'pi pi-cog' },
  { label: '火花', value: 'pi pi-sparkles' },
]

const ICON_POSITIONS: Array<{ label: string; value: SiteLink['iconPos'] }> = [
  { label: '左侧', value: 'left' },
  { label: '右侧', value: 'right' },
]

interface FooterForm {
  icpNumber: string
  policeNumber: string
  policeUrl: string
  copyright: string
  text: string
  showVisits: boolean
  startedAt: Date | null
}

type FieldErrors = Record<string, string>

const toast = useToast()
const confirm = useConfirm()
const site = useSiteStore()

const tab = ref('branding')

const loading = ref(false)
const error = ref('')
const view = ref<SiteAdminView | null>(null)

const homeSaving = ref(false)
const footerSaving = ref(false)
const appearanceSaving = ref(false)
const companionSaving = ref(false)
const deploymentSaving = ref(false)
const uploadingLogo = ref(false)
const removingLogo = ref(false)
const uploadingFavicon = ref(false)
const removingFavicon = ref(false)

const logoInput = ref<HTMLInputElement | null>(null)
const faviconInput = ref<HTMLInputElement | null>(null)

const homeErrors = ref<FieldErrors>({})
const footerErrors = ref<FieldErrors>({})
const deploymentErrors = ref<FieldErrors>({})
const companionErrors = ref<FieldErrors>({})

function emptyLink(): SiteLink {
  return { icon: 'pi pi-arrow-right', iconPos: 'left', title: '', description: '', to: '/' }
}

function cloneLink(item: SiteLink): SiteLink {
  return { ...item }
}

// Stable per-row keys keep component state attached to the same card when the
// list is reordered or shortened. These keys never reach the API payload.
const cardKeys = new WeakMap<SiteLink, number>()
let nextCardKey = 0

function cardKey(item: SiteLink): number {
  let key = cardKeys.get(item)
  if (key === undefined) {
    nextCardKey += 1
    key = nextCardKey
    cardKeys.set(item, key)
  }
  return key
}

function cloneHome(home: SiteHomeContent): SiteHomeContent {
  return {
    ...home,
    tagline: home.tagline || DEFAULT_HOME_TAGLINE,
    primaryCta: cloneLink(home.primaryCta),
    secondaryCta: cloneLink(home.secondaryCta),
    quickLinks: (home.quickLinks ?? []).map(cloneLink),
    growthLinks: (home.growthLinks ?? []).map(cloneLink),
  }
}

function emptyHome(): SiteHomeContent {
  return {
    eyebrow: '',
    title: '',
    titleHighlight: '',
    tagline: DEFAULT_HOME_TAGLINE,
    description: '',
    primaryCta: emptyLink(),
    secondaryCta: emptyLink(),
    quickTitle: '',
    quickLinks: [],
    growthTitle: '',
    growthLinks: [],
    guestTitle: '',
    guestDescription: '',
  }
}

function emptyFooter(): FooterForm {
  return {
    icpNumber: '',
    policeNumber: '',
    policeUrl: '',
    copyright: '',
    text: '',
    showVisits: true,
    startedAt: null,
  }
}

function footerFromView(next: SiteAdminView): FooterForm {
  const startedAt = new Date(next.startedAt)
  return {
    ...next.footer,
    startedAt: Number.isNaN(startedAt.getTime()) ? null : startedAt,
  }
}

const homeForm = ref<SiteHomeContent>(emptyHome())
const footerForm = ref<FooterForm>(emptyFooter())
const appearanceForm = ref<SiteAppearance>({ badgeSize: 22 })
const companionForm = ref<SiteCompanionSettings>({ ...DEFAULT_COMPANION })

const companionNameLength = computed(() => [...companionForm.value.name].length)

function cloneCompanion(value: SiteCompanionSettings): SiteCompanionSettings {
  return {
    enabled: value.enabled,
    name: value.name,
    defaultActivityLevel: value.defaultActivityLevel,
    contestMute: value.contestMute,
    allowUserDisable: value.allowUserDisable,
  }
}

function emptyDeployment(): SiteDeployment {
  return {
    siteDomain: '',
    apiDomain: '',
    cdnDomain: '',
    assetsDomain: '',
    forceHttps: false,
    notes: '',
  }
}

const deploymentForm = ref<SiteDeployment>(emptyDeployment())

const deploymentText = computed(() => JSON.stringify(deploymentForm.value, null, 2))
const httpsWarning = computed(
  () =>
    deploymentForm.value.forceHttps &&
    [
      deploymentForm.value.siteDomain,
      deploymentForm.value.apiDomain,
      deploymentForm.value.cdnDomain,
    ]
      .filter(Boolean)
      .some((domain) => domain.trim().startsWith('http://')),
)

const logoUrl = computed(() => view.value?.logoUrl ?? '')
const faviconUrl = computed(() => view.value?.faviconUrl ?? '')
const runningDays = computed(() => view.value?.runningDays ?? 0)
const visitCount = computed(() => view.value?.visitCount ?? 0)
const formattedVisits = computed(() => new Intl.NumberFormat('zh-CN').format(visitCount.value))

const previewGreeting = computed(() => {
  const hour = new Date().getHours()
  if (hour < 5 || hour >= 23) return '夜深了，同学'
  if (hour < 9) return '早上好，同学'
  if (hour < 12) return '上午好，同学'
  if (hour < 14) return '中午好，同学'
  if (hour < 18) return '下午好，同学'
  return '晚上好，同学'
})

function applyView(next: SiteAdminView) {
  view.value = next
  homeForm.value = cloneHome(next.home)
  footerForm.value = footerFromView(next)
  appearanceForm.value = { badgeSize: next.appearance?.badgeSize ?? 22 }
  companionForm.value = cloneCompanion(next.companion ?? DEFAULT_COMPANION)
  deploymentForm.value = { ...emptyDeployment(), ...next.deployment }
  homeErrors.value = {}
  footerErrors.value = {}
  deploymentErrors.value = {}
  companionErrors.value = {}
}

/** Keep the public store in sync so this SPA's header/footer update immediately. */
function applyMutation(next: SiteAdminView) {
  view.value = next
  site.setConfig(next)
}

/** The companion endpoints return the bare settings, so merge them into the view. */
function applyCompanion(next: SiteCompanionSettings) {
  if (view.value) {
    view.value = { ...view.value, companion: next }
    site.setConfig(view.value)
  }
  companionForm.value = cloneCompanion(next)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const next = await getAdminSite()
    applyView(next)
    if (!next.companion) {
      // Older deployments may omit companion from the admin view; the
      // dedicated endpoint still knows the current defaults.
      try {
        companionForm.value = cloneCompanion(await getAdminSiteCompanion())
      } catch {
        companionForm.value = cloneCompanion(DEFAULT_COMPANION)
      }
    }
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '加载站点设置失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

function apiErrorMessage(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

function checkText(
  errors: FieldErrors,
  key: string,
  label: string,
  value: string,
  max: number,
  required = false,
) {
  const text = value.trim()
  if (required && text.length === 0) {
    errors[key] = `${label}不能为空`
    return
  }
  if ([...text].length > max) {
    errors[key] = `${label}不能超过 ${max} 个字符`
  }
}

function checkLink(
  errors: FieldErrors,
  prefix: string,
  label: string,
  item: SiteLink,
  maxTitle: number,
) {
  checkText(errors, `${prefix}.title`, `${label}标题`, item.title, maxTitle, true)
  checkText(errors, `${prefix}.description`, `${label}描述`, item.description ?? '', 120)
  const to = item.to.trim()
  if (!to.startsWith('/') || to.startsWith('//')) {
    errors[`${prefix}.to`] = `${label}链接必须以 / 开头`
  }
  const icon = item.icon.trim()
  if (!icon.startsWith('pi pi-') || icon === 'pi pi-') {
    errors[`${prefix}.icon`] = `${label}图标必须以 pi pi- 开头`
  }
  if (item.iconPos !== 'left' && item.iconPos !== 'right') {
    errors[`${prefix}.iconPos`] = `${label}图标位置只能是左或右`
  }
}

function validateHome(): boolean {
  const errors: FieldErrors = {}
  const home = homeForm.value
  checkText(errors, 'eyebrow', '首页眉标', home.eyebrow, 40)
  checkText(errors, 'title', '首页标题', home.title, 60)
  checkText(errors, 'titleHighlight', '首页标题高亮', home.titleHighlight, 30)
  checkText(errors, 'tagline', '首页宣传语', home.tagline ?? '', MAX_TAGLINE_LEN)
  checkText(errors, 'description', '首页描述', home.description, 300)
  checkLink(errors, 'primaryCta', '主按钮', home.primaryCta, 20)
  checkLink(errors, 'secondaryCta', '次按钮', home.secondaryCta, 20)
  checkText(errors, 'quickTitle', '快速开始标题', home.quickTitle, 20)
  checkText(errors, 'growthTitle', '成长中心标题', home.growthTitle, 20)
  home.quickLinks.forEach((item, index) => {
    checkLink(errors, `quickLinks.${index}`, `快速开始第 ${index + 1} 项`, item, 40)
  })
  home.growthLinks.forEach((item, index) => {
    checkLink(errors, `growthLinks.${index}`, `成长中心第 ${index + 1} 项`, item, 40)
  })
  checkText(errors, 'guestTitle', '游客提示标题', home.guestTitle, 60)
  checkText(errors, 'guestDescription', '游客提示描述', home.guestDescription, 300)

  homeErrors.value = errors
  return Object.keys(errors).length === 0
}

function validateFooter(): boolean {
  const errors: FieldErrors = {}
  const footer = footerForm.value
  checkText(errors, 'icpNumber', 'ICP 备案号', footer.icpNumber, 100)
  checkText(errors, 'policeNumber', '公安备案号', footer.policeNumber, 100)
  checkText(errors, 'policeUrl', '公安备案链接', footer.policeUrl, 500)
  if (footer.policeUrl.trim() !== '' && !footer.policeUrl.trim().startsWith('https://')) {
    errors.policeUrl = '公安备案链接必须以 https:// 开头'
  }
  checkText(errors, 'copyright', '版权信息', footer.copyright, 200)
  checkText(errors, 'text', '页脚说明', footer.text, 1000)
  if (!footer.startedAt || Number.isNaN(footer.startedAt.getTime())) {
    errors.startedAt = '请选择网站运行起始日'
  } else if (footer.startedAt.getTime() > Date.now() + 24 * 60 * 60 * 1000) {
    errors.startedAt = '网站运行起始日不能晚于当前时间'
  }

  footerErrors.value = errors
  return Object.keys(errors).length === 0
}

async function saveHome() {
  if (!validateHome()) return
  homeSaving.value = true
  try {
    const next = await updateAdminSiteHome(cloneHome(homeForm.value))
    applyMutation(next)
    homeForm.value = cloneHome(next.home)
    toast.add({ severity: 'success', summary: '首页已保存', life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '保存失败',
      detail: apiErrorMessage(e, '保存首页内容失败，请稍后再试'),
      life: 4000,
    })
  } finally {
    homeSaving.value = false
  }
}

async function saveFooter() {
  if (!validateFooter()) return
  const startedAt = footerForm.value.startedAt
  if (!startedAt) return

  footerSaving.value = true
  try {
    const input: SiteFooterInput = {
      icpNumber: footerForm.value.icpNumber,
      policeNumber: footerForm.value.policeNumber,
      policeUrl: footerForm.value.policeUrl,
      copyright: footerForm.value.copyright,
      text: footerForm.value.text,
      showVisits: footerForm.value.showVisits,
      startedAt: startedAt.toISOString(),
    }
    const next = await updateAdminSiteFooter(input)
    applyMutation(next)
    footerForm.value = footerFromView(next)
    toast.add({ severity: 'success', summary: '页脚已保存', life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '保存失败',
      detail: apiErrorMessage(e, '保存页脚失败，请稍后再试'),
      life: 4000,
    })
  } finally {
    footerSaving.value = false
  }
}

function checkDeploymentDomain(
  errors: FieldErrors,
  key: keyof SiteDeployment,
  label: string,
  value: string,
  required: boolean,
) {
  const raw = value.trim().replace(/\/+$/, '')
  if (!raw) {
    if (required) errors[key] = `${label}不能为空`
    return
  }
  try {
    const parsed = new URL(raw)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      errors[key] = `${label}必须使用 http 或 https`
      return
    }
    if (parsed.username || parsed.password) {
      errors[key] = `${label}不能包含用户名或密码`
      return
    }
    if (parsed.pathname !== '/' || parsed.search || parsed.hash) {
      errors[key] = `${label}只能填写域名，不能包含路径、参数或锚点`
    }
  } catch {
    errors[key] = `${label}格式不正确`
  }
}

function validateDeployment(): boolean {
  const errors: FieldErrors = {}
  checkDeploymentDomain(errors, 'siteDomain', '主站域名', deploymentForm.value.siteDomain, true)
  checkDeploymentDomain(errors, 'apiDomain', 'API 域名', deploymentForm.value.apiDomain, false)
  checkDeploymentDomain(errors, 'cdnDomain', 'CDN 域名', deploymentForm.value.cdnDomain, false)
  checkDeploymentDomain(
    errors,
    'assetsDomain',
    '资源域名',
    deploymentForm.value.assetsDomain,
    false,
  )
  if ([...deploymentForm.value.notes].length > 500) {
    errors.notes = '部署备注不能超过 500 个字符'
  }
  deploymentErrors.value = errors
  return Object.keys(errors).length === 0
}

function normalizedDeployment(): SiteDeployment {
  const clean = (value: string) => value.trim().replace(/\/+$/, '')
  return {
    siteDomain: clean(deploymentForm.value.siteDomain),
    apiDomain: clean(deploymentForm.value.apiDomain),
    cdnDomain: clean(deploymentForm.value.cdnDomain),
    assetsDomain: clean(deploymentForm.value.assetsDomain),
    forceHttps: deploymentForm.value.forceHttps,
    notes: deploymentForm.value.notes.trim(),
  }
}

async function saveDeployment() {
  if (!validateDeployment()) return
  deploymentSaving.value = true
  try {
    const next = await updateAdminSiteDeployment(normalizedDeployment())
    if (view.value) {
      view.value = { ...view.value, deployment: next }
      site.setConfig({
        ...view.value,
        deployment: { cdnDomain: next.cdnDomain, assetsDomain: next.assetsDomain },
      })
    }
    deploymentForm.value = { ...emptyDeployment(), ...next }
    toast.add({ severity: 'success', summary: '部署设置已保存', life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '保存失败',
      detail: apiErrorMessage(e, '保存部署设置失败，请稍后再试'),
      life: 4000,
    })
  } finally {
    deploymentSaving.value = false
  }
}

async function copyDeployment() {
  try {
    await copyText(deploymentText.value)
    toast.add({ severity: 'success', summary: '部署配置已复制', life: 3000 })
  } catch {
    toast.add({ severity: 'warn', summary: '复制失败，请手动选择配置', life: 3000 })
  }
}

async function saveAppearance() {
  const size = appearanceForm.value.badgeSize
  if (!Number.isInteger(size) || size < 16 || size > 48) {
    toast.add({
      severity: 'warn',
      summary: '徽章大小需为 16–48 px',
      life: 3000,
    })
    return
  }

  appearanceSaving.value = true
  try {
    const next = await updateAdminSiteAppearance({ badgeSize: size })
    applyMutation(next)
    appearanceForm.value = { badgeSize: next.appearance?.badgeSize ?? 22 }
    toast.add({ severity: 'success', summary: '外观设置已保存', life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '保存失败',
      detail: apiErrorMessage(e, '保存外观设置失败，请稍后再试'),
      life: 4000,
    })
  } finally {
    appearanceSaving.value = false
  }
}

function validateCompanion(): boolean {
  const errors: FieldErrors = {}
  const name = companionForm.value.name.trim()
  if (name.length === 0) {
    errors.name = '伙伴名称不能为空'
  } else if ([...name].length > COMPANION_NAME_MAX) {
    errors.name = `伙伴名称不能超过 ${COMPANION_NAME_MAX} 个字符`
  }
  if (!ACTIVITY_OPTIONS.some((item) => item.value === companionForm.value.defaultActivityLevel)) {
    errors.defaultActivityLevel = '请选择有效的默认活跃度'
  }
  companionErrors.value = errors
  return Object.keys(errors).length === 0
}

async function saveCompanion() {
  if (!validateCompanion()) return
  companionSaving.value = true
  try {
    const next = await updateAdminSiteCompanion({
      enabled: companionForm.value.enabled,
      name: companionForm.value.name.trim(),
      defaultActivityLevel: companionForm.value.defaultActivityLevel,
      contestMute: companionForm.value.contestMute,
      allowUserDisable: companionForm.value.allowUserDisable,
    })
    applyCompanion(next)
    toast.add({ severity: 'success', summary: '伙伴设置已保存', life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '保存失败',
      detail: apiErrorMessage(e, '保存伙伴设置失败，请稍后再试'),
      life: 4000,
    })
  } finally {
    companionSaving.value = false
  }
}

function addLink(list: SiteLink[]) {
  if (list.length >= MAX_HOME_LINKS) {
    toast.add({ severity: 'warn', summary: `最多只能添加 ${MAX_HOME_LINKS} 张卡片`, life: 3000 })
    return
  }
  list.push(emptyLink())
}

function removeLink(list: SiteLink[], index: number) {
  list.splice(index, 1)
  // Positional error keys would otherwise point at the wrong card.
  homeErrors.value = {}
}

function moveLink(list: SiteLink[], index: number, delta: number) {
  const target = index + delta
  if (target < 0 || target >= list.length) return
  const [item] = list.splice(index, 1)
  list.splice(target, 0, item)
  homeErrors.value = {}
}

function validateUpload(file: File, maxBytes: number, label: string): boolean {
  if (!ACCEPT_TYPES.includes(file.type)) {
    toast.add({
      severity: 'warn',
      summary: '图片格式不支持',
      detail: `${label}仅支持 PNG / JPEG / WebP / GIF`,
      life: 4000,
    })
    return false
  }
  if (file.size > maxBytes) {
    toast.add({
      severity: 'warn',
      summary: '图片过大',
      detail: `${label}不能超过 ${Math.round(maxBytes / 1024 / 1024)} MiB`,
      life: 4000,
    })
    return false
  }
  return true
}

async function onLogoChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file || !validateUpload(file, LOGO_MAX_BYTES, 'Logo')) return

  uploadingLogo.value = true
  try {
    applyMutation(await uploadAdminSiteLogo(file))
    toast.add({ severity: 'success', summary: 'Logo 已上传', life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '上传失败',
      detail: apiErrorMessage(e, '上传 Logo 失败，请稍后再试'),
      life: 4000,
    })
  } finally {
    uploadingLogo.value = false
  }
}

async function onFaviconChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file || !validateUpload(file, FAVICON_MAX_BYTES, '站点图标')) return

  uploadingFavicon.value = true
  try {
    applyMutation(await uploadAdminSiteFavicon(file))
    toast.add({ severity: 'success', summary: '站点图标已上传', life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '上传失败',
      detail: apiErrorMessage(e, '上传站点图标失败，请稍后再试'),
      life: 4000,
    })
  } finally {
    uploadingFavicon.value = false
  }
}

async function removeLogo() {
  removingLogo.value = true
  try {
    applyMutation(await deleteAdminSiteLogo())
    toast.add({ severity: 'success', summary: 'Logo 已移除', life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '移除失败',
      detail: apiErrorMessage(e, '移除 Logo 失败，请稍后再试'),
      life: 4000,
    })
  } finally {
    removingLogo.value = false
  }
}

async function removeFavicon() {
  removingFavicon.value = true
  try {
    applyMutation(await deleteAdminSiteFavicon())
    toast.add({ severity: 'success', summary: '站点图标已移除', life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '移除失败',
      detail: apiErrorMessage(e, '移除站点图标失败，请稍后再试'),
      life: 4000,
    })
  } finally {
    removingFavicon.value = false
  }
}

function confirmRemoveLogo() {
  confirm.require({
    header: '移除 Logo',
    message: '移除后站点将回退为文字品牌，确定继续吗？',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '移除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => void removeLogo(),
  })
}

function confirmRemoveFavicon() {
  confirm.require({
    header: '移除站点图标',
    message: '移除后浏览器将显示默认图标，确定继续吗？',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '移除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => void removeFavicon(),
  })
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="admin-site">
    <AppPageHeader
      eyebrow="ADMIN // SITE"
      title="站点设置"
      description="管理站点 Logo、首页内容与页脚信息，所有修改保存后立即对全站生效。"
    />

    <AppErrorState v-if="error" :description="error" @retry="load" />

    <AppLoadingState v-else-if="loading && !view" label="正在加载站点设置" />

    <AppSurface v-else-if="view" padding="lg" class="admin-site__surface">
      <Tabs v-model:value="tab">
        <TabList>
          <Tab value="branding">品牌</Tab>
          <Tab value="appearance">外观</Tab>
          <Tab value="companion">伙伴</Tab>
          <Tab value="home">首页</Tab>
          <Tab value="footer">页脚</Tab>
          <Tab value="deployment">部署</Tab>
        </TabList>

        <TabPanels>
          <TabPanel value="branding">
            <div class="branding">
              <Message severity="info" :closable="false" role="status">
                上传后会立即替换站点展示；建议使用透明背景图片。
              </Message>

              <div class="branding__grid">
                <section class="branding__card">
                  <div class="branding__head">
                    <h2 class="branding__title">站点 Logo</h2>
                    <Tag
                      :value="logoUrl ? '已上传' : '未上传'"
                      :severity="logoUrl ? 'success' : 'secondary'"
                    />
                  </div>
                  <div class="branding__preview branding__preview--logo">
                    <img
                      v-if="logoUrl"
                      data-test="logo-preview"
                      :src="logoUrl"
                      alt="站点 Logo 预览"
                      class="branding__image"
                    />
                    <span v-else class="branding__placeholder">
                      <i class="pi pi-image" aria-hidden="true" />
                      未上传 Logo
                    </span>
                  </div>
                  <p class="branding__hint">支持 PNG / JPEG / WebP / GIF，不超过 2 MiB。</p>
                  <div class="branding__actions">
                    <input
                      ref="logoInput"
                      data-test="logo-file"
                      type="file"
                      class="visually-hidden"
                      :accept="ACCEPT_IMAGES"
                      @change="onLogoChange"
                    />
                    <Button
                      type="button"
                      label="上传 Logo"
                      icon="pi pi-upload"
                      :loading="uploadingLogo"
                      @click="logoInput?.click()"
                    />
                    <Button
                      v-if="logoUrl"
                      type="button"
                      label="移除 Logo"
                      icon="pi pi-trash"
                      severity="danger"
                      outlined
                      :loading="removingLogo"
                      @click="confirmRemoveLogo"
                    />
                  </div>
                </section>

                <section class="branding__card">
                  <div class="branding__head">
                    <h2 class="branding__title">站点图标</h2>
                    <Tag
                      :value="faviconUrl ? '已上传' : '未上传'"
                      :severity="faviconUrl ? 'success' : 'secondary'"
                    />
                  </div>
                  <div class="branding__preview branding__preview--favicon">
                    <img
                      v-if="faviconUrl"
                      data-test="favicon-preview"
                      :src="faviconUrl"
                      alt="站点图标预览"
                      class="branding__image"
                    />
                    <span v-else class="branding__placeholder">
                      <i class="pi pi-desktop" aria-hidden="true" />
                      未上传站点图标
                    </span>
                  </div>
                  <p class="branding__hint">
                    浏览器标签页图标，支持 PNG / JPEG / WebP / GIF，不超过 1 MiB。
                  </p>
                  <div class="branding__actions">
                    <input
                      ref="faviconInput"
                      data-test="favicon-file"
                      type="file"
                      class="visually-hidden"
                      :accept="ACCEPT_IMAGES"
                      @change="onFaviconChange"
                    />
                    <Button
                      type="button"
                      label="上传图标"
                      icon="pi pi-upload"
                      :loading="uploadingFavicon"
                      @click="faviconInput?.click()"
                    />
                    <Button
                      v-if="faviconUrl"
                      type="button"
                      label="移除图标"
                      icon="pi pi-trash"
                      severity="danger"
                      outlined
                      :loading="removingFavicon"
                      @click="confirmRemoveFavicon"
                    />
                  </div>
                </section>
              </div>
            </div>
          </TabPanel>

          <TabPanel value="appearance">
            <form class="site-form" data-test="appearance-form" @submit.prevent="saveAppearance">
              <section class="section">
                <h2 class="section__title">身份展示</h2>
                <Message severity="info" :closable="false">
                  徽章大小会应用到测评记录、排行榜、比赛榜单、个人主页与装扮预览。
                </Message>
                <div class="field-grid">
                  <AppFormField v-slot="{ id }" label="徽章大小">
                    <InputNumber
                      :id="id"
                      v-model="appearanceForm.badgeSize"
                      input-id="badge-size"
                      data-test="badge-size"
                      :min="16"
                      :max="48"
                      suffix=" px"
                      show-buttons
                      fluid
                    />
                  </AppFormField>
                </div>
                <div class="appearance-preview" data-test="appearance-preview">
                  <span class="appearance-preview__label">预览</span>
                  <span
                    class="appearance-preview__badge"
                    :style="{
                      width: `${appearanceForm.badgeSize}px`,
                      height: `${appearanceForm.badgeSize}px`,
                      fontSize: `${Math.max(8, Math.round(appearanceForm.badgeSize * 0.5))}px`,
                    }"
                  >
                    徽
                  </span>
                  <span class="appearance-preview__name">【称号】昵称</span>
                </div>
              </section>

              <div class="form-actions">
                <Button
                  type="submit"
                  label="保存外观"
                  icon="pi pi-check"
                  :loading="appearanceSaving"
                />
              </div>
            </form>
          </TabPanel>

          <TabPanel value="companion">
            <form class="site-form" data-test="companion-form" @submit.prevent="saveCompanion">
              <Message severity="info" :closable="false">
                伙伴设置会应用到全站：站点开关关闭后所有用户都不显示伙伴，用户级偏好仍会保留。
              </Message>

              <section class="companion-card">
                <div class="section__head">
                  <h2 class="section__title">
                    <i class="pi pi-sparkles" aria-hidden="true" />
                    伙伴设置
                  </h2>
                  <Tag
                    :value="companionForm.enabled ? '已启用' : '已停用'"
                    :severity="companionForm.enabled ? 'success' : 'secondary'"
                  />
                </div>

                <div class="field-grid">
                  <AppFormField
                    v-slot="{ id }"
                    label="伙伴名称"
                    required
                    :hint="`${companionNameLength}/${COMPANION_NAME_MAX}`"
                    :error="companionErrors.name"
                  >
                    <InputText
                      :id="id"
                      v-model="companionForm.name"
                      data-test="companion-name"
                      :maxlength="COMPANION_NAME_MAX"
                      fluid
                    />
                  </AppFormField>
                  <AppFormField
                    v-slot="{ id }"
                    label="默认活跃度"
                    hint="新用户首次启用伙伴时使用"
                    :error="companionErrors.defaultActivityLevel"
                  >
                    <Select
                      v-model="companionForm.defaultActivityLevel"
                      :input-id="id"
                      data-test="companion-activity"
                      :options="ACTIVITY_OPTIONS"
                      option-label="label"
                      option-value="value"
                      fluid
                    />
                  </AppFormField>
                </div>

                <div class="field-grid">
                  <AppFormField v-slot="{ id }" label="站点开关">
                    <div class="toggle-row">
                      <ToggleSwitch
                        v-model="companionForm.enabled"
                        :input-id="id"
                        data-test="companion-enabled"
                      />
                      <span class="toggle-row__status">
                        {{ companionForm.enabled ? '全站显示伙伴' : '全站隐藏伙伴' }}
                      </span>
                    </div>
                  </AppFormField>
                  <AppFormField v-slot="{ id }" label="比赛静音">
                    <div class="toggle-row">
                      <ToggleSwitch
                        v-model="companionForm.contestMute"
                        :input-id="id"
                        data-test="companion-contest-mute"
                      />
                      <span class="toggle-row__status">
                        {{ companionForm.contestMute ? '比赛期间不提示' : '比赛期间正常提示' }}
                      </span>
                    </div>
                  </AppFormField>
                  <AppFormField v-slot="{ id }" label="允许用户关闭">
                    <div class="toggle-row">
                      <ToggleSwitch
                        v-model="companionForm.allowUserDisable"
                        :input-id="id"
                        data-test="companion-allow-disable"
                      />
                      <span class="toggle-row__status">
                        {{
                          companionForm.allowUserDisable
                            ? '用户可以自行关闭伙伴'
                            : '用户不能关闭伙伴'
                        }}
                      </span>
                    </div>
                  </AppFormField>
                </div>
              </section>

              <div class="form-actions">
                <Button
                  type="submit"
                  label="保存伙伴设置"
                  icon="pi pi-check"
                  :loading="companionSaving"
                />
              </div>
            </form>
          </TabPanel>

          <TabPanel value="deployment">
            <form class="site-form" data-test="deployment-form" @submit.prevent="saveDeployment">
              <section class="section">
                <h2 class="section__title">部署域名</h2>
                <Message severity="info" :closable="false">
                  这里只记录部署信息，不会开通云服务器、修改 DNS、申请证书或刷新 CDN；不保存云厂商
                  AccessKey、数据库密码或 JWT 密钥。
                </Message>
                <div class="field-grid">
                  <AppFormField
                    label="主站域名"
                    input-id="deployment-site"
                    required
                    :error="deploymentErrors.siteDomain"
                    hint="例如 https://oj.example.com"
                  >
                    <InputText
                      id="deployment-site"
                      v-model="deploymentForm.siteDomain"
                      data-test="deployment-site"
                      fluid
                      placeholder="https://oj.example.com"
                    />
                  </AppFormField>
                  <AppFormField
                    label="API 域名"
                    input-id="deployment-api"
                    optional
                    :error="deploymentErrors.apiDomain"
                    hint="留空表示同源"
                  >
                    <InputText
                      id="deployment-api"
                      v-model="deploymentForm.apiDomain"
                      data-test="deployment-api"
                      fluid
                      placeholder="https://api.example.com"
                    />
                  </AppFormField>
                  <AppFormField
                    label="CDN 域名"
                    input-id="deployment-cdn"
                    optional
                    :error="deploymentErrors.cdnDomain"
                    hint="留空表示不使用 CDN"
                  >
                    <InputText
                      id="deployment-cdn"
                      v-model="deploymentForm.cdnDomain"
                      data-test="deployment-cdn"
                      fluid
                      placeholder="https://cdn.example.com"
                    />
                  </AppFormField>
                  <AppFormField
                    label="资源/附件域名"
                    input-id="deployment-assets"
                    optional
                    :error="deploymentErrors.assetsDomain"
                    hint="留空回退 CDN"
                  >
                    <InputText
                      id="deployment-assets"
                      v-model="deploymentForm.assetsDomain"
                      data-test="deployment-assets"
                      fluid
                      placeholder="https://assets.example.com"
                    />
                  </AppFormField>
                </div>

                <div class="toggle-block">
                  <span class="toggle-label">安全</span>
                  <div class="toggle-row">
                    <span class="toggle-field">
                      <ToggleSwitch
                        v-model="deploymentForm.forceHttps"
                        input-id="deployment-https"
                        data-test="deployment-https"
                      />
                      <label for="deployment-https">强制 HTTPS</label>
                    </span>
                  </div>
                </div>

                <Message
                  v-if="httpsWarning"
                  severity="warn"
                  :closable="false"
                  data-test="https-warning"
                >
                  已勾选强制 HTTPS，但仍有域名使用 http://，请确认证书与重定向配置。
                </Message>

                <AppFormField
                  label="部署备注"
                  input-id="deployment-notes"
                  optional
                  :error="deploymentErrors.notes"
                  :hint="`${deploymentForm.notes.length}/500`"
                >
                  <Textarea
                    id="deployment-notes"
                    v-model="deploymentForm.notes"
                    data-test="deployment-notes"
                    rows="4"
                    maxlength="500"
                    fluid
                    placeholder="证书、DNS、CDN 刷新等运维备注"
                  />
                </AppFormField>

                <div class="deployment-preview">
                  <div class="deployment-preview__head">
                    <h3>配置预览</h3>
                    <Button
                      type="button"
                      label="复制配置"
                      icon="pi pi-copy"
                      size="small"
                      text
                      data-test="deployment-copy"
                      @click="copyDeployment"
                    />
                  </div>
                  <pre data-test="deployment-preview">{{ deploymentText }}</pre>
                </div>

                <ul class="deployment-checklist">
                  <li>DNS 已解析到服务器，证书覆盖主站/API/CDN 域名。</li>
                  <li>CDN 已配置跨域与缓存策略，附件下载保留 Content-Disposition。</li>
                  <li>强制 HTTPS redirect 与 HSTS 在反向代理层生效。</li>
                </ul>
              </section>

              <div class="form-actions">
                <Button
                  type="submit"
                  label="保存部署设置"
                  icon="pi pi-check"
                  :loading="deploymentSaving"
                />
              </div>
            </form>
          </TabPanel>

          <TabPanel value="home">
            <form class="site-form" data-test="home-form" @submit.prevent="saveHome">
              <Message severity="info" :closable="false">
                首页会自动展示动态问候、真实用户统计、学习概览与平台状态；这里只配置欢迎文案、操作按钮和功能卡片。
              </Message>

              <section class="section home-preview-section">
                <div class="section__head">
                  <h2 class="section__title">
                    <i class="pi pi-eye" aria-hidden="true" />
                    首页预览
                  </h2>
                  <span class="home-preview__label">LIVE PREVIEW</span>
                </div>

                <div class="home-preview" data-test="home-preview">
                  <div class="home-preview__bar">
                    <span class="home-preview__greeting">
                      <span class="home-preview__dot" aria-hidden="true" />
                      {{ previewGreeting }}
                    </span>
                    <span class="home-preview__version">M4-C</span>
                  </div>
                  <h3 class="home-preview__title">
                    {{ homeForm.title || '主标题' }}
                    <span v-if="homeForm.titleHighlight">{{ homeForm.titleHighlight }}</span>
                  </h3>
                  <p class="home-preview__tagline">
                    {{ homeForm.tagline || DEFAULT_HOME_TAGLINE }}
                  </p>
                  <p class="home-preview__description">
                    {{ homeForm.description || '宣传描述会显示在这里。' }}
                  </p>
                  <div class="home-preview__actions">
                    <span v-if="homeForm.primaryCta.title" class="home-preview__button is-primary">
                      {{ homeForm.primaryCta.title }}
                    </span>
                    <span
                      v-if="homeForm.secondaryCta.title"
                      class="home-preview__button is-secondary"
                    >
                      {{ homeForm.secondaryCta.title }}
                    </span>
                  </div>
                  <div class="home-preview__cards">
                    <span
                      v-for="link in homeForm.quickLinks.slice(0, 4)"
                      :key="link.to"
                      class="home-preview__card"
                    >
                      <i :class="link.icon" aria-hidden="true" />
                      {{ link.title || '未命名卡片' }}
                    </span>
                    <span v-if="homeForm.quickLinks.length === 0" class="home-preview__empty">
                      暂无快速开始卡片
                    </span>
                  </div>
                </div>

                <p class="home-preview__note">
                  动态问候、在线状态、个人统计、学习概览与平台状态由真实数据生成，预览中不包含实时数值。
                </p>
              </section>

              <section class="section">
                <h2 class="section__title">
                  <i class="pi pi-sparkles" aria-hidden="true" />
                  欢迎区
                </h2>
                <div class="field-grid">
                  <AppFormField
                    v-slot="{ id }"
                    label="眉标"
                    optional
                    hint="显示在动态问候旁，最多 40 字"
                    :error="homeErrors.eyebrow"
                  >
                    <InputText
                      :id="id"
                      v-model="homeForm.eyebrow"
                      data-test="home-eyebrow"
                      maxlength="40"
                      fluid
                    />
                  </AppFormField>
                  <AppFormField
                    v-slot="{ id }"
                    label="标题高亮"
                    optional
                    :error="homeErrors.titleHighlight"
                  >
                    <InputText
                      :id="id"
                      v-model="homeForm.titleHighlight"
                      data-test="home-title-highlight"
                      maxlength="30"
                      fluid
                    />
                  </AppFormField>
                  <AppFormField v-slot="{ id }" label="主标题" :error="homeErrors.title">
                    <InputText
                      :id="id"
                      v-model="homeForm.title"
                      data-test="home-title"
                      maxlength="60"
                      fluid
                    />
                  </AppFormField>
                </div>
                <AppFormField
                  v-slot="{ id }"
                  label="品牌宣传语"
                  optional
                  :hint="`显示在主标题下方，留空时使用默认宣传语 · ${(homeForm.tagline ?? '').length}/${MAX_TAGLINE_LEN}`"
                  :error="homeErrors.tagline"
                >
                  <InputText
                    :id="id"
                    v-model="homeForm.tagline"
                    data-test="home-tagline"
                    :maxlength="MAX_TAGLINE_LEN"
                    fluid
                  />
                </AppFormField>
                <AppFormField
                  v-slot="{ id }"
                  label="宣传描述"
                  optional
                  :hint="`显示在宣传语下方，建议 1–2 句话 · ${homeForm.description.length}/300`"
                  :error="homeErrors.description"
                >
                  <Textarea
                    :id="id"
                    v-model="homeForm.description"
                    data-test="home-description"
                    rows="3"
                    maxlength="300"
                    auto-resize
                    fluid
                  />
                </AppFormField>
              </section>

              <section class="section">
                <h2 class="section__title">
                  <i class="pi pi-link" aria-hidden="true" />
                  按钮
                </h2>
                <p class="section__hint">
                  主按钮用于引导核心操作，次按钮提供次要入口；留空标题即可隐藏对应按钮。
                </p>
                <div class="cta-grid">
                  <div class="cta-card">
                    <h3 class="cta-card__title">主按钮</h3>
                    <div class="field-grid">
                      <AppFormField
                        v-slot="{ id }"
                        label="标题"
                        :error="homeErrors['primaryCta.title']"
                      >
                        <InputText
                          :id="id"
                          v-model="homeForm.primaryCta.title"
                          data-test="primary-cta-title"
                          fluid
                        />
                      </AppFormField>
                      <AppFormField
                        v-slot="{ id }"
                        label="链接"
                        :error="homeErrors['primaryCta.to']"
                      >
                        <InputText
                          :id="id"
                          v-model="homeForm.primaryCta.to"
                          data-test="primary-cta-to"
                          fluid
                        />
                      </AppFormField>
                      <AppFormField
                        v-slot="{ id }"
                        label="图标"
                        :error="homeErrors['primaryCta.icon']"
                      >
                        <Select
                          v-model="homeForm.primaryCta.icon"
                          :input-id="id"
                          data-test="primary-cta-icon"
                          :options="ICON_OPTIONS"
                          option-label="label"
                          option-value="value"
                          editable
                          fluid
                        />
                      </AppFormField>
                      <AppFormField v-slot="{ id }" label="图标位置">
                        <Select
                          v-model="homeForm.primaryCta.iconPos"
                          :input-id="id"
                          data-test="primary-cta-icon-pos"
                          :options="ICON_POSITIONS"
                          option-label="label"
                          option-value="value"
                          fluid
                        />
                      </AppFormField>
                    </div>
                    <AppFormField
                      v-slot="{ id }"
                      label="描述"
                      optional
                      :error="homeErrors['primaryCta.description']"
                    >
                      <Textarea
                        :id="id"
                        v-model="homeForm.primaryCta.description"
                        data-test="primary-cta-description"
                        rows="2"
                        auto-resize
                        fluid
                      />
                    </AppFormField>
                  </div>

                  <div class="cta-card">
                    <h3 class="cta-card__title">次按钮</h3>
                    <div class="field-grid">
                      <AppFormField
                        v-slot="{ id }"
                        label="标题"
                        :error="homeErrors['secondaryCta.title']"
                      >
                        <InputText
                          :id="id"
                          v-model="homeForm.secondaryCta.title"
                          data-test="secondary-cta-title"
                          fluid
                        />
                      </AppFormField>
                      <AppFormField
                        v-slot="{ id }"
                        label="链接"
                        :error="homeErrors['secondaryCta.to']"
                      >
                        <InputText
                          :id="id"
                          v-model="homeForm.secondaryCta.to"
                          data-test="secondary-cta-to"
                          fluid
                        />
                      </AppFormField>
                      <AppFormField
                        v-slot="{ id }"
                        label="图标"
                        :error="homeErrors['secondaryCta.icon']"
                      >
                        <Select
                          v-model="homeForm.secondaryCta.icon"
                          :input-id="id"
                          data-test="secondary-cta-icon"
                          :options="ICON_OPTIONS"
                          option-label="label"
                          option-value="value"
                          editable
                          fluid
                        />
                      </AppFormField>
                      <AppFormField v-slot="{ id }" label="图标位置">
                        <Select
                          v-model="homeForm.secondaryCta.iconPos"
                          :input-id="id"
                          data-test="secondary-cta-icon-pos"
                          :options="ICON_POSITIONS"
                          option-label="label"
                          option-value="value"
                          fluid
                        />
                      </AppFormField>
                    </div>
                    <AppFormField
                      v-slot="{ id }"
                      label="描述"
                      optional
                      :error="homeErrors['secondaryCta.description']"
                    >
                      <Textarea
                        :id="id"
                        v-model="homeForm.secondaryCta.description"
                        data-test="secondary-cta-description"
                        rows="2"
                        auto-resize
                        fluid
                      />
                    </AppFormField>
                  </div>
                </div>
              </section>

              <section class="section">
                <div class="section__head">
                  <h2 class="section__title">
                    <i class="pi pi-bolt" aria-hidden="true" />
                    快速开始
                  </h2>
                  <Button
                    type="button"
                    label="添加卡片"
                    icon="pi pi-plus"
                    size="small"
                    severity="secondary"
                    outlined
                    data-test="add-quick-link"
                    @click="addLink(homeForm.quickLinks)"
                  />
                </div>
                <AppFormField v-slot="{ id }" label="区块标题" :error="homeErrors.quickTitle">
                  <InputText :id="id" v-model="homeForm.quickTitle" data-test="quick-title" fluid />
                </AppFormField>

                <p v-if="homeForm.quickLinks.length === 0" class="empty-hint">
                  暂无卡片，点击“添加卡片”创建。
                </p>

                <div
                  v-for="(item, index) in homeForm.quickLinks"
                  :key="cardKey(item)"
                  class="card-editor"
                >
                  <div class="card-editor__head">
                    <span class="card-editor__label">卡片 {{ index + 1 }}</span>
                    <div class="card-editor__tools">
                      <Button
                        type="button"
                        icon="pi pi-arrow-up"
                        text
                        rounded
                        size="small"
                        aria-label="上移"
                        :data-test="`quick-up-${index}`"
                        :disabled="index === 0"
                        @click="moveLink(homeForm.quickLinks, index, -1)"
                      />
                      <Button
                        type="button"
                        icon="pi pi-arrow-down"
                        text
                        rounded
                        size="small"
                        aria-label="下移"
                        :data-test="`quick-down-${index}`"
                        :disabled="index === homeForm.quickLinks.length - 1"
                        @click="moveLink(homeForm.quickLinks, index, 1)"
                      />
                      <Button
                        type="button"
                        icon="pi pi-trash"
                        text
                        rounded
                        size="small"
                        severity="danger"
                        aria-label="删除"
                        :data-test="`quick-remove-${index}`"
                        @click="removeLink(homeForm.quickLinks, index)"
                      />
                    </div>
                  </div>
                  <div class="field-grid">
                    <AppFormField
                      v-slot="{ id }"
                      label="图标"
                      :error="homeErrors[`quickLinks.${index}.icon`]"
                    >
                      <Select
                        v-model="item.icon"
                        :input-id="id"
                        :data-test="`quick-icon-${index}`"
                        :options="ICON_OPTIONS"
                        option-label="label"
                        option-value="value"
                        editable
                        fluid
                      />
                    </AppFormField>
                    <AppFormField v-slot="{ id }" label="图标位置">
                      <Select
                        v-model="item.iconPos"
                        :input-id="id"
                        :data-test="`quick-icon-pos-${index}`"
                        :options="ICON_POSITIONS"
                        option-label="label"
                        option-value="value"
                        fluid
                      />
                    </AppFormField>
                    <AppFormField
                      v-slot="{ id }"
                      label="标题"
                      :error="homeErrors[`quickLinks.${index}.title`]"
                    >
                      <InputText
                        :id="id"
                        v-model="item.title"
                        :data-test="`quick-title-${index}`"
                        fluid
                      />
                    </AppFormField>
                    <AppFormField
                      v-slot="{ id }"
                      label="链接"
                      :error="homeErrors[`quickLinks.${index}.to`]"
                    >
                      <InputText
                        :id="id"
                        v-model="item.to"
                        :data-test="`quick-to-${index}`"
                        fluid
                      />
                    </AppFormField>
                  </div>
                  <AppFormField
                    v-slot="{ id }"
                    label="描述"
                    optional
                    :error="homeErrors[`quickLinks.${index}.description`]"
                  >
                    <Textarea
                      :id="id"
                      v-model="item.description"
                      :data-test="`quick-desc-${index}`"
                      rows="2"
                      auto-resize
                      fluid
                    />
                  </AppFormField>
                </div>
              </section>

              <section class="section">
                <div class="section__head">
                  <h2 class="section__title">
                    <i class="pi pi-star" aria-hidden="true" />
                    成长中心
                  </h2>
                  <Button
                    type="button"
                    label="添加卡片"
                    icon="pi pi-plus"
                    size="small"
                    severity="secondary"
                    outlined
                    data-test="add-growth-link"
                    @click="addLink(homeForm.growthLinks)"
                  />
                </div>
                <AppFormField v-slot="{ id }" label="区块标题" :error="homeErrors.growthTitle">
                  <InputText
                    :id="id"
                    v-model="homeForm.growthTitle"
                    data-test="growth-title"
                    fluid
                  />
                </AppFormField>

                <p v-if="homeForm.growthLinks.length === 0" class="empty-hint">
                  暂无卡片，点击“添加卡片”创建。
                </p>

                <div
                  v-for="(item, index) in homeForm.growthLinks"
                  :key="cardKey(item)"
                  class="card-editor"
                >
                  <div class="card-editor__head">
                    <span class="card-editor__label">卡片 {{ index + 1 }}</span>
                    <div class="card-editor__tools">
                      <Button
                        type="button"
                        icon="pi pi-arrow-up"
                        text
                        rounded
                        size="small"
                        aria-label="上移"
                        :data-test="`growth-up-${index}`"
                        :disabled="index === 0"
                        @click="moveLink(homeForm.growthLinks, index, -1)"
                      />
                      <Button
                        type="button"
                        icon="pi pi-arrow-down"
                        text
                        rounded
                        size="small"
                        aria-label="下移"
                        :data-test="`growth-down-${index}`"
                        :disabled="index === homeForm.growthLinks.length - 1"
                        @click="moveLink(homeForm.growthLinks, index, 1)"
                      />
                      <Button
                        type="button"
                        icon="pi pi-trash"
                        text
                        rounded
                        size="small"
                        severity="danger"
                        aria-label="删除"
                        :data-test="`growth-remove-${index}`"
                        @click="removeLink(homeForm.growthLinks, index)"
                      />
                    </div>
                  </div>
                  <div class="field-grid">
                    <AppFormField
                      v-slot="{ id }"
                      label="图标"
                      :error="homeErrors[`growthLinks.${index}.icon`]"
                    >
                      <Select
                        v-model="item.icon"
                        :input-id="id"
                        :data-test="`growth-icon-${index}`"
                        :options="ICON_OPTIONS"
                        option-label="label"
                        option-value="value"
                        editable
                        fluid
                      />
                    </AppFormField>
                    <AppFormField v-slot="{ id }" label="图标位置">
                      <Select
                        v-model="item.iconPos"
                        :input-id="id"
                        :data-test="`growth-icon-pos-${index}`"
                        :options="ICON_POSITIONS"
                        option-label="label"
                        option-value="value"
                        fluid
                      />
                    </AppFormField>
                    <AppFormField
                      v-slot="{ id }"
                      label="标题"
                      :error="homeErrors[`growthLinks.${index}.title`]"
                    >
                      <InputText
                        :id="id"
                        v-model="item.title"
                        :data-test="`growth-title-${index}`"
                        fluid
                      />
                    </AppFormField>
                    <AppFormField
                      v-slot="{ id }"
                      label="链接"
                      :error="homeErrors[`growthLinks.${index}.to`]"
                    >
                      <InputText
                        :id="id"
                        v-model="item.to"
                        :data-test="`growth-to-${index}`"
                        fluid
                      />
                    </AppFormField>
                  </div>
                  <AppFormField
                    v-slot="{ id }"
                    label="描述"
                    optional
                    :error="homeErrors[`growthLinks.${index}.description`]"
                  >
                    <Textarea
                      :id="id"
                      v-model="item.description"
                      :data-test="`growth-desc-${index}`"
                      rows="2"
                      auto-resize
                      fluid
                    />
                  </AppFormField>
                </div>
              </section>

              <section class="section">
                <h2 class="section__title">
                  <i class="pi pi-user" aria-hidden="true" />
                  游客提示
                </h2>
                <p class="section__hint">未登录访客会在欢迎区下方看到这段登录引导。</p>
                <div class="field-grid">
                  <AppFormField v-slot="{ id }" label="标题" :error="homeErrors.guestTitle">
                    <InputText
                      :id="id"
                      v-model="homeForm.guestTitle"
                      data-test="guest-title"
                      maxlength="60"
                      fluid
                    />
                  </AppFormField>
                </div>
                <AppFormField
                  v-slot="{ id }"
                  label="描述"
                  optional
                  :hint="`${homeForm.guestDescription.length}/300`"
                  :error="homeErrors.guestDescription"
                >
                  <Textarea
                    :id="id"
                    v-model="homeForm.guestDescription"
                    data-test="guest-description"
                    rows="3"
                    maxlength="300"
                    auto-resize
                    fluid
                  />
                </AppFormField>
              </section>

              <div class="form-actions">
                <Button type="submit" label="保存首页" icon="pi pi-check" :loading="homeSaving" />
              </div>
            </form>
          </TabPanel>

          <TabPanel value="footer">
            <form class="site-form" data-test="footer-form" @submit.prevent="saveFooter">
              <section class="section">
                <h2 class="section__title">备案信息</h2>
                <div class="field-grid">
                  <AppFormField
                    v-slot="{ id }"
                    label="ICP 备案号"
                    optional
                    :error="footerErrors.icpNumber"
                  >
                    <InputText
                      :id="id"
                      v-model="footerForm.icpNumber"
                      data-test="footer-icp"
                      fluid
                    />
                  </AppFormField>
                  <AppFormField
                    v-slot="{ id }"
                    label="公安备案号"
                    optional
                    :error="footerErrors.policeNumber"
                  >
                    <InputText
                      :id="id"
                      v-model="footerForm.policeNumber"
                      data-test="footer-police"
                      fluid
                    />
                  </AppFormField>
                </div>
                <AppFormField
                  v-slot="{ id }"
                  label="公安备案链接"
                  optional
                  hint="留空或填写 https:// 开头的链接"
                  :error="footerErrors.policeUrl"
                >
                  <InputText
                    :id="id"
                    v-model="footerForm.policeUrl"
                    data-test="footer-police-url"
                    fluid
                  />
                </AppFormField>
              </section>

              <section class="section">
                <h2 class="section__title">版权与说明</h2>
                <AppFormField
                  v-slot="{ id }"
                  label="版权信息"
                  optional
                  :error="footerErrors.copyright"
                >
                  <InputText
                    :id="id"
                    v-model="footerForm.copyright"
                    data-test="footer-copyright"
                    fluid
                  />
                </AppFormField>
                <AppFormField
                  v-slot="{ id }"
                  label="自定义说明"
                  optional
                  hint="保留换行；留空则页脚不显示该行"
                  :error="footerErrors.text"
                >
                  <Textarea
                    :id="id"
                    v-model="footerForm.text"
                    data-test="footer-text"
                    rows="4"
                    auto-resize
                    fluid
                  />
                </AppFormField>
              </section>

              <section class="section">
                <h2 class="section__title">运行信息</h2>
                <div class="field-grid">
                  <AppFormField v-slot="{ id }" label="运行起始日" :error="footerErrors.startedAt">
                    <DatePicker
                      v-model="footerForm.startedAt"
                      :input-id="id"
                      data-test="footer-started-at"
                      date-format="yy-mm-dd"
                      show-icon
                      fluid
                    />
                  </AppFormField>
                  <AppFormField v-slot="{ id }" label="显示访问量">
                    <div class="toggle-row">
                      <ToggleSwitch
                        v-model="footerForm.showVisits"
                        :input-id="id"
                        data-test="footer-show-visits"
                      />
                      <span class="toggle-row__status">
                        {{ footerForm.showVisits ? '页脚显示访问量' : '页脚隐藏访问量' }}
                      </span>
                    </div>
                  </AppFormField>
                </div>

                <dl class="stats">
                  <div class="stats__item">
                    <dt>本站已运行</dt>
                    <dd>
                      <span data-test="running-days">{{ runningDays }}</span> 天
                    </dd>
                  </div>
                  <div class="stats__item">
                    <dt>累计访问量</dt>
                    <dd data-test="visit-count">{{ formattedVisits }}</dd>
                  </div>
                </dl>
                <p class="stats__hint">运行天数与访问量由服务端统计，仅在此处只读展示。</p>
              </section>

              <div class="form-actions">
                <Button type="submit" label="保存页脚" icon="pi pi-check" :loading="footerSaving" />
              </div>
            </form>
          </TabPanel>
        </TabPanels>
      </Tabs>
    </AppSurface>
  </div>
</template>

<style scoped>
.admin-site {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.admin-site__surface {
  min-width: 0;
}

.branding {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  padding-top: var(--kapoj-space-3);
}

.branding__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: var(--kapoj-space-4);
}

.branding__card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-4);
  background: var(--kapoj-bg);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.branding__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
}

.branding__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 0.95rem;
  font-weight: 600;
}

.branding__preview {
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  background: var(--kapoj-surface);
  border: 1px dashed var(--kapoj-border-strong);
  border-radius: var(--kapoj-radius-md);
}

.branding__preview--logo {
  min-height: 120px;
}

.branding__preview--favicon {
  min-height: 96px;
}

.branding__image {
  max-width: 100%;
  max-height: 96px;
  object-fit: contain;
}

.branding__preview--favicon .branding__image {
  width: 48px;
  height: 48px;
}

.branding__placeholder {
  display: inline-flex;
  flex-direction: column;
  align-items: center;
  gap: var(--kapoj-space-2);
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.branding__placeholder i {
  font-size: 22px;
}

.branding__hint {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
  line-height: 1.5;
}

.branding__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
}

.site-form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
  padding-top: var(--kapoj-space-3);
}

.section {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  padding-bottom: var(--kapoj-space-4);
  border-bottom: 1px solid var(--kapoj-border);
}

.section:last-of-type {
  padding-bottom: 0;
  border-bottom: 0;
}

.section__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
}

.section__title {
  display: flex;
  gap: var(--kapoj-space-2);
  align-items: center;
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  font-weight: 600;
}

.section__title i {
  color: var(--kapoj-brand);
  font-size: 0.9em;
}

.section__hint {
  margin: calc(var(--kapoj-space-2) * -1) 0 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
  line-height: 1.5;
}

.home-preview-section {
  gap: var(--kapoj-space-3);
}

.home-preview {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: var(--kapoj-space-5);
  background:
    radial-gradient(
      circle at 90% 0%,
      color-mix(in srgb, var(--kapoj-brand) 10%, transparent),
      transparent 42%
    ),
    var(--kapoj-bg);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-lg);
}

.home-preview__label {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  letter-spacing: 0.1em;
}

.home-preview__bar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
  align-items: center;
  justify-content: space-between;
}

.home-preview__greeting {
  display: inline-flex;
  gap: 6px;
  align-items: center;
  padding: 2px var(--kapoj-space-2);
  color: var(--kapoj-primary-strong);
  font-size: 10px;
  font-weight: 600;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-brand) 22%, transparent);
  border-radius: 999px;
}

.home-preview__dot {
  width: 5px;
  height: 5px;
  background: currentcolor;
  border-radius: 50%;
}

.home-preview__version {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  letter-spacing: 0.08em;
}

.home-preview__title {
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text);
  font-size: clamp(1.45rem, 1.1rem + 1vw, 2rem);
  font-weight: 750;
  line-height: 1.1;
  letter-spacing: -0.035em;
}

.home-preview__title span {
  color: var(--kapoj-brand);
}

.home-preview__tagline {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 13px;
  font-weight: 600;
  line-height: 1.5;
}

.home-preview__description {
  max-width: 72ch;
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 12px;
  line-height: 1.6;
}

.home-preview__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
  margin-top: var(--kapoj-space-1);
}

.home-preview__button {
  padding: 4px 10px;
  font-size: 11px;
  font-weight: 600;
  line-height: 1.5;
  border-radius: 8px;
}

.home-preview__button.is-primary {
  color: var(--kapoj-text-inverted);
  background: var(--kapoj-primary-strong);
}

.home-preview__button.is-secondary {
  color: var(--kapoj-text-secondary);
  background: transparent;
  border: 1px solid var(--kapoj-border-strong);
}

.home-preview__cards {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding-top: var(--kapoj-space-3);
  margin-top: var(--kapoj-space-1);
  border-top: 1px solid var(--kapoj-border);
}

.home-preview__card {
  display: inline-flex;
  gap: 5px;
  align-items: center;
  padding: 4px 8px;
  color: var(--kapoj-text-secondary);
  font-size: 11px;
  background: var(--kapoj-surface);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.home-preview__card i {
  color: var(--kapoj-brand);
  font-size: 10px;
}

.home-preview__empty {
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.home-preview__note {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 11px;
  line-height: 1.5;
}

.field-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--kapoj-space-3);
}

.cta-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: var(--kapoj-space-4);
}

.cta-card,
.companion-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-4);
  background: var(--kapoj-bg);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.cta-card__title {
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.85rem;
  font-weight: 600;
}

.card-editor {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-4);
  background: var(--kapoj-bg);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.card-editor__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
}

.card-editor__label {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.card-editor__tools {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-1);
}

.empty-hint {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
}

.appearance-preview {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  margin-top: var(--kapoj-space-4);
  padding: var(--kapoj-space-3);
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.appearance-preview__label {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.appearance-preview__badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--kapoj-primary);
  font-weight: 700;
  background: var(--kapoj-primary-soft);
  border-radius: 4px;
}

.appearance-preview__name {
  color: var(--kapoj-text);
  font-weight: 600;
}

.toggle-row {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  min-height: 42px;
}

.toggle-row__status {
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
}

.stats {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-4);
  margin: 0;
  padding: var(--kapoj-space-3) var(--kapoj-space-4);
  background: var(--kapoj-bg);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.stats__item {
  display: flex;
  align-items: baseline;
  gap: var(--kapoj-space-2);
}

.stats__item dt {
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.stats__item dd {
  margin: 0;
  color: var(--kapoj-text);
  font-family: var(--kapoj-font-mono);
  font-size: 13px;
}

.stats__hint {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.visually-hidden {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

.deployment-preview {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  padding: var(--kapoj-space-3);
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.deployment-preview__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
}

.deployment-preview__head h3 {
  margin: 0;
  font-size: 0.9rem;
}

.deployment-preview pre {
  max-height: 240px;
  margin: 0;
  overflow: auto;
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-word;
}

.deployment-checklist {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-left: 1.1rem;
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

@media (max-width: 640px) {
  .form-actions :deep(.p-button) {
    width: 100%;
  }

  .branding__actions :deep(.p-button) {
    flex: 1 1 auto;
  }

  .stats {
    flex-direction: column;
    gap: var(--kapoj-space-2);
  }
}
</style>
