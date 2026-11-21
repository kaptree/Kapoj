import { expect, test, type APIRequestContext, type APIResponse } from '@playwright/test'

const apiURL = process.env.E2E_API_URL ?? 'http://127.0.0.1:18080'
const adminUser = process.env.E2E_ADMIN_USER ?? 'm2badmin'
const adminPassword = process.env.E2E_ADMIN_PASSWORD ?? 'correct-horse'

const suffix = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
const e2eUser = `e2e_${suffix}`
const e2eEmail = `e2e-${suffix}@example.com`
const e2eNickname = `e2e-${suffix}`
const e2ePassword = 'correct-horse'
const problemSlug = `e2e-core-${suffix}`
const problemTitle = `E2E A+B ${suffix}`

let adminApi: APIRequestContext
let userApi: APIRequestContext
let adminToken = ''
let problemId = 0
let userSession = { accessToken: '', user: { id: 0 } }
let userHeaders = { Authorization: '' }
let acBalance = 0

type ProblemStats = {
  id: number
  isPublic: boolean
  submitCount: number
  acceptCount: number
}

type CoinPage = {
  balance: number
  items: Array<{
    amount: number
    reason: string
    refType: string
    refId: number
  }>
}

type Activity = {
  submissionCount: number
  acceptedProblemCount: number
}

type ActivitySummary = {
  todayPoints: number
}

async function apiData<T>(res: APIResponse): Promise<T> {
  if (!res.ok()) {
    const detail = (await res.text()).slice(0, 300)
    throw new Error(`API ${res.url()} failed: ${res.status()} ${detail}`)
  }
  const body = (await res.json()) as { code: number; message: string; data: T }
  return body.data
}

async function apiLogin(
  ctx: APIRequestContext,
  account: string,
  password: string,
): Promise<{ accessToken: string; user: { id: number } }> {
  const res = await ctx.post('/api/auth/login', { data: { account, password } })
  return apiData<{ accessToken: string; user: { id: number } }>(res)
}

function apiHeaders() {
  return { Authorization: `Bearer ${adminToken}` }
}

function acCode(): string {
  return '#include <iostream>\nint main(){int a,b;std::cin>>a>>b;std::cout<<a+b<<"\\n";}\n'
}

function waCode(): string {
  return '#include <iostream>\nint main(){int a,b;std::cin>>a>>b;std::cout<<a-b<<"\\n";}\n'
}

test.describe.serial('core flow smoke', () => {
  test.describe.configure({ timeout: 180_000 })

  test.beforeAll(async ({ playwright }) => {
    adminApi = await playwright.request.newContext({ baseURL: apiURL })
    userApi = await playwright.request.newContext({ baseURL: apiURL })
    adminToken = (await apiLogin(adminApi, adminUser, adminPassword)).accessToken

    const problemInput = {
      title: problemTitle,
      slug: problemSlug,
      statementMd: '输入两个整数 a 和 b，输出 a + b。',
      inputFormatMd: '一行两个整数 a、b。',
      outputFormatMd: '一行一个整数 a + b。',
      samples: [{ input: '1 2\n', output: '3\n' }],
      hintMd: '',
      type: 'traditional',
      tags: ['e2e'],
      difficulty: 1,
      difficultyAuto: false,
      timeLimitMs: 1000,
      memoryLimitMb: 256,
      isPublic: false,
      source: 'e2e',
    }

    const created = await apiData<{ id: number }>(
      await adminApi.post('/api/problems', { headers: apiHeaders(), data: problemInput }),
    )
    problemId = created.id

    const upload = await adminApi.post(`/api/problems/${problemId}/testcases`, {
      headers: apiHeaders(),
      multipart: {
        input: { name: 'input.txt', mimeType: 'text/plain', buffer: Buffer.from('1 2\n') },
        output: { name: 'output.txt', mimeType: 'text/plain', buffer: Buffer.from('3\n') },
        score: '100',
        isSample: 'false',
      },
    })
    expect(upload.status()).toBe(201)

    const published = await apiData<ProblemStats>(
      await adminApi.put(`/api/problems/${problemId}`, {
        headers: apiHeaders(),
        data: { ...problemInput, isPublic: true },
      }),
    )
    expect(published.isPublic).toBe(true)
  })

  test.afterAll(async () => {
    try {
      if (adminApi && adminToken && problemId > 0) {
        const res = await adminApi.delete(`/api/problems/${problemId}`, {
          headers: apiHeaders(),
        })
        if (!res.ok()) {
          console.warn(`core-flow cleanup failed: ${res.status()} ${await res.text()}`)
        }
      }
    } finally {
      await adminApi?.dispose()
      await userApi?.dispose()
    }
  })

  test('registers, submits AC/WA and settles rewards', async ({ page }) => {
    const pageErrors: string[] = []
    const consoleErrors: string[] = []
    page.on('pageerror', (err) => pageErrors.push(err.message))
    page.on('console', (msg) => {
      if (msg.type() === 'error') consoleErrors.push(msg.text())
    })

    await test.step('registers, skips email verification, and logs in', async () => {
      await page.goto('/register')
      await page.getByLabel('用户名').fill(e2eUser)
      await page.getByLabel('邮箱').fill(e2eEmail)
      await page.getByLabel('密码').fill(e2ePassword)
      await page.getByLabel('昵称').fill(e2eNickname)
      await page.getByRole('button', { name: '注册' }).click()
      await expect(page).toHaveURL('/')
      await expect(page.getByText('注册成功')).toBeVisible()
      await expect(page.locator('.user-menu__trigger')).toBeVisible()

      await page.locator('.user-menu__trigger').click()
      await page.getByRole('menuitem', { name: '退出登录' }).click()
      await expect(page).toHaveURL('/')

      await page.goto('/login')
      await page.getByLabel('用户名或邮箱').fill(e2eUser)
      await page.getByLabel('密码').fill(e2ePassword)
      await page.getByRole('button', { name: '登录' }).click()
      await expect(page).toHaveURL('/')
      await expect(page.locator('.user-menu__trigger')).toContainText(e2eNickname)
    })

    await test.step('submits AC code and waits for accepted', async () => {
      await page.goto(`/problems/${problemId}/submit`)
      const editor = page.locator('.cm-content')
      await expect(editor).toBeVisible({ timeout: 20_000 })
      await editor.click()
      await page.keyboard.press(process.platform === 'darwin' ? 'Meta+A' : 'Control+A')
      await page.keyboard.insertText(acCode())
      await expect(editor).toContainText('std::cin')

      await page.getByRole('button', { name: '提交 C++ 代码' }).click()
      await expect(page).toHaveURL(new RegExp(`/submissions\\?problemId=${problemId}`))

      const sseConnected = await page
        .getByText('实时刷新')
        .waitFor({ state: 'visible', timeout: 10_000 })
        .then(() => true)
        .catch(() => false)
      test.info().annotations.push({
        type: 'sse',
        description: sseConnected ? 'connected' : 'not connected; polling fallback',
      })

      const row = page.locator('.submissions-table tbody tr').first()
      await expect(row.getByText('通过')).toBeVisible({ timeout: 120_000 })

      const scoreColumn = await page
        .locator('.submissions-table thead th')
        .evaluateAll((headers) => headers.findIndex((h) => h.textContent?.trim() === '得分'))
      expect(scoreColumn).toBeGreaterThanOrEqual(0)
      await expect(row.locator('td').nth(scoreColumn)).toHaveText('100')
    })

    await test.step('credits coins and activity after AC', async () => {
      userSession = await apiLogin(userApi, e2eUser, e2ePassword)
      userHeaders = { Authorization: `Bearer ${userSession.accessToken}` }

      const problem = await apiData<ProblemStats>(
        await adminApi.get(`/api/problems/${problemId}`, { headers: apiHeaders() }),
      )
      expect(problem.submitCount).toBe(1)
      expect(problem.acceptCount).toBe(1)

      await expect
        .poll(
          async () => {
            const coins = await apiData<CoinPage>(
              await userApi.get('/api/coins/transactions', { headers: userHeaders }),
            )
            const reward = coins.items.find(
              (item) =>
                item.reason === 'problem_accepted' &&
                item.refType === 'problem' &&
                item.refId === problemId,
            )
            return reward?.amount ?? 0
          },
          { timeout: 30_000 },
        )
        .toBe(3)

      const coins = await apiData<CoinPage>(
        await userApi.get('/api/coins/transactions', { headers: userHeaders }),
      )
      acBalance = coins.balance

      await page.locator('.user-menu__trigger').click()
      await expect(page.locator('.user-menu__coins strong')).toHaveText(String(acBalance))
      await page.keyboard.press('Escape')

      const summary = await apiData<ActivitySummary>(
        await userApi.get('/api/activity/summary', { headers: userHeaders }),
      )
      expect(summary.todayPoints).toBeGreaterThanOrEqual(2)

      const activity = await apiData<Activity>(
        await userApi.get(`/api/users/${userSession.user.id}/activity`),
      )
      expect(activity.submissionCount).toBe(1)
      expect(activity.acceptedProblemCount).toBe(1)
    })

    await test.step('submits WA and keeps counts correct', async () => {
      await page.goto(`/problems/${problemId}/submit`)
      const editor = page.locator('.cm-content')
      await expect(editor).toBeVisible({ timeout: 20_000 })
      await editor.click()
      await page.keyboard.press(process.platform === 'darwin' ? 'Meta+A' : 'Control+A')
      await page.keyboard.insertText(waCode())
      await expect(editor).toContainText('a-b')

      await page.getByRole('button', { name: '提交 C++ 代码' }).click()
      await expect(page).toHaveURL(new RegExp(`/submissions\\?problemId=${problemId}`))

      const rows = page.locator('.submissions-table tbody tr')
      await expect(rows).toHaveCount(2)
      await expect(rows.nth(0).getByText('答案错误')).toBeVisible({ timeout: 60_000 })
      await expect(rows.nth(1).getByText('通过')).toBeVisible()

      const problem = await apiData<ProblemStats>(
        await adminApi.get(`/api/problems/${problemId}`, { headers: apiHeaders() }),
      )
      expect(problem.submitCount).toBe(2)
      expect(problem.acceptCount).toBe(1)

      const activity = await apiData<Activity>(
        await userApi.get(`/api/users/${userSession.user.id}/activity`),
      )
      expect(activity.submissionCount).toBe(2)
      expect(activity.acceptedProblemCount).toBe(1)

      const coins = await apiData<CoinPage>(
        await userApi.get('/api/coins/transactions', { headers: userHeaders }),
      )
      // Achievements or other asynchronous awards may add coins between the AC
      // snapshot and this check; only the absence of a duplicate AC reward is
      // guaranteed.
      expect(coins.balance).toBeGreaterThanOrEqual(acBalance)
      expect(
        coins.items.filter(
          (item) =>
            item.reason === 'problem_accepted' &&
            item.refType === 'problem' &&
            item.refId === problemId,
        ),
      ).toHaveLength(1)

      const summary = await apiData<ActivitySummary>(
        await userApi.get('/api/activity/summary', { headers: userHeaders }),
      )
      expect(summary.todayPoints).toBeGreaterThanOrEqual(4)
    })

    expect(pageErrors).toEqual([])
    expect(consoleErrors).toEqual([])
  })
})
