import { flushPromises, mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import {
  createAdminRole,
  deleteAdminRole,
  listAdminPermissions,
  listAdminRoles,
  replaceAdminRolePermissions,
  updateAdminRole,
} from '@/api/admin'
import { ApiError } from '@/api/http'
import type { AdminPermissionItem, AdminRoleItem } from '@/api/types'
import { PERM } from '@/permissions'
import AdminRolesView from './AdminRolesView.vue'

const toast = vi.hoisted(() => ({ add: vi.fn() }))
const confirm = vi.hoisted(() => ({ require: vi.fn() }))
const auth = vi.hoisted(() => ({
  can: vi.fn<(code: string) => boolean>(() => false),
}))

vi.mock('primevue/usetoast', () => ({ useToast: () => toast }))
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => confirm }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/admin', () => ({
  listAdminPermissions: vi.fn(),
  listAdminRoles: vi.fn(),
  createAdminRole: vi.fn(),
  updateAdminRole: vi.fn(),
  deleteAdminRole: vi.fn(),
  replaceAdminRolePermissions: vi.fn(),
}))

function permissionItem(overrides: Partial<AdminPermissionItem> = {}): AdminPermissionItem {
  return {
    code: 'user.read',
    name: '查看用户',
    groupName: '用户与角色',
    description: '查看用户列表与详情',
    sortOrder: 1,
    ...overrides,
  }
}

function roleItem(overrides: Partial<AdminRoleItem> = {}): AdminRoleItem {
  return {
    id: 1,
    code: 'content_viewer',
    name: '内容查看员',
    description: '只读内容',
    isSystem: false,
    permissions: ['problem.view_hidden'],
    userCount: 3,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

/** A representative permission directory covering three groups, in API order. */
function directory(): AdminPermissionItem[] {
  return [
    permissionItem({
      code: 'admin.dashboard.view',
      name: '查看仪表盘',
      groupName: '管理后台',
      sortOrder: 1,
    }),
    permissionItem({
      code: 'user.read',
      name: '查看用户',
      groupName: '用户与角色',
      sortOrder: 2,
    }),
    permissionItem({
      code: 'role.read',
      name: '查看角色',
      groupName: '用户与角色',
      sortOrder: 3,
    }),
    permissionItem({
      code: 'problem.create',
      name: '创建题目',
      groupName: '题库',
      sortOrder: 4,
    }),
  ]
}

async function mountPage() {
  const wrapper = mount(AdminRolesView, { global: { plugins: [PrimeVue] } })
  await flushPromises()
  return wrapper
}

function buttons(wrapper: Awaited<ReturnType<typeof mountPage>>, label: string) {
  return wrapper.findAll('button').filter((item) => item.text().trim() === label)
}

function dialog(): HTMLElement {
  return document.querySelector('.p-dialog') as HTMLElement
}

function dialogButton(label: string): HTMLButtonElement {
  return Array.from(dialog().querySelectorAll('button')).find(
    (item) => item.textContent?.trim() === label,
  ) as HTMLButtonElement
}

function setField(element: Element, value: string) {
  const proto =
    element instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype
  Object.getOwnPropertyDescriptor(proto, 'value')!.set!.call(element, value)
  element.dispatchEvent(new Event('input', { bubbles: true }))
}

/** Toggles a permission checkbox via its native change event, then flushes a tick. */
async function toggle(element: Element) {
  element.dispatchEvent(new Event('change', { bubbles: true }))
  await nextTick()
}

function groupNames(): string[] {
  return Array.from(dialog().querySelectorAll('.permission-group__name')).map(
    (item) => item.textContent ?? '',
  )
}

beforeEach(() => {
  toast.add.mockReset()
  confirm.require.mockReset()

  vi.mocked(listAdminPermissions).mockReset()
  vi.mocked(listAdminRoles).mockReset()
  vi.mocked(createAdminRole).mockReset()
  vi.mocked(updateAdminRole).mockReset()
  vi.mocked(deleteAdminRole).mockReset()
  vi.mocked(replaceAdminRolePermissions).mockReset()

  vi.mocked(listAdminPermissions).mockResolvedValue([])
  vi.mocked(listAdminRoles).mockResolvedValue([])

  auth.can.mockReset()
  auth.can.mockReturnValue(false)

  document.body.innerHTML = ''
})

describe('AdminRolesView', () => {
  it('groups permissions by group and shows role user counts', async () => {
    auth.can.mockImplementation((code) => code === PERM.role.create)
    vi.mocked(listAdminRoles).mockResolvedValue([
      roleItem({ name: '内容查看员', userCount: 5, permissions: ['problem.view_hidden'] }),
    ])
    vi.mocked(listAdminPermissions).mockResolvedValue(directory())
    const wrapper = await mountPage()

    expect(listAdminRoles).toHaveBeenCalledTimes(1)
    expect(listAdminPermissions).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('内容查看员')
    expect(wrapper.text()).toContain('5')
    expect(wrapper.text()).toContain('1')

    await buttons(wrapper, '新建角色')[0]!.trigger('click')
    await flushPromises()

    expect(groupNames()).toEqual(['管理后台', '用户与角色', '题库'])
    expect(document.getElementById('perm-problem.create')).toBeTruthy()
    wrapper.unmount()
  })

  it('creates a custom role with selected permissions', async () => {
    auth.can.mockImplementation((code) => code === PERM.role.create)
    vi.mocked(listAdminPermissions).mockResolvedValue(directory())
    vi.mocked(createAdminRole).mockResolvedValue(roleItem({ id: 9 }))
    const wrapper = await mountPage()

    await buttons(wrapper, '新建角色')[0]!.trigger('click')
    await flushPromises()

    setField(dialog().querySelector('.code-input')!, 'content_editor')
    setField(dialog().querySelector('.name-input')!, '内容编辑员')
    setField(dialog().querySelector('.desc-input')!, '负责内容编辑')

    // Select the whole 题库 group via its select-all button.
    ;(dialog().querySelector('[data-select-all="题库"]') as HTMLButtonElement).click()
    await flushPromises()

    dialogButton('创建').click()
    await flushPromises()

    expect(createAdminRole).toHaveBeenCalledWith({
      code: 'content_editor',
      name: '内容编辑员',
      description: '负责内容编辑',
      permissionCodes: ['problem.create'],
    })
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }))
    wrapper.unmount()
  })

  it('edits custom role name/description and permissions', async () => {
    auth.can.mockImplementation(
      (code) => code === PERM.role.update || code === PERM.role.permissions.update,
    )
    vi.mocked(listAdminRoles).mockResolvedValue([
      roleItem({
        id: 7,
        code: 'content_editor',
        name: '内容编辑员',
        description: '旧描述',
        permissions: ['problem.create'],
        userCount: 2,
      }),
    ])
    vi.mocked(listAdminPermissions).mockResolvedValue(directory())
    vi.mocked(updateAdminRole).mockResolvedValue(roleItem())
    vi.mocked(replaceAdminRolePermissions).mockResolvedValue(roleItem())
    const wrapper = await mountPage()

    await buttons(wrapper, '编辑')[0]!.trigger('click')
    await flushPromises()

    setField(dialog().querySelector('.name-input')!, '内容维护员')
    setField(dialog().querySelector('.desc-input')!, '新描述')

    // Toggle problem.create off and role.read on.
    await toggle(document.getElementById('perm-problem.create')!)
    await toggle(document.getElementById('perm-role.read')!)
    await flushPromises()

    dialogButton('保存').click()
    await flushPromises()

    expect(updateAdminRole).toHaveBeenCalledWith(7, {
      name: '内容维护员',
      description: '新描述',
    })
    expect(replaceAdminRolePermissions).toHaveBeenCalledWith(7, ['role.read'])
    expect(listAdminRoles).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('protects system role code/name and super_admin permissions', async () => {
    auth.can.mockImplementation(
      (code) => code === PERM.role.update || code === PERM.role.permissions.update,
    )
    vi.mocked(listAdminRoles).mockResolvedValue([
      roleItem({ id: 1, code: 'super_admin', name: '超级管理员', isSystem: true }),
      roleItem({
        id: 2,
        code: 'content_admin',
        name: '内容管理员',
        isSystem: true,
        permissions: ['problem.create'],
        userCount: 1,
      }),
    ])
    vi.mocked(listAdminPermissions).mockResolvedValue(directory())
    const wrapper = await mountPage()

    // No delete button for either system role.
    expect(buttons(wrapper, '删除')).toHaveLength(0)
    // No permission button for super_admin (immutable).
    expect(buttons(wrapper, '权限')).toHaveLength(1)

    await buttons(wrapper, '编辑')[0]!.trigger('click')
    await flushPromises()

    // super_admin: name disabled and bypass note shown, no matrix checkboxes.
    expect((dialog().querySelector('.name-input') as HTMLInputElement).disabled).toBe(true)
    expect((dialog().querySelector('.code-input') as HTMLInputElement).disabled).toBe(true)
    expect(dialog().textContent).toContain('绕过全部权限检查')
    expect(dialog().querySelector('.permission-group__name')).toBeFalsy()
    wrapper.unmount()
  })

  it('deletes only custom roles', async () => {
    auth.can.mockImplementation((code) => code === PERM.role.delete)
    vi.mocked(listAdminRoles).mockResolvedValue([
      roleItem({ id: 1, code: 'super_admin', name: '超级管理员', isSystem: true }),
      roleItem({ id: 8, code: 'temp', name: '临时角色', isSystem: false }),
    ])
    vi.mocked(deleteAdminRole).mockResolvedValue(null)
    const wrapper = await mountPage()

    // Only the custom role exposes a delete button.
    const deleteButtons = buttons(wrapper, '删除')
    expect(deleteButtons).toHaveLength(1)

    await deleteButtons[0]!.trigger('click')
    const options = confirm.require.mock.calls[0]![0] as { header?: string; accept?: () => void }
    expect(options.header).toBe('删除角色')
    options.accept?.()
    await flushPromises()

    expect(deleteAdminRole).toHaveBeenCalledWith(8)
    expect(listAdminRoles).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('read-only user sees no mutation buttons', async () => {
    auth.can.mockImplementation((code) => code === PERM.role.read)
    vi.mocked(listAdminRoles).mockResolvedValue([
      roleItem({ id: 1, code: 'super_admin', name: '超级管理员', isSystem: true }),
      roleItem({ id: 8, code: 'temp', name: '临时角色', isSystem: false }),
    ])
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain('超级管理员')
    expect(wrapper.text()).toContain('临时角色')
    for (const label of ['新建角色', '编辑', '权限', '删除']) {
      expect(buttons(wrapper, label)).toHaveLength(0)
    }
    wrapper.unmount()
  })

  it('shows backend errors such as duplicate code', async () => {
    auth.can.mockImplementation((code) => code === PERM.role.create)
    vi.mocked(listAdminPermissions).mockResolvedValue(directory())
    vi.mocked(createAdminRole).mockRejectedValue(new ApiError(400, 40001, '角色 code 已存在'))
    const wrapper = await mountPage()

    await buttons(wrapper, '新建角色')[0]!.trigger('click')
    await flushPromises()

    setField(dialog().querySelector('.code-input')!, 'content_editor')
    setField(dialog().querySelector('.name-input')!, '内容编辑员')
    dialogButton('创建').click()
    await flushPromises()

    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'error',
        summary: '操作失败',
        detail: '角色 code 已存在',
      }),
    )
    wrapper.unmount()
  })

  it('shows a distinct error when metadata saves but permissions fail', async () => {
    auth.can.mockImplementation(
      (code) => code === PERM.role.update || code === PERM.role.permissions.update,
    )
    vi.mocked(listAdminRoles).mockResolvedValue([
      roleItem({
        id: 7,
        code: 'content_editor',
        name: '内容编辑员',
        description: '旧描述',
        permissions: ['problem.create'],
        userCount: 2,
      }),
    ])
    vi.mocked(listAdminPermissions).mockResolvedValue(directory())
    vi.mocked(updateAdminRole).mockResolvedValue(roleItem())
    vi.mocked(replaceAdminRolePermissions).mockRejectedValue(
      new ApiError(500, 50000, '权限写入失败'),
    )
    const wrapper = await mountPage()

    await buttons(wrapper, '编辑')[0]!.trigger('click')
    await flushPromises()

    setField(dialog().querySelector('.name-input')!, '内容维护员')
    setField(dialog().querySelector('.desc-input')!, '新描述')
    await toggle(document.getElementById('perm-problem.create')!)
    await flushPromises()

    dialogButton('保存').click()
    await flushPromises()

    expect(updateAdminRole).toHaveBeenCalledWith(7, {
      name: '内容维护员',
      description: '新描述',
    })
    expect(replaceAdminRolePermissions).toHaveBeenCalledWith(7, [])
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({
        severity: 'error',
        summary: '角色信息已保存，权限保存失败',
        detail: '权限写入失败',
      }),
    )
    // The list was refreshed so the metadata stays current.
    expect(listAdminRoles).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('does not fire a success toast when nothing changed', async () => {
    auth.can.mockImplementation(
      (code) => code === PERM.role.update || code === PERM.role.permissions.update,
    )
    vi.mocked(listAdminRoles).mockResolvedValue([
      roleItem({
        id: 7,
        code: 'content_editor',
        name: '内容编辑员',
        description: '旧描述',
        permissions: ['problem.create'],
        userCount: 2,
      }),
    ])
    vi.mocked(listAdminPermissions).mockResolvedValue(directory())
    const wrapper = await mountPage()

    await buttons(wrapper, '编辑')[0]!.trigger('click')
    await flushPromises()

    dialogButton('保存').click()
    await flushPromises()

    expect(updateAdminRole).not.toHaveBeenCalled()
    expect(replaceAdminRolePermissions).not.toHaveBeenCalled()
    expect(toast.add).toHaveBeenCalledWith(expect.objectContaining({ summary: '无变更' }))
    expect(toast.add).not.toHaveBeenCalledWith(expect.objectContaining({ summary: '角色已更新' }))
    wrapper.unmount()
  })
})
