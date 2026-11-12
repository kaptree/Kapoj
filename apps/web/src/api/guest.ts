import { http } from './client'
import type { GuestPermissions } from './types'

/** Public permissions granted to the built-in guest role. */
export const getGuestPermissions = () => http.get<GuestPermissions>('/guest/permissions')
