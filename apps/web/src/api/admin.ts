import { http } from './client'
import type {
  AdminCoinAdjustInput,
  AdminCoinAdjustResult,
  AdminRoleItem,
  AdminStats,
  AdminUserBanInput,
  AdminUserDetail,
  AdminUserItem,
  AdminUserRolesInput,
  AdminUserUpdateInput,
  Page,
  Status,
} from './types'

/** Dashboard aggregate counters (admin.dashboard.view). */
export const getAdminStats = () => http.get<AdminStats>('/admin/stats')

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
