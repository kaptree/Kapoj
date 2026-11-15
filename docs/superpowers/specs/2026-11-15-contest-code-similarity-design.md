# 比赛代码重复率检测设计

## 1. 背景

KOJ 的比赛详情页已经包含题目、榜单和公告。管理员需要在比赛结束后检查同一道题目的代码提交是否存在抄袭，并把结果以列表、并排代码对照和 CSV 的形式提供。

本设计只做“疑似抄袭”辅助判断，不自动封禁、不自动改分、不改变比赛榜单。

## 2. 目标与非目标

### 目标

- 比赛结束后，拥有比赛管理权限的用户可以按题目查看和配置查重。
- 每道题保存重复率阈值，默认 80%。
- 点击「计算本题」或「计算全部」后异步计算，不阻塞 HTTP 请求。
- 同一道题、同一种语言的代码提交互相比对。
- 相似度达到阈值的提交对中，较早提交作为原提交，较晚提交作为疑似抄袭。
- 提供作弊列表、左右代码详情、重复率和 CSV 导出。
- 支持可选的公共模板代码，排除题目提供的模板。
- 所有代码和查重结果只对比赛管理员可见。

### 非目标

- 不自动判定用户作弊，不自动处罚。
- 不比较不同编程语言之间的代码。
- 不比较客观题答案。
- 不提供公开的查重结果页。
- 不引入 AI/向量模型。
- 不依赖 MOSS、JPlag 或外部代码上传服务。

## 3. 算法决策

选择 **Go 原生 token + Winnowing**。

理由：

- 项目后端为 Go，直接在现有服务端 worker 中运行，不需要 Java/Node sidecar，也不增加 JVM 常驻内存。
- 不依赖外部服务，代码不离开平台。
- 对改格式、改注释、改变量名等常见伪装不敏感。
- 可以按题目/语言独立计算，结果可控。
- 代价是对深层语义改写、语句重排的识别弱于 JPlag；结果必须由管理员复核。

已评估但不采用：

- **JPlag**：精度好，但需要 Java 25 sidecar、GPL-3.0 许可审查和更高内存。
- **Dolos**：MIT 许可，但需要 Node 运行时，Go 语言支持不确定。
- **MOSS**：外部服务，代码需要上传，隐私和限流不可控。
- **AST/PDG/图算法**：多语言解析器成本过高，不适合一期。
- **AI/语义相似度**：不稳定、不可解释，不适合作为处分辅助。

## 4. 已确认默认值

| 项目 | 默认值 |
|---|---|
| 比较范围 | 本场比赛、本题目的所有代码提交 |
| 排除项 | 客观题、空代码、过短代码、非报名用户的测试提交 |
| 同一用户 | 不互相比对 |
| 阈值 | 80%，可在 1–100 调整 |
| 判断指标 | 最高重复率（containment 最大值） |
| 原提交 | 提交时间较早者；时间相同则 ID 较小者 |
| 疑似抄袭 | 同一匹配对中提交时间较晚者 |
| 模板 | 每题可选公共模板，按语言排除 |
| 权限 | 复用 `contest.update` |
| 操作 | 计算本题 + 计算全部 |
| CSV | 服务端生成、UTF-8 BOM、防公式注入 |
| 异步 | 每题一个 Asynq 任务，队列并发 1 |
| 查重结果 | 仅管理员可见，不公开 |

## 5. 算法细节

### 5.1 Token 化

支持 `c`、`cpp`、`python3`、`java`、`go`。

归一化规则：

- 去除空白和注释。
- 字符串统一为 `STR`，字符统一为 `CHR`，数字统一为 `NUM`。
- 非关键字标识符统一为 `ID`。
- 关键字保留原文。
- 运算符和标点保留原文。
- C/C++ 的预处理指令行归为 `PP` 并跳过该行，减少 `#include`、`#pragma` 等模板代码干扰。
- Python 保留换行、缩进和反缩进 token，避免丢失控制流结构。
- tokenizer 是宽松的：无法解析的片段尽量跳过；没有有效 token 的提交不参与比较。

### 5.2 Winnowing

默认参数：

- `k = 20` 个 token 的 k-gram。
- `w = 10` 个窗口。
- 64 位多项式滚动哈希。
- 每个窗口选择最小哈希，平局取最右。
- 每个提交保存去重后的 fingerprint 集合，并记录 fingerprint 数量和 token 数量。
- 单提交少于 `80` 个有效 token 时跳过。
- 匹配的共享 fingerprint 少于 `5` 时不算匹配。

### 5.3 相似度

设 `A`、`B` 为两个提交的 fingerprint 集合，`shared = |A ∩ B|`。

- **最高重复率**：`max(shared/|A|, shared/|B|) * 100`，用于是否超过阈值。
- **Dice 相似度**：`2*shared/(|A|+|B|) * 100`，用于详情和 CSV 参考。

页面主指标显示“最高重复率”，详情同时显示 Dice 相似度。

### 5.4 模板和公共代码

- 每题可配置一个公共模板及模板语言。
- 对提交和模板使用同一语言 tokenizer，模板的 fingerprint 从提交 fingerprint 中移除。
- 当参与比较的提交数不少于 5 时，默认丢弃出现在超过 50% 提交中的 fingerprint，减少公共 I/O、宏和固定模板引起的误报。
- 该高频率过滤可以通过 `ignoreCommonCode` 开关关闭。

### 5.5 提交对

- 只比较同一语言。
- 忽略同一用户的提交对。
- 按 `(created_at, id)` 排序，较早者为原提交，较晚者为疑似提交。
- 每个匹配对单独入库；一个疑似用户可能有多条记录。
- 相似度低于阈值或共享 fingerprint 不足的记录不写入。

## 6. 数据模型

新增三张表。

### `contest_similarity_settings`

- `id`
- `contest_id`
- `problem_id`
- `threshold_percent`（默认 80）
- `ignore_common_code`（默认 true）
- `base_language`（空表示没有模板）
- `base_code`（文本）
- `updated_by`
- `updated_at`
- 唯一索引 `(contest_id, problem_id)`

### `contest_similarity_runs`

- `id`
- `contest_id`
- `problem_id`
- `status`：`queued` / `running` / `completed` / `failed`
- `threshold_percent`
- `ignore_common_code`
- `engine`（`winnowing-v1`）
- `engine_version`（`1`）
- `requested_by`
- `submission_count`
- `match_count`
- `error_message`
- `started_at`
- `finished_at`
- `created_at`

### `contest_similarity_matches`

- `id`
- `run_id`
- `contest_id`
- `problem_id`
- `original_submission_id`
- `suspect_submission_id`
- `original_user_id`
- `suspect_user_id`
- `language`
- `similarity_percent`
- `dice_percent`
- `shared_fingerprints`
- `original_fingerprints`
- `suspect_fingerprints`
- `original_created_at`
- `suspect_created_at`
- `created_at`
- 唯一索引 `(run_id, original_submission_id, suspect_submission_id)`

不修改 `submissions` 表，不复制代码到新表。

## 7. API

所有接口要求登录并拥有 `contest.update`；比赛必须已结束。

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/api/contests/:id/similarity/overview` | 题目、阈值、最近一次 run 状态 |
| PUT | `/api/contests/:id/similarity/problems/:problemId` | 保存阈值/模板 |
| POST | `/api/contests/:id/similarity/runs` | 计算单题或全部题目 |
| GET | `/api/contests/:id/similarity/runs/:runId` | 查询任务状态 |
| GET | `/api/contests/:id/similarity/matches` | 分页查询最新结果 |
| GET | `/api/contests/:id/similarity/matches/:matchId` | 代码对照详情 |
| GET | `/api/contests/:id/similarity/export.csv` | 导出 CSV |

结果列表只使用每道题最新一次成功 run，避免重复展示历史结果。

## 8. 异步处理

- 新增 Asynq 队列 `similarity`。
- 队列并发 1，任务超时 10 分钟，最多重试 1 次。
- 每题一个任务；「计算全部」为每道配置过的题目创建一个 run 并入队。
- 如果同一题已有 `queued`/`running` 任务，直接返回现有 run，避免重复计算。
- 任务失败写入 sanitized 错误信息，页面显示“计算失败”，详细错误写日志。
- 页面轮询 overview 接口，间隔 2 秒；没有运行中的任务时停止轮询。

## 9. 前端

- `ContestDetailView` 在比赛结束且 `contest.update` 权限存在时显示「查重」Tab。
- 新增 `ContestSimilarityPanel`：
  - 题目设置表：题号、标题、提交数、阈值、最近状态、结果数。
  - 设置对话框：阈值、忽略公共代码、模板语言、模板代码。
  - 操作：保存设置、计算本题、计算全部。
  - 结果表：题目、原提交用户/时间、疑似用户/时间、重复率、操作。
  - 过滤、分页、CSV 导出。
- 新增 `ContestSimilarityDetailDialog`：
  - 左右代码并排，复用 `CodeViewer`。
  - 显示重复率、Dice、阈值、语言、提交时间。
  - 手机端改为上下布局或 Tab。
- 所有错误、空态、加载态使用现有 `Message`、`AppEmptyState`、`AppLoadingState`、`Tag`。

## 10. CSV

- 服务端生成，UTF-8 BOM。
- 表头中文，字段稳定。
- 对以 `= + - @` 开头的用户/题目字段加 `'` 前缀，防止 Excel 公式注入。
- 支持 `problemId` 过滤；不传则导出当前比赛全部最新结果。

## 11. 安全与边界

- 后端强制校验比赛已结束。
- 仅 `contest.update` 可读代码和结果。
- 不返回其他比赛的代码。
- 代码详情只来自本场本题目提交。
- 单题最大提交数默认 1000；超过时拒绝创建任务并提示管理员缩小范围。
- 单份代码最大 64 KiB（现有提交限制）。
- 模板最大 64 KiB。
- 任务运行期间不阻塞 API；worker 崩溃后 run 保持 queued/running，需要下次启动时标记失败或允许重新入队（一期在 worker 启动时标记超过 15 分钟的 running 为 failed）。
- CSV 和页面均标注“疑似抄袭，需人工复核”。

## 12. 测试与验收

- Tokenizer：五种语言的归一化、注释/空白/重命名、字符串/数字、Python 缩进、C/C++ 预处理。
- Winnowing：确定性、窗口最小值、相同/不同代码。
- Engine：重命名副本高分、无关代码低分、模板排除、高频过滤、同用户不配对、阈值和最小匹配数。
- Service：权限、比赛未结束、题目归属、阈值校验、重复任务保护、最新 run、失败状态。
- Handler：401/403、CSV BOM/转义、分页、详情代码。
- Frontend：Tab 权限、设置保存、计算状态轮询、结果列表、详情对照、CSV 下载、移动端布局。
- 全量：`go test`、`vue-tsc`、`vitest`、`vite build`、ESLint、Prettier。

## 13. 风险

- Token 级检测无法证明抄袭；页面和导出使用“疑似”表述。
- Go 代码中的宏/模板可能造成误报；公共模板和高频 fingerprint 过滤是主要缓解手段。
- 单题提交超过 1000 时 O(N²) pair 聚合可能变慢；一期限制并提示，后续可做采样或更高效的候选筛选。
- 语言 tokenizer 是简化实现，不是完整编译器前端；对非常规语法要容错并跳过，不让整批任务失败。
