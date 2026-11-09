# 客观题（答案题）设计与实施计划

> 目标：在现有传统题/特判题/交互题之外新增 `objective` 题型。用户直接提交文本答案，judge 跳过代码编译与沙盒，按“去首尾空白 + 忽略大小写 + 多可接受答案”判定；正确答案只对题目管理员可见。

## 1. 背景与现状

- `problem.Type` 只有 `traditional`、`special`、`interactive`；题目编辑页的类型下拉没有答案题。
- 提交输入固定要求 `language` + `code`；judge 固定要求测试点并执行 Docker 编译/运行；没有直接答案比对链路。
- 题目发布要求至少 1 个测试点；客观题没有测试点，需要单独放行。
- 题目详情对“已通过用户”会放开题解字段；正确答案不能沿用这条规则，否则 AC 后会泄露答案。
- 比赛、锁定、AI 禁用、提交统计、SSE、游戏化 AC 事件都复用现有提交链路，客观题不能绕开这些边界。

## 2. 目标与非目标

### 目标

- 新建/编辑题目时可选 `objective`（客观题/答案题）。
- 题目模型保存 1–20 个可接受答案；答案不进入公开题目详情，只有题目管理员（`CanViewSolution`/super）可见。
- 提交页对客观题显示答案输入框，不再显示语言选择、代码编辑器、AI 工具栏。
- 提交接口接受 `answer`，不需要 `language`/`code`；后端按题目类型校验。
- judge 对客观题不编译、不运行 Docker、不依赖测试点；直接比对答案，写入 `accepted`/`wrong_answer`，AC 计 100 分。
- 客观题可发布、可加入比赛、可参与排行榜、学习路线、成就/任务，沿用现有 AC 事件与统计。
- 代码题行为不变；用户提交的客观题答案只对本人和具备赛时代码查看权限的管理员可见，其他用户任何时候都不可见（避免 AC 提交泄露正确答案）。
- 前端题目详情、比赛题目列表、提交记录展示“客观题”标识，并隐藏无意义的时间/空间限制。

### 非目标

- 不做选择题选项 UI（单选/多选按钮）、选项 A/B/C/D 的结构化配置；后台仍用文本答案。
- 不做答案富文本、图片答案、公式答案或附件上传。
- 不做答案相似度、正则、数值容差或人工评分。
- 不自动向 AC 用户展示正确答案；题解/提示仍按现有资源区规则展示。
- 不改变代码题的评测、测试点、发布和比赛逻辑。
- 不新增权限；沿用 `problem.create/update/publish` 等现有题目权限。

### 反目标

- 任何非题目管理员通过题目详情、提交详情、比赛页面或列表拿到正确答案。
- 客观题因为没有测试点而无法发布，或误触发“题目还没有测试点”。
- 客观题提交仍进入沙盒，或代码题提交被答案字段绕过。
- 客观题答案在比赛进行中被其他选手看到。

## 3. 约束

- 后端 Go + Gin + GORM + PostgreSQL；judge 是独立 Go 模块 + pgx。
- 题目/提交模型通过 `db.Migrate` AutoMigrate 加列；历史数据默认空答案/空提交答案。
- 答案匹配规则固定为：`strings.EqualFold(strings.TrimSpace(userAnswer), strings.TrimSpace(acceptedAnswer))`；多个可接受答案任意一个匹配即 AC。
- 每个答案 1–2000 个字符；每题 1–20 个可接受答案；后台按行拆分、去重、去首尾空白。
- 客观题不要求测试点；代码题仍要求至少 1 个测试点（特判题仍要求 checker）。
- 正确答案只通过题目管理接口返回；`problemapi.get` 的 `includeSolution` 不能用于答案，必须单独的 `includeAnswer`。
- `includeAnswer` 只授予 `problem.update` 或 super（真正的题目编辑者），不沿用包含 testcase/checker 管理的 `CanViewSolution`。
- 提交详情中的 `answer` 只对提交者本人或 `CanViewLiveContest` 管理员可见；这与 `code` 的比赛保护规则独立，避免非比赛题 AC 后通过测评记录泄露答案。
- judge 对客观题不调用 sandbox；`language` 使用保留值 `answer`，`code` 为空，`answer` 存用户答案。
- 客观题 AC 分数固定 100；时间为 0、内存为 0、judge cases 为空。
- 现有提交保护逻辑（比赛进行中隐藏他人代码/详情）同样隐藏答案。
- 中文文案、错误码、审计和现有 UI 风格保持一致。

## 4. 方案选择

### 4.1 判定位置

- **方案 A：judge 队列内判定（选用）**。提交仍创建 `pending` 并入队；judge `Claim` 后按题型分支，客观题直接比对并 `Finish`。
- 方案 B：提交服务同步比对并直接写终态。少一次队列，但重复统计/事件/SSE 逻辑，且绕开 judge 的重试与幂等。
- 方案 C：前端比对。答案会下发到浏览器，直接泄露，排除。

选择 A。复用现有 submit → queue → judge → finish → SSE/game event 链路。

### 4.2 答案存储

- **方案 A：`problems.answers_json` JSONB 数组（选用）**。支持多个可接受答案，迁移简单，读写与现有 `dbtypes.List[string]` 一致。
- 方案 B：单列 `answer` 文本。只能一个答案，无法支持“多答案”。
- 方案 C：单独 `problem_answers` 表。适合更多元数据，但当前不需要。

选择 A。

### 4.3 提交协议

- **方案 A：`answer` 字段 + 类型感知校验（选用）**。代码题仍要求 `language`+`code`；客观题要求 `answer`，语言固定为 `answer`。
- 方案 B：客观题伪装成 `language=answer` + `code=答案`。字段语义混乱，代码展示/保护逻辑容易误用。
- 方案 C：新增独立 `/submissions/answer` 接口。前端/比赛/权限都要分叉，收益不大。

选择 A，沿用 `POST /api/submissions`。

### 4.4 正确答案可见性

- **方案 A：仅题目管理员可见（选用）**。管理编辑页通过 `includeAnswer` 获取；普通用户/AC 用户/参赛者都拿不到。
- 方案 B：AC 后可见。会泄露答案，且比赛中可能被分享。
- 方案 C：赛后可见。需要额外比赛上下文和“正确答案”展示位，超出本次需求。

选择 A。

## 5. 设计

### 5.1 数据模型

`problems` 新增：

```text
answers_json jsonb not null default '[]'
```

对应 Go：

```go
const TypeObjective Type = "objective"

type Problem struct {
    ...
    Type    Type                 `gorm:"size:20;not null;default:traditional"`
    Answers dbtypes.List[string] `gorm:"column:answers_json;type:jsonb;not null;default:'[]'"`
    ...
}
```

`submissions` 新增：

```text
answer text not null default ''
```

对应 Go：

```go
type Submission struct {
    ...
    Language Language `gorm:"size:20;not null"`
    Code     string   `gorm:"type:text;not null"`
    Answer   string   `gorm:"type:text;not null;default:''"`
    ...
}
```

`Language` 增加保留值：

```go
const LanguageAnswer Language = "answer"
```

`Valid()` 包含 `LanguageAnswer`；代码提交表单的语言列表仍只列 c/cpp/python3/java/go。

### 5.2 题目输入与校验

`problem.Input` 增加：

```go
Answers []string `json:"answers"`
```

`Normalize`：

- `Type.Valid()` 包含 `objective`，错误文案改为 “traditional、special、interactive 或 objective”。
- 客观题：
  - 1–20 个答案；
  - 每条 `TrimSpace` 后 1–2000 字符；
  - 拒绝控制字符（允许普通空格）；
  - 按忽略大小写去重；重复答案直接拒绝，提示“可接受答案不能重复”。
- 非客观题：`Answers` 强制清空为 `[]`，避免历史/误传答案残留。
- 时间/空间限制仍按现有规则校验；客观题使用默认值，前端隐藏这些字段。
- 样例、题面、提示、来源等规则不变。

`apply` 写入 `p.Answers`；`editableColumns` 增加 `answers_json`。

发布校验（repository.Update）：

- `p.Type == TypeObjective`：不要求测试点；
- 其他类型：保持“公开题至少 1 个测试点”；
- `TypeSpecial`：仍要求 checker；
- 客观题删除最后一个测试点不应报“公开题目至少保留 1 个测试点”（如存在历史测试点），可在 `DeleteTestcase` 守卫中按题型放行。

### 5.3 题目 API 与答案可见性

`ProblemDetail` 新增可选字段：

```go
Answers *[]string `json:"answers,omitempty"`
```

- `ToDetail(p, includeSolution, includeAnswer, author)`：只有 `includeAnswer == true` 时填 `Answers`。
- `problemapi.get`：
  - `includeSolution` 沿用现有“题解管理权限或本人已 AC”；
  - `includeAnswer` 只等于 `problem.update` 或 super（题目编辑者）；不沿用 `CanViewSolution`，避免 testcase/checker 管理角色看到答案；
  - 不因 `accepted.HasAccepted` 放开答案。
- 题目编辑页有 `problem.update`，因此能拿到答案；公开详情、比赛详情、AC 用户都拿不到。
- `ProblemEditView` 只在 `objective` 时显示“正确答案”编辑区；切回其他类型时清空 `answers` 并提示不会保存。
- 审计中间件只记录 method/path/status，不记录请求体，答案不会进入审计。

### 5.4 提交输入与校验

`submission.Input` 增加：

```go
Answer string `json:"answer"`
```

校验拆分为：

```go
func (in Input) Normalize() (Input, error)         // problemId/contestId 等公共字段
func (in Input) ValidateForProblem(t problem.Type) (Input, error)
```

- 公共：`problemId` 正整数、`contestId` 非负。
- 客观题：
  - 要求 `strings.TrimSpace(Answer)` 非空；
  - 答案长度 1–2000 字符；
  - 忽略客户端传来的 `language`/`code`，统一写 `LanguageAnswer`、`Code=""`、`Answer=trimmed`。
- 代码题：
  - 保持语言枚举与代码非空、≤64 KiB；
  - 忽略 `answer`，统一写 `Answer=""`。
- `Service.Create` 先做公共 Normalize、查题目，再 `ValidateForProblem(p.Type)`，然后走原有比赛/锁定/可见性检查。
- 客观题跳过交互题、checker、测试点数量检查；代码题逻辑不变。
- 创建提交时写入 `Answer`；其余入队、限流、事件、SSE 不变。
- 比赛提交同样支持客观题：比赛报名、题目归属、锁定、提交冷却全部复用；答案在赛时保护下隐藏。

### 5.5 judge 判定

`judge/internal/model`：

```go
type Submission struct {
    ...
    Answer string
}

type Problem struct {
    ...
    Answers []string
}
```

`judge/internal/db.Claim`：

- `submissions` 查询增加 `answer`；
- `problems` 查询增加 `answers_json`，扫描为 `[]byte` 后 `json.Unmarshal` 到 `[]string`；
- 测试点查询保持原样（客观题通常为空）。

`pipeline.Run`：

```text
Claim
publish(judging)
if prob.Type == "objective":
    status = accepted if any(EqualFold(TrimSpace(sub.Answer), TrimSpace(answer)))
    finish(status, score=100 if accepted else 0, time=0, memory=0, cases=[])
    return
// 以下为现有代码题路径
if len(cases)==0 -> error
if !sub.Language.Valid() -> error
compile/sandbox/...
```

- 客观题不读取 `sub.Code`，不要求 `Language` 在代码语言集合中。
- AC 时 `Finish` 仍递增 `problems.accept_count`、发布 game accepted 事件；成就/任务/学习路线正常触发。
- `Finish` 的 cases 为空数组，前端展示为空。
- 代码题逻辑与错误顺序不变。

### 5.6 前端题目编辑

`ProblemEditView.vue`：

- `TYPE_OPTIONS` 增加 `{ label: '客观题（直接提交答案）', value: 'objective' }`。
- `FormState` 增加 `answers: string[]`（或独立的 `answersText` ref）。
- 当 `form.type === 'objective'`：
  - 显示“正确答案”Textarea，提示“每行一个可接受答案；判定时忽略首尾空白和大小写”；
  - 隐藏时间限制/内存限制输入（仍以默认值保存）；
  - 隐藏 `TestcaseManager`、checker 上传和 AI 造数据入口；
  - 保存时把文本按行分割、trim、过滤空行后写入 `answers`。
- 其他类型：不显示答案区，保存 `answers: []`。
- `ProblemDetail` 类型增加 `answers?: string[]`；编辑页填充已有答案。

### 5.7 前端题目详情与比赛列表

`ProblemDetailView.vue`：

- `typeLabels` 增加 `objective: '客观题'`。
- 客观题 meta bar 显示“直接作答”标签，不显示 ms/MB；保留难度、标签、来源、作者。
- 主按钮文案改为“提交答案”。
- 题面/提示/资源区不变；正确答案不在页面任何位置展示。

`ContestDetailView.vue`：

- `ContestProblemItem` 增加 `type`（后端 `ProblemRow` 选 `p.type`）。
- 客观题题目行的“限制”列显示 `Tag value="客观题"`，不显示 ms/MB；其他类型保持现有时间/空间标签。
- 赛后统计/跳转逻辑不变。

### 5.8 前端提交页

`SubmitView.vue`：

- `isObjective = computed(() => problem.value?.type === 'objective')`。
- 客观题：
  - 隐藏语言 Select、Monaco 编辑器、AI 工具栏/横幅、代码草稿；
  - 显示“你的答案”Textarea（maxlength 2000，显示字数）；
  - 提交按钮文案“提交答案”；
  - 调用 `createSubmission({ problemId, contestId?, answer })`，不传 language/code；
  - 提交成功仍跳转测评记录；
  - 不加载/显示 AI 配置。
- 代码题路径保持现有行为。
- 比赛客观题仍显示“比赛提交”与返回比赛按钮。

### 5.9 前端测评记录

`SubmissionsView.vue`：

- `languageLabel('answer')` → “客观题”；语言筛选增加“客观题”选项（代码提交页的语言列表不变）。
- 列表耗时/内存：客观题显示 `—`，不显示 0 ms/0 KB。
- 详情抽屉：
  - 语言显示“客观题”；
  - 客观题显示“答案”区块，展示用户提交的答案；
  - `answerHidden` 时显示“答案仅提交者本人可见。”；
  - 不显示代码块；比赛进行中他人仍按现有 `codeHidden` 显示代码保护提示。
- 代码题行为不变。

### 5.10 OpenAPI 与类型

`packages/api/openapi.yaml`：

- `ProblemType` enum 增加 `objective`。
- `ProblemInput.answers`：array of string，maxItems 20，items maxLength 2000。
- `ProblemDetail.answers`：可选 array of string，描述“仅题目管理员可见”。
- `SubmissionLanguage` enum 增加 `answer`。
- `SubmissionInput`：`language`/`code` 改为可选，新增 `answer`（maxLength 2000）；描述说明代码题与客观题的互斥要求。
- `SubmissionDetail.answer`：可选 string，仅提交者本人或 `CanViewLiveContest` 管理员可见。
- `SubmissionDetail.answerHidden`：boolean，说明答案是否因非本人/权限不足而隐藏。
- `ContestProblemItem.type`：新增 `ProblemType`。
- 生成 `schema.d.ts` 与 `types.ts` 别名。

### 5.11 权限与审计

- 不新增权限；题目创建/编辑/发布仍用现有 `problem.create/update/publish`。
- 答案字段只在管理员可见，不进入公开 API。
- 提交详情中的答案使用独立可见性：仅提交者本人或 `CanViewLiveContest` 管理员可读；不沿用 `contestProtected` 的“赛后可公开”规则。
- 审计不记录答案。

## 6. 测试与验收

### 6.1 后端题目

- 类型枚举接受 `objective`；空类型仍默认 traditional；非法类型报错包含 objective。
- 客观题答案：1–20 条、trim、长度、控制字符、忽略大小写重复拒绝；非客观题清空 answers。
- 公开题客观题无测试点可发布；传统题无测试点仍报错；特判题仍要求 checker。
- 公开详情/游客/AC 用户不返回 `answers`；题目管理员返回；`includeSolution` 不放开答案。
- 迁移：`problems.answers_json`、`submissions.answer` 列存在。

### 6.2 后端提交

- 客观题只传 `answer` 成功创建，语言为 `answer`、代码为空、答案 trim 后入库。
- 客观题空答案/超长/代码题不传语言或代码 → 40001。
- 代码题传 answer 不改变行为；代码题仍要求语言/代码。
- 客观题无测试点可提交；交互题仍拒绝；特判题 checker 检查不变。
- 比赛客观题提交复用报名/锁定/冷却校验。
- 提交详情：本人始终可见答案；其他用户任何时候都不可见（`answerHidden=true`），无论是否比赛/赛后；`CanViewLiveContest` 管理员可查看。

### 6.3 judge

- 客观题 AC：大小写/首尾空白不同仍通过；多个可接受答案任意一个命中。
- 客观题 WA：不匹配返回 `wrong_answer`，score 0。
- 客观题无测试点仍能完成，不编译、不调用 sandbox。
- AC 后 `accept_count` +1、game accepted 事件发布。
- 传统题路径回归：无测试点仍报错、编译/运行/特判不变。
- `pipeline_test` 增加 objective fixture；testutil 增加 `answers_json` 与 objective 插入 helper。

### 6.4 前端

- `ProblemEditView`：类型可选 objective；答案区显隐、保存 answers、隐藏测试点/AI 造数据、切回传统题清空。
- `ProblemDetailView`：客观题标签、隐藏 ms/MB、按钮“提交答案”、不显示答案。
- `ContestDetailView`：客观题限制列显示“客观题”。
- `SubmitView`：客观题显示答案输入、隐藏代码/语言/AI；提交 payload 只有 answer；代码题回归。
- `SubmissionsView`：客观题语言/答案展示、耗时内存 `—`、比赛隐藏提示。
- 前端全量测试、lint、build。

### 6.5 真实冒烟

- 临时 server + 测试库 + storage：创建客观题（2 个可接受答案），不添加测试点直接发布。
- 提交 ` A `、`a`、错误答案，分别 AC/AC/WA；AC 分数 100、耗时/内存 0。
- 题目详情游客/普通用户/AC 用户无 answers；管理员编辑页有 answers。
- 加入比赛：比赛提交、锁定、榜单、赛后统计正常；他人在赛时看不到答案。
- 前端浏览器检查客观题创建、详情、提交、测评记录、比赛列表（无桌面浏览器时记录为延后）。

## 7. 已知取舍

- 答案匹配只做 trim + 忽略大小写；不做内部空白折叠、全半角归一化、数值容差或正则。
- 客观题 AC 固定 100 分；不按 `contest_problems.score` 自定义分数计算（该字段目前也不参与现有榜单计分，保持一致）。
- 客观题仍保留 `time_limit_ms`/`memory_limit_mb` 列（默认值），但前端隐藏且 judge 不使用；避免迁移和旧接口破坏。
- 选择题只作为“答案文本”支持，不提供选项按钮/选项配置 UI。
- 正确答案不自动展示给用户；管理员可在题目编辑页查看。
- 客观题可存在历史测试点；发布不要求测试点，judge 忽略测试点。
- 比赛榜单对客观题仍按提交时间计算罚时；AC 分数 100，IOI/OI 总分为 100。
