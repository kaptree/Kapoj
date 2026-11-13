# 个人主页 · Hero 词云设计

> 日期：2026-11-13
> 范围：`/users/:id` 个人主页查看态
> 关联：`apps/web/src/views/UserProfileView.vue`、`apps/web/src/components/profile/ProfileEvaluationCard.vue`

## 1. 背景与目标

当前个人主页把“词云评价”和“别人的评价列表”一起放在右侧栏。词云是规整的 flex 换行，评价列表还会展示评论人、时间、分页和删除操作。用户希望：

- 把词云提到姓名/@用户名附近、个人简介上方（`USER // PROFILE` 区域）。
- 不再列出别人的评价记录。
- 词云本身是不规则排列的。
- 点击词云面板，弹出一个评价输入框。

目标：让词云成为个人主页的身份视觉焦点，而不是侧栏里的一个普通卡片；评价输入从“常驻表单”变成“点击词云后的一次轻量交互”。

## 2. 非目标

- 不改后端评价 API、聚合规则（最多 24 项）、权限或 40 字限制。
- 不改个人主页的贡献图、Rating 历史、荣誉证书、关注/粉丝逻辑。
- 不做评价点赞、回复、举报、审核队列。
- 不展示别人的评价明细列表；不做“加载更多”。
- 不做词云词条的单独点击/筛选。

## 3. 已确认决策

1. 采用 **A Hero 词云**：顶部左侧头像 + `USER // PROFILE` + 昵称 + `@用户名`，右侧是不规则词云；简介、徽章、关注/统计在下方。
2. 词云面板整体可点击；点击后弹出评价 Dialog。
3. 评价输入框不再常驻右侧栏；右侧栏只保留荣誉证书。
4. 别人的评价列表、分页、删除他人评价、评论人头像/时间全部移除。
5. 当前用户自己的评价仍可创建、更新、删除；在 Dialog 内完成。
6. 词云用确定性不规则布局：字号按出现次数、颜色按文本稳定哈希、轻微旋转、螺旋散点排布、避免重叠；同一数据每次渲染位置稳定。
7. 不新增后端字段/接口；继续使用 `GET /users/:id/evaluations` 的 `cloud`、`total`、`myEvaluation`、`canEvaluate`。

## 4. 页面结构

```text
┌ USER // PROFILE ─────────────────────────────────────────────┐
│ [头像]  kaptree                        评价词云（不规则散点）  │
│         @kaptree                       12 条评价 · 点击写评价  │
└───────────────────────────────────────────────────────────────┘
┌ 个人资料 ────────────────────────────────────────────────────┐
│ [徽章/称号] 个人简介……                                        │
│ 粉丝 0   关注 0                              [关注]           │
│ [提交/通过] [Rating] [金币] [注册时间]                        │
└───────────────────────────────────────────────────────────────┘
┌ 代码提交贡献图 ───────────────────────┐  ┌ 荣誉证书 ───────┐
└───────────────────────────────────────┘  └─────────────────┘
┌ Rating 历史 ──────────────────────────┐
└───────────────────────────────────────┘
```

- 顶部 Hero 替换现有 `AppPageHeader` + `profile-head` 的重复身份区；昵称/@用户名只出现一次。
- 头像在 Hero 左侧；词云在 Hero 右侧，宽度约 52–58%，高度 200–240px。
- 简介、徽章、关注/粉丝、统计保留在 Hero 下方的 `AppSurface` 中。
- 右侧栏只保留 `ProfileCertificatesCard`；评价卡从页面移除。
- 移动端：Hero 纵向排列，头像/昵称在上，词云在下、简介再在下；词云高度约 160–180px。

## 5. 词云交互

### 5.1 面板

- 词云面板是一个可点击区域（`cursor:pointer`），右上角/底部有轻量提示：`点击词云，写一句评价` + 编辑图标。
- 面板同时显示 `N 条评价` 小徽标；`N` 来自 `total`。
- 鼠标悬停：面板边框/光晕增强；键盘可达的“写评价”按钮位于面板右上角，`aria-label="写评价"`。
- 点击面板任意空白/词条：打开评价 Dialog；点击右上角按钮同样打开。
- 空态：面板中央显示“还没有评价 / 点击写下第一条评价”，仍可点击打开 Dialog。
- 加载态：骨架/“正在加载评价”。
- 错误态：面板内显示“评价加载失败”+“重试”按钮；重试只重新拉评价，不影响主页其它数据。
- 游客/本人/不可评价用户：面板仍可点击，Dialog 内显示对应提示，不弹输入框。

### 5.2 词条

- 每个词条显示评价原文；长文本最多显示约 22 个字符，超出省略，`title` 保留全文。
- 字号：`0.85rem + min(count - 1, 4) * 0.18rem`，再按文本长度和可用空间缩放。
- 颜色：沿用现有 `primary/success/warning/special/re` 五色稳定哈希，或改用更克制的 primary/info/success/special；实现时以现有 token 为准。
- 旋转：-8°–8°，由文本哈希确定性生成；部分词条不旋转，避免所有词都歪。
- 词条不可单独点击；`pointer-events:none` 让点击落到面板。
- 无障碍：词条文本保留在 DOM 中；面板提供 `aria-label="评价词云，共 N 条评价"`，并有一个真正的“写评价”按钮。

## 6. 词云布局算法

新增纯函数 `apps/web/src/utils/profileWordCloudLayout.ts`：

```ts
export interface ProfileWordCloudItem {
  text: string
  count: number
}

export interface ProfileWordCloudPlacement {
  text: string
  count: number
  x: number
  y: number
  width: number
  height: number
  fontSize: number
  rotation: number
  colorIndex: number
}

export interface ProfileWordCloudLayout {
  placements: ProfileWordCloudPlacement[]
  overflow: number
}

export function buildProfileWordCloudLayout(
  items: ProfileWordCloudItem[],
  width: number,
  height: number,
  options?: { maxWords?: number; gap?: number; maxFontSize?: number },
): ProfileWordCloudLayout
```

规则：

1. 按 `count` 降序、`text` 升序排序；最多取 `maxWords`（默认 24，移动端 16–18）。
2. 估算文本宽度：CJK 字符约 1em，ASCII 约 0.58em；单条最大宽度限制为容器宽度的 45%（桌面）/ 60%（移动端），超出用省略号展示。
3. 字号先按出现次数计算，再按文本长度缩小；最小 0.75rem。
4. 从容器中心开始螺旋放置：角度步进约 0.35rad，半径逐步增大；用轴对齐矩形碰撞检测（词条盒子 + gap 4–6px），旋转只影响视觉、不参与碰撞。
5. 所有坐标限制在容器内；放不下的词条计入 `overflow`，面板角落显示 `+N`，不破坏布局。
6. 使用文本哈希 + 固定种子生成旋转/微小偏移；相同输入必须得到相同布局。
7. 容器尺寸变化时用 `ResizeObserver` 重新计算；测试环境无尺寸时使用 640×220 的默认值。

## 7. 评价 Dialog

新增 `apps/web/src/components/profile/ProfileEvaluationDialog.vue`：

- Props：`visible`、`targetUserId`、`targetActive`、`myEvaluation`、`canEvaluate`、`isSelf`、`isLoggedIn`。
- Emits：`update:visible`、`saved`、`deleted`。
- 使用 PrimeVue `Dialog`，`modal`，宽度 `min(92vw, 30rem)`；标题“写评价”/“更新评价”。
- 内容：
  - 可评价：`Textarea`（40 字上限，自动聚焦，禁止换行/制表符，提交前归一化为空格）、计数器、提示“用一句话评价 TA（40 字以内）”。
  - 已有评价：预填内容；底部“删除我的评价”文字按钮（二次确认），提交按钮文案“更新评价”。
  - 游客：提示“登录后可以给 TA 写评价”+ 登录按钮。
  - 本人：提示“这是你的主页，不能给自己评价”。
  - 目标用户不可评价：提示“该用户当前不可评价”。
- 提交：`saveMyEvaluation` → toast → 关闭 → emit `saved`；删除：`deleteMyEvaluation` → toast → 关闭 → emit `deleted`。
- 失败：Dialog 内展示错误信息，不关闭，保留输入。
- 关闭后重新打开：若 `myEvaluation` 已更新，表单以最新值预填；取消不保存。

## 8. 数据流

`UserProfileView` 统一加载评价数据，避免词云和 Dialog 重复请求：

```text
loadProfile(id)
  ├─ getProfile(id)
  ├─ getUserSocial(id)
  ├─ getUserActivity(id)
  └─ listUserEvaluations(id, 1, 1)   // 只要 cloud/total/myEvaluation/canEvaluate
```

- `pageSize=1` 只是为了减少 `items` 负载；`cloud` 是聚合结果，与分页无关。
- 评价加载失败不阻塞主页：`evaluationError` 只影响词云/Dialog 的错误态。
- `saved`/`deleted` 后重新调用 `loadEvaluations`，词云与 Dialog 同步更新。
- `ProfileWordCloud` 只接收展示数据，不发请求。
- `ProfileEvaluationDialog` 负责表单状态与保存/删除请求。

## 9. 移除内容

- 删除 `ProfileEvaluationCard.vue` 及其 spec（或将其替换为 `ProfileEvaluationDialog.vue` + `ProfileWordCloud.vue`）。
- 移除评价列表、头像、时间、分页、`loadMore`、`deleteUserEvaluation` 调用。
- 移除右侧栏评价卡；`profile-aside` 只保留证书卡。
- 移除 `formatDateTime` 在评价卡中的使用。

## 10. 测试

### 10.1 布局纯函数

- 24 条短文本在 640×220 内全部放置，无重叠。
- 相同输入生成相同位置；不同输入/种子不同。
- 长文本宽度被限制、字号缩小。
- 容器很小时返回 `overflow > 0`，不产生越界坐标。
- 旋转角度在 -8°–8°。
- 空数组返回空 placements。

### 10.2 组件

`ProfileWordCloud.spec.ts`：
- 渲染词条、总数、点击面板 emit `open`。
- 空态/加载/错误/重试。
- 长文本省略与 `title`。
- 右上角“写评价”按钮可点击。
- `overflow` 显示 `+N`。

`ProfileEvaluationDialog.spec.ts`：
- 游客、本人、不可评价三种提示。
- 可评价时输入、计数器、提交调用 `saveMyEvaluation`，成功后关闭并 emit `saved`。
- 已有评价预填、更新文案、删除确认、`deleteMyEvaluation`、emit `deleted`。
- 失败保留输入并展示错误。
- 换行/制表符归一化为空格，40 字限制。

`UserProfileView.spec.ts`：
- Hero 中词云在简介之前（DOM 顺序）。
- 页面只请求一次评价数据；词云收到 cloud/total。
- 点击词云打开 Dialog；保存后重新加载评价。
- 右侧栏只有证书卡，不再有评价列表。
- 评价加载失败不影响主页其他内容。

### 10.3 真实浏览器

- 桌面：`/users/33` 词云在姓名右侧、简介上方；点击面板打开 Dialog；无评论列表。
- 移动端 375px：Hero 纵向排列，词云不横向溢出，Dialog 可用。
- 游客/本人/登录用户三种 Dialog 状态。

## 11. 风险与取舍

- **词云密度**：24 条长评价在小屏可能拥挤；用最大宽度、字号缩放、`overflow +N` 兜底。
- **可点击面板**：整块可点击会牺牲词条单独交互；本次不需要词条点击，换取更大的点击区域。
- **移除评价列表**：主页主人不能再删除别人的评价；这是用户明确要求“不列出别人评论记录”的取舍。自己的评价仍可删除。
- **Hero 重排**：昵称/@用户名从 `AppPageHeader` 移到 Hero，避免重复；需要同步更新页面测试。
- **无后端改动**：评价聚合和权限保持现状；如果以后需要“只返回 cloud”的轻量接口，可另开任务。

## 12. 涉及文件

```text
apps/web/src/views/UserProfileView.vue
apps/web/src/views/UserProfileView.spec.ts
apps/web/src/components/profile/ProfileWordCloud.vue
apps/web/src/components/profile/ProfileWordCloud.spec.ts
apps/web/src/components/profile/ProfileEvaluationDialog.vue
apps/web/src/components/profile/ProfileEvaluationDialog.spec.ts
apps/web/src/utils/profileWordCloudLayout.ts
apps/web/src/utils/profileWordCloudLayout.spec.ts
apps/web/src/components/profile/ProfileEvaluationCard.vue        (remove)
apps/web/src/components/profile/ProfileEvaluationCard.spec.ts    (remove)
```

## 13. 批准记录

- 2026-11-13：用户选择 A Hero 词云；确认点击词云面板弹出评价输入框；确认不列出别人的评论记录。
