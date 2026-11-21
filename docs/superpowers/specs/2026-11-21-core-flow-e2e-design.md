# 核心链路端到端冒烟测试设计

> 日期：2026-11-21
> 状态：待评审
> 关联：`apps/web/e2e/`、`.github/workflows/ci.yml`、`docker/docker-compose.yml`、`docker/docker-compose.prod.yml`

## 1. 目标

为 kapOJ 增加一条稳定的核心链路浏览器冒烟测试，覆盖“注册 → 登录 → 管理员建题 → 用户 AC/WA → 评测结算 → 金币/活跃度/计数”全流程，并让它在 PR 的 e2e job 中必跑。

具体目标：

- 新增 `apps/web/e2e/core-flow.spec.ts`，独立串行执行。
- 新用户随机注册，跳过邮箱验证后能正常登录。
- 管理员通过真实 API 创建 A+B 传统题，上传 1 组测试点并发布。
- 用户通过真实 UI 提交 AC 代码，借助 SSE/轮询等待 `accepted`。
- AC 后断言金币流水（`problem_accepted`，+3）和活跃度入账。
- 再提交 WA 代码，断言 `wrong_answer`。
- 断言题目 `submit_count/accept_count` 正确（AC 后 1/1，WA 后 2/1）。
- 新增可复用的 e2e 全栈启动脚本，复用 Compose、预拉评测镜像，首次运行 ≤ 10 分钟，重复运行走缓存。
- CI e2e job 在 PR 时加载新 spec；本地可单独运行。
- 数据隔离：独立 Compose 项目、独立 PostgreSQL/MinIO 卷、`e2e_` 测试用户名前缀；栈销毁时清除数据。
- README 和 `apps/web/e2e/README.md` 更新；失败时能定位 trace、视频和 HTML 报告。

## 2. 现状

- Playwright `testDir` 是 `apps/web/e2e`，`workers: 1`、`fullyParallel: false`、`globalSetup` 只做后端健康检查。
- 现有失败产物：`trace: 'retain-on-failure'`、`screenshot: 'only-on-failure'`，没有视频。
- CI e2e job 在 PR 与手动触发时运行；使用 Compose 全栈（`--profile app`），端口 `8080/80/5433/6380/9000-9001`，种子 `m2badmin`/`m2bsolver1/2`，结束后 `down -v`。
- MinIO 在基础 Compose 中挂载宿主目录 `minio-data`，`down -v` 不会清理；与主开发栈共用端口和数据库卷，隔离性不足。
- 邮箱验证默认开启（`mail_settings.require_email_verification=true`），未配置 SMTP 时注册会被拦截；CI 种子账号通过 SQL 写入已验证邮箱。
- 用户名规则是 `^[a-zA-Z0-9_]{3,32}$`，**不允许连字符**，所以 `e2e-` 不能作为用户名前缀；邮箱/昵称可以使用 `e2e-`。
- 现有 `admin-rbac.spec.ts` 的权限矩阵断言仍期望 44 个复选框，而当前权限目录是 45 个，导致 e2e job 当前为红；本次一并修正。
- AC 金币由 gamification 事件 worker 异步入账：首次 AC 奖励 3 金币，`coin_transactions.reason=problem_accepted`；提交活跃度 +2/次（每日上限 40）。

## 3. 非目标

- 不重写现有管理后台 e2e 用例。
- 不覆盖 Agent、比赛、学习路线、AI 等其他业务链路。
- 不把 e2e 数据写入主开发栈的 `kapoj` 数据库或 `minio-data` 宿主目录。
- 不新增生产环境专用测试后门；邮箱验证关闭只发生在独立 e2e 栈的数据库种子阶段。
- 不要求所有语言评测；AC 使用 C++（`gcc:14`），其他镜像仍预拉取以覆盖常见语言。
- 不引入 Playwright 之外的新测试框架。

## 4. 方案对比

### A. 独立 e2e Compose 项目 + 启动脚本（采用）

新增 `docker/docker-compose.e2e.yml`、`scripts/e2e-stack.sh`，项目名 `kapoj-e2e`，独立端口和命名卷；脚本负责 `.env.e2e`、镜像预拉取、启动/健康检查、种子账号、关闭验证、清理。

- 优点：与主开发栈完全隔离；MinIO 使用命名卷，`down -v` 真正清理；CI 与本地共用同一套准备逻辑；端口固定、可重复。
- 缺点：新增一个 Compose override 和一个脚本；首次构建比复用现有栈多花一点时间。

### B. 复用现有 `kapoj` 全栈

直接使用现有 Compose 项目和端口，测试结束后 `down -v`。

- 优点：改动最少。
- 缺点：与主开发栈端口/数据冲突；MinIO 宿主目录不会清理；无法保证本地重复运行干净。

### C. 本地后端 + Vite dev server

沿用现有 e2e 的默认 18080/5173 模式，不启动 Compose。

- 优点：本地最快。
- 缺点：不验证 judge 容器、MinIO、SSE 的完整全栈；不满足本次环境准备要求。

采用 A。

## 5. 环境准备设计

### 5.1 Compose override

新增 `docker/docker-compose.e2e.yml`：

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

- 使用 `-p kapoj-e2e` 覆盖基础 Compose 的 `name`。
- 端口固定：API `18080`、Web `15173`、PG `15433`、Redis `16380`、MinIO `19000/19001`。
- PostgreSQL/MinIO 使用命名卷，`down -v` 会删除；不再挂载宿主 `minio-data`。
- `web` 来自 `docker-compose.prod.yml`；e2e 只暴露 `15173:80`，不占 443。

### 5.2 `.env.e2e`

- 由 `scripts/e2e-stack.sh` 从 `.env.example` 生成；已存在则复用（重复运行走缓存）。
- 生成时替换随机密钥：`POSTGRES_PASSWORD`、`MINIO_ROOT_PASSWORD`、`KAPOJ_JWT_SECRET`、`KAPOJ_AI_ENCRYPTION_KEY`、`KAPOJ_MAIL_ENCRYPTION_KEY`。
- 权限 `0600`；加入 `.gitignore`，不提交任何真实密钥。
- 不修改主 `.env`。

### 5.3 `scripts/e2e-stack.sh`

命令：

```bash
bash scripts/e2e-stack.sh up     # 生成 env、预拉镜像、启动、健康检查、种子账号
bash scripts/e2e-stack.sh down   # down -v --remove-orphans，删除 e2e 数据库/MinIO 卷
bash scripts/e2e-stack.sh logs   # 失败时输出最近 300 行日志
bash scripts/e2e-stack.sh status # docker compose ps
```

`up` 步骤：

1. 检查 `docker`/`docker compose`。
2. 确保 `.env.e2e` 存在并含随机密钥。
3. 预拉评测镜像（`gcc:14`、`python:3.12-slim`、`eclipse-temurin:21-jdk`、`golang:1.26`）；已存在的镜像跳过，首次运行后走 Docker 缓存。支持 `KAPOJ_E2E_SKIP_PULL=1` 跳过。
4. `docker compose -p kapoj-e2e --env-file .env.e2e -f docker/docker-compose.yml -f docker/docker-compose.prod.yml -f docker/docker-compose.e2e.yml --profile app up -d --build`。
5. 等待 `http://127.0.0.1:18080/health` 和 `http://127.0.0.1:15173/health` 就绪，最多 180 秒；失败时打印 `ps` 和日志。
6. 种子数据（通过 `docker compose exec -T postgres psql`）：
   - `mail_settings` 设置 `require_email_verification=false`、`sending_enabled=false`（测试环境“后台标记”，生产逻辑不变）。
   - 若 `m2badmin` 不存在，用 API 注册；然后 SQL 设置 `email_verified_at=now()` 并分配内置 `super_admin` 角色。
   - 从 `m2badmin` 复制 `password_hash/salt` 创建/保留 `m2bsolver1`、`m2bsolver2`，供现有 admin-rbac e2e 使用。
   - 所有种子账号幂等；重复 `up` 不会重置已有数据。
7. 输出 API/Web 地址、推荐测试命令和本次启动耗时；若首次启动超过 600 秒则打印警告（便于验收“首次 ≤ 10 分钟”）。

`down` 删除 `kapoj-e2e` 项目的容器、网络和命名卷，做到每轮测试后数据库与 MinIO 对象清理。保留 `.env.e2e` 作为缓存。

### 5.4 本地脚本接入

根 `package.json` 增加：

```json
"e2e:stack:up": "bash scripts/e2e-stack.sh up",
"e2e:stack:down": "bash scripts/e2e-stack.sh down",
"test:e2e:core": "bash scripts/e2e-stack.sh up && E2E_API_URL=http://127.0.0.1:18080 E2E_BASE_URL=http://127.0.0.1:15173 corepack pnpm --filter web test:e2e -- e2e/core-flow.spec.ts"
```

- 只跑核心链路：`corepack pnpm test:e2e:core`
- 跑全部 e2e（栈已启动）：`E2E_API_URL=http://127.0.0.1:18080 E2E_BASE_URL=http://127.0.0.1:15173 corepack pnpm test:e2e`
- 清理：`corepack pnpm e2e:stack:down`

## 6. 核心测试设计

### 6.1 文件与执行模型

- 文件：`apps/web/e2e/core-flow.spec.ts`（现有 `testDir` 是 `apps/web/e2e`，放这里才会被自动收集；不新建 `tests/e2e` 目录）。
- `test.describe.serial` + `test.describe.configure({ timeout: 180_000 })`；Playwright 配置本身 `workers: 1`，保证独立串行。
- 使用 `test.step` 标记“注册/登录/建题/AC/结算/WA/计数”阶段，失败时报告可读。
- 收集 `pageerror`/`console.error`，结束时断言为空；忽略已知的浏览器扩展/环境噪声时需在测试中显式过滤，不能静默放过。

### 6.2 测试数据

- 随机后缀：`Date.now().toString(36) + Math.random().toString(36).slice(2, 6)`。
- 用户名：`e2e_<suffix>`（符合平台 `[a-zA-Z0-9_]{3,32}` 规则）。
- 邮箱：`e2e-<suffix>@example.com`；昵称：`e2e-<suffix>`；密码：`correct-horse`。
- 题目：`title=E2E A+B <suffix>`，`slug=e2e-core-<suffix>`。
- 所有测试数据带 `e2e_`/`e2e-` 标识；题目结束后通过管理员 API 下线/删除，数据最终由 `e2e-stack.sh down -v` 清除。

### 6.3 管理员建题（API setup）

- 用 `m2badmin/correct-horse` 登录真实 API，获取 token。
- `POST /api/problems`：
  - `title`、`slug`、`statementMd`（A+B 题面）、`inputFormatMd`/`outputFormatMd`、`samples=[{input:"1 2\n",output:"3\n"}]`、`type:"traditional"`、`difficulty:1`、`difficultyAuto:false`、`timeLimitMs:1000`、`memoryLimitMb:256`、`isPublic:false`、`source:"e2e"`。
  - 创建接口会忽略 `isPublic`，新题默认未公开。
- `POST /api/problems/:id/testcases` multipart：`input=1 2\n`、`output=3\n`、`score=100`、`isSample=false`；断言 201。
- `PUT /api/problems/:id` 整体替换并设置 `isPublic:true`，完成“上架”；断言 200 且 `isPublic=true`。
- 记录 `problemId` 供 UI 提交和清理。

### 6.4 注册、跳过验证、登录（UI）

- `page.goto('/register')`，按 label 填写用户名/邮箱/密码/昵称，点击“注册”。
- e2e 栈已关闭邮箱验证，注册成功应自动登录并跳转 `/`；断言 toast “注册成功”和用户菜单昵称。
- 通过用户菜单点击“退出登录”，回到 `/`；再走 `/login` 用用户名+密码登录，验证“跳过验证后正常登录”。
- 注册/登录的断言只依赖真实 UI 和真实 API，不 mock 认证。

### 6.5 AC 提交与评测等待（UI + SSE/轮询）

- `page.goto('/problems/:id/submit')`，等待 `.cm-content`。
- 用 `.cm-content.fill(acCode)` 填入 A+B C++ 代码；断言编辑器内容包含关键代码。
- 点击按钮“提交 C++ 代码”，等待跳转 `/submissions?problemId=:id`。
- 断言页面出现“实时刷新”（SSE 已连接）；即使 SSE 短暂不可用，页面每 30 秒轮询，状态等待超时 120 秒。
- 定位 `.submissions-table tbody tr` 第一行，等待状态文本“通过”，并断言得分 100。

### 6.6 AC 后金币与活跃度

- 用新用户 API 登录，获取 `accessToken` 和 `user.id`。
- 轮询 `GET /api/coins/transactions`（用户 token），直到：
  - `balance >= 3`
  - 存在 `reason=problem_accepted`、`amount=3`、`refType=problem`、`refId=problemId` 的流水。
- 点击用户菜单，断言 UI 金币余额显示 3（或至少包含 3）。
- 轮询 `GET /api/activity/summary`（用户 token），断言 `todayPoints >= 2`；再查 `GET /api/users/:id/activity`，断言 `submissionCount=1`、`acceptedProblemCount=1`。

### 6.7 WA 提交与计数

- 再次进入提交页，填入错误代码（例如输出 `a-b`），提交。
- 等待 `/submissions?problemId=:id`，第一行状态“答案错误”；第二行仍为“通过”。
- `GET /api/problems/:id`：`submitCount=2`、`acceptCount=1`。
- `GET /api/users/:id/activity`：`submissionCount=2`、`acceptedProblemCount=1`。
- 再查金币流水：只有 1 条 `problem_accepted`，余额仍为 3；`GET /api/activity/summary` 的 `todayPoints >= 4`。

### 6.8 清理

- `afterAll`：
  - 用管理员 token `DELETE /api/problems/:id`（已有提交时平台会下线而非物理删除）；best-effort，失败只记录警告。
  - 释放 API contexts。
- 新用户无法通过公开 API 删除；由 `e2e-stack.sh down -v` 在每轮 CI 结束时清除数据库和 MinIO 卷。
- 本地重复运行使用随机后缀，不会与旧数据冲突；需要彻底清理时执行 `pnpm e2e:stack:down`。

## 7. CI 设计

`.github/workflows/ci.yml` 的 `e2e` job：

- 保留 `if: pull_request || workflow_dispatch`，PR 必跑。
- 删除内联的 `.env` 生成、Compose 启动、健康检查、种子账号步骤，改为：

```yaml
- name: Start e2e stack
  run: bash scripts/e2e-stack.sh up
```

- 运行 Playwright：

```yaml
- name: Run Playwright E2E
  env:
    E2E_API_URL: http://127.0.0.1:18080
    E2E_BASE_URL: http://127.0.0.1:15173
  run: corepack pnpm test:e2e
```

- 失败时上传 HTML 报告和 `apps/web/test-results`（trace/视频/截图）：

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

- 失败日志与关闭：

```yaml
- name: Dump e2e stack logs on failure
  if: failure()
  run: bash scripts/e2e-stack.sh logs

- name: Stop e2e stack
  if: always()
  run: bash scripts/e2e-stack.sh down
```

- 现有 e2e job 的 60 分钟超时保留；首次启动目标 ≤ 10 分钟，脚本会打印耗时并在超过 600 秒时警告。
- 顺手修正 `apps/web/e2e/admin-rbac.spec.ts` 的权限复选框断言：`toHaveCount(44)` → `toHaveCount(45)`，让现有 e2e job 恢复绿色。

## 8. 调试与文档

Playwright 配置新增：

```ts
video: 'retain-on-failure',
```

失败产物：

- HTML 报告：`apps/web/playwright-report/index.html`
- trace/视频/截图：`apps/web/test-results/<test-name>/`
- 查看 trace：`corepack pnpm --filter web exec playwright show-trace <path>/trace.zip`

`README.md`「测试与检查」更新：

```bash
corepack pnpm e2e:stack:up       # 首次构建 + 预拉评测镜像；重复运行走缓存
corepack pnpm test:e2e:core      # 只跑核心链路（自动起栈）
E2E_API_URL=http://127.0.0.1:18080 E2E_BASE_URL=http://127.0.0.1:15173 corepack pnpm test:e2e
corepack pnpm e2e:stack:down     # 清理 e2e 数据库与 MinIO 卷
```

`apps/web/e2e/README.md` 更新：

- 新 spec 说明、固定端口、独立 Compose 项目与卷。
- 邮箱验证跳过方式：e2e 栈种子 SQL 将 `mail_settings.require_email_verification` 置 false；生产代码不放测试后门。
- 测试用户前缀为什么是 `e2e_` 而不是 `e2e-`（用户名规则不允许连字符）。
- 失败报告、trace、视频、日志位置。
- 首次运行与重复运行的时间预期。

## 9. 验收标准

- `docker compose` 全栈在独立项目 `kapoj-e2e` 中启动，端口不与主栈冲突。
- 首次 `bash scripts/e2e-stack.sh up` 在干净机器上 ≤ 10 分钟；重复运行明显更快并复用镜像/构建缓存。
- `corepack pnpm test:e2e:core` 可单独跑通 core-flow，并输出 accepted/金币/活跃度/计数断言结果。
- PR 的 e2e job 加载 `core-flow.spec.ts` 并执行；失败时 artifact 包含 HTML 报告、trace、视频。
- `down -v` 后 e2e PostgreSQL 与 MinIO 卷被删除，下一轮从干净数据开始。
- 主开发栈的 `.env`、`kapoj` 数据库、`minio-data` 不被 e2e 修改。
- README 与 e2e README 包含启动、单跑、调试、清理命令。
- 现有 admin-rbac e2e 的权限数断言修正后不再因 44/45 误报。
