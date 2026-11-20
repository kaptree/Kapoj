# 用户 CSV 批量导入设计

> 日期：2026-11-20
> 状态：待评审
> 关联：`hydro-users.csv`、`docs/持续开发需求.md`、`docs/任务清单.md`

## 1. 目标

在管理后台 `/admin/users` 增加「导入用户」功能，读取 HydroOJ 导出的用户 CSV，批量创建本地用户。

首期只处理当前这份 `hydro-users.csv` 的格式：

```csv
_id,uname,mail,hash,salt,hashType
2,Kaptree,1196991321@qq.com,2df6...,A6bda...,hydro
```

导入后：

- 用户可以用旧 HydroOJ 密码登录（后端已兼容 HydroOJ PBKDF2 哈希）。
- 不需要给 2418 个用户逐个发验证邮件；导入时直接标记邮箱已验证。
- 旧用户名尽量保留；不符合当前用户名规则时生成稳定、可登录的 ASCII 用户名。
- 已存在的账号默认跳过，不修改密码、邮箱、角色或任何现有数据。
- 导入前可预览，导入后能看到成功/跳过/失败统计和逐行原因。
- 导入对话框内置「CSV 格式说明」按钮，展开后可查看字段规则和可直接参考的样例。
- 预览/结果页提供「下载账号映射 CSV」，只导出被生成 `hydro_<id>` 的用户，方便管理员通知新登录名。
- 不把 CSV 中的密码哈希、邮箱写进日志，也不提交真实 CSV 到仓库。

## 2. 数据勘察

当前 `hydro-users.csv`：

| 项目 | 结果 |
| --- | --- |
| 数据行 | 2418 |
| 编码 | UTF-8 BOM |
| 列 | `_id,uname,mail,hash,salt,hashType` |
| `hashType` | 全部 `hydro` |
| 邮箱 | 全部通过 `ValidateEmail`，无重复 |
| 旧用户名长度 | 2–21 字符 |
| 合法 ASCII 用户名 | 1659 |
| 不合法用户名 | 759（中文、空格、连字符、点、2 字符等） |
| 昵称 | 全部 1–32 字符，无控制字符 |
| 哈希/盐 | 全部 64 位十六进制 / 32 字符 |
| 与现有库用户名冲突 | 2：`Kaptree`、`test` |
| 与现有库邮箱冲突 | 0 |

按本设计预览的预期结果（以当前库为准）：

```text
总数 2418 / 可导入 2416 / 冲突 2 / 无效 0 / 生成用户名 759
```

`Kaptree` 与 `test` 会被跳过，现有账号保持不变。

## 3. 非目标

- 不覆盖或合并现有账号（包括不替换 `kaptree` 的密码）。
- 不导入角色、金币、Rating、提交、比赛、学习进度；新用户统一为普通用户、1500 分、0 金币。
- 不保留 Hydro 旧 `_id`；不新增 `legacy_id` 字段。
- 不做异步任务、后台队列、分批进度条；2418 行同步处理即可。
- 不支持非 UTF-8、非 Hydro 格式、超过 5000 行的 CSV。
- 不发送欢迎邮件、不触发密码重置。
- 不导出完整 2418 行映射；映射 CSV 只包含生成用户名的用户，不含 `hash`/`salt`。
- 不在前端编辑 CSV 行或逐条修改导入数据。

## 4. 关键决策

### 4.1 用户名映射

1. 对 `uname` 做 `TrimSpace` 和 `strings.ToLower`。
2. 如果结果匹配当前规则 `^[a-z0-9_]{3,32}$`，保留为登录用户名。
3. 否则生成 `hydro_<旧ID>`（例如 `hydro_2`）。
4. 生成名若与现有用户或本批其他用户名冲突，追加 `_2`、`_3`，直到唯一；**生成名冲突只改名，不跳过该用户**。
5. 只有「原始合法用户名」或邮箱与现有账号冲突时才跳过整行；非法用户名生成 `hydro_<id>` 的过程不会导致跳过。
6. `nickname` 始终保存原始 `uname`（去首尾空格），中文、空格、大小写都保留。

示例：

| 旧 uname | 登录用户名 | 昵称 |
| --- | --- | --- |
| `Kaptree` | `kaptree`（冲突则跳过） | `Kaptree` |
| `让我睡觉` | `hydro_6` | `让我睡觉` |
| `Ninler耀` | `hydro_10` | `Ninler耀` |
| `a b` | `hydro_123` | `a b` |
| `ab` | `hydro_456` | `ab` |
| `already_valid` | `already_valid` | `already_valid` |

### 4.2 冲突处理

- 只支持 `skip`：用户名或邮箱已存在时跳过整行，并在预览/结果中报告原因。
- 不修改现有账号的任何字段。
- 本批 CSV 内部的重复 `_id`、邮箱、用户名，**所有出现的行都标记为无效**，不保留第一条；避免不确定地选择某一行导入。

### 4.3 密码与邮箱验证

- 直接写入 CSV 的 `PasswordHash` 和 `Salt`，不重新哈希；用户继续使用旧密码。
- `hash` 统一转小写后再存。
- 导入时设置 `EmailVerifiedAt = now()`，避免历史用户被邮箱验证流程拦住。
- 不生成随机密码，不要求首次登录改密。
- 后续如需要强制重置，可单独做管理员操作，不在本期范围。

### 4.4 权限

新增权限码：

```text
user.import  批量导入用户
```

- 分组：用户与角色。
- 默认只有 `super_admin` 可用（走 super 绕过）；其他角色需要管理员在 RBAC 权限矩阵里显式授予。
- 导入接口**同时要求 `user.read` 和 `user.import`**：预览/映射响应包含邮箱，只有 `user.import` 的角色返回 403；只有 `user.read` 也不能导入。super_admin 不受影响。
- 前端 `PERM.user.import` 新增同名键。
- 后端路由使用 `RequirePermission(rbac.PermUserImport)`。
- 审计 action 使用 `user.import`，只记录统计数字，不记录邮箱、哈希、CSV 内容。

### 4.5 无数据库迁移

不新增列、不新增表。权限目录通过现有 RBAC seed 自动补齐；导入用户复用 `users` 表。

## 5. 方案对比

### A. 同步预览 + 同步提交（推荐）

- `POST /api/admin/users/import?dryRun=true` 只解析、校验、查冲突，不写库。
- `POST /api/admin/users/import` 重新解析并写入可导入行。
- 前端先预览，确认后再提交同一文件。
- 优点：写库前能看到冲突/生成用户名；实现简单；2418 行一次请求即可。
- 缺点：文件上传两次（约 321KB，可忽略）。

### B. 单步上传即导入

- 上传后直接写库，只在结果里报告。
- 优点：少一步。
- 缺点：管理员无法在写库前检查冲突和生成用户名；不采用。

### C. 异步批次表 + 后台任务

- 上传后落批次表和临时文件，Asynq 后台处理，前端轮询进度。
- 优点：适合十万级用户。
- 缺点：多一套表、队列、清理和进度 UI；当前 2418 行不需要。

采用 A。

## 6. 后端设计

### 6.1 包结构

新增：

```text
apps/server/internal/userimport/
  csv.go        # CSV 解析、行校验、用户名生成
  service.go    # 预览、冲突检查、批量写入
  csv_test.go
  service_test.go
```

修改：

```text
apps/server/internal/adminapi/role_handler.go   # 注册路由
apps/server/internal/adminapi/user_import_handler.go  # multipart + 权限
apps/server/internal/rbac/catalog.go           # user.import
apps/server/internal/rbac/users.go             # 审计常量（如需要）
```

`userimport` 直接使用 `*gorm.DB` 做冲突查询和批量插入，和现有 `rbac` 管理服务保持一致；解析和校验复用 `internal/user` 的 `ValidateEmail`/`ValidateUsername`/`ValidateNickname`，密码哈希复用 `internal/password` 的格式约定（只校验格式，不重新计算）。

### 6.2 CSV 解析

限制：

- 最大 5 MiB。
- 最多 5000 个数据行。
- 仅 UTF-8，允许 BOM。
- 表头必须包含 `_id,uname,mail,hash,salt,hashType`；大小写不敏感，允许额外列。
- 空白行跳过。

逐行规则：

| 字段 | 规则 |
| --- | --- |
| `_id` | 正整数；同一 CSV 内唯一 |
| `uname` | trim 后 1–32 个字符；无 NUL/控制字符；作为昵称 |
| `mail` | trim + 小写；`ValidateEmail`；同一 CSV 内唯一 |
| `hash` | 64 位十六进制；转小写 |
| `salt` | 32 个字符 |
| `hashType` | 大小写不敏感等于 `hydro` |

任意字段失败 → 该行 `invalid`，保留行号、旧 ID、原始 `uname`、邮箱和中文原因，不导入。

### 6.3 用户名与冲突解析

解析顺序：

1. 先收集所有显式合法用户名（规范化后），建立保留集合。
2. 再处理非法用户名，生成 `hydro_<id>` 并处理保留集合冲突。
3. 查询现有用户：按 500 个一批，使用 `LOWER(username) IN (...)` 与 `LOWER(email) IN (...)`。
4. 对每行标记：
   - `valid`：可导入，带最终用户名和 `generated` 标志；
   - `conflict`：用户名或邮箱已存在；
   - `invalid`：格式错误。

### 6.4 写入

- 只写入 `valid` 行。
- 每行构造 `user.User`：
  - `Username`：最终用户名（小写）
  - `Nickname`：原始 `uname`
  - `Email`：规范化邮箱
  - `EmailVerifiedAt`：`now`
  - `PasswordHash` / `Salt`：CSV 原值（hash 小写）
  - `Role = user.RoleUser`
  - `Status = user.StatusActive`
  - `Rating = user.InitialRating`
  - `Coins = 0`
- 在一个事务内按 200 行一批插入，使用 `ON CONFLICT DO NOTHING`。
- 正常情况 `imported = RowsAffected`；跳过的冲突行 = 预览冲突 + 写入时唯一冲突。
- 非唯一冲突等意外数据库错误：回滚全部，返回 500。
- 重复导入同一文件时，所有行都会因用户名/邮箱已存在被跳过，不会产生重复用户。

### 6.5 接口

```text
POST /api/admin/users/import?dryRun=true|false
Authorization: Bearer <admin>
Content-Type: multipart/form-data
file: <hydro-users.csv>
```

- `dryRun=true`：只预览。
- `dryRun` 缺省为 `false`（提交）。
- 需要 `user.import` 权限。

响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "dryRun": true,
    "summary": {
      "total": 2418,
      "valid": 2416,
      "conflicts": 2,
      "invalid": 0,
      "generatedUsernames": 759,
      "imported": 0,
      "skipped": 0,
      "failed": 0
    },
    "issues": [
      {
        "line": 2,
        "legacyId": 2,
        "uname": "Kaptree",
        "email": "1196991321@qq.com",
        "status": "conflict",
        "reason": "用户名已存在"
      }
    ],
    "issuesTruncated": false,
    "mapping": [
      {
        "legacyId": 6,
        "uname": "让我睡觉",
        "email": "553352330@qq.com",
        "username": "hydro_6"
      }
    ],
    "samples": [
      {
        "line": 3,
        "legacyId": 3,
        "uname": "Moira",
        "email": "1420048209@qq.com",
        "username": "moira",
        "generated": false
      },
      {
        "line": 6,
        "legacyId": 6,
        "uname": "让我睡觉",
        "email": "553352330@qq.com",
        "username": "hydro_6",
        "generated": true
      }
    ]
  }
}
```

- `issues` 最多返回 500 条，`issuesTruncated=true` 表示还有更多。
- `mapping` 只包含最终 `generated=true` 且可导入的行；预览时用于通知，提交时返回实际导入的生成用户名行。最多 5000 条。
- `samples` 最多 20 条可导入行，包含生成用户名的样例。
- 响应绝不包含 `hash`、`salt`。
- 提交时 `summary.imported/skipped/failed` 为实际结果；`valid` 仍表示预览阶段可导入数。

错误：

- 40001：CSV 表头错误、编码错误、超过大小/行数、空文件、格式错误。
- 403：无 `user.import` 权限。
- 500：数据库写入失败。

### 6.6 审计

成功或部分成功后写一条审计：

```text
action: user.import
targetType: user
targetId: 0
summary: 导入用户：成功 2416，跳过 2，失败 0
```

审计内容不包含邮箱、用户名列表、哈希或文件内容。

## 7. 前端设计

### 7.1 入口

`/admin/users` 页面标题区增加「导入用户」按钮：

- 仅 `auth.can(PERM.user.import)` 时显示。
- 点击打开 `AdminUserImportDialog`。

### 7.2 对话框流程

1. **格式说明**
   - 对话框顶部提供「CSV 格式说明」按钮（`data-test="import-help-toggle"`）。
   - 点击展开内联说明面板，按钮 `aria-expanded` 同步；再次点击收起。
   - 面板包含支持字段、字段规则、文件限制和一段样例 CSV；不调用后端。
   - 说明面板默认收起，不能遮挡文件选择和预览结果。

2. **选择文件**
   - 原生文件选择，接受 `.csv`。
   - 显示文件名和大小。
   - 客户端先检查 ≤5 MiB；不通过则直接提示。

3. **预览**
   - 调用 `POST /admin/users/import?dryRun=true`。
   - 展示统计卡片：总数、可导入、冲突、无效、生成用户名。
   - 展示冲突/无效表格：行号、旧 ID、原用户名、邮箱、原因。
   - 展示可导入样例表格：最终用户名、昵称、是否生成。
   - 当 `mapping.length > 0` 时显示「下载账号映射 CSV」按钮。
   - 提示：「已存在的账号会跳过，不会修改密码或邮箱。」

4. **确认导入**
   - 只在 `valid > 0` 时可用。
   - 调用 `POST /admin/users/import`。
   - 导入中禁用关闭/重复点击。

5. **结果**
   - 展示成功、跳过、失败数。
   - 成功后触发父页面刷新用户列表并 toast。

### 7.3 CSV 格式说明面板

说明面板固定包含以下内容，避免用户去翻文档：

**支持字段**

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `_id` | 是 | Hydro 旧用户 ID，正整数，同一文件内唯一；用于生成 `hydro_<id>` |
| `uname` | 是 | 原用户名，1–32 字符；作为昵称；合法 ASCII 用户名保留为登录名，否则生成 `hydro_<id>` |
| `mail` | 是 | 邮箱，同一文件内唯一；登录时也可以使用邮箱 |
| `hash` | 是 | 64 位十六进制 HydroOJ 密码哈希 |
| `salt` | 是 | 32 字符密码盐 |
| `hashType` | 是 | 目前仅支持 `hydro` |

**文件限制**

- UTF-8（允许 BOM），最大 5 MiB，最多 5000 个数据行。
- 表头必须包含上述 6 列；列顺序不限，额外列会忽略。
- 已存在的用户名或邮箱会跳过，不修改现有账号。
- 不展示、不导出 `hash`/`salt`。

**样例 CSV**

```csv
_id,uname,mail,hash,salt,hashType
1001,alice,alice@example.com,c519b45b4e118f223b8edfa027e6b07a3446fa506e634ff3f71608fb6abf5d17,aB3xY9mKpQ2rStUvWxYz0123456789AB,hydro
1002,小助手,xiaozhushou@example.com,c519b45b4e118f223b8edfa027e6b07a3446fa506e634ff3f71608fb6abf5d17,aB3xY9mKpQ2rStUvWxYz0123456789AB,hydro
```

样例行中 `hash`/`salt` 对应演示密码 `HelloWorld`；真实导入请使用 HydroOJ 导出的值。`小助手` 这类非 ASCII 用户名会生成 `hydro_1002`，昵称仍为 `小助手`。

### 7.4 文件与隐私

- 前端不解析 CSV 内容，只上传文件。
- 不展示 `hash`/`salt`。
- 预览表格中的邮箱只给有权限的管理员看。
- 不把 CSV 内容写入 localStorage。

### 7.5 账号映射下载

- 只在存在生成用户名时显示按钮；预览和导入结果都可下载。
- 文件由前端根据 API 返回的 `mapping` 生成，不额外请求后端。
- 列固定为：

```csv
_id,uname,mail,username
6,让我睡觉,553352330@qq.com,hydro_6
```

- `_id` 是旧 Hydro ID；`uname` 是原用户名/昵称；`mail` 是邮箱；`username` 是新登录名。
- 使用标准 CSV 转义（逗号、双引号、换行）；UTF-8 BOM 可选，默认不加。
- 只包含生成用户名的可导入行；不包含 `hash`/`salt`，不包含未改名用户。
- 以 `=`, `+`, `-`, `@` 开头的 `uname` 前加 `'`，防止表格软件把旧用户名当公式执行。
- 不写入 localStorage；下载后由管理员自行保管。

## 8. 权限与安全

- 新权限 `user.import`，默认仅 super admin。
- 上传大小硬限制 5 MiB；行数硬限制 5000。
- 仅接受 UTF-8；拒绝 NUL 和控制字符。
- 不记录 CSV 原文、密码哈希、盐、邮箱到日志。
- 真实 `hydro-users.csv` 加入 `.gitignore`，不提交仓库。
- 测试使用合成 CSV，不使用真实用户数据。
- 导入接口只允许 POST，沿用现有 Bearer 鉴权与 RBAC。

## 9. 测试计划

### 9.1 后端

- 解析：BOM、表头顺序、额外列、空行、CRLF、非 UTF-8、超限、字段缺失。
- 校验：非法邮箱、非法 `_id`、重复 `_id`/邮箱/用户名、非法 hash/salt/hashType。
- 用户名：合法保留、小写化、中文/空格/连字符/点/2 字符生成 `hydro_<id>`；生成名与现有账号或同批用户名冲突时追加 `_2` 而不是跳过。
- 重复行：CSV 内重复 `_id`/邮箱/用户名时，所有出现的行都判为无效。
- 冲突：现有用户名冲突、现有邮箱冲突、大小写不敏感冲突。
- 写入：hash/salt 原样保留、邮箱已验证、角色/评级/金币默认、`ON CONFLICT` 幂等。
- 登录：用合成的 Hydro 哈希导入后，旧密码可以登录。
- 权限：无 `user.import` 返回 403。
- 审计：成功导入后写入一条 `user.import` 审计。
- 真实 CSV 预览（手动/临时库）：预期 2418/2416/2/0/759。

### 9.2 前端

- 有权限显示导入按钮，无权限隐藏。
- 点击「CSV 格式说明」展开字段表、文件限制和样例，再点收起；不触发任何 API。
- 文件大小/扩展名校验。
- 预览统计、冲突表、生成用户名样例。
- 生成用户名时出现「下载账号映射 CSV」；无生成用户名时不显示。
- 下载内容只包含生成用户名的可导入行，CSV 转义正确，不含 hash/salt。
- 可导入为 0 时禁用确认。
- 提交成功后 emit 并刷新列表。
- 请求失败显示中文错误，不泄露原始响应。

### 9.3 可选 E2E

管理员登录 → 打开导入对话框 → 上传合成 CSV → 预览 → 确认 → 列表出现新用户。使用临时测试用户并在结束后清理。

## 10. 涉及文件

后端：

- `apps/server/internal/userimport/{csv.go,service.go,csv_test.go,service_test.go}`
- `apps/server/internal/adminapi/user_import_handler.go`
- `apps/server/internal/adminapi/role_handler.go`
- `apps/server/internal/rbac/catalog.go`
- `apps/server/internal/rbac/users.go`（如新增审计常量）

前端：

- `apps/web/src/components/admin/AdminUserImportDialog.vue`（新建）
- `apps/web/src/components/admin/AdminUserImportDialog.spec.ts`
- `apps/web/src/views/AdminUsersView.vue`
- `apps/web/src/views/AdminUsersView.spec.ts`
- `apps/web/src/api/admin.ts`
- `apps/web/src/permissions.ts`

契约/文档：

- `packages/api/openapi.yaml`
- `apps/web/src/api/schema.d.ts`
- `.gitignore`（`hydro-users.csv`）
- `docs/开发日志.md`
- `docs/任务清单.md`（可选）

## 11. 实施顺序

1. 后端解析/校验/用户名生成 + 单元测试。
2. 冲突检查与批量写入 + 集成测试（含旧密码登录）。
3. HTTP multipart、权限、审计、OpenAPI。
4. 前端 API、导入对话框、AdminUsersView 集成。
5. 前端测试、构建、lint。
6. 用真实 CSV 做一次 preview（不提交）；在临时库做一次完整导入验证。
7. 文档、`.gitignore`、全量验证、提交。

## 12. 待确认

1. 是否同意「保留旧密码 + 导入时标记邮箱已验证」？（推荐）
2. 是否同意 `user.import` 默认只给 super admin，其他角色按需在 RBAC 里授予？（推荐）
3. 是否同意先做预览 + 提交两步，且冲突只跳过、不覆盖？（已按你的选择设计）
4. 是否同意不保留旧 `_id`、不加数据库字段？（已按你的选择设计）
5. 账号映射只导出生成 `hydro_<id>` 的用户，不导出完整 2418 行。（已确认）

如果以上没有异议，下一步会写实施计划并开始编码。
