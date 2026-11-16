import { expect, test, type Page } from '@playwright/test'

const limitedUser = process.env.E2E_LIMITED_USER ?? 'm2bsolver1'
const limitedPassword = process.env.E2E_LIMITED_PASSWORD ?? 'correct-horse'
const adminUser = process.env.E2E_ADMIN_USER ?? 'm2badmin'
const adminPassword = process.env.E2E_ADMIN_PASSWORD ?? 'correct-horse'

async function login(page: Page, account: string, password: string) {
  await page.goto('/login')
  await page.getByLabel('用户名或邮箱').fill(account)
  await page.getByLabel('密码').fill(password)
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL('/')
}

test.describe('companion phase 0-1', () => {
  test('login shows static art and no dock', async ({ page }) => {
    await page.goto('/login')
    await expect(page.locator('.auth-intro__companion')).toBeVisible()
    await expect(page.locator('.companion-dock')).toHaveCount(0)
  })

  test('logged-in home and problems show the dock and a note', async ({ page }) => {
    await login(page, limitedUser, limitedPassword)
    await expect(page.locator('.companion-dock__toggle')).toBeVisible()
    await page.locator('.companion-dock__toggle').click()
    await expect(page.locator('.companion-dock__panel')).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(page.locator('.companion-dock__panel')).toHaveCount(0)
    await page.goto('/problems')
    await expect(page.locator('.companion-note')).toBeVisible()
    await page.getByRole('button', { name: '关闭伙伴提示' }).click()
    await page.reload()
    await expect(page.locator('.companion-note')).toHaveCount(0)
  })

  test('admin does not get the companion', async ({ page }) => {
    await login(page, adminUser, adminPassword)
    await page.goto('/admin')
    await expect(page.locator('.companion-dock')).toHaveCount(0)
    await expect(page.locator('.companion-note')).toHaveCount(0)
  })
})
