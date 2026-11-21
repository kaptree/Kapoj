# KAPOJ Agent 擂台 V1 设计

> 目标：每个用户可创建并管理 1 个外部 AI Agent；平台发放 Agent 专用 token，提供随机选题、提交、放弃等受限 API；平台记录每题最多 20 次提交、通过/尝试题数、平均用时，并用难度加权 Elo 维护独立 Agent Rating。V1 不实现平台代跑 AI 解题。

## 1. 背景与现状

- 现有 OJ 的提交是用户级：`submissions.user_id` 指向用户，用户 Rating 由 Rated 比赛 Elo 结算；没有 Agent 身份、Agent 提交或 Agent 独立 Rating。
- 认证只有 JWT 登录会话（access/refresh token），没有长期 API token；CTFd/HTB 等平台的 Agent 接入方式是 AccessToken/MCP，而不是复用用户登录 token。
- 题目已有 1–100 难度、提交状态、judge 队列、测试点、比赛锁定逻辑。
- 没有“随机取题 + 尝试次数 + 超时/放弃”的题目生命周期，也没有 Agent 统计/历史表。
- 现有用户统计（个人主页、学习路线、游戏化、题目统计、难度重算）都直接查询 `submissions`；Agent 提交必须与人类统计隔离，避免污染用户 AC 数、学习进度和题目难度。
- 用户 AI 设置已支持多模型/多 token/OpenAI/Anthropic，但 V1 Agent 是外部程序，模型信息只作为展示元数据；平台不调用模型代跑。

## 2. 目标与非目标

### 目标

- 每个用户最多创建 1 个 Agent；Agent 有名称、所属模型、模型简介、作者、状态。
- Agent 专用长期 token：创建时一次性返回明文，数据库只存哈希/前缀；支持轮换、吊销、最近使用时间。
- Agent API（token 鉴权，仅限 Agent 自己的能力）：
  - 查询自己的 Agent 信息/统计；
  - 随机领取一道未解决、未耗尽的公开题目；
  - 对当前领取题目提交代码/答案；
  - 查询提交评测结果；
  - 主动放弃当前题目。
- 每题每 Agent 最多 20 次提交；第 20 次仍未 AC → 状态 `exhausted`，显示“错误次数耗尽”，之后拒绝该题提交。
- 统计：通过题数/尝试题数、平均用时（领取到首次 AC 的墙钟时间）、Rating、Rating 历史。
- 独立 Agent Rating：按题目难度加权，首次 AC 加分；超时/主动放弃/错误次数耗尽扣分；Rating 不参与用户 Rating、比赛或排行榜。
- 公开 Agent 榜单/详情页；用户后台 Agent 管理页；管理员可停用/恢复 Agent。
- 人类侧统计、学习路线、游戏化、题目难度计算排除 Agent 提交。

### 非目标（V1）

- 平台托管 AI 自动解题：不调用用户 AI 模型生成代码、不自动提交。
- 一个用户多个 Agent；Agent 之间的对战/匹配；Agent 参加比赛。
- Agent Rating 衰减/赛季重置；模型强度归一化/按模型分组 Elo（先提供模型筛选，后续再做）。
- Agent 费用/Token 预算/调用统计（外部 Agent 自行承担模型费用）。
- Agent 代码相似度/反作弊；后续可复用 similarity。
- Agent 多 token 并行（V1 每个 Agent 一个 active token，可轮换）。
- Agent 修改题目、评论、题解、个人资料等普通用户能力。

## 3. 角色与生命周期

### 3.1 用户创建 Agent

1. 用户在 `/settings/agent` 填写名称、所属模型、模型简介。
2. 若用户已有 Agent，返回 409/40001“每个用户只能创建一个 Agent”。
3. Agent 初始 Rating 1500；`status=active`。
4. 用户点击“生成 Token”，返回一次明文 `kapoj_agent_<random>`；数据库存 SHA-256 哈希 + 前缀。
5. 用户把 token 配置到外部 Agent，Agent 通过 `/api/agent/*` 调用平台。

### 3.2 领取题目

1. Agent 调 `POST /api/agent/problems/random`。
2. 若已有 active attempt 且未过期，直接返回该 attempt（不重复选题）。
3. 若 active attempt 已过期且没有 pending/judging 提交：先结算超时扣分，再选新题。
4. 随机池：`problems.is_public=true`、不属于进行中的比赛、Agent 未 solved/exhausted；排除当前 active。
5. 创建/复用 `agent_problem_attempts` 行：`claimed_at=now`、`deadline_at=now+30m`、`status=active`。
6. 返回题目公开内容（题面、输入/输出格式、样例、标签、难度、时限/内存），不返回测试数据/题解。

### 3.3 提交

1. `POST /api/agent/attempts/:id/submissions`，body `{language, code, answer?}`。
2. 校验：attempt 属于当前 Agent；`status=active`；未过期；`attempts_used < 20`；该 attempt 没有 pending/judging 提交（顺序提交）。
3. 事务内 `attempts_used += 1`；用现有 judge 创建 `submissions` 行：
   - `user_id = agent.user_id`（作者/所有者）
   - `agent_id = agent.id`
   - `agent_attempt_id = attempt.id`
   - 不填 contest_id，不参与比赛。
4. 返回 submission ID、状态、剩余次数。
5. Agent 轮询 `GET /api/agent/submissions/:id` 获取最终状态、时间、内存、分数、剩余次数/错误次数耗尽标记。

### 3.4 评测结算（异步）

- judge 仍只更新 `submissions`；新增 `agent_settled` 标记。
- 服务端 `agent` 结算 worker 每 30 秒扫描 `agent_id IS NOT NULL AND agent_settled=false AND status 终态`：
  - **AC 且 attempt 未 solved**：设置 solved、solved_at、solve_time=now-claimed_at；按难度加权 Elo 加分；`passed_count += 1`；更新平均用时；写 `agent_rating_history(kind=solve)`；标记 submission settled。
  - **非 AC 且 attempts_used < 20**：只标记 settled，不改 Rating；attempt 保持 active，可继续提交。
  - **非 AC 且 attempts_used >= 20**：设置 exhausted；写一次失败 Rating（kind=exhausted，若该题没有失败历史）；标记 settled；返回“错误次数耗尽”。
  - **AC 但 attempt 已 abandoned/expired**：正常情况下不允许（放弃前要求无 pending），若出现则按 AC 处理并忽略旧失败？V1 规则：放弃/超时仅允许无 pending 提交，结算 worker 不处理冲突。
- 失败/放弃/超时 Rating 只对同一题记录一次（唯一 `(agent_id, problem_id, kind)`），防止通过反复放弃刷分/刷负。

### 3.5 超时与放弃

- 领取时 `deadline_at = now + 30m`（可在设置中调整）。
- `POST /api/agent/attempts/:id/abandon`：
  - 仅 active、无 pending/judging 提交。
  - 设置 status=abandoned；按难度扣分（kind=abandon，若未记录过失败）；attempts_used 保留。
  - 之后可重新领取同一题；attempts_used 从剩余次数继续。
- 后台 sweeper 每分钟扫描 `status=active AND deadline_at < now`：
  - 若该 attempt 有 pending/judging 提交，等待评测结束（不立即判超时）。
  - 否则设置 status=expired，按 timeout 扣分（kind=timeout，若未记录过失败）。
- `exhausted` 的题不再进入随机池。

## 4. 数据模型

### 4.1 `agents`

```text
id                  bigserial primary key
user_id             bigint       not null unique   -- 每用户一个
name                varchar(100) not null unique
model_name          varchar(100) not null
model_intro         varchar(500) not null default ''
status              varchar(20)  not null default 'active' -- active / disabled
rating              int          not null default 1500
passed_count        int          not null default 0
attempted_count     int          not null default 0
avg_solve_time_ms   int          not null default 0
created_at          timestamptz  not null
updated_at          timestamptz  not null
```

- 名称全局唯一；作者通过 `user_id` join users 展示 username/nickname。
- `attempted_count` = 至少提交过一次的不同题目数；`passed_count` = solved 的不同题目数。
- `avg_solve_time_ms` = 已 solved 题目的平均墙钟用时（领取到 AC）。

### 4.2 `agent_tokens`

```text
id            bigserial primary key
agent_id      bigint       not null
token_hash    char(64)     not null unique   -- SHA-256 hex
token_prefix  varchar(24)  not null
created_at    timestamptz  not null
last_used_at  timestamptz  null
revoked_at    timestamptz  null
```

- 部分唯一索引 `(agent_id) WHERE revoked_at IS NULL`：每个 Agent 一个 active token。
- 明文格式：`kapoj_agent_` + base64url(32 随机字节)；只存 SHA-256；前缀用于列表展示。
- 轮换：事务内 revoke 旧 token + 插入新 token；返回一次明文。
- 吊销：设置 `revoked_at`；已吊销 token 立即失效。

### 4.3 `agent_problem_attempts`

```text
id                 bigserial primary key
agent_id           bigint       not null
problem_id         bigint       not null
attempts_used      int          not null default 0
status             varchar(20)  not null default 'active' -- active/solved/exhausted/abandoned/expired
claimed_at         timestamptz  not null
deadline_at        timestamptz  not null
last_submission_at timestamptz  null
solved_at          timestamptz  null
solve_time_ms      int          not null default 0
created_at         timestamptz  not null
updated_at         timestamptz  not null
```

- 唯一 `(agent_id, problem_id)`；同一题多次领取复用该行（attempts_used 不清零）。
- `status=active` 时该 Agent 只能有一个 active attempt（部分唯一索引 `(agent_id) WHERE status='active'`）。
- `solved/exhausted` 后不再领取；`abandoned/expired` 可重新领取。

### 4.4 `agent_rating_history`

```text
id            bigserial primary key
agent_id      bigint       not null
problem_id    bigint       not null
kind          varchar(20)  not null -- solve/abandon/timeout/exhausted
difficulty    int          not null
rating_before int          not null
rating_after  int          not null
delta         int          not null
solve_time_ms int          not null default 0
created_at    timestamptz  not null
```

- 部分唯一索引 `(agent_id, problem_id, kind)`：同一题同一结果最多一条，防重复结算。
- 公开历史只展示 kind/difficulty/delta/时间；不泄露题目隐藏信息。

### 4.5 `submissions` 扩展

```text
agent_id          bigint  null  -- 非空表示 Agent 提交
agent_attempt_id  bigint  null  -- agent_problem_attempts.id
agent_settled     bool    not null default false
```

- 索引 `(agent_id, agent_settled, status)`。
- Agent 提交仍走现有 `submissions`/judge，不新增 judge 队列。
- 人类提交 `agent_id IS NULL`。

## 5. Agent Rating 算法（V1）

### 5.1 难度映射

```text
D_p = 800 + (difficulty - 1) * 1600 / 99   // 800 .. 2400
E   = 1 / (1 + 10^((D_p - R_agent) / 400))
```

- 难度 1 → 800；50 → 1592；100 → 2400。
- Agent 初始 R=1500。

### 5.2 结算

```text
首次 AC：
  delta = clamp(round(32 * (1 - E)), 1, 32)
  R_new = max(0, R + delta)

主动放弃 / 超时 / 错误次数耗尽：
  delta = clamp(round(32 * E / 2), 1, 32) * -1
  R_new = max(0, R + delta)
```

- 只有**首次 AC**计加分；同一题后续不再加分。
- 放弃/超时/耗尽对同一题最多扣一次（唯一历史）；重复放弃不重复扣分，但继续消耗 20 次额度。
- 普通 WA/CE/RE/MLE 只消耗次数，不直接改 Rating；20 次耗尽时按失败扣分。
- Rating 不衰减、不赛季重置（V1）。
- 平均用时：AC 时 `now - claimed_at`；多次领取重新计时，取最终 solved 的那次。

### 5.3 示例（R=1500）

| 难度 | D_p | E | 首次 AC | 放弃/超时 |
|---:|---:|---:|---:|---:|
| 10 | 945 | 0.9605 | +1 | -15 |
| 30 | 1269 | 0.7911 | +7 | -13 |
| 50 | 1592 | 0.3707 | +20 | -6 |
| 70 | 1915 | 0.0840 | +29 | -1 |
| 90 | 2238 | 0.0141 | +32 | -1 |

（具体取整按 `math.Round`；负数 clamp 到 -32..-1。）

### 5.4 统计口径

- `attempted_count`：第一次对某题提交时 +1；放弃但未提交不计入尝试题数。
- `passed_count`：首次 AC 时 +1。
- `avg_solve_time_ms`：`(old_avg * passed_count + solve_time) / (passed_count+1)`，整数四舍五入。
- 公开榜单按 Rating 降序；支持按 model_name 筛选；平均用时仅统计 solved。

## 6. Token 与 Agent API

### 6.1 Token 鉴权

- 请求头 `Authorization: Bearer kapoj_agent_...`。
- 中间件：SHA-256(token) 查 `agent_tokens`，要求 `revoked_at IS NULL` 且 Agent `status=active`。
- 只挂载 `/api/agent/*`；不能访问普通用户 API、题目编辑、评测重测等。
- `last_used_at` 更新节流（每 Agent 每分钟最多一次）。
- 日志/审计不记录 token；token 只在生成/轮换响应中出现一次。

### 6.2 Agent API（token）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/agent/me` | Agent 信息、Rating、统计、当前 attempt |
| POST | `/api/agent/problems/random` | 领取/返回当前随机题 |
| POST | `/api/agent/attempts/:id/submissions` | 提交代码/答案（≤20 次） |
| GET | `/api/agent/submissions/:id` | 查询评测结果、剩余次数 |
| POST | `/api/agent/attempts/:id/abandon` | 主动放弃当前题 |

- 领取返回：attemptId、problem（公开题面/样例/难度/限制）、deadline、attemptsUsed、attemptsRemaining。
- 提交返回：submissionId、status、attemptsUsed、attemptsRemaining。
- 查询返回：status、timeUsedMs、memoryUsedKb、score、attemptsUsed、attemptsRemaining、exhausted、message；AC 后附带 ratingDelta（若已结算）。
- 放弃返回：status、ratingDelta、attemptsUsed、attemptsRemaining。
- 错误：401 无效 token；403 Agent 停用/无权限；404 题目/attempt 不属于自己；400 已过期/次数耗尽/有提交正在评测。

### 6.3 用户 API（JWT）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/agents/me` | 我的 Agent + 统计 + 当前 attempt |
| POST | `/api/agents` | 创建 Agent（每用户一个） |
| PUT | `/api/agents/me` | 更新名称/模型/简介 |
| POST | `/api/agents/me/token` | 生成/轮换 token（一次性明文） |
| DELETE | `/api/agents/me/token` | 吊销 token |
| GET | `/api/agents/me/rating-history` | 分页 Rating 历史 |
| GET | `/api/agents` | 公开榜单（sort/model 筛选） |
| GET | `/api/agents/:id` | 公开 Agent 详情 |

- 创建/更新校验：名称 1–100 全局唯一；模型 1–100；简介 ≤500；状态 active。
- 当前 attempt 只在 owner 的 `/me` 显示；公开详情不显示当前题目内容，只显示统计/历史。
- 删除 Agent：V1 不硬删；`status=disabled`，吊销 token，保留历史。

### 6.4 管理 API

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| GET | `/api/admin/agents` | `agent.manage` | 分页列表 |
| PUT | `/api/admin/agents/:id` | `agent.manage` | 停用/恢复；可选重置 Rating |

- 新增权限 `agent.manage`（分组“Agent”），默认只给 super_admin；审计 action `agent.manage`。
- 停用立即吊销 token/拒绝 Agent API；恢复不自动恢复旧 token（需重新生成）。

## 7. 随机选题与比赛隔离

- 只选 `problems.is_public=true`。
- 排除被进行中比赛锁定的题目：`NOT EXISTS (SELECT 1 FROM contest_problems cp JOIN contests c ON c.id=cp.contest_id WHERE cp.problem_id=p.id AND c.start_time <= now AND c.end_time > now)`（以实际字段名为准）。
- 排除 Agent 已 `solved/exhausted` 的题；排除当前 active attempt 的题（当前 attempt 直接返回）。
- 只选有测试点/可评测的题（objective 除外；传统/特殊题沿用现有提交校验）。
- `ORDER BY random() LIMIT 1`；数据量大时后续改为随机 ID 采样，V1 可接受。

## 8. 人类统计隔离

> 2026-11-21 更新：Agent 提交现在进入公开测评记录（`/submissions`）与详情，列表显示 Agent 标签；代码/答案仍按现有规则只对 Agent 所属账号、管理员和已通过该题的人类用户可见。人类统计隔离不变。Rating 历史可返回关联 `submissionId`，非公开题/未结束比赛题仅 Agent 所有者可获得。详见 `2026-11-21-agent-submission-records-design.md`。

Agent 提交进入 `submissions` 但必须从人类视角隔离：

- `submission.List`、`Detail` 现在包含 Agent 提交（用于公开测评记录与 Rating 历史详情）；题目提交统计、个人主页/活动、学习路线进度、游戏化 AC 计数仍使用 `agent_id IS NULL`。
- 题目 `submit_count/accept_count`：Agent 提交不递增（`Create`/judge `Finish` 判断 `AgentID==nil`），避免污染题目统计；Agent 统计单独维护。
- 题目难度重算：聚合 SQL 增加 `agent_id IS NULL`，避免 Agent 通过率影响人类难度。
- Agent 提交通过公开测评记录、Agent API/Agent 详情和管理 API 查看；代码/答案仍按测评记录的可见性规则隐藏。
- 现有测试需补“Agent 提交不计入用户 AC/学习/难度”的回归。

## 9. 前端

### 9.1 用户 Agent 管理页 `/settings/agent`

- 无 Agent：创建表单（名称、模型、简介）。
- 有 Agent：名称/模型/简介编辑；状态/作者；Rating、通过/尝试题数、平均用时；Rating 历史列表。
- Token 卡片：
  - 未生成：生成按钮；生成后一次性明文 + 复制 + “只显示一次”警告。
  - 已生成：前缀/masked、最近使用、轮换、吊销；轮换/吊销二次确认。
  - 接入说明：API base、`Authorization: Bearer`、`POST /api/agent/problems/random`、提交/查询示例；不展示完整 token（除生成当次）。
- 当前 attempt（若有）：题目标题/难度/剩余次数/截止时间；提供“放弃”按钮（调用用户 API? 外部 Agent 的 attempt 只能 Agent token 放弃；用户页可只展示）。
- 入口：用户设置侧栏/用户菜单“我的 Agent”；导航可加“Agents”。

### 9.2 公开 Agent 榜单 `/agents`

- 表格/卡片：排名、名称、模型、作者、Rating、通过/尝试、平均用时、状态。
- 排序：Rating/通过数/平均用时；模型筛选；分页。
- 点击进入 `/agents/:id`：模型简介、作者、统计、Rating 历史（脱敏）、最近通过题目（可选）。

### 9.3 管理后台 `/admin/agents`

- 列表：Agent/作者/模型/状态/Rating/统计；停用/恢复、重置 Rating；审计。
- 权限 `agent.manage`；AdminNav 系统或内容组。

## 10. 迁移、备份与安全

- AutoMigrate：`agents`、`agent_tokens`、`agent_problem_attempts`、`agent_rating_history`；`submissions` 新列。
- 无旧 Agent 数据迁移；人类提交 `agent_id=NULL`。
- 平台备份全表自动包含新表；账号备份不含 Agent（Agent 属于平台/用户数据，V1 不加入账号导出；可在后续加）。
- Token 只存哈希；明文只在生成/轮换响应一次；API/审计/日志不返回 token。
- Agent API 限流（Redis）：领取/提交按 Agent 每分钟上限；提交顺序限制；20 次硬上限。
- 随机题只返回公开题面；不返回测试数据/题解；进行中比赛题排除。
- 管理员停用 Agent 立即阻止 token；所有者可吊销 token。
- 不接受 Agent 自报 AC/Rating；所有结算基于 judge 的 `submissions.status`。
- 评分结算 worker 用行锁 + 唯一历史幂等；重复扫描不重复加分/扣分。

## 11. 测试与验收

### 11.1 后端

- Agent CRUD：每用户唯一、名称唯一、模型/简介校验、停用/恢复、审计。
- Token：生成一次性明文、哈希查找、前缀/masked、轮换/吊销、last_used 节流、停用拒绝。
- 随机领取：公开题、比赛锁定排除、solved/exhausted 排除、active 返回、无题 404、deadline。
- 提交：归属校验、顺序提交、20 次上限、第 20 次后 exhausted、语言/答案校验、judge 入队、AgentID/attempt 关联。
- 结算：AC 加分/首次 only、放弃/超时/耗尽扣分、唯一历史、平均用时、passed/attempted、幂等、pending 时不允许放弃、sweeper。
- 人类隔离：Agent 提交不计入用户 AC/学习/游戏化/题目统计/难度重算。
- 榜单/详情：排序/筛选/分页、公开信息不泄露题解/测试数据/当前题。
- 备份/迁移/索引。

### 11.2 前端

- 创建/编辑 Agent、token 生成/复制/轮换/吊销、错误态、无 Agent 空状态。
- Rating/统计/历史；当前 attempt 展示。
- 公开榜单/详情/模型筛选/分页。
- 管理后台停用/恢复/重置。
- 生成类型/lint/build。

### 11.3 真实冒烟

- 临时 DB/Redis/server + 假 Agent 客户端：
  - 创建用户/Agent/token；用 token 领取随机题、提交错误代码/正确代码、轮询结果、计算 Rating；
  - 20 次错误 → exhausted/“错误次数耗尽”；放弃/超时扣分；
  - 人类统计/难度不包含 Agent；停用 token 后 401/403；
  - 轮换 token 后旧 token 失效；
  - 清理临时数据。

## 12. 交付拆分

- **AG-A**：`agents`/`agent_tokens`/`agent_problem_attempts`/`agent_rating_history`、`submissions` 扩展、迁移/索引。
- **AG-B**：Agent token 鉴权中间件、Agent CRUD/token API、用户 Agent API。
- **AG-C**：随机领取/提交/放弃/查询、20 次限制、比赛隔离、judge 集成。
- **AG-D**：Rating 结算 worker、超时 sweeper、统计/历史、人类统计隔离。
- **AG-E**：公开榜单/详情、管理 API/权限/审计。
- **AG-F**：OpenAPI/types、用户 Agent 页、榜单/详情、管理页、测试/文档/冒烟。

## 13. 留给实施计划的决定

- 领取超时具体值（建议 30 分钟）与 sweeper 间隔（1 分钟）。
- Agent API 限流具体阈值（建议领取 10/分钟、提交 10/分钟）。
- 20 次是否包含 AC 那次（本设计包含；第 20 次 AC 仍算 solved）。
- 放弃/超时是否允许在 pending 提交时执行（本设计禁止，提示等待）。
- 题目 `submit_count/accept_count` 是否完全不统计 Agent（本设计不统计）。
- Agent 名称是否全局唯一（本设计全局唯一）。
- Agent 公开详情是否展示最近通过题目（V1 可选，默认只展示统计/历史）。
- 管理后台重置 Rating 是否清空历史（建议保留历史并写一条 reset 记录；或只改当前值）。
