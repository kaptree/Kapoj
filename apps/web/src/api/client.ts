import { createHttp } from './http'
import { useAuthStore } from '@/stores/auth'
import { router } from '@/router'

// The store and router are looked up per call, so this module can be imported
// before Pinia is installed.
export const http = createHttp({
  getAccessToken: () => useAuthStore().accessToken,
  getRefreshToken: () => useAuthStore().refreshToken,
  setTokens: (pair) => useAuthStore().setTokens(pair),
  onAuthFailure: () => {
    useAuthStore().clear()
    const current = router.currentRoute.value
    if (current.meta.requiresAuth) {
      void router.push({ name: 'login', query: { redirect: current.fullPath } })
    }
  },
})
