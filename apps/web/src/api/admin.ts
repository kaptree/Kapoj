import { http } from './client'
import type {
  AdminAuditItem,
  AdminCoinAdjustInput,
  AdminCoinAdjustResult,
  AdminPermissionItem,
  AdminRoleInput,
  AdminRoleItem,
  AdminRolePermissionsInput,
  AdminRoleUpdateInput,
  AdminStats,
  AdminUserBanInput,
  AdminUserDetail,
  AdminUserItem,
  AdminUserRolesInput,
  AdminUserUpdateInput,
  Page,
  RatingSettingsInput,
  RatingSettingsView,
  Status,
} from './types'

/** Dashboard aggregate counters (admin.dashboard.view). */
export const getAdminStats = () => http.get<AdminStats>('/admin/stats')

/** Current rating settings together with the active season (rating.settings.manage). */
export const getAdminRatingSettings = () => http.get<RatingSettingsView>('/admin/rating/settings')

/** Replace rating settings; returns the updated view with the new season (rating.settings.manage). */
export const updateAdminRatingSettings = (input: RatingSettingsInput) =>
  http.put<RatingSettingsView>('/admin/rating/settings', input)

export interface ListAdminUsersQuery {
  keyword?: string
  status?: Status
  roleId?: number
  page?: number
  pageSize?: number
}

/** User list (user.read). */
export const listAdminUsers = (q: ListAdminUsersQuery) =>
  http.get<Page<AdminUserItem>>('/admin/users', {
    params: {
      keyword: q.keyword || undefined,
      status: q.status || undefined,
      roleId: q.roleId ?? undefined,
      page: q.page,
      page_size: q.pageSize,
    },
  })

/** User detail, roles and super status (user.read). */
export const getAdminUser = (id: number | string) => http.get<AdminUserDetail>(`/admin/users/${id}`)

/** Edit nickname/bio/avatarUrl (user.update). */
export const updateAdminUser = (id: number | string, input: AdminUserUpdateInput) =>
  http.put<AdminUserDetail>(`/admin/users/${id}`, input)

/** Replace the user's roles with the given id set (user.roles.assign). */
export const replaceAdminUserRoles = (id: number | string, roleIds: number[]) =>
  http.put<AdminUserDetail>(`/admin/users/${id}/roles`, { roleIds } satisfies AdminUserRolesInput)

/** Ban a user; reason is written to the audit summary when present (user.ban). */
export const banAdminUser = (id: number | string, reason?: string) =>
  http.post<null>(`/admin/users/${id}/ban`, { reason } satisfies AdminUserBanInput)

/** Unban a user (user.ban). */
export const unbanAdminUser = (id: number | string) => http.post<null>(`/admin/users/${id}/unban`)

/** Adjust a user's coin balance (user.coins.adjust). */
export const adjustAdminUserCoins = (id: number | string, input: AdminCoinAdjustInput) =>
  http.post<AdminCoinAdjustResult>(`/admin/users/${id}/coins`, input)

/** Role list used for user role assignment and filtering (role.read). */
export const listAdminRoles = () => http.get<AdminRoleItem[]>('/admin/roles')

/** Permission directory, grouped and sorted by the server (role.read). */
export const listAdminPermissions = () => http.get<AdminPermissionItem[]>('/admin/permissions')

/** Create a custom role together with its permission set (role.create). */
export const createAdminRole = (input: AdminRoleInput) =>
  http.post<AdminRoleItem>('/admin/roles', input)

/** Update a role's name/description; code is immutable (role.update). */
export const updateAdminRole = (id: number | string, input: AdminRoleUpdateInput) =>
  http.put<AdminRoleItem>(`/admin/roles/${id}`, input)

/** Delete a custom role (role.delete). */
export const deleteAdminRole = (id: number | string) => http.delete<null>(`/admin/roles/${id}`)

/** Replace a role's full permission set (role.permissions.update). */
export const replaceAdminRolePermissions = (id: number | string, permissionCodes: string[]) =>
  http.put<AdminRoleItem>(`/admin/roles/${id}/permissions`, {
    permissionCodes,
  } satisfies AdminRolePermissionsInput)

export interface ListAdminAuditQuery {
  actorId?: number
  action?: string
  targetType?: string
  from?: string
  to?: string
  page?: number
  pageSize?: number
}

/** Audit log page (admin.audit.view). */
export const listAdminAudit = (q: ListAdminAuditQuery) =>
  http.get<Page<AdminAuditItem>>('/admin/audit', {
    params: {
      actorId: q.actorId ?? undefined,
      action: q.action || undefined,
      targetType: q.targetType || undefined,
      from: q.from || undefined,
      to: q.to || undefined,
      page: q.page,
      page_size: q.pageSize,
    },
  })
