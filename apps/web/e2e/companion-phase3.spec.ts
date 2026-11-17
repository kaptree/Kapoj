import { expect, test, type APIRequestContext, type APIResponse, type Page } from '@playwright/test'

const apiURL = process.env.E2E_API_URL ?? 'http://127.0.0.1:18080'
const baseURL = process.env.E2E_BASE_URL ?? 'http://127.0.0.1:5173'
const limitedUser = process.env.E2E_LIMITED_USER ?? 'm2bsolver1'
const limitedPassword = process.env.E2E_LIMITED_PASSWORD ?? 'correct-horse'

async function login(page: Page, account: string, password: string) {
  await page.goto('/login')
  await page.getByLabel('用户名或邮箱').fill(account)
  await page.getByLabel('密码').fill(password)
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL('/')
}

async function apiData<T>(res: APIResponse): Promise<T> {
  const body = (await res.json()) as { code: number; message: string; data: T }
  if (!res.ok()) throw new Error(`API ${res.url()} failed: ${body.code} ${body.message}`)
  return body.data
}

async function apiToken(ctx: APIRequestContext): Promise<string> {
  const session = await apiData<{ accessToken: string }>(
    await ctx.post('/api/auth/login', {
      data: { account: limitedUser, password: limitedPassword },
    }),
  )
  return session.accessToken
}

async function setPublicProfile(
  ctx: APIRequestContext,
  headers: Record<string, string>,
  publicProfile: boolean,
): Promise<void> {
  await apiData(
    await ctx.post('/api/companion/settings', {
      headers,
      data: { publicProfile },
    }),
  )
}

test.describe('companion phase 3', () => {
  test('profile card is visible to self and to guests only when public', async ({
    page,
    browser,
    playwright,
  }) => {
    const api = await playwright.request.newContext({ baseURL: apiURL })
    let headers: Record<string, string> | null = null
    try {
      const token = await apiToken(api)
      headers = { Authorization: `Bearer ${token}` }
      const me = await apiData<{ id: number }>(await api.get('/api/users/me', { headers }))
      expect(me.id).toBeGreaterThan(0)

      // Pin the shared account to the private baseline so the self check really
      // exercises the owner branch (visible even when publicProfile is false).
      await setPublicProfile(api, headers, false)

      await login(page, limitedUser, limitedPassword)
      await page.goto(`/users/${me.id}`)

      // The card root is `.companion-profile-card` (the plan's `.companion-profile`).
      const card = page.locator('[data-test="companion-profile-card"]')
      await expect(card).toBeVisible()
      await expect(card.locator('.companion-profile-card__private')).toBeVisible()

      // Phase 3 must not replace the existing community word cloud.
      await expect(page.locator('[data-test="profile-word-cloud"]')).toBeVisible()

      await setPublicProfile(api, headers, true)

      // A brand-new context has no session, so this is the guest/public branch.
      const guest = await browser.newContext()
      try {
        const guestPage = await guest.newPage()
        await guestPage.goto(`${baseURL}/users/${me.id}`)
        const guestCard = guestPage.locator('[data-test="companion-profile-card"]')
        await expect(guestCard).toBeVisible()
        await expect(guestCard.locator('.companion-profile-card__private')).toHaveCount(0)
      } finally {
        await guest.close()
      }
    } finally {
      // Never leave the shared account public, even when an assertion fails.
      if (headers) {
        try {
          await setPublicProfile(api, headers, false)
        } catch {
          // Best effort: do not mask the real failure.
        }
      }
      await api.dispose()
    }
  })
})
