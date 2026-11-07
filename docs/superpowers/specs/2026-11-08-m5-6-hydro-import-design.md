# M5-6 HydroOJ 题包导入设计

> 目标：在题目管理入口上传 HydroOJ 导出的 zip 题包，按每个题目文件夹解析 `problem.yaml`（title/tag）、`problem_zh.md`（题面）与 `testdata/*.in|*.out`，为上传者创建默认下线的 kapOJ 题目与测试数据；题目详情页展示创建者并可跳转主页。

## 1. 背景与现状

- 现有题目创建走 `POST /api/problems`（`problem.create`），一次一题；测试数据通过 `POST /problems/:id/testcases` 逐个上传。
- `Problem` 已有 `Title`、`Slug`、`StatementMD`、`Tags`、`Difficulty`、`TimeLimitMS`、`MemoryLimitMB`、`Source`、`CreatedBy`、`IsPublic`；测试数据 `ProblemTestcase` 有 `Score`（默认 10）。
- 题目详情 `ProblemDetail` 目前只在管理员/已通过用户下返回 `createdBy`，没有对所有人公开的创建者摘要；题目详情页没有创建者展示。
- 样例题包 `Export.zip` 结构：
  ```text
  6760/
    problem.yaml       # pid, owner, title, tag, nSubmit, nAccept
    problem_zh.md      # Markdown 题面（含样例代码块）
    testdata/
      1.in / 1.out
      ...
      gen.py / std.cpp # 非测试数据，忽略
  ```
- 设计要求（DEVELOPMENT §7）：大小限制、防路径穿越、slug 冲突不覆盖、导入后默认下线。

## 2. 目标与非目标

### 目标

- `POST /api/problems/import/hydro`：登录且拥有 `problem.create` 的用户上传 zip，导入其中所有合法题目。
- 每个题目：
  - `problem.yaml` 的 `title` → `problems.title`，`tag` → `problems.tags`；
  - `problem_zh.md`（或 `problem.md`）→ `problems.statement_md`；
  - `testdata/` 中同名 `.in`/`.out` 成对 → `problem_testcases`（按文件名数字/字典序排序，score=10）；
  - `created_by` = 上传者；`is_public=false`；`source='HydroOJ'`；默认难度 1、时间 1000ms、内存 256MB、traditional。
- 返回逐题结果：成功 `{id,title,slug,testcaseCount}`，失败 `{folder,title,error}`；合法题包允许部分成功。
- 题目详情返回公开创建者摘要 `{id,username,nickname}`；前端在题目标题上方展示“创建者”并链接 `/users/:id`。
- 题目列表/导入页提供“导入题包”入口（`problem.create`）。

### 非目标

- 不导入 `additional_file/`、`gen.py`、`std.cpp` 等非测试数据文件；不执行题包内脚本。
- 不解析 `problem.yaml` 的 time/memory/difficulty（用户明确只取 title/tag；使用默认值）。
- 不自动上架；导入后一律下线，由拥有 `problem.publish` 的管理员审核发布。
- 不覆盖已有 slug；不合并/更新已有题目。
- 不做 HydroOJ 在线 API 拉取、增量同步、导入回滚 UI、进度条/断点续传。
- 不改变现有单题创建/测试数据上传接口。

## 3. 题包格式与解析规则

### 3.1 目录发现

- 扫描 zip 条目，识别“包含 `problem.yaml` 的目录”为题目文件夹；支持题目文件夹嵌套在任意包装目录下。
- 立刻跳过：`__MACOSX/`、隐藏文件/目录、非 regular 文件、符号链接、绝对路径、含 `..` 路径段、加密条目；命中危险条目直接判该题包非法。
- 题目文件夹名（最后一级目录）用于生成 slug：`hydro-` + 文件夹名小写化、非 `[a-z0-9-]` 替换为 `-`、去首尾 `-`；结果必须满足现有 slug 规则（2–63 位），否则该题失败。
- zip 内重复题目文件夹名/重复 slug → 每个重复项失败，不覆盖。

### 3.2 必需文件

每个题目文件夹必须包含：

- `problem.yaml`（必需，UTF-8）：解析 `title`、`tag`（字符串数组）。`title` trim 后 1–100 字符；`tag` 走现有 `normalizeTags`（最多 10 个、去重/trim）。
- `problem_zh.md` 优先；缺失或内容为空时回退到 `problem.md`。两者都缺失/为空 → 该题失败。内容 ≤100,000 字符；整段作为 `statement_md`，不拆 input/output/samples。
- `testdata/` 下至少 1 组同名 `.in`/`.out`；仅接受 `.in`/`.out` 后缀，按 basename 数字优先、再字典序排序；`.in` 无 `.out` 或反之 → 该题失败。空文件是合法输入/输出（0 字节允许），只要求成对存在。
- 每个测试文件 ≤16MB（沿用 `maxTestcaseBytes`）；单题测试数据总数 ≤200MB；单题测试点 ≤500。
- 其他文件忽略。

### 3.3 题包限制

- 压缩包文件 ≤200MB；解压总量 ≤1GB；条目数 ≤5000；题目文件夹数 ≤100。
- 上传路由 body 限制 210MB（200MB 文件 + multipart 开销）；handler 用 `FileHeader.Size` 校验文件 ≤200MB → 40001；body 超过 210MB 由中间件返回 413。不放大现有 JSON/上传组限制。
- 超出 ZIP/解压/条目/题数限制返回 40001（中文错误）；zip 损坏/加密/无任何题目文件夹返回 40001。

## 4. 数据映射

| HydroOJ | kapOJ |
|---|---|
| `problem.yaml.title` | `problems.title` |
| `problem.yaml.tag` | `problems.tags` |
| `problem_zh.md` / `problem.md` | `problems.statement_md` |
| 文件夹名 | `problems.slug`（`hydro-<sanitized>`） |
| 上传者 | `problems.created_by` |
| — | `problems.is_public=false` |
| — | `problems.source='HydroOJ'` |
| — | `problems.type='traditional'`、difficulty=1、time=1000ms、memory=256MB |
| `testdata/<base>.in/.out` | `problem_testcases.input_file/output_file`，score=10，sort_order 按排序 |

- `input_format_md`/`output_format_md`/`samples_json`/`hint_md` 为空；题面中的样例代码块由 Markdown 正常渲染。
- 导入的题目与测试数据使用现有 storage key 规则；文件名不直接作为磁盘路径，避免 zip slip。

## 5. API

### 5.1 导入

`POST /api/problems/import/hydro`（multipart/form-data，字段 `file`）

- 认证 + `problem.create`；body ≤200MB；审计 best-effort `problem.create`（每个成功题目一条，target=`problem`，targetID=新题 id）。
- 响应 200（题包可解析，至少发现一个题目文件夹）：
  ```json
  {
    "imported": [
      { "id": 101, "title": "求和", "slug": "hydro-6760", "testcaseCount": 10 }
    ],
    "failed": [
      { "folder": "6761", "title": "坏题", "error": "缺少 problem_zh.md" }
    ]
  }
  ```
- 题包整体非法（损坏、无题目文件夹、超限、路径穿越）→ 40001；若整包级错误发生在已导入部分题目之后（例如伪造解压大小导致实际读取超限），先回滚本次已导入题目及其测试文件，再返回 40001，不留下部分数据。单题失败只进入 `failed`，不阻断其他题。
- slug 已存在 → 该题失败“题目标识已存在：hydro-xxxx”。
- 导入后题目默认下线；不自动创建比赛/学习路线。

### 5.2 题目详情创建者

- `ProblemDetail` 新增可选 `author: { id, username, nickname }`（公开，所有能看到该题的用户；不含 email）。
- 仅 `GET /problems/:id` 负责填充 author；`POST/PUT` 响应可以省略（前端详情页以 GET 为准）。
- `created_by <= 0` 或用户行不存在：author 省略/null，不报错；数据库查询失败才返回 500。
- 前端题目详情头部 meta 区显示“创建者 <nickname>”并链接 `/users/:id`；无 author 则不显示；昵称为空时用 username。

## 6. 导入流程与一致性

1. 入口校验：登录、`problem.create`、multipart 文件存在、大小限制。
2. 读取 zip（不落盘解压）：先扫描条目做路径/大小/类型校验，再按题目文件夹处理。
3. 每个题目在独立事务/清理单元中：
   - 构建 `problem.Input`，`Create` 题目（默认下线）；slug 冲突/Create 失败 → 记录失败，不产生数据。
   - 逐个测试点：打开 zip 条目 reader → `Service.AddTestcase`（内部 storage 写入 + DB 插入）。
   - 任一测试点失败：删除该题已写入的测试文件并删除题目（`Service.Delete`/仓储清理），记录失败；不影响其他题目。
4. 成功后按题目写入 best-effort 审计；汇总结果返回。
5. 导入不改变其他题目/测试数据；不覆盖已有 slug。

## 7. 前端

### 7.1 导入页 `/problems/import`

- 路由 `problem-import`，`requiresAuth` + `problem.create`。
- `AppPageHeader` + `AppSurface`：文件选择（`.zip`，单文件）、上传按钮、限制说明（200MB、默认下线）、结果表（成功：标题/slug/测试点数/编辑链接；失败：文件夹/错误）。
- 成功后提供“返回题库”“继续导入”；失败项逐条展示中文错误。
- 题目列表页在 `problem.create` 时显示“导入题包”按钮，与“新建题目”并列。
- 编辑链接仅在 `problem.update` 时显示；否则仅显示“已导入（下线）”。

### 7.2 创建者展示

- 题目详情头部 meta 区增加创建者链接；`RouterLink` 到 `/users/:id`；无 author 时不显示。
- 测试：作者摘要渲染、链接、游客/普通用户可见、昵称回退 username。

## 8. 权限与审计

- 导入接口与页面：`problem.create`；不新增权限码。
- 编辑/发布导入题目沿用 `problem.update`/`problem.publish`。
- 导入成功后对每个题目写 best-effort 审计 `problem.create`，target type `problem`，target id 新题 id；审计失败只记录日志，不影响导入结果。
- 创建者摘要公开但不含 email；隐藏题仍要求 `problem.view_hidden`/super 才能看详情。

## 9. 配置与迁移

- 不新增数据库表/列（复用 `problems`/`problem_testcases`）；仅在 `ProblemDetail` DTO 增加 `author`。
- 新增常量：压缩包 200MB、解压 1GB、5000 条目、100 题、500 测试点/题、16MB/文件；可在 `problem` 包内定义，不做环境变量。
- 新增直接依赖 `go.yaml.in/yaml/v3`（模块图已有 indirect）用于解析 `problem.yaml`；`go mod tidy`。
- OpenAPI 增加导入 path、`HydroImportResult`/`HydroImportedProblem`/`HydroImportFailure`、`ProblemDetail.author`；生成前端类型。

## 10. 测试与验收

### 10.1 后端

- 解析：单题/多题/嵌套包装目录；title/tag/题面映射；数字/字典序测试点；忽略 gen.py/std.cpp；`problem.md` 回退。
- 校验：缺 problem.yaml/题面/testdata、in/out 不成对、空题面、slug 非法/冲突、zip 损坏、路径穿越、符号链接、超大小/条目/题数/测试点；空测试文件（0 字节）允许。
- 作者：GET 详情填充 author；created_by=0/缺失用户省略；DB 错误 500；POST/PUT 可省略。
- 导入：默认下线、creator=上传者、source=HydroOJ、默认限制、测试数据可被 judge 读取、重复导入不覆盖。
- 部分成功：一题失败不影响其他题；失败题不留下题目/文件。
- 作者：ProblemDetail author 对所有可读用户返回；隐藏题 404；DTO/OpenAPI 同步。
- 权限 401/403；审计成功/失败 best-effort。

### 10.2 前端

- 导入页：文件选择/上传/成功/失败/限制说明/权限/编辑链接条件。
- 题目列表“导入题包”入口权限。
- 题目详情创建者展示/链接/昵称回退/游客可见。
- 生成类型/lint/build。

### 10.3 真实冒烟

- 使用根目录 `Export.zip` 真实上传：导入 `6760` 题目，title=求和、tags=循环/DAI、题面 Markdown、10 组测试数据、默认下线、creator=上传者。
- 题目详情显示创建者并可跳转主页；管理员编辑/发布流程可用；普通用户看不到下线题。
- 重复上传同一 zip：slug 冲突逐题失败、不覆盖。
- 清理导入题目/测试文件/审计；不删除既有数据。

## 11. 交付拆分

- M5-6A：zip 解析/校验/导入服务、API、作者摘要（后端）。
- M5-6B：导入页、题目列表入口、详情创建者展示、OpenAPI/生成类型（前端）。
- M5-6C：文档、全量检查、真实 `Export.zip` 冒烟。

## 12. 留给实施计划的决定

- 导入服务放 `problem` 包的文件拆分与函数签名。
- 测试数据 reader 流式读取与失败清理的具体实现。
- 逐题审计的调用位置（handler 或 service）。
- 前端上传进度是否显示（无 XHR 进度则只显示 loading）。
