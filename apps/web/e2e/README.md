# kapOJ 浏览器 E2E

使用 Playwright 和本机 Google Chrome 验证核心链路与管理后台。

## 前置条件

- Docker 与 Compose v2.24+（e2e override 使用 `!override`）。
- Node 22 + corepack pnpm。
- 本机安装 Google Chrome（默认 `channel: chrome`，可用 `E2E_BROWSER_CHANNEL` 覆盖）。
- 失败视频需要 Playwright ffmpeg：`corepack pnpm --filter web exec playwright install ffmpeg`（trace 不需要）。

## 快速开始

```bash
# 仓库根目录
corepack pnpm e2e:stack:up       # 首次构建 + 预拉评测镜像；重复运行走缓存
corepack pnpm test:e2e:core      # 只跑 core-flow 核心链路（自动起栈并安装 ffmpeg）
E2E_API_URL=http://127.0.0.1:18080 E2E_BASE_URL=http://127.0.0.1:15173 corepack pnpm test:e2e
corepack pnpm e2e:stack:down     # 停止并删除 e2e PostgreSQL/MinIO 卷
```

也可以先手动起栈，再按文件运行：

```bash
corepack pnpm e2e:stack:up
E2E_API_URL=http://127.0.0.1:18080 E2E_BASE_URL=http://127.0.0.1:15173 \
  corepack pnpm --filter web exec playwright test e2e/core-flow.spec.ts
```

## 独立 e2e 栈

`scripts/e2e-stack.sh` 使用独立 Compose 项目 `kapoj-e2e` 和 `docker/docker-compose.e2e.yml`：

| 项目         | 值                                                         |
| ------------ | ---------------------------------------------------------- |
| Compose 项目 | `kapoj-e2e`                                                |
| API          | `http://127.0.0.1:18080`                                   |
| Web          | `http://127.0.0.1:15173`                                   |
| PostgreSQL   | `127.0.0.1:15433`                                          |
| Redis        | `127.0.0.1:16380`                                          |
| MinIO        | `19000`（S3）/ `19001`（控制台）                           |
| 数据         | 命名卷 `kapoj-e2e_e2e-pgdata` / `kapoj-e2e_e2e-minio-data` |
| 密钥         | 仓库根 `.env.e2e`（自动生成、`0600`、已忽略）              |

首次 `up` 会拉取评测镜像并构建 server/judge/web，目标 ≤ 10 分钟；本地后续运行复用 Docker、pnpm 与镜像缓存。`down` 使用 `down -v`，删除 e2e 数据库和 MinIO 卷；不会修改主 `.env` 或宿主 `minio-data`。

项目名和端口固定，一台 Docker 主机同时只能运行一个 e2e 栈。切换 worktree 或删除 `.env.e2e` 后，如果旧命名卷仍在，PostgreSQL 会因密码不匹配而启动失败；先运行 `bash scripts/e2e-stack.sh down -v` 再重新 `up`。

## 种子账号与邮箱验证

`e2e-stack.sh up` 会幂等种子：

- `m2badmin` / `correct-horse`：`super_admin`，用于管理后台和建题 API。
- `m2bsolver1`、`m2bsolver2` / `correct-horse`：受限/普通账号，供现有管理后台用例使用。
- `mail_settings.require_email_verification=false`：仅 e2e 数据库的种子设置，生产代码不包含测试后门。因此新注册用户会自动登录，不需要真实 SMTP。

## 测试数据约定

- `core-flow.spec.ts` 使用随机后缀创建用户 `e2e_<suffix>`、邮箱 `e2e-<suffix>@example.com`、昵称 `e2e-<suffix>`。
- 用户名前缀用下划线而不是连字符：平台用户名规则是 `^[a-zA-Z0-9_]{3,32}$`，`e2e-` 不合法。
- 题目 slug 为 `e2e-core-<suffix>`；用例结束会通过管理员 API 下线题目。
- 数据最终由 `e2e-stack.sh down -v` 清除；本地重复运行不会与旧数据冲突。

## core-flow 覆盖

`core-flow.spec.ts` 是独立串行用例：

1. 新用户注册（邮箱验证已跳过）→ 退出 → 登录。
2. 管理员 API 创建 A+B 传统题 → 上传 1 组测试点 → 发布。
3. 用户在真实提交页输入 C++ AC 代码 → 等待状态行“通过”（SSE 或轮询）→ 断言得分 100。
4. 断言 `problem_accepted` 金币流水、UI 金币余额、活跃度和题目 `submitCount/acceptCount`。
5. 再提交 WA 代码 → 等待“答案错误” → 断言计数为 2/1、金币不重复发放、活跃度增长。

SSE 连接状态只作为测试注解；“实时刷新”不是失败条件，评测状态行才是 120 秒硬断言。

## 环境变量

| 变量                                        | 默认值                         | 说明                                                                                                                                                    |
| ------------------------------------------- | ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `E2E_API_URL`                               | `http://127.0.0.1:18080`       | 后端直连地址                                                                                                                                            |
| `E2E_BASE_URL`                              | `http://127.0.0.1:5173`        | 前端地址；设置后 Playwright 不启动 Vite。e2e 栈使用 `http://127.0.0.1:15173`                                                                            |
| `E2E_BROWSER_CHANNEL`                       | `chrome`                       | Playwright 浏览器 channel                                                                                                                               |
| `E2E_ADMIN_USER` / `E2E_ADMIN_PASSWORD`     | `m2badmin` / `correct-horse`   | 超管账号                                                                                                                                                |
| `E2E_LIMITED_USER` / `E2E_LIMITED_PASSWORD` | `m2bsolver1` / `correct-horse` | 受限账号                                                                                                                                                |
| `E2E_LIMITED_ROLE`                          | `auditor`                      | 管理后台用例临时分配给受限账号的角色                                                                                                                    |
| `E2E_PSQL`                                  | 自动探测                       | `learning-paths.spec.ts` 使用的 psql 命令；API 端口为 `18080` 且 `kapoj-e2e-postgres-1` 存在时用它，否则用 `kapoj-postgres-1`。自定义栈用此变量显式覆盖 |

## 失败调试

- HTML 报告：`apps/web/playwright-report/index.html`
- trace/视频/截图：`apps/web/test-results/<test-name>/`
- 查看 trace（`--filter web` 的工作目录是 `apps/web`，路径不要再带 `apps/web/`）：
  ```bash
  corepack pnpm --filter web exec playwright show-trace test-results/<test-name>/trace.zip
  ```
- 查看 e2e 栈日志：
  ```bash
  bash scripts/e2e-stack.sh logs 300
  ```
- CI 的 e2e job 在失败时上传 `playwright-report` 和 `playwright-test-results` 两个 artifact，保留 7 天。

## 既有管理后台用例

`admin-rbac.spec.ts` 仍验证超管管理壳、用户编辑、角色权限矩阵、审计页、游戏化页和移动端 Drawer，以及受限账号的只读行为。权限矩阵断言当前为 45 个复选框。
