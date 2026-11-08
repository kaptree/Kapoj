# M4-3 轻量社交互动设计

> 目标：在现有解题思路论坛之上交付题解帖子点赞、用户关注、站内通知，并把评论（论坛回复）、点赞、关注接入活跃度与共享分；不新增通用评论表、不做关注动态流。

## 1. 背景与现状

- 现有“解题思路”论坛：`problem_solution_posts` 帖子 + `problem_solution_replies` 回复，支持软删除；帖子列表/详情已有作者摘要。
- `activity_logs` 已支持提交/参赛/签到；`shared_items` 已支持比赛共享分，`type=solution` 与 `like_count` 已为题解点赞预留，但恒为 0。
- 用户主页已有公开资料；没有关注关系、通知表或社交 API。
- 顶部栏 `AppShell` 有主题切换与用户菜单，没有通知入口。

## 2. 目标与非目标

### 目标

- **题解点赞**：登录用户可对解题思路帖子点赞/取消；帖子列表/详情返回点赞数与本人是否已赞。
- **关注**：登录用户可关注/取消关注其他 active 用户；公开显示粉丝数/关注数/本人是否已关注，可查看粉丝与关注列表。
- **站内通知**：回复我的帖子、赞我的帖子、关注我时，给接收者生成站内通知；支持未读数、分页列表、单条/全部已读。
- **活跃度**：论坛回复（评论）+3、点赞 +1、关注 +2，每类每日上限 20 分；重复点赞/关注/同一回复不重复计分。
- **共享分**：题解帖子每获得 1 个赞，作者共享分 +5（`shared_items.type=solution`）；取消赞扣回；帖子删除移除共享项。
- **前端**：题解列表/详情点赞按钮；用户主页关注按钮、粉丝/关注列表；顶部通知铃铛与 `/notifications` 页。
- 复用论坛回复作为“评论”，不新增独立评论系统。

### 非目标

- 不做题目/比赛/学习路线通用评论，不做评论点赞、回复嵌套、@提醒、富文本。
- 不做关注动态流、推荐、私信、邮件/推送通知、实时 SSE/WebSocket 通知。
- 不做通知删除/归档、免打扰设置、点赞/关注排行榜（共享榜已有）。
- 不做匿名/游客互动；所有写操作登录。
- 不新增 RBAC 权限码；不改变现有论坛编辑/删除权限。

## 3. 数据模型

### 3.1 problem_solution_post_likes

```text
problem_solution_post_likes (
  id, user_id, post_id, created_at
)
-- unique(user_id, post_id)
-- index(post_id)
```

- 点赞表放在 `problem` 域；帖子列表/详情直接用它计算 `likeCount`/`likedByMe`。
- 帖子软删除时删除该帖全部点赞；写操作校验帖子存在且未删除。

### 3.2 user_follows

```text
user_follows (
  id, follower_id, followee_id, created_at
)
-- unique(follower_id, followee_id)
-- index(followee_id)
-- check follower_id <> followee_id
```

- 只能关注 active 用户；不能关注自己。
- 取消关注不删除历史活跃度/通知。

### 3.3 notifications

```text
notifications (
  id, user_id, actor_id, type, dedupe_key UNIQUE,
  target_type, target_id, link, read_at, created_at
)
-- index(user_id, created_at DESC)
-- index(user_id, read_at)
```

- `type`: `solution_reply` / `solution_like` / `follow`。
- `user_id` 接收者；`actor_id` 触发者；`link` 为站内相对路径（如 `/problems/1/solutions/2`、`/users/3`）。
- `dedupe_key`：
  - 回复：`reply:<replyId>`（每条回复一条通知）。
  - 点赞：`like:<actorId>:<postId>`（同一人对同一帖只通知一次，取消/重复赞不重复）。
  - 关注：`follow:<actorId>:<followeeId>`（同一对关注关系只通知一次，取消/重复关注不重复；不同接收者不会冲突）。
- 自己触发自己的动作不生成通知。
- 通知只读自己；标记已读幂等。

## 4. 活跃度与共享分

### 4.1 活跃度

| action | 分值 | ref_type / ref_id | 每日上限 |
|---|---|---|---|
| `solution_reply` | 3 | `solution_reply` / replyID | 20 |
| `solution_like` | 1 | `solution_post` / postID | 20 |
| `follow` | 2 | `user` / followeeID | 20 |

- 复用 `activity_logs` 唯一约束 `(user_id, action, ref_type, ref_id)`：重复点赞/关注/同一回复不重复计分；取消赞/取消关注不扣分。
- 每日上限按本地日期与 action 求和；达到上限 no-op，不影响主操作。
- 写活跃度 best-effort；失败只日志，不阻断点赞/关注/回复。
- 成功写活跃度后递增活跃度榜缓存版本。

### 4.2 共享分

- 帖子被赞：`shared_items` upsert `(type='solution', ref_id=postID, user_id=postAuthor, like_count += 1)`。
- 取消赞：`like_count = max(0, like_count - 1)`；不删除行（0 也不影响榜单）。
- 帖子删除：删除该帖 `shared_items` 行与点赞；共享榜不再计入。
- 点赞/取消/删除成功后 best-effort 递增共享榜缓存版本。
- 共享分公式沿用 `20 * 比赛共享数 + 5 * 题解点赞数`；无需改共享榜查询。

## 5. API

### 5.1 题解点赞

| 方法 | 路径 | 认证 | 说明 |
|---|---|---|---|
| POST | `/api/problems/:id/solutions/:postId/like` | 登录 | 点赞；返回 `{liked:true, likeCount}`；重复点赞幂等 |
| DELETE | `/api/problems/:id/solutions/:postId/like` | 登录 | 取消赞；返回 `{liked:false, likeCount}`；未赞幂等 |

- 先复用讨论服务校验题目/帖子可见性（隐藏题 404、帖子不存在 404）；再点赞。
- 帖子列表项/详情新增 `likeCount`、`likedByMe`（游客 false）。
- 自己赞自己的帖子：禁止，返回 40001「不能点赞自己的帖子」；不产生点赞/通知/活跃度/共享分。
- 同一用户重复点赞幂等；取消赞后再次点赞重新计入共享分，但活跃度因唯一 ref 不再加分。

### 5.2 关注

| 方法 | 路径 | 认证 | 说明 |
|---|---|---|---|
| POST | `/api/users/:id/follow` | 登录 | 关注；返回 `{following:true, followersCount}`；重复幂等；自己/不存在/非 active → 40001/404 |
| DELETE | `/api/users/:id/follow` | 登录 | 取消关注；返回 `{following:false, followersCount}`；未关注幂等 |
| GET | `/api/users/:id/social` | 可选 | `{followersCount, followingCount, isFollowing}`；游客 `isFollowing=false` |
| GET | `/api/users/:id/followers?page=&page_size=` | 可选 | 粉丝分页；公开用户摘要 |
| GET | `/api/users/:id/following?page=&page_size=` | 可选 | 关注分页；公开用户摘要 |

- 用户摘要字段：`id, username, nickname, avatarUrl, rating`；不含 email。
- 关注自己返回 40001；目标用户不存在 404；目标非 active 40001。
- 公开 `social`/`followers`/`following` GET 对已存在但封禁/非 active 的用户仍返回计数/列表（与 `/users/:id/profile` 一致）；只有写关注要求 active。
- 粉丝/关注列表按关注时间倒序、id 倒序。

### 5.3 通知

| 方法 | 路径 | 认证 | 说明 |
|---|---|---|---|
| GET | `/api/notifications?page=&page_size=` | 登录 | 自己的通知，按 created_at DESC,id DESC |
| GET | `/api/notifications/unread-count` | 登录 | `{count}` |
| POST | `/api/notifications/:id/read` | 登录 | 标记单条已读；非本人 404 |
| POST | `/api/notifications/read-all` | 登录 | 全部标记已读 |

`NotificationItem`：

```json
{
  "id": 1,
  "type": "solution_like",
  "actor": { "id": 2, "username": "bob", "nickname": "Bob" },
  "targetType": "solution_post",
  "targetId": 9,
  "link": "/problems/1/solutions/9",
  "readAt": null,
  "createdAt": "..."
}
```

- 通知标题由前端按 type 生成（“回复了你的解题思路”“赞了你的解题思路”“关注了你”）；后端返回 `link`。
- 规范映射：

| type | targetType | targetId | link | dedupe_key |
|---|---|---|---|---|
| solution_like | `solution_post` | postID | `/problems/<problemId>/solutions/<postId>` | `like:<actorId>:<postId>` |
| solution_reply | `solution_post` | postID | `/problems/<problemId>/solutions/<postId>` | `reply:<replyId>` |
| follow | `user` | actorID（关注者） | `/users/<actorId>` | `follow:<actorId>:<followeeId>` |
- 回复通知在论坛回复创建成功后 best-effort 写入；点赞/关注通知在点赞/关注事务内写入。
- 帖子软删除后，通知保留（点击可能 404）；不做历史清理，避免误删其他通知。

## 6. 处理流程与一致性

### 6.1 点赞

1. 登录 + 题目/帖子可见性校验。
2. 事务：
   - `INSERT problem_solution_post_likes ... ON CONFLICT DO NOTHING`；未插入（已赞）直接返回当前计数。
   - 插入成功则 `shared_items` upsert +1。
   - 若帖子作者 ≠ 点赞者，插入通知（`dedupe_key` 冲突忽略）。
3. 提交后 best-effort：活跃度 `solution_like`、共享榜版本递增。
4. 取消赞：事务删除 like；若删除成功 shared_items -1；不写活跃度/通知；返回计数。

### 6.2 回复（评论）

- 现有 `CreateReply` 成功后：
  - best-effort 活跃度 `solution_reply`（ref=replyID）。
  - 若帖子作者 ≠ 回复者，插入通知 `dedupe_key=reply:<replyID>`，link 指向帖子详情。
- 回复编辑/删除不改变活跃度/通知。

### 6.3 关注

1. 校验目标存在、active、非自己。
2. 事务插入 `user_follows ON CONFLICT DO NOTHING`；插入成功则写通知（dedupe `follow:<actor>`）。
3. 提交后 best-effort 活跃度 `follow`。
4. 取消关注：删除关系；不扣活跃度/不删通知。

### 6.4 帖子删除

- 现有帖子软删除后，best-effort 清理：删除该帖全部点赞、`shared_items(type=solution,ref_id=postID)`、递增共享榜版本。
- 活动日志保留（已获得的分数不退）。
- 通知保留。

## 7. 前端

### 7.1 题解列表/详情

- 帖子列表卡片显示点赞数；登录用户显示点赞按钮（已赞高亮），点击调用点赞/取消；乐观更新失败回滚并用 Toast 提示。
- 详情页同样显示点赞按钮/计数；游客点击跳登录（`redirect` 当前路径）。
- `likeCount`/`likedByMe` 由 API 返回；不在前端自行推导。

### 7.2 用户主页

- 资料头部显示粉丝数/关注数；点击数字打开 Dialog（粉丝/关注两个 Tab，分页）。
- 登录且非本人显示“关注/已关注”按钮；游客点击跳登录。
- 关注成功/取消后刷新 social 状态；失败 Toast。
- 用户摘要（ID/昵称/头像/Rating）链接到 `/users/:id`。

### 7.3 通知

- 顶部栏 `AppShell` 在用户菜单旁增加通知铃铛（仅登录显示）；未读数 badge；点击进入 `/notifications`。
- 铃铛挂载时拉取未读数；每 60 秒轮询一次（页面隐藏时暂停可选）；路由切换后刷新；组件卸载清理计时器。
- `/notifications` 页面：分页列表、类型标题、actor、时间、已读/未读样式；点击标记已读并跳转 link；“全部已读”按钮；空态/错误/加载。
- 路由 `notifications` 需登录；游客访问跳登录。
- 不新增导航项（铃铛入口即可）。

## 8. 权限与安全

- 所有写操作需登录；隐藏题帖子沿用讨论区可见性（无 `problem.view_hidden`/super 404）。
- 关注目标必须 active；不能关注自己；通知只能读自己的。
- 帖子编辑/删除权限不变；删除帖子额外清理点赞/共享项。
- 用户摘要不含 email；通知链接只使用站内相对路径。
- 速率/防刷：点赞/关注唯一约束 + 活跃度每日上限；不新增 IP 限流。

## 9. 配置与迁移

- 新增表：`problem_solution_post_likes`、`user_follows`、`notifications`；加入 `db.Migrate` 与迁移测试。
- `activity_logs.action` 新值 `solution_reply`/`solution_like`/`follow`；`ref_type` 新值 `solution_reply`/`solution_post`/`user`。
- `SolutionPostItem`/`SolutionPostDetail` 增加 `likeCount`/`likedByMe`；新增关注/通知 DTO。
- 无新环境变量、无新 RBAC 权限码。
- OpenAPI/生成类型、文档更新。

## 10. 测试与验收

### 10.1 后端

- 点赞：首次/重复/取消/重复取消；计数；likedByMe；隐藏题/不存在帖子 404；自己赞自己 40001；共享项增减；活跃度唯一/上限；通知只一次；帖子删除清理。
- 关注：自己/不存在/非 active 写入；重复关注；取消/重复取消；粉丝/关注计数与 isFollowing；列表分页/摘要；封禁用户 GET 仍可读；活跃度/通知。
- 回复：通知给帖子作者（自己回复自己无通知）；活跃度 +3 唯一/上限；编辑/删除不改分。
- 通知：列表/未读/单条/全部已读；非本人 404；分页/排序；dedupe。
- 共享榜：solution like_count 计入共享分；缓存版本递增；删除帖子后移除。
- 前端无关的后端全量检查。

### 10.2 前端

- 题解列表/详情点赞、游客跳登录、失败回滚。
- 用户主页关注按钮/计数/粉丝关注 Dialog/分页/空态。
- 通知铃铛未读数、轮询/卸载、通知页已读/全部已读/跳转/空态。
- 生成类型/lint/build。

### 10.3 真实冒烟

- 两个临时用户：A 发帖/回复；B 点赞、回复、关注 A；A 收到三类通知；未读数/已读；A 的活跃度 +3/+1/+2 上限与唯一；A 的共享分 +5/赞；取消赞/关注后共享分/关系变化；删除帖子清理点赞/共享项。
- 隐藏题帖子点赞/关注权限；游客行为；粉丝/关注列表。
- 清理临时用户/帖子/回复/点赞/关注/通知/活跃度/共享项/审计；不删除既有数据。

## 11. 交付拆分

- M4-3A：点赞模型/服务/API、讨论 likeCount/likedByMe、共享分/活跃度/通知钩子。
- M4-3B：关注模型/服务/API、通知服务/API、活跃度接入。
- M4-3C：OpenAPI/生成类型。
- M4-3D：前端点赞/关注/通知铃铛与通知页。
- M4-3E：文档、全量检查、真实冒烟。

## 12. 留给实施计划的决定

- 社交服务包拆分（`social` 包或 problem/user 扩展）与依赖注入方式。
- 通知 link 生成位置与帖子删除后的通知保留策略。
- 点赞列表计数的批量查询方式。
- 通知轮询间隔/页面隐藏策略的前端细节。
