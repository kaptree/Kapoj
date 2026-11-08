# AI 加密密钥管理设计（管理后台）

> 目标：管理后台可以直接自动生成或手动填写 `KAPOJ_AI_ENCRYPTION_KEY`，写入服务端本地密钥文件，运行时立即生效；已有密钥轮换时自动重加密所有用户 AI Key。环境变量密钥优先且后台只读。

## 1. 背景与现状

- M5-A 的 AI 加密密钥来自环境变量 `KAPOJ_AI_ENCRYPTION_KEY`；未配置时 AI 路由不挂载（404）。
- 管理后台已有 AI 开关（`ai_settings.enabled`、`ai.settings.manage`），但无法管理密钥；用户必须改环境变量并重启。
- 用户 API Key 以 AES-256-GCM 加密存于 `user_ai_configs.api_key_encrypted`；根密钥丢失/更换会导致历史密文无法解密。
- 用户要求：后台可自动/手动生成并填充密钥，作为管理后台板块；选择服务端本地密钥文件持久化；已有密钥轮换时自动重加密用户 Key。

## 2. 目标与非目标

### 目标

- 新增密钥文件配置 `KAPOJ_AI_KEY_FILE`（默认 `data/ai.key`，权限 0600）。
- 管理后台 `/admin/ai` 增加“加密密钥”板块：
  - 显示状态：未配置 / 已配置（文件）/ 由环境变量管理；脱敏值/指纹。
  - 自动生成：`crypto/rand` 32 字节 → base64；保存后立即生效；首次生成后一次性显示完整密钥供备份。
  - 手动填写：粘贴 base64 32 字节密钥并保存；校验格式/长度。
  - 已有密钥时允许重新生成/替换：自动用旧密钥解密、新密钥重新加密所有 `user_ai_configs`；事务失败回滚。
  - 环境变量 `KAPOJ_AI_ENCRYPTION_KEY` 存在时，后台只读显示“由环境变量管理”，生成/手动保存返回 40001。
- 运行时切换：无需重启；新密钥立即用于后续加密/解密。
- 未配置密钥时：
  - AI 路由仍可访问 `GET /ai/configs`（返回 `keyConfigured=false`、providers）；
  - 用户保存配置/AI 调用返回 40001“AI 加密密钥未配置”；
  - 提交页/用户 AI 设置页显示“管理员尚未配置 AI 密钥”，不显示“AI 功能未启用”（后者仅用于管理开关关闭）。
- 默认 `ai_settings.enabled=true`；密钥配置后 AI 立即可用（仍受开关控制）。

### 非目标

- 不把根密钥存进数据库；不通过浏览器长期展示/回显完整密钥（仅生成后一次性显示）。
- 不做 KMS/HSM/云 Secret Manager 集成；不做多密钥版本并存（仅当前+备份文件）。
- 不改变用户 API Key 的 AES-GCM 格式（仍 `v1:`+base64 nonce+ciphertext）。
- 不做按用户/按 provider 的密钥；不改变 provider seed、限流、提示缓存。
- 不做密钥丢失后的用户 Key 恢复（无旧密钥则无法解密；后台拒绝破坏性轮换并提示恢复旧密钥）。

## 3. 密钥文件与配置

```text
KAPOJ_AI_KEY_FILE  可选，默认 data/ai.key
KAPOJ_AI_ENCRYPTION_KEY 可选，优先级高于文件
```

- 文件内容：单行 base64 编码的 32 字节密钥（`openssl rand -base64 32` 等价）；允许末尾换行。
- 文件权限：`0600`；父目录不存在时 `0700` 创建。
- 读取：
  - env 非空：校验 base64/32 字节；非法则启动失败（保持现有语义）；来源 `env`；忽略文件。
  - env 空：尝试读取文件；不存在 → 未配置；格式非法 → 记录错误日志并视为未配置（后台可覆盖）；文件不可读 → 启动失败或未配置？选择启动失败（配置错误需修复），但后台无法修复不可读文件；记录错误并视为未配置，后台可写覆盖。
- 写入：临时文件 `ai.key.tmp`（0600）→ `os.Rename` 原子替换；轮换时旧文件备份 `ai.key.bak`，成功删除备份，失败恢复。
- 不把密钥写入日志/审计/API 响应（生成接口除外，一次性返回）。

## 4. 运行时架构

### 4.1 KeyManager

- `ai.KeyManager`（`sync.RWMutex`）：
  - `Configured() bool`
  - `Source() string`（`env`/`file`/`none`）
  - `Masked() string`（前 3…后 4，长度≤8 为 `****`）
  - `Fingerprint() string`（SHA-256 hex）
  - `Encrypt/Decrypt`（无密钥返回 `httpx.ErrValidation("AI 加密密钥未配置")`）
  - `Replace(enc *Encryptor, source string)`（轮换成功后在内存切换）
- `ai.Service` 改为持有 `*KeyManager`，不再持有单个 `*Encryptor`；所有 Encrypt/Decrypt 走 KeyManager。
- `main`：
  - 解析 `AIEncryptionKey`/`AIKeyFile`；构造 KeyManager（env 优先/文件）。
  - **始终构造 AI Service**（不再以密钥是否存在决定是否挂载）；`d.AI` 非 nil。
  - `/api/ai/*` 路由始终挂载，`RequireAIEnabled` 只检查管理开关。
- `ConfigView` 新增 `keyConfigured bool`；`GetConfig` 返回；`SaveConfig` 无密钥返回 40001。
- `aiapi` 的 complete/format/optimize/hint 无密钥返回 40001（hint 缓存命中仍可返回）。
- `ai_settings.keyConfigured` 改为 `KeyManager.Configured()` 动态判断，不再用 `d.AI==nil`。
- 管理开关启用时要求 `keyConfigured=true`（保存时检查当前 KeyManager）。

### 4.2 管理密钥服务

- `ai.KeyService{db, km, filePath}`：
  - `View() KeyView{configured, source, masked, fingerprint}`。
  - `Generate(ctx, actorID) (*KeyView, generatedKey string, error)`。
  - `SetManual(ctx, actorID, rawKey string) (*KeyView, error)`。
  - `Rotate(ctx, actorID, newEnc) error`：
    1. 若 source=env → 40001。
    2. 若无旧密钥但存在 `user_ai_configs` 行 → 40001“存在无法解密的用户 AI 配置，请先恢复旧密钥”；无旧密钥且无配置 → 直接保存。
    3. 若有旧密钥：事务内 `SELECT ... FOR UPDATE` 所有 `user_ai_configs`，逐行旧解密/新加密；任一失败回滚。
    4. 写新密钥文件（备份旧文件）；事务提交；更新 `ai_settings.key_fingerprint`（同事务或紧随其后，失败则回滚文件/恢复备份）。
    5. `KeyManager.Replace(newEnc, "file")`；删除备份；审计（不含密钥）。
  - 密钥文件写入失败 → 回滚 DB；DB 失败 → 恢复备份文件。
- 管理 API：
  - `GET /api/admin/ai/key`（`ai.settings.manage`）→ KeyView。
  - `POST /api/admin/ai/key/generate`（同权限）→ `{...KeyView, key}`；已有密钥时轮换。
  - `PUT /api/admin/ai/key`（同权限）`{key}` → KeyView；已有密钥时轮换。
  - 环境变量来源时 POST/PUT → 40001。
  - 审计 action `ai.settings.manage`，target `ai_key`，targetID 0，summary“生成/更新 AI 加密密钥（指纹 …）”；不写密钥/完整指纹？指纹可写（不可逆），或只写“已更新”。建议只写“已生成/更新 AI 加密密钥”，不写指纹。
- 管理开关与密钥：
  - 关闭 AI 不删除密钥/用户配置；重新开启恢复。
  - 未配置密钥时允许关闭；启用返回 40001。

## 5. 数据模型

```text
ai_settings (
  ... existing ...
  key_fingerprint text not null default ''  -- 仅诊断/一致性，不存密钥
)
```

- 迁移新增列；密钥本身只在文件。
- 轮换事务更新 `key_fingerprint`；启动时若文件密钥指纹与 DB 不一致，记录警告（不自动选旧密钥）；后台显示指纹，管理员可恢复备份。
- 不做密钥版本表。

## 6. 前端

### 6.1 管理后台 `/admin/ai`

在现有开关下方新增“加密密钥”`AppSurface`：
- 状态 Tag：未配置 / 已配置（文件）/ 由环境变量管理；显示 masked 与 fingerprint。
- 未配置：
  - `自动生成密钥` 按钮（生成 32 字节 base64）；
  - `手动填写` Password/Textarea + 保存；
  - 生成后一次性显示完整密钥（只读输入 + 复制按钮 + “请立即备份，丢失后用户 AI Key 无法解密”警告）；刷新后只显示 masked。
- 已有密钥（文件）：
  - 显示 masked/fingerprint；
  - `重新生成` / `手动替换`（二次确认）；保存后自动重加密用户 Key；
  - 若存在用户配置且旧密钥不可用，后端 40001，前端提示恢复旧密钥。
- 环境变量：只读，隐蔽生成/手动按钮，文案“由 KAPOJ_AI_ENCRYPTION_KEY 管理”。
- 密钥操作成功 Toast；失败中文；审计由后端记录。
- 类型/OpenAPI：`AIKeyView`、`AIKeyGenerateResult`、`AIKeyInput`；`AIConfigView.keyConfigured`。

### 6.2 用户侧

- `GET /ai/configs` 返回 `keyConfigured`；`AIConfigView` 增加字段。
- 用户 AI 设置页（`/settings/ai`）：`keyConfigured=false` 且 AI 已启用时显示“管理员尚未配置 AI 加密密钥，请联系管理员”，表单禁用；AI 路由关闭（404）仍显示“AI 功能未启用”。
- 提交页 AI 工具栏 banner：
  - 404/开关关闭 → “AI 功能未启用”；
  - keyConfigured=false → “管理员尚未配置 AI 密钥”；
  - 未配置用户 key → 现有“尚未配置 AI 模型”+ 用户设置链接。
- 题目 AI 提示：无密钥且无缓存 → “管理员尚未配置 AI 密钥”；缓存命中仍展示。

## 7. 权限与安全

- 复用 `ai.settings.manage`（第 36 权限），默认仅超管；密钥与开关同一权限。
- 密钥只写文件（0600）；不进 DB/日志/审计/普通 API；生成接口一次性返回。
- 手动输入校验 base64/32 字节；错误不泄露其他信息。
- 轮换事务与文件备份；失败回滚；无旧密钥时拒绝破坏性覆盖。
- 环境变量来源禁止后台修改，避免双源冲突。
- 用户 API Key 仍只返回脱敏值；密钥管理不改变用户配置权限。

## 8. 测试与验收

### 8.1 后端

- KeyManager：env 优先、文件读取、缺失、非法、masked/fingerprint、Replace 并发安全。
- KeyService：生成/手动/环境只读；首次保存；轮换重加密（成功、解密失败回滚、DB 失败恢复文件、文件写失败回滚）；无旧密钥但有用户配置 40001；审计。
- AI 路由：无密钥 `GET /ai/configs` 200 keyConfigured=false；保存/complete/format/optimize 40001；hint 缓存可用；有密钥正常；开关关闭 404。
- 管理 API：权限/401/403、GET/POST/PUT、审计；开关启用要求 key。
- DB：`ai_settings.key_fingerprint` 列；迁移。
- 权限 36（已有）。

### 8.2 前端

- 管理页密钥板块：未配置/已配置/env；生成一次性显示/复制；手动；轮换确认；错误；开关与密钥联动。
- 用户 AI 设置/提交/提示的 keyConfigured 状态与 404 状态区分。
- 生成类型/lint/build。

### 8.3 真实冒烟

- 无 env/无文件：服务启动、AI 路由 200 keyConfigured=false；后台自动生成 → 文件 0600、AI 可用；用户配置/补全正常。
- 手动填写合法/非法；env 来源只读；开关关闭→404；重新开启恢复。
- 已有用户配置轮换：旧 key 加密的配置在新 key 后仍能使用（解密成功）；旧 key 不再可用；备份清理。
- 无旧密钥但有用户配置：轮换被拒 40001。
- 清理临时密钥文件/用户配置/审计；恢复原密钥/环境；不删除既有数据。

## 9. 交付拆分

- KEY-A：KeyManager/配置/文件读写 + AI Service 重构 + 无密钥语义 + 开关联动。
- KEY-B：KeyService 轮换/重加密/备份 + 管理 API + 审计。
- KEY-C：OpenAPI/生成类型。
- KEY-D：管理页密钥板块 + 用户侧 keyConfigured 状态。
- KEY-E：文档、全量检查、真实冒烟。

## 10. 留给实施计划的决定

- KeyManager/KeyService 文件拆分与构造函数签名。
- 密钥文件默认路径与 `StorageDir` 的关系（默认 `data/ai.key`；允许 env 覆盖）。
- 轮换的备份文件命名/恢复细节。
- 生成后完整密钥的展示组件（只读输入/复制按钮）。
