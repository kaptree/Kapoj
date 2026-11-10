import { describe, expect, it } from 'vitest'

import { router } from './index'

describe('router', () => {
  it('lazy-loads the admin layout so it stays out of the initial bundle', () => {
    const adminRoute = router.getRoutes().find((route) => route.path === '/admin')

    expect(adminRoute).toBeDefined()
    expect(typeof adminRoute?.components?.default).toBe('function')
  })
})
