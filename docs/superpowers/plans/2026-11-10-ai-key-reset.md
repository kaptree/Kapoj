# AI 加密密钥一键重置实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在后台 AI 设置页增加「清除旧密钥并生成新密钥」：删除所有用户 AI 配置和旧平台密钥，生成并返回一个随机新密钥，便于旧密钥丢失后的恢复。

**Architecture:** 后端新增 `SettingsService.ResetKey` 和 `POST /admin/ai/key/reset`，在 `keyMu` 锁内先写新密钥文件（备份旧文件），再在同一 DB 事务内删除 `user_ai_configs`、更新指纹、写审计；失败恢复旧文件。前端在密钥操作区增加危险色按钮和二次确认，复用现有一次性密钥展示区。

**Tech Stack:** Go + Gin + GORM + PostgreSQL；Vue 3 + PrimeVue 4；OpenAPI 生成前端类型；Vitest。

**Spec:** `docs/superpowers/specs/2026-11-10-ai-key-reset-design.md`

## Global Constraints

- 仅 `ai.settings.manage` 权限可调用；环境变量来源 `KAPOJ_AI_ENCRYPTION_KEY` 只读，返回 40001。
- 重置必须删除**所有** `user_ai_configs`，不保留旧密文。
- 新密钥为 32 字节随机数 + base64；文件 0600、父目录 0700。
- 不修改 `ai_settings.enabled`，不清空 `ai_hints`。
- 密钥文件写入、用户配置删除、指纹更新、审计必须在一次操作中完成；任一步失败恢复旧密钥文件并回滚 DB。
- 完整新密钥只在响应中一次性返回，不写数据库、不写日志、不写审计。
- 前端按钮只在 `canManage && !isEnvSource` 时显示；点击必须二次确认，明确“删除所有用户 AI Key、不可撤销”。
- 复用现有 `AIKeyGenerateResult` 类型和一次性密钥展示/复制/隐藏逻辑。
- 不新增权限、不改 AI provider 列表或用户侧 AI 设置页。
- 后端测试使用 `go test -p 1`；前端跑 focused spec、全量测试、lint、build。
- 不提交 `Export.zip` 或 `.superpowers` 临时文件。

## Review Focus

- 旧密钥丢失但仍有用户 AI 配置时，重置必须成功并清空这些配置（Task 1）。
- 环境变量来源绝不能删除用户配置或覆盖密钥（Task 1）。
- 审计/DB 失败时，旧密钥文件与用户配置必须完整恢复；不能留下半重置状态（Task 1）。
- 新密钥不得泄露到数据库、审计或日志；只在响应中一次性返回（Task 1、Task 2）。
- 前端取消确认不得调用 API；确认后必须展示一次性密钥并提示用户需重新配置（Task 3）。

---

### Task 1: 后端重置服务、路由与测试

**Files:**
- Modify: `apps/server/internal/ai/key_service.go`
- Modify: `apps/server/internal/ai/key_service_test.go`
- Modify: `apps/server/internal/adminapi/ai_handler.go`
- Modify: `apps/server/internal/adminapi/role_handler.go`
- Modify: `apps/server/internal/adminapi/ai_handler_test.go`

**Interfaces:**
- Produces:
  - `func (s *SettingsService) ResetKey(ctx context.Context, actorID int64, audit func(*gorm.DB) error) (*KeyGenerateResult, error)`
  - `POST /api/admin/ai/key/reset`
  - audit summary `清除旧密钥并生成新密钥`，target `ai_key`

- [ ] **Step 1: Write failing service tests**

在 `apps/server/internal/ai/key_service_test.go` 添加：

1. `TestKeyResetPurgesUserConfigs`
   - 写旧 key 文件，构造一条用旧 key 加密的 `UserConfig`。
   - `ResetKey(ctx, 42, nil)`。
   - 断言返回 key 解码为 32 字节。
   - `user_ai_configs` count = 0。
   - 文件内容为新 key，`.bak` 不存在。
   - `KeyView` configured/file，fingerprint 等于 `sha256Hex(newKey)`。
   - `ai_settings.key_fingerprint` 等于新指纹，`enabled` 保持原值。
2. `TestKeyResetWithoutOldKey`
   - 无 key 文件，但存在一条任意密文 `UserConfig`。
   - `ResetKey` 成功，配置被删除，文件创建，内存密钥切换。
3. `TestKeyResetEnvReadOnly`
   - env key 来源；`ResetKey` 返回 40001。
   - 用户配置数量、文件/内存指纹不变。
4. `TestKeyResetAuditFailureRestores`
   - 旧 key 文件 + 一条用户配置。
   - `ResetKey` 的 audit 回调返回错误。
   - 返回错误；用户配置仍存在；旧 key 文件内容恢复；`.bak` 不存在；内存指纹仍为旧 key。

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj/apps/server
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
  go test -p 1 -count=1 ./internal/ai/
```

Expected: FAIL，`ResetKey` 未定义。

- [ ] **Step 3: Implement service**

`ai/key_service.go`：

- `GenerateKey` / `SetKey` 改为调用带模式参数的共享 `rotate`：
  ```go
  type keyRotationMode bool
  const (
      keepUserConfigs keyRotationMode = true
      purgeUserConfigs keyRotationMode = false
  )
  ```
  或等价的私有参数；不要导出。
- `rotate(ctx, actorID, raw, audit, mode)`：
  - env 检查、keyFile 检查、校验密钥不变。
  - 仅 `keepUserConfigs` 执行 `cfgCount > 0 && !hasOld` 拒绝。
  - 文件写入/备份不变。
  - DB 事务内：
    - `ensureSettings`。
    - `keepUserConfigs` → `reencryptConfigs`。
    - `purgeUserConfigs` → `tx.Where("1 = 1").Delete(&UserConfig{})`。
    - 更新指纹/updated_by/updated_at。
    - audit。
  - 失败 `restoreKeyFile`；成功 `removeKeyBackup` + `keys.Replace`。
- 新增：
  ```go
  // ResetKey 删除所有用户 AI 配置并用新的随机密钥替换平台密钥。
  // 用于旧密钥丢失后的恢复；不保留旧密文。
  func (s *SettingsService) ResetKey(ctx context.Context, actorID int64, audit func(*gorm.DB) error) (*KeyGenerateResult, error) {
      raw, err := generateKeyBase64()
      if err != nil {
          return nil, err
      }
      view, err := s.rotate(ctx, actorID, raw, audit, purgeUserConfigs)
      if err != nil {
          return nil, err
      }
      return &KeyGenerateResult{KeyView: *view, Key: raw}, nil
  }
  ```

`adminapi/ai_handler.go`：

```go
func (h *handler) resetAIKey(c *gin.Context) {
	actor := auditActor(c)
	result, err := h.ai.ResetKey(c.Request.Context(), actor.ActorID, func(tx *gorm.DB) error {
		return rbac.WriteAuditTx(
			tx, actor,
			rbac.AuditActionAISettingsManage,
			rbac.AuditTargetAIKey, 0,
			"清除旧密钥并生成新密钥",
		)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}
```

`adminapi/role_handler.go`：

```go
api.POST("/admin/ai/key/reset", append(requirePermission(s, string(rbac.PermAISettingsManage)), h.resetAIKey)...)
```

- [ ] **Step 4: Add failing handler tests**

在 `apps/server/internal/adminapi/ai_handler_test.go` 添加：

- `TestAdminAIKeyResetAndAudit`：
  - 用 `newAIKeyFixture(t, "")`；创建一条 `ai.UserConfig`。
  - 先 `POST /api/admin/ai/key/generate` 生成旧 key（或直接写文件），再创建配置。
  - `POST /api/admin/ai/key/reset` 用 super token → 200；`data.key` 为 32 字节 base64；`configured=true/source=file`。
  - `user_ai_configs` count = 0。
  - 审计 action `ai.settings.manage`、target `ai_key`、summary 含「清除旧密钥并生成新密钥」，且不含 key。
- `TestAdminAIKeyResetEnvReadonly`：
  - env fixture，`POST reset` → 40001；用户配置仍存在。
- `TestAdminAIKeyResetPermission`：
  - 未登录 401；content_admin 403；super 200。

- [ ] **Step 5: Run tests to verify they fail then pass**

```bash
cd /Users/admin/codespace/kapoj/apps/server
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
  go test -p 1 -count=1 ./internal/ai/ ./internal/adminapi/
```

Expected: 先 FAIL（路由 404/ResetKey 未定义），实现后 PASS。

- [ ] **Step 6: Commit**

```bash
git add apps/server/internal/ai/key_service.go apps/server/internal/ai/key_service_test.go apps/server/internal/adminapi/ai_handler.go apps/server/internal/adminapi/role_handler.go apps/server/internal/adminapi/ai_handler_test.go
git commit -m "feat(ai): add destructive key reset"
```

---

### Task 2: OpenAPI、生成类型与前端客户端

**Files:**
- Modify: `packages/api/openapi.yaml`
- Generated: `apps/web/src/api/schema.d.ts`
- Modify: `apps/web/src/api/admin.ts`
- Test: `apps/web/src/api/admin.spec.ts`（新建）

**Interfaces:**
- Consumes: Task 1 的 `POST /api/admin/ai/key/reset` 响应 `AIKeyGenerateResult`。
- Produces:
  - `resetAdminAIKey(): Promise<AIKeyGenerateResult>`
  - OpenAPI path `/admin/ai/key/reset`

- [ ] **Step 1: Write failing client test**

新建 `apps/web/src/api/admin.spec.ts`：

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'

const http = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  delete: vi.fn(),
  postForm: vi.fn(),
  putForm: vi.fn(),
}))

vi.mock('./client', () => ({ http }))

import { resetAdminAIKey } from './admin'

describe('admin api', () => {
  beforeEach(() => {
    Object.values(http).forEach((fn) => fn.mockReset())
  })

  it('posts to the AI key reset endpoint', async () => {
    http.post.mockResolvedValue({
      configured: true,
      source: 'file',
      masked: 'abc…wxyz',
      fingerprint: 'fp',
      key: 'BASE64SECRET',
    })

    await expect(resetAdminAIKey()).resolves.toMatchObject({ key: 'BASE64SECRET' })
    expect(http.post).toHaveBeenCalledWith('/admin/ai/key/reset')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/api/admin.spec.ts
```

Expected: FAIL，`resetAdminAIKey` 未导出。

- [ ] **Step 3: Implement contract/client**

`packages/api/openapi.yaml`：

- 在 `/admin/ai/key/generate` 后新增：
  ```yaml
  /admin/ai/key/reset:
    post:
      summary: 清除旧密钥并生成新密钥（ai.settings.manage）
      description: >-
        删除所有用户已保存的 AI 配置（provider/model/API Key），替换平台加密密钥并生成
        32 字节随机新密钥。旧密钥丢失后的恢复入口；不可撤销，用户需重新填写 API Key。
        环境变量来源时返回 40001。完整密钥仅在响应中一次性返回。
      security: [{ bearer: [] }]
      responses:
        '200':
          description: 重置后的密钥状态与一次性完整密钥
          content:
            application/json:
              schema: { $ref: '#/components/schemas/AIKeyGenerateResultResponse' }
        '400': { $ref: '#/components/responses/Error' }
        '401': { $ref: '#/components/responses/Error' }
        '403': { $ref: '#/components/responses/Error' }
        '500': { $ref: '#/components/responses/Error' }
  ```

`apps/web/src/api/admin.ts`：

```ts
/** Delete all user AI configs and replace the platform key with a new random key (ai.settings.manage). */
export const resetAdminAIKey = () =>
  http.post<AIKeyGenerateResult>('/admin/ai/key/reset')
```

重新生成：

```bash
corepack pnpm gen:api
```

- [ ] **Step 4: Run test and typecheck**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/api/admin.spec.ts
corepack pnpm --filter web exec vue-tsc -b --force
```

Expected: PASS / 类型检查通过。

- [ ] **Step 5: Commit**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts apps/web/src/api/admin.ts apps/web/src/api/admin.spec.ts
git commit -m "feat(api): add AI key reset contract"
```

---

### Task 3: 后台 AI 设置页重置按钮与确认

**Files:**
- Modify: `apps/web/src/views/AdminAiView.vue`
- Modify: `apps/web/src/views/AdminAiView.spec.ts`

**Interfaces:**
- Consumes: `resetAdminAIKey()`、现有 `confirm`、`toast`、`keySaving`、`generatedKey`。
- Produces:
  - 危险按钮「清除旧密钥并生成新密钥」
  - 确认弹窗文案
  - 成功后一次性密钥展示 + Toast

- [ ] **Step 1: Write failing UI tests**

在 `AdminAiView.spec.ts`：

- 在 mock 中加入 `resetAdminAIKey: vi.fn()`，`beforeEach` reset。
- 添加：
  1. `shows the destructive reset action for file/none sources`
     - `canManage=true`，keyView source file → 按钮存在；source none 也存在。
  2. `hides the reset action for env source`
     - source env → 按钮不存在。
  3. `confirms before resetting and shows the one-time key`
     - source file；`resetAdminAIKey` resolve `generateResult({ key: 'NEWSECRET' })`。
     - 点击按钮 → `confirm.require` 1 次，API 未调用。
     - 执行 `options.accept()` → API 调用 1 次；`input[readonly].value === 'NEWSECRET'`；Toast summary「旧密钥已清除」；`getAdminAISettings` 再次调用。
  4. `cancels reset without calling the API`
     - 点击后不 accept；API 未调用。
  5. `shows an error toast when reset fails`
     - reject `ApiError(500, 50000, '重置失败')`；Toast severity error，detail 含「重置失败」。
- 现有 env/自动生成/手动填写测试保持通过。

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/views/AdminAiView.spec.ts
```

Expected: FAIL，按钮不存在。

- [ ] **Step 3: Implement UI**

`AdminAiView.vue`：

- import `resetAdminAIKey`。
- 增加：
  ```ts
  function confirmResetKey() {
    confirm.require({
      header: '清除旧密钥并生成新密钥',
      message: '将删除所有用户已保存的 AI API Key，并生成新的平台加密密钥。此操作不可撤销，用户需要重新填写自己的 API Key。',
      icon: 'pi pi-exclamation-triangle',
      acceptLabel: '清除并生成',
      rejectLabel: '取消',
      accept: () => void doResetKey(),
    })
  }

  async function doResetKey() { ... }
  ```
- `doResetKey`：
  - `keySaving = true`
  - `const result = await resetAdminAIKey()`
  - `key.value = result`、`generatedKey.value = result.key`、`manualKey.value = ''`
  - `await refreshSettings()`
  - Toast success summary「旧密钥已清除」，detail「新密钥已生成；请立即备份，用户需重新填写 AI API Key」
  - 失败 Toast summary「重置失败」，detail 使用 `ApiError.message`
  - finally `keySaving = false`
- 模板 key-actions 内、现有生成按钮后：
  ```vue
  <Button
    type="button"
    label="清除旧密钥并生成新密钥"
    icon="pi pi-trash"
    severity="danger"
    outlined
    :loading="keySaving"
    data-test="reset-ai-key"
    @click="confirmResetKey"
  />
  ```
- 加一行小提示：
  「旧密钥丢失时使用此操作；会删除所有用户已保存的 AI Key。」
- CSS：`.key-actions { flex-wrap: wrap; }`；移动端按钮 `width: 100%`。

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/views/AdminAiView.spec.ts
corepack pnpm --filter web test
corepack pnpm --filter web lint
corepack pnpm --filter web build
```

Expected: 全部通过。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/AdminAiView.vue apps/web/src/views/AdminAiView.spec.ts
git commit -m "feat(web): add AI key reset action"
```

---

### Task 4: 全量验证、真实冒烟与文档同步

**Files:**
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`（当前进度一行，可选）

- [ ] **Step 1: Backend checks**

```bash
cd /Users/admin/codespace/kapoj/apps/server
test -z "$(gofmt -l .)"
go vet ./...
/private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
  go test -p 1 -count=1 ./...
```

Expected: 全部通过。

- [ ] **Step 2: Frontend checks**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm gen:api
corepack pnpm --filter web test
corepack pnpm --filter web lint
corepack pnpm --filter web build
```

Expected: 全部通过。

- [ ] **Step 3: Isolated real smoke**

使用隔离临时库/端口，不动主服务：

1. 临时 server + 临时 storage；创建 super 用户并生成一个旧密钥。
2. 插入一条用户 AI 配置（用旧密钥加密）。
3. 调用 `POST /api/admin/ai/key/reset`：
   - 200，返回 32 字节 base64 新 key。
   - `user_ai_configs` count = 0。
   - 旧 `.bak` 不存在，文件内容为新 key。
   - 审计 summary「清除旧密钥并生成新密钥」，不含 key。
   - 再用新 key 调用 `GET /api/ai/settings`（或无用户配置时应正常返回 keyConfigured=true）。
4. env 来源临时 server：reset 返回 40001，用户配置不变。
5. 清理临时服务/数据库/Redis/storage。

- [ ] **Step 4: Browser smoke（有浏览器时）**

- 管理员打开 `/admin/ai`：密钥区显示「清除旧密钥并生成新密钥」。
- 点击出现确认；取消不发请求。
- 确认后显示一次性新密钥和成功 Toast，密钥区状态更新。
- env 来源不显示危险按钮（可用单测覆盖，浏览器不强制）。
- 若浏览器不可用，记录「浏览器冒烟延后」，不声称已验证。

- [ ] **Step 5: Update docs**

- `docs/任务清单.md`：新增/更新「跨里程碑：AI 加密密钥一键重置 ✅」，记录交付摘要与测试结果。
- `docs/开发日志.md`：追加 `2026-11-10：AI 加密密钥一键重置`，含接口、破坏性语义、测试与冒烟。
- `README.md`：当前进度补一行（可选）。

- [ ] **Step 6: Commit**

```bash
git add docs/任务清单.md docs/开发日志.md README.md
git commit -m "docs: record AI key reset delivery"
```

---

## Self-Review

- **Spec coverage:** 后端重置/删除配置（Task 1）、契约/客户端（Task 2）、前端按钮/确认/一次性密钥（Task 3）、验证/文档（Task 4）。
- **Type consistency:** `ResetKey`、`POST /admin/ai/key/reset`、`resetAdminAIKey`、`AIKeyGenerateResult`、`data-test="reset-ai-key"` 在任务间一致。
- **Review Focus mapping:** 旧密钥丢失恢复（Task 1）、env 只读（Task 1）、失败回滚（Task 1）、密钥不泄露（Task 1/2）、取消不调用/一次性展示（Task 3）。
- **Proportion:** 任务按可独立验收的交付物拆分；计划给出签名、关键语义和测试断言，不逐行抄写实现。
