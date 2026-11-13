import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

const apiURL = process.env.E2E_API_URL ?? 'http://127.0.0.1:18080'
const adminUser = process.env.E2E_ADMIN_USER ?? 'm2badmin'
const adminPassword = process.env.E2E_ADMIN_PASSWORD ?? 'correct-horse'
const limitedUser = process.env.E2E_LIMITED_USER ?? 'm2bsolver1'
const limitedPassword = process.env.E2E_LIMITED_PASSWORD ?? 'correct-horse'
const limitedRoleCode = process.env.E2E_LIMITED_ROLE ?? 'auditor'

let api: APIRequestContext
let adminToken = ''
let limitedUserId = 0
let originalRoleIds: number[] = []

async function apiData<T>(res: Awaited<ReturnType<APIRequestContext['get']>>): Promise<T> {
  const body = (await res.json()) as { code: number; message: string; data: T }
  if (!res.ok()) throw new Error(`API ${res.url()} failed: ${body.code} ${body.message}`)
  return body.data
}

async function apiLogin(ctx: APIRequestContext, account: string, password: string) {
  const res = await ctx.post('/api/auth/login', { data: { account, password } })
  return apiData<{ accessToken: string }>(res)
}

async function apiHeaders() {
  return { Authorization: `Bearer ${adminToken}` }
}

async function findUser(username: string) {
  const res = await api.get(
    `/api/admin/users?keyword=${encodeURIComponent(username)}&page=1&page_size=20`,
    {
      headers: await apiHeaders(),
    },
  )
  const page = await apiData<{
    items: Array<{ id: number; username: string; roles: Array<{ id: number }> }>
  }>(res)
  const user = page.items.find((item) => item.username === username)
  if (!user) throw new Error(`E2E user not found: ${username}`)
  return user
}

async function replaceRoles(userId: number, roleIds: number[]) {
  const res = await api.put(`/api/admin/users/${userId}/roles`, {
    headers: await apiHeaders(),
    data: { roleIds },
  })
  if (!res.ok()) {
    const body = (await res.json()) as { message?: string }
    throw new Error(`replace roles failed: ${body.message ?? res.status()}`)
  }
}

async function loginUI(page: Page, account: string, password: string) {
  await page.goto('/login')
  await page.getByLabel('用户名或邮箱').fill(account)
  await page.getByLabel('密码').fill(password)
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL('/')
}

function collectConsoleErrors(page: Page) {
  const errors: string[] = []
  page.on('console', (msg) => {
    if (msg.type() === 'error') errors.push(msg.text())
  })
  page.on('pageerror', (err) => errors.push(err.message))
  return errors
}

test.describe.serial('admin RBAC browser acceptance', () => {
  test.beforeAll(async ({ playwright }) => {
    api = await playwright.request.newContext({ baseURL: apiURL })
    const session = await apiLogin(api, adminUser, adminPassword)
    adminToken = session.accessToken

    const rolesRes = await api.get('/api/admin/roles', { headers: await apiHeaders() })
    const roles = await apiData<Array<{ id: number; code: string }>>(rolesRes)
    const limitedRole = roles.find((role) => role.code === limitedRoleCode)
    if (!limitedRole) throw new Error(`E2E role not found: ${limitedRoleCode}`)

    const user = await findUser(limitedUser)
    limitedUserId = user.id
    originalRoleIds = user.roles.map((role) => role.id)
    await replaceRoles(limitedUserId, [limitedRole.id])
  })

  test.afterAll(async () => {
    try {
      if (api && limitedUserId > 0) await replaceRoles(limitedUserId, originalRoleIds)
    } finally {
      await api?.dispose()
    }
  })

  test('super admin can use the admin shell and permission matrix', async ({ page }) => {
    const consoleErrors = collectConsoleErrors(page)
    await loginUI(page, adminUser, adminPassword)

    await page.goto('/admin')
    await expect(page.getByRole('heading', { name: '管理后台' })).toBeVisible()
    await expect(page.getByText('管理快捷入口')).toBeVisible()
    await expect(page.getByText('内容快捷入口')).toBeVisible()

    await page.goto('/admin/users')
    await expect(page.getByRole('heading', { name: '用户管理' })).toBeVisible()
    await expect(page.getByText(adminUser).first()).toBeVisible()
    const limitedRow = page.locator('tr').filter({ hasText: limitedUser }).first()
    await limitedRow.getByRole('button', { name: '编辑' }).click()
    await expect(page.locator('#edit-nickname')).toHaveValue(limitedUser)
    await page.locator('.p-dialog:visible').getByRole('button', { name: '取消' }).click()

    await page.goto('/admin/roles')
    await expect(page.getByRole('heading', { name: '角色权限' })).toBeVisible()
    await expect(page.getByText('超级管理员')).toBeVisible()
    const contentAdminRow = page.locator('tr').filter({ hasText: 'content_admin' }).first()
    await contentAdminRow.getByRole('button', { name: '权限' }).click()
    await expect(page.locator('.p-dialog:visible input[type=checkbox]')).toHaveCount(38)
    await page.locator('.p-dialog:visible').getByRole('button', { name: '取消' }).click()

    await page.goto('/admin/audit')
    await expect(page.getByRole('heading', { name: '审计日志' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: 'ACTION' })).toBeVisible()

    await page.goto('/admin/gamification')
    await expect(page.getByRole('tab', { name: '成就' })).toBeVisible()
    await expect(page.getByRole('tab', { name: '任务' })).toBeVisible()

    await page.setViewportSize({ width: 690, height: 800 })
    await page.goto('/admin')
    await page.getByRole('button', { name: '打开导航' }).click()
    const drawer = page.locator('#app-mobile-nav')
    for (const label of [
      '用户管理',
      '角色权限',
      '审计日志',
      'Rating 设置',
      '游戏化管理',
      '题库管理',
      '比赛管理',
    ]) {
      await expect(drawer.getByRole('link', { name: label })).toBeVisible()
    }

    expect(consoleErrors).toEqual([])
  })

  test('auditor sees read-only admin pages and cannot enter gamification', async ({ page }) => {
    const consoleErrors = collectConsoleErrors(page)
    await loginUI(page, limitedUser, limitedPassword)

    await page.goto('/admin')
    await expect(page.getByRole('heading', { name: '管理后台' })).toBeVisible()
    await expect(page.getByText('管理快捷入口')).toBeVisible()

    await page.goto('/admin/users')
    await expect(page.getByRole('heading', { name: '用户管理' })).toBeVisible()
    await expect(page.getByRole('button', { name: '编辑' })).toHaveCount(0)
    await expect(page.getByRole('button', { name: '封禁' })).toHaveCount(0)
    await expect(page.getByRole('button', { name: '金币' })).toHaveCount(0)

    await page.goto('/admin/roles')
    await expect(page.getByRole('heading', { name: '角色权限' })).toBeVisible()
    await expect(page.getByRole('button', { name: '新建角色' })).toHaveCount(0)

    await page.goto('/admin/audit')
    await expect(page.getByRole('heading', { name: '审计日志' })).toBeVisible()

    await page.goto('/admin/gamification')
    await expect(page).toHaveURL('/forbidden')
    await expect(page.getByRole('main').getByText('没有权限')).toBeVisible()

    await page.setViewportSize({ width: 690, height: 800 })
    await page.goto('/admin')
    await page.getByRole('button', { name: '打开导航' }).click()
    const drawer = page.locator('#app-mobile-nav')
    for (const label of ['用户管理', '角色权限', '审计日志']) {
      await expect(drawer.getByRole('link', { name: label })).toBeVisible()
    }
    await expect(drawer.getByRole('link', { name: 'Rating 设置' })).toHaveCount(0)
    await expect(drawer.getByRole('link', { name: '游戏化管理' })).toHaveCount(0)
    await expect(drawer.getByRole('link', { name: '题库管理' })).toHaveCount(0)

    expect(consoleErrors).toEqual([])
  })
})
