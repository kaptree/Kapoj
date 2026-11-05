import { http } from './client'
import type {
  AchievementInput,
  AchievementItem,
  AdminCoinAdjustInput,
  AdminCoinAdjustResult,
  BadgeInput,
  BadgeItem,
  CoinTransactionPage,
  GamificationConfig,
  TaskInput,
  TaskItem,
  TitleInput,
  TitleItem,
} from './types'

export interface IncludeInactiveOptions {
  includeInactive?: boolean
}

const includeInactive = (opts: IncludeInactiveOptions) => ({
  includeInactive: opts.includeInactive ? 'true' : undefined,
})

export const listAchievements = (opts: IncludeInactiveOptions = {}) =>
  http.get<AchievementItem[]>('/achievements', { params: includeInactive(opts) })

export const createAchievement = (input: AchievementInput) =>
  http.post<AchievementItem>('/achievements', input)

export const updateAchievement = (id: number | string, input: AchievementInput) =>
  http.put<AchievementItem>(`/achievements/${id}`, input)

export const deleteAchievement = (id: number | string) => http.delete<null>(`/achievements/${id}`)

export const listTasks = (opts: IncludeInactiveOptions = {}) =>
  http.get<TaskItem[]>('/tasks', { params: includeInactive(opts) })

export const claimTask = (id: number | string) => http.post<TaskItem>(`/tasks/${id}/claim`)

export const createTask = (input: TaskInput) => http.post<TaskItem>('/tasks', input)

export const updateTask = (id: number | string, input: TaskInput) =>
  http.put<TaskItem>(`/tasks/${id}`, input)

export const deleteTask = (id: number | string) => http.delete<null>(`/tasks/${id}`)

export const listBadges = (opts: IncludeInactiveOptions = {}) =>
  http.get<BadgeItem[]>('/badges', { params: includeInactive(opts) })

export const getGamificationConfig = () => http.get<GamificationConfig>('/gamification/config')

export const createBadge = (input: BadgeInput) => http.post<BadgeItem>('/badges', input)

export const updateBadge = (id: number | string, input: BadgeInput) =>
  http.put<BadgeItem>(`/badges/${id}`, input)

export const deleteBadge = (id: number | string) => http.delete<null>(`/badges/${id}`)

export const uploadBadgeImage = (id: number | string, file: File) => {
  const form = new FormData()
  form.append('file', file)
  return http.postForm<null>(`/badges/${id}/image`, form)
}

export const purchaseBadge = (id: number | string) => http.post<null>(`/badges/${id}/purchase`)

export const equipBadge = (id: number | string) => http.post<null>(`/badges/${id}/equip`)

export const unequipBadge = (id: number | string) => http.post<null>(`/badges/${id}/unequip`)

export const listUserBadges = (userId: number | string) =>
  http.get<BadgeItem[]>(`/users/${userId}/badges`)

export const listTitles = (opts: IncludeInactiveOptions = {}) =>
  http.get<TitleItem[]>('/titles', { params: includeInactive(opts) })

export const createTitle = (input: TitleInput) => http.post<TitleItem>('/titles', input)

export const updateTitle = (id: number | string, input: TitleInput) =>
  http.put<TitleItem>(`/titles/${id}`, input)

export const deleteTitle = (id: number | string) => http.delete<null>(`/titles/${id}`)

export const purchaseTitle = (id: number | string) => http.post<null>(`/titles/${id}/purchase`)

export const equipTitle = (id: number | string) => http.post<null>(`/titles/${id}/equip`)

export const unequipTitle = (id: number | string) => http.post<null>(`/titles/${id}/unequip`)

export const listCoinTransactions = (q: { page: number; pageSize: number }) =>
  http.get<CoinTransactionPage>('/coins/transactions', {
    params: { page: q.page, page_size: q.pageSize },
  })

export const adminAdjustCoins = (userId: number | string, input: AdminCoinAdjustInput) =>
  http.post<AdminCoinAdjustResult>(`/admin/users/${userId}/coins`, input)
