import { execSync } from 'node:child_process'
import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

/**
 * Learning path frontend acceptance.
 *
 * Uses the real backend at `E2E_API_URL` (default http://127.0.0.1:18080) and
 * the frontend dev server at `E2E_BASE_URL` (default http://127.0.0.1:5173).
 * The admin creates an official path (root required 2 + two children) through
 * the API; progress is driven by inserting accepted submissions for the
 * ordinary users directly into PostgreSQL (docker exec kapoj-postgres-1). All
 * temporary problems/paths/nodes/progress/submissions/audit rows are removed in
 * `afterAll`; existing data is never touched.
 */

const apiURL = process.env.E2E_API_URL ?? 'http://127.0.0.1:18080'
const adminUser = process.env.E2E_ADMIN_USER ?? 'm2badmin'
const adminPassword = process.env.E2E_ADMIN_PASSWORD ?? 'correct-horse'
const normalUser = process.env.E2E_NORMAL_USER ?? 'm2bsolver1'
const normalPassword = process.env.E2E_NORMAL_PASSWORD ?? 'correct-horse'
const secondUser = process.env.E2E_SECOND_USER ?? 'm2bsolver2'
const secondPassword = process.env.E2E_SECOND_PASSWORD ?? 'correct-horse'

const runId = Date.now().toString()
const slugPrefix = `e2e-lp-${runId}`
const codeMarker = `// e2e-lp-${runId}`
const psqlCommand = process.env.E2E_PSQL ?? 'docker exec kapoj-postgres-1 psql -U kapoj -d kapoj'

/** Run a single SQL statement against the dev database and return trimmed stdout. */
function sql(query: string): string {
  const out = execSync(`${psqlCommand} -Atc ${JSON.stringify(query)}`, { encoding: 'utf8' })
  return out.trim()
}

/**
 * Run an `INSERT ... RETURNING id` statement and return the numeric id. psql
 * appends the command tag (e.g. `INSERT 0 1`) after the returned rows, so take
 * the first line.
 */
function sqlId(query: string): number {
  const first = sql(query).split('\n')[0]?.trim() ?? ''
  return Number(first)
}

/** The maximum id currently present in a table (0 when the table is empty). */
function sqlMaxId(table: string): number {
  return Number(sql(`SELECT COALESCE(MAX(id), 0) FROM ${table}`))
}

/** Count rows for a table/condition, used to assert cleanup restores baselines. */
function sqlCount(expr: string): number {
  return Number(sql(`SELECT count(*) FROM ${expr}`))
}

type Envelope<T> = { code: number; message: string; data: T }

async function apiData<T>(res: Awaited<ReturnType<APIRequestContext['post']>>): Promise<T> {
  const body = (await res.json()) as Envelope<T>
  if (!res.ok()) throw new Error(`API ${res.url()} failed: ${body.code} ${body.message}`)
  return body.data
}

async function apiLogin(ctx: APIRequestContext, account: string, password: string) {
  const res = await ctx.post('/api/auth/login', { data: { account, password } })
  return apiData<{ accessToken: string }>(res)
}

interface LearningPathItem {
  id: number
  title: string
  isOfficial: boolean
  isPublic: boolean
  isActive: boolean
  nodeCount: number
  myCompletedCount: number
  sortOrder: number
}

interface LearningPathNodeItem {
  id: number
  parentId: number | null
  title: string
  problems: Array<{ problemId: number; title: string; slug: string; difficulty: number }>
  requiredCount: number
  solvedCount: number
  status: 'locked' | 'active' | 'completed'
  children: LearningPathNodeItem[]
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

/** A desktop algorithm-tree node, identified by its accessible title. */
function treeNode(page: Page, title: string) {
  return page.locator(`.tree-node[aria-label^="${title}"]`).first()
}

/** A mobile vertical-tree node whose visible text contains `title`. */
function mobileTreeNode(page: Page, title: string) {
  return page.locator('.mobile-node').filter({ hasText: title }).first()
}

/** A list card whose text contains `title`. */
function pathCard(page: Page, title: string) {
  return page.locator('.path-card').filter({ hasText: title }).first()
}

test.describe.serial('learning path frontend acceptance', () => {
  let api: APIRequestContext
  let adminToken = ''
  let userToken = ''

  let normalUserId = 0
  let secondUserId = 0
  let adminUserId = 0

  let officialPathId = 0
  let officialTitle = ''
  let officialRootId = 0
  let widePathId = 0
  let widePathTitle = ''
  let widePathDeepNodeTitle = ''

  let privatePathId = 0
  let privateTitle = ''

  const problemIds: number[] = []
  const problemTitles: string[] = []

  const baseline = {
    problems: 0,
    paths: 0,
    nodes: 0,
    progress: 0,
    progressMaxId: 0,
    submissions: 0,
    learningAudit: 0,
    learningAuditMaxId: 0,
  }

  async function adminHeaders() {
    return { Authorization: `Bearer ${adminToken}` }
  }

  async function userHeaders() {
    return { Authorization: `Bearer ${userToken}` }
  }

  async function createOfficialPath() {
    const path = await apiData<LearningPathItem>(
      await api.post('/api/learning-paths', {
        headers: await adminHeaders(),
        data: {
          title: officialTitle,
          descriptionMd: '官方路线 E2E（临时）',
          isOfficial: true,
          isPublic: true,
          sortOrder: 0,
        },
      }),
    )
    officialPathId = path.id

    const root = await apiData<LearningPathNodeItem>(
      await api.post(`/api/learning-paths/${officialPathId}/nodes`, {
        headers: await adminHeaders(),
        data: {
          title: '根节点',
          problemIds: [problemIds[0], problemIds[1], problemIds[2]],
          requiredCount: 2,
        },
      }),
    )
    officialRootId = root.id
    await apiData<LearningPathNodeItem>(
      await api.post(`/api/learning-paths/${officialPathId}/nodes`, {
        headers: await adminHeaders(),
        data: {
          title: '子节点一',
          parentId: officialRootId,
          problemIds: [problemIds[3]],
          requiredCount: 1,
        },
      }),
    )
    await apiData<LearningPathNodeItem>(
      await api.post(`/api/learning-paths/${officialPathId}/nodes`, {
        headers: await adminHeaders(),
        data: {
          title: '子节点二',
          parentId: officialRootId,
          problemIds: [problemIds[4]],
          requiredCount: 1,
        },
      }),
    )
  }

  async function createDeepWidePath() {
    const path = await apiData<LearningPathItem>(
      await api.post('/api/learning-paths', {
        headers: await adminHeaders(),
        data: {
          title: widePathTitle,
          descriptionMd: '深层宽树 E2E（临时）',
          isOfficial: true,
          isPublic: true,
          sortOrder: 1,
        },
      }),
    )
    widePathId = path.id

    let parentIds: Array<number | null> = [null]
    for (let level = 1; level <= 5; level++) {
      const nextParentIds: number[] = []
      for (const parentId of parentIds) {
        for (let sibling = 1; sibling <= 2; sibling++) {
          const title = `${widePathTitle} 第${level}层-${nextParentIds.length + 1}-兄弟${sibling}`
          const node = await apiData<LearningPathNodeItem>(
            await api.post(`/api/learning-paths/${widePathId}/nodes`, {
              headers: await adminHeaders(),
              data: {
                title,
                ...(parentId === null ? {} : { parentId }),
                problemIds: [problemIds[0]],
                requiredCount: 1,
              },
            }),
          )
          nextParentIds.push(node.id)
          if (level === 5) widePathDeepNodeTitle = title
        }
      }
      parentIds = nextParentIds
    }
  }

  function insertAccepted(userId: number, problemId: number, idx: number) {
    sql(
      `INSERT INTO submissions (user_id, problem_id, language, code, status, created_at, judged_at) ` +
        `VALUES (${userId}, ${problemId}, 'cpp', '${codeMarker}-${idx}', 'accepted', now(), now())`,
    )
  }

  test.beforeAll(async ({ playwright }) => {
    api = await playwright.request.newContext({ baseURL: apiURL })

    const admin = await apiLogin(api, adminUser, adminPassword)
    adminToken = admin.accessToken
    const normal = await apiLogin(api, normalUser, normalPassword)
    userToken = normal.accessToken

    normalUserId = Number(sql(`SELECT id FROM users WHERE username='${normalUser}'`))
    secondUserId = Number(sql(`SELECT id FROM users WHERE username='${secondUser}'`))
    adminUserId = Number(sql(`SELECT id FROM users WHERE username='${adminUser}'`))
    if (!normalUserId || !secondUserId || !adminUserId) throw new Error('E2E users not found')

    // Record baselines (counts + max ids) so cleanup can be scoped and verified.
    baseline.problems = sqlCount('problems')
    baseline.paths = sqlCount('learning_paths')
    baseline.nodes = sqlCount('learning_path_nodes')
    baseline.progress = sqlCount('user_learning_progress')
    baseline.progressMaxId = sqlMaxId('user_learning_progress')
    baseline.submissions = sqlCount('submissions')
    baseline.learningAudit = sqlCount(`admin_audit_logs WHERE target_type='learning_path'`)
    baseline.learningAuditMaxId = sqlMaxId('admin_audit_logs')

    // Create five temporary public problems (self-contained fixtures). The root
    // references three problems (requiredCount 2) so the latch test can raise
    // requiredCount to 3; the two children each reference one more problem.
    for (let i = 1; i <= 5; i++) {
      const title = `${slugPrefix} E2E 题${i}`
      const id = sqlId(
        `INSERT INTO problems (title, slug, is_public, created_by, created_at, updated_at) ` +
          `VALUES ('${title}', '${slugPrefix}-p${i}', true, ${adminUserId}, now(), now()) RETURNING id`,
      )
      problemIds.push(id)
      problemTitles.push(title)
    }

    officialTitle = `${slugPrefix} 官方路线`
    privateTitle = `${slugPrefix} 私有路线`
    widePathTitle = `${slugPrefix} 深层宽树`

    await createOfficialPath()

    // Ordinary user creates a private custom path (root referencing one problem).
    const priv = await apiData<LearningPathItem>(
      await api.post('/api/learning-paths', {
        headers: await userHeaders(),
        data: { title: privateTitle, descriptionMd: '私有路线 E2E（临时）', isPublic: false },
      }),
    )
    privatePathId = priv.id
    // Reference problemIds[2] (p2) which the ordinary user has not AC'd, so the
    // owner sees the root as "进行中" rather than "已完成".
    await apiData<LearningPathNodeItem>(
      await api.post(`/api/learning-paths/${privatePathId}/nodes`, {
        headers: await userHeaders(),
        data: { title: '私有根节点', problemIds: [problemIds[2]], requiredCount: 1 },
      }),
    )

    await createDeepWidePath()
  })

  test.afterAll(async () => {
    // Clean up only the rows this test created; never touch existing rows.
    // `user_learning_progress` is derived and recomputed on read: reading the
    // list/tree as the admin or ordinary users also writes progress for the
    // pre-existing path 1, so we remove only rows created after the baseline
    // snapshot that belong to our readers or temporary paths.
    try {
      sql(`DELETE FROM submissions WHERE code LIKE '${codeMarker}%'`)
      sql(
        `DELETE FROM user_learning_progress ` +
          `WHERE id > ${baseline.progressMaxId} ` +
          `AND (path_id IN (${officialPathId}, ${privatePathId}, ${widePathId}) ` +
          `OR user_id IN (${adminUserId}, ${normalUserId}, ${secondUserId}))`,
      )
      sql(
        `DELETE FROM learning_path_nodes ` +
          `WHERE path_id IN (${officialPathId}, ${privatePathId}, ${widePathId})`,
      )
      sql(
        `DELETE FROM learning_paths WHERE id IN (${officialPathId}, ${privatePathId}, ${widePathId})`,
      )
      sql(`DELETE FROM problems WHERE slug LIKE '${slugPrefix}%'`)
      sql(
        `DELETE FROM admin_audit_logs ` +
          `WHERE target_type='learning_path' AND actor_id=${adminUserId} ` +
          `AND id > ${baseline.learningAuditMaxId}`,
      )

      expect(sqlCount('problems')).toBe(baseline.problems)
      expect(sqlCount('learning_paths')).toBe(baseline.paths)
      expect(sqlCount('learning_path_nodes')).toBe(baseline.nodes)
      expect(sqlCount('user_learning_progress')).toBe(baseline.progress)
      expect(sqlCount('submissions')).toBe(baseline.submissions)
      expect(sqlCount(`admin_audit_logs WHERE target_type='learning_path'`)).toBe(
        baseline.learningAudit,
      )
    } finally {
      await api?.dispose()
    }
  })

  test('guest sees official path: root active, children locked', async ({ page }) => {
    const consoleErrors = collectConsoleErrors(page)
    await page.goto('/learning-paths')
    await expect(pathCard(page, officialTitle)).toBeVisible()
    await expect(pathCard(page, officialTitle)).toContainText('官方')

    await page.goto(`/learning-paths/${officialPathId}`)
    await expect(page.getByRole('heading', { name: officialTitle })).toBeVisible()
    await expect(page.getByText(`已完成 0 / 3 节点`)).toBeVisible()

    const rootNode = treeNode(page, '根节点')
    await expect(rootNode).toHaveClass(/status-active/)
    await expect(rootNode).toHaveAttribute('aria-label', /0 \/ 2/)
    await expect(rootNode).toHaveAttribute('title', '根节点')
    await expect(rootNode.locator('.tree-node__label-title')).toHaveText('根节点')
    await expect(rootNode.locator('.tree-node__label-status')).toHaveText('0 / 2')
    await expect(treeNode(page, '子节点一')).toHaveAttribute('aria-label', /未解锁/)
    await expect(treeNode(page, '子节点一').locator('.tree-node__label-status')).toHaveText(
      '未解锁',
    )
    await expect(treeNode(page, '子节点二')).toHaveAttribute('aria-label', /未解锁/)

    // Detail panel defaults to the first active node (the root) with its problems.
    await expect(page.locator('.node-panel')).toContainText(problemTitles[0])
    await expect(page.locator('.node-panel')).toContainText(problemTitles[1])

    expect(consoleErrors).toEqual([])
  })

  test('desktop algorithm tree: selecting a leaf shows and opens its problem leaves', async ({
    page,
  }) => {
    const consoleErrors = collectConsoleErrors(page)
    await page.goto(`/learning-paths/${officialPathId}`)
    await treeNode(page, '子节点一').click()
    const leaves = page.locator('.tree-leaf')
    await expect(leaves.first()).toBeVisible()
    await expect(leaves.first()).toContainText(problemTitles[3])
    await leaves.first().click()
    await expect(page).toHaveURL(new RegExp(`/problems/${problemIds[3]}$`))
    expect(consoleErrors).toEqual([])
  })

  test('ordinary user sees the same initial state and myCompletedCount', async ({ page }) => {
    const consoleErrors = collectConsoleErrors(page)
    await loginUI(page, normalUser, normalPassword)

    await page.goto('/learning-paths')
    await expect(pathCard(page, officialTitle)).toContainText('已完成 0 / 3 节点')

    await page.goto(`/learning-paths/${officialPathId}`)
    await expect(treeNode(page, '根节点')).toHaveClass(/status-active/)
    await expect(treeNode(page, '子节点一')).toHaveAttribute('aria-label', /未解锁/)
    await expect(treeNode(page, '子节点二')).toHaveAttribute('aria-label', /未解锁/)

    expect(consoleErrors).toEqual([])
  })

  test('progress: root completed, then children completed, myCompletedCount updates', async ({
    page,
  }) => {
    // AC two of the root's three problems (requiredCount 2).
    insertAccepted(normalUserId, problemIds[0], 1)
    insertAccepted(normalUserId, problemIds[1], 2)

    const consoleErrors = collectConsoleErrors(page)
    await loginUI(page, normalUser, normalPassword)
    await page.goto(`/learning-paths/${officialPathId}`)
    const rootNode = treeNode(page, '根节点')
    await expect(rootNode).toHaveClass(/status-completed/)
    await expect(rootNode).toHaveAttribute('aria-label', /2 \/ 2/)
    await expect(treeNode(page, '子节点一')).toHaveClass(/status-active/)
    await expect(treeNode(page, '子节点二')).toHaveClass(/status-active/)

    // AC the two child problems.
    insertAccepted(normalUserId, problemIds[3], 3)
    insertAccepted(normalUserId, problemIds[4], 4)

    await page.reload()
    await expect(rootNode).toHaveClass(/status-completed/)
    await expect(treeNode(page, '子节点一')).toHaveClass(/status-completed/)
    await expect(treeNode(page, '子节点二')).toHaveClass(/status-completed/)
    await expect(page.getByText('已完成 3 / 3 节点')).toBeVisible()

    await page.goto('/learning-paths')
    await expect(pathCard(page, officialTitle)).toContainText('已完成 3 / 3 节点')

    expect(consoleErrors).toEqual([])
  })

  test('completed latch: raising required keeps completed user, other user recomputes', async ({
    page,
  }) => {
    // Raise the root requiredCount 2 -> 3 (full-node replace via admin API).
    await apiData(
      await api.put(`/api/learning-paths/${officialPathId}/nodes/${officialRootId}`, {
        headers: await adminHeaders(),
        data: {
          title: '根节点',
          problemIds: [problemIds[0], problemIds[1], problemIds[2]],
          requiredCount: 3,
        },
      }),
    )

    // Completed user stays completed with the latched solvedCount (2 / 3).
    const completedConsole = collectConsoleErrors(page)
    await loginUI(page, normalUser, normalPassword)
    await page.goto(`/learning-paths/${officialPathId}`)
    const rootNode = treeNode(page, '根节点')
    await expect(rootNode).toHaveClass(/status-completed/)
    await expect(rootNode).toHaveAttribute('aria-label', /2 \/ 3/)

    // A different user with only one AC recomputes against the new config.
    insertAccepted(secondUserId, problemIds[0], 5)

    const secondConsole = collectConsoleErrors(page)
    await page.goto('/')
    await page.evaluate(() => localStorage.clear())
    await loginUI(page, secondUser, secondPassword)
    await page.goto(`/learning-paths/${officialPathId}`)
    await expect(treeNode(page, '根节点')).toHaveClass(/status-active/)
    await expect(treeNode(page, '根节点')).toHaveAttribute('aria-label', /1 \/ 3/)
    await expect(treeNode(page, '子节点一')).toHaveAttribute('aria-label', /未解锁/)

    expect(completedConsole).toEqual([])
    expect(secondConsole).toEqual([])
  })

  test('private custom path: owner sees, guest 404, public visible, admin down/restore', async ({
    page,
  }) => {
    // Owner sees it in their own list and detail.
    const ownerConsole = collectConsoleErrors(page)
    await loginUI(page, normalUser, normalPassword)
    await page.goto('/learning-paths')
    await page.getByRole('button', { name: '我创建的' }).click()
    await expect(pathCard(page, privateTitle)).toBeVisible()
    await page.goto(`/learning-paths/${privatePathId}`)
    await expect(page.getByRole('heading', { name: privateTitle })).toBeVisible()
    await expect(treeNode(page, '私有根节点')).toHaveClass(/status-active/)
    expect(ownerConsole).toEqual([])

    // Guest cannot see the private path. The expected `GET .../tree` 404 is the
    // point of this assertion, so only flag non-404 console errors.
    const guestConsole = collectConsoleErrors(page)
    await page.goto('/')
    await page.evaluate(() => localStorage.clear())
    await page.goto(`/learning-paths/${privatePathId}`)
    await expect(page.getByText('学习路线不存在或无权查看')).toBeVisible()
    expect(guestConsole.filter((e) => !e.includes('404'))).toEqual([])

    // Make it public -> guest sees it.
    await apiData(
      await api.put(`/api/learning-paths/${privatePathId}`, {
        headers: await userHeaders(),
        data: { title: privateTitle, descriptionMd: '私有路线 E2E（临时）', isPublic: true },
      }),
    )

    const guest2Console = collectConsoleErrors(page)
    await page.goto('/')
    await page.evaluate(() => localStorage.clear())
    await page.goto('/learning-paths')
    await expect(pathCard(page, privateTitle)).toBeVisible()
    await page.goto(`/learning-paths/${privatePathId}`)
    await expect(page.getByRole('heading', { name: privateTitle })).toBeVisible()
    expect(guest2Console).toEqual([])

    // Admin downs it via the admin page; the author's list no longer shows it.
    const adminConsole = collectConsoleErrors(page)
    await page.goto('/')
    await page.evaluate(() => localStorage.clear())
    await loginUI(page, adminUser, adminPassword)
    await page.goto('/admin/learning-paths')
    await expect(page.getByRole('heading', { name: '学习路线管理' })).toBeVisible()
    const privateRow = page.locator('tr').filter({ hasText: privateTitle }).first()
    await expect(privateRow).toContainText('上架')
    await privateRow.getByRole('button', { name: '下架' }).click()
    await page.locator('.p-dialog:visible').getByRole('button', { name: '下架' }).click()
    await expect(privateRow).toContainText('已下架')
    expect(adminConsole).toEqual([])

    // Author no longer sees the downed path in their own list.
    const authorConsole = collectConsoleErrors(page)
    await page.goto('/')
    await page.evaluate(() => localStorage.clear())
    await loginUI(page, normalUser, normalPassword)
    await page.goto('/learning-paths')
    await page.getByRole('button', { name: '我创建的' }).click()
    await expect(page.locator('.path-card').filter({ hasText: privateTitle })).toHaveCount(0)
    expect(authorConsole).toEqual([])

    // Admin restores it.
    const restoreConsole = collectConsoleErrors(page)
    await page.goto('/')
    await page.evaluate(() => localStorage.clear())
    await loginUI(page, adminUser, adminPassword)
    await page.goto('/admin/learning-paths')
    const downedRow = page.locator('tr').filter({ hasText: privateTitle }).first()
    await expect(downedRow).toContainText('已下架')
    await downedRow.getByRole('button', { name: '恢复' }).click()
    await page.locator('.p-dialog:visible').getByRole('button', { name: '恢复' }).click()
    await expect(downedRow).toContainText('上架')
    expect(restoreConsole).toEqual([])
  })

  test('node editor: add/edit/delete leaf, has-children delete disabled, validation', async ({
    page,
  }) => {
    const consoleErrors = collectConsoleErrors(page)
    await loginUI(page, adminUser, adminPassword)
    await page.goto(`/learning-paths/${officialPathId}/edit`)
    await expect(page.getByRole('heading', { name: '编辑学习路线' })).toBeVisible()

    // Root (with children) delete disabled; leaves enabled; up/down order states.
    const rootRow = page.locator('.p-treetable-tbody tr').filter({ hasText: '根节点' }).first()
    await expect(rootRow.getByRole('button', { name: '删除' })).toBeDisabled()
    const child1Row = page.locator('.p-treetable-tbody tr').filter({ hasText: '子节点一' }).first()
    await expect(child1Row.getByRole('button', { name: '删除' })).toBeEnabled()
    await expect(child1Row.getByRole('button', { name: '上移' })).toBeDisabled()
    await expect(child1Row.getByRole('button', { name: '下移' })).toBeEnabled()
    const child2Row = page.locator('.p-treetable-tbody tr').filter({ hasText: '子节点二' }).first()
    await expect(child2Row.getByRole('button', { name: '上移' })).toBeEnabled()
    await expect(child2Row.getByRole('button', { name: '下移' })).toBeDisabled()

    // Validation: a node with no problems is rejected.
    await page.getByRole('button', { name: '添加根节点' }).click()
    let dialog = page.locator('.p-dialog:visible')
    await dialog.getByPlaceholder('节点标题').fill('无题目节点')
    await dialog.getByRole('button', { name: '保存' }).click()
    await expect(dialog).toContainText('请至少添加一道题目')
    await dialog.getByRole('button', { name: '取消' }).click()

    // Add a root node with one problem via the remote problem selector.
    await page.getByRole('button', { name: '添加根节点' }).click()
    dialog = page.locator('.p-dialog:visible')
    await dialog.getByPlaceholder('节点标题').fill('新增叶子')
    await dialog.locator('.p-select').filter({ hasText: '搜索题目并选择添加' }).click()
    await page.locator('.p-select-option').filter({ hasText: problemTitles[0] }).first().click()
    await dialog.getByRole('button', { name: '保存' }).click()
    await expect(page.locator('.p-treetable-tbody')).toContainText('新增叶子')

    // Edit the new leaf title (keeps its existing problems).
    const leafRow = page.locator('.p-treetable-tbody tr').filter({ hasText: '新增叶子' }).first()
    await leafRow.getByRole('button', { name: '编辑' }).click()
    dialog = page.locator('.p-dialog:visible')
    await dialog.getByPlaceholder('节点标题').fill('新增叶子改')
    await dialog.getByRole('button', { name: '保存' }).click()
    await expect(page.locator('.p-treetable-tbody')).toContainText('新增叶子改')

    // Delete the leaf (confirm dialog).
    const editedRow = page
      .locator('.p-treetable-tbody tr')
      .filter({ hasText: '新增叶子改' })
      .first()
    await editedRow.getByRole('button', { name: '删除' }).click()
    await page.locator('.p-dialog:visible').getByRole('button', { name: '删除' }).click()
    await expect(page.locator('.p-treetable-tbody')).not.toContainText('新增叶子改')

    expect(consoleErrors).toEqual([])
  })

  test('deep and wide algorithm tree renders every node and centers the selected node', async ({
    page,
  }) => {
    const consoleErrors = collectConsoleErrors(page)
    await page.goto(`/learning-paths/${widePathId}`)
    await expect(page.locator('.tree-node')).toHaveCount(62)

    const selectedNode = treeNode(page, widePathDeepNodeTitle)
    await selectedNode.click()
    await expect(selectedNode).toHaveClass(/selected/)
    await expect
      .poll(() =>
        selectedNode.evaluate((node) => {
          const nodeRect = node.getBoundingClientRect()
          const viewportRect = document.querySelector('.tree-viewport')!.getBoundingClientRect()
          return (
            nodeRect.left >= viewportRect.left &&
            nodeRect.right <= viewportRect.right &&
            nodeRect.top >= viewportRect.top &&
            nodeRect.bottom <= viewportRect.bottom
          )
        }),
      )
      .toBe(true)
    expect(consoleErrors).toEqual([])
  })

  test('light theme keeps algorithm tree nodes and problem leaves visible', async ({ page }) => {
    const consoleErrors = collectConsoleErrors(page)
    await page.addInitScript(() => localStorage.setItem('kapoj.theme', 'dark'))
    await page.goto(`/learning-paths/${officialPathId}`)
    await page.getByRole('button', { name: '切换到亮色主题' }).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
    await expect(treeNode(page, '根节点')).toBeVisible()

    await treeNode(page, '子节点一').click()
    await expect(page.locator('.tree-leaf').first()).toBeVisible()
    expect(consoleErrors).toEqual([])
  })

  test('reduced motion disables algorithm-tree animation after selecting a node', async ({
    page,
  }) => {
    const consoleErrors = collectConsoleErrors(page)
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await page.goto(`/learning-paths/${officialPathId}`)
    const selectedNode = treeNode(page, '子节点一')
    await selectedNode.click()
    await expect(selectedNode).toHaveClass(/selected/)

    const animationName = await page
      .locator('.tree-crowns circle.sparkle')
      .first()
      .evaluate((element) => getComputedStyle(element).animationName)
    expect(animationName).toBe('none')
    expect(consoleErrors).toEqual([])
  })

  test('375px layout: vertical tree, inline dossier, usable dialog, no horizontal scroll', async ({
    page,
  }) => {
    const consoleErrors = collectConsoleErrors(page)
    await page.setViewportSize({ width: 375, height: 800 })

    await page.goto('/learning-paths')
    await expect(pathCard(page, officialTitle)).toBeVisible()
    const listOverflow = await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    )
    expect(listOverflow).toBeLessThanOrEqual(0)

    await page.goto(`/learning-paths/${officialPathId}`)
    await expect(page.locator('.mobile-tree')).toBeVisible()
    await expect(page.locator('.tree-viewport')).toHaveCount(0)
    const mobileNode = mobileTreeNode(page, '子节点一')
    await expect(mobileNode).toBeVisible()
    await mobileNode.click()
    await expect(page.locator('.mobile-node-panel')).toBeVisible()
    await expect(page.locator('.mobile-node-panel')).toContainText(problemTitles[3])
    const treeWidth = await page.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }))
    expect(treeWidth.scrollWidth).toBeLessThanOrEqual(treeWidth.clientWidth)

    await loginUI(page, adminUser, adminPassword)
    await page.goto('/admin/learning-paths')
    await expect(page.getByRole('heading', { name: '学习路线管理' })).toBeVisible()

    await page.goto(`/learning-paths/${officialPathId}/edit`)
    await page.getByRole('button', { name: '添加根节点' }).click()
    const dialog = page.locator('.p-dialog:visible')
    await expect(dialog).toBeVisible()
    await expect(dialog.getByPlaceholder('节点标题')).toBeVisible()
    await dialog.getByRole('button', { name: '取消' }).click()

    expect(consoleErrors).toEqual([])
  })
})
