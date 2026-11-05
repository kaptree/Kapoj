# M4 学习路线模块设计

> 依据 `DEVELOPMENT.md` 3.8、4.5 与 `docs/任务清单.md` M4-6/M4-7。
> 本模块是 M4 的第二阶段：学习路线树、服务端进度、官方路线管理、用户自定义路线与前端树页面。

## 1. 目标

- **官方路线**：题目管理员/超级管理员创建树状学习路线，按算法模块/阶段组织题目。
- **用户自定义路线**：所有登录用户可创建私有路线；作者可公开；管理员可下架/删除违规公开路线，无审核队列。
- **树与解锁**：节点支持父节点自关联；根节点默认解锁，子节点仅在父节点完成后解锁。
- **进度**：服务端根据 AC 提交自动计算节点 `solved_count`；`required_count` 道题完成即节点完成；不接受客户端上报。
- **前端**：路线列表、树状详情、题目清单、官方管理页、用户自定义编辑页；使用 PrimeVue 4。
- **数据完整性**：被活跃路线引用的题目不能下线/删除；已完成节点锁存，不因改题回退。

## 2. 非目标

- 不实现审核队列/审核状态；公开自定义路线由管理员事后下架。
- 不实现评论、点赞、关注、通知（M4 后续阶段）。
- 不实现路线内题目讨论、题解。
- 不实现学习计划/日历/提醒。
- 不实现路线导入/导出。
- 不实现 M5 功能。

## 3. 已确认产品规则

### 3.1 自定义路线

- 任意登录用户可创建自定义路线，默认 `is_public=false`（私有）。
- 作者可在编辑页切换公开/私有；公开后出现在路线列表，其他用户可查看树与用自己的进度挑战。
- 管理员（题目管理员/超级管理员）可下架/删除任何公开自定义路线；不提供审核状态。
- 作者可编辑/删除自己的路线；管理员可编辑/删除任意路线。
- **下架与恢复**：`DELETE` 为软删除（`is_active=false`）。作者删除自己的路线后，该路线从作者常规列表/编辑页消失；只有管理员可通过 `includeInactive=true` 查看并恢复。管理员下架的公开路线同样只对管理员可见/可恢复，作者不能自行重新上架。
- `isActive` 仅管理员可在更新时设置；非管理员提交的 `isActive` 被忽略/拒绝。`PUT` 时未提交的 `isActive`/`isOfficial` 保持原值。
- 官方路线 `isOfficial=true` 强制 `isPublic=true`；作者不能把自己的自定义路线改为官方。
- 自定义路线不能标记 `is_official=true`；官方标记仅管理员创建官方路线时使用。
- `isOfficial` 创建后不可修改：管理员也不能把已有自定义路线提升为官方或取消官方标记，更新提交不同值返回 40001“不能修改路线官方标记”。

### 3.2 官方路线

- 仅 `problem_admin`/`super_admin` 可创建/编辑/删除官方路线。
- 官方路线始终公开（`is_public=true` 强制），可被管理员下架（`is_active=false`）。
- 官方路线的节点/题目/排序由管理员维护。

### 3.3 进度与完成

- 节点 `solved_count` = 用户在该节点题目集合中去重 AC 的题数（全时间，不限比赛/任务）。
- `completed = solved_count >= required_count`；`required_count` 默认等于题目数。
- 根节点未完成时 `active`；子节点父节点 `completed` 时 `active`，否则 `locked`。
- 已完成节点锁存：管理员/作者之后修改节点题目或提高 `required_count`，已完成用户保持 `completed`；`solved_count` 保留完成时的值，不因改题回退；未完成/进行中节点按新配置重算。
- 树 DTO 同时返回当前 `requiredCount` 与锁存的 `solvedCount`；已完成节点前端以“已完成”状态为准，可显示“完成时 X 题”避免进度条数字误解。
- 游客：根节点 `active`，其他 `locked`；不写用户进度。
- 读取树时对登录用户重新计算并 upsert；AC 事件 worker 也调用同一 `EvaluateUser`，保证及时更新。
- 不接受客户端上报进度。

### 3.4 题目下线守卫

- 被任一活跃学习路线节点引用的题目不能下线或物理删除。
- 题目服务通过可选 `LearningGuard` 在更新/删除前校验，返回 40001。
- 管理员必须先调整/删除路线节点，才能下线题目。

## 4. 数据模型

### 4.1 learning_paths

```text
learning_paths (
  id, title, description_md, cover_url,
  is_official, is_public, is_active, created_by,
  sort_order, created_at, updated_at
)
```

- `is_active=false` 为软删除；官方/自定义都适用。
- `created_by` 必填（官方为管理员，自定义为作者）。
- `is_official=true` 时强制 `is_public=true`。
- 列表排序：官方优先、`sort_order`、`id`。

### 4.2 learning_path_nodes

```text
learning_path_nodes (
  id, path_id, parent_id, title, description_md,
  problem_ids_json, sort_order, required_count,
  created_at, updated_at
)
```

- `parent_id` 自关联，必须属于同一 path；根节点 NULL。
- `problem_ids_json` 1–50 个不重复、存在且公开的题目。
- `required_count` 1–题目数；默认题目数。
- 同一父节点下按 `sort_order,id` 排序。
- 删除仅允许叶子节点；有子节点返回 40001。
- 节点树不允许环，最大深度 8（防御性限制）。

### 4.3 user_learning_progress

```text
user_learning_progress (
  id, user_id, path_id, node_id, status,
  solved_count, completed_at, updated_at
)
-- 唯一约束：(user_id, node_id)
```

- `status`：`locked`/`active`/`completed`。
- 进度由服务端写；删除节点时级联删除该节点进度。
- 删除/下架路线保留进度行，重新上架时可继续使用。

## 5. API

### 5.1 路线

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| GET | `/api/learning-paths` | 公开（可选登录） | 官方 + 公开自定义 + 自己创建的私有；`mine=true` 只看自己；`includeInactive=true` 仅管理员 |
| GET | `/api/learning-paths/:id/tree` | 公开（可选登录） | 树 + 题目 + 当前用户进度；私有路线仅作者/管理员 |
| POST | `/api/learning-paths` | 登录 | 自定义默认私有；管理员可创建官方 |
| PUT | `/api/learning-paths/:id` | 作者/管理员 | `isOfficial` 仅创建时管理员可设、创建后不可修改；普通用户不能设置 |
| DELETE | `/api/learning-paths/:id` | 作者/管理员 | 软删除 |
| POST | `/api/learning-paths/:id/nodes` | 作者/管理员 | 创建节点 |
| PUT | `/api/learning-paths/:id/nodes/:nodeId` | 作者/管理员 | 更新节点（含 parent/sort/required） |
| DELETE | `/api/learning-paths/:id/nodes/:nodeId` | 作者/管理员 | 仅叶子 |

### 5.2 DTO

```json
{
  "id": 1,
  "title": "算法入门",
  "descriptionMd": "...",
  "coverUrl": "",
  "isOfficial": true,
  "isPublic": true,
  "isActive": true,
  "author": { "id": 1, "username": "admin", "nickname": "管理员" },
  "nodeCount": 5,
  "myCompletedCount": 2,
  "createdAt": "..."
}
```

Tree：

```json
{
  "path": { "...": "LearningPathItem" },
  "nodes": [
    {
      "id": 1,
      "parentId": null,
      "title": "基础语法",
      "descriptionMd": "...",
      "problems": [
        { "problemId": 1, "title": "...", "slug": "...", "difficulty": 1 }
      ],
      "requiredCount": 2,
      "solvedCount": 1,
      "status": "active",
      "children": []
    }
  ]
}
```

- 游客 `solvedCount=0`、根 `active`、其余 `locked`。
- 私有路线对非作者/非管理员返回 40400。
- 题目只返回 `problemId/title/slug/difficulty`，不返回题解。
- 公开树的 `problemIds` 对非作者/非管理员只包含公开题目 id；作者/管理员保留完整存储列表（含已下线/缺失题目）用于编辑。
- 作者/管理员返回 `isActive`、创建者信息；公开列表可不返回敏感字段。

## 6. 进度计算与事件

- `learning.Service.EvaluateUser(ctx,userID)`：
  1. 查询用户可见的所有 active 路线（官方/公开/自己的私有）。
  2. 对每条路线计算节点 AC 数；按父节点完成状态确定 `active/locked`。
  3. upsert `user_learning_progress`：已完成保持 completed；未完成按最新配置更新。
- `GET tree` 对登录用户先 `EvaluateUser`，再读进度。
- `GET /learning-paths` 对登录用户同样先 `EvaluateUser`，保证 `myCompletedCount` 首次加载正确；游客计数为 0。路径数量有限，接受读放大。
- `gamequeue` worker 当前由 gamification 处理；M4-2 使用组合 handler：
  - gamification `HandleGameEvent`
  - learning `HandleGameEvent`（只对 `submission.accepted` 重新计算该用户）
  - 组合器依次调用，任一失败返回错误触发重试；两者幂等。
- 进度计算不依赖客户端时间。

## 7. 题目守卫

- `problem` 服务增加可选 `LearningGuard`：
  - `IsProblemInActivePath(ctx,problemID) (bool,error)`
- `Update` 将 `is_public` 从 true 改为 false、`Delete` 物理删除前检查；命中返回 40001 “题目被学习路线引用，不能下线/删除”。
- Router 注入 learning repository/service；测试覆盖官方/公开/私有自定义路线引用。
- 题目下线后已有 AC 不影响进度；守卫避免新的不可完成节点。

## 8. 前端

### 8.1 路线列表

- `/learning-paths`：官方/公开自定义卡片；筛选“全部/官方/自定义/我创建的”；登录用户显示“新建路线”按钮。
- 卡片：封面（可空）、标题、描述摘要、官方/公开标签、作者、节点数、我的完成数。
- 私有路线仅自己可见，显示“私有”标签。
- 空态/加载/错误/分页（若数量大）。

### 8.2 路线树

- `/learning-paths/:id`：
  - PrimeVue `TreeTable`（或 `Tree` + 详情面板）展示节点层级。
  - 状态：completed 绿色、active 主色、locked 灰色锁图标；`solvedCount/requiredCount` 进度条。
  - 点击节点右侧显示描述、题目清单（题目链接到题库，带返回路线上下文可选）。
  - 游客根 active、其余 locked；私有路线无权限显示 404 页面。
  - 移动端树横向滚动或折叠层级，可读可点。

### 8.3 编辑页

- `/learning-paths/new`、`/learning-paths/:id/edit`：作者/管理员。
- `/admin/learning-paths`：题目管理员/超级管理员管理**所有**路线（官方 + 公开自定义 + `includeInactive` 的下架路线），支持新建官方、编辑、下架/恢复、删除。公开自定义路线的管理员下架/恢复也在这里完成；公开列表/树页面不放下架按钮。
- 表单：标题、描述 Markdown、封面 URL、公开开关（官方强制公开）、排序。
- 节点编辑器：
  - 树形 TreeTable + 每节点“添加子节点/编辑/删除/上移/下移”。
  - 节点弹窗：标题、描述 Markdown、父节点选择（同路线、不可形成环）、题目远程搜索（1–50）、`requiredCount`（默认题目数、范围 1–题目数）、排序。
  - 删除仅叶子；有子节点禁用并提示。
  - 已领取/进度存在不冻结编辑；完成锁存在后端，前端提示“已完成用户不受影响”。
- 官方管理页仅 `problem_admin`/`super_admin`；自定义编辑仅作者/管理员。
- 管理员可编辑/下架/恢复/删除任意路线；作者编辑自定义路线时不能修改 `isOfficial`/`isActive`。
- 使用 PrimeVue 4，不新增 UI 依赖。

## 9. 事件与缓存

- 学习路线列表/树不做 Redis 缓存；数据量小，直接数据库，避免进度缓存失效复杂化。
- AC 事件触发进度更新；读取补算兜底。
- 题目守卫检查使用数据库查询；路径/节点数量有限。

## 10. 测试与验收

### 10.1 后端

- 模型/迁移/唯一约束。
- 官方/自定义权限：作者可编辑、他人不能、管理员可编辑任意、普通用户不能设置官方。
- 私有路线不可见/树 404；公开路线游客可看。
- 节点校验：父节点同路线、无环、最大深度、required_count 范围、题目公开/数量。
- 只允许删除叶子；软删除路线/节点进度保留/级联。
- 进度：根 active/子 locked；部分完成/完成；已完成锁存；改题后未完成重算；游客状态。
- AC 事件更新；读取补算。
- LearningGuard：题目被活跃路线引用时不能下线/删除；移除引用后可下线。
- 任务清单/OpenAPI/文档。

### 10.2 前端

- 工具函数：节点状态标签、进度百分比、树构建/扁平化、私有/官方标签。
- 组件测试：TreeTable 空态/锁定/完成状态；编辑页节点校验；私有路线无权限提示。
- lint/type-check/build。

### 10.3 真实浏览器 E2E

1. 管理员创建官方路线（根节点 + 两个子节点），用户看到根 active、子 locked。
2. 用户 AC 根节点所需题目，刷新后根 completed、子 active；完成子节点。
3. 普通用户创建私有路线、添加节点/题目，仅自己可见；切换公开后游客可见；管理员可下架。
4. 已完成节点改题/提高 required_count 后已完成用户不回退；未完成用户按新配置重算。
5. 题目被路线引用时不能下线/删除；移除节点后可下线。
6. 移动端树/编辑弹窗可操作；无 CDN/控制台错误。

## 11. 交付拆分

### M4-2A：后端模型、CRUD 与权限

- learning_paths/nodes/progress 模型/迁移。
- 官方/自定义权限、私有/公开可见性、节点校验/树约束。
- API、OpenAPI、后端测试。

### M4-2B：进度、事件与题目守卫

- EvaluateUser、树进度、完成锁存。
- gamequeue 组合 handler。
- problem LearningGuard。
- 测试。

### M4-2C：前端列表/树/编辑/管理

- API/types/utils。
- `/learning-paths` 列表、`/:id` 树、`/new`/edit、`/admin/learning-paths`。
- PrimeVue TreeTable/对话框/远程选题。
- 前端测试。

### M4-2D：文档、全量检查与 E2E

- 更新 DEVELOPMENT/任务清单/开发日志/README/运行手册/PrimeVue migration UI-31。
- 全量检查、真实浏览器 E2E。

## 12. 风险与取舍

- **自定义公开无审核**：公开内容靠管理员事后治理；可能短时出现低质量路线。
- **题目守卫限制题目下线**：管理员需先调整路线；比题目消失导致路线不可完成更安全。
- **完成锁存**：已完成用户不因改题回退，管理员改题只影响未完成用户；文档明确。
- **无路线缓存**：每次读取查库并补算，规模小可接受；若路线/用户量增长再缓存。
- **树深度 8**：防御性限制，足够算法模块/阶段层级；更深的树需调整。
- **用户自定义路线质量**：不提供审核/评分；后续社交阶段可补充。

## 13. Builder check 决议

1. 自定义公开规则：默认私有，作者可公开，管理员可下架/删除；无审核状态。
2. 官方路线：仅 problem_admin/super_admin；强制公开。
3. 进度锁存：已完成节点不回退；未完成按新配置重算。
4. 题目守卫：被任何活跃路线引用（含私有）都不能下线/删除。
5. 叶子删除：有子节点返回 40001。
6. 树读取：登录用户先 EvaluateUser，游客根 active/其余 locked。
7. 事件：组合 handler 调 gamification + learning，均幂等。
8. 前端：PrimeVue TreeTable/编辑弹窗；不新增 UI 依赖。

## 14. 留给实施计划的决定

- 具体包拆分（learning/learningapi）。
- 树构建/扁平化工具与 TreeTable 数据适配。
- 编辑器节点排序/拖拽是否实现（M4-2C 可先用上移/下移按钮）。
- OpenAPI schema 字段描述与示例。
- E2E 夹具与官方/自定义路线数据组织。

以上不改变本文确认的产品行为、权限与进度语义。
