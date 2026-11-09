import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import DatePicker from 'primevue/datepicker'
import Select from 'primevue/select'
import ToggleSwitch from 'primevue/toggleswitch'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  deleteAdminSiteFavicon,
  deleteAdminSiteLogo,
  getAdminSite,
  updateAdminSiteFooter,
  updateAdminSiteHome,
  uploadAdminSiteFavicon,
  uploadAdminSiteLogo,
} from '@/api/site'
import { ApiError } from '@/api/http'
import type { SiteAdminView, SiteHomeContent, SiteLink } from '@/api/types'
import { useSiteStore } from '@/stores/site'
import AdminSiteView from './AdminSiteView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/api/site', () => ({
  getSite: vi.fn(),
  recordSiteVisit: vi.fn(),
  getAdminSite: vi.fn(),
  updateAdminSiteHome: vi.fn(),
  updateAdminSiteFooter: vi.fn(),
  uploadAdminSiteLogo: vi.fn(),
  deleteAdminSiteLogo: vi.fn(),
  uploadAdminSiteFavicon: vi.fn(),
  deleteAdminSiteFavicon: vi.fn(),
}))

function link(overrides: Partial<SiteLink> = {}): SiteLink {
  return {
    icon: 'pi pi-book',
    iconPos: 'left',
    title: '题库',
    description: '按标签与难度筛选，开始一次练习。',
    to: '/problems',
    ...overrides,
  }
}

function home(overrides: Partial<SiteHomeContent> = {}): SiteHomeContent {
  return {
    eyebrow: 'KAPOJ // ONLINE JUDGE',
    title: '欢迎来到',
    titleHighlight: 'kapOJ',
    description: '题库、比赛、排行榜、成就与任务已经上线。',
    primaryCta: link({
      icon: 'pi pi-arrow-right',
      iconPos: 'right',
      title: '开始刷题',
      description: '',
      to: '/problems',
    }),
    secondaryCta: link({
      icon: 'pi pi-trophy',
      iconPos: 'left',
      title: '浏览比赛',
      description: '',
      to: '/contests',
    }),
    quickTitle: '快速开始',
    quickLinks: [link()],
    growthTitle: '成长中心',
    growthLinks: [
      link({
        icon: 'pi pi-star',
        title: '成就墙',
        description: '完成条件，解锁成就与奖励。',
        to: '/achievements',
      }),
    ],
    guestTitle: '登录后记录你的成长',
    guestDescription: '保存提交、领取任务、解锁成就。',
    ...overrides,
  }
}

function siteView(overrides: Partial<SiteAdminView> = {}): SiteAdminView {
  return {
    logoUrl: '',
    faviconUrl: '',
    home: home(),
    footer: {
      icpNumber: '京ICP备123号',
      policeNumber: '京公网安备456号',
      policeUrl: 'https://beian.mps.gov.cn/',
      copyright: '© kapOJ',
      text: '本站说明',
      showVisits: true,
    },
    startedAt: '2026-01-01T00:00:00Z',
    runningDays: 12,
    visitCount: 3456,
    ...overrides,
  }
}

async function mountPage() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const wrapper = mount(AdminSiteView, { global: { plugins: [PrimeVue, pinia] } })
  await flushPromises()
  return wrapper
}

type Page = Awaited<ReturnType<typeof mountPage>>

async function openTab(wrapper: Page, label: string) {
  const found = wrapper.findAll('[role="tab"]').find((item) => item.text().trim() === label)
  expect(found, `tab "${label}"`).toBeTruthy()
  await found!.trigger('click')
  await flushPromises()
}

function button(wrapper: Page, label: string) {
  const found = wrapper.findAll('button').find((item) => item.text().trim() === label)
  expect(found, `button "${label}"`).toBeTruthy()
  return found!
}

function inputValue(wrapper: Page, selector: string): string {
  return (wrapper.find(selector).element as HTMLInputElement).value
}

function selectByTest(wrapper: Page, test: string) {
  const found = wrapper
    .findAllComponents(Select)
    .find((item) => item.attributes('data-test') === test)
  expect(found, `select "${test}"`).toBeTruthy()
  return found!
}

async function chooseFile(wrapper: Page, selector: string, file: File) {
  const input = wrapper.find(selector)
  Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
  await input.trigger('change')
  await flushPromises()
}

beforeEach(() => {
  toast.add.mockReset()
  confirm.require.mockReset()
  vi.mocked(getAdminSite).mockReset()
  vi.mocked(updateAdminSiteHome).mockReset()
  vi.mocked(updateAdminSiteFooter).mockReset()
  vi.mocked(uploadAdminSiteLogo).mockReset()
  vi.mocked(deleteAdminSiteLogo).mockReset()
  vi.mocked(uploadAdminSiteFavicon).mockReset()
  vi.mocked(deleteAdminSiteFavicon).mockReset()

  vi.mocked(getAdminSite).mockResolvedValue(siteView())
})

describe('AdminSiteView', () => {
  it('loads and saves homepage content', async () => {
    const wrapper = await mountPage()
    await openTab(wrapper, '首页')

    expect(inputValue(wrapper, '[data-test="home-title"]')).toBe('欢迎来到')
    expect(inputValue(wrapper, '[data-test="quick-title-0"]')).toBe('题库')

    await wrapper.find('[data-test="home-title"]').setValue('新的欢迎语')
    await wrapper
      .find('[data-test="primary-cta-title"]')
      .setValue('这是一个明显超过二十个字符限制的主按钮标题')

    await wrapper.find('form[data-test="home-form"]').trigger('submit')
    await flushPromises()
    expect(updateAdminSiteHome).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('主按钮标题不能超过 20 个字符')

    await wrapper.find('[data-test="primary-cta-title"]').setValue('开始刷题')
    vi.mocked(updateAdminSiteHome).mockResolvedValue(
      siteView({ home: home({ title: '新的欢迎语' }) }),
    )

    await wrapper.find('form[data-test="home-form"]').trigger('submit')
    await flushPromises()

    expect(updateAdminSiteHome).toHaveBeenCalledWith(
      expect.objectContaining({
        title: '新的欢迎语',
        primaryCta: expect.objectContaining({ title: '开始刷题' }),
        quickLinks: expect.any(Array),
        growthLinks: expect.any(Array),
      }),
    )
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
    wrapper.unmount()
  })

  it('refreshes the public site store after a successful save', async () => {
    vi.mocked(updateAdminSiteHome).mockResolvedValue(
      siteView({ home: home({ title: '同步后的标题' }) }),
    )
    const wrapper = await mountPage()
    await openTab(wrapper, '首页')

    await wrapper.find('form[data-test="home-form"]').trigger('submit')
    await flushPromises()

    expect(useSiteStore().config?.home.title).toBe('同步后的标题')
    wrapper.unmount()
  })

  it('rejects non-http(s) links and wrong icon formats before saving', async () => {
    const wrapper = await mountPage()
    await openTab(wrapper, '首页')

    await wrapper.find('[data-test="quick-to-0"]').setValue('problems')
    selectByTest(wrapper, 'quick-icon-0').vm.$emit('update:modelValue', 'book')
    await wrapper.find('form[data-test="home-form"]').trigger('submit')
    await flushPromises()

    expect(updateAdminSiteHome).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('快速开始第 1 项链接必须以 / 开头')
    expect(wrapper.text()).toContain('快速开始第 1 项图标必须以 pi pi- 开头')
    wrapper.unmount()
  })

  it('adds, moves and removes homepage cards', async () => {
    vi.mocked(getAdminSite).mockResolvedValue(
      siteView({
        home: home({
          quickLinks: [link({ title: 'A', to: '/a' }), link({ title: 'B', to: '/b' })],
        }),
      }),
    )
    vi.mocked(updateAdminSiteHome).mockResolvedValue(siteView())
    const wrapper = await mountPage()
    await openTab(wrapper, '首页')

    await wrapper.find('[data-test="add-quick-link"]').trigger('click')
    await wrapper.find('[data-test="quick-title-2"]').setValue('C')
    await wrapper.find('[data-test="quick-to-2"]').setValue('/c')
    await wrapper.find('[data-test="quick-up-2"]').trigger('click')
    await flushPromises()

    expect(inputValue(wrapper, '[data-test="quick-title-1"]')).toBe('C')

    await wrapper.find('[data-test="quick-remove-1"]').trigger('click')
    await flushPromises()
    expect(inputValue(wrapper, '[data-test="quick-title-1"]')).toBe('B')

    await wrapper.find('form[data-test="home-form"]').trigger('submit')
    await flushPromises()

    expect(updateAdminSiteHome).toHaveBeenCalledWith(
      expect.objectContaining({
        quickLinks: [
          expect.objectContaining({ title: 'A', to: '/a' }),
          expect.objectContaining({ title: 'B', to: '/b' }),
        ],
      }),
    )
    wrapper.unmount()
  })

  it('uploads and removes logo and favicon', async () => {
    const wrapper = await mountPage()

    expect(wrapper.find('input[data-test="logo-file"]').attributes('accept')).toBe(
      'image/png,image/jpeg,image/webp,image/gif',
    )
    expect(wrapper.find('input[data-test="favicon-file"]').attributes('accept')).toBe(
      'image/png,image/jpeg,image/webp,image/gif',
    )

    const logoFile = new File(['logo'], 'logo.png', { type: 'image/png' })
    vi.mocked(uploadAdminSiteLogo).mockResolvedValue(siteView({ logoUrl: '/api/site/logo?v=2' }))
    await chooseFile(wrapper, 'input[data-test="logo-file"]', logoFile)

    expect(uploadAdminSiteLogo).toHaveBeenCalledWith(logoFile)
    expect(wrapper.find('img[data-test="logo-preview"]').attributes('src')).toBe(
      '/api/site/logo?v=2',
    )
    expect(useSiteStore().logoUrl).toBe('/api/site/logo?v=2')

    vi.mocked(deleteAdminSiteLogo).mockResolvedValue(siteView())
    await button(wrapper, '移除 Logo').trigger('click')
    expect(confirm.require).toHaveBeenCalledTimes(1)
    confirm.require.mock.calls[0][0].accept()
    await flushPromises()

    expect(deleteAdminSiteLogo).toHaveBeenCalledTimes(1)
    expect(wrapper.find('img[data-test="logo-preview"]').exists()).toBe(false)

    const faviconFile = new File(['icon'], 'favicon.png', { type: 'image/png' })
    vi.mocked(uploadAdminSiteFavicon).mockResolvedValue(
      siteView({ faviconUrl: '/api/site/favicon?v=2' }),
    )
    await chooseFile(wrapper, 'input[data-test="favicon-file"]', faviconFile)

    expect(uploadAdminSiteFavicon).toHaveBeenCalledWith(faviconFile)
    expect(wrapper.find('img[data-test="favicon-preview"]').attributes('src')).toBe(
      '/api/site/favicon?v=2',
    )
    expect(useSiteStore().faviconUrl).toBe('/api/site/favicon?v=2')

    vi.mocked(deleteAdminSiteFavicon).mockResolvedValue(siteView())
    await button(wrapper, '移除图标').trigger('click')
    expect(confirm.require).toHaveBeenCalledTimes(2)
    confirm.require.mock.calls[1][0].accept()
    await flushPromises()

    expect(deleteAdminSiteFavicon).toHaveBeenCalledTimes(1)
    expect(wrapper.find('img[data-test="favicon-preview"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('validates and saves footer fields', async () => {
    const wrapper = await mountPage()
    await openTab(wrapper, '页脚')

    expect(wrapper.text()).toContain('12')
    expect(wrapper.text()).toContain('3,456')
    expect(inputValue(wrapper, '[data-test="footer-icp"]')).toBe('京ICP备123号')

    await wrapper.find('[data-test="footer-police-url"]').setValue('http://example.com')
    await wrapper.find('form[data-test="footer-form"]').trigger('submit')
    await flushPromises()

    expect(updateAdminSiteFooter).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('必须以 https:// 开头')

    await wrapper.find('[data-test="footer-police-url"]').setValue('https://example.com/police')
    wrapper
      .findComponent(DatePicker)
      .vm.$emit('update:modelValue', new Date('2026-02-03T00:00:00Z'))
    wrapper.findComponent(ToggleSwitch).vm.$emit('update:modelValue', false)
    vi.mocked(updateAdminSiteFooter).mockResolvedValue(siteView())

    await wrapper.find('form[data-test="footer-form"]').trigger('submit')
    await flushPromises()

    expect(updateAdminSiteFooter).toHaveBeenCalledWith(
      expect.objectContaining({
        icpNumber: '京ICP备123号',
        policeUrl: 'https://example.com/police',
        showVisits: false,
        startedAt: '2026-02-03T00:00:00.000Z',
      }),
    )
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
    wrapper.unmount()
  })

  it('shows load error and retries', async () => {
    vi.mocked(getAdminSite)
      .mockRejectedValueOnce(new ApiError(500, 50000, '服务器错误'))
      .mockResolvedValueOnce(siteView())

    const wrapper = await mountPage()
    expect(wrapper.text()).toContain('服务器错误')
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(0)

    await button(wrapper, '重试').trigger('click')
    await flushPromises()

    expect(getAdminSite).toHaveBeenCalledTimes(2)
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(3)
    expect(wrapper.text()).toContain('品牌')
    expect(wrapper.text()).toContain('首页')
    expect(wrapper.text()).toContain('页脚')
    wrapper.unmount()
  })
})
