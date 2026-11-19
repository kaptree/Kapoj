import { expect, test, type Page, type Route } from '@playwright/test'

const limitedUser = process.env.E2E_LIMITED_USER ?? 'm2bsolver1'
const limitedPassword = process.env.E2E_LIMITED_PASSWORD ?? 'correct-horse'

async function login(page: Page, account: string, password: string) {
  await page.goto('/login')
  await page.getByLabel('用户名或邮箱').fill(account)
  await page.getByLabel('密码').fill(password)
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL('/')
}

const fullState = {
  bond: 400,
  level: 6,
  levelName: '只对你',
  nextThreshold: 380,
  dailyGain: 0,
  dailyCap: 15,
  activityLevel: 'normal',
  dockEnabled: true,
  publicProfile: false,
  equippedSkin: 'outfit-1',
  unlockedSkins: ['outfit-1', 'outfit-2', 'outfit-3', 'outfit-4', 'outfit-5'],
  personalityPrompt: '',
  aiDialogueEnabled: true,
  aiReady: true,
  aiRemainingToday: 29,
  unreadCount: 0,
  todayEvents: [],
}

async function mockCompanionState(page: Page, overrides: Record<string, unknown> = {}) {
  await page.route('**/api/companion', async (route: Route) => {
    if (route.request().method() !== 'GET') return route.continue()
    await route.fulfill({
      json: {
        code: 0,
        message: 'ok',
        data: { ...fullState, ...overrides },
      },
    })
  })
}

async function mockSettings(page: Page) {
  await page.route('**/api/companion/settings', async (route: Route) => {
    const patch = route.request().postDataJSON() as Record<string, unknown>
    await route.fulfill({
      json: { code: 0, message: 'ok', data: { ...fullState, ...patch } },
    })
  })
}

test.describe('companion phase 4', () => {
  test('dock pets with an expression and opens the settings link', async ({ page }) => {
    await login(page, limitedUser, limitedPassword)

    const character = page.locator('.companion-dock__character')
    await expect(character).toBeVisible()
    await character.click()

    await expect(page.locator('.companion-q-avatar')).not.toHaveAttribute(
      'data-expression',
      'neutral',
    )
    await expect(page.locator('.companion-dock__floating-bubble')).toBeVisible()

    await page.locator('.companion-dock__menu').click()
    await expect(page.locator('.companion-dock__panel')).toBeVisible()
    await expect(page.getByRole('link', { name: '伙伴设置' })).toBeVisible()
  })

  test('settings show personality and wardrobe controls', async ({ page }) => {
    await login(page, limitedUser, limitedPassword)
    await page.goto('/appearance')

    await expect(page.locator('.companion-personality')).toBeVisible()
    await expect(page.locator('[data-test="personality-prompt"]')).toBeVisible()
    await expect(page.locator('.companion-wardrobe')).toBeVisible()
    await expect(page.locator('[data-test="wardrobe-outfit-outfit-1"]')).toBeVisible()
  })

  test('AI dialogue line replaces the thinking bubble', async ({ page }) => {
    await mockCompanionState(page, { aiReady: true })
    await page.route('**/api/companion/dialogue', async (route: Route) => {
      await route.fulfill({
        json: {
          code: 0,
          message: 'ok',
          data: { source: 'ai', line: 'E2E AI 台词', remainingToday: 28 },
        },
      })
    })
    await login(page, limitedUser, limitedPassword)

    await page.locator('.companion-dock__character').click()
    await expect(page.locator('.companion-dock__floating-bubble')).toContainText('E2E AI 台词')
  })

  test('a failed AI request falls back to a static pet line without an error toast', async ({
    page,
  }) => {
    await mockCompanionState(page, { aiReady: true })
    await page.route('**/api/companion/dialogue', async (route: Route) => {
      await route.fulfill({
        status: 500,
        json: { code: 50000, message: 'upstream down', data: null },
      })
    })
    await login(page, limitedUser, limitedPassword)

    await page.locator('.companion-dock__character').click()
    await expect(page.locator('.companion-dock__floating-bubble')).toBeVisible()
    await expect(page.locator('.companion-dock__floating-bubble')).not.toHaveText('……')
    await expect(page.locator('.companion-dock__error')).toHaveCount(0)
  })

  test('wardrobe equips an unlocked outfit', async ({ page }) => {
    await mockCompanionState(page, { equippedSkin: 'outfit-1' })
    await mockSettings(page)
    await login(page, limitedUser, limitedPassword)
    await page.goto('/appearance')

    const outfit = page.locator('[data-test="wardrobe-outfit-outfit-3"]')
    await expect(outfit).toBeEnabled()
    await outfit.click()
    await expect(outfit).toHaveAttribute('aria-current', 'true')
    await expect(outfit).toContainText('已穿戴')
  })

  test('running contest hides the dock', async ({ page }) => {
    await page.route('**/api/contests/7', async (route: Route) => {
      await route.fulfill({
        json: { code: 0, message: 'ok', data: { id: 7, status: 'running' } },
      })
    })
    await login(page, limitedUser, limitedPassword)
    await page.goto('/contests/7')
    await expect(page.locator('.companion-dock')).toHaveCount(0)
  })
})
