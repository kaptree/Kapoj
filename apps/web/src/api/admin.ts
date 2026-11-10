import { http } from './client'
import type {
  AdminCertificateView,
  AIKeyGenerateResult,
  AIKeyInput,
  AIKeyView,
  AdminAISettingsInput,
  AdminAISettingsView,
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
  CertificateReviewInput,
  CertificateTypeInput,
  CertificateTypeView,
  CertificateView,
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

/** Current AI feature switch and encryption key status (ai.settings.manage). */
export const getAdminAISettings = () => http.get<AdminAISettingsView>('/admin/ai/settings')

/** Update the AI feature switch; enabling requires a configured key (ai.settings.manage). */
export const updateAdminAISettings = (input: AdminAISettingsInput) =>
  http.put<AdminAISettingsView>('/admin/ai/settings', input)

/** Current AI encryption key status: source/masked/fingerprint, never the full key (ai.settings.manage). */
export const getAdminAIKey = () => http.get<AIKeyView>('/admin/ai/key')

/** Generate and persist a new AI encryption key; the full key is returned once for backup (ai.settings.manage). */
export const generateAdminAIKey = () => http.post<AIKeyGenerateResult>('/admin/ai/key/generate')

/** Delete all user AI configs and replace the platform key with a new random key (ai.settings.manage). */
export const resetAdminAIKey = () => http.post<AIKeyGenerateResult>('/admin/ai/key/reset')

/** Manually set the AI encryption key, rotating (and re-encrypting user keys) if one exists (ai.settings.manage). */
export const setAdminAIKey = (input: AIKeyInput) => http.put<AIKeyView>('/admin/ai/key', input)

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

export interface ListAdminCertificatesQuery {
  /** pending/approved/rejected/all; empty or pending both mean pending. */
  status?: 'pending' | 'approved' | 'rejected' | 'all' | ''
  page?: number
  pageSize?: number
}

/** Certificate review list (certificate.manage); defaults to pending. */
export const listAdminCertificates = (q: ListAdminCertificatesQuery = {}) =>
  http.get<Page<AdminCertificateView>>('/admin/certificates', {
    params: {
      status: q.status || undefined,
      page: q.page,
      page_size: q.pageSize,
    },
  })

/** Approve or reject a pending certificate (certificate.manage). */
export const reviewCertificate = (id: number | string, input: CertificateReviewInput) =>
  http.post<CertificateView>(`/admin/certificates/${id}/review`, input)

/** All certificate types, optionally including inactive (certificate.manage). */
export const listAdminCertificateTypes = (includeInactive = false) =>
  http.get<CertificateTypeView[]>('/admin/certificate-types', {
    params: { includeInactive: includeInactive ? 'true' : undefined },
  })

/** Create a certificate type (certificate.manage). */
export const createAdminCertificateType = (input: CertificateTypeInput) =>
  http.post<CertificateTypeView>('/admin/certificate-types', input)

/** Update a certificate type (certificate.manage). */
export const updateAdminCertificateType = (id: number | string, input: CertificateTypeInput) =>
  http.put<CertificateTypeView>(`/admin/certificate-types/${id}`, input)

/** Delete an unreferenced certificate type (certificate.manage). */
export const deleteAdminCertificateType = (id: number | string) =>
  http.delete<null>(`/admin/certificate-types/${id}`)
