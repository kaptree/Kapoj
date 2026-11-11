import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { listUserCertificates } from '@/api/certificates'
import { ApiError } from '@/api/http'
import type { ProfileCertificate, ProfileCertificatePage } from '@/api/types'
import { formatDateTime } from '@/utils/format'

import ProfileCertificatesCard from './ProfileCertificatesCard.vue'

const auth = vi.hoisted(() => ({ isLoggedIn: false, user: null as { id: number } | null }))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/certificates', () => ({ listUserCertificates: vi.fn() }))

const TARGET_ID = 7

function certificate(overrides: Partial<ProfileCertificate> = {}): ProfileCertificate {
  return {
    id: 1,
    type: 'ACM-ICPC 区域赛',
    level: '银奖',
    verifyCode: 'KAPOJ-ABCD-EFGH',
    approvedAt: '2026-10-02T09:30:00Z',
    ...overrides,
  }
}

function page(overrides: Partial<ProfileCertificatePage> = {}): ProfileCertificatePage {
  return {
    total: 1,
    items: [certificate()],
    ...overrides,
  }
}

const routes = [
  { path: '/users/:id', name: 'user-profile', component: { template: '<div />' } },
  {
    path: '/settings/certificates',
    name: 'certificate-settings',
    component: { template: '<div />' },
  },
  { path: '/verify', name: 'certificate-verify', component: { template: '<div />' } },
]

async function mountCard(props: { targetUserId?: number } = {}) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/users/7')
  await router.isReady()
  const wrapper = mount(ProfileCertificatesCard, {
    props: { targetUserId: TARGET_ID, ...props },
    global: { plugins: [router, PrimeVue] },
  })
  await flushPromises()
  return wrapper
}

function cardItems(wrapper: Awaited<ReturnType<typeof mountCard>>) {
  return wrapper.findAll('[data-test="certificate-item"]')
}

function button(wrapper: Awaited<ReturnType<typeof mountCard>>, label: string) {
  return wrapper.findAll('button').find((item) => item.text().trim() === label)
}

beforeEach(() => {
  auth.isLoggedIn = false
  auth.user = null
  vi.mocked(listUserCertificates).mockReset()
  vi.mocked(listUserCertificates).mockResolvedValue(page())
})

describe('ProfileCertificatesCard', () => {
  it('renders approved certificate details and a verification link', async () => {
    vi.mocked(listUserCertificates).mockResolvedValue(
      page({
        total: 2,
        items: [
          certificate(),
          certificate({
            id: 2,
            type: '蓝桥杯',
            level: '省一',
            verifyCode: 'KAPOJ-IJKL-MNOP',
            approvedAt: '2026-09-12T10:00:00Z',
          }),
        ],
      }),
    )

    const wrapper = await mountCard()

    expect(listUserCertificates).toHaveBeenCalledWith(TARGET_ID)
    const items = cardItems(wrapper)
    expect(items).toHaveLength(2)

    const first = items[0]
    expect(first.text()).toContain('ACM-ICPC 区域赛')
    expect(first.text()).toContain('银奖')
    expect(first.text()).toContain(formatDateTime('2026-10-02T09:30:00Z'))
    expect(first.text()).toContain('KAPOJ-ABCD-EFGH')
    expect(first.text()).toContain('已认证')
    expect(first.find('.pi-verified').exists()).toBe(true)
    expect(first.get('.certificate-item__code').classes()).toContain('mono')

    const verify = first.get('a[href^="/verify?code="]')
    expect(verify.attributes('href')).toBe('/verify?code=KAPOJ-ABCD-EFGH')
    expect(verify.text()).toContain('核验')

    expect(items[1].text()).toContain('蓝桥杯')
    expect(items[1].text()).toContain('KAPOJ-IJKL-MNOP')

    wrapper.unmount()
  })

  it('shows the empty state without a CTA for guests', async () => {
    vi.mocked(listUserCertificates).mockResolvedValue(page({ total: 0, items: [] }))

    const wrapper = await mountCard()

    expect(wrapper.text()).toContain('暂无已认证证书')
    expect(cardItems(wrapper)).toHaveLength(0)
    expect(wrapper.find('a[href="/settings/certificates"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('offers the profile owner a link to submit certificates when empty', async () => {
    auth.isLoggedIn = true
    auth.user = { id: TARGET_ID }
    vi.mocked(listUserCertificates).mockResolvedValue(page({ total: 0, items: [] }))

    const wrapper = await mountCard()

    expect(wrapper.text()).toContain('暂无已认证证书')
    const cta = wrapper.get('a[href="/settings/certificates"]')
    expect(cta.text()).toContain('去提交证书')

    wrapper.unmount()
  })

  it('does not show the submit CTA to another logged-in user', async () => {
    auth.isLoggedIn = true
    auth.user = { id: 99 }
    vi.mocked(listUserCertificates).mockResolvedValue(page({ total: 0, items: [] }))

    const wrapper = await mountCard()

    expect(wrapper.text()).toContain('暂无已认证证书')
    expect(wrapper.find('a[href="/settings/certificates"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('never renders certificate images or rejection details', async () => {
    const wrapper = await mountCard()

    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('驳回')
    expect(wrapper.text()).not.toContain('审核人')

    wrapper.unmount()
  })

  it('renders loading while the certificate request is pending', async () => {
    let resolvePage: (value: ProfileCertificatePage) => void = () => undefined
    vi.mocked(listUserCertificates).mockReturnValue(
      new Promise<ProfileCertificatePage>((resolve) => {
        resolvePage = resolve
      }),
    )

    const wrapper = await mountCard()
    expect(wrapper.text()).toContain('正在加载证书')

    resolvePage(page())
    await flushPromises()
    expect(cardItems(wrapper)).toHaveLength(1)

    wrapper.unmount()
  })

  it('shows an error with retry without breaking the card', async () => {
    vi.mocked(listUserCertificates)
      .mockRejectedValueOnce(new ApiError(500, 50000, '服务器错误'))
      .mockResolvedValueOnce(page())

    const wrapper = await mountCard()

    expect(wrapper.text()).toContain('证书加载失败')
    expect(wrapper.text()).toContain('服务器错误')
    const retry = button(wrapper, '重试')
    expect(retry).toBeTruthy()

    await retry!.trigger('click')
    await flushPromises()

    expect(listUserCertificates).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).not.toContain('服务器错误')
    expect(cardItems(wrapper)).toHaveLength(1)

    wrapper.unmount()
  })

  it('reloads when the target user changes', async () => {
    const wrapper = await mountCard()
    expect(listUserCertificates).toHaveBeenCalledWith(TARGET_ID)

    await wrapper.setProps({ targetUserId: 8 })
    await flushPromises()

    expect(listUserCertificates).toHaveBeenLastCalledWith(8)

    wrapper.unmount()
  })
})
