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
  AdminUserImportResult,
  AdminUserItem,
  AdminUserRolesInput,
  AdminUserUpdateInput,
  CertificateReviewInput,
  CertificateTypeInput,
  CertificateTypeView,
  CertificateView,
  DifficultySettingsInput,
  DifficultySettingsView,
  JobInput,
  JobRunView,
  JobView,
  Page,
  PlatformModelInput,
  PlatformModelTestResult,
  PlatformModelView,
  ProblemImportSettings,
  ProblemImportSettingsInput,
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

/** All platform-level AI models with masked tokens (ai.settings.manage). */
export const listAdminAIPlatformModels = () => http.get<PlatformModelView[]>('/admin/ai/models')

/** Create a platform model; apiKey is required and never returned (ai.settings.manage). */
export const createAdminAIPlatformModel = (input: PlatformModelInput) =>
  http.post<PlatformModelView>('/admin/ai/models', input)

/** Update a platform model; an empty/omitted apiKey keeps the stored token (ai.settings.manage). */
export const updateAdminAIPlatformModel = (id: number | string, input: PlatformModelInput) =>
  http.put<PlatformModelView>(`/admin/ai/models/${id}`, input)

/** Delete an unselected platform model (ai.settings.manage). */
export const deleteAdminAIPlatformModel = (id: number | string) =>
  http.delete<null>(`/admin/ai/models/${id}`)

/** Send one lightweight request to the model and persist the masked result (ai.settings.manage). */
export const testAdminAIPlatformModel = (id: number | string) =>
  http.post<PlatformModelTestResult>(`/admin/ai/models/${id}/test`)

/** Select the current platform model; null clears the selection (ai.settings.manage). */
export const selectAdminAIPlatformModel = (modelId: number | null) =>
  http.put<AdminAISettingsView>('/admin/ai/platform-model', { modelId })

/** Scheduled jobs with their latest state and active run (system.jobs.manage). */
export const listAdminJobs = () => http.get<JobView[]>('/admin/jobs')

/** Replace a job's enabled flag, cron expression and timezone (system.jobs.manage). */
export const updateAdminJob = (key: string, input: JobInput) =>
  http.put<JobView>(`/admin/jobs/${encodeURIComponent(key)}`, input)

/** Enqueue a manual run; responds 202 with the queued run (system.jobs.manage). */
export const runAdminJob = (key: string) =>
  http.post<JobRunView>(`/admin/jobs/${encodeURIComponent(key)}/run`)

export interface ListAdminJobRunsQuery {
  jobKey?: string
  status?: string
  page?: number
  pageSize?: number
}

/** All job runs, newest first, optionally filtered by job and status (system.jobs.manage). */
export const listAdminJobRuns = (q: ListAdminJobRunsQuery = {}) =>
  http.get<Page<JobRunView>>('/admin/job-runs', {
    params: {
      jobKey: q.jobKey || undefined,
      status: q.status || undefined,
      page: q.page,
      page_size: q.pageSize,
    },
  })

/** One run's detail and live progress (system.jobs.manage). */
export const getAdminJobRun = (id: number | string) => http.get<JobRunView>(`/admin/job-runs/${id}`)

/** Difficulty formula settings (system.jobs.manage). */
export const getAdminDifficultySettings = () =>
  http.get<DifficultySettingsView>('/admin/difficulty/settings')

/** Replace all difficulty formula settings (system.jobs.manage). */
export const updateAdminDifficultySettings = (input: DifficultySettingsInput) =>
  http.put<DifficultySettingsView>('/admin/difficulty/settings', input)

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

function importForm(file: File): FormData {
  const form = new FormData()
  form.append('file', file)
  return form
}

/** Parse and preview a HydroOJ user CSV without writing (user.import). */
export const previewAdminUsersImport = (file: File) =>
  http.postForm<AdminUserImportResult>('/admin/users/import?dryRun=true', importForm(file))

/** Import the valid rows from a HydroOJ user CSV (user.import). */
export const importAdminUsers = (file: File) =>
  http.postForm<AdminUserImportResult>('/admin/users/import', importForm(file))

/** Current problem package import size settings (site.settings.manage). */
export const getAdminProblemImportSettings = () =>
  http.get<ProblemImportSettings>('/admin/problems/import/settings')

/** Update the problem package import size limit (site.settings.manage). */
export const updateAdminProblemImportSettings = (input: ProblemImportSettingsInput) =>
  http.put<ProblemImportSettings>('/admin/problems/import/settings', input)
