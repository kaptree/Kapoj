# PrimeVue 迁移与科技风美化进度

> 本文档是美化工程的唯一进度来源。每次会话开始先读这里，结束时更新状态、验证结果和设计决策。
>
> - 最后更新：2026-11-08
> - 当前阶段：P2 页面（UI-19 待开始）；M4-C 新功能 UI-30、M4-7 学习路线 UI-31、M4-3 社交互动 UI-32、RBAC-D 管理后台 UI-36、M4-4 Rating 设置 UI-37、M5-6 题目导入 UI-35、M5-5 AI 面板 UI-34 与 AI 管理开关/加密密钥 UI-38 已完成
> - 工作分支：`feat/m1-problems-judge`（RBAC-D 管理后台在 `feat/rbac-d-admin-ui` 完成，M4-3 社交互动在 `feat/m4-3-social` 完成）
> - 本次基线：`0f2d62d feat(web): rating history card`

## 状态汇总

| 状态 | 数量 |
| --- | ---: |
| done | 27 |
| in-progress | 0 |
| blocked | 0 |
| todo | 3 |

其中现有 UI 迁移任务 `UI-00～UI-18` 已完成，M4-C 新功能 `UI-30`（排行榜、签到与 Rating 历史卡片）、M4-7 新功能 `UI-31`（学习路线）、M4-3 新功能 `UI-32`（社交互动：题解点赞、关注/粉丝 Dialog、通知铃铛与通知页）、RBAC-D 管理后台 `UI-36`（管理壳/仪表盘/用户/角色/审计页面）、M5-6 题目导入 `UI-35`（导入页）、M5-5 AI 面板 `UI-34`（提交页工具栏/详情 AI 提示/配置页）与 AI 管理开关/加密密钥 `UI-38`（`/admin/ai` 设置页）已直接使用 PrimeVue 4 完成；剩余 `UI-19` 题库编辑与 `UI-20` 异常页；`UI-33` 为尚未实现的新功能，不计入现有页面迁移完成度。

M4-C 排行榜、签到与 Rating 历史（UI-30）、M4-7 学习路线（UI-31）、M4-3 社交互动（UI-32）、RBAC-D 管理后台（UI-36）、M4-4 Rating 设置页（UI-37）、M5-6 题目导入页（UI-35）、M5-5 AI 面板（UI-34）与 AI 管理开关/加密密钥页（UI-38）已直接使用 PrimeVue 4 完成；现有页面迁移剩余 UI-19 → UI-20。若后续某页面进入功能开发，先标记 `blocked` 并跳过。

## 进度表

| ID | 模块 | 页面/文件 | 优先级 | 状态 | 依赖 | 验收 | 备注 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| UI-00 | 基础设施 | PrimeVue preset / CSS tokens / `App.vue` Toast+ConfirmDialog / `components/ui/*` | P0 | done | - | build/test/lint；暗/亮 token；全局浮层可用 | 2026-11-05 完成。PrimeVue 4.5.5 + `@primeuix/themes` 1.2.5 + `primeicons` 7.0.0 |
| UI-01 | 布局 | `layouts/DefaultLayout.vue`、`components/UserMenu.vue`、`components/layout/AppNav.vue`、AppShell、主题切换入口 | P0 | done | UI-00 | 桌面/平板/手机可用；键盘焦点与抽屉可操作；暗/亮正常 | 2026-11-05 完成。桌面侧栏 + 移动 Drawer + 主题切换 + 用户菜单 |
| UI-02 | 认证 | `LoginView`、`RegisterView`、`ProfileEditView`、`validation.ts`、`components/auth/AuthLayout.vue`、`components/ui/AppFormField.vue` | P0 | done | UI-00、UI-01 | 表单校验、错误提示、成功 Toast 与原行为一致 | 2026-11-05 完成。PrimeVue 输入组件 + 框架无关校验 + 共享表单字段 |
| UI-03 | 共享 | `IdentityShowcase.vue` + spec | P1 | done | UI-00 | 无 Element Plus；称号/徽章尺寸与颜色不变 | 2026-11-05 完成。PrimeVue Avatar + 本地 Tooltip 指令；图片失败回退首字母 |
| UI-04 | 核心 | `HomeView` | P1 | done | UI-01 | 信息层级、快捷入口、响应式 | 2026-11-05 完成。Hero + 系统状态 + 快速开始/成长中心卡片；游客/登录/加载态 |
| UI-05 | 核心 | `ProblemListView` | P1 | done | UI-00 | 筛选/分页/query 同步不变；空态/加载/错误齐全 | 2026-11-05 完成。PrimeVue DataTable/Select/Paginator/ToggleSwitch；保留管理端筛选 |
| UI-06 | 核心 | `ProblemDetailView` + spec | P1 | done | UI-00、UI-03 | Markdown/KaTeX/代码高亮保留；题目权限不变 | 2026-11-05 完成。PrimeVue Tabs/ProgressBar/Message；MarkdownView 支持明暗主题 |
| UI-07 | 核心 | `SubmitView` | P1 | done | UI-00 | Monaco、草稿 localStorage、比赛参数、SSE 跳转不变 | 2026-11-05 完成。PrimeVue Select/Button/Toast；草稿键稳定化；Monaco 未替换 |
| UI-08 | 核心 | `SubmissionsView` | P1 | done | UI-00、UI-03 | 筛选/分页/SSE/详情抽屉/赛时保护不变 | 2026-11-05 完成。PrimeVue DataTable/InputNumber/Paginator/Drawer；SSE 与详情竞态保留 |
| UI-09 | 核心 | `ContestListView` + spec | P1 | done | UI-00 | 筛选/分页/管理员操作/确认框不变 | 2026-11-05 完成。PrimeVue DataTable/Select/Checkbox/Paginator/Dialog；中文 ConfirmDialog 与克隆输入框 |
| UI-10 | 核心 | `ContestDetailView`、`ContestRanklist` + spec | P1 | done | UI-00、UI-03 | 倒计时、报名、公告、榜单 SSE、赛时保护不变 | 2026-11-05 完成。PrimeVue Tabs/DataTable/Dialog；SSE 与刷新链保留 |
| UI-11 | 核心 | `ContestEditView` + spec | P1 | done | UI-00 | MarkdownEditor、题目整体替换、密码留空语义不变 | 2026-11-05 完成。PrimeVue DatePicker/Select/DataTable；开始后冻结与远程选题保留 |
| UI-12 | 用户 | `UserProfileView` + spec | P2 | done | UI-03 | 资料、Rating、金币、身份展示 | 2026-11-05 完成。PrimeVue Avatar + 共享页头/卡片/状态；头像失败回退首字母 |
| UI-13 | 游戏化 | `AchievementsView` + spec | P2 | done | UI-00 | 筛选、进度、奖励展示 | 2026-11-05 完成。SelectButton/ProgressBar/卡片网格；图标回退与登录态重载保留 |
| UI-14 | 游戏化 | `TasksView` + spec | P2 | done | UI-00 | 领取确认、状态、进度 | 2026-11-05 完成。PrimeVue ProgressBar/Tag/ConfirmDialog；领取与刷新链保留 |
| UI-15 | 游戏化 | `BadgeShopView` + spec | P2 | done | UI-00 | 分类筛选、购买确认、库存/余额 | 2026-11-05 完成。PrimeVue SelectButton/Avatar/Tag；购买/装备链与图片回退保留 |
| UI-16 | 游戏化 | `TitleShopView` + spec | P2 | done | UI-00 | 解锁进度、购买/装备 | 2026-11-05 完成。PrimeVue ProgressBar/Tag/ConfirmDialog；称号颜色与装备替换保留 |
| UI-17 | 游戏化 | `AppearanceView` + spec | P2 | done | UI-00 | 装备/卸下、金币流水、分页 | 2026-11-05 完成。DataTable/Paginator/IdentityShowcase；分页与金额样式保留 |
| UI-18 | 管理 | `GamificationAdminView` + 4 个 admin panel + spec | P2 | done | UI-00 | 四个 Tab、CRUD、图片上传、题目远程搜索 | 2026-11-05 完成。PrimeVue Tabs/DataTable/Dialog/MultiSelect/ColorPicker；上传与冻结语义保留 |
| UI-19 | 题库管理 | `ProblemEditView` + `TestcaseManager` + `SampleEditor` + spec | P2 | todo | UI-00 | Markdown、测试点上传/排序、样例编辑 | 建议下一项 |
| UI-20 | 异常页 | `ForbiddenView`、`NotFoundView` | P2 | todo | UI-00 | 空态/结果页视觉统一 | 最简单，适合收尾 |
| UI-30 | 新功能 | M4-C 排行榜前端（含签到与 Rating 历史卡片） | P3 | done | UI-00、UI-10 | 直接使用 PrimeVue；Rating 榜/历史 | 2026-11-05 完成。`LeaderboardView` 三 Tab/签到、`RatingLevelName`、`RatingHistoryCard`；复用 Tabs/DataTable/Paginator/Toast |
| UI-36 | 新功能 | RBAC-D 管理后台壳：`AdminLayout`/`AdminNav` + 仪表盘/用户/角色/审计页面 | P3 | done | UI-00、UI-18 | 直接使用 PrimeVue；权限驱动菜单/按钮 | 2026-11-07 完成。`AdminLayout`/`AdminNav`（桌面侧栏 + 移动 Drawer）、`AdminDashboardView`/`AdminUsersView`/`AdminRolesView`/`AdminAuditView`；复用 AppPageHeader/AppSurface/DataTable/Dialog/MultiSelect/Toast；`auth.can/canAny` 驱动菜单与动作 |
| UI-31 | 新功能 | 学习路线 | P3 | done | UI-00 | 直接使用 PrimeVue；树形/解锁进度 | 2026-11-07 完成。`LearningPathListView`/`LearningPathDetailView`/`LearningPathEditView`/`AdminLearningPathsView` + `LearningNodeDialog`；复用 TreeTable/DataTable/Dialog/Select/InputNumber/ProgressBar/Tag |
| UI-32 | 新功能 | 社交互动 | P3 | done | UI-00 | 直接使用 PrimeVue；点赞/关注/通知 | 2026-11-08 完成。题解列表/详情点赞按钮、`UserProfileView` 关注按钮 + `FollowListDialog`（粉丝/关注 Tab）、`NotificationBell`（未读 badge/60s 轮询）、`NotificationsView`；复用 AppPageHeader/AppSurface/DataTable/Dialog/TabView/Paginator/Toast/Button；不新增 UI 依赖 |
| UI-33 | 新功能 | 证书认证 | P3 | todo | UI-00 | 直接使用 PrimeVue；提交/审核/核验 | 尚未实现 |
| UI-34 | 新功能 | AI 辅助面板 | P3 | done | UI-00、UI-06 | 直接使用 PrimeVue；API key 配置/提示 | 2026-11-08 完成。`SubmitView` AI 工具栏（补全/格式化/优化 + 预览 Dialog，区分「AI 功能未启用」/「管理员尚未配置 AI 密钥」/「尚未配置 AI 模型」）、`ProblemDetailView` AI 提示、`AiSettingsView`（`/settings/ai`，区分 404 与 `keyConfigured=false`）与 `UserMenu`「AI 设置」入口；复用 AppPageHeader/AppSurface/AppFormField/Select/InputText/Password/Message/Button/Toast/Dialog；不新增 UI 依赖 |
| UI-35 | 新功能 | 题目导入 | P3 | done | UI-00 | 直接使用 PrimeVue；HydroOJ 导入 | 2026-11-08 完成。`ProblemImportView`（`/problems/import`）复用 AppPageHeader/AppSurface/DataTable/Message/Button；题目列表「导入题包」入口与详情创建者链接；不新增 UI 依赖 |
| UI-37 | 新功能 | M4-4 Rating 设置页：`AdminRatingView` | P3 | done | UI-00、UI-36 | 直接使用 PrimeVue；衰减/赛季设置与只读当前赛季 | 2026-11-08 完成。`AdminRatingView`（`/admin/rating`）复用 AppPageHeader/AppSurface/AppFormField/ToggleSwitch/InputNumber/InputText/Toast；`AdminNav` 增加「Rating」入口；不新增 UI 依赖 |
| UI-38 | 新功能 | AI 管理开关 + 加密密钥：`AdminAiView` | P3 | done | UI-00、UI-36 | 直接使用 PrimeVue；状态 Tag/ToggleSwitch/密钥缺失禁用 | 2026-11-08 完成。`AdminAiView`（`/admin/ai`）复用 AppPageHeader/AppSurface/AppFormField/ToggleSwitch/Message/Tag/Toast，并新增「加密密钥」板块（状态 Tag 未配置/文件/env、自动生成一次性显示 + 复制、手动填写、轮换二次确认、env 只读）；`AdminNav` 增加「AI 设置」入口（`ai.settings.manage`）；不新增 UI 依赖 |

## Legacy allowlist

以下文件仍在使用 Element Plus。迁移完成一个任务后，从本列表移除对应文件；全部完成后本列表必须为空。

| 归属任务 | 文件 |
| --- | --- |
| UI-19 | `src/views/ProblemEditView.vue`、`src/components/TestcaseManager.vue`、`src/components/TestcaseManager.spec.ts`、`src/components/SampleEditor.vue` |
| UI-20 | `src/views/ForbiddenView.vue`、`src/views/NotFoundView.vue` |
| 最终清理 | `src/main.ts`（迁移期间保留 Element Plus 引导；所有页面完成后移除依赖） |

当前 allowlist：7 个文件（含 3 个测试文件与 `main.ts`）。

## 设计决策

1. **版本锁定**：目标为 PrimeVue 4，不使用默认安装到的 PrimeVue 5。当前为 `primevue 4.5.5`、`@primeuix/themes 1.2.5`、`primeicons 7.0.0`。
2. **暗色优先**：`<html>` 默认带 `app-dark`，PrimeVue 通过 `darkModeSelector: '.app-dark'` 切换暗色 token；移除该 class 即进入亮色。
3. **主题持久化**：新增 localStorage key `kapoj.theme`（`dark` / `light`），默认 `dark`。现有 `kapoj.auth` 等 key 未改动。
4. **设计令牌**：颜色、间距、圆角、字体、阴影集中在 `src/styles/tokens.css`；PrimeVue preset 在 `src/theme/preset.ts`。两者必须同步修改。SFC 内不写散落 hex 色值。
5. **过渡期背景**：UI-01 已将 AppShell 与全局页面底色切到暗色；未迁移的 Element Plus 卡片仍为浅色，作为过渡期内容面板。异常页 `el-result` 的标题/副标题已在 `styles.css` 中做暗色兼容覆盖，页面迁移后移除该例外。
6. **同组件不混用**：`App.vue`、`DefaultLayout.vue`、`UserMenu.vue`、`AppNav.vue` 已只使用 PrimeVue。`main.ts` 是唯一允许同时安装两套库的引导文件。其余组件在迁移时不得混用。
7. **共享 UI 首批组件**：`AppSurface`、`AppPageHeader`、`AppEmptyState`、`AppErrorState`、`AppLoadingState`、`AppStatusTag`、`AppDifficultyTag`、`AppThemeToggle`。后续页面优先组合这些组件，避免重复样式。
8. **状态色**：AC 绿、WA 红、TLE 琥珀、MLE 紫、RE 粉红、CE 灰、Judging 青色脉冲、SE 红粉；难度 1–5 分别绿/青/琥珀/橙/红。全部通过 CSS 变量。
9. **确认框**：全局将 ConfirmDialog 的取消按钮设为 secondary outline。PrimeVue 默认 locale 为英文，后续 `confirm.require` 必须显式传 `acceptLabel: '确定'`、`rejectLabel: '取消'`（或统一封装 feedback composable）。
10. **不改业务层**：本次未修改路由 path/name/meta、Pinia store、API 请求/响应、权限、SSE、localStorage 草稿、分页/筛选 query。
11. **不引入第二套 UI 库**：除 PrimeVue/primeicons 外没有新增 UI 依赖；Monaco、md-editor-v3、KaTeX、highlight.js 保留。
12. **AppShell 结构**：桌面为 232px sticky 侧栏 + 60px sticky 顶栏；`<=900px` 切换为顶栏菜单按钮 + PrimeVue Drawer。Drawer 打开时对 `.app-frame` 设置 `inert` 与 `aria-hidden`，Esc 通过窗口 `keydown` 兜底关闭并把焦点还给菜单按钮。
13. **导航**：`AppNav` 同时供桌面侧栏和移动 Drawer 使用；支持前缀路由高亮、`aria-current`、键盘焦点；排行榜保持 M4 禁用，游戏化管理仅 `super_admin` 可见。
14. **用户菜单**：PrimeVue Menu + Avatar；保留金币余额、我的主页、装扮管理、编辑资料、退出登录；打开时刷新 `/users/me`，退出后回首页；游客显示登录/注册。
15. **顶栏与主题**：顶栏展示 `KAPOJ // CONSOLE` 与当前路由标题；`AppThemeToggle` 已接入 AppShell，主题切换会持久化到 `kapoj.theme`。
16. **表单方案**：PrimeVue 4 未内置表单校验，项目也不引入 `@primevue/forms`；使用原生 `<form>` + PrimeVue `InputText` / `Password` / `Textarea` / `Message` / `Button`，校验规则集中在 `src/validation.ts`，保持与 `apps/server/internal/user/validate.go` 同步。
17. **共享表单字段**：`AppFormField` 负责 label、必填/可选标记、error/hint、`aria-describedby` 与 `role="alert"`；内部用 `useId()` 生成稳定 id，避免手写重复 id。
18. **认证布局**：`AuthLayout` 在桌面展示品牌介绍与表单双栏，`<=900px` 隐藏介绍、只保留表单卡片；登录/注册路由在顶栏隐藏游客登录/注册按钮，避免与表单重复。
19. **资料页状态**：`ProfileEditView` 使用 `ref` 表单状态（规避 Vue 模板类型推断对 `reactive` + 插槽的误判），并通过 `currentUser` computed 读取 store；`fetchMe` 失败时保留缓存资料并显示警告，保存失败同时显示内联错误与 Toast。
20. **身份展示**：`IdentityShowcase` 使用 PrimeVue `Avatar` + 本地 `v-tooltip` 指令；图片优先，加载失败时回退首字母；无效称号色通过 `safeTitleColor(color, 'var(--kapoj-text)')` 使用令牌兜底，旧页面仍保留 `#303133` 默认值。
21. **尺寸**：`IdentityShowcase` 的 `small/default` 通过 18px/22px 内联尺寸和 `identity-showcase--small` 修饰类实现，不再依赖 Element Plus 的 `size` 数值。
22. **首页结构**：`HomeView` 使用 Hero + 系统状态、登录/游客/加载档案条、快速开始与成长中心卡片；所有入口均为现有路由，不新增 API 或路由。首页只读取 `auth` store，登录用户显示昵称、Rating 与金币，游客显示登录/注册引导。
23. **题库表格**：`ProblemListView` 使用 PrimeVue `DataTable` / `Column` / `Select` / `Paginator` / `ToggleSwitch` / `InputText`；保留 `keyword` / `tag` / `difficulty` / `includeHidden` 筛选、URL query 同步、20 条分页与最新请求序号。
24. **表格布局**：列宽压缩为 64 / 190 / 84 / 120 / 140，表格最小宽度 610px；平板/桌面无需横向滚动，手机在表格容器内横向滚动，不产生页面级横向溢出。未启用固定表头（20 行分页）。
25. **表单状态**：`ProblemListView` 的筛选状态使用 `ref` 对象（与资料页一致），规避 Vue 模板类型推断对 `reactive` + 插槽的误判。
26. **测试环境**：新增 `src/test/setup.ts` 提供 `matchMedia` / `ResizeObserver` polyfill，并在 `vite.config.ts` 的 Vitest `setupFiles` 中启用；仅影响测试，不改变生产运行时。
27. **题目详情布局**：`ProblemDetailView` 使用页头 + 元信息条 + 双栏内容（题面 / Tabs），`<=1024px` 堆叠；样例输入输出双栏在 `<=700px` 单列。右侧 Tabs 在桌面 sticky。
28. **Markdown 主题**：`MarkdownView` 新增 `theme?: 'light' | 'dark'`，默认 `light` 保持旧页面；题目详情根据 `useTheme().isDark` 传入，并将预览外层背景/内边距透明化，正文/标题使用 `--kapoj-text`，代码块保留 highlight.js 深色主题。
29. **统计与复制**：提交统计使用自定义 `dl` + `ProgressBar`；复制样例通过 `useToast` 反馈，失败时提示手动选择文本。
30. **插槽类型收窄**：PrimeVue 多层插槽会丢失 `problem` 的模板类型收窄，详情页在插槽内使用可选链/空数组兜底；不改变运行逻辑。
31. **提交页结构**：`SubmitView` 使用页头 + 元信息 + 编辑器卡片；PrimeVue `Select` / `Button` / Toast，Monaco `CodeEditor` 保持不变。工具栏在 `<=600px` 纵向排布、语言选择与提交按钮占满整行。
32. **草稿作用域**：提交页在 setup 时捕获稳定的 problem/contest id 生成 `kapoj:code:<practice|contest:id>:<problemId>:<lang>`；`onBeforeUnmount` 总是落盘当前草稿，避免卸载阶段 route 参数清空导致 NaN key 或丢稿。
33. **比赛上下文**：有效 `contestId` 显示“比赛提交”和返回比赛链接，提交与跳转 query 均保留 `contestId`；无效参数提示“比赛参数无效，将按练习提交处理”，不请求比赛。
34. **提交反馈**：空代码、提交成功、提交失败分别使用 Toast；成功后跳转 `/submissions?problemId=...`，比赛提交附带 `contestId`。
35. **测评记录结构**：`SubmissionsView` 使用页头 live 状态 + 刷新、筛选表单、DataTable、Paginator 与右侧详情 Drawer；草稿筛选与 URL applied 筛选分离，SSE/分页只使用 applied。
36. **SSE 与轮询**：保留 300ms 合并刷新、30 秒轮询、`EventSource` 打开/错误/消息处理和卸载清理；live 状态用脉冲圆点展示。
37. **详情抽屉**：自定义 2 列元信息 + 代码/错误/测试点表；`codeHidden` 时只显示比赛保护提示，不渲染代码与测试点。详情请求保留 detailSeq 竞态保护，失败显示 Toast + Drawer 内错误重试。
38. **测评表格**：列宽压缩为 50/120/150/95/75/75/80/50/140，最小宽度 840px；桌面 1200px 完整显示，手机在表格容器内横向滚动。行点击打开详情，行内用户/题目链接使用 `@click.stop`。
39. **比赛列表结构**：`ContestListView` 使用页头（登录提示、总数、新建按钮）+ 筛选表单 + DataTable + Paginator；草稿筛选与 URL applied 筛选分离，SSE/分页等后台刷新只使用 applied。
40. **管理员操作**：新建/编辑跳转现有 `contest-create` / `contest-edit` 路由；克隆使用 PrimeVue Dialog + `AppFormField`，默认标题为 `${title}（副本）`，提交后跳转编辑器；删除使用 `useConfirm`，显式中文 `删除 / 取消` 与危险按钮，成功后 Toast 并重新加载列表。
41. **比赛行展示**：已报名与状态使用 PrimeVue `Tag`；赛制/报名类型沿用 `utils/contest.ts`；行点击进入详情，操作按钮使用 `@click.stop`；`canManage` 继续通过 `hasRole` 支持 `super_admin`。
42. **比赛表格布局**：列宽约 220 / 70 / 80 / 190 / 100 / 170，最小宽度 830px；桌面 1200px 完整显示操作列，手机在表格容器内横向滚动，不产生页面级横向溢出。
43. **插槽内访问 store/router**：模板插槽中不直接使用 `auth.xxx` / `router.push`，改为 `isLoggedIn` computed 与事件方法；规避当前 vue-tsc 对非 ref setup 绑定误加 `.value` 的类型错误，行为不变。
44. **比赛详情结构**：`ContestDetailView` 使用 `AppPageHeader` + 元信息条 + Markdown 简介 + Tabs（题目/榜单/公告）；Tabs 启用 `lazy`，榜单首次打开时才请求。元信息使用 PrimeVue `Tag`，倒计时单独高亮。
45. **倒计时与刷新链**：保留服务端时钟偏移、1 秒 ticker、阶段变化触发后台刷新、SSE 300ms 合并刷新、30 秒轮询与卸载清理；`refreshTick` 同时驱动榜单重载。
46. **报名流程**：公开报名直接提交；密码/邀请码报名使用 Dialog + `AppFormField` / `Password` / `InputText`，成功后 Toast 并重新加载比赛；游客保留带 `redirect` 的登录链接。
47. **管理员操作与公告**：报名用户 Dialog、克隆 Dialog（默认 `${title}（副本）`）、删除 ConfirmDialog 均使用中文文案；公告支持发布/编辑/删除，正文用 `MarkdownView`，删除使用中文确认。
48. **榜单**：PrimeVue DataTable 替代 el-table；保留 `AC (n)` / `-n` / 分数语义、AC 颜色、`selfOnly` 警告、身份展示、空态与错误重试。
49. **Markdown 主题**：比赛简介与公告按 `useTheme().isDark` 传入 `MarkdownView` 主题；代码块继续使用 highlight.js 深色主题。
50. **比赛编辑结构**：`ContestEditView` 使用 `AppPageHeader`（取消/保存）+ 基础信息面板 + 题目面板；表单使用 `ref`，新建/编辑由路由参数决定。
51. **日期与冻结**：PrimeVue `DatePicker` 开启 `showTime`/`showSeconds`，显示格式 `yy-mm-dd`，提交仍为 ISO 字符串；比赛开始后冻结开始时间、赛制与题目列表，结束时间与保存仍可用。
52. **题目编辑**：题目下拉先用 `listProblems({ pageSize: 100 })` 与比赛已有题目预热，过滤时调用 `listProblems({ keyword, pageSize: 20 })`；编号自动 A–Z、输入归一化为单个大写字母，分数范围 1–1000，最多 26 题。
53. **密码留空语义**：编辑时密码/邀请码输入框始终为空，占位符提示“留空保留原密码/原邀请码”；保存仍提交空字符串，由服务端保留旧值。新建时提示 `4–64 字节`。
54. **PrimeVue 中文 locale**：新增 `src/theme/primevue-locale.ts` 局部中文配置，在 `main.ts` 合并到 PrimeVue 默认 locale；覆盖 DatePicker 月份/星期/按钮与 Select 空态等默认文案。`MarkdownEditor` 新增可选 `theme`（默认 light），比赛编辑页按应用主题传入 dark/light。
55. **用户主页结构**：`UserProfileView` 使用 `AppPageHeader`（昵称 + `@username`）+ `AppSurface`；资料卡为 PrimeVue `Avatar` + `IdentityShowcase` + 简介，统计区为自定义 `dl` 三列卡片。
56. **头像回退**：`Avatar` 绑定 `avatarUrl` 并监听 `error` 事件，加载失败自动切换为昵称/用户名首字母；切换路由用户时重置失败状态。
57. **主页状态**：加载使用 `AppLoadingState`，404 仍为“用户不存在”，其他错误为“加载失败，请稍后再试”，错误态提供重试；注册时间继续使用 `zh-CN` 日期格式，Rating/金币保持原值。
58. **成就墙结构**：`AchievementsView` 使用 `AppPageHeader`（右侧解锁计数 Tag）+ 游客 CTA + `SelectButton` 筛选 + 成就卡片网格；卡片使用 `AppSurface`。
59. **筛选与刷新**：全部/已解锁/未解锁的标签与数量由 achievements 计算；继续监听 `auth.isLoggedIn` 重新拉取，并用请求序号保护旧响应。
60. **成就卡片**：PrimeVue `Avatar` 展示图标，失败时回退成就名首字；状态用 `Tag`，奖励展示金币/徽章，已解锁显示时间，未解锁显示每条条件的 `ProgressBar`（完成项绿色）。
61. **状态与布局**：首次加载用 `AppLoadingState`，无数据错误用 `AppErrorState` 重试，已有数据时错误用 `Message` 横幅；空筛选用 `AppEmptyState`。网格 `auto-fill minmax(300px, 1fr)`，`<=640px` 单列。
62. **任务大厅结构**：`TasksView` 使用 `AppPageHeader`（登录时显示“进度只统计领取后的 AC”Tag）+ 游客 CTA + 任务卡片网格；卡片使用 `AppSurface`，状态用 PrimeVue `Tag`。
63. **领取流程**：`useConfirm` 确认框保留原文案与“领取/取消”中文按钮；成功后 Toast、刷新 `auth.fetchMe()` 并重新加载列表；失败 Toast 显示服务端消息；`claimingId` 同时阻止其他任务重复领取。
64. **任务卡片**：Markdown 描述按明暗主题渲染；展示题数/金币奖励/开始截止/完成时间；仅在 `claimedAt` 存在时显示进度条，`progressPercent` 保留上限 100 与题目数为 0 的特殊处理；完成后卡片使用成功色描边，底部按状态显示领取按钮或提示。
65. **状态与布局**：首次加载 `AppLoadingState`，无数据错误 `AppErrorState` 重试，已有数据时错误 `Message`；空态 `AppEmptyState`。网格 `auto-fill minmax(320px, 1fr)`，`<=640px` 单列。
66. **徽章商城结构**：`BadgeShopView` 使用 `AppPageHeader`（余额/装备上限 Tag）+ 游客 CTA + `SelectButton` 分类 + 徽章卡片网格；卡片使用 `AppSurface`。
67. **购买与装备**：`useConfirm` 保留“花费 X 金币/免费购买”中文确认；`runAction` 统一 Toast、刷新 `auth.fetchMe()` 与列表，`actingId` 阻止并发操作；装备/卸下成功后同步装备数量与上限禁用状态。
68. **徽章卡片**：分类/装备中/已拥有/已下线状态、价格、限量库存；按钮按游客/已拥有/售罄/余额不足/可购买切换，并展示装备已满、售罄、差额提示。
69. **Avatar 图片与回退**：发现 PrimeVue `Avatar` 同时传 `image` 与 `label` 时优先渲染 label，导致图片不显示；统一改为有图片时不传 label、失败后再回退首字。已同步修复 `AchievementsView` 与 `UserProfileView`。
70. **状态与布局**：加载/空态/错误重试/已有数据错误横幅与其它页面一致；网格 `auto-fill minmax(300px, 1fr)`，`<=640px` 单列。
71. **称号商店结构**：`TitleShopView` 使用 `AppPageHeader`（余额 Tag）+ 游客 CTA + 称号卡片网格；卡片使用 `AppSurface`，名称颜色由 `safeTitleColor` 限制。
72. **解锁与购买**：条件文本支持 Rating / AC 题目数 / 成就 code；条件进度条完成时绿色；购买沿用中文 ConfirmDialog，成功后 Toast、刷新 `auth.fetchMe()` 与列表。
73. **装备与状态**：装备中/已购买/已解锁/未解锁/已下线状态与按钮一一对应；装备后提示原称号已自动卸下；余额不足显示差额；已下线但已解锁/已购买的称号仍可兑换使用。
74. **颜色回退**：`safeTitleColor(item.color, 'var(--kapoj-text)')` 在非法/空颜色时使用主题文字色，保证暗色与亮色都可读。
75. **装扮管理结构**：`AppearanceView` 使用 `AppPageHeader` + 总览（余额/当前装扮 `IdentityShowcase`）+ 徽章/称号装备区 + 金币流水区，各区块使用 `AppSurface`。
76. **加载链**：`fetchMe` 后并行请求用户徽章、称号与装备上限，再加载金币流水；监听 `auth.user?.id` 变化重新加载；徽章/称号操作后只刷新对应区块并同步用户余额。
77. **装备操作**：`actingKey` 区分徽章/称号并阻止并发；徽章上限禁用与提示、称号颜色安全回退、已下线标记保持不变；Toast 与图片失败回退沿用现有模式。
78. **金币流水**：PrimeVue `DataTable` + `Paginator`，页码为组件内状态（20 条/页），保留原因中文标签、备注、正负金额颜色与格式；错误无数据时显示重试，已有数据时显示错误横幅。
79. **响应式**：总览与徽章网格在 `<=640px` 单列，金币流水在表格容器内横向滚动，不产生页面级横向溢出。
80. **管理页外壳**：`GamificationAdminView` 继续仅允许 `super_admin`；无权限显示 `AppEmptyState` 锁提示，有权限使用 PrimeVue `Tabs` + `lazy` 渲染四个管理面板。
81. **统一管理面板模式**：四个面板均使用工具栏（数量 + 新建）、`DataTable`、`AppErrorState`/`Message`、`Dialog` 表单、中文 `ConfirmDialog`、Toast 与 `includeInactive` 列表；错误与空态共用共享组件。
82. **成就面板**：保留 code 创建后不可改、组合条件 2–8 项、条件/奖励/排序校验；奖励徽章只能更换不能清除，已下线原徽章以禁用选项展示。
83. **徽章面板**：本地文件选择保留 2 MiB 与 PNG/JPEG/WebP/GIF 校验、Object URL 预览、保存后上传与失败重试；编辑时库存默认省略，勾选“调整库存”才提交 `stock`；外链图片沿用 http(s) 校验与缓存破坏。
84. **任务面板**：`MultiSelect` 远程搜索题目、最多 20 道；开始/结束使用两个 `DatePicker`；`hasRecords` 时冻结题目与奖励，Markdown 描述按主题渲染。
85. **称号面板**：`ColorPicker` 提供颜色，非法值回退主题文字色；解锁类型支持 Rating / AC 题数 / 成就 code，成就下拉支持加载失败重试。
86. **RBAC-D 管理后台授权**：管理入口与按钮从「`super_admin`/`hasRole`」迁移为 `auth.can`/`canAny`；游戏化管理不再仅 `super_admin` 可见，而是任一 `gamification.*.manage` 权限即可进入，tab 按权限生成。决策 13/41/80 中「游戏化管理仅 `super_admin`」「`canManage` 通过 `hasRole`」的表述已被 RBAC-D 取代（管理壳见 UI-36）。

## 验证记录

### 2026-11-05 / UI-00

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；仅有既有的 >500 kB chunk 提示（Monaco/编辑器相关），无新增构建错误。
- `corepack pnpm --filter web test`：通过，20 个测试文件 / 98 个测试。新增 10 个测试：主题切换 4、状态标签 4、难度标签 2。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：新增/修改的 UI 与主题文件无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；`main.ts` 与 36 个 legacy allowlist 文件保留。
- 浏览器检查（`http://127.0.0.1:5173/`）：
  - 控制台无 error。
  - `html.app-dark`、`data-theme=dark`、`color-scheme: dark` 生效。
  - PrimeVue token：暗色 `--p-primary-color: #00d9f5`、`--p-surface-900: #0c121c`；移除 `app-dark` 后亮色 `--p-primary-color: #007f94`。
  - `Toast` 可显示成功消息；`ConfirmDialog` 可显示，取消按钮为 secondary outline，确定按钮为青色主按钮。
- 移动端：共享组件含响应式规则；完整移动端页面检查待 UI-01 建立 AppShell 后执行。

### 2026-11-05 / UI-01

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；仅保留既有的 >500 kB chunk 提示（Monaco/编辑器相关）。
- `corepack pnpm --filter web test`：通过，22 个测试文件 / 104 个测试。新增 6 个：`AppNav` 4（导航分组、管理员可见性、M4 禁用、前缀路由高亮）、`UserMenu` 2（游客入口、登录态触发器）。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：`layouts/`、`UserMenu.vue`、`components/layout/`、`components/ui/`、`theme/` 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 从 36 降至 34。
- 浏览器检查（`http://127.0.0.1:5173/`）：
  - 690px 窄屏：侧栏隐藏、菜单按钮显示、Drawer 打开后焦点落在关闭按钮；`.app-frame` 同时具有 `inert` 与 `aria-hidden="true"`。
  - 390px 移动视口（同源 iframe）：侧栏 `display:none`、菜单按钮 `display:flex`、页面标题品牌显示、主内容 padding `12px 8px`；Drawer 宽度 300px，导航项完整，背景 frame inert。
  - 1200px 桌面视口（同源 iframe）：侧栏 `232px`、主内容 padding `24px`、顶栏上下文显示、移动菜单按钮隐藏。
  - Esc 关闭 Drawer 后焦点回到“打开导航”按钮；点击 Drawer 中“题库”会关闭抽屉、更新 `aria-current` 与顶栏标题。
  - 主题切换：暗色 `--p-primary-color: #00d9f5` → 亮色 `#007f94`，body 背景 `#f4f7fb`，切回暗色后 `kapoj.theme` 持久化为 `dark`。
  - 游客菜单显示登录/注册；伪造 `super_admin` 会话后侧栏出现“游戏化管理”，用户菜单显示 `Alice / @alice / 金币余额 1,234` 与四个菜单项；点击“退出登录”后本地会话清空并回到首页。
  - `/forbidden` 的 `el-result` 标题/副标题在暗色画布上分别为 `#e6edf3` / `#94a3b8`，可读。
  - 控制台仅有 `/api/users/me` 与 `/api/auth/logout` 的 404，原因是浏览器验收时后端未启动；没有 Vue 运行时或 UI 错误。

### 2026-11-05 / UI-02

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；仅保留既有的 >500 kB chunk 提示（Monaco/编辑器相关）。
- `corepack pnpm --filter web test`：通过，27 个测试文件 / 129 个测试。新增 25 个：`validation` 12、`AppFormField` 2、`LoginView` 3、`RegisterView` 4、`ProfileEditView` 4。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：认证页面、`validation.ts`、`AppFormField`、`AuthLayout` 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 从 34 降至 30。
- 浏览器检查（`http://127.0.0.1:5173/`）：
  - `/login`（690px）：表单卡片 420px，介绍栏隐藏；空提交显示“请输入用户名或邮箱 / 请输入密码”，输入框带 `aria-invalid="true"` 与对应 `aria-describedby`；填写后提交返回 404 时内联显示“请求失败（HTTP 404）”，按钮恢复可用。
  - `/register`（690px）：用户名/邮箱/密码/昵称四字段，昵称标记“可选”，用户名与密码 hint 正常；浏览器布局与输入顺序正确。
  - 1200px 桌面（同源 iframe）：认证双栏布局为 `436px / 420px`，介绍栏显示品牌、标题、特性与状态；顶栏游客登录/注册按钮隐藏。
  - 390px 移动（同源 iframe）：介绍栏隐藏，面板宽 374px，表单宽 340px，主内容 padding `12px 8px`。
  - `/settings/profile`（缓存会话）：`fetchMe` 404 时显示“无法刷新最新资料，正在使用本地缓存”，表单从缓存填充；预览显示 Alice / @alice / 简介 / Rating 1680 / 金币 1234。
  - 资料页空昵称与非法头像 URL 分别显示“请输入昵称”“头像地址需为 http 或 https 链接”；有效提交返回 404 时同时显示内联错误与“保存失败”Toast。
  - 移动端资料页表单在预览之前；亮色主题下 body `#f4f7fb`、面板白色、文字 `#0f172a`，认证卡片正常。
  - 控制台仅有 `/api/users/me`、`/api/auth/logout` 等 404（后端未启动）；无 Vue/UI 错误。

### 2026-11-05 / UI-03

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；仅保留既有的 >500 kB chunk 提示。
- `corepack pnpm --filter web test`：通过，27 个测试文件 / 130 个测试。`IdentityShowcase` 5 个测试覆盖称号/徽章、空装备、无效颜色、尺寸和图片失败回退；`gamification` 增加自定义 fallback 断言。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：`IdentityShowcase.vue` 与 spec 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 从 30 降至 28。
- 浏览器组件探针（开发服务器动态挂载）：
  - 默认徽章 22px、small 徽章 18px，`identity-showcase--small` 生效。
  - `aria-label` 分别为“早鸟”“探索者”；PrimeVue Tooltip 悬停显示“早鸟”。
  - 无效称号色显示为 `rgb(230, 237, 243)`（`--kapoj-text`）；有效色 `#7C3AED` 保持紫色。
  - 徽章图片 404 时自动回退为首字母“探”，不显示破图。
  - 空装备仍不渲染根节点。

### 2026-11-05 / UI-04

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；仅保留既有的 >500 kB chunk 提示。
- `corepack pnpm --filter web test`：通过，28 个测试文件 / 133 个测试。新增 `HomeView` 3 个测试：游客入口、登录态档案条与装扮入口、登录档案加载态。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：`HomeView.vue` 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 从 28 降至 27。
- 浏览器检查（`http://127.0.0.1:5173/`）：
  - 游客 690px：Hero、系统状态、游客引导与快速开始/成长中心卡片正常；卡片网格两列；链接覆盖 `/problems`、`/contests`、`/submissions`、`/achievements`、`/tasks`、两个商店。
  - 登录态 690px（伪造 localStorage）：档案条显示 Alice / @alice / Rating 1680 / 金币 1,234；出现 `/appearance` 与 `/settings/profile`，游客引导消失。
  - 1200px 桌面（同源 iframe）：Hero 双栏 `493px / 329px`，快速开始三列；侧栏与顶栏正常。
  - 亮色主题：body `#f4f7fb`、Hero/卡片白色、文字 `#0f172a`；切回暗色后持久化正常。
  - 游客首页控制台无 error；登录态验收使用伪造会话，后端未启动时 `/users/me` 404 属环境预期。

### 2026-11-05 / UI-05

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；仅保留既有的 >500 kB chunk 提示。
- `corepack pnpm --filter web test`：通过，29 个测试文件 / 139 个测试。新增 `ProblemListView` 6 个测试：路由 query 初始化、筛选 trim/重置页码、标签筛选与分页、管理员未公开开关、空态、错误重试。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：`ProblemListView.vue` 与 spec 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 从 27 降至 26。
- 浏览器检查（临时 mock API `127.0.0.1:18080` + Vite 代理，验收后已关闭，未提交 mock 文件）：
  - 690px：表格 20 行、表头完整、总题数 45、分页 3 页；难度标签、标签 chip、通过/提交率正常；页面无横向溢出。
  - 分页点击第 2 页 → `/problems?page=2`，首行为“示例题目 21”。
  - 标签点击 → `/problems?tag=math`；搜索关键词 → `/problems?keyword=sum&tag=math`；难度选择“提高”并提交 → `/problems?difficulty=3`；重置 → `/problems`。
  - 管理员伪造会话：显示“新建题目”、`/problems/new` 链接与“显示未公开”开关；开启后第 3 行显示“未公开”。
  - 390px 同源 iframe：筛选单列、操作按钮 340px、表格容器 338px/610px 内部横向滚动，页面 `scrollWidth` 390。
  - 控制台无 error / warning。
- 已知限制：手机端表格需在容器内横向滚动；未启用固定表头。

### 2026-11-05 / UI-06

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；仅保留既有的 >500 kB chunk 提示。
- `corepack pnpm --filter web test`：通过，29 个测试文件 / 142 个测试。`ProblemDetailView` 3 个测试覆盖页面渲染、跨题目统计竞争、404 错误；`MarkdownView` 新增明暗主题类测试。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：`ProblemDetailView.vue`、spec、`MarkdownView.vue` 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 从 26 降至 24。
- 浏览器检查（临时 mock API `127.0.0.1:18080` + Vite 代理，验收后已关闭，未提交 mock 文件）：
  - 816px：标题、难度/类型/时限/内存/来源/标签、题面 Markdown、代码块、两组样例、Tabs 正常；题面卡片背景 `#111927`，Markdown 预览透明且正文 `#e6edf3`。
  - Tabs 切换到“提交统计”显示 100 / 50 / 50.0%，进度条 `aria-valuenow=50`；“解题思路”“解题代码”“关联比赛”内容正常。
  - 复制按钮在剪贴板不可用时显示“复制失败，请手动选择文本”Toast，错误路径正常。
  - 未公开题目 + `contestId=5`：显示比赛保护提示，提交链接带 `?contestId=5`，“关联比赛”显示“返回比赛 #5”。
  - 管理员伪造会话：显示“编辑”入口，链接为 `/problems/1/edit`。
  - 390px 同源 iframe：详情单列、样例单列（340px）、TabList 340px、页面 `scrollWidth` 390。
  - 亮色主题：题面卡片白色、Markdown 正文 `#0f172a`、代码块保持深色高亮。
  - 控制台无 error / warning（首次 Vite 依赖预优化时的 504 在重载后消失，属开发服务器行为）。

### 2026-11-05 / UI-07

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；SubmitView 仍包含 Monaco chunk，属既有体积。
- `corepack pnpm --filter web test`：通过，30 个测试文件 / 149 个测试。新增 `SubmitView` 7 个测试：草稿/语言切换、空代码、提交成功、比赛上下文、无效比赛参数、404、卸载落盘。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：`SubmitView.vue` 与 spec 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 从 24 降至 23。
- 浏览器检查（临时 mock API `127.0.0.1:18080` + Vite 代理，验收后已关闭，未提交 mock 文件）：
  - 816px：页头/元信息/语言筛选/提交按钮/编辑器正常；C++ 草稿从 localStorage 载入 Monaco；编辑器高度 520px。
  - 提交成功：Toast “已提交 #42 / 正在评测”，跳转 `/submissions?problemId=1`。
  - 比赛上下文：`?contestId=5` 显示“比赛提交”和“返回比赛：Weekly Contest”，提交后跳转 `/submissions?problemId=1&contestId=5`。
  - 无效比赛参数：Toast “比赛参数无效 / 将按练习提交处理”，不请求比赛。
  - `/problems/2/submit` 404：显示“加载失败 / 题目不存在 / 重试”，返回题库链接 `/problems`，编辑器不渲染。
  - 390px 同源 iframe：语言选择与提交按钮 340px，编辑器 338×520，页面 `scrollWidth` 390，Monaco 正常加载。
  - 亮色主题：卡片白色、文字深色；Monaco 保持 vs-dark。
  - 控制台无 error / warning。

### 2026-11-05 / UI-08

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；DataTable 产生独立 chunk，属正常代码分割。
- `corepack pnpm --filter web test`：通过，31 个测试文件 / 155 个测试。新增 `SubmissionsView` 6 个测试：路由 query/行渲染、昵称筛选与分页、SSE 状态与合并刷新、详情抽屉、codeHidden、详情失败。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：`SubmissionsView.vue` 与 spec 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 从 23 降至 22。
- 浏览器检查（临时 mock API `127.0.0.1:18080` + Vite 代理，验收后已关闭，未提交 mock 文件）：
  - 816px：live 状态“实时刷新”、20 行、45 条记录、3 页分页、筛选 6 项、身份展示与状态标签正常。
  - 昵称筛选 `Alice` → `/submissions?nickname=Alice`；分页第 2 页 → `/submissions?nickname=Alice&page=2`。
  - 详情抽屉：代码、错误信息、测试点表正常；`codeHidden` 行显示比赛保护提示且不渲染代码/测试点。
  - 桌面 1200px 同源 iframe：表格容器 868/868，完整显示“提交时间”列。
  - 390px 同源 iframe：页面 `scrollWidth` 390，筛选两列 166px，表格容器 338px 内部横向滚动，详情单列（322px），Drawer 358px。
  - 亮色主题：卡片白色、文字深色、状态色正常。
  - 控制台无 error / warning。

### 2026-11-05 / UI-09

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；仅有既有的 >500 kB chunk 提示（Monaco 相关），无新增错误。
- `corepack pnpm --filter web test`：通过，32 个测试文件 / 162 个测试。新增 `ContestListView` 7 个测试：路由 query/行渲染、关键词筛选与分页、游客隐藏管理控件、行点击详情、克隆 Dialog、中文删除确认、空态/错误态。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：`ContestListView.vue` 与 spec 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 从 22 降至 21。
- 浏览器检查（临时 mock API `127.0.0.1:18080` + Vite 代理，验收后已关闭，未提交 mock 文件）：
  - 816px：20 行、34 场比赛、2 页分页、5 项筛选、管理员新建/编辑/克隆/删除操作正常；比赛/赛制/状态/时间/规模/操作列与 Tag 展示正常。
  - 关键词筛选 `第 1 届` → `/contests?keyword=第+1+届`，仅 1 行；重置清空 query。
  - “只看我报名的” → `/contests?mine=true`，20 行均带“已报名”。
  - 分页第 2 页 → `/contests?page=2`，14 行。
  - `/contests?includeHidden=true&mode=icpc&status=upcoming`：15 场，赛制均为 ICPC、状态均为未开始，筛选项回填正确。
  - 克隆 Dialog：默认标题“第 1 届算法挑战赛（副本）”，改为 `Copy Contest` 后提交，Toast“比赛已克隆”，跳转 `/contests/99/edit`。
  - 删除确认：文案“删除比赛 / 确定删除比赛「第 1 届算法挑战赛」吗？”，按钮“取消 / 删除”；确认后 Toast“已删除”并重新加载列表。
  - 桌面 1200px 同源 iframe：表格容器 868/868，完整显示“操作”列。
  - 390px 同源 iframe：页面 `scrollWidth` 390，筛选两列 166px，表格容器 338/830 内部横向滚动，克隆 Dialog 359px（输入框 317px），确认框 343px。
  - 亮色主题：卡片白色、文字深色、状态色正常。
  - 控制台无 error / warning。

### 2026-11-05 / UI-10

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；仅有既有的 >500 kB chunk 提示（Monaco 相关），无新增错误。
- `corepack pnpm --filter web test`：通过，34 个测试文件 / 176 个测试。新增 14 个测试：
  - `ContestDetailView` 10 个：加载与倒计时/题目/公告、游客登录与未开始空态、公开报名、密码报名 Dialog、报名用户/编辑/克隆、中文删除确认、公告 CRUD、SSE 刷新与清理、404 返回列表、共享 DataTable。
  - `ContestRanklist` 4 个：ICPC 行与 AC/罚时、IOI 总分与 `selfOnly` 警告、`refreshTick` 重载、错误重试恢复。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：`ContestDetailView.vue`、`ContestRanklist.vue` 与两份 spec 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 从 21 降至 19。
- 浏览器检查（临时 mock API `127.0.0.1:18080` + Vite 代理，验收后已关闭，未提交 mock 文件）：
  - 816px：标题、4 个状态 Tag、倒计时、开始/结束时间与报名人数正常；3 道题、提交按钮、Tabs 题目/榜单/公告正常；Markdown 简介含列表与代码块。
  - 榜单：2 行、列 `#/选手/通过 罚时/A/B/C`，`2 / 120`、`AC (2)`、`-1` 与 AC 颜色正常；SSE 后生成时间从 `20:00:02` 变为 `20:00:03`，确认榜单随刷新链重载。
  - 公告：发布/编辑/删除完整走通，Markdown 渲染与中文删除确认正常。
  - 报名用户 Dialog 显示 2 行；克隆 Dialog 默认“Weekly Contest 42（副本）”，提交后 Toast 并跳转 `/contests/99/edit`；详情删除确认文案与 `取消/删除` 按钮正常，取消后停留原页。
  - 密码报名：Dialog 显示中文密码占位符，提交后 Toast“报名成功”，Tag 更新为“已报名”，报名按钮消失。
  - 游客：显示“登录后报名”，链接为 `/login?redirect=/contests/5`。
  - 未开始且无题目：显示“题目将在比赛开始后公布。”；404：显示“比赛不存在或无权访问”、重新加载与返回列表。
  - 桌面 1200px 同源 iframe：内容宽 920px，题目表格 832/832 无横向滚动。
  - 390px 同源 iframe：页面 `scrollWidth` 390，内容 374px；题目表格 302/361、榜单表格 302/316 为容器内滚动；克隆 Dialog 359px，删除确认框 230px。
  - 亮色主题：卡片白色、文字深色，Markdown 切换为 light；暗色下 Markdown 代码块保持深色。
  - 控制台无 error / warning。

### 2026-11-05 / UI-11

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；仅有既有的 >500 kB chunk 提示（Monaco 相关），无新增错误。
- `corepack pnpm --filter web test`：通过，35 个测试文件 / 186 个测试。新增 10 个测试：
  - `ContestEditView` 9 个：新建保存、编辑加载与密码留空、标题校验、时间顺序校验、题目未选校验、题目增删排序、远程搜索、开始后冻结、加载失败。
  - `MarkdownEditor` 1 个：显式传递 dark 主题。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：`ContestEditView.vue` 与 spec 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 从 19 降至 18。
- 浏览器检查（临时 mock API `127.0.0.1:18080` + Vite 代理，验收后已关闭，未提交 mock 文件）：
  - 816px 编辑页：双列 363/363，标题长度提示、开始/结束时间、IOI/密码报名、密码留空占位符、公开/计 Rating 开关、2 行题目与远程下拉均正常。
  - DatePicker 面板：头部“2026 11月”、星期“一…日”，确认中文 locale 生效。
  - 题目操作：添加得到 A/B/C，输入小写 d 归一化为 D；下移后 B/A/C；删除后 B/C。保存 PUT 载荷中 `password`/`inviteCode` 为空字符串，题目顺序为 B(problem 2)、C(problem 3)；Toast“比赛已更新”并跳转 `/contests/5`。
  - 新建页默认 ICPC / 公开报名 / 公开 / 不计 Rating；选择题目 `Add Two Numbers` 后保存，POST 载荷正确，Toast“比赛已创建”，跳转 `/contests/99`。
  - 已开始比赛 `/contests/7/edit`：开始时间、赛制、添加题目、编号、题目下拉、分数、上移/下移/删除均禁用；结束时间与保存可用；显示冻结提示。
  - 390px 同源 iframe：单列 340px，页面 `scrollWidth` 390，题目表格 338/464 容器内滚动，编辑器宽 340px。
  - 1200px 同源 iframe：双列 427/427，内容 920px，题目表格 868/868 无横向滚动。
  - 亮色主题：卡片白色、文字深色，MarkdownEditor 切换为 light；暗色下为 dark。
  - 控制台无 error / warning。

### 2026-11-05 / UI-12

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；仅有既有的 >500 kB chunk 提示（Monaco 相关），无新增错误。
- `corepack pnpm --filter web test`：通过，36 个测试文件 / 193 个测试。新增 `UserProfileView` 7 个测试：资料/身份/统计、加载态、空简介、404 重试、通用错误、路由 id 切换、头像失败回退。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：`UserProfileView.vue` 与 spec 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 从 18 降至 17。
- 浏览器检查（临时 mock API `127.0.0.1:18080` + Vite 代理，验收后已关闭，未提交 mock 文件）：
  - 816px `/users/7`：页头 Alice / `@alice`，头像图片、称号“卷王”与徽章、两行简介、Rating 1680、金币 1234、注册时间 2026/1/1 正常；资料卡宽 720px。
  - `/users/8`：头像 URL 404 后自动回退首字母 B；空简介显示“这个人还没有写简介。”；统计正常。
  - `/users/404`：显示“用户不存在”，重试按钮可再次请求；`/users/500` 显示“加载失败，请稍后再试”。
  - 390px 同源 iframe：页面 `scrollWidth` 390，资料卡 374px，头像 64px，资料头部纵向排列，统计单列 340px。
  - 1200px 同源 iframe：资料卡 720px，资料头部横向，统计三列 215px，无横向溢出。
  - 亮色主题：卡片白色、文字深色；暗色 token 正常。
  - 控制台无 error / warning。

### 2026-11-05 / UI-13

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；仅有既有的 >500 kB chunk 提示（Monaco 相关），无新增错误。
- `corepack pnpm --filter web test`：通过，37 个测试文件 / 200 个测试。新增 `AchievementsView` 7 个测试：计数/奖励/进度/解锁时间、筛选、游客 CTA、登录态重载、空筛选、错误重试、图标回退。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：`AchievementsView.vue` 与 spec 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 从 17 降至 16。
- 浏览器检查（临时 mock API `127.0.0.1:18080` + Vite 代理，验收后已关闭，未提交 mock 文件）：
  - 慢响应模式：先显示“正在加载成就”且无卡片，数据到达后渲染 6 张卡片。
  - 816px：解锁计数 2/6，筛选为全部（6）/已解锁（2）/未解锁（4）；奖励金币/徽章、解锁时间 2026/1/1 08:00:00 正常；进度条 60%、100%、60%、0%，完成项为绿色。
  - 筛选：已解锁仅 2 张（首战告捷、连胜达人），未解锁 4 张，全部恢复 6 张。
  - 游客：显示登录 CTA，链接 `/login?redirect=/achievements`，计数 0/6，6 张卡片均为未解锁。
  - 500 错误：显示“加载成就失败”与重试；恢复后重试加载 6 张卡片。
  - 图标失败（404）回退成就名首字“全”。
  - 390px 同源 iframe：页面 `scrollWidth` 390，网格单列 374px，筛选条 353px。
  - 1200px 同源 iframe：网格两列 452px，页面无横向溢出。
  - 亮色主题：卡片白色、文字深色，进度条底色正常；暗色 token 正常。
  - 控制台无 error / warning。

### 2026-11-05 / UI-14

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；仅有既有的 >500 kB chunk 提示（Monaco 相关），无新增错误。
- `corepack pnpm --filter web test`：通过，38 个测试文件 / 208 个测试。新增 `TasksView` 8 个测试：状态/奖励/时间/进度、领取确认与刷新、领取失败、游客 CTA、登录态重载、空态、加载态、错误重试。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：`TasksView.vue` 与 spec 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 从 16 降至 15。
- 浏览器检查（临时 mock API `127.0.0.1:18080` + Vite 代理，验收后已关闭，未提交 mock 文件）：
  - 慢响应模式：先显示“正在加载任务”且无卡片，数据到达后渲染 7 张任务卡。
  - 登录态：状态覆盖可领取/进行中/已完成/已过期/未开始/已结束/已下线；奖励、Markdown 描述、开始/截止/完成时间、3 / 5 进度条与完成卡片描边正常。
  - 领取：确认框文案“确定领取任务「新手任务」吗？领取后需在截止时间前完成全部题目。”，按钮“取消/领取”；确认后 Toast“领取成功，完成全部题目即可获得奖励”，卡片变为进行中并显示进度。
  - 500 错误显示“加载任务失败”与重试，恢复后重试加载 7 张卡片。
  - 游客：显示登录 CTA 与登录链接 `/login?redirect=/tasks`；可领取任务显示“登录后领取”，不显示领取按钮。
  - 390px 同源 iframe：页面 `scrollWidth` 390，网格单列 374px。
  - 1200px 同源 iframe：网格两列 452px，页面无横向溢出。
  - 亮色主题：卡片白色、文字深色，进度条底色正常；暗色 token 正常。
  - 控制台无 error / warning。

### 2026-11-05 / UI-15

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；仅有既有的 >500 kB chunk 提示（Monaco 相关），无新增错误。
- `corepack pnpm --filter web test`：通过，39 个测试文件 / 217 个测试。新增 `BadgeShopView` 9 个测试：分类/状态/库存/余额、分类筛选、购买确认与刷新、购买失败、装备/卸下、游客 CTA、登录态重载、加载/空/错误、图标回退。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：`BadgeShopView.vue` 与 spec 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 从 15 降至 14。
- 浏览器检查（临时 mock API `127.0.0.1:18080` + Vite 代理，验收后已关闭，未提交 mock 文件）：
  - 慢响应模式：先显示“正在加载徽章商城”，数据到达后渲染 8 张卡片。
  - 816px：分类为全部/竞赛/成就/外观/新手；装备上限 1/2、余额 500 金币；限量库存、免费、余额不足、售罄、已拥有/装备中/已下线状态正常。
  - 分类筛选：成就 2 张、竞赛 3 张、全部 8 张。
  - 图片：有效图标显示；故意 404 的图标回退首字。
  - 购买：确认“确定花费 100 金币购买徽章「竞赛徽章」吗？”；确认后余额 400、库存 4、卡片变为已拥有并可装备，Toast“购买成功，可在装扮管理中装备”。
  - 装备“老玩家”后成为 2/2，元老徽章装备按钮禁用；卸下“荣耀勋章”后回到 1/2 并解除禁用。
  - 500 错误显示“加载徽章商城失败”与重试，恢复后加载 8 张卡片。
  - 游客：CTA、登录链接 `/login?redirect=/shop/badges`，8 张卡片均为“登录后购买”并提示登录后操作。
  - 390px 同源 iframe：页面 `scrollWidth` 390，网格单列 374px，筛选条 326px。
  - 1200px 同源 iframe：网格两列 452px，页面无横向溢出。
  - 亮色主题：卡片白色、文字深色、价格橙色正常。
  - 控制台仅出现 mock 故意 404 的 `missing-badge.svg` 资源错误；应用无异常，回退逻辑正常。

### 2026-11-05 / UI-16

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；仅有既有的 >500 kB chunk 提示（Monaco 相关），无新增错误。
- `corepack pnpm --filter web test`：通过，40 个测试文件 / 224 个测试。新增 `TitleShopView` 7 个测试：状态/价格/条件/进度、购买确认与刷新、购买失败、装备/卸下、游客 CTA、登录态重载、加载/空/错误。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：`TitleShopView.vue` 与 spec 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 从 14 降至 13。
- 浏览器检查（临时 mock API `127.0.0.1:18080` + Vite 代理，验收后已关闭，未提交 mock 文件）：
  - 慢响应模式：先显示“正在加载称号商店”，数据到达后渲染 7 张称号卡。
  - 登录态：余额 500 金币；已解锁/已购买/装备中/未解锁/已下线状态正常；Rating、AC 题目数、成就 code 条件与 1700/1600、20/100、1/1 进度正常。
  - 称号颜色：有效 `#RRGGBB` 正常显示；非法颜色回退主题文字色（暗色 `rgb(230,237,243)`、亮色 `rgb(15,23,42)`）。
  - 购买：确认“确定花费 100 金币购买称号「荣耀王者」吗？”；确认后余额 400，卡片变为已购买并可装备。
  - 装备“老兵”后“传奇”自动卸下，提示“已装备称号，原称号已自动卸下”；随后卸下“老兵”恢复已购买状态。
  - 500 错误显示“加载称号商店失败”与重试，恢复后加载 7 张卡片。
  - 游客：CTA、登录链接 `/login?redirect=/shop/titles`，7 张卡片均为“登录后查看”并提示登录后查看进度。
  - 390px 同源 iframe：页面 `scrollWidth` 390，网格单列 374px。
  - 1200px 同源 iframe：网格两列 452px，页面无横向溢出。
  - 亮色主题：卡片白色、文字深色、称号颜色正常。
  - 控制台无 error / warning。

### 2026-11-05 / UI-17

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；仅有既有的 >500 kB chunk 提示（Monaco/编辑器相关），无新增错误。
- `corepack pnpm --filter web test`：通过，41 个测试文件 / 233 个测试。新增 `AppearanceView` 9 个测试：总览/徽章/称号/流水、徽章装备与上限、称号装备/卸下、流水分页、流水错误重试、页面错误重试、加载/空态、头像回退。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：`AppearanceView.vue` 与 spec 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 从 13 降至 12。
- 浏览器检查（临时 mock API `127.0.0.1:18080` + Vite 代理，验收后已关闭，未提交 mock 文件）：
  - 慢响应模式：先显示“正在加载装扮信息”，随后渲染总览与各区块。
  - 总览：余额 500 金币，当前装扮显示称号“荣耀王者”与已装备徽章；徽章区 3 件、称号区仅 2 件已购买。
  - 徽章图片：有效图标显示；404 图标回退首字“绝”；无图徽章回退“老”。发现并修复了装扮徽章头像缺少 `@error` 回退的问题。
  - 徽章装备：装备“老玩家”后 `2 / 2`，出现装备已满提示，已下线徽章装备按钮禁用；卸下“竞赛徽章”后恢复 `1 / 2` 并解除禁用。
  - 称号装备：装备“老兵”后“荣耀王者”自动卸下，当前装扮同步更新；卸下后仅保留徽章。
  - 金币流水：时间/原因/备注/金额正常，`+100` 绿色、`-50` 红色、`+30` 绿色；第 2 页显示“管理员调整 补偿 +77”。
  - 流水 500 错误：徽章与称号仍正常加载，流水区显示错误与重试；恢复后加载 3 行。
  - 390px 同源 iframe：页面 `scrollWidth` 390，总览/徽章单列 340px，流水表格容器 338/410 内部滚动。
  - 1200px 同源 iframe：总览 260/594，徽章三列 282px，页面无横向溢出。
  - 亮色主题：面板白色、文字深色、收入绿色正常。
  - 控制台仅出现验收时故意触发的流水 500 与缺失图标 404；应用无异常。

### 2026-11-05 / UI-18

- `corepack pnpm --filter web build`：通过。Vite 8.3.5；仅有既有的 >500 kB chunk 提示（Monaco/编辑器相关），无新增错误。
- `corepack pnpm --filter web test`：通过，46 个测试文件 / 255 个测试。新增 22 个测试：
  - 管理外壳 2 个：非超级管理员拦截、四 Tab 懒加载。
  - 成就面板 5 个：列表、创建、组合条件编辑、校验、删除。
  - 徽章面板 5 个：列表、限量创建与图片上传、库存保留/调整、校验、删除。
  - 任务面板 5 个：列表、远程选题创建、有领取记录冻结、校验、删除。
  - 称号面板 5 个：列表、Rating 创建、成就条件编辑、校验、删除。
- `corepack pnpm --filter web lint`：通过，ESLint + Prettier 无报错。
- Element Plus 残留检查：管理视图、四个面板与全部 spec 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 从 12 降至 7。
- 浏览器检查（临时 mock API `127.0.0.1:18080` + Vite 代理，验收后已关闭，未提交 mock 文件）：
  - 超级管理员进入 `/admin/gamification`：四个 Tab（成就/任务/徽章/称号）懒加载正常。
  - 成就：2 行，条件/金币/徽章奖励/上下架正常；新建“浏览器成就”后计数 3 与 Toast；删除确认后恢复 2。
  - 任务：2 行，时间/奖励/题目数/状态/领取记录正常；新建 Dialog 中 MultiSelect 加载 `#1 Two Sum`、`#2 Add Two Numbers`，含 2 个日期时间选择器与 Markdown 编辑器。
  - 徽章：2 行，图片 `/api/admin-badge.svg` 正常；新建 Dialog 含本地文件输入与“无图片”预览位。
  - 称号：2 行，Rating 与成就 code 条件正常；新建 Dialog 含 ColorPicker 与解锁类型选择。
  - 列表失败：返回 500 时显示“加载失败”与重试，恢复后加载 2 行。
  - 390px 同源 iframe：页面 `scrollWidth` 390，Tab 列表 340px，表格容器 302/805 内部滚动。
  - 1200px 同源 iframe：管理面板 920px，表格容器 832/832 无横向滚动。
  - 亮色主题：面板白色、文字深色；暗色 token 正常。
  - 首次访问因 Vite 依赖预构建出现 504，刷新后无错误；控制台仅保留验收时故意触发的 500。

### 2026-11-05 / UI-30

- `corepack pnpm --filter web test`：50 个测试文件 / 280 个测试通过。新增/更新 `LeaderboardView`、`RatingHistoryCard`、`RatingLevelName`、`rating` 工具与 `HomeView` 断言；`HomeView` 系统状态更新为 M4-C。
- `corepack pnpm --filter web lint`：ESLint 与 Prettier 通过。
- `corepack pnpm --filter web build`：通过；仅有既有的 >500 kB chunk 提示，无新增错误。
- 浏览器检查（headless Chrome/CDP，Vite 5173 代理后端 18080）：
  - 游客 `/leaderboard`：Rating/活跃度/共享三个 Tab 正常；Rating 每页 20 行、分页第 2 页 2 行；青名颜色 `rgb(3, 168, 158)`；黑红名首字符 `rgb(0, 0, 0)`、其余 `rgb(255, 0, 0)`，用户链接 `title`/`aria-label` 为“昵称（黑红名）”，等级列显示“青名/黑红名”文字并带等级色边框与圆点；活跃度/共享榜无等级列，共享榜显示 `m2badmin 120`。
  - 游客活跃度 Tab 显示登录 CTA；无令牌调用 `POST /api/activity/signin` 返回 401。
  - `m2bsolver1` 活跃度 Tab 显示今日/总活跃度 17，按钮“今日已签到”禁用。一次性用户 `m4ce2e215243` 首次签到 Toast 为 `+5 活跃度 / +5 金币`，摘要 0→5/5、金币 0→5、活跃度榜刷新；重复点击禁用按钮不产生请求，重复 API 返回 `awarded=false`，刷新后状态保持。
  - `/users/2` Rating 历史表格（OI，公共比赛链接 `/contests/3`）和 SVG 折线（2 个数据点）正常；临时私密比赛行显示“私密比赛”且不生成链接；`/users/1` 显示“暂无 Rating 记录”。
  - 临时将所有用户置为 `disabled`（验收后恢复 `active`）验证三榜空态：Rating/活跃度/共享分别显示“暂无 Rating 排行”“暂无活跃度排行”“暂无共享排行”。
  - 首页排行榜快捷入口与 M4-C 系统状态正确；侧栏版本 M4-C、导航“排行榜”可点击。
  - 375px（headless Chrome/CDP 视口模拟，in-app browser 固定 816px 无法覆盖）：页面宽度 375 无横向溢出，`.table-scroll` 287/480 可横向滚动，滚动后等级列完整可见；颜色与装备展示正常。
  - 控制台无业务错误，仅既有 `favicon.ico` 404；无 CDN/外部请求。
- 测试夹具：临时分页用户 `m4cp01`～`m4cp14`、临时私密比赛与 `rating_history` 行已在验收后删除；一次性签到用户 `m4ce2e215243` 保留（金币 5、活跃度 5），用于后续签到/榜单排查。

### 2026-11-07 / UI-31

- `corepack pnpm --filter web test`：69 个测试文件 / 441 个测试通过。新增/更新 `LearningPathListView`、`LearningPathDetailView`、`LearningPathEditView`、`AdminLearningPathsView`、`LearningNodeDialog`、`utils/learning`、`validation`（`validateCoverUrl`）与 `AppNav`/`AdminNav` 断言。
- `corepack pnpm --filter web lint`：ESLint 与 Prettier 通过。
- `corepack pnpm --filter web build`：通过；仅有既有的 >500 kB chunk 提示（Monaco/编辑器相关），无新增错误。
- Element Plus 残留检查：四个学习路线视图、`LearningNodeDialog` 与全部 spec 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 保持 7 个文件不变（UI-19/UI-20/`main.ts`）。
- 真实浏览器 E2E（Playwright + 本机 Chrome，见 `apps/web/e2e/learning-paths.spec.ts`）：官方/自定义路线列表与卡片、树详情（`TreeTable` 状态 Tag/进度条、游客根 active/其余 locked）、编辑页节点树（添加/编辑/上移/下移/删除叶子、题目/`requiredCount` 校验）、官方管理页（下架/恢复）、私有/公开可见性、完成锁存、`myCompletedCount` 更新与 375px 窄屏横向滚动/对话框可操作，控制台无错误。

### 2026-11-08 / UI-35

- `corepack pnpm --filter web test`：71 个测试文件 / 464 个测试通过。新增/更新 `ProblemImportView`、`ProblemListView`（导入入口）、`ProblemDetailView`（创建者链接）与 `api/problems` 断言。
- `corepack pnpm --filter web lint`：ESLint 与 Prettier 通过。
- `corepack pnpm --filter web build`：通过；仅有既有的 >500 kB chunk 提示（Monaco/编辑器相关），无新增错误。
- Element Plus 残留检查：`ProblemImportView.vue` 与 spec 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 保持 7 个文件不变（UI-19/UI-20/`main.ts`）。
- 真实导入冒烟（后端 `127.0.0.1:18080` + 前端 5173 代理 18080）：API 上传根目录 `Export.zip` 返回 `imported=[hydro-6760 求和 / 循环、DAI / 10]`、`failed=[]`，重复上传进入 `failed`（题目标识已存在）不覆盖；数据库确认 `is_public=false`、`created_by`=上传者、`source=HydroOJ`、10 组测试点 `score=10`/`sort_order` 1..10、Markdown 题面；`GET /problems/:id` 游客/管理员返回 `author{id,username,nickname}`。真实浏览器（in-app browser，816px 视口）：`m2badmin` 登录 → `/problems/import` 页头/文件选择（`.zip` accept、200MB 提示）/「开始导入」禁用态 → 选择 `Export.zip` 后按钮启用 → 上传显示「导入成功 1 道」结果表（求和/hydro-6760/10/编辑链接）→ 详情页「创建者 m2badmin」链接 `/users/1`，控制台无错误。375px 窄屏视觉检查因 in-app browser 固定 816px 视口延后至最终评审（前端单测已覆盖移动端布局规则）。

### 2026-11-08 / UI-32

- `corepack pnpm --filter web test`：74 个测试文件 / 493 个测试通过。新增/更新 `ProblemSolutionsView`、`ProblemSolutionDetailView`、`UserProfileView`、`NotificationBell`、`NotificationsView`、`FollowListDialog` 与 `api/social` 断言（点赞按钮/计数/乐观回滚、游客跳登录、关注/粉丝 Dialog、铃铛未读/轮询/卸载、通知页已读/全部已读/跳转/空态）。
- `corepack pnpm --filter web lint`：ESLint 与 Prettier 通过。
- `corepack pnpm --filter web build`：通过；仅有既有的 >500 kB chunk 提示（Monaco/编辑器相关），无新增错误。
- Element Plus 残留检查：新增/修改的社交视图、`NotificationBell`、`FollowListDialog` 与全部 spec 无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 保持 7 个文件不变（UI-19/UI-20/`main.ts`）。
- 真实浏览器 E2E 因会话无桌面浏览器连接而延后（in-app browser 未连接）；后端真实 API 冒烟（`127.0.0.1:18080`）已覆盖点赞/关注/通知三类交互、未读/已读、权限与清理，前端行为由上述单测覆盖。

### 2026-11-08 / UI-34

- `corepack pnpm --filter web test`：全量测试通过（新增 `CodeEditor` 光标字节偏移/插入、`SubmitView` AI 工具栏/预览/错误映射、`ProblemDetailView` AI 提示、`AiSettingsView` 配置/校验、`UserMenu`「AI 设置」入口断言）。
- `corepack pnpm --filter web lint`：ESLint 与 Prettier 通过。
- `corepack pnpm --filter web build`：通过；仅有既有的 >500 kB chunk 提示（Monaco/编辑器相关），无新增错误。
- Element Plus 残留检查：`AiSettingsView.vue` 与 `SubmitView`/`ProblemDetailView` 新增 AI 区域无 `element-plus`、`<el-`、`ElMessage`、`v-loading`；legacy allowlist 保持 7 个文件不变（UI-19/UI-20/`main.ts`）。
- 真实 fake-provider 冒烟（临时 server `127.0.0.1:18081` + 假 OpenAI 服务 `127.0.0.1:18082`，临时把 `deepseek.base_url` 指向假服务）：补全/格式化/优化/提示首次生成/缓存命中/未配置 40001/限流 42903/隐藏题 404 均通过 API 验证；浏览器验收因会话无桌面浏览器连接而延后（前端单测已覆盖工具栏/预览/配置页/提示与移动端布局规则）。

## 已知问题与风险

1. **亮色主题首屏闪烁**：`index.html` 默认 `app-dark`，已保存亮色偏好的用户在 JS 执行前可能短暂看到暗色。可后续在 `index.html` 增加极小的启动脚本，或接受该闪烁。
2. **PrimeVue locale**：已在 `src/theme/primevue-locale.ts` 补局部中文（DatePicker、Select 空态等）；未覆盖的组件默认文案仍可能为英文，首次使用需确认或显式传中文。
3. **包体增长**：PrimeVue 主题与 primeicons 增加首屏 CSS/字体体积；Monaco 的既有大 chunk 警告仍在，非本次引入。
4. **未迁移页面视觉仍为浅色卡片**：暗色 AppShell 已上线，但 Element Plus 卡片仍是浅色内容面板。UI-02 起逐页替换；异常页文字已做临时兼容覆盖。
5. **浏览器验收限制**：in-app browser 窗口固定为 816px 宽，375px 通过 headless Chrome/CDP 覆盖；桌面 1200px 与手机 390px 布局也通过同源 iframe 验证结构与媒体查询，未做真实设备截图。
6. **开发期 HMR 警告**：编辑 `DefaultLayout.vue` 时 Vite 可能打印一次 `Cannot access 'DefaultLayout' before initialization` 并提示 HMR 失败；完整刷新后页面正常，生产构建与测试不受影响。根因是根布局被 `router/index.ts` 静态导入；如后续要消除，可评估将根布局改为动态 import（会改变首屏加载方式）。
7. **客户端校验需同步服务端**：`validation.ts` 是客户端规则的唯一来源，已用 `validation.spec.ts` 覆盖主要边界；服务端规则变更时必须同步更新，服务端仍是最终权威。
8. **Vite 依赖预构建 504**：首次访问新迁移且引入新 PrimeVue 组件的页面时，开发服务器可能返回 `Outdated Optimize Dep` / 动态模块加载失败；刷新一次后恢复，生产构建不受影响。
9. **md-editor 异步清理偶发告警**：全量测试中 `sanitize.markdown.spec.ts` 偶尔在环境销毁后收到 md-editor-v3 的异步定时器错误；重跑通过，不影响断言结果。后续可在该 spec 中增加延迟卸载或清理定时器。

## 下一个建议任务

**UI-19 — ProblemEditView + TestcaseManager + SampleEditor 题库管理**

- 迁移 `ProblemEditView.vue`、`TestcaseManager.vue`、`SampleEditor.vue` 与对应 spec 到 PrimeVue。
- 保留题目字段、MarkdownEditor、样例增删、测试点上传/排序/删除/下载、输入输出校验与保存跳转。
- 使用 `AppPageHeader`、`AppSurface`、`DataTable`、`Dialog`、`InputNumber`、`FileUpload` 或原生文件输入与共享状态组件；不改变上传表单字段与 API。
- 测试点操作期间保持 loading/禁用与错误提示；390px 下表格容器内滚动、亮/暗主题正常。
- 完成后从 allowlist 移除 4 个文件并更新验证记录。
