import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import InputNumber from 'primevue/inputnumber'
import MultiSelect from 'primevue/multiselect'
import Paginator from 'primevue/paginator'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import {
  adjustAdminUserCoins,
  banAdminUser,
  getAdminUser,
  listAdminRoles,
  listAdminUsers,
  replaceAdminUserRoles,
  unbanAdminUser,
  updateAdminUser,
} from '@/api/admin'
import { ApiError } from '@/api/http'
import type { AdminRoleItem, AdminUserDetail, AdminUserItem, Profile } from '@/api/types'
import { getProfile } from '@/api/users'
import { PERM } from '@/permissions'
import AdminUsersView from './AdminUsersView.vue'

const PAGE_SIZE = 20

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))
const auth = vi.hoisted(() => ({
  can: vi.fn<(code: string) => boolean>(() => false),
  isSuperAdmin: false,
  user: { id: 1 } as { id: number },
}))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/admin', () => ({
  listAdminUsers: vi.fn(),
  getAdminUser: vi.fn(),
  updateAdminUser: vi.fn(),
  replaceAdminUserRoles: vi.fn(),
  banAdminUser: vi.fn(),
  unbanAdminUser: vi.fn(),
  adjustAdminUserCoins: vi.fn(),
  listAdminRoles: vi.fn(),
}))
vi.mock('@/api/users', () => ({ getProfile: vi.fn() }))

function userItem(overrides: Partial<AdminUserItem> = {}): AdminUserItem {
  return {
    id: 1,
    username: 'alice',
    nickname: 'Alice',
    email: 'alice@example.com',
    role: 'user',
    rating: 1500,
    coins: 100,
    status: 'active',
    roles: [],
    createdAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function userDetail(overrides: Partial<AdminUserDetail> = {}): AdminUserDetail {
  return { ...userItem(), isSuperAdmin: false, ...overrides }
}

function roleItem(overrides: Partial<AdminRoleItem> = {}): AdminRoleItem {
  return {
    id: 1,
    code: 'super_admin',
    name: '超级管理员',
    description: '',
    isSystem: true,
    permissions: [],
    userCount: 0,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function profile(overrides: Partial<Profile> = {}): Profile {
  return {
    id: 1,
    username: 'alice',
    nickname: 'Alice',
    avatarUrl: '',
    bio: '',
    rating: 1500,
    coins: 100,
    equippedBadges: [],
    equippedTitle: null,
    createdAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

async function mountPage() {
  const wrapper = mount(AdminUsersView, {
    global: { plugins: [PrimeVue] },
  })
  await flushPromises()
  return wrapper
}

function buttons(wrapper: Awaited<ReturnType<typeof mountPage>>, label: string) {
  return wrapper.findAll('button').filter((item) => item.text().trim() === label)
}

function dialog(): HTMLElement {
  return document.querySelector('.p-dialog') as HTMLElement
}

function dialogButton(label: string) {
  return Array.from(dialog().querySelectorAll('button')).find(
    (item) => item.textContent?.trim() === label,
  )
}

function setField(element: Element, value: string) {
  const proto =
    element instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype
  Object.getOwnPropertyDescriptor(proto, 'value')!.set!.call(element, value)
  element.dispatchEvent(new Event('input', { bubbles: true }))
}

beforeEach(() => {
  toast.add.mockReset()
  confirm.require.mockReset()

  vi.mocked(listAdminUsers).mockReset()
  vi.mocked(getAdminUser).mockReset()
  vi.mocked(updateAdminUser).mockReset()
  vi.mocked(replaceAdminUserRoles).mockReset()
  vi.mocked(banAdminUser).mockReset()
  vi.mocked(unbanAdminUser).mockReset()
  vi.mocked(adjustAdminUserCoins).mockReset()
  vi.mocked(listAdminRoles).mockReset()
  vi.mocked(getProfile).mockReset()

  vi.mocked(listAdminUsers).mockResolvedValue({ total: 0, items: [] })
  vi.mocked(listAdminRoles).mockResolvedValue([])

  auth.can.mockReset()
  auth.can.mockReturnValue(false)
  auth.isSuperAdmin = false
  auth.user = { id: 1 }
})

describe('AdminUsersView', () => {
  it('loads users with filters and pagination', async () => {
    vi.mocked(listAdminUsers).mockResolvedValue({
      total: 25,
      items: [userItem(), userItem({ id: 2, username: 'bob', nickname: 'Bob' })],
    })
    const wrapper = await mountPage()

    expect(listAdminUsers).toHaveBeenCalledWith({
      keyword: undefined,
      status: undefined,
      roleId: undefined,
      page: 1,
      pageSize: PAGE_SIZE,
    })
    expect(wrapper.text()).toContain('Alice')
    expect(wrapper.text()).toContain('Bob')

    await wrapper.find('.keyword-input').setValue('alice')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(listAdminUsers).toHaveBeenLastCalledWith(
      expect.objectContaining({ keyword: 'alice', page: 1 }),
    )

    wrapper.findComponent(Paginator).vm.$emit('page', { page: 1 })
    await flushPromises()
    expect(listAdminUsers).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 }))
    wrapper.unmount()
  })

  it('read-only auditor sees no edit/role/coin/ban actions', async () => {
    auth.can.mockImplementation((code) => code === PERM.role.read)
    vi.mocked(listAdminUsers).mockResolvedValue({ total: 1, items: [userItem()] })
    vi.mocked(listAdminRoles).mockResolvedValue([roleItem()])
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('Alice')
    // The auditor has role.read, so the role filter is loaded and shown.
    expect(listAdminRoles).toHaveBeenCalledTimes(1)

    for (const label of ['编辑', '角色', '金币', '封禁', '解封']) {
      expect(buttons(wrapper, label)).toHaveLength(0)
    }
    wrapper.unmount()
  })

  it('edits nickname/bio/avatar with profile prefill', async () => {
    auth.can.mockImplementation((code) => code === PERM.user.update)
    vi.mocked(listAdminUsers).mockResolvedValue({ total: 1, items: [userItem()] })
    vi.mocked(getAdminUser).mockResolvedValue(userDetail({ id: 1, nickname: 'Alice Admin' }))
    vi.mocked(getProfile).mockResolvedValue(
      profile({ bio: 'hello', avatarUrl: 'https://example.com/a.png' }),
    )
    vi.mocked(updateAdminUser).mockResolvedValue(userDetail())
    const wrapper = await mountPage()

    await buttons(wrapper, '编辑')[0]!.trigger('click')
    await flushPromises()

    expect(getAdminUser).toHaveBeenCalledWith(1)
    expect(getProfile).toHaveBeenCalledWith(1)
    expect((dialog().querySelector('.nickname-input') as HTMLInputElement).value).toBe(
      'Alice Admin',
    )
    expect((dialog().querySelector('.bio-input') as HTMLTextAreaElement).value).toBe('hello')
    expect((dialog().querySelector('.avatar-input') as HTMLInputElement).value).toBe(
      'https://example.com/a.png',
    )

    setField(dialog().querySelector('.nickname-input')!, 'New Nick')
    setField(dialog().querySelector('.bio-input')!, 'new bio')
    setField(dialog().querySelector('.avatar-input')!, 'https://example.com/b.png')
    dialogButton('保存')!.click()
    await flushPromises()

    expect(updateAdminUser).toHaveBeenCalledWith(1, {
      nickname: 'New Nick',
      bio: 'new bio',
      avatarUrl: 'https://example.com/b.png',
    })
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '用户资料已更新' }),
    )
    wrapper.unmount()
  })

  it('assigns roles and disables super_admin for non-super actor', async () => {
    auth.can.mockImplementation(
      (code) => code === PERM.user.roles.assign || code === PERM.role.read,
    )
    auth.isSuperAdmin = false
    vi.mocked(listAdminUsers).mockResolvedValue({
      total: 1,
      items: [userItem({ roles: [{ id: 2, code: 'user_admin', name: '用户管理员' }] })],
    })
    vi.mocked(listAdminRoles).mockResolvedValue([
      roleItem({ id: 1, code: 'super_admin', name: '超级管理员' }),
      roleItem({ id: 2, code: 'user_admin', name: '用户管理员' }),
    ])
    vi.mocked(replaceAdminUserRoles).mockResolvedValue(userDetail())
    const wrapper = await mountPage()

    await buttons(wrapper, '角色')[0]!.trigger('click')
    await flushPromises()

    const options = wrapper.findComponent(MultiSelect).props('options') as Array<{
      value: number
      label: string
      disabled: boolean
    }>
    expect(options.find((option) => option.value === 1)?.disabled).toBe(true)
    expect(options.find((option) => option.value === 2)?.disabled).toBe(false)

    dialogButton('保存')!.click()
    await flushPromises()
    expect(replaceAdminUserRoles).toHaveBeenCalledWith(1, [2])
    wrapper.unmount()
  })

  it('adjusts coins with validation and refreshes', async () => {
    auth.can.mockImplementation((code) => code === PERM.user.coins.adjust)
    vi.mocked(listAdminUsers).mockResolvedValue({ total: 1, items: [userItem()] })
    vi.mocked(adjustAdminUserCoins).mockResolvedValue({ balance: 150 })
    const wrapper = await mountPage()

    await buttons(wrapper, '金币')[0]!.trigger('click')
    await flushPromises()

    // Invalid: amount is zero.
    dialogButton('保存')!.click()
    await flushPromises()
    expect(adjustAdminUserCoins).not.toHaveBeenCalled()
    expect(dialog().textContent).toContain('调整金额不能为 0')

    wrapper.findComponent(InputNumber).vm.$emit('update:modelValue', 100)
    setField(dialog().querySelector('.note-input')!, 'bonus')
    await nextTick()
    dialogButton('保存')!.click()
    await flushPromises()

    expect(adjustAdminUserCoins).toHaveBeenCalledWith(1, { amount: 100, note: 'bonus' })
    expect(listAdminUsers).toHaveBeenCalledTimes(2)
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ severity: 'success', summary: '金币已调整' }),
    )
    wrapper.unmount()
  })

  it('bans/unbans with confirm; disables self and super target', async () => {
    auth.can.mockImplementation((code) => code === PERM.user.ban)
    auth.isSuperAdmin = false
    auth.user = { id: 1 }
    vi.mocked(listAdminUsers).mockResolvedValue({
      total: 4,
      items: [
        userItem({ id: 1, username: 'self', nickname: 'Self' }),
        userItem({
          id: 2,
          username: 'super',
          nickname: 'Super',
          roles: [{ id: 1, code: 'super_admin', name: '超级管理员' }],
        }),
        userItem({ id: 3, username: 'bob', nickname: 'Bob' }),
        userItem({ id: 4, username: 'carol', nickname: 'Carol', status: 'banned' }),
      ],
    })
    vi.mocked(banAdminUser).mockResolvedValue(null)
    vi.mocked(unbanAdminUser).mockResolvedValue(null)
    const wrapper = await mountPage()

    const banButtons = buttons(wrapper, '封禁')
    expect(banButtons).toHaveLength(3)
    expect((banButtons[0]!.element as HTMLButtonElement).disabled).toBe(true)
    expect((banButtons[1]!.element as HTMLButtonElement).disabled).toBe(true)
    expect((banButtons[2]!.element as HTMLButtonElement).disabled).toBe(false)

    await banButtons[2]!.trigger('click')
    const banOptions = confirm.require.mock.calls[0]![0] as {
      header?: string
      accept?: () => void
    }
    expect(banOptions.header).toBe('封禁用户')
    banOptions.accept?.()
    await flushPromises()
    expect(banAdminUser).toHaveBeenCalledWith(3)

    const unbanButtons = buttons(wrapper, '解封')
    expect(unbanButtons).toHaveLength(1)
    await unbanButtons[0]!.trigger('click')
    const unbanOptions = confirm.require.mock.calls[1]![0] as {
      header?: string
      accept?: () => void
    }
    expect(unbanOptions.header).toBe('解封用户')
    unbanOptions.accept?.()
    await flushPromises()
    expect(unbanAdminUser).toHaveBeenCalledWith(4)
    wrapper.unmount()
  })

  it('shows list load errors inline (forbidden)', async () => {
    auth.can.mockImplementation((code) => code === PERM.user.read)
    vi.mocked(listAdminUsers).mockRejectedValue(new ApiError(403, -1, '没有权限'))
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('没有权限')
    wrapper.unmount()
  })

  it('shows action API errors via toast (last super / self-ban)', async () => {
    auth.can.mockImplementation((code) => code === PERM.user.ban)
    auth.user = { id: 1 }
    vi.mocked(listAdminUsers).mockResolvedValue({
      total: 1,
      items: [userItem({ id: 2, username: 'bob', nickname: 'Bob' })],
    })
    vi.mocked(banAdminUser).mockRejectedValue(
      new ApiError(400, 40001, '不能封禁最后一个活跃的超级管理员'),
    )
    const wrapper = await mountPage()

    await buttons(wrapper, '封禁')[0]!.trigger('click')
    const options = confirm.require.mock.calls[0]![0] as { accept?: () => void }
    options.accept?.()
    await flushPromises()

    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'error',
        summary: '操作失败',
        detail: '不能封禁最后一个活跃的超级管理员',
      }),
    )
    wrapper.unmount()
  })
})
