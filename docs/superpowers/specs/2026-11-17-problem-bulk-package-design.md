# 题目批量管理与题包导入导出设计

**日期：** 2026-11-17
**状态：** 已实施（2026-11-17）
**范围：** 题包导入即公开、题库批量管理、单题/批量 ZIP 导出、kapOJ 题包导入

## 1. 目标

1. 导入 HydroOJ 或 kapOJ 题包时，可勾选“导入后立即公开”；新建题和覆盖题都逐题尝试发布。
2. 题库页新增“批量管理”入口；拥有 `problem.manage_all` 的管理员可多选题目，批量公开、隐藏、删除、下载。
3. 题目详情页右侧新增“题目题包”卡片，创建者/管理员可将单题导出为 ZIP。
4. 定义 `kapOJ 题包 v1`：包含题目完整元数据、题面、样例、测试数据、checker、附件、题面图片、客观题答案、官方题解；导出包可重新导入本平台。
5. 保留现有 HydroOJ 题包导入，不破坏已有流程。

## 2. 已确认决策

- 题包采用 **kapOJ 题包 v1**，导入自动识别 kapOJ/HydroOJ 两种格式。
- 题包包含测试数据、checker、客观题答案、官方题解；仅创建者或 `problem.manage_all` 可下载。
- 批量下载输出一个总 ZIP，内部按 `problems/<slug>/` 组织，根目录有批量 manifest。
- 批量管理入口仅对 `problem.manage_all` 显示；服务端同样要求该权限。
- 导入即公开对新建和覆盖题都尝试发布；无 `problem.publish` 权限时服务端拒绝该选项。
- 批量删除沿用现有规则：无提交物理删除；有提交只下线；学习路线引用等失败逐题返回，不影响其他题目。

## 3. 非目标

- 不把题包作为数据库备份替代品（数据库/账号备份是独立功能）。
- 不导出用户提交、排名、讨论、题解评论等运行时数据。
- 不实现题目版本历史、差异合并或冲突内容三方合并。
- 不改变单题编辑页现有字段与权限模型。

## 4. kapOJ 题包 v1

### 4.1 单题包结构

```text
manifest.json
statement.md
input_format.md
output_format.md
hint.md
samples.json
answers.json                 # 仅客观题
testdata/meta.json
testdata/001.in
testdata/001.out
checker/checker              # 仅特判题且存在 checker
attachments/<safe-name>      # 公开附件，保留原始文件名
images/<safe-name>           # 题面图片
solution.md                  # 官方题解正文
solution_code.txt            # 官方题解代码
```

`manifest.json`：

```json
{
  "format": "kapoj-problem",
  "formatVersion": 1,
  "exportedAt": "2026-11-17T00:00:00Z",
  "problem": {
    "slug": "two-sum",
    "title": "两数之和",
    "type": "traditional",
    "difficulty": 2,
    "timeLimitMs": 1000,
    "memoryLimitMb": 256,
    "source": "",
    "tags": ["数组"],
    "isPublic": true,
    "solutionPriceCoins": 0
  },
  "files": {
    "statement": "statement.md",
    "inputFormat": "input_format.md",
    "outputFormat": "output_format.md",
    "hint": "hint.md",
    "samples": "samples.json",
    "answers": "answers.json",
    "testdata": "testdata/meta.json",
    "checker": "checker/checker",
    "attachments": [{"file": "attachments/a.pdf", "filename": "a.pdf", "contentType": "application/pdf"}],
    "images": [{"file": "images/a.png", "filename": "a.png", "contentType": "image/png"}],
    "solution": "solution.md",
    "solutionCode": "solution_code.txt"
  }
}
```

- 文件名去除路径分隔符，重名自动加序号；manifest 保存原始文件名和映射。
- testcase meta 保存 `sortOrder/score/isSample`，文件按排序编号命名。
- 题面中的站内图片 URL 导出时记录映射；导入时替换为导入后新题目的图片 URL。
- 不导出数字 ID、提交数、通过数、创建时间、创建者 ID、数据库存储键。
- 单题包也适用于“批量包中的一道题”。

### 4.2 批量包结构

```text
manifest.json                 # format=kapoj-problems, formatVersion=1
problems/<slug>/...           # 每道题一个单题包
```

根 manifest 记录题目列表、slug、标题和相对目录。批量导入、批量下载共用这一格式。

## 5. 导入

### 5.1 格式识别

- 读取 ZIP 根或单层目录中的 `manifest.json`：
  - `format=kapoj-problem` → 单题 kapOJ 包；
  - `format=kapoj-problems` → 批量包，逐题导入；
  - 无 kapOJ manifest 且存在 `problem.yaml` → 保持现有 HydroOJ 导入。
- 继续使用现有大小/条目/解压上限，并增加 kapOJ manifest 字段校验。

### 5.2 导入即公开

- 导入页增加 `导入后立即公开` 复选框，仅拥有 `problem.publish` 时显示。
- API 导入参数增加 `publish=true|false`；为 true 时服务端要求 `problem.publish`，否则返回 403。
- 对每道成功新建/覆盖的题：
  - 导入先按现有规则保存为未公开；
  - 导入完成后逐题尝试发布；
  - 发布成功标记 `published=true`；
  - 付费路线题目、学习路线引用、缺测试点/checker 等原因发布失败时，导入本身保留，返回 `publishError`，不整体回滚。
- 覆盖导入时保持现有“冲突策略 ask/skip/overwrite”；`publish` 只影响最终可见性。
- 导入结果扩展为：

```json
{
  "imported": [{
    "id": 1, "title": "...", "slug": "...", "testcaseCount": 3,
    "overwritten": false, "published": true, "publishError": ""
  }],
  "failed": [],
  "conflicts": [],
  "skipped": [],
  "warnings": []
}
```

- Hydro 导入同样支持 `publish`，避免两套导入行为不一致。

### 5.3 kapOJ 导入步骤

1. 校验 manifest、slug、题目类型、文件映射、大小与重名。
2. 按冲突策略创建/覆盖题目元数据（题面、格式、提示、样例、难度、标签、时限、内存、来源、答案）。
3. 导入 testdata（保留分值、样例标记、顺序）。
4. 导入 checker（仅特判题）、附件、题面图片并完成 URL 重写。
5. 导入官方题解与价格（仅管理员/创建者导入权限范围内）。
6. 提交后逐题尝试发布（如果勾选立即公开）。
7. 任一道题创建过程中失败，清理该题已写入的文件/测试点，不影响其他题。

## 6. 导出

### 6.1 单题导出

- `GET /api/problems/:id/package`
- 权限：题目创建者或 `problem.manage_all`。
- 响应：`application/zip`，`Content-Disposition: attachment; filename="<slug>.zip"`，`Cache-Control: private, no-store`。
- 流式写 ZIP，避免把测试数据一次性读入内存。
- 权限不足返回 404/403；隐藏题同样适用。

### 6.2 批量导出

- `POST /api/problems/bulk/export`，body `{ problemIds: [1,2,3] }`。
- 权限：仅 `problem.manage_all`；任一题目不存在/无权管理时整批返回 403/404，不返回部分包，避免越权泄露。
- 输出 `problems-export-<timestamp>.zip`，内部 `problems/<slug>/` + 根 manifest。
- 数量上限默认 100 道（与导入一致）；超过返回 40001。
- 导出流设置总大小上限，超过时中止并返回错误；已开始响应后不可修改 HTTP 状态，因此先检查元数据总大小再开始写响应。

## 7. 批量管理

### 7.1 题库页面

- 仅 `problem.manage_all` 显示“批量管理”按钮。
- 点击后：
  - DataTable 显示多选列，支持本页全选；
  - 顶部出现操作条：`公开`、`隐藏`、`删除`、`下载`、`取消`；
  - 显示已选数量；跨页选择不清空，翻页后保留。
- 批量删除/隐藏/公开使用确认对话框；结果以逐题汇总展示。
- 批量下载调用总 ZIP 接口并触发浏览器下载。

### 7.2 API

`POST /api/problems/bulk/actions`：

```json
{
  "action": "publish | hide | delete",
  "problemIds": [1, 2, 3]
}
```

返回：

```json
{
  "results": [
    {"id": 1, "status": "success", "offlined": false, "error": ""},
    {"id": 2, "status": "failed", "offlined": false, "error": "题目被学习路线引用，不能下线/删除"}
  ]
}
```

- 仅 `problem.manage_all` 可调用。
- 逐题独立执行，部分成功是正常结果，不整体回滚。
- `publish`：
  - 已公开：返回 success + 备注“已公开”；
  - 未公开：执行发布校验（测试点/checker、付费路线、学习路线等），失败记录该题原因。
- `hide`：设置为未公开；被活跃学习路线引用时失败（沿用现有规则）。
- `delete`：
  - 无提交：物理删除题目、测试点、checker、附件和文件；
  - 有提交：仅下线，返回 `offlined=true`；
  - 学习路线引用：该项失败。
- 批量操作写审计，记录 action、题目 ID 列表和成功/失败数量；逐题信息可在响应和前端结果面板查看。

### 7.3 题目详情右侧卡片

- 右侧/底部布局新增“题目题包”卡片：
  - 标题、格式版本；
  - 文件数量/大致大小；
  - `下载 ZIP` 按钮；
  - 说明“包含测试数据、checker、附件和题解，请妥善保管”。
- 仅 `problem.canEdit`（创建者/管理员）显示；导出仍由后端二次校验。

## 8. 权限与安全

- 新增（或复用）以下判断：
  - 批量管理/批量导出：`problem.manage_all`；
  - 单题下载：题目创建者或 `problem.manage_all`；
  - 导入即公开：额外要求 `problem.publish`。
- 题包包含测试数据、checker、客观题答案、官方题解，视为敏感文件：
  - 只通过鉴权下载接口返回；
  - `Cache-Control: private, no-store`；
  - 不写入日志、不在公开 DTO 中返回文件键；
  - 导出 ZIP 不进入公共附件接口。
- ZIP 导入继续执行 Zip Slip、大小、条目数、UTF-8/文件名安全校验。
- 导入/导出/批量操作均写审计。

## 9. 接口与 OpenAPI 变更

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| POST | `/api/problems/import/hydro` | `problem.create` + 可选 `problem.publish` | 增加 `publish` 参数；继续支持 Hydro |
| POST | `/api/problems/import/package` | `problem.create` + 可选 `problem.publish` | kapOJ 单题/批量包导入；自动识别格式 |
| GET | `/api/problems/:id/package` | 创建者/`problem.manage_all` | 单题 ZIP 下载 |
| POST | `/api/problems/bulk/actions` | `problem.manage_all` | 批量公开/隐藏/删除 |
| POST | `/api/problems/bulk/export` | `problem.manage_all` | 批量 ZIP 下载 |

导入结果 schema 增加 `published`/`publishError`；批量操作/批量导出增加独立 schema。所有接口同步 OpenAPI 并生成前端类型。

## 10. 前端变更

- `ProblemImportView.vue`：增加“导入后立即公开”复选框；无发布权限时不显示/禁用；展示逐题发布结果与警告。
- `ProblemListView.vue`：新增“批量管理”按钮、多选列、操作条、确认框、逐题结果面板、批量下载。
- `ProblemDetailView.vue`：新增右侧题包下载卡片。
- `api/problems.ts`：增加题包导入、单题导出、批量操作、批量导出函数。
- 前端对 ZIP 下载统一使用 `getBlob` + 临时下载链；不使用 CDN/第三方组件。

## 11. 验收标准

- [ ] 导入 Hydro/kapOJ 包时可勾选“导入后立即公开”，新建和覆盖题都逐题发布，失败不回滚并显示原因。
- [ ] 无 `problem.publish` 的用户不能提交 `publish=true`。
- [ ] 管理员在题库页可多选并批量公开、隐藏、删除、下载。
- [ ] 批量删除按现有规则物理删除/下线/逐题失败，不影响其他题目。
- [ ] 单题详情页可下载包含题面、样例、测试数据、checker、附件、图片、答案、题解的 ZIP。
- [ ] 批量下载得到一个总 ZIP，内部按 slug 组织，可再次导入。
- [ ] 导出包不包含数字 ID、提交统计、创建者 ID、用户提交等运行时数据。
- [ ] 越权下载、越权批量操作、Zip Slip、超限包、非法 manifest 被拒绝。
- [ ] OpenAPI、前端类型、后端/前端测试、lint、build 全部通过。
