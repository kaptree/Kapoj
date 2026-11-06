import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import DatePicker from 'primevue/datepicker'
import InputNumber from 'primevue/inputnumber'
import Paginator from 'primevue/paginator'
import Select from 'primevue/select'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { listAdminAudit } from '@/api/admin'
import { ApiError } from '@/api/http'
import type { AdminAuditItem } from '@/api/types'
import AdminAuditView from './AdminAuditView.vue'

const PAGE_SIZE = 20

vi.mock('@/api/admin', () => ({
  listAdminAudit: vi.fn(),
}))

function auditItem(overrides: Partial<AdminAuditItem> = {}): AdminAuditItem {
  return {
    id: 1,
    actorId: 2,
    action: 'user.ban',
    targetType: 'user',
    targetId: 3,
    summary: '封禁用户 alice',
    ip: '127.0.0.1',
    userAgent: 'vitest',
    createdAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

async function mountPage() {
  const wrapper = mount(AdminAuditView, { global: { plugins: [PrimeVue] } })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  vi.mocked(listAdminAudit).mockReset()
  vi.mocked(listAdminAudit).mockResolvedValue({ total: 0, items: [] })
})

describe('AdminAuditView', () => {
  it('loads audit with actor/action/target/date filters and pagination', async () => {
    vi.mocked(listAdminAudit).mockResolvedValue({
      total: 25,
      items: [auditItem(), auditItem({ id: 2, action: 'role.create', summary: '创建角色' })],
    })
    const wrapper = await mountPage()

    expect(listAdminAudit).toHaveBeenCalledWith({
      actorId: undefined,
      action: undefined,
      targetType: undefined,
      from: undefined,
      to: undefined,
      page: 1,
      pageSize: PAGE_SIZE,
    })
    expect(wrapper.text()).toContain('封禁用户 alice')

    wrapper.findComponent(InputNumber).vm.$emit('update:modelValue', 7)
    await wrapper.find('.action-input').setValue('user.ban')
    wrapper.findComponent(Select).vm.$emit('update:modelValue', 'user')
    const pickers = wrapper.findAllComponents(DatePicker)
    pickers[0]!.vm.$emit('update:modelValue', new Date(2026, 0, 1))
    pickers[1]!.vm.$emit('update:modelValue', new Date(2026, 0, 31))
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(listAdminAudit).toHaveBeenLastCalledWith({
      actorId: 7,
      action: 'user.ban',
      targetType: 'user',
      from: '2026-01-01',
      to: '2026-01-31',
      page: 1,
      pageSize: PAGE_SIZE,
    })

    wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()
    expect(listAdminAudit).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 }))
    wrapper.unmount()
  })

  it('formats YYYY-MM-DD from/to and omits empty filters', async () => {
    vi.mocked(listAdminAudit).mockResolvedValue({ total: 0, items: [] })
    const wrapper = await mountPage()

    await wrapper.find('.action-input').setValue('   ')
    const pickers = wrapper.findAllComponents(DatePicker)
    pickers[0]!.vm.$emit('update:modelValue', new Date(2026, 1, 5))
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(listAdminAudit).toHaveBeenLastCalledWith(
      expect.objectContaining({
        actorId: undefined,
        action: undefined,
        from: '2026-02-05',
        to: undefined,
      }),
    )
    wrapper.unmount()
  })

  it('renders summary/target/IP and empty state', async () => {
    vi.mocked(listAdminAudit).mockResolvedValue({
      total: 1,
      items: [auditItem({ targetType: 'contest', targetId: 42 })],
    })
    let wrapper = await mountPage()

    expect(wrapper.text()).toContain('封禁用户 alice')
    expect(wrapper.text()).toContain('contest')
    expect(wrapper.text()).toContain('42')
    expect(wrapper.text()).toContain('127.0.0.1')
    expect(wrapper.text()).toContain('vitest')
    wrapper.unmount()

    vi.mocked(listAdminAudit).mockResolvedValue({ total: 0, items: [] })
    wrapper = await mountPage()
    expect(wrapper.text()).toContain('暂无审计记录')
    wrapper.unmount()
  })

  it('shows API error with retry', async () => {
    vi.mocked(listAdminAudit).mockRejectedValue(new ApiError(403, -1, '没有权限'))
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('没有权限')

    vi.mocked(listAdminAudit).mockResolvedValue({ total: 0, items: [] })
    const retry = wrapper.findAll('button').find((item) => item.text().includes('重试'))
    expect(retry).toBeTruthy()
    await retry!.trigger('click')
    await flushPromises()
    expect(listAdminAudit).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('no write/edit controls', async () => {
    vi.mocked(listAdminAudit).mockResolvedValue({ total: 1, items: [auditItem()] })
    const wrapper = await mountPage()

    for (const label of ['编辑', '删除', '新建', '创建', '保存']) {
      expect(wrapper.findAll('button').some((item) => item.text().trim() === label)).toBe(false)
    }
    wrapper.unmount()
  })
})
