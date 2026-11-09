# 站点管理、首页内容与品牌配置设计

> 目标：把网站 Logo/favicon、首页欢迎区/快速开始/成长中心、页脚备案信息与运行/访问数据集中到后台管理；新增独立 `site.settings.manage` 权限，公开站点读取统一配置并在配置缺失时回退到当前硬编码内容。

## 1. 背景与现状

- 首页欢迎语、快速开始 4 张卡片、成长中心 4 张卡片、游客提示都硬编码在 `apps/web/src/views/HomeView.vue`。
- 品牌是 `AppBrand.vue` 的 CSS 方块 `K` + `kapOJ` 文字；顶部 header 另有一份文字品牌；认证页 `AuthLayout.vue` 再有一份；favicon 是静态 `public/favicon.svg`。
- 没有站点页脚，没有访问量统计，也没有通用站点设置表；`system.settings.manage` 是预留权限，尚未使用。
- 管理后台已有 `/admin/ai`、`/admin/rating` 等设置页模式：单例设置、`RequirePermission`、中文审计、PrimeVue 表单。
- 图片上传已有本地 storage、magic bytes 校验、公开读取与 `Cache-Control` 的成熟模式（头像、徽章、证书图片）。

## 2. 目标与非目标

### 目标

- 后台可上传/移除站点 Logo 与 favicon；公开站点、侧栏、顶部、认证页使用同一 Logo，favicon 动态替换浏览器标签图标。
- 后台可编辑首页：
  - 欢迎区 eyebrow、标题、标题高亮词、描述、主/次按钮；
  - 快速开始标题与卡片（图标/标题/描述/内部链接，可增删、上下移动）；
  - 成长中心标题与卡片（同上）；
  - 游客提示标题与描述。
- 后台可编辑页脚：
  - ICP 备案号、公安备案号与链接、版权文本、自定义说明文本；
  - 网站运行起始日（用于计算“已运行 N 天”）；
  - 是否显示访问量；
  - 只读展示当前累计访问量与运行天数。
- 页脚展示在公开站点所有页面；运行天数与访问量由服务端返回，避免客户端时钟/缓存不一致。
- 访问量按“每浏览器会话 +1”累计：同一标签页会话只计一次；新会话/新标签页计一次。
- 新增单一权限 `site.settings.manage`（分组「站点」），与现有 `system.settings.manage` 独立；默认仅 super，角色页可分配。
- 所有写操作写中文审计；公开配置读取无需登录。
- 配置缺失、接口失败、图片未上传时，前端回退到当前硬编码内容/静态 favicon/文字品牌，不出现空白站点。

### 非目标

- 不做全局主题色、背景、字体、圆角等 CSS 主题编辑器。
- 不做自由页面搭建器、自定义 HTML、脚本或外部 iframe。
- 不编辑首页右侧系统状态面板、登录后个人资料条、成长中心里“装扮管理”这个登录用户专属入口。
- 不统计登录用户数、PV/UV 报表、来源、地区或留存；只累计总访问量。
- 不做图片裁剪、多分辨率 Logo、深/浅色两套 Logo。
- 不改变现有首页路由、导航结构或认证逻辑。

### 反目标

- 不允许通过首页链接/页脚配置注入 `javascript:`、外链跳转或 HTML/脚本。
- 不允许 favicon/Logo 上传非图片、超大文件或带脚本的 SVG。
- 不允许没有 `site.settings.manage` 的用户读取或修改管理配置。
- 不允许公开首页因站点配置读取失败而崩溃或丢失现有文案。

## 3. 约束

- 后端 Go + Gin + GORM + PostgreSQL；迁移走 `db.Migrate`。
- 前端 Vue 3 + PrimeVue 4 + Pinia；API 类型由 `packages/api/openapi.yaml` 生成。
- 图片存储使用现有 `storage.Storage`；`KAPOJ_STORAGE_DIR` 必须与 judge 共享的目录一致。
- 现有权限目录为 37 个权限；新增第 38 个 `site.settings.manage`。Seed 每次启动只补齐权限目录，不自动给已有角色授权；super 天然绕过。
- 首页配置是结构化数据，不接受任意 HTML；所有文本按纯文本渲染，换行保留。
- 页脚运行天数以服务端 `startedAt` 计算，访问量以数据库计数器为准。

## 4. 方案选择

### 4.1 配置存储

- **方案 A：单例 `site_settings` + JSONB 首页内容（选用）**。品牌/页脚用普通列，首页用 `dbtypes.JSON[HomeContent]`。字段清晰、迁移简单，首页结构变更不需要加几十列。
- 方案 B：每个首页区域一张表/多行 key-value。查询与版本管理更复杂，收益不大。
- 方案 C：前端静态 JSON 文件 + 后台改文件。无法在容器/多实例部署下持久化，排除。

选择 A。

### 4.2 访问量

- **方案 A：前端 sessionStorage 去重 + 数据库原子计数（选用）**。同一标签页会话只 POST 一次；后端 `UPDATE count = count + 1 RETURNING count`。
- 方案 B：每次首页加载 +1。刷新会放大数字。
- 方案 C：Redis 每日 IP 去重。更接近 UV，但需要额外键管理和清理，用户已选择“每浏览器会话 +1”。

选择 A。

### 4.3 权限

- **方案 A：单一 `site.settings.manage`（选用）**。用户明确要求单独划分权限，但不要求品牌/首页/页脚各自独立；一个权限覆盖整个站点管理页。
- 方案 B：`site.branding.manage` / `site.homepage.manage` / `site.footer.manage` 三个权限。粒度更细，但当前只有一个站点管理页，增加授权复杂度。
- 方案 C：复用 `system.settings.manage`。不符合“单独划分权限”。

选择 A。

### 4.4 管理界面

- **方案 A：单个 `/admin/site` 页面 + 三个 Tab（品牌/首页/页脚）（选用）**。一个权限、一个入口、共用加载/错误/保存模式。
- 方案 B：三个独立管理页。导航更长，且当前规模不需要。

选择 A。

## 5. 设计

### 5.1 数据模型

`site_settings` 单例：

```text
id                  smallint primary key          -- 固定 1
logo_key            varchar(255) not null default ''
logo_content_type   varchar(100) not null default ''
favicon_key         varchar(255) not null default ''
favicon_content_type varchar(100) not null default ''
started_at          timestamptz not null          -- 站点运行起始日
footer_icp          varchar(100) not null default ''
footer_police       varchar(100) not null default ''
footer_police_url   varchar(500) not null default ''
footer_copyright    varchar(200) not null default ''
footer_text         text not null default ''
show_visits         boolean not null default true
home_json           jsonb not null default '{}'   -- HomeContent
updated_by          bigint not null default 0
updated_at          timestamptz not null
```

`site_visit_counters` 单例：

```text
id          smallint primary key    -- 固定 1
count       bigint not null default 0
updated_at  timestamptz not null
```

首次读取任一单例时懒创建：`INSERT ... ON CONFLICT DO NOTHING` 后重新读取，避免并发创建冲突。首次创建时直接写入完整的 `DefaultHomeContent()`；若历史行 `home_json` 为空对象，读取时补成完整默认值并持久化，因此 API 永远返回完整的首页结构，不会把 `{}` 暴露给前端。

`HomeContent`：

```go
type Link struct {
    Icon        string `json:"icon"`        // e.g. "pi pi-book"
    IconPos     string `json:"iconPos"`     // left / right，CTA 使用；普通卡片忽略
    Title       string `json:"title"`
    Description string `json:"description"`
    To          string `json:"to"`          // 仅站内路径，如 /problems
}

type HomeContent struct {
    Eyebrow         string `json:"eyebrow"`
    Title           string `json:"title"`
    TitleHighlight  string `json:"titleHighlight"`
    Description     string `json:"description"`
    PrimaryCTA      Link   `json:"primaryCta"`
    SecondaryCTA    Link   `json:"secondaryCta"`
    QuickTitle      string `json:"quickTitle"`
    QuickLinks      []Link `json:"quickLinks"`
    GrowthTitle     string `json:"growthTitle"`
    GrowthLinks     []Link `json:"growthLinks"`
    GuestTitle      string `json:"guestTitle"`
    GuestDescription string `json:"guestDescription"`
}
```

默认值与当前 `HomeView.vue` 完全一致；默认快速开始为题库/比赛/排行榜/测评记录，成长中心为成就墙/任务大厅/徽章商城/称号商店。登录后仍自动追加“装扮管理”入口，不进入可编辑列表。

### 5.2 校验

首页：

- `eyebrow` ≤ 40 字符；`title` ≤ 60；`titleHighlight` ≤ 30；`description` ≤ 300。
- CTA：`title` 1–20；`to` 必须是 `/` 开头且不能是 `//`、`http://`、`https://`、`javascript:`；`icon` 必须是 `pi pi-` 前缀；`iconPos` 只能是 `left`/`right`。
- `quickTitle`/`growthTitle` ≤ 20。
- `quickLinks`/`growthLinks` 各 0–8 条；每条 `title` 1–40、`description` ≤ 120、`to`/`icon` 规则同上；不允许重复 `to`。
- `guestTitle` ≤ 60；`guestDescription` ≤ 300。
- 所有文本去除首尾空白；控制字符拒绝。

页脚：

- `icp`/`police` ≤ 100；`policeUrl` ≤ 500，空或 `https://` 开头；`copyright` ≤ 200；`text` ≤ 1000（保留换行）。
- `startedAt` 必填、不能晚于当前时间 + 1 天；保存为 UTC。
- `showVisits` 布尔。

图片：

- Logo ≤ 2 MiB；favicon ≤ 1 MiB。
- 允许 PNG/JPEG/WebP/GIF，使用 magic bytes 校验；拒绝 SVG（避免脚本注入）和其他类型。
- Logo 上传到 `site/logo/<random>`，favicon 上传到 `site/favicon/<random>`；先写新文件，再更新 DB，最后 best-effort 删除旧文件。DB 更新失败时删除新文件。

### 5.3 公开 API

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/site` | 公开站点配置：`logoUrl`、`faviconUrl`、`home`、`footer`、`startedAt`、`runningDays`、`visitCount` |
| GET | `/api/site/logo` | 返回已上传 Logo；无则 404；`Cache-Control: public, max-age=31536000, immutable` |
| GET | `/api/site/favicon` | 返回已上传 favicon；无则 404；同上 |
| POST | `/api/site/visit` | 访问量 +1，返回 `{visitCount}`；无需登录 |

`logoUrl`/`faviconUrl` 为空表示未上传；非空时带 `?v=<site_settings.updated_at unix>` 便于替换后缓存失效。`runningDays = max(0, floor((now - startedAt)/24h))`。

公开读取不返回 `logoKey`、`faviconKey`、`updatedBy` 等内部字段。

### 5.4 管理 API

全部要求登录 + `site.settings.manage`：

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/admin/site` | 管理视图（公开视图 + 原始页脚字段 + 访问量/运行天数） |
| PUT | `/api/admin/site/home` | 整体更新 `HomeContent`，返回管理视图 |
| PUT | `/api/admin/site/footer` | 更新页脚字段与 `startedAt`，返回管理视图 |
| POST | `/api/admin/site/logo` | multipart `file`，上传/替换 Logo |
| DELETE | `/api/admin/site/logo` | 移除 Logo |
| POST | `/api/admin/site/favicon` | multipart `file`，上传/替换 favicon |
| DELETE | `/api/admin/site/favicon` | 移除 favicon |

错误：

- 未登录 401；无权限 403。
- 校验失败 40001。
- 图片缺失/格式不支持/过大 40001。
- 无文件可删时删除接口仍返回成功（幂等）。

审计：

- action = `site.settings.manage`；target = `site_settings` 或 `site_asset`。
- summary：修改首页内容 / 修改页脚内容 / 更新站点 Logo / 移除站点 Logo / 更新站点图标 / 移除站点图标。
- 设置写入与审计尽量同事务；图片文件与 DB 无法同事务时，DB 成功后 best-effort 写审计并记录失败日志，不阻断响应。

### 5.5 RBAC

- 新增权限码：`site.settings.manage`（第 38 个），分组「站点」，描述“编辑站点品牌、首页内容与页脚”。
- 默认仅 super；`content_admin` 等既有内置角色不自动获得（Seed 只补目录，不恢复/新增角色授权）。管理员可在 `/admin/roles` 分配。
- 新增 `PERM.site.settings.manage`、`ADMIN_ACCESS_PERMISSIONS`、AdminNav「站点」分组与 `/admin/site` 路由（`permission` 守卫）。
- 审计 action 加入 `rbac.AuditActionSiteSettingsManage`。

### 5.6 前端数据流

新增 Pinia `site` store：

- `config: SitePublicView | null`；`ensureLoaded()` 只请求一次，失败静默回退。
- 默认内容常量与后端默认一致；`home`/`footer` 的 computed 在 config 为空时返回默认值。
- `logoUrl`/`faviconUrl` 为空时使用现有静态品牌/`/favicon.svg`。
- `recordVisit()`：检查 `sessionStorage['kapoj:site-visited']`；未设置则 `POST /api/site/visit`，成功后写 `visitCount` 并设置标记；同一会话重复调用无请求。
- `applyFavicon()`：配置加载/变化后更新 `<link rel="icon">` 的 href；移除后恢复 `/favicon.svg`。

`App.vue` 启动时 `ensureLoaded()`；`DefaultLayout` 挂载时 `recordVisit()`（只统计公开站点会话，不统计纯后台会话）。`HomeView` 使用 store 的 `home` 数据渲染。`SiteFooter` 使用 store 的 `footer`/`visitCount`/`runningDays`。

### 5.7 品牌与 favicon

- 新增/扩展共享品牌组件：上传了 Logo 时显示 `<img>`（高度 32/34，保持比例，`object-fit: contain`），否则显示现有 `K` 方块 + `kapOJ` 文字。
- 侧栏、顶部 header、移动 Drawer、登录/注册页使用同一个 Logo 来源。
- favicon 通过动态 `<link rel="icon">` 替换；`index.html` 的 `/favicon.svg` 作为初始/回退。
- Logo 的 `alt` 使用“kapOJ 首页”；图片加载失败时回退文字品牌（`@error` 处理）。

### 5.8 页脚

新增 `SiteFooter.vue`，由 `AppShell` 的 `#footer` slot 渲染在公开页面主内容之后；后台布局不渲染公开页脚。

展示顺序：

```text
© 2026 kapOJ
京ICP备xxxx号 · 京公网安备xxxx号
自定义说明（保留换行）
本站已运行 N 天 · 总访问量 1,234
```

- ICP 文本链接到 `https://beian.miit.gov.cn/`（有值时）。
- 公安备案号有 `policeUrl` 时链接，否则纯文本。
- `showVisits=false` 时只控制页脚是否显示访问量，不停止计数（后台仍累计，管理页仍可查看）。
- 空字段不渲染分隔符。
- 移动端自动换行，字号 10–11px，使用现有 muted 文本 token。

### 5.9 管理页 `/admin/site`

单页 + Tabs：

- **品牌**：Logo 预览/上传/移除；favicon 预览/上传/移除；支持格式与大小提示；上传成功刷新 `?v=`。
- **首页**：欢迎区字段；主/次 CTA；快速开始标题 + 卡片编辑器；成长中心标题 + 卡片编辑器；游客提示；保存；卡片行支持添加、删除、上移、下移，图标用 PrimeVue `Select` 预设列表。
- **页脚**：ICP/公安备案/链接/版权/自定义说明/运行起始日/显示访问量开关；只读展示当前访问量与运行天数；保存。

交互：加载态、错误重试、保存 Toast、未保存离开不强制拦截；表单校验在服务端和前端各做一次。所有 Tab 共用 `site.settings.manage` 权限。

### 5.10 管理入口

`AdminNav` 新增分组「站点」和“站点设置”入口，仅 `site.settings.manage` 可见。`/admin/site` 路由 meta `permission: PERM.site.settings.manage`，无权限直接 403。

## 6. API 契约

`packages/api/openapi.yaml` 新增：

- `SiteLink`、`SiteHomeContent`、`SiteFooterView`、`SitePublicView`、`SiteAdminView`、`SiteFooterInput`。
- 公开 paths：`GET /site`、`GET /site/logo`、`GET /site/favicon`、`POST /site/visit`。
- 管理 paths：`GET /admin/site`、`PUT /admin/site/home`、`PUT /admin/site/footer`、`POST/DELETE /admin/site/logo`、`POST/DELETE /admin/site/favicon`。
- 生成 `apps/web/src/api/schema.d.ts`，同步 `types.ts` 别名。

## 7. 测试与验收

### 7.1 后端

- 迁移：`site_settings`、`site_visit_counters` 表存在；懒创建幂等；默认内容/`startedAt`/`showVisits=true`。
- 公开配置：游客可读；Logo/favicon 未上传时 URL 为空；上传后 URL 带版本；运行天数按 `startedAt` 计算；访问量读取正确。
- 管理权限：无权限 403、未登录 401、super 200；所有写接口鉴权一致。
- 首页校验：长度、链接协议、图标前缀、重复链接、卡片数量、控制字符；合法保存后公开读取一致。
- 页脚校验：URL、日期、长度；`startedAt` 影响运行天数；`showVisits` 影响公开视图。
- 图片：PNG/JPEG/WebP/GIF 上传成功；SVG/文本/超大拒绝；替换后旧文件清理；删除幂等；公开读取 Content-Type/缓存头/404。
- 访问量：并发/重复 POST 原子递增；返回最新计数。
- 审计：每个写操作 action/target/summary；无权限不写成功审计。
- RBAC：权限目录 38、包含 `site.settings.manage`；默认仅 super；前端权限目录/管理入口/路由守卫测试。

### 7.2 前端

- `site` store：加载成功/失败回退；默认内容与当前首页一致；favicon 替换/回退；sessionStorage 去重访问上报、失败不阻塞。
- `HomeView`：使用配置渲染欢迎区/快速开始/成长中心/游客提示；默认回退；登录后仍追加装扮管理。
- 品牌：Logo 上传后侧栏/顶部/Drawer/认证页显示图片；未上传显示文字品牌；图片失败回退文字。
- 页脚：公开页面显示；字段为空不显示分隔符；ICP/公安链接；运行天数/访问量格式化；`showVisits=false` 隐藏访问量；移动端换行。
- 管理页：三个 Tab 加载/保存/错误/校验；Logo/favicon 上传/移除预览；卡片增删/上下移动；离开重载后数据保持；无权限路由 403。
- 全量前端测试、lint、build。

### 7.3 真实冒烟

- 临时 server + 测试库 + 临时 storage：上传 PNG Logo/favicon，公开读取与 `?v=` 变化；替换/删除清理旧文件。
- 保存首页/页脚，游客 `GET /api/site` 与首页渲染一致；运行天数、访问量、备案链接正确。
- `POST /api/site/visit` 两次计数 +2；前端同一标签页只上报一次。
- 无权限用户访问 `/api/admin/site` 403；super 正常；审计记录。
- 浏览器检查首页、页脚、管理页三 Tab、Logo/favicon、移动端 390px 与暗/亮主题。

## 8. 已知取舍

- Logo 只支持单张图片；不做深/浅色两套或裁剪。上传图片会替换整块品牌视觉，未上传时保留现有文字品牌。
- favicon 只支持栅格图片（PNG/JPEG/WebP/GIF），不支持 SVG/ICO；现代浏览器使用 PNG favicon 已足够，静态 SVG 保留为回退。
- 访问量是会话级累计，不是 UV/PV 报表；刷新不增加，新标签页/新会话增加，恶意客户端仍可伪造 POST。`showVisits` 只影响页脚展示，不影响后台计数。
- 首页是结构化字段，不是自由页面搭建器；新增区块需要改代码和 schema。
- 新权限默认只给 super；既有 `content_admin` 不会自动获得，需要管理员在角色页显式分配。
- 页脚运行天数是整数天，不显示小时/分钟；起始日由管理员维护，首次默认当天。
- 站点配置读取失败时首页回退到当前硬编码内容，后台显示错误；这保证可用性但可能短暂显示旧内容。
