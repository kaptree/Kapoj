# M5-4 AI 出题造数据设计

> 目标：管理后台在题目编辑中提交标程，AI 生成 Python 造数据脚本；judge 在 Docker 沙盒运行脚本生成输入、运行标程生成输出；管理员预览确认后写入题目测试数据。

## 1. 背景与现状

- 现有题目编辑可逐个上传 `.in/.out` 测试点；judge 已用 Docker 沙盒编译/运行 C/C++/Python/Java/Go，限制网络、CPU、内存、只读、pids 和输出。
- M5-A 已提供用户 AI 配置、OpenAI 兼容调用、限流；M5-5 已提供 AI 前端。
- judge 是独立 Go 模块与进程，现有 Asynq 队列仅处理 `judge:submission`；服务器与 judge 共享 PostgreSQL/Redis/Storage 目录。
- DEVELOPMENT §6.4 原设想 cyaron；当前 Python 沙盒镜像 `python:3.12-slim` 不包含第三方包，且沙盒无网络，无法安装。因此改为 **纯 Python 3 标准库**造数据脚本（AI 提示明确要求），效果等价且无需改镜像。

## 2. 目标与非目标

### 目标

- 新增题目测试数据 AI 生成流程：管理员上传/粘贴标程（语言+源码）、选择生成组数；AI 生成纯 Python 3 造数据脚本。
- judge 在 Docker 沙盒中：
  - 编译/检查 Python 生成脚本；
  - 编译标程；
  - 对每组 seed 运行脚本（stdin 一个整数 seed）得到一组输入；
  - 用标程对输入运行得到输出；
  - 把生成文件写入共享 storage，记录生成结果。
- 管理后台预览每组输入/输出（截断预览），确认后写入 `problem_testcases`；可丢弃生成结果并清理文件。
- 只支持传统题（traditional）；特殊题/交互题不支持自动生成输出。
- 安全：脚本和标程只在 judge Docker 沙盒执行；网络 none、只读、资源/输出限制；不在服务器进程执行。
- 默认生成 1–10 组，score=10，sort_order 接在已有测试点后；入库前必须管理员确认。

### 非目标

- 不自动发布题目；生成后仍需管理员发布。
- 不生成 checker、special/interactive 答案、样例（`is_sample=false`）。
- 不安装 cyaron/第三方包；不改 judge Python 镜像。
- 不做 AI 直接生成 output（output 必须由标程运行得到）。
- 不做生成历史长期归档/定时清理（仅当前生成记录与文件；确认后文件归测试点）。
- 不改变现有手工测试点上传/排序/删除接口。

## 3. 数据模型

### 3.1 testcase_generations

```text
testcase_generations (
  id bigserial primary key,
  problem_id bigint not null,
  user_id bigint not null,
  status text not null,        -- pending/running/ready/failed/confirmed/discarded
  language text not null,      -- 标程语言
  standard_code text not null,
  generator_code text not null,
  testcase_count int not null,
  time_limit_ms int not null,  -- 标程运行限制（题目限制）
  memory_limit_mb int not null,
  error_message text not null default '',
  created_at timestamptz not null,
  updated_at timestamptz not null
)
-- index(problem_id, created_at desc)
```

### 3.2 testcase_generation_cases

```text
testcase_generation_cases (
  id bigserial primary key,
  generation_id bigint not null,
  sort_order int not null,
  input_file text not null,
  output_file text not null,
  input_size bigint not null,
  output_size bigint not null
)
-- unique(generation_id, sort_order)
```

- 生成文件 key：`problems/<problemID>/generated/<generationID>/<n>.in|.out`，写入共享 storage。
- 确认：用相同 file key/size 插入 `problem_testcases`（score=10，sort_order 接在现有后），生成记录 `confirmed`；文件继续被测试点引用。
- 丢弃/失败：删除该 generation 的 cases、生成文件；记录保留 `discarded`/`failed`。
- 迁移加入 `db.Migrate`；服务器拥有表结构，judge 只读写。

## 4. 流程

### 4.1 创建生成任务（服务器）

`POST /api/problems/:id/testcase-generations`（`problem.testcase.manage`）

请求：

```json
{ "language": "cpp", "sourceCode": "...", "testcaseCount": 5 }
```

- 校验：题目存在、`type=traditional`、语言合法、源码 ≤64KiB、count 1–10；题目时间/内存限制作为标程运行限制。
- 调 AI `GenerateTestcaseScript`（使用当前管理员 AI 配置）：
  - 输入题目 title/statement/inputFormat/outputFormat/samples、标程、语言、count；
  - 输出纯 Python 3 脚本（≤64KiB）；
  - 无平台密钥 40002；用户未配置 40001；限流 42903；上游 50200；Redis 50300。
- 插入 `testcase_generations(status=pending)`，保存 generator_code/standard_code/limits；投递 judge 队列 `judge:testcase-generation {generationId}`。
- 返回 generation 视图（pending）。

### 4.2 judge 执行（新任务）

`apps/judge` 新增 handler：

1. `Claim(generationId)`：仅 `pending` 可认领，置 `running`；不存在/已终态跳过。任务 `MaxRetry(0)`，失败由管理员重新发起。
2. 删除该 generation 旧 cases/文件（防御重试残留）。
3. 编译生成器（python3 `py_compile`）；失败 → `failed` + 编译错误摘要。
4. 编译标程（按 language）；失败 → `failed` + 错误摘要。
5. 对 i=1..count：
   - 临时输入文件写 seed（`i\n`）；
   - 沙盒运行生成器：stdin seed，stdout 即该组输入；限制 5s/256MB/16MB；
   - 空输出、超时、OOM、非 0、输出截断 → 失败；
   - 写 storage input key；
   - 沙盒运行标程：输入=该组输入；限制题目 time/memory/16MB；
   - 超时/RE/OOM/输出截断/空输出 → 失败；
   - 写 storage output key；插入 case 行。
6. 全部成功 → `ready`；任一失败 → 删除已写文件/cases，`failed` + 中文/编译错误摘要。
7. 生成器/标程均在现有 Docker 沙盒（network none、read-only、cap-drop、CPU/内存/pids 限制）；输入输出大小受限。

### 4.3 预览与确认（服务器）

- `GET /api/problems/:id/testcase-generations/:genId`（`problem.testcase.manage`）：
  - 返回生成状态、错误、cases 元数据（序号/大小），`ready` 时可查看。
- `GET .../cases/:caseId/preview?kind=input|output`：
  - 从 storage 读取，最多返回 64KiB 文本；非 UTF-8 用替换字符；`truncated` 标记。
- `POST .../confirm`：
  - 仅 `ready`；事务/顺序插入 `problem_testcases`（score=10，`is_sample=false`，sort_order 从现有最大值+1），标记 `confirmed`；返回创建数量。
  - 已有测试点不删除/不覆盖。
- `DELETE ...` / `POST .../discard`：
  - `pending/running` 不能丢弃？允许标记 discarded 由 judge 检查状态后停止；简化：仅 `ready/failed` 可丢弃；删除 cases/文件，状态 discarded。
- 列表/详情需要题目管理权限；隐藏题由 `problem.testcase.manage` 已有的管理语义覆盖（不新增 view_hidden）。

## 5. API 与 DTO

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| POST | `/api/problems/:id/testcase-generations` | `problem.testcase.manage` | 创建生成任务 |
| GET | `/api/problems/:id/testcase-generations/:genId` | 同上 | 状态/cases 元数据 |
| GET | `.../cases/:caseId/preview?kind=input|output` | 同上 | 截断预览 |
| POST | `.../confirm` | 同上 | 确认入库 |
| DELETE | `/api/problems/:id/testcase-generations/:genId` | 同上 | 丢弃/清理 |

`TestcaseGenerationView`：

```json
{
  "id": 1,
  "status": "ready",
  "language": "cpp",
  "testcaseCount": 5,
  "generatorCode": "...",
  "errorMessage": "",
  "createdAt": "...",
  "cases": [
    { "id": 10, "sortOrder": 1, "inputSize": 12, "outputSize": 6 }
  ]
}
```

- 创建响应同视图（pending）；前端轮询 GET（1–2s，最长 10 分钟）。
- 失败返回错误摘要（编译错误最多 8KB，运行错误中文摘要）。
- 不返回标程/生成器完整源码给普通用户；仅 testcase.manage 管理员可见（generatorCode 可用于审计）。

## 6. 前端

- 新页面 `/problems/:id/testcase-generator`（路由 `problem-testcase-generator`，`problem.testcase.manage`），PrimeVue 4。
- 入口：`ProblemEditView` 的测试点区域（`TestcaseManager`）增加“AI 造数据”按钮（有权限时），跳转带返回。
- 表单：
  - 语言 Select（c/cpp/python3/java/go）；
  - 标程 CodeEditor（Monaco，可复用 CodeEditor）；
  - 生成组数 InputNumber 1–10；
  - 说明：仅传统题、脚本在沙盒运行、确认后才会入库；
  - “生成数据”按钮（调用 AI + 创建任务）。
- 生成中：状态/轮询/错误；显示 AI 生成的脚本预览（只读/折叠）。
- 完成后：cases 表格（序号/输入大小/输出大小）；点击预览输入/输出 Dialog（截断提示）；“确认入库”“丢弃”。
- 失败：显示编译/运行错误；可返回修改标程重新生成。
- AI 未配置/密钥缺失：40002/40001 提示与 `/settings/ai` 链接。
- 移动端单列/表格滚动。

## 7. 权限与安全

- 仅 `problem.testcase.manage` 可创建/查看/确认/丢弃；不新增权限码。
- 标程/生成脚本只在 judge Docker 沙盒执行；服务器不 `os/exec`。
- 沙盒沿用 network none、read-only、cap-drop、CPU/内存/pids/输出限制；生成器运行 5s/256MB，标程使用题目限制。
- 源码/脚本/输出长度限制；生成文件写入 storage，不直接暴露路径。
- AI prompt 只要求生成数据脚本；不执行模型返回 HTML/Markdown；strip code fence。
- 不自动发布题目；确认只新增测试点，不删除既有数据。

## 8. 测试与验收

### 8.1 后端

- AI prompt/script：无密钥 40002、用户未配置 40001、限流 42903、上游 502、代码围栏剥离、脚本大小限制。
- 创建生成：权限、传统题限制、语言/源码/组数校验、pending 行、队列任务。
- judge testgen：claim 幂等、生成器/标程编译失败、空/超时/OOM/非 0/截断、多组成功、文件 key/size、失败清理、状态 ready/failed、重试守卫。
- 预览：input/output 截断/非 UTF-8、非本人 problem 权限。
- 确认：ready only、testcase 行 score/sort_order、文件 key 复用、状态 confirmed、重复确认幂等/40001；丢弃清理。
- 迁移/DB 索引；不破坏现有测试点接口。

### 8.2 前端

- 页面权限/入口；表单校验；创建/轮询/ready/failed；脚本预览；cases/预览 Dialog；确认/丢弃；AI 错误；移动端。
- 生成类型/lint/build。

### 8.3 真实冒烟

- 临时题目（传统题）+ 标程；假 AI provider 返回一个纯 Python 脚本；真实 judge + Docker 生成 3 组；预览；确认后测试点入库并可被提交评测；丢弃清理。
- 编译失败/运行超时/组数超限/非传统题/AI 未配置。
- 清理临时题目/测试点/生成记录/文件/配置；不删除既有数据。

## 9. 交付拆分

- GEN-A：服务器 generation 模型/AI 脚本生成/创建 API/队列契约。
- GEN-B：judge testgen handler/沙盒生成/DB/storage。
- GEN-C：预览/确认/丢弃 API + testcase 入库。
- GEN-D：OpenAPI/生成类型 + 前端页面/入口。
- GEN-E：文档、全量检查、真实冒烟（judge Docker）。

## 10. 留给实施计划的决定

- 服务器 `GenerationService` 包拆分/依赖（AI、queue、storage、repo）。
- judge testgen 包/任务注册与重试策略。
- 生成器脚本 stdin seed 格式（整数 i，1-based）。
- 状态轮询间隔/超时与前端取消策略。
