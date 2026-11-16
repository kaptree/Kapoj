# KapOJ 二次元风格与伙伴系统设计

- 日期：2026-11-16
- 状态：已批准（Phase 0–1）
- 方案：角色化设计语言（B）+ 系统级伙伴（C）+ 毒舌傲娇人格
- 首个实施范围：Phase 0–1（视觉骨架 + 角色接入，不含后端好感度）
- 代号：Kapo（小 K，最终名字待定）

## 1. 背景与目标

KapOJ 目前的视觉是深色优先的青绿科技风：`apps/web/src/styles/tokens.css` 定义 token，`apps/web/src/theme/preset.ts` 定义 PrimeVue 主题，页面以数据密集的控制台布局为主。它可靠、克制，但对年轻用户缺少记忆点。

这次要把平台从"工程控制台"推向"有角色的学习社区"，并在后续接入用户生成的 KapOJ 二次元形象。目标：

1. 让新用户第一眼感到"这是一个有性格的 OJ"，而不是通用后台。
2. 让角色承担情绪反馈和陪伴，但不干扰刷题、比赛和评测信息的可读性。
3. 建立一套可持续扩展的角色资产、文案、好感度和换装体系，后续形象图可以直接按规格交付。
4. 暗色/亮色、桌面/移动、无障碍、性能都要成立，而不是只做一张首页海报。

## 2. 非目标与反目标

非目标：

- 不重写路由、数据流或评测逻辑。
- 不做抽卡、盲盒、付费货币或任何随机付费机制。
- 不替换现有图标体系为 emoji；角色负责性格，图标负责识别。
- 不把题库、提交记录、评测详情改成漫画分镜；数据区域保持安静。
- 不引入外部统计、追踪或第三方角色素材。

反目标（即使"技术上做出来了"也算失败）：

- 用户在比赛中被弹窗、台词或动效打断。
- 红色系品牌色与 WA/错误色混淆，导致误操作。
- 角色形象被性化、低龄化暗示或与既有 IP 雷同。
- 首屏因为立绘变慢、抖动或布局跳动。
- 用户找不到关闭角色的开关。

## 3. 约束

- 前端：Vue 3 + PrimeVue 4 + Pinia + Vite，组件测试 Vitest，E2E Playwright。
- 后端：Go，`apps/server/internal/...`，已有 site、gamification、activity、ai、contest 等模块。
- 主题：暗色/亮色双主题，token 与 PrimeVue preset 必须同步。
- 现有能力：每日签到 `/activity/signin`、任务/成就领取、徽章/称号装扮、AI 提示与代码操作、比赛模式。
- 资产：站点 logo/favicon 已有上传能力；角色包第一版随前端静态资源发布，后续再考虑管理端上传。
- 角色为白发、萝莉外形的全年龄角色。所有设计和素材必须非性化，禁止泳装、透视、暗示性姿势或任何擦边内容。
- 用户可能在不同设备登录，好感度和偏好需要服务端同步；访客使用本地降级。

## 4. 方案选择

考虑过三条路线：

- **A. 贴纸层**：不动现有视觉，只加角色贴纸和气泡。成本低，但解决不了"保守感"，角色也容易变成装饰。
- **B. 角色化设计语言（选中）**：保留深色底和信息可读性，重做品牌色、排版标签、卡片形状、空状态和反馈语言，让角色成为视觉系统的一部分。
- **C. 全站漫画化**：页面变成分镜和大幅立绘。冲击力强，但牺牲信息密度、比赛体验和性能。

选择 B。C 的伙伴系统能力保留，但表现层坚持"核心安静、边缘有性格"。

## 5. 角色设定

### 5.1 基本设定

- 代号：Kapo（小 K）。最终名字由用户定稿，代码中统一使用 `kapo` 作为资源 key。
- 外观：银白长发、绯红眼睛、黑色 oversized 连帽衫（胸口绯红 K 字）、银色发饰/绯红缎带、呆毛。全年龄、非性化。
- 身份：住在评测机里的"观测者"。她声称自己只是负责记录提交，实际上会偷偷记住用户的连续练习、第一次 AC 和失败次数。
- 性格：毒舌傲娇。先吐槽，再帮忙；嘴上嫌弃，行动上护短。好感度越高，越容易露出关心的一面。
- 与系统的关系：青绿是"机器/评测"的颜色，绯红是"她"的颜色。她不是系统的吉祥物，而是系统里唯一有情绪的那个住户。

### 5.2 语气规则

- 吐槽对象是代码、题目、情况，不是用户本人。
- 不攻击外貌、性别、身份、水平；不说"你太菜了"这类贴标签的话。
- 每次最多 2 句，中文不超过 40 字；失败文案必须给下一步动作。
- 低好感：8 分嫌弃 + 2 分关心；高好感：4 分嫌弃 + 6 分关心。
- 比赛进行中完全静默，不吐槽、不庆祝。
- 用户关闭伙伴后，所有角色文案和立绘消失，不保留"幽灵提示"。

### 5.3 文案示例

| 场景 | 低好感（Lv0–2） | 高好感（Lv4–6） |
| --- | --- | --- |
| 首次访问 | "新面孔？……随便找个位置坐吧。" | "回来了。……我才没有在等你。" |
| 每日签到 | "签到了。今天也别让我记太多 WA。" | "签到了。……嗯，今天也一起。" |
| AC | "过了。下一题，别得意。" | "……不错。我说了不错，别让我重复。" |
| WA | "第 3 个点。别看我，看代码。" | "第 3 个点。……我陪你再走一遍。" |
| CE | "编译器都看不下去了。" | "先让它编译通过。……我等你。" |
| TLE | "跑得比我还慢。" | "超时了。……优化一下，你可以的。" |
| 空提交 | "还没有记录。去写第一道。" | "空的。……我陪你写第一道。" |
| 404 | "这里什么都没有。……跟着我。" | "又迷路了？……算了，跟我来。" |
| 连续 7 天 | "七天。……勉强夸你一下。" | "七天了。……我记着呢。" |
| 摸摸头 | "别、别随便摸。" | "……就一下。" |

## 6. 视觉语言

### 6.1 色彩

原则：**青绿负责系统与交互，绯红负责角色与品牌情绪，语义色不参与品牌化。**

新增角色 token（暗色）：

```css
--kapoj-kapo: #e23a4e;          /* 绯红主色，装饰/品牌 */
--kapoj-kapo-strong: #ff5c70;   /* 暗色下的高对比强调 */
--kapoj-kapo-text: #ff7a88;     /* 暗色正文可用，AA */
--kapoj-kapo-soft: rgb(226 58 78 / 12%);
--kapoj-silver: #c9d1dc;        /* 银白 */
--kapoj-silver-strong: #eef2f7;
--kapoj-ink: #0b0b10;           /* 墨黑 */
--kapoj-manga-line: rgb(238 242 247 / 18%);
--kapoj-manga-dot: rgb(226 58 78 / 14%);
--kapoj-sticker-shadow: 4px 4px 0 rgb(0 0 0 / 45%);
```

亮色覆盖：

```css
--kapoj-kapo: #c81e36;
--kapoj-kapo-strong: #a8142a;
--kapoj-kapo-text: #a8142a;
--kapoj-kapo-soft: rgb(200 30 54 / 8%);
--kapoj-silver: #5e6f86;
--kapoj-silver-strong: #172b4d;
--kapoj-ink: #1a1a22;
--kapoj-manga-line: rgb(23 43 77 / 12%);
--kapoj-manga-dot: rgb(200 30 54 / 8%);
--kapoj-sticker-shadow: 4px 4px 0 rgb(23 43 77 / 14%);
```

使用规则：

- `--kapoj-primary` 保持青绿，继续负责按钮、链接、焦点环、选中态；不把全站主色改成红色，避免与 WA/错误色混淆。
- 绯红只用于：品牌标记、页面编号/漫画标签、角色相关表面、角色 CTA、侧边栏 active 指示条、角色气泡描边。
- 语义色不变：AC 绿、WA 红、TLE 黄、MLE 紫、RE 粉、SE 深红。危险操作继续用 `--kapoj-danger`，并始终带图标和文字。
- 绯红文本在暗色用 `--kapoj-kapo-text`，在亮色用 `#a8142a`，保证 AA。
- 大面积为绯红的表面只允许出现在角色 dock、首页角色卡、活动页；表格和表单不用绯红底。

### 6.2 字体与排版

- 正文、表单、数据继续用现有 `--kapoj-font-sans`；代码继续用 `--kapoj-font-mono`。不引入新的中日文 webfont，避免首屏体积。
- 标题增加"漫画标签"排版：编号 + 大写英文 + 片假名装饰 + 细线。

```text
┌ 02 / LEARNING OVERVIEW  学習記録 ─────────────────────
```

- 英文/片假名只做装饰，中文仍是唯一信息载体；`aria-label` 不读装饰文本。
- 标题字重 700–800，字距略收紧；正文行高不变。
- 漫画标签只用于内容页（首页、成就、任务、个人主页、学习路线、装扮、商店、登录/404）；管理后台、表格页和表单页保持现有 `AppPageHeader`，不做全站改造。

### 6.3 形状与材质

- **贴纸卡**：1px 银灰描边 + 4px 硬阴影；hover 时上移 1px、阴影变 5px。只用于角色卡、首页主卡、成就卡，不用于表格行。
- **漫画标签**：左侧 3px 绯红竖条 + 编号 + 大写英文；用于 section 标题。
- **对话气泡**：银白气泡 + 1px 描边 + 小三角，气泡尾巴指向角色；文字用正文色。
- **网点纹理**：`radial-gradient` 5%–8% 透明度，只出现在 hero、空状态、dock 背景；绝不铺在代码块、表格、表单后面。
- **切角**：仅首页 hero 和活动卡使用 10px 切角；普通卡片保持 10–12px 圆角，避免全站尖角化。
- **图标**：继续用 PrimeIcons；角色相关动作可加小徽记，但不把 emoji 当图标。

### 6.4 动效

- 角色 idle：呼吸 4s、眨眼 6–9s 随机；只动 `transform`/`opacity`，不用 layout 属性。
- dock 展开：180ms ease-out；气泡出现：120ms 上浮 + 淡入。
- 评测反应：toast 200ms 进入，2.5s 后自动退出；不阻塞提交操作。
- 所有动效尊重 `prefers-reduced-motion`；用户设置"减少动效"时只保留淡入淡出。
- 页面隐藏时暂停 idle 动画；比赛模式全部暂停。

### 6.5 暗色/亮色

- 暗色是默认和主设计目标：银白文字、墨黑表面、绯红点缀。
- 亮色不是简单反色：角色卡使用暖白纸感表面，绯红加深，网点透明度降到 8%，阴影从硬阴影改为柔和投影。
- 两套主题都要通过对比度检查；新增 token 在 `tokens.css` 与 `preset.ts` 同步。

### 6.6 禁止事项

- 不用绯红做错误/危险色。
- 不在表格、代码编辑器、评测详情背后铺网点或立绘。
- 不用自动播放语音、音效或全屏弹窗欢迎。
- 不用"抽卡""盲盒""限时付费"等机制。
- 不使用性化姿势、透明衣物、泳装、内衣或暗示性镜头。

## 7. 布局与出现位

### 7.1 全局伙伴 dock

挂在 `DefaultLayout.vue`，而不是 `AppShell.vue`：管理后台使用 `AdminLayout`，不出现伙伴。**dock 只给登录用户**；访客不显示 dock，只在首页 hero、登录/注册和 404 看到静态立绘。比赛进行中 dock 整体隐藏，见 9.1。

Phase 0–1 收起态（没有后端好感度）：

```text
                                      ┌─────────┐
                                      │  (chibi) │  ← 56px 头像按钮
                                      │   ●      │     新台词点，无等级
                                      └─────────┘
```

Phase 0–1 展开态（右下，宽 320px）：

```text
┌──────────────────────────────┐
│  (半身立绘)   「又来了？…」    │
│                              │
│  今日还没签到。                │
│  [今日签到] [设置] [安静]      │
└──────────────────────────────┘
```

Phase 2 起，展开态中间增加好感条和等级（属于后续范围）：

```text
│  ███████░░░  Lv.3 搭档  120/170│
│  [今日签到] [伙伴档案] [安静]   │
```

行为：

- 首次访问每天最多主动说一句；之后只在事件触发时说话。
- 点击头像展开；`Esc` 收起；焦点进入面板后可键盘操作。
- 移动端：56px 头像，展开为底部抽屉，最大高度 60vh；不遮挡提交按钮。
- 层级低于 toast/dialog，高于页面内容；不与通知铃铛、用户菜单重叠。
- 关闭后完全移除；用户可在设置中重新开启。

Phase 0–1 的台词与反应全部由前端本地状态驱动：签到结果用现有 API 返回值，评测结果在提交/记录页读取现有数据；不新增事件表，不做跨设备同步。好感度、等级、皮肤、档案卡都留到 Phase 2+。

### 7.2 首页

保留现有 hero 文案和 CTA，把右侧平台状态改为"她的观测台"：

```text
┌──────────────────────────────────────────────────────────────┐
│ 早上好，@user                          KAPOJ // OBSERVATORY  │
│ 让每一次提交，都成为成长。                                     │
│ [开始刷题] [查看比赛]                                          │
│                                                              │
│              (半身立绘 K)      「又来了？…题不会自己变简单。」   │
│                        ┌ 平台状态 · 紧凑卡 ┐                  │
│                        │ 评测正常 / 已运行  │                  │
│                        └─────────────────┘                  │
└──────────────────────────────────────────────────────────────┘
```

- 状态信息不删除，缩成浮在角色旁的紧凑卡；移动端角色退到背景右侧 30% 透明，状态卡全宽。
- 首页以下的学习概览、快捷入口、成长入口只换 section 标题和卡片材质，不重排数据。
- 登录用户 hero 右侧显示半身；访客显示 chibi + 注册 CTA。
- 伙伴关闭时不回退旧版布局，右侧替换为抽象"观测信号"面板（同心圆 + K 字水印），保持新视觉语言和布局稳定；不维护双布局。

### 7.3 登录/注册（只放静态图）

- 登录、注册、忘记密码、重置密码、验证邮箱都用**静态立绘**：桌面端右侧全身/半身图 + 一句欢迎；移动端图退到表单上方或背景，不遮挡输入。
- 不使用 dock、不弹气泡、不做 hover 互动、不播放 idle 动画；就是一张图 + 一行字，保证首屏快、登录流程安静。
- 登录失败不嘲讽，只给一句"……再看一眼密码。我等你。"
- 注册成功："账号建好了。……别让我等太久。"
- 图片必须带固定宽高或 `aspect-ratio`，移动端断点隐藏装饰图也不造成布局跳动。

### 7.4 空 / 加载 / 错误状态

扩展现有组件，而不是新造一套：

- `AppEmptyState.vue`：新增可选 `mascot`（表情 key）和 `line`；默认映射：题库空 → `pout`，提交空 → `thinking`，通知空 → `sleepy`，成就空 → `smug`。
- `AppLoadingState.vue`：保留 spinner + skeleton，角色只做 24px chibi 旁注，不替代加载指示。
- `AppErrorState.vue`：网络错误用 `annoyed`，404 用 `lost`，权限不足用 `smug`。
- 所有角色图 `alt=""`，台词放在可见文本中，`role="status"` 只播报一次。

### 7.5 评测反馈

- 结果 toast 增加角色头像和一句反应；不新增阻塞式弹窗。
- 首次 AC、连续 7 天、Rating 变化才用较大反馈（仍是 toast/dialog，可关闭）。
- 比赛进行中完全静默；比赛结束后补一句总结。
- 失败文案必须带下一步：看测试点、检查边界、先编译、优化复杂度。

### 7.6 AI 助手

AI 人格第一版只做**表现层**，不改服务端提示词和缓存，避免破坏 `complete`/`format`/`optimize` 的返回契约：

- `ProblemDetailView` 的 AI 提示面板显示 chibi 头像，标题为"K 的提示"；`SubmitView` 的补全/格式化/优化按钮旁只加小头像和 loading 文案，返回内容仍是原来的代码或 JSON。
- 不新增 `persona` 请求字段，不改变提示词、缓存 key 和模型配置。
- Lv3 解锁的是 AI 面板的头像框/配色，不是 AI 功能本身；功能仍按现有权限和比赛禁用规则。
- AI 失败时她只说"……线路断了。不是你的错。"
- 后续如果要做真正的语气提示词，必须为 hint 单独建缓存 key，且只影响提示，不影响代码操作类接口；这属于 Phase 3 之后的独立设计。

### 7.7 个人主页与伙伴档案

- 保留现有 hero 右侧的评价词云，不替换、不搬走；词云是身份焦点，伙伴不能抢它。
- 在 hero 下方新增"伙伴档案"卡：好感等级、称号、一起的天数、解锁皮肤、最常触发的一句台词（系统统计，不让用户手选）。
- hero 内只放小型角色徽记，不挤压词云；Lv4 起在下方"伙伴档案"卡里显示半身像。是否公开展示由用户设置决定。
- 可见性默认公开，可在设置中改为仅自己可见；关闭伙伴或设为仅自己时，其他用户看不到档案卡和角色位。
- 不展示用户失败次数等敏感统计。

### 7.8 装扮与商店

- `AppearanceView` 新增"伙伴"区：启用开关、活跃度、减少动效、主页可见性。
- 皮肤分为等级解锁和金币直购两类；只做直购，不做随机、不做付费货币。
- 等级皮肤与 8.3 一致：Lv2「程序员套件」、Lv4「文化祭」、Lv5「雪夜」；Lv3 是头像框，Lv6 是专属称号 + 主页背景，不是皮肤。
- 「礼服」和「新年」属于活动皮肤，不进等级表；活动皮肤可以直购或活动赠送。
- 商店的皮肤价格由管理端配置，购买/装备走独立 API；Phase 4 才实现，第一版不建商店页面。
- 换装只影响 dock、首页、主页和档案卡；不影响评测图标和语义色。

### 7.9 全站页面出现位地图

"每个页面都有她"不等于每个页面都放立绘。按三档控制：

- **A 档 · 角色主场**：大幅静态立绘或半身像 + 台词，允许占据视觉焦点。
- **B 档 · 角色点缀**：页面顶部或内容区底部放一条 `CompanionNote`（28px chibi + 一行字 + 关闭按钮），不改变原有布局，不遮挡数据。
- **C 档 · 安静**：不常驻角色，只在空/加载/错误和关键成功反馈里出现 24–28px chibi；不逐个改造所有编辑页的成功 toast。

`CompanionNote` 规则：每个页面每次访问最多一条；用户关闭后当天不再出现；不自动轮播；比赛进行中全部隐藏；高度固定，不能造成 CLS。

实现上，B 档由 `CompanionPageNote` 挂在 `DefaultLayout` 的 `<RouterView />` 前，按路由名从 `COMPANION_PAGE_NOTES` 取台词统一渲染，避免逐页改模板；只有需要特殊位置的页面（如首页 hero、登录静态图、404）才单独放 `CompanionAvatar` / `CompanionNote`。

| 页面 | 档位 | 具体表现 |
| --- | --- | --- |
| 首页 | A | hero 半身像 + 问候；移动端背景化 |
| 登录 / 注册 / 忘记密码 / 重置密码 / 验证邮箱 | A（静态） | 右侧静态立绘 + 一句欢迎；无 dock、无交互 |
| 404 / 403 | A（静态） | `lost` / `smug` 立绘 + 指路台词 + 返回按钮 |
| 题库列表 | B | 题目列表上方 `CompanionNote`："今天的题……自己挑。" |
| 题目详情 | B | 题面卡片底部一行批注；不进入题面正文，不遮挡样例/代码 |
| 提交页 | B | 代码编辑器上方小徽记 + 提交成功/失败反应；不进入编辑器 |
| 官方题解 / 题解列表 / 题解详情 | B | 作者信息下方 chibi 批注；不打断阅读 |
| 比赛列表 | B | 赛程卡上方一条批注；进行中的比赛卡不显示 |
| 比赛详情 | B（进行中 C） | 赛前/赛后批注；进行中静默，见 9.1 |
| 提交记录 | B | 表格上方一条批注；结果变化用 toast 反应，不插入表格行 |
| 排行榜 | B | 榜单标题旁 chibi 徽记 + 一句；不遮挡前三名 |
| 学习路线列表 / 详情 | B | 路线卡旁小徽记；节点详情用空状态台词 |
| 任务 / 成就 | A–B | 页面顶部半身/胸像 + 领取反应；列表本身保持数据密度 |
| 徽章商店 / 称号商店 | B | 商品网格上方批注；试穿预览可用 chibi，不替换商品图 |
| 装扮管理 | A–B | "伙伴"设置区 + 当前形象预览；其他装扮区不变 |
| 通知中心 | B | 空状态 `sleepy`；有通知时只在顶部一条批注 |
| 用户主页 | B（Phase 3 起 A–B） | 保留词云；伙伴徽记在 hero，档案卡在下方 |
| 证书核验 | B | 核验成功盖一个 "K 已核验" 贴纸；失败用 `thinking` |
| 个人资料 / 账号安全 / AI 设置 | C | 保存成功/失败时 chibi 反应；不常驻 |
| 题目编辑 / 比赛编辑 / 学习路线编辑 / 测试数据生成 | C | 只在保存、生成、校验结果里出现；编辑区保持安静 |
| 管理后台全部页面 | 无 | 不出现伙伴 |

新增组件 `CompanionNote.vue`（纯展示）和 `CompanionPageNote.vue`（按路由取台词、处理关闭与静默）；页面不传配置时不渲染，保证渐进接入。

## 8. 好感度系统（Phase 2+，首个实施计划不包含）

本节定义后续版本的数据和规则；Phase 0–1 不建表、不做接口、不在 dock 显示好感条。

### 8.1 原则

- 只增不减，不制造"断签惩罚"焦虑。
- 只由服务端事件记账，客户端不能刷。
- 每日有上限，避免刷题量被游戏化绑架。
- 好感度是"她记得你"，不是战力数值；不提供评测加成。

### 8.2 获取规则

| 事件 | 好感 | 定义与上限 |
| --- | --- | --- |
| 每日签到 | +3 | 当日首次签到，每日一次 |
| 当日首次 AC | +2 | 当天第一次 AC，每日一次 |
| 每道题首次 AC | +1 | 该题历史首次 AC；每日最多 +3 |
| 领取任务 | +1 | 点击领取时加分，不是完成时；每日最多 +2 |
| 解锁成就 | +1 | 成就真正解锁时加分；每日最多 +2 |
| 参加比赛并提交 | +5 | 在该比赛内首次提交时加分，每场一次；不要求完赛或结算 |
| 摸摸头 | +1 | 每日一次 |
| 连续 7 / 30 / 100 天 | +5 / +10 / +20 | 每个里程碑一次 |

每日自然增长上限 15，包含比赛 +5；里程碑奖励不计入上限。按上表计算，单日最多可自然获得 3+2+3+2+2+1+5=18，触顶后不再增加。

事件 key 与幂等：

```text
signin:{date}                 # 每日签到
ac:daily:{date}               # 当日首次 AC，+2
ac:first:{problemId}          # 每题历史首次 AC，+1；另用当日计数器限 +3
task:{taskId}                 # 领取任务
achievement:{achievementId}   # 成就解锁
contest:{contestId}           # 比赛内首次提交
pet:{date}                    # 每日摸摸头
streak:{days}                 # 7 / 30 / 100 天里程碑
```

实现优先复用现有 `activity_logs` 的 `(user_id, action, ref_type, ref_id)` 去重模式；`ac:*` 需要从评测结果/首次 AC 记录中取事件，不能由前端上报。`contest:{id}` 用报名+提交组合判定，不使用不存在的"完赛"事件。

### 8.3 等级

| Lv | 称号 | 阈值 | 解锁 |
| --- | --- | --- | --- |
| 0 | 陌生 | 0 | 基础台词 |
| 1 | 认识 | 20 | dock 展开台词、日常吐槽 |
| 2 | 熟悉 | 50 | 「程序员套件」皮肤 |
| 3 | 搭档 | 100 | 表情头像框、AI 面板头像框（不含 AI 功能） |
| 4 | 在意 | 170 | 「文化祭」皮肤、主页 hero 半身位 |
| 5 | 傲娇 | 260 | 「雪夜」皮肤、摸摸头关心台词 |
| 6 | 只对你 | 380 | 专属称号「她的观测对象」、主页背景 |

「礼服」「新年」是活动皮肤，不占等级位；等级皮肤不售卖，活动皮肤可直购或赠送。

### 8.4 后端模型

新增 `apps/server/internal/companion`：

```go
type State struct {
    UserID        int64
    Bond          int
    Level         int
    DailyGain     int
    DailyDate     string
    EquippedSkin  string
    UnlockedSkins []string
    ActivityLevel string // quiet | normal | lively
    DockEnabled   bool
    PublicProfile bool
    LastPetDate   string
}

type Event struct {
    ID        int64
    UserID    int64
    Key       string // 幂等键，见 8.2
    Type      string // signin | ac | task | achievement | contest | pet | streak
    Bond      int
    Payload   string // JSON：题目、比赛、评级变化等，供前端选台词
    ReadAt    *time.Time
    CreatedAt time.Time
}
```

`companion_events` 同时承担"好感记账"和"待播放反应"两个职责。唯一索引 `(user_id, key)` 保证幂等；`read_at` 为空表示还没在 UI 播放过。

API：

- `GET /api/companion`：返回状态、今日获取明细、解锁列表、下一级阈值。
- `GET /api/companion/events?afterId=&unread=1`：返回当前用户尚未播放的事件，最多 20 条；没有事件时返回空数组。
- `POST /api/companion/events/read`：按 id 批量标记已读。
- `POST /api/companion/settings`：`activityLevel`、`dockEnabled`、`publicProfile`、`equippedSkin`。
- `POST /api/companion/pet`：每日一次，返回台词 key 和是否加分。
- `GET /api/admin/site/companion` / `PUT /api/admin/site/companion`：开关、默认活跃度、比赛静默、是否允许用户关闭。

事件投递模型（不依赖现有 SSE）：

- 现有 `/api/submissions/stream` 只广播 `{ submissionId }`，没有用户和 verdict，不能直接用于角色反应。第一版不改它。
- dock 在以下时机拉取未读事件：应用启动、页面重新可见、用户完成签到/提交/领取后、以及每 60 秒一次（仅页面可见时）。
- 事件播放后批量标记已读；多标签页用同一接口，重复播放可接受但不重复加好感（幂等键保证）。
- 后续如需实时，可增加用户级 `/api/companion/stream`，但不作为 Phase 2 的前置条件。

事件接入点：

- `activity` 签到成功后写 `signin:{date}`。
- 评测 AC 后写 `ac:daily:{date}` 和 `ac:first:{problemId}`；后者由服务端查历史首次 AC，不能由前端上报。
- 任务领取、成就解锁成功后写对应 key。
- 比赛内首次提交成功后写 `contest:{id}`。
- 数据库迁移同时提供 Postgres 与内存实现，沿用现有 repository 模式。

### 8.5 文案内容

- 台词放在前端 `apps/web/src/content/companion/lines.zh-CN.ts`，按 `event × level tier` 组织；服务端只存状态和事件，不存文案。
- 每类事件至少 3 条低好感、3 条高好感，避免重复。
- 台词 key 稳定，方便后续做多语言和活动替换。

## 9. 专注模式与用户偏好

### 9.1 比赛专注模式

不能只靠路由 meta，因为题目/提交页和普通练习共用路由。用 `useCompanionMute()` 统一判断：

- `contest-detail`：比赛状态为 `running` 时静默。
- `problem-detail` / `problem-submit`：存在 `?contestId=` 且该比赛状态为 `running` 时静默。
- 其他路由：不静默。
- 静默时 **dock 整体隐藏**，不缩成图标、不主动说话、不弹评测反应、暂停 idle；header 显示一个静态"专注模式"标识，不带角色台词。
- 比赛结束后恢复 dock，并在下一次事件拉取时补一条赛后总结（不弹窗）。
- 管理端 `contestMute` 只对参赛者生效，不搞全站静默；需要全站安静时管理员直接关闭伙伴。

### 9.2 用户设置

Phase 0–1 `AppearanceView` 新增（仅本地偏好）：

- 启用伙伴：开/关（默认开）。
- 活跃度：安静 / 标准 / 热闹。
  - 安静：只响应签到、AC、404，不主动说话。
  - 标准：默认，事件反馈 + 每日一句。
  - 热闹：额外空状态、加载、hover 台词。
- 减少动效：跟随系统 / 强制减少。
- 重置偏好。

Phase 2+ 再加：主页可见（公开 / 仅自己）、服务端同步、好感重置。

关闭伙伴的语义：

- 移除：dock、气泡、反应 toast、AI 面板头像、伙伴档案卡、个人主页角色位。
- 保留：绯红/银白/墨黑视觉语言、漫画标签、贴纸卡；这些是平台新品牌，不是"角色组件"。
- 首页、登录页和主页用抽象占位（观测信号/几何纹理）填充角色位，不回退旧布局，不维护双布局。
- 其他用户查看该用户主页时，同样看不到角色位和档案卡；偏好本身不公开。

Phase 0–1 偏好只存 `localStorage` key `kapoj.companion.prefs`；Phase 2 起改为服务端同步，本地作为离线兜底。

## 10. 组件与代码结构

Phase 0–1 需要：

```text
apps/web/src/components/companion/
  CompanionDock.vue          # 全局 dock（仅登录用户）
  CompanionAvatar.vue        # 立绘/表情渲染
  CompanionBubble.vue        # 气泡
  CompanionNote.vue          # B 档页面批注条（纯展示）
  CompanionPageNote.vue      # 按路由取台词、关闭与静默
  CompanionReaction.vue      # 签到/评测 toast 反应（本地）
  CompanionSettings.vue      # 设置区
apps/web/src/composables/
  useCompanion.ts            # 选台词、触发本地反应
  useCompanionMute.ts        # 比赛进行中判断（contestId + status）
  useReducedMotion.ts
apps/web/src/stores/companion.ts   # 本地偏好 + 当日已读台词
apps/web/src/content/companion/
  lines.zh-CN.ts
  manifest.ts
apps/web/src/styles/
  companion.css              # 仅角色相关样式，token 进 tokens.css
```

Phase 2+ 再加：

```text
apps/web/src/components/companion/CompanionBondMeter.vue
apps/web/src/components/companion/CompanionProfileCard.vue
apps/server/internal/companion/
  model.go service.go repository.go memory.go postgres.go
apps/server/internal/companionapi/
  handler.go handler_test.go
apps/web/src/api/companion.ts
```

改动点（Phase 0–1）：

- `DefaultLayout.vue`：挂 `<CompanionDock />`（仅登录用户）；管理后台不挂。
- `AppEmptyState.vue` / `AppLoadingState.vue` / `AppErrorState.vue`：可选 mascot/line props，默认不变。
- `HomeView.vue` / `HomeProfileCard.vue`：hero 角色位与伙伴摘要。
- `LoginView.vue` / `RegisterView.vue` / `ForgotPasswordView.vue` / `ResetPasswordView.vue` / `VerifyEmailView.vue`：静态立绘 + 一句欢迎，无交互。
- `NotFoundView.vue` / `ForbiddenView.vue`：`lost` / `smug` 静态立绘。
- `ProblemListView.vue` / `ProblemDetailView.vue` / `SubmitView.vue` / `ProblemSolutionsView.vue` / `ProblemSolutionDetailView.vue`：`CompanionNote` + AI 提示头像。
- `ContestListView.vue` / `ContestDetailView.vue` / `SubmissionsView.vue` / `LeaderboardView.vue` / `LearningPathListView.vue` / `LearningPathDetailView.vue`：`CompanionNote`。
- `AchievementsView.vue` / `TasksView.vue` / `BadgeShopView.vue` / `TitleShopView.vue` / `NotificationsView.vue` / `CertificateVerifyView.vue`：按 7.9 的 A/B 档接入。
- `ProfileEditView.vue` / `SecuritySettingsView.vue` / `AiSettingsView.vue` / 各类编辑页：C 档，只在成功/失败反馈出现。
- `AppearanceView.vue`：伙伴设置区（本地偏好）。
- `styles/tokens.css` + `theme/preset.ts`：新增 token，保持同步。

Phase 2+ 再改：`UserProfileView.vue`（伙伴档案与词云共存）、`AdminSiteView.vue`（伙伴设置）、`api/companion.ts` 与后端模块、`SubmissionsView.vue` 的事件驱动反应。

## 11. 角色资产交付规格

### 11.1 画风与角色板

- 日系赛璐璐平涂，干净线稿，2–3 级阴影；禁止厚涂写实。
- 银白长发（高光 `#eef2f7`，中间 `#c9d1dc`，阴影 `#8f9bab`），绯红眼睛（`#e23a4e`，暗部 `#a8142a`），黑色连帽衫（`#15151c`），绯红 K 字与缎带。
- 标志性剪影：呆毛 + 侧发 + 过大的连帽衫袖口。
- 全年龄、非性化；不采用泳装、内衣、透视、暗示性姿势、湿身、舔唇等表现。
- 不得与任何既有动漫/游戏角色高度相似；交付时确认原创。

### 11.2 表情差分（最低 11 个）

`neutral` `smug` `pout` `blush` `surprised` `annoyed` `thinking` `sleepy` `victory` `panic` `lost`

404 和错误路由使用 `lost`，不复用 `panic`。每个表情保持眼睛、发饰、服装一致，只改眉眼、嘴型和少量特效符号。

### 11.3 姿态与尺寸

| 用途 | 姿态 | 画布 | 导出 |
| --- | --- | --- | --- |
| dock 收起 / 空状态 | chibi 头像 | 512×512 | @1x、@2x WebP |
| dock 展开 / 首页 | 半身 | 1200×1400 | @1x、@2x WebP |
| 登录页 / 活动页 | 全身 | 1600×2400 | @1x、@2x WebP |
| 角色板 | 正面/侧面/背面 | 2048 高 | PNG 母版 |

- 透明背景，不烘焙投影和文字；阴影由 CSS 处理。
- 统一光源左上，线宽一致；导出 WebP quality 82，母版 PNG 另存。
- 命名：`/mascot/{pose}/{expression}@2x.webp`，例如 `/mascot/bust/pout@2x.webp`。
- 提供 `public/mascot/manifest.json`，列出姿态、表情、尺寸和版本号。
- 眨眼只做 chibi 和 bust 的 `neutral`：额外交付 `neutral-blink` 两帧或闭合眼层，CSS 每 6–9 秒切换一次；其他表情和全身像不做眨眼。
- 首屏只预加载 chibi neutral；其余按需加载。

**首发子集（控制体积）**：

- chibi：neutral、neutral-blink、smug、pout、annoyed、surprised、victory；
- bust：neutral、neutral-blink、smug、pout、blush；
- full：neutral。

其余表情和姿态在后续版本补齐；manifest 必须能表达"未交付"。

### 11.4 性能预算

- 首屏新增角色资源 ≤ 120KB；首发子集 ≤ 1.5MB；完整角色包后续单独评估。
- 新增 JS gzip ≤ 30KB。
- 图片必须带 `width`/`height` 或 `aspect-ratio`，避免 CLS。
- 不在 CSS 里内联 base64 大图。

## 12. 管理端配置（Phase 2+）

Phase 0–1 不加管理端配置，角色开关和活跃度走用户本地偏好；全局开关等 Phase 2 再做。

`AdminSiteView` 新增"伙伴设置"：

- 全局启用/关闭。
- 角色显示名（默认 Kapo）。
- 默认活跃度（安静/标准/热闹）。
- 比赛进行中让参赛者的伙伴静默（只影响参赛者，不搞全站静默）。
- 是否允许用户关闭。
- 角色资源包版本（只读，随前端发布）。
- 预留 `mascotPackUrl`，后续支持上传自定义包；第一版不实现上传。

写入现有 site 配置，字段：

```json
{
  "companion": {
    "enabled": true,
    "name": "Kapo",
    "defaultActivity": "normal",
    "contestMute": true,
    "allowUserDisable": true,
    "mascotPackUrl": ""
  }
}
```

## 13. 分期实施

**首个实施计划覆盖 Phase 0–1**（用户已确认）：交付视觉骨架、角色接入、dock、本地反应和本地偏好；不建后端、不做好感度、不做商店。Phase 2–4 作为后续计划，本文只定方向，不排期。

### Phase 0：视觉骨架（不需要立绘）

- 新增角色 token、漫画标签、贴纸卡、section 标题。
- `DefaultLayout` 加空 dock 占位；新增 `useCompanionMute`。
- 首页 hero 重排，为角色位留空。
- 暗/亮主题检查，更新主题测试。

### Phase 1：角色接入（用占位剪影）

- 资源 manifest、`CompanionAvatar`、`CompanionBubble`、`CompanionNote`、`CompanionDock`。
- 首页、登录/注册/找回密码/重置密码/验证邮箱、404/403、空/加载/错误状态接入。
- 按 7.9 的页面地图接入 B 档 `CompanionNote`：题库、题目、提交、题解、比赛、提交记录、排行榜、学习路线、任务/成就、商店、通知、证书核验。
- 登录/注册只放静态图，不挂 dock、不做交互。
- 访客无 dock；偏好本地存储。
- 签到/评测/404 的本地反应；不跨设备、不加好感。
- 先用占位剪影和 3 个表情，验证布局与性能。

### Phase 2：好感度与事件（后续计划）

- 后端 companion 模块、迁移、API、`companion_events` 事件箱。
- 签到/AC/任务/成就/比赛事件接入，幂等与每日上限。
- dock 好感条、摸摸头、签到与评测反应；事件轮询与已读。
- 设置区、服务端偏好同步、管理端配置。

### Phase 3：人格化 AI 与档案（后续计划）

- AI 面板表现层人格（不改提示词/缓存）。
- 个人主页伙伴档案、可见性。
- 等级解锁皮肤与表情包。

### Phase 4：换装与活动（后续计划）

- 装扮/商店直购皮肤、活动皮肤。
- 分享卡、周年/节日台词。
- 角色资源包上传（可选）。

每个 Phase 独立可上线；Phase 0–1 不依赖后端，Phase 2 起依赖迁移。

## 14. 验收标准

Phase 0–1（首个实施计划）：

- [ ] 暗色/亮色下，绯红与 WA 红可区分；危险按钮仍用 danger 色 + 图标 + 文案。
- [ ] 首页 LCP 相比改造前回退 < 100ms，CLS < 0.02。
- [ ] dock 只出现在默认布局，管理后台完全不出现；访客版只出现在首页/登录/注册/404。
- [ ] dock 可键盘打开/关闭，`Esc` 生效，焦点不丢失。
- [ ] 开启"减少动效"后无持续动画。
- [ ] 比赛进行中（contestId + running）dock 隐藏、无弹窗、无主动台词、无 idle 动画；普通练习题不静默。
- [ ] 用户关闭伙伴后，dock/气泡/头像/静态立绘/`CompanionNote` 全部消失；绯红视觉语言仍保留，布局不塌陷。
- [ ] 登录/注册/找回密码/重置密码/验证邮箱只有静态图，没有 dock、气泡或 hover 互动。
- [ ] 7.9 页面地图中的 A/B 档页面都有对应表现；C 档编辑页只在成功/失败反馈出现。
- [ ] `CompanionNote` 每页每次访问最多一条，关闭后当天不再出现，且不造成 CLS。
- [ ] 空/加载/错误状态在没有角色资源时仍正常显示；`lost` 表情缺失时回退 `neutral`。
- [ ] 签到、评测结果、404 的本地反应不依赖新增后端接口。
- [ ] 角色素材无性化内容，无既有 IP 相似风险。
- [ ] 组件单测、E2E 冒烟通过；不新增后端测试。

Phase 2+（后续验收）：

- [ ] 个人主页评价词云保留，伙伴档案不抢占词云位置。
- [ ] 好感度事件幂等：重复签到/重复 AC 不重复加分；每日上限 15 生效；比赛 +5 计入上限。
- [ ] 好感度与皮肤在换设备后一致；访客使用本地偏好。
- [ ] 后端单测、迁移和事件箱测试通过。

## 15. 风险与对策

| 风险 | 对策 |
| --- | --- |
| 二次元过度，老用户反感 | 核心页安静、比赛静默、可完全关闭、活跃度三档 |
| 绯红与错误色混淆 | 绯红只做装饰/品牌；语义色不变；危险操作带图标和文案 |
| 立绘拖慢首屏 | 首屏只加载 chibi，其余懒加载；预算与 CI 检查 |
| 好感度变成打卡压力 | 只增不减、每日上限、无评测加成、无断签惩罚 |
| 形象风格不统一 | 角色板 + 固定色板 + manifest + 交付验收 |
| 角色为低龄外形被性化 | 明确全年龄、非性化规范；素材验收一票否决 |
| 后端范围膨胀 | 首个计划只做 Phase 0–1；Phase 2 才加状态/事件/偏好，商店和上传后置 |

## 16. 已确认与待定

已确认：

- **首个实施计划范围：Phase 0–1**（视觉骨架 + 角色接入；不含后端好感度）。
- 伙伴关闭后保留新视觉语言，只移除角色元素。

待定（不阻塞 Phase 0–1）：

- 角色最终名字（先用 Kapo / 小 K 占位）。
- 伙伴档案默认公开还是仅自己可见（Phase 3 前定）。
- 控制台彩蛋是否保留（当前默认保留一条）。

后续版本默认值（可在对应阶段推翻）：

- 皮肤只用金币直购，不做付费货币和随机机制。
- Phase 2 好感度只增不减、每日上限 15、无评测加成。

## 17. 附录：新增 token 与台词结构

```ts
// content/companion/lines.zh-CN.ts
export const lines = {
  greeting: {
    low: ['新面孔？……随便找个位置坐吧。'],
    high: ['回来了。……我才没有在等你。'],
  },
  ac: {
    low: ['过了。下一题，别得意。'],
    high: ['……不错。我说了不错，别让我重复。'],
  },
  // ...
} as const
```

```ts
// composables/useCompanion.ts 伪代码
const { level, activityLevel, muted } = useCompanionStore()
const line = pickLine(event, tierOf(level), activityLevel)
if (!muted && line) companion.react({ event, line, expression })
```
