# RBAC 管理后台浏览器自动化

使用 Playwright 和本机 Google Chrome，验证 `/admin` 管理后台与权限驱动 UI。

## 前置条件

- 后端运行在 `http://127.0.0.1:18080`，PostgreSQL/Redis 已启动并完成 Seed。
- 默认测试账号：
  - 管理员：`m2badmin` / `correct-horse`
  - 受限账号：`m2bsolver1` / `correct-horse`
- 本机安装 Google Chrome（默认通过 `channel: chrome` 使用系统浏览器）。
  如需使用其它 Playwright channel，设置 `E2E_BROWSER_CHANNEL`。

## 运行

```bash
# 仓库根目录
corepack pnpm test:e2e
```

Playwright 会在需要时启动前端 dev server（默认 `http://127.0.0.1:5173`，代理到
`:18080`），并复用已运行的服务。后端未就绪时 `global-setup.ts` 会直接给出提示。

## 环境变量

| 变量                                        | 默认值                         | 说明                                  |
| ------------------------------------------- | ------------------------------ | ------------------------------------- |
| `E2E_API_URL`                               | `http://127.0.0.1:18080`       | 后端地址                              |
| `E2E_BASE_URL`                              | `http://127.0.0.1:5173`        | 前端地址；设置后不自动启动 dev server |
| `E2E_BROWSER_CHANNEL`                       | `chrome`                       | Playwright 浏览器 channel             |
| `E2E_ADMIN_USER` / `E2E_ADMIN_PASSWORD`     | `m2badmin` / `correct-horse`   | 超管账号                              |
| `E2E_LIMITED_USER` / `E2E_LIMITED_PASSWORD` | `m2bsolver1` / `correct-horse` | 受限账号                              |
| `E2E_LIMITED_ROLE`                          | `auditor`                      | 测试期间临时分配给受限账号的内置角色  |

## 测试行为

- 超管：登录、仪表盘、用户编辑预填、角色权限矩阵、审计页、游戏化页、移动端 Drawer。
- 受限账号：临时分配 `auditor`，验证只读操作、管理菜单过滤、游戏化路由跳转 403，
  测试结束后恢复原角色。
- 该测试会通过真实 API 修改受限账号角色，因此会产生 `user.roles.assign` 审计记录；
  角色分配会恢复，审计记录保留作为测试痕迹。
- 失败时在 `apps/web/test-results/` 保留 trace/截图/视频，HTML 报告在
  `apps/web/playwright-report/`。
