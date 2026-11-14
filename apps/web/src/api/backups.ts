import { http } from './client'
import type {
  BackupCreateAccountRequest,
  BackupOverview,
  BackupPackage,
  BackupRestoreJob,
  BackupSchedule,
  BackupScheduleInput,
} from './types'

export const getBackupOverview = () => http.get<BackupOverview>('/admin/backups')

export const createPlatformBackup = () => http.post<BackupPackage>('/admin/backups/platform')

export const createAccountBackup = (input: BackupCreateAccountRequest = {}) =>
  http.post<BackupPackage>('/admin/backups/accounts', input)

export const updateBackupSchedule = (kind: 'platform' | 'accounts', input: BackupScheduleInput) =>
  http.put<BackupSchedule>(`/admin/backups/schedules/${kind}`, input)

export const downloadBackup = (id: number | string) => http.getBlob(`/admin/backups/${id}/download`)

export interface RestoreRequest {
  packageId?: number
  userId?: number
  confirm?: string
}

export const restorePlatformBackup = (input: RestoreRequest, file?: File) => {
  if (file) {
    const form = new FormData()
    form.append('file', file)
    form.append('confirm', input.confirm ?? '')
    return http.postForm<BackupRestoreJob>('/admin/backups/restore/platform', form)
  }
  return http.post<BackupRestoreJob>('/admin/backups/restore/platform', {
    packageId: input.packageId,
    confirm: input.confirm,
  })
}

export const restoreAccountBackup = (input: RestoreRequest, file?: File) => {
  if (file) {
    const form = new FormData()
    form.append('file', file)
    form.append('userId', String(input.userId))
    return http.postForm<BackupRestoreJob>('/admin/backups/restore/account', form)
  }
  return http.post<BackupRestoreJob>('/admin/backups/restore/account', input)
}

export const getRestoreJob = (id: number | string) =>
  http.get<BackupRestoreJob>(`/admin/backups/restores/${id}`)

export const restartServer = () => http.post<{ restarting: boolean }>('/admin/backups/restart')
