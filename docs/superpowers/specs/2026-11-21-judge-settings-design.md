# KAPOJ 评测与测试数据限制后台设置设计

> 目标：把 judge 运行时输出限制和测试数据上传/导入的输入输出限制从硬编码改为管理后台“系统 → 评测设置”可调，管理员修改后新提交和新上传立即生效，默认值保持当前行为。

## 1. 背景

- judge 当前硬编码：输出下限 1 MiB、余量 1 MiB、最大输出 16 MiB、标准答案读取上限 16 MiB。题目 111 的期望输出最大约 1.86 MiB，曾因固定 1 MiB 被误判为“输出超过限制”。
- 测试数据上传硬编码：单文件 16 MiB（单题、批量、Hydro/题包导入）、批量总大小 200 MiB、单题导入总大小 200 MiB。管理员无法按服务器资源或题型调整。
- 已有 `problem_import_settings` 只控制题包 zip 大小/解压上限，位于站点设置，不覆盖测试点文件。
- 管理后台“系统”组当前有定时任务、邮件、备份；`system.settings.manage` 权限已存在但未使用。

## 2. 目标与非目标

### 目标

- 新增系统页 `/admin/judge`（“评测设置”），使用 `system.settings.manage`。
- 可配置 judge 运行时：
  - 输出下限
  - 输出余量
  - 最大输出（同时作为标准答案读取上限）
- 可配置测试数据：
  - 单个输入文件上限
  - 单个输出文件上限
  - 批量上传总大小上限
  - 单题导入总大小上限（Hydro/题包导入）
- judge/server 都读取同一张 `judge_settings` 单例行；修改无需重启，下一次提交/上传生效。
- 修改写审计；所有校验错误中文；默认值与当前常量完全一致。
- 保留已有题包 zip/解压设置，不改变其入口。

### 非目标

- 不调整题目时间/内存限制、测试点数量上限、checker 大小、题包 zip/解压上限。
- 不调整 judge 并发、Docker 镜像、队列、CPU/内存沙盒参数。
- 不追溯修改已有测试数据；只影响后续上传/导入和后续评测。
- 不做按题目/按比赛覆盖，只做全站单例。

## 3. 数据模型

`judge_settings` 单例（`id=1`）：

```text
id                         bigint primary key
output_floor_bytes         bigint not null default 1048576    -- 1 MiB
output_slack_bytes         bigint not null default 1048576    -- 1 MiB
max_output_bytes           bigint not null default 16777216   -- 16 MiB
max_input_file_bytes       bigint not null default 16777216   -- 16 MiB
max_output_file_bytes      bigint not null default 16777216   -- 16 MiB
max_batch_total_bytes      bigint not null default 209715200  -- 200 MiB
max_problem_total_bytes    bigint not null default 209715200  -- 200 MiB
updated_by                 bigint not null default 0
updated_at                 timestamptz not null
```

约束/校验：

- `output_floor_bytes`：1 KiB–64 MiB。
- `output_slack_bytes`：0–64 MiB。
- `max_output_bytes`：1–64 MiB，且 `>= output_floor_bytes`、`>= output_slack_bytes`。
- `max_input_file_bytes`：1–256 MiB。
- `max_output_file_bytes`：1–64 MiB，且 `<= max_output_bytes`（保证标准答案可被读取）。
- `max_batch_total_bytes`：1–2048 MiB。
- `max_problem_total_bytes`：1–2048 MiB。
- 单例行缺失时懒创建默认值；读取失败时 judge 回退默认值并记录 warn。

## 4. 服务端

新增 `apps/server/internal/judgesettings`：

- `Settings` GORM 模型、`SettingsView`、`SettingsInput`。
- `SettingsService.Get(ctx)` 懒创建默认行；`Update(ctx, actorID, in, audit)` 校验、行锁、写 `updated_by/updated_at` 和审计。
- `TestcaseLimits(ctx)` 返回 `problem` 包需要的四项限制（输入、输出、批量总量、单题导入总量）。
- `db.Migrate` AutoMigrate `judge_settings`。

管理 API：

| 方法 | 路径 | 权限 |
|---|---|---|
| GET | `/api/admin/judge/settings` | `system.settings.manage` |
| PUT | `/api/admin/judge/settings` | `system.settings.manage` |

- PUT 审计 action `system.settings.manage`，target `judge_settings`。
- 响应包含字节值与 `updatedAt`。

## 5. 测试数据上传接入

`problem.Service` 增加可选 `TestcaseLimitsProvider`：

```go
type TestcaseLimits struct {
    MaxInputBytes         int64
    MaxOutputBytes        int64
    MaxBatchTotalBytes    int64
    MaxProblemTotalBytes  int64
}
```

- 默认值与当前常量一致；provider 为 nil/读取失败时使用默认值。
- 单测试点上传：输入/输出分别使用 `MaxInputBytes`/`MaxOutputBytes`。
- 批量上传：单文件同上限，批次总量使用 `MaxBatchTotalBytes`。
- Hydro/题包导入：单文件使用输入/输出上限，单题总量使用 `MaxProblemTotalBytes`。
- 已有 `problem_import_settings` 的题包 zip/解压限制保持不变。
- 上传路由改用新的 testcase upload 组，外层 body 硬上限 1 GiB；批量路由 body 硬上限 2 GiB + 16 MiB。服务层仍按后台设置精确校验，固定值只防超大请求。

## 6. judge 接入

- judge 新增 `model.JudgeSettings` 与 `db.Repository.JudgeSettings(ctx)`，读取 `judge_settings`；行缺失/查询失败回退默认。
- 每次 `Run` 读取一次设置（单次查询，避免每个测试点查询）。
- 输出上限：

```text
expected = 本批测试点 output_size 最大值
limit = clamp(max(output_floor, expected + output_slack), output_floor, max_output)
```

- 标准答案读取使用 `max_output_bytes`。
- 不因管理员下调输入上限而拒绝已有测试数据；输入大小限制只作用于后续上传/导入。

## 7. 前端

- 新页面 `AdminJudgeView.vue`，路由 `/admin/judge`，name `admin-judge`，权限 `system.settings.manage`。
- 管理导航“系统”组新增“评测设置”。
- 表单分两组，UI 使用 MiB（允许 0.5 MiB 步进；API 仍传字节）：
  - **评测运行**：输出下限、输出余量、最大输出。
  - **测试数据**：单个输入文件、单个输出文件、批量上传总量、单题导入总量。
- 提示：修改只影响后续提交/上传；已有数据不自动修改；降低输出上限可能导致已有大输出题目被截断。
- 保存成功 Toast；加载/保存失败中文；字段范围错误前端提示、后端兜底。
- OpenAPI、生成类型、`api/admin.ts` 增加 `getAdminJudgeSettings` / `updateAdminJudgeSettings`。

## 8. 测试与验收

- 设置服务：默认值懒创建、范围校验、floor/slack/output 关系、审计回滚、更新立即读取。
- problem：低限制拒绝单文件/批量/导入；高限制接受；默认值保持原行为。
- judge：自定义 settings 计算输出上限；标准答案读取使用配置；表缺失/读取失败回退默认。
- 前端：加载、保存、范围错误、导航权限。
- OpenAPI 生成、lint、build、server/judge/web 全量测试。
- 真实冒烟：临时库修改输出上限，重评题目 111 标准答案通过；调低限制后新上传被拒；恢复默认值。

## 9. 交付拆分

1. `judge_settings` 模型/迁移/服务/审计/API。
2. problem 上传与导入接入动态限制；上传 body 组调整。
3. judge 读取设置并应用到输出/答案读取。
4. OpenAPI/types/API client。
5. 管理后台“评测设置”页面与导航。
6. 文档、全量验证、真实冒烟。

## 10. 已定决策

- 使用独立 `judge_settings` 单例，不复用 `site_settings` JSON 或环境变量；判断依据是强校验、审计、server/judge 共享读取。
- 题目 111 这类大输出题目：输出上限按期望输出 + 余量动态提升，最高受管理员 `max_output_bytes` 限制。
- 已有题包 zip/解压设置保持在站点设置，不迁移到本页，避免扩大改动。
