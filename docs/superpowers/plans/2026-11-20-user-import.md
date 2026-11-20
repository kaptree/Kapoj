# HydroOJ 用户 CSV 批量导入 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在管理后台按 HydroOJ CSV 批量创建用户，保留旧密码哈希、预览冲突、跳过现有账号，并提供格式说明和改名用户映射下载。

**Architecture:** 新增 `internal/userimport` 领域包负责 CSV 解析、用户名映射、冲突检查和事务写入；`adminapi` 只做 multipart、权限和审计；前端在 `/admin/users` 打开导入对话框，预览/提交同一文件，格式说明和账号映射都在前端完成。

**Tech Stack:** Go 1.26、Gin、GORM/PostgreSQL、HydroOJ PBKDF2 密码；Vue 3.5、PrimeVue 4.5、Pinia、Vitest、OpenAPI TypeScript 生成。

**Spec:** `docs/superpowers/specs/2026-11-20-user-import-design.md`

## Global Constraints

- 仅接受 UTF-8（允许 BOM）；CSV 最大 **5 MiB**、最多 **5000** 个数据行。
- 表头必须包含 `_id,uname,mail,hash,salt,hashType`；列顺序不限，额外列忽略，空行跳过。
- `_id` 正整数且同文件唯一；`mail` 合法、小写、同文件唯一；`hash` 64 位十六进制；`salt` 32 字符；`hashType` 仅支持 `hydro`。
- 合法 ASCII 用户名保留（转小写）；不合法用户名生成 `hydro_<旧ID>`；生成名冲突追加 `_2`/`_3`，不跳过用户。
- 只有原始合法用户名或邮箱与现有账号冲突时才跳过；不修改现有账号任何字段。
- CSV 内重复的 `_id`/邮箱/用户名，所有出现的行都无效。
- 直接保存 `PasswordHash` + `Salt`，不重新哈希；导入时 `EmailVerifiedAt = now()`。
- 导入用户 `Role=user`、`Status=active`、`Rating=1500`、`Coins=0`；不导入角色/金币/Rating。
- 不新增数据库列/表；不保留旧 `_id`。
- 新权限码 `user.import`，默认仅 super_admin；不隐含 `user.read`。
- 响应、日志、审计不包含 `hash`/`salt`；审计只记录成功/跳过/失败计数。
- 真实 `hydro-users.csv` 不提交仓库，必须加入 `.gitignore`；测试只用合成 CSV。
- 预览/结果接口最多返回 500 条 issues、20 条 samples；`mapping` 只含 `generated=true` 且可导入/已导入的行，最多 5000 条。
- 后端测试使用 `KAPOJ_TEST_DATABASE_URL` 指向测试库，`-p 1`；前端命令在 `apps/web`。

## Pre-flight

Before Task 1, commit the approved design and this plan:

```bash
git add docs/superpowers/specs/2026-11-20-user-import-design.md docs/superpowers/plans/2026-11-20-user-import.md
git commit -m "docs(userimport): add design and implementation plan"
```

## Review Focus

1. **现有账号绝不被覆盖**：`Kaptree`/`test` 冲突必须跳过，原密码、邮箱、角色、金币保持不变。Task 2 的 `TestProcessSkipsExistingUser` 固定。
2. **旧密码连续可用**：导入后 `password.Verify` 必须能用旧密码通过，且 `EmailVerifiedAt` 非空。Task 2 的 `TestProcessPreviewAndImport` 固定。
3. **用户名映射与冲突**：759 个非法用户名生成 `hydro_<id>`；与现有 `hydro_6` 或同批显式名冲突时追加后缀，昵称保留原文。Task 1/2 固定。
4. **坏 CSV 不产生半成品**：错误表头、非 UTF-8、超大小/行数、重复行返回明确错误或逐行无效，提交不写入任何无效/冲突行。Task 1/3 固定。
5. **权限与 PII**：无 `user.import` 返回 403；响应和审计不出现 hash/salt/邮箱明细。Task 3 固定。

---

### Task 1: CSV 解析、校验与用户名映射

**Files:**
- Create: `apps/server/internal/userimport/csv.go`
- Create: `apps/server/internal/userimport/csv_test.go`

**Interfaces:**
- Consumes: `user.Normalize`, `user.ValidateEmail`, `user.ValidateUsername`, `user.ValidateNickname`, `httpx.ErrValidation`.
- Produces:
  ```go
  const (
      maxCSVBytes = 5 << 20
      maxCSVRows  = 5000
  )

  type rowStatus string
  const (
      rowValid    rowStatus = "valid"
      rowConflict rowStatus = "conflict"
      rowInvalid  rowStatus = "invalid"
  )

  type row struct {
      line     int
      legacyID int64
      uname    string
      email    string
      hash     string
      salt     string
      hashType string
      username string
      generated bool
      status   rowStatus
      reason   string
  }

  func parseHydroCSV(r io.Reader) ([]*row, error)
  func applyUsernames(rows []*row, taken map[string]struct{})
  ```

- [ ] **Step 1: Write the failing parser/mapping tests**

Use `package userimport` (internal test) so `row` and unexported functions are visible. Use a helper:

```go
const (
    testHash = "c519b45b4e118f223b8edfa027e6b07a3446fa506e634ff3f71608fb6abf5d17"
    testSalt = "aB3xY9mKpQ2rStUvWxYz0123456789AB"
)

func csvText(lines ...string) string {
    return "_id,uname,mail,hash,salt,hashType\n" + strings.Join(lines, "\n") + "\n"
}
```

Tests to write:

```go
func TestParseHydroCSVValidRowAndBOM(t *testing.T) {
    rows, err := parseHydroCSV(strings.NewReader("\ufeff" + csvText(
        "2,Alice,Alice@Example.com,"+testHash+","+testSalt+",hydro",
    )))
    require.NoError(t, err)
    require.Len(t, rows, 1)
    assert.Equal(t, int64(2), rows[0].legacyID)
    assert.Equal(t, "Alice", rows[0].uname)
    assert.Equal(t, "alice@example.com", rows[0].email)
    assert.Equal(t, rowValid, rows[0].status)
}

func TestParseHydroCSVInvalidRows(t *testing.T) {
    // 每个 case 断言 status=rowInvalid 且 reason 非空。
    cases := []string{
        "x,Alice,a@example.com," + testHash + "," + testSalt + ",hydro",       // _id
        "2,,a@example.com," + testHash + "," + testSalt + ",hydro",            // uname 空
        "2,Alice,not-an-email," + testHash + "," + testSalt + ",hydro",        // mail
        "2,Alice,a@example.com,short," + testSalt + ",hydro",                  // hash
        "2,Alice,a@example.com," + testHash + ",short,hydro",                  // salt
        "2,Alice,a@example.com," + testHash + "," + testSalt + ",bcrypt",      // hashType
        "2,Alice\u0001,a@example.com," + testHash + "," + testSalt + ",hydro", // control
    }
}

func TestParseHydroCSVDuplicateRowsAllInvalid(t *testing.T) {
    // 两行同 email：两行都必须 invalid，reason 包含“重复”。
}

func TestParseHydroCSVRejectsBadInput(t *testing.T) {
    // 空文件、缺列、非 UTF-8、超过 5000 行、超过 5 MiB 都返回 40001 AppError。
}

func TestApplyUsernames(t *testing.T) {
    rows := []*row{
        {line: 2, legacyID: 2, uname: "Alice", status: rowValid},
        {line: 3, legacyID: 6, uname: "让我睡觉", status: rowValid},
        {line: 4, legacyID: 7, uname: "hydro_7", status: rowValid},
        {line: 5, legacyID: 8, uname: "a b", status: rowValid},
        {line: 6, legacyID: 9, uname: "ab", status: rowValid},
    }
    taken := map[string]struct{}{"hydro_9": {}}
    applyUsernames(rows, taken)

    assert.Equal(t, "alice", rows[0].username)
    assert.False(t, rows[0].generated)
    assert.Equal(t, "hydro_6", rows[1].username)
    assert.True(t, rows[1].generated)
    assert.Equal(t, "hydro_7", rows[2].username) // 显式合法名优先保留
    assert.Equal(t, "hydro_8", rows[3].username)
    assert.Equal(t, "hydro_9_2", rows[4].username) // 生成名冲突追加后缀
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/server && go test ./internal/userimport -run 'TestParse|TestApply' -count=1`
Expected: FAIL because `userimport` / `parseHydroCSV` / `applyUsernames` do not exist.

- [ ] **Step 3: Implement `csv.go`**

Implementation requirements:

- `readCSV`: `io.LimitReader(r, maxCSVBytes+1)`; reject if `len(data) > maxCSVBytes` with `httpx.ErrValidation("CSV 不能超过 5 MiB")`.
- Strip UTF-8 BOM; reject `!utf8.Valid(data)` with a 40001 error.
- `csv.NewReader(bytes.NewReader(data))` with `FieldsPerRecord = -1`, `ReuseRecord = false`.
- Read header, trim/lower keys, require all six columns; missing → 40001.
- Iterate records; skip rows where every field is empty; reject more than `maxCSVRows` with 40001.
- Build each `row`: trim fields; parse `_id` with `strconv.ParseInt`; validate `uname` length 1–32 + no `unicode.IsControl`; `user.Normalize` email + `ValidateEmail`; lowercase hash and validate 64 hex chars; salt len 32; `strings.EqualFold(hashType, "hydro")`.
- First invalid field sets `status=rowInvalid` and a specific Chinese `reason`; keep raw values for the issue row.
- After parsing, detect duplicates among valid rows for `_id`, email, and normalized valid username (`user.Normalize(uname)` + `ValidateUsername == nil`); mark **all** rows in each duplicate group invalid with reason containing “重复”.
- `applyUsernames`:
  - First pass over `status==rowValid`: if normalized `uname` passes `ValidateUsername`, set `username`, `generated=false`, add to `taken`.
  - Second pass over `status==rowValid` without `username`: base `hydro_<legacyID>`; while `taken[candidate]` exists append `_2`, `_3`…; set `username`, `generated=true`, add to `taken`.
  - Do not assign usernames to invalid rows.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd apps/server && go test ./internal/userimport -run 'TestParse|TestApply' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/userimport/csv.go apps/server/internal/userimport/csv_test.go
git commit -m "feat(userimport): parse and map HydroOJ user CSV"
```

---

### Task 2: 冲突检查、预览与事务导入

**Files:**
- Create: `apps/server/internal/userimport/service.go`
- Create: `apps/server/internal/userimport/service_test.go`

**Interfaces:**
- Consumes: Task 1 `parseHydroCSV`, `applyUsernames`, `row`; `user.User`, `user.InitialRating`, `password.Verify`.
- Produces:
  ```go
  type Summary struct {
      Total int `json:"total"`
      Valid int `json:"valid"`
      Conflicts int `json:"conflicts"`
      Invalid int `json:"invalid"`
      GeneratedUsernames int `json:"generatedUsernames"`
      Imported int `json:"imported"`
      Skipped int `json:"skipped"`
      Failed int `json:"failed"`
  }

  type Issue struct {
      Line int `json:"line"`
      LegacyID int64 `json:"legacyId"`
      Uname string `json:"uname"`
      Email string `json:"email"`
      Status string `json:"status"`
      Reason string `json:"reason"`
  }

  type Mapping struct {
      LegacyID int64 `json:"legacyId"`
      Uname string `json:"uname"`
      Email string `json:"email"`
      Username string `json:"username"`
  }

  type Sample struct {
      Line int `json:"line"`
      LegacyID int64 `json:"legacyId"`
      Uname string `json:"uname"`
      Email string `json:"email"`
      Username string `json:"username"`
      Generated bool `json:"generated"`
  }

  type Result struct {
      DryRun bool `json:"dryRun"`
      Summary Summary `json:"summary"`
      Issues []Issue `json:"issues"`
      IssuesTruncated bool `json:"issuesTruncated"`
      Mapping []Mapping `json:"mapping"`
      Samples []Sample `json:"samples"`
  }

  type AuditFunc func(tx *gorm.DB, summary Summary) error

  type Service struct{ db *gorm.DB }
  func NewService(db *gorm.DB) *Service
  func (s *Service) Process(ctx context.Context, r io.Reader, dryRun bool, audit AuditFunc) (*Result, error)
  ```

- [ ] **Step 1: Write the failing service tests**

Use `testutil.PG(t)` and a helper:

```go
func seedUser(t *testing.T, g *gorm.DB, username, email, hash, salt string) *user.User {
    t.Helper()
    u := &user.User{
        Username: username, Nickname: username, Email: email,
        PasswordHash: hash, Salt: salt, Role: user.RoleUser, Status: user.StatusActive,
    }
    require.NoError(t, g.Create(u).Error)
    return u
}

func mustUser(t *testing.T, g *gorm.DB, username string) *user.User {
    t.Helper()
    var u user.User
    require.NoError(t, g.Where("username = ?", username).First(&u).Error)
    return &u
}
```

Tests:

```go
func TestProcessPreviewAndImport(t *testing.T) {
    g := testutil.PG(t)
    svc := NewService(g)
    hash, salt, err := password.Hash("old-password")
    require.NoError(t, err)
    csv := csvText(
        "2,Alice,Alice@Example.com,"+hash+","+salt+",hydro",
        "6,让我睡觉,me@example.com,"+hash+","+salt+",hydro",
    )

    preview, err := svc.Process(context.Background(), strings.NewReader(csv), true, nil)
    require.NoError(t, err)
    assert.Equal(t, 2, preview.Summary.Total)
    assert.Equal(t, 2, preview.Summary.Valid)
    assert.Equal(t, 1, preview.Summary.GeneratedUsernames)
    require.Len(t, preview.Mapping, 1)
    assert.Equal(t, "hydro_6", preview.Mapping[0].Username)
    var count int64
    require.NoError(t, g.Model(&user.User{}).Count(&count).Error)
    assert.Zero(t, count, "preview must not write")

    result, err := svc.Process(context.Background(), strings.NewReader(csv), false, nil)
    require.NoError(t, err)
    assert.Equal(t, 2, result.Summary.Imported)

    var alice user.User
    require.NoError(t, g.Where("username = ?", "alice").First(&alice).Error)
    assert.Equal(t, "Alice", alice.Nickname)
    assert.NotNil(t, alice.EmailVerifiedAt)
    assert.Equal(t, user.InitialRating, alice.Rating)
    assert.Zero(t, alice.Coins)
    assert.True(t, password.Verify("old-password", alice.Salt, alice.PasswordHash))
    assert.Equal(t, "hydro_6", mustUser(t, g, "hydro_6").Username)
}

func TestProcessSkipsExistingUser(t *testing.T) {
    // seed kaptree with a known hash; import CSV row Kaptree + a new user.
    // assert conflict=1, imported=1, kaptree hash unchanged, kaptree email unchanged.
}

func TestProcessGeneratedNameCollision(t *testing.T) {
    // seed hydro_6; import row _id=6 uname=让我睡觉; assert username hydro_6_2.
}

func TestProcessDuplicateRowsAllInvalid(t *testing.T) {
    // two rows same email; valid=0, invalid=2, no users written even on commit.
}

func TestProcessEmailConflict(t *testing.T) {
    // seed a@example.com under another username; row with new username + same email -> conflict.
}

func TestProcessSecondImportIsIdempotent(t *testing.T) {
    // commit twice; second imported=0, skipped=2, users count still 2.
}

func TestProcessAuditFailureRollsBack(t *testing.T) {
    // audit returns errors.New("audit"); commit returns error and users count 0.
}

func TestProcessMappingOnlyImportedGeneratedRows(t *testing.T) {
    // generated row + generated conflict; mapping contains only the imported one.
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL=... go test ./internal/userimport -run 'TestProcess' -count=1 -p 1`
Expected: FAIL because `NewService`/`Process` do not exist.

- [ ] **Step 3: Implement `service.go`**

Implementation requirements:

- `Process`:
  1. `rows, err := parseHydroCSV(r)`; return error as-is.
  2. Query existing names/emails:
     - Explicit valid usernames from `row.uname` (normalized + `ValidateUsername`).
     - Query explicit names in chunks of 500 with `LOWER(username) IN ?`.
     - Query all names matching `LOWER(username) LIKE 'hydro%'` (covers generated base and suffix candidates).
     - Query emails in chunks with `LOWER(email) IN ?`.
     - Store lowercased in `map[string]struct{}`.
  3. `applyUsernames(rows, existingNames)`.
  4. Mark conflicts on `status==rowValid`: explicit username in existing names, or email in existing emails. Set `rowConflict` + reason; count.
  5. Build `Result`:
     - `Summary.Total`, `Valid`, `Conflicts`, `Invalid`.
     - `Issues` = invalid/conflict rows, capped at 500, `IssuesTruncated`.
     - `Samples` = first 20 valid rows, including final username/generated.
     - `Mapping` = valid generated rows.
     - `Summary.GeneratedUsernames = len(Mapping)`.
  6. If `dryRun`, return.
  7. Commit in a transaction:
     - `now := time.Now().UTC()`.
     - For each valid row: create `user.User` with fields from the design; `tx.Clauses(clause.OnConflict{DoNothing: true}).Create(u)`.
     - `RowsAffected == 0` → `Summary.Skipped++`, append a conflict issue (respect 500 cap).
     - `RowsAffected == 1` → `Summary.Imported++`; if generated append mapping.
     - Call `audit(tx, Summary)` after inserts; return its error.
     - Unexpected error → rollback (return error).
- Use `gorm.io/gorm/clause`.
- `Issues`/`Samples`/`Mapping` must be initialized as empty slices, not nil, so JSON is `[]`.
- `Summary.Skipped` starts with conflict count; `Summary.Failed` stays 0 (unexpected DB error aborts).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL=... go test ./internal/userimport -count=1 -p 1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/userimport/service.go apps/server/internal/userimport/service_test.go
git commit -m "feat(userimport): preview and import users transactionally"
```

---

### Task 3: 管理 API、权限、审计与 OpenAPI

**Files:**
- Create: `apps/server/internal/adminapi/user_import_handler.go`
- Create: `apps/server/internal/adminapi/user_import_handler_test.go`
- Modify: `apps/server/internal/adminapi/role_handler.go`
- Modify: `apps/server/internal/adminapi/role_handler_test.go` (fixture)
- Modify: `apps/server/internal/rbac/catalog.go`
- Modify: `apps/server/internal/rbac/audit.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`

**Interfaces:**
- Consumes: Task 2 `userimport.NewService`, `Process`, `Summary`; `rbac.WriteAuditTx`.
- Produces:
  ```go
  const PermUserImport PermissionCode = "user.import"
  const AuditActionUserImport = "user.import"

  func RegisterUserImportRoute(api gin.IRouter, s *rbac.Service, svc *userimport.Service)
  func (h *handler) importUsers(c *gin.Context)
  ```
- `handler` struct adds `userImport *userimport.Service`.

- [ ] **Step 1: Write the failing handler/permission tests**

Add a multipart helper to `role_handler_test.go`:

```go
func (f *fixture) multipart(path, token, filename string, data []byte) (*httptest.ResponseRecorder, map[string]any) {
    var buf bytes.Buffer
    mw := multipart.NewWriter(&buf)
    part, err := mw.CreateFormFile("file", filename)
    require.NoError(f.t, err)
    _, err = part.Write(data)
    require.NoError(f.t, err)
    require.NoError(f.t, mw.Close())
    req := httptest.NewRequest(http.MethodPost, path, &buf)
    req.Header.Set("Content-Type", mw.FormDataContentType())
    req.Header.Set("User-Agent", testUserAgent)
    req.RemoteAddr = "203.0.113.10:1234"
    if token != "" { req.Header.Set("Authorization", "Bearer "+token) }
    w := httptest.NewRecorder()
    f.engine.ServeHTTP(w, req)
    var resp map[string]any
    _ = json.Unmarshal(w.Body.Bytes(), &resp)
    return w, resp
}
```

`newFixture` adds:

```go
adminapi.RegisterUserImportRoute(r.Group("/api"), svc, userimport.NewService(g))
```

Tests:

```go
func TestAdminUserImportRequiresPermission(t *testing.T) {
    // plain user -> 403; super -> 200 for dryRun=true.
}

func TestAdminUserImportPreviewAndCommit(t *testing.T) {
    // super token; hash/salt from password.Hash("old-password").
    // dryRun=true returns summary.valid=1; DB count unchanged.
    // dryRun=false creates user; response summary.imported=1.
    // response body must not contain hash/salt.
    // query rbac.AdminAuditLog action=user.import; summary contains 成功 1; no email/hash.
}

func TestAdminUserImportRejectsBadCSV(t *testing.T) {
    // missing header -> 400; non-UTF8 -> 400; oversize -> 400/413.
}

func TestAdminUserImportRouteNotShadowedByUserID(t *testing.T) {
    // POST /api/admin/users/import must not hit /admin/users/:id.
}
```

Also add a catalog assertion:

```go
func TestPermissionCatalogIncludesUserImport(t *testing.T) {
    // rbac.AllPermissions() contains code user.import.
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL=... go test ./internal/adminapi ./internal/rbac -run 'UserImport|UserImportPermission' -count=1 -p 1`
Expected: FAIL because the route/handler/permission do not exist.

- [ ] **Step 3: Implement the handler and route**

`user_import_handler.go`:

- `RegisterUserImportRoute` constructs `&handler{svc: s, userImport: svc}` and registers:
  ```go
  api.POST("/admin/users/import", append(requirePermission(s, string(rbac.PermUserImport)), h.importUsers)...)
  ```
- `importUsers`:
  - `c.Request.ParseMultipartForm(1 << 20)`; on `*http.MaxBytesError` return `httpx.ErrPayloadTooLarge`, else `httpx.ErrBadRequest`; defer `MultipartForm.RemoveAll()`.
  - `c.FormFile("file")`; missing → 40001 “缺少文件字段 file”.
  - `fh.Size > 5<<20` → 40001 “CSV 不能超过 5 MiB”.
  - Open file; `dryRun := strings.EqualFold(c.Query("dryRun"), "true")`.
  - `actor := auditActor(c)`.
  - `result, err := h.userImport.Process(ctx, f, dryRun, func(tx *gorm.DB, s userimport.Summary) error { ... })`; only pass audit when `!dryRun`.
  - Audit summary: `fmt.Sprintf("导入用户：成功 %d，跳过 %d，失败 %d", s.Imported, s.Skipped, s.Failed)`.
  - `httpx.OK(c, result)`.

`role_handler.go`:
- Add `userImport *userimport.Service` to `handler`.
- `RegisterRoutes` leaves the rest unchanged.
- `RegisterUserImportRoute` is a separate function.

`rbac/catalog.go`:
- Add `PermUserImport`.
- Add catalog entry after `PermUserRead`:
  ```go
  {Code: PermUserImport, Name: "批量导入用户", GroupName: PermissionGroupUser, Description: "通过 HydroOJ CSV 批量创建用户"},
  ```

`rbac/audit.go`:
- Add `AuditActionUserImport = "user.import"`.

`router/router.go`:
- Import `userimport`.
- Inside `if d.RBAC != nil`, call:
  ```go
  adminapi.RegisterUserImportRoute(uploads, d.RBAC, userimport.NewService(d.DB))
  ```
  before `adminapi.RegisterRoutes(...)`.
- The `uploads` group already allows 40 MiB; the handler enforces 5 MiB.

- [ ] **Step 4: Add OpenAPI and regenerate types**

Add `POST /admin/users/import` with `dryRun` query and multipart `file`; schemas `AdminUserImportSummary`, `AdminUserImportIssue`, `AdminUserImportMapping`, `AdminUserImportSample`, `AdminUserImportResult`, `AdminUserImportResponse`. Required arrays and lowerCamel fields exactly match the Go JSON tags. No `hash`/`salt` fields.

Run:

```bash
cd packages/api && npx openapi-typescript openapi.yaml -o ../../apps/web/src/api/schema.d.ts
```

- [ ] **Step 5: Run the tests to verify they pass**

Run:

```bash
cd apps/server
KAPOJ_TEST_DATABASE_URL=... go test ./internal/userimport ./internal/adminapi ./internal/rbac ./internal/router -count=1 -p 1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/server/internal/adminapi apps/server/internal/rbac apps/server/internal/router apps/server/internal/userimport packages/api apps/web/src/api/schema.d.ts
git commit -m "feat(admin): add user CSV import API"
```

---

### Task 4: 前端导入对话框、格式说明与映射下载

**Files:**
- Modify: `apps/web/src/api/admin.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/permissions.ts`
- Modify: `apps/web/src/permissions.spec.ts`
- Create: `apps/web/src/utils/userImport.ts`
- Create: `apps/web/src/utils/userImport.spec.ts`
- Create: `apps/web/src/components/admin/AdminUserImportDialog.vue`
- Create: `apps/web/src/components/admin/AdminUserImportDialog.spec.ts`

**Interfaces:**
- Consumes: generated `AdminUserImportResult`, `AdminUserImportIssue`, `AdminUserImportMapping`; `http.postForm`.
- Produces:
  ```ts
  export function previewAdminUsersImport(file: File): Promise<AdminUserImportResult>
  export function importAdminUsers(file: File): Promise<AdminUserImportResult>
  export function mappingCsv(rows: readonly AdminUserImportMapping[]): string
  ```
- `AdminUserImportDialog` props/emits:
  ```ts
  defineProps<{ visible: boolean }>()
  defineEmits<{ 'update:visible': [value: boolean]; imported: [] }>()
  ```

- [ ] **Step 1: Write the failing utility/permission tests**

`userImport.spec.ts`:

```ts
it('builds the mapping csv with header and escaping', () => {
  expect(mappingCsv([{ legacyId: 6, uname: '让,我', email: 'a@example.com', username: 'hydro_6' }]))
    .toBe('_id,uname,mail,username\n6,"让,我",a@example.com,hydro_6\n')
})
it('returns only the header for an empty mapping', () => {
  expect(mappingCsv([])).toBe('_id,uname,mail,username\n')
})
```

`permissions.spec.ts`: assert `PERM.user.import === 'user.import'`; do not add it to the `ADMIN_ACCESS_PERMISSIONS` list.

`AdminUserImportDialog.spec.ts`:

```ts
const api = vi.hoisted(() => ({
  previewAdminUsersImport: vi.fn(),
  importAdminUsers: vi.fn(),
}))
vi.mock('@/api/admin', () => api)
```

Tests:

- `clicking CSV help shows fields, limits and sample; clicking again hides it`:
  - mount with `visible: true`, `global: { plugins: [PrimeVue], stubs: { teleport: true } }`.
  - click `[data-test="import-help-toggle"]`.
  - assert text contains `_id`, `uname`, `hashType`, `hydro`, and `"1001,alice"`.
  - click again; `aria-expanded=false`.
- `previews a selected CSV and renders summary/issues/samples`:
  - create a `File`, set `[data-test="import-file"]` files, trigger `change`, click `[data-test="import-preview"]`.
  - mock `previewAdminUsersImport` resolve with `{ dryRun:true, summary:{total:3,valid:2,...}, issues:[...], mapping:[...], samples:[...], issuesTruncated:false }`.
  - assert API called with the File and summary counts displayed.
- `shows the mapping download only when mapping is non-empty`:
  - preview with mapping → button `[data-test="import-mapping"]` exists; preview with empty mapping → absent.
- `commits and emits imported`:
  - mock preview + import; click confirm; assert `importAdminUsers` called and `wrapper.emitted('imported')`.
- `shows a Chinese error when the request fails`:
  - mock reject with `new ApiError(400, 40001, 'CSV 表头错误')`; assert text contains it.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/web && npm test -- src/utils/userImport.spec.ts src/components/admin/AdminUserImportDialog.spec.ts src/permissions.spec.ts`
Expected: FAIL because the modules/functions/components do not exist.

- [ ] **Step 3: Implement the API, permission and utility**

`api/types.ts`:

```ts
export type AdminUserImportResult = Schemas['AdminUserImportResult']
export type AdminUserImportIssue = Schemas['AdminUserImportIssue']
export type AdminUserImportMapping = Schemas['AdminUserImportMapping']
```

`api/admin.ts`:

```ts
function importForm(file: File): FormData {
  const form = new FormData()
  form.append('file', file)
  return form
}
export const previewAdminUsersImport = (file: File) =>
  http.postForm<AdminUserImportResult>('/admin/users/import?dryRun=true', importForm(file))
export const importAdminUsers = (file: File) =>
  http.postForm<AdminUserImportResult>('/admin/users/import', importForm(file))
```

`permissions.ts`: add `import: 'user.import'` under `user`. **Do not** add it to `ADMIN_ACCESS_PERMISSIONS`: import alone must not expose the admin shell; a role that can import also needs `user.read` to reach `/admin/users`.

`utils/userImport.ts`:

```ts
export function mappingCsv(rows: readonly AdminUserImportMapping[]): string {
  const esc = (v: string) => /[",\n\r]/.test(v) ? `"${v.replaceAll('"', '""')}"` : v
  return '_id,uname,mail,username\n' +
    rows.map((r) => [String(r.legacyId), esc(r.uname), esc(r.email), esc(r.username)].join(',')).join('\n') +
    (rows.length ? '\n' : '')
}
```

- [ ] **Step 4: Implement `AdminUserImportDialog.vue`**

State and behavior:

- `visible` computed proxy for `Dialog` `v-model:visible`.
- `file`, `preview`, `result`, `previewing`, `importing`, `error`, `helpOpen`.
- File input `accept=".csv,text/csv"`, `data-test="import-file"`; validate `.csv` extension and `size <= 5*1024*1024`; reset preview/result.
- `preview` button `data-test="import-preview"` disabled without file/previewing; calls `previewAdminUsersImport(file)`.
- Help button `data-test="import-help-toggle"` with `aria-expanded`; inline panel contains:
  - a field table with `_id`, `uname`, `mail`, `hash`, `salt`, `hashType`;
  - file limits;
  - `<pre>` sample CSV using the exact sample from the spec (header + `1001,alice...` + `1002,小助手...`).
- Summary cards: total/valid/conflicts/invalid/generated.
- Issues `DataTable` columns: line, legacyId, uname, email, reason.
- Samples `DataTable`: username, uname, generated.
- Mapping download button `data-test="import-mapping"` shown when `mapping.length > 0`; on click build `mappingCsv` and download via `Blob` + `URL.createObjectURL` + anchor; revoke afterwards.
- Confirm button `data-test="import-confirm"` disabled unless `preview?.summary.valid > 0`; calls `importAdminUsers(file)`; sets `result`; emits `imported`.
- Result summary: imported/skipped/failed; keep mapping download if result.mapping non-empty.
- Error `Message` or paragraph with `role="alert"`.
- Use `Message`/`Button`/`Dialog`/`DataTable`/`Column` from PrimeVue; no new dependency.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd apps/web && npm test -- src/utils/userImport.spec.ts src/components/admin/AdminUserImportDialog.spec.ts src/permissions.spec.ts`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/api/admin.ts apps/web/src/api/types.ts apps/web/src/permissions.ts apps/web/src/permissions.spec.ts apps/web/src/utils/userImport.ts apps/web/src/utils/userImport.spec.ts apps/web/src/components/admin/AdminUserImportDialog.vue apps/web/src/components/admin/AdminUserImportDialog.spec.ts
git commit -m "feat(web): add user import dialog with CSV help"
```

---

### Task 5: AdminUsersView 入口与导入后刷新

**Files:**
- Modify: `apps/web/src/views/AdminUsersView.vue`
- Modify: `apps/web/src/views/AdminUsersView.spec.ts`

**Interfaces:**
- Consumes: Task 4 `AdminUserImportDialog`, `PERM.user.import`.
- Produces: `[data-test="import-users"]` button; dialog emits `imported` → `load()`.

- [ ] **Step 1: Write the failing view tests**

Add a named stub to the test file:

```ts
const ImportDialogStub = defineComponent({
  name: 'AdminUserImportDialog',
  props: { visible: Boolean },
  emits: ['update:visible', 'imported'],
  template: '<div v-if="visible" data-test="import-dialog" />',
})
```

Update `mountPage()` to add `stubs: { AdminUserImportDialog: ImportDialogStub }`.

Tests:

```ts
it('shows the import action only with user.import', async () => {
  auth.can.mockImplementation((code) => code === PERM.user.import)
  const wrapper = await mountPage()
  expect(wrapper.find('[data-test="import-users"]').exists()).toBe(true)
  await wrapper.get('[data-test="import-users"]').trigger('click')
  expect(wrapper.find('[data-test="import-dialog"]').exists()).toBe(true)
  wrapper.unmount()

  auth.can.mockReturnValue(false)
  const hidden = await mountPage()
  expect(hidden.find('[data-test="import-users"]').exists()).toBe(false)
  hidden.unmount()
})

it('reloads users after a successful import', async () => {
  auth.can.mockImplementation((code) => code === PERM.user.import)
  const wrapper = await mountPage()
  expect(listAdminUsers).toHaveBeenCalledTimes(1)
  wrapper.findComponent(ImportDialogStub).vm.$emit('imported')
  await flushPromises()
  expect(listAdminUsers).toHaveBeenCalledTimes(2)
  wrapper.unmount()
})
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/web && npm test -- src/views/AdminUsersView.spec.ts`
Expected: FAIL because the button/dialog integration does not exist.

- [ ] **Step 3: Implement the view integration**

- Import `AdminUserImportDialog`, `PERM` already imported.
- Add:
  ```ts
  const canImport = computed(() => auth.can(PERM.user.import))
  const importVisible = ref(false)
  ```
- In `<AppPageHeader>`, add:
  ```vue
  <template #actions>
    <Button
      v-if="canImport"
      label="导入用户"
      icon="pi pi-upload"
      data-test="import-users"
      @click="importVisible = true"
    />
  </template>
  ```
- Before the closing `</div>` of the view, add:
  ```vue
  <AdminUserImportDialog v-model:visible="importVisible" @imported="load" />
  ```
- Keep existing dialogs and permission checks unchanged.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd apps/web && npm test -- src/views/AdminUsersView.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/AdminUsersView.vue apps/web/src/views/AdminUsersView.spec.ts
git commit -m "feat(web): add user import entry to admin users"
```

---

### Task 6: 文档、忽略真实 CSV、全量验证

**Files:**
- Modify: `.gitignore`
- Modify: `docs/开发日志.md`
- Modify: `docs/任务清单.md`（可选，新增一条跨里程碑记录）
- Modify: `运行手册.md`（短说明：导入入口、CSV 隐私、失败回退）

- [ ] **Step 1: Add the real CSV to `.gitignore`**

Add:

```gitignore
# 旧 OJ 用户导出（含邮箱与密码哈希，禁止提交）
hydro-users.csv
```

Verify: `git check-ignore -v hydro-users.csv` prints the rule; `git status` must not list it.

- [ ] **Step 2: Update docs**

`docs/开发日志.md` add a 2026-11-20 entry:

- 管理后台用户 CSV 批量导入；HydroOJ hash/salt 原样保留；邮箱已验证；
- 合法用户名保留、非法生成 `hydro_<id>`、冲突跳过、重复行全无效；
- 预览/提交、格式说明、账号映射下载；
- 权限 `user.import`；
- 验证结果（后端/前端测试数、真实 CSV 预览 2418/2416/2/0/759、临时库导入次数）。

`运行手册.md` add a short section:
- 入口 `/admin/users` → 导入用户；
- CSV 含敏感数据，导入后删除；
- 冲突账号不会被修改；
- 生成用户名用户可用邮箱登录。

- [ ] **Step 3: Run backend full suite**

```bash
cd apps/server
KAPOJ_TEST_DATABASE_URL="postgres://.../kapoj_test?sslmode=disable" go test -count=1 -p 1 ./...
```

Expected: all packages pass.

- [ ] **Step 4: Run frontend full checks**

```bash
cd apps/web
npm run build
npm test
npm run lint
```

Expected: build succeeds; all tests pass; lint clean. Accept the existing >500 kB chunk warning.

- [ ] **Step 5: Manual preview with the real CSV**

Start a current-code server against the main DB (read-only preview) and call:

```bash
curl -X POST 'http://127.0.0.1:18080/api/admin/users/import?dryRun=true' \
  -H 'Authorization: Bearer <kaptree-super-token>' \
  -F 'file=@hydro-users.csv'
```

Expected: `total=2418`, `valid=2416`, `conflicts=2`, `invalid=0`, `generatedUsernames=759`, issues include `Kaptree` and `test`, mapping has 759 rows. Do **not** call the commit endpoint against the main DB.

- [ ] **Step 6: Temporary-database full import**

Use a temporary test DB (e.g. `kapoj_userimport_smoke`) with the current server on a temporary port; import a synthetic CSV or a copy of the real CSV, then verify:
- 2416 users imported, 2 skipped;
- generated user can log in with the old password (use a known synthetic row for login);
- re-import imports 0 and skips all;
- audit row exists with counts only.

Drop the temporary database and stop the temporary server afterwards.

- [ ] **Step 7: Commit docs and ignore rule**

```bash
git add .gitignore docs/开发日志.md docs/任务清单.md 运行手册.md
git commit -m "docs(userimport): document HydroOJ user import"
```

---

## Self-Review Notes

- Spec coverage: parser/limits (Task 1), username mapping/collisions (Task 1/2), password/email verified/defaults (Task 2), conflicts/idempotency/audit (Task 2/3), permission/OpenAPI (Task 3), format help/mapping download/UI (Task 4), admin entry (Task 5), docs/ignore/verification (Task 6).
- Type consistency: `Result`/`Summary`/`Issue`/`Mapping`/`Sample` JSON tags match Task 3 OpenAPI and Task 4 frontend types; `Process(ctx, r, dryRun, audit)` is used by the handler and tests; `mappingCsv` consumes the same `AdminUserImportMapping`.
- Review Focus tests are placed: #1 Task 2 `TestProcessSkipsExistingUser`; #2 Task 2 `TestProcessPreviewAndImport`; #3 Task 1 `TestApplyUsernames` + Task 2 collision; #4 Task 1 bad input + Task 3 bad CSV; #5 Task 3 permission/audit tests.
- Builder rulings: generated-name collisions suffix rather than skip; duplicate CSV rows are all invalid; `user.import` does not imply `user.read`; mapping export only generated users.
