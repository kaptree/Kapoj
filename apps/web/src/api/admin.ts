import { http } from './client'
import type { AdminStats } from './types'

/** Dashboard aggregate counters (admin.dashboard.view). */
export const getAdminStats = () => http.get<AdminStats>('/admin/stats')
