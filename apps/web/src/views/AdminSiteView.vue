<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import DatePicker from 'primevue/datepicker'
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
  updateAdminSiteFooter,
  updateAdminSiteHome,
  uploadAdminSiteFavicon,
  uploadAdminSiteLogo,
} from '@/api/site'
import type { SiteAdminView, SiteFooterInput, SiteHomeContent, SiteLink } from '@/api/types'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useSiteStore } from '@/stores/site'

const ACCEPT_IMAGES = 'image/png,image/jpeg,image/webp,image/gif'
const ACCEPT_TYPES = ['image/png', 'image/jpeg', 'image/webp', 'image/gif']
const LOGO_MAX_BYTES = 2 * 1024 * 1024
const FAVICON_MAX_BYTES = 1 * 1024 * 1024
const MAX_HOME_LINKS = 8

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
const uploadingLogo = ref(false)
const removingLogo = ref(false)
const uploadingFavicon = ref(false)
const removingFavicon = ref(false)

const logoInput = ref<HTMLInputElement | null>(null)
const faviconInput = ref<HTMLInputElement | null>(null)

const homeErrors = ref<FieldErrors>({})
const footerErrors = ref<FieldErrors>({})

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

const logoUrl = computed(() => view.value?.logoUrl ?? '')
const faviconUrl = computed(() => view.value?.faviconUrl ?? '')
const runningDays = computed(() => view.value?.runningDays ?? 0)
const visitCount = computed(() => view.value?.visitCount ?? 0)
const formattedVisits = computed(() => new Intl.NumberFormat('zh-CN').format(visitCount.value))

function applyView(next: SiteAdminView) {
  view.value = next
  homeForm.value = cloneHome(next.home)
  footerForm.value = footerFromView(next)
  homeErrors.value = {}
  footerErrors.value = {}
}

/** Keep the public store in sync so this SPA's header/footer update immediately. */
function applyMutation(next: SiteAdminView) {
  view.value = next
  site.setConfig(next)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    applyView(await getAdminSite())
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
          <Tab value="home">首页</Tab>
          <Tab value="footer">页脚</Tab>
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

          <TabPanel value="home">
            <form class="site-form" data-test="home-form" @submit.prevent="saveHome">
              <section class="section">
                <h2 class="section__title">欢迎区</h2>
                <div class="field-grid">
                  <AppFormField v-slot="{ id }" label="眉标" optional :error="homeErrors.eyebrow">
                    <InputText :id="id" v-model="homeForm.eyebrow" data-test="home-eyebrow" fluid />
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
                      fluid
                    />
                  </AppFormField>
                  <AppFormField v-slot="{ id }" label="主标题" :error="homeErrors.title">
                    <InputText :id="id" v-model="homeForm.title" data-test="home-title" fluid />
                  </AppFormField>
                </div>
                <AppFormField v-slot="{ id }" label="描述" optional :error="homeErrors.description">
                  <Textarea
                    :id="id"
                    v-model="homeForm.description"
                    data-test="home-description"
                    rows="3"
                    auto-resize
                    fluid
                  />
                </AppFormField>
              </section>

              <section class="section">
                <h2 class="section__title">按钮</h2>
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
                  <h2 class="section__title">快速开始</h2>
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
                  <h2 class="section__title">成长中心</h2>
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
                <h2 class="section__title">游客提示</h2>
                <div class="field-grid">
                  <AppFormField v-slot="{ id }" label="标题" :error="homeErrors.guestTitle">
                    <InputText
                      :id="id"
                      v-model="homeForm.guestTitle"
                      data-test="guest-title"
                      fluid
                    />
                  </AppFormField>
                </div>
                <AppFormField
                  v-slot="{ id }"
                  label="描述"
                  optional
                  :error="homeErrors.guestDescription"
                >
                  <Textarea
                    :id="id"
                    v-model="homeForm.guestDescription"
                    data-test="guest-description"
                    rows="3"
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
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  font-weight: 600;
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

.cta-card {
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
