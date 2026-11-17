import {
  expect,
  test,
  type APIRequestContext,
  type APIResponse,
  type Locator,
  type Page,
} from '@playwright/test'

const apiURL = process.env.E2E_API_URL ?? 'http://127.0.0.1:18080'
const limitedUser = process.env.E2E_LIMITED_USER ?? 'm2bsolver1'
const limitedPassword = process.env.E2E_LIMITED_PASSWORD ?? 'correct-horse'
const adminUser = process.env.E2E_ADMIN_USER ?? 'm2badmin'
const adminPassword = process.env.E2E_ADMIN_PASSWORD ?? 'correct-horse'

const DAILY_PET_LIMIT = '今天已经摸过了。……明天再来。'

async function login(page: Page, account: string, password: string) {
  await page.goto('/login')
  await page.getByLabel('用户名或邮箱').fill(account)
  await page.getByLabel('密码').fill(password)
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL('/')
}

/** Wait for the settings PATCH before reloading; otherwise reload cancels it. */
async function chooseActivity(page: Page, control: Locator): Promise<void> {
  const saved = page.waitForResponse(
    (res) =>
      res.url().includes('/api/companion/settings') &&
      res.request().method() === 'POST' &&
      res.ok(),
  )
  await control.click()
  await saved
}

async function apiData<T>(res: APIResponse): Promise<T> {
  const body = (await res.json()) as { code: number; message: string; data: T }
  if (!res.ok()) throw new Error(`API ${res.url()} failed: ${body.code} ${body.message}`)
  return body.data
}

/**
 * The dock hides the bond meter at bond 0, and a brand new account starts
 * there. Petting once is the cheapest deterministic way to give the meter a
 * value without touching any other state; accounts that already have bond are
 * left alone.
 */
async function ensureBond(ctx: APIRequestContext): Promise<void> {
  const session = await apiData<{ accessToken: string }>(
    await ctx.post('/api/auth/login', {
      data: { account: limitedUser, password: limitedPassword },
    }),
  )
  const headers = { Authorization: `Bearer ${session.accessToken}` }
  const state = await apiData<{ bond: number }>(await ctx.get('/api/companion', { headers }))
  if (state.bond > 0) return
  await apiData(await ctx.post('/api/companion/pet', { headers }))
}

test.describe('companion phase 2', () => {
  test('dock shows bond progress and reacts to petting', async ({ page, playwright }) => {
    const api = await playwright.request.newContext({ baseURL: apiURL })
    try {
      await ensureBond(api)
    } finally {
      await api.dispose()
    }

    await login(page, limitedUser, limitedPassword)
    await expect(page.locator('.companion-dock__toggle')).toBeVisible()
    await page.locator('.companion-dock__toggle').click()
    await expect(page.locator('.companion-dock__panel')).toBeVisible()

    const meter = page.locator('[data-test="companion-bond"]')
    const meterText = meter.locator('.companion-bond__text')
    const bubble = page.locator('.companion-dock__panel .companion-bubble')

    await expect(meter).toBeVisible()
    await expect(meterText).toHaveText(/^Lv\.\d+ \S+ \d+\/\d+$/)
    await expect(meter.getByRole('progressbar', { name: /羁绊$/ })).toHaveAttribute(
      'aria-valuenow',
      /^\d+$/,
    )
    await expect(meter.locator('.companion-bond__daily')).toHaveText(/^今日 \d+\/\d+$/)

    const bubbleBefore = ((await bubble.textContent()) ?? '').trim()
    const bondBefore = ((await meterText.textContent()) ?? '').trim()

    await page.locator('.companion-dock__pet').click()

    // Petting is idempotent per day. The reply is either a fresh pet line, the
    // fixed daily-limit line, or an unchanged line plus a higher bond meter.
    await expect
      .poll(
        async () => {
          const bubbleAfter = ((await bubble.textContent()) ?? '').trim()
          const bondAfter = ((await meterText.textContent()) ?? '').trim()
          return (
            bubbleAfter === DAILY_PET_LIMIT ||
            bubbleAfter !== bubbleBefore ||
            bondAfter !== bondBefore
          )
        },
        { message: 'petting should change the bubble or the bond meter' },
      )
      .toBe(true)

    await expect(page.locator('.companion-dock__error')).toHaveCount(0)
    await expect(meter).toBeVisible()
    await expect(meterText).toHaveText(/^Lv\.\d+ \S+ \d+\/\d+$/)
  })

  test('quiet activity preference persists after reload', async ({ page }) => {
    await login(page, limitedUser, limitedPassword)
    await page.goto('/appearance')

    const settings = page.locator('.companion-settings')
    await expect(settings).toBeVisible()

    const quiet = settings.locator('[data-test="activity-quiet"]')
    const normal = settings.locator('[data-test="activity-normal"]')

    try {
      // Self-healing baseline: clears quiet left behind by an interrupted run,
      // so this test never depends on the previous run's final state.
      await chooseActivity(page, normal)
      await expect(normal).toHaveAttribute('aria-pressed', 'true')

      await chooseActivity(page, quiet)
      await expect(quiet).toHaveAttribute('aria-pressed', 'true')

      await page.reload()
      await expect(settings).toBeVisible()
      await expect(quiet).toHaveAttribute('aria-pressed', 'true')
    } finally {
      // Always leave the shared test account on normal, even when an assertion
      // above fails. Keep this best-effort so it cannot mask the real failure.
      try {
        await chooseActivity(page, normal)
        await expect(normal).toHaveAttribute('aria-pressed', 'true', { timeout: 2_000 })
      } catch {
        // The page may already be closing or navigating; nothing to restore.
      }
    }
  })

  test('admin site page shows the companion settings card', async ({ page }) => {
    await login(page, adminUser, adminPassword)
    await page.goto('/admin/site')
    await expect(page.getByRole('heading', { name: '站点设置' })).toBeVisible()

    await page.getByRole('tab', { name: '伙伴' }).click()
    const form = page.locator('[data-test="companion-form"]')
    await expect(form).toBeVisible()
    await expect(page.getByRole('heading', { name: '伙伴设置' })).toBeVisible()
    await expect(page.locator('[data-test="companion-name"]')).toBeVisible()
    await expect(page.locator('[data-test="companion-enabled"]')).toBeVisible()
  })
})
