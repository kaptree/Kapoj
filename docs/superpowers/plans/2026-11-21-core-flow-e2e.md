# 核心链路 E2E 冒烟测试实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增独立的 e2e 全栈环境与 `core-flow.spec.ts`，覆盖注册/建题/AC/WA/金币/活跃度/计数，并接入 PR e2e job。

**Architecture:** 用独立 Compose 项目 `kapoj-e2e`、固定端口和命名卷启动全栈；`scripts/e2e-stack.sh` 负责环境、镜像缓存、种子和清理；Playwright spec 用真实管理员 API 建题、真实 UI 注册/登录/提交，状态行作为评测完成的硬断言。

**Tech Stack:** Docker Compose、Bash、Playwright 1.65、Vue 3、Go 后端、PostgreSQL、Redis、MinIO。

**Spec:** `docs/superpowers/specs/2026-11-21-core-flow-e2e-design.md`

## Global Constraints

- 新 spec 必须放在 `apps/web/e2e/core-flow.spec.ts`：现有 `testDir` 是 `apps/web/e2e`，放在 `tests/e2e/` 不会被收集。
- Playwright 已配置 `workers: 1`、`fullyParallel: false`；spec 内使用 `test.describe.serial` 和 `test.describe.configure({ timeout: 180_000 })`。
- e2e 栈固定项目名 `kapoj-e2e`，端口 API `18080`、Web `15173`、PG `15433`、Redis `16380`、MinIO `19000/19001`。
- PostgreSQL/MinIO 必须使用独立命名卷；不得写入主栈的 `kapoj` 数据库或 `minio-data` 宿主目录。
- `.env.e2e` 从 `.env.example` 生成，随机密钥，权限 `0600`，加入 `.gitignore`；不得修改主 `.env`。
- 邮箱验证关闭只发生在 e2e 数据库种子阶段：`mail_settings.require_email_verification=false`，必须使用 `INSERT ... ON CONFLICT (id) DO UPDATE`；生产代码不新增测试后门。
- 测试用户名使用 `e2e_<随机>`（平台用户名规则 `^[a-zA-Z0-9_]{3,32}$` 不允许连字符）；邮箱/昵称使用 `e2e-` 前缀。
- 评测状态行是硬断言（120 秒，覆盖 SSE 和 30 秒轮询）；SSE 的“实时刷新”只记录为注解，不作为失败条件。
- 金币不硬编码余额：断言 `problem_accepted` 流水 `amount=3`，UI 余额与 API `balance` 一致；活跃度断言 `todayPoints` 的增长和 `submissionCount/acceptedProblemCount`。
- 失败产物必须包含 HTML 报告和 `apps/web/test-results`（trace/视频/截图）；Playwright 增加 `video: 'retain-on-failure'`。
- PR 的 e2e job 必须加载新 spec；现有 `admin-rbac.spec.ts` 的权限复选框数从 44 修正为 45。
- 首次 e2e 栈启动目标 ≤ 10 分钟；重复运行复用 Docker 构建缓存、pnpm 缓存和已拉取镜像。

## Review Focus

- CodeMirror 的真实输入：不能用 `fill()` 直接改 `.cm-content`；测试必须用点击 + 全选 + `keyboard.insertText`，并断言编辑器内容，确保提交的是输入的代码。
- 邮箱验证种子：`mail_settings` 是惰性创建的单例，必须 UPSERT；否则首轮测试注册会返回验证流程而不是自动登录。
- 评测完成判定：SSE 可能经 Caddy 断开或缓冲；状态行必须在 120 秒内通过 SSE 或轮询变成“通过”，测试不能只依赖 SSE。
- 异步结算：金币由 gamification worker 异步入账；必须轮询 `coin_transactions`，且不能假设余额恰好是 3（成就可能额外奖励）。
- 数据隔离：e2e 栈必须使用独立项目、端口、PG/MinIO 卷；`down -v` 后卷必须消失，主栈数据不受影响。

## File Structure

- `docker/docker-compose.e2e.yml`：e2e 端口、`.env.e2e`、命名卷 override。
- `scripts/e2e-stack.sh`：`.env.e2e` 生成、镜像预拉取、启动/健康检查、种子、日志、清理。
- `.gitignore`：忽略 `.env.e2e`。
- `apps/web/e2e/core-flow.spec.ts`：核心链路 spec。
- `apps/web/playwright.config.ts`：失败保留视频。
- `apps/web/e2e/admin-rbac.spec.ts`：权限数 44→45。
- `.github/workflows/ci.yml`：e2e job 使用脚本、运行全部 spec、上传报告/trace/视频。
- `README.md`、`apps/web/e2e/README.md`：启动、单跑、清理、调试说明。
- `package.json`：`e2e:stack:up`、`e2e:stack:down`、`test:e2e:core`。

---

## Task 1: 独立 Compose override 与栈脚本骨架

**Files:**
- Create: `docker/docker-compose.e2e.yml`
- Create: `scripts/e2e-stack.sh`
- Modify: `.gitignore`
- Modify: `package.json`

**Interfaces:**
- Produces: `bash scripts/e2e-stack.sh up|down|logs|status`；Compose 项目 `kapoj-e2e`；端口 `18080/15173/15433/16380/19000/19001`；`.env.e2e`。

- [ ] **Step 1: Create the Compose override**

Create `docker/docker-compose.e2e.yml` with exactly these overrides:

```yaml
services:
  postgres:
    ports: !override
      - "15433:5432"
    volumes: !override
      - e2e-pgdata:/var/lib/postgresql/data

  redis:
    ports: !override
      - "16380:6379"

  minio:
    ports: !override
      - "19000:9000"
      - "19001:9001"
    volumes: !override
      - e2e-minio-data:/data

  server:
    env_file: !override
      - ../.env.e2e
    environment:
      KAPOJ_MINIO_BUCKET: kapoj-e2e
      KAPOJ_MINIO_BACKUP_BUCKET: kapoj-e2e-backups
    ports: !override
      - "18080:8080"

  judge:
    env_file: !override
      - ../.env.e2e
    environment:
      KAPOJ_MINIO_BUCKET: kapoj-e2e
      KAPOJ_MINIO_BACKUP_BUCKET: kapoj-e2e-backups

  web:
    ports: !override
      - "15173:80"

volumes:
  e2e-pgdata:
  e2e-minio-data:
```

- [ ] **Step 2: Create the stack script skeleton**

Create `scripts/e2e-stack.sh` with `set -euo pipefail`, root detection, `ENV_FILE=.env.e2e`, and a `compose` array:

```bash
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
ENV_FILE=".env.e2e"
COMPOSE=(docker compose -p kapoj-e2e --env-file "$ENV_FILE"
  -f docker/docker-compose.yml
  -f docker/docker-compose.prod.yml
  -f docker/docker-compose.e2e.yml
  --profile app)
```

Implement:

- `ensure_env`: if `.env.e2e` missing, copy `.env.example` and use Python 3 to replace `POSTGRES_PASSWORD`, `MINIO_ROOT_PASSWORD`, `KAPOJ_JWT_SECRET`, `KAPOJ_AI_ENCRYPTION_KEY`, `KAPOJ_MAIL_ENCRYPTION_KEY` with random values; `chmod 600`.
- `pull_images`: for `gcc:14 python:3.12-slim eclipse-temurin:21-jdk golang:1.26`, skip if `docker image inspect` succeeds, otherwise `docker pull` with up to 3 attempts; skip all when `KAPOJ_E2E_SKIP_PULL=1`.
- `wait_health`: poll `http://127.0.0.1:18080/health` and `http://127.0.0.1:15173/health` for up to 180 seconds; on failure print `"${COMPOSE[@]}" ps` and 200 log lines, then exit 1.
- `up`: `ensure_env`, `pull_images`, `"${COMPOSE[@]}" up -d --build`, `wait_health`, print API/Web URLs and the test command. Print elapsed seconds.
- `down`: call `ensure_env` first (so a failed first `up` can still be cleaned), then `"${COMPOSE[@]}" down -v --remove-orphans`.
- `logs`: call `ensure_env` first, then `"${COMPOSE[@]}" logs --no-color --tail="${2:-300}"`.
- `status`: call `ensure_env` first, then `"${COMPOSE[@]}" ps`.
- Dispatch on `$1` with a usage message for unknown commands.

- [ ] **Step 3: Check shell syntax and Compose config**

Run: `bash -n scripts/e2e-stack.sh && bash scripts/e2e-stack.sh up` (or first `bash scripts/e2e-stack.sh down` if a stale e2e project exists).

Expected: script creates `.env.e2e`, pulls/skips images, builds and starts the stack, both health endpoints return 200, and the script prints the API/Web URLs. If the stack does not become healthy, fix the Compose/ports/env before continuing.

- [ ] **Step 4: Verify isolation and cleanup**

Run: `docker compose -p kapoj-e2e --env-file .env.e2e -f docker/docker-compose.yml -f docker/docker-compose.prod.yml -f docker/docker-compose.e2e.yml ps`

Expected: only `kapoj-e2e-*` containers; the main `kapoj-*` stack remains untouched. Then run `bash scripts/e2e-stack.sh down` and `docker volume ls | grep kapoj-e2e || true`.

Expected: e2e containers stop and `kapoj-e2e` named volumes are gone.

- [ ] **Step 5: Ignore `.env.e2e` and add root scripts**

Add `.env.e2e` to `.gitignore` (next to `.env`). Add to root `package.json` scripts:

```json
"e2e:stack:up": "bash scripts/e2e-stack.sh up",
"e2e:stack:down": "bash scripts/e2e-stack.sh down"
```

- [ ] **Step 6: Commit**

```bash
git add docker/docker-compose.e2e.yml scripts/e2e-stack.sh .gitignore package.json
git commit -m "feat(e2e): add isolated full-stack environment"
```

---

## Task 2: 种子管理员、测试账号与邮箱验证关闭

**Files:**
- Modify: `scripts/e2e-stack.sh`

**Interfaces:**
- Consumes: Task 1 `up` and `compose`.
- Produces: `m2badmin/correct-horse`（super_admin）、`m2bsolver1/correct-horse`、`m2bsolver2/correct-horse`；`mail_settings.require_email_verification=false`。

- [ ] **Step 1: Add `seed` to the script**

Implement `seed` and call it from `up` after `wait_health`:

1. Upsert mail settings:
   ```sql
   INSERT INTO mail_settings (id, updated_at, require_email_verification, sending_enabled)
   VALUES (1, now(), false, false)
   ON CONFLICT (id) DO UPDATE
     SET require_email_verification = false,
         sending_enabled = false,
         updated_at = now();
   ```
2. If `m2badmin` is absent, register it through the real API:
   ```bash
   curl -fsS -X POST http://127.0.0.1:18080/api/auth/register \
     -H 'Content-Type: application/json' \
     --data '{"username":"m2badmin","email":"m2badmin@example.com","password":"correct-horse"}'
   ```
3. Run SQL to verify admin and assign super_admin:
   ```sql
   UPDATE users SET email_verified_at = COALESCE(email_verified_at, now()), status = 'active'
   WHERE username = 'm2badmin';

   INSERT INTO user_roles (user_id, role_id, assigned_at)
   SELECT u.id, r.id, now()
   FROM users u CROSS JOIN roles r
   WHERE u.username = 'm2badmin' AND r.code = 'super_admin'
   ON CONFLICT (user_id, role_id) DO NOTHING;
   ```
4. Seed solvers by copying the admin hash/salt:
   ```sql
   INSERT INTO users (username, nickname, email, email_verified_at, password_hash, salt, role, rating, coins, bio, status, created_at, updated_at)
   SELECT 'm2bsolver1', 'm2bsolver1', 'm2bsolver1@example.com', now(), password_hash, salt, 'user', 1500, 0, '', 'active', now(), now()
   FROM users WHERE username = 'm2badmin'
   ON CONFLICT (username) DO NOTHING;

   INSERT INTO users (username, nickname, email, email_verified_at, password_hash, salt, role, rating, coins, bio, status, created_at, updated_at)
   SELECT 'm2bsolver2', 'm2bsolver2', 'm2bsolver2@example.com', now(), password_hash, salt, 'user', 1500, 0, '', 'active', now(), now()
   FROM users WHERE username = 'm2badmin'
   ON CONFLICT (username) DO NOTHING;
   ```
5. Make the seed idempotent: repeat `up` must not fail when accounts/roles already exist.

- [ ] **Step 2: Verify seed data**

Run: `bash scripts/e2e-stack.sh up` then:

```bash
curl -fsS -X POST http://127.0.0.1:18080/api/auth/login \
  -H 'Content-Type: application/json' \
  --data '{"account":"m2badmin","password":"correct-horse"}'
curl -fsS -X POST http://127.0.0.1:18080/api/auth/login \
  -H 'Content-Type: application/json' \
  --data '{"account":"m2bsolver1","password":"correct-horse"}'
docker compose -p kapoj-e2e --env-file .env.e2e -f docker/docker-compose.yml -f docker/docker-compose.prod.yml -f docker/docker-compose.e2e.yml exec -T postgres \
  psql -U kapoj -d kapoj -tAc "SELECT require_email_verification FROM mail_settings WHERE id=1"
```

Expected: both logins return 200 with `accessToken`; the settings query prints `f`; repeat `up` succeeds.

- [ ] **Step 3: Commit**

```bash
git add scripts/e2e-stack.sh
git commit -m "feat(e2e): seed admin accounts and disable email verification"
```

---

## Task 3: 核心链路 spec

**Files:**
- Create: `apps/web/e2e/core-flow.spec.ts`
- Modify: `package.json`

**Interfaces:**
- Consumes: Task 2 seeded admin, `e2e_` random user, `problem.Input` API, gamification/activity APIs.
- Produces: `core-flow.spec.ts` with one serial end-to-end test.

- [ ] **Step 1: Create the spec with admin setup**

Create `apps/web/e2e/core-flow.spec.ts`. Use the existing `admin-rbac.spec.ts` helpers as a model (`apiData`, `apiLogin`, `apiHeaders`). Add:

```ts
import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

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

async function apiData<T>(res: Awaited<ReturnType<APIRequestContext['get']>>): Promise<T> {
  const body = (await res.json()) as { code: number; message: string; data: T }
  if (!res.ok()) throw new Error(`API ${res.url()} failed: ${body.code} ${body.message}`)
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

async function apiHeaders() {
  return { Authorization: `Bearer ${adminToken}` }
}
```

`beforeAll(async ({ playwright }) => { ... })`:

- Create `adminApi` and `userApi` with `playwright.request.newContext({ baseURL: apiURL })`.
- `adminToken = (await apiLogin(adminApi, adminUser, adminPassword)).accessToken`.
- Create the problem:
  ```ts
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
    await adminApi.post('/api/problems', { headers: await apiHeaders(), data: problemInput }),
  )
  problemId = created.id
  ```
- Upload one testcase:
  ```ts
  const upload = await adminApi.post(`/api/problems/${problemId}/testcases`, {
    headers: await apiHeaders(),
    multipart: {
      input: { name: 'input.txt', mimeType: 'text/plain', buffer: Buffer.from('1 2\n') },
      output: { name: 'output.txt', mimeType: 'text/plain', buffer: Buffer.from('3\n') },
      score: '100',
      isSample: 'false',
    },
  })
  expect(upload.status()).toBe(201)
  ```
- Publish:
  ```ts
  const published = await apiData<{ isPublic: boolean }>(
    await adminApi.put(`/api/problems/${problemId}`, {
      headers: await apiHeaders(),
      data: { ...problemInput, isPublic: true },
    }),
  )
  expect(published.isPublic).toBe(true)
  ```

- [ ] **Step 2: Add the end-to-end test**

Add `test.describe.serial('core flow smoke', () => { ... })` with `test.describe.configure({ mode: 'serial', timeout: 180_000 })` and one test. Use `test.step` for each phase.

**Registration and login:**

```ts
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
```

**AC submission:**

```ts
await test.step('submits AC code and waits for accepted', async () => {
  await page.goto(`/problems/${problemId}/submit`)
  const editor = page.locator('.cm-content')
  await expect(editor).toBeVisible()
  await editor.click()
  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+A' : 'Control+A')
  await page.keyboard.insertText(
    '#include <iostream>\nint main(){int a,b;std::cin>>a>>b;std::cout<<a+b<<"\\n";}\n',
  )
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
  await expect(row.locator('td').nth(7)).toHaveText('100')
})
```

**AC settlement assertions:**

```ts
await test.step('credits coins and activity after AC', async () => {
  userSession = await apiLogin(userApi, e2eUser, e2ePassword)
  userHeaders = { Authorization: `Bearer ${userSession.accessToken}` }

  const problem = await apiData<{ submitCount: number; acceptCount: number }>(
    await adminApi.get(`/api/problems/${problemId}`, { headers: await apiHeaders() }),
  )
  expect(problem.submitCount).toBe(1)
  expect(problem.acceptCount).toBe(1)

  await expect
    .poll(
      async () => {
        const pageData = await apiData<{
          balance: number
          items: Array<{ amount: number; reason: string; refType: string; refId: number }>
        }>(await userApi.get('/api/coins/transactions', { headers: userHeaders }))
        const reward = pageData.items.find(
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

  const coins = await apiData<{ balance: number }>(
    await userApi.get('/api/coins/transactions', { headers: userHeaders }),
  )
  acBalance = coins.balance

  await page.locator('.user-menu__trigger').click()
  await expect(page.locator('.user-menu__coins')).toContainText(String(acBalance))
  await page.keyboard.press('Escape')

  const summary = await apiData<{ todayPoints: number }>(
    await userApi.get('/api/activity/summary', { headers: userHeaders }),
  )
  expect(summary.todayPoints).toBeGreaterThanOrEqual(2)

  const activity = await apiData<{ submissionCount: number; acceptedProblemCount: number }>(
    await userApi.get(`/api/users/${userSession.user.id}/activity`),
  )
  expect(activity.submissionCount).toBe(1)
  expect(activity.acceptedProblemCount).toBe(1)
})
```

**WA submission and counts:**

```ts
await test.step('submits WA and keeps counts correct', async () => {
  await page.goto(`/problems/${problemId}/submit`)
  const editor = page.locator('.cm-content')
  await expect(editor).toBeVisible()
  await editor.click()
  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+A' : 'Control+A')
  await page.keyboard.insertText(
    '#include <iostream>\nint main(){int a,b;std::cin>>a>>b;std::cout<<a-b<<"\\n";}\n',
  )
  await page.getByRole('button', { name: '提交 C++ 代码' }).click()
  await expect(page).toHaveURL(new RegExp(`/submissions\\?problemId=${problemId}`))

  const rows = page.locator('.submissions-table tbody tr')
  await expect(rows).toHaveCount(2)
  await expect(rows.nth(0).getByText('答案错误')).toBeVisible({ timeout: 60_000 })
  await expect(rows.nth(1).getByText('通过')).toBeVisible()

  const problem = await apiData<{ submitCount: number; acceptCount: number }>(
    await adminApi.get(`/api/problems/${problemId}`, { headers: await apiHeaders() }),
  )
  expect(problem.submitCount).toBe(2)
  expect(problem.acceptCount).toBe(1)

  const activity = await apiData<{ submissionCount: number; acceptedProblemCount: number }>(
    await userApi.get(`/api/users/${userSession.user.id}/activity`),
  )
  expect(activity.submissionCount).toBe(2)
  expect(activity.acceptedProblemCount).toBe(1)

  const coins = await apiData<{
    balance: number
    items: Array<{ amount: number; reason: string; refId: number }>
  }>(await userApi.get('/api/coins/transactions', { headers: userHeaders }))
  expect(coins.balance).toBe(acBalance)
  expect(
    coins.items.filter(
      (item) => item.reason === 'problem_accepted' && item.refId === problemId,
    ),
  ).toHaveLength(1)
})
```


- [ ] **Step 3: Add cleanup and console error assertion**

`afterAll`:

```ts
test.afterAll(async () => {
  try {
    if (adminApi && adminToken && problemId) {
      await adminApi.delete(`/api/problems/${problemId}`, { headers: await apiHeaders() })
    }
  } finally {
    await adminApi?.dispose()
    await userApi?.dispose()
  }
})
```

Collect `pageerror` and `console.error` at test start; at the end assert the array is empty. Do not filter errors silently; if an expected error exists, document the filter.

- [ ] **Step 4: Add the root single-run script**

Add to root `package.json`:

```json
"test:e2e:core": "bash scripts/e2e-stack.sh up && E2E_API_URL=http://127.0.0.1:18080 E2E_BASE_URL=http://127.0.0.1:15173 corepack pnpm --filter web test:e2e -- e2e/core-flow.spec.ts"
```

- [ ] **Step 5: Run the spec against the e2e stack**

Run: `corepack pnpm test:e2e:core`

Expected: one test passes; output includes accepted row, coin/activity/count assertions. If it fails, read the failure artifact and fix the spec/env.

- [ ] **Step 6: Commit**

```bash
git add apps/web/e2e/core-flow.spec.ts package.json
git commit -m "test(e2e): add core registration, judging and reward flow"
```

---

## Task 4: 失败视频与现有 admin-rbac 计数修正

**Files:**
- Modify: `apps/web/playwright.config.ts`
- Modify: `apps/web/e2e/admin-rbac.spec.ts`

**Interfaces:**
- Produces: `video: 'retain-on-failure'`; admin-rbac permission count 45.

- [ ] **Step 1: Add video capture**

In `apps/web/playwright.config.ts` `use`, add:

```ts
video: 'retain-on-failure',
```

- [ ] **Step 2: Fix the permission count**

In `apps/web/e2e/admin-rbac.spec.ts`, change:

```ts
await expect(page.locator('.p-dialog:visible input[type=checkbox]')).toHaveCount(44)
```

to:

```ts
await expect(page.locator('.p-dialog:visible input[type=checkbox]')).toHaveCount(45)
```

- [ ] **Step 3: Run the existing admin-rbac spec**

Run: `E2E_API_URL=http://127.0.0.1:18080 E2E_BASE_URL=http://127.0.0.1:15173 corepack pnpm --filter web test:e2e -- e2e/admin-rbac.spec.ts`

Expected: both admin-rbac tests pass; no permission-count failure.

- [ ] **Step 4: Commit**

```bash
git add apps/web/playwright.config.ts apps/web/e2e/admin-rbac.spec.ts
git commit -m "test(e2e): retain failure video and update permission count"
```

---

## Task 5: CI e2e job 接入启动脚本与产物上传

**Files:**
- Modify: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: `scripts/e2e-stack.sh`, `core-flow.spec.ts`.
- Produces: e2e job starts the isolated stack, runs all specs, uploads report + test-results, always downs the stack.

- [ ] **Step 1: Replace inline stack preparation**

In the `e2e` job, delete the steps `Prepare .env with random secrets`, `Start full stack`, `Wait for backend and web health`, and `Seed E2E accounts`. Replace with:

```yaml
- name: Start e2e stack
  run: bash scripts/e2e-stack.sh up
```

- [ ] **Step 2: Point Playwright at the e2e ports**

Replace the `Run Playwright E2E` env/run with:

```yaml
- name: Run Playwright E2E
  env:
    E2E_API_URL: http://127.0.0.1:18080
    E2E_BASE_URL: http://127.0.0.1:15173
  run: corepack pnpm test:e2e
```

- [ ] **Step 3: Upload report and failure artifacts**

Keep the report upload and add test-results:

```yaml
- name: Upload Playwright report
  if: failure()
  uses: actions/upload-artifact@v7
  with:
    name: playwright-report
    path: apps/web/playwright-report
    retention-days: 7
    if-no-files-found: ignore

- name: Upload Playwright traces and video
  if: failure()
  uses: actions/upload-artifact@v7
  with:
    name: playwright-test-results
    path: apps/web/test-results
    retention-days: 7
    if-no-files-found: ignore
```

- [ ] **Step 4: Replace logs/stop steps**

```yaml
- name: Dump e2e stack logs on failure
  if: failure()
  run: bash scripts/e2e-stack.sh logs

- name: Stop e2e stack
  if: always()
  run: bash scripts/e2e-stack.sh down
```

- [ ] **Step 5: Validate the workflow YAML**

Run: `ruby -e 'require "yaml"; YAML.load_file(".github/workflows/ci.yml"); puts "yaml ok"'`

Expected: `yaml ok`. If Ruby is unavailable, use a YAML parser available in the environment and report the command.

- [ ] **Step 6: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci(e2e): use isolated stack and upload failure artifacts"
```

---

## Task 6: README 与 e2e 调试文档

**Files:**
- Modify: `README.md`
- Modify: `apps/web/e2e/README.md`

**Interfaces:**
- Consumes: all previous tasks.
- Produces: documented startup, single-run, cleanup, email-skip, prefix, and debug-artifact instructions.

- [ ] **Step 1: Update the root README test section**

Replace the single `corepack pnpm test:e2e` line with:

```bash
corepack pnpm e2e:stack:up       # 首次构建 + 预拉评测镜像；重复运行走缓存
corepack pnpm test:e2e:core      # 只跑核心链路（自动起栈）
E2E_API_URL=http://127.0.0.1:18080 E2E_BASE_URL=http://127.0.0.1:15173 corepack pnpm test:e2e
corepack pnpm e2e:stack:down     # 清理 e2e PostgreSQL 与 MinIO 卷
```

Also state: first run target ≤ 10 minutes; PR e2e job runs the full suite; failures upload HTML report + trace/video artifacts.

- [ ] **Step 2: Update `apps/web/e2e/README.md`**

Document:

- `core-flow.spec.ts` coverage and dedicated ports/project/volumes.
- Email verification skip: seed SQL sets `mail_settings.require_email_verification=false`; production code unchanged.
- Username prefix: `e2e_` because the platform regex forbids `e2e-`; email/nickname use `e2e-`.
- Commands: `pnpm e2e:stack:up`, `pnpm test:e2e:core`, full e2e with env, `pnpm e2e:stack:down`.
- Failure artifacts: `apps/web/playwright-report/index.html`, `apps/web/test-results/` (trace/video/screenshot), `pnpm --filter web exec playwright show-trace <trace.zip>`.
- Data isolation and cleanup: `down -v` removes e2e DB/MinIO; main `.env`/`minio-data` untouched.

- [ ] **Step 3: Check formatting and docs**

Run: `corepack pnpm lint:web` (checks e2e code and Prettier) and inspect the README snippets with `grep -n "test:e2e:core\\|e2e:stack" README.md apps/web/e2e/README.md`.

Expected: lint passes; both files contain the new commands.

- [ ] **Step 4: Commit**

```bash
git add README.md apps/web/e2e/README.md
git commit -m "docs(e2e): document isolated stack and failure debugging"
```

---

## Task 7: 全量验证与首次启动计时

**Files:**
- No code changes unless verification finds a defect.

**Interfaces:**
- Consumes: all previous tasks.

- [ ] **Step 1: Clean start and measure first-run time**

Run:

```bash
bash scripts/e2e-stack.sh down
/usr/bin/time -p bash scripts/e2e-stack.sh up
```

Expected: stack healthy; elapsed `real` ≤ 600 seconds. Record the number for the final report. If it exceeds 600 seconds, investigate build/image cache before claiming acceptance.

- [ ] **Step 2: Repeat start to prove cache reuse**

Run: `bash scripts/e2e-stack.sh down && /usr/bin/time -p bash scripts/e2e-stack.sh up`

Expected: second run is faster than the first and reuses Docker/image/pnpm caches; do not require a specific number, but report it.

- [ ] **Step 3: Run the core spec and the full e2e suite**

Run:

```bash
corepack pnpm test:e2e:core
E2E_API_URL=http://127.0.0.1:18080 E2E_BASE_URL=http://127.0.0.1:15173 corepack pnpm test:e2e
```

Expected: core spec passes; full suite passes (including the corrected admin-rbac count). If any existing spec is environment-dependent, report the exact failure and whether it is pre-existing.

- [ ] **Step 4: Verify cleanup and isolation**

Run:

```bash
bash scripts/e2e-stack.sh down
docker volume ls | grep kapoj-e2e || true
docker ps --format '{{.Names}}' | grep kapoj-e2e || true
```

Expected: no `kapoj-e2e` containers or named volumes remain. The main `kapoj` stack is unchanged. Confirm `minio-data` was not written by checking `git status --short` and the main stack health.

- [ ] **Step 5: Final checks and commit**

```bash
corepack pnpm lint:web
corepack pnpm build:web
git status --short
```

Expected: lint/build pass; only intended files are changed. Commit any verification-driven fixes with a focused message; do not commit `.env.e2e`, reports, or test-results.
