import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { AdminMailView } from '@/api/types'
import AdminMailViewComponent from './AdminMailView.vue'

const mailApi = vi.hoisted(() => ({
  getAdminMail: vi.fn(),
  updateAdminMail: vi.fn(),
  testAdminMail: vi.fn(),
}))

const toast = vi.hoisted(() => ({
  add: vi.fn(),
}))

vi.mock('@/api/mail', () => mailApi)
vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))

function mailView(overrides: Partial<AdminMailView> = {}): AdminMailView {
  return {
    provider: 'qq',
    smtpHost: 'smtp.qq.com',
    smtpPort: 465,
    smtpTls: 'ssl',
    smtpUsername: 'bot@qq.com',
    passwordState: 'none',
    fromEmail: 'bot@qq.com',
    fromName: 'kapOJ',
    replyTo: '',
    publicBaseUrl: 'https://oj.example.com',
    sendingEnabled: false,
    requireEmailVerification: true,
    ready: false,
    updatedBy: 0,
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

async function mountMail(view: AdminMailView) {
  mailApi.getAdminMail.mockResolvedValue(view)
  mailApi.updateAdminMail.mockResolvedValue(view)
  const wrapper = mount(AdminMailViewComponent, {
    global: { plugins: [PrimeVue, createPinia()] },
  })
  await flushPromises()
  return wrapper
}

function buttonByText(wrapper: ReturnType<typeof mount>, text: string) {
  return wrapper.findAll('button').find((button) => button.text().includes(text))
}

beforeEach(() => {
  mailApi.getAdminMail.mockReset()
  mailApi.updateAdminMail.mockReset()
  mailApi.testAdminMail.mockReset()
  toast.add.mockReset()
})

describe('AdminMailView', () => {
  it('loads and saves the settings without sending the password back', async () => {
    const wrapper = await mountMail(
      mailView({ ready: true, sendingEnabled: true, passwordState: 'ready' }),
    )

    const save = buttonByText(wrapper, '保存')
    expect(save).toBeTruthy()
    await save!.trigger('click')
    await flushPromises()

    expect(mailApi.updateAdminMail).toHaveBeenCalledWith(
      expect.objectContaining({
        provider: 'qq',
        smtpHost: 'smtp.qq.com',
        smtpPort: 465,
        smtpTls: 'ssl',
        smtpUsername: 'bot@qq.com',
        fromEmail: 'bot@qq.com',
        publicBaseUrl: 'https://oj.example.com',
        sendingEnabled: true,
        requireEmailVerification: true,
      }),
    )
    expect(mailApi.updateAdminMail.mock.calls[0]![0]).not.toHaveProperty('smtpPassword')
  })

  it('never renders a stored password and shows the password state', async () => {
    const wrapper = await mountMail(
      mailView({ ready: true, sendingEnabled: true, passwordState: 'ready' }),
    )

    const password = wrapper.find('input[type="password"]')
    expect((password.element as HTMLInputElement).value).toBe('')
    expect(wrapper.text()).toContain('已就绪')
  })

  it('blocks enabling enforcement while sending is disabled', async () => {
    const wrapper = await mountMail(
      mailView({ ready: false, sendingEnabled: false, requireEmailVerification: false }),
    )

    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    expect(checkboxes.length).toBeGreaterThanOrEqual(2)
    await checkboxes[1]!.setValue(true)
    const save = buttonByText(wrapper, '保存')
    await save!.trigger('click')
    await flushPromises()

    expect(mailApi.updateAdminMail).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('请先启用邮件发送')
  })

  it('sends a test email using the saved configuration', async () => {
    mailApi.testAdminMail.mockResolvedValue(null)
    const wrapper = await mountMail(
      mailView({ ready: true, sendingEnabled: true, passwordState: 'ready' }),
    )

    await wrapper.find('input[placeholder="收件人邮箱"]').setValue('alice@example.com')
    const test = buttonByText(wrapper, '发送测试')
    await test!.trigger('click')
    await flushPromises()

    expect(mailApi.testAdminMail).toHaveBeenCalledWith('alice@example.com')
  })

  it('rejects an invalid public base URL before saving', async () => {
    const wrapper = await mountMail(mailView({ ready: true, sendingEnabled: true }))
    const url = wrapper.find('input[placeholder="https://oj.example.com"]')
    await url.setValue('https://oj.example.com/subpath')
    const save = buttonByText(wrapper, '保存')
    await save!.trigger('click')
    await flushPromises()

    expect(mailApi.updateAdminMail).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('站点公开地址')
  })
})
