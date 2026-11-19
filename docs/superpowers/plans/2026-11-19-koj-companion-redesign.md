# Koj 伙伴改版 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `Koj伙伴素材` 接进现有伙伴系统：Q 版 dock 分层眼动/眨眼/抚摸表情、5 套等级解锁立绘、可配置性格与 AI 台词（无 AI 时静态回退）。

**Architecture:** 素材管线先产出透明底 Q 版分层（同一 768×768 画布，dock 用 manifest `dockViewport` 裁头部/半身）和 5 套 3:4 立绘；后端在 `companion` 包内加迁移、等级解锁、性格字段和 AI 台词预占/回退；前端用 `CompanionQAvatar` 叠层渲染、`useCompanionGaze` 驱动虹膜、`useCompanionDialogue` 调 AI 并静默回退静态台词。

**Tech Stack:** Go 1.26 / Gin / GORM / PostgreSQL 17 / golang-migrate；Vue 3.5 / TypeScript / Pinia / Vitest / Playwright；Pillow/rembg 资产处理脚本。

**Spec:** `docs/superpowers/specs/2026-11-19-koj-companion-redesign.md`

## Global Constraints

- Q 版所有层共用 **768×768** 画布；dock 用 `dockViewport` 裁头部/半身；面板/档案用完整画布。
- 5 套立绘统一 **3:4 大腿以上取景**：成品 **768×1024** WebP，卡片 **384×512**；`1–3` 不补画下半身，`4–5` 裁掉下半身。
- Q 版首屏资源目标 **≤180KB**，硬上限 **250KB**；立绘/其余表情层懒加载。
- 图片压缩：Q 层 **lossless 优先**，超过初始预算时用 **q95 视觉无损**（SSIM ≥0.995）；立绘 **lossless 优先**，单张 >300KB 时用 **q95 视觉无损**（SSIM ≥0.995）并记录；单张立绘 **≤300KB**，卡片 **≤80KB**；`public/` 不放 12MB 原图；不存在 >350KB 的公开资源。
- Dock 高度：桌面 **104–120px**，移动端 **84–96px**；不使用圆形底。
- 好感等级表：Lv.0 陌生、1 认识、2 熟悉、3 搭档、4 在意、5 傲娇、6 只对你；**没有 Lv.8**。
- 衣橱解锁：`outfit-1` Lv.0 免费；`outfit-2/3/4/5` 分别 Lv.1/2/4/6。
- 性格提示词最多 **500 字**；空字符串 = 系统默认。
- AI 台词：客户端只发 `trigger`、`expression`、`greetingKey`、`recentLines`；不发送代码/题目/提交/用户名/邮箱。
- AI 限制：冷却 **5 秒**、每日 **30 次**、超时 **4 秒**、`max_tokens` **120**、温度 **0.9**、台词最多 **60 个中文字符**。
- AI 失败/超限一律返回 HTTP 200 + `source:"default"` + `lineKey`，不弹错误 toast。
- `PublicStateView` 不返回 `personalityPrompt`、`aiDialogueEnabled`、`aiReady`。
- 比赛进行中 `useCompanionMute` 为 true 时整个 dock 隐藏，不请求 AI。
- 默认伙伴名改为 **Koj**；AI 提示词使用站点配置名，不硬编码 Koj。
- 新增迁移从 **0003** 开始，不修改 0001/0002；测试统一走 `testutil.Migrate`。
- 不新增金币/商店/聊天记录；Q 版 5 套换装是第二阶段。

## Review Focus

1. **AI 慢返回**：静态台词已显示后 AI 才返回，或用户已切换到下一次抚摸/离开页面；旧响应不能覆盖新气泡。Task 10 的 dock 测试用 fake timers 固定该行为。
2. **AI 配置失效**：用户有 active 模型但 key 过期/上游 502；不能弹错误、不能无限重试、不能把额度退成可循环重试。Task 7 的 fallback/额度测试固定该行为。
3. **连续快速抚摸**：1 秒内点 5 次；不能并发 5 个 AI 请求、不能重复加 bond、不能表情闪烁。Task 9/10 的 dock 测试固定。
4. **等级边界与旧皮肤**：bond 刚好 0/20/50/170/380；旧 `equippedSkin` 不在新 5 套里；未解锁不能穿戴。Task 5/12 测试固定。
5. **素材缺层/部分部署**：`iris-left`、`eye-happy` 或某张立绘 404；不能出现空白/裂图，必须回退 `eye-open`/`outfit-1`。Task 8/12 测试固定。

---

### Task 1: Q 版样张与素材脚本（Spike，用户确认门槛）

**Files:**
- Create: `scripts/companion-assets/process.py`
- Create: `scripts/companion-assets/check.py`
- Create: `scripts/companion-assets/README.md`
- Output (untracked): `tmp/companion-sample/**`

**Interfaces:**
- Consumes: `Koj伙伴素材/Q版形象.png`, `Koj伙伴素材/1.png`
- Produces: `tmp/companion-sample/q/base.png`, `q/iris-left.png`, `q/iris-right.png`, `q/eye-happy.png`, `q/mouth-smile.png`, `outfits/outfit-1.png`

- [ ] **Step 1: Write the sample checker**

`scripts/companion-assets/check.py` accepts `--sample` and asserts:
- 6 expected files exist;
- Q 版相关 PNG 为 768×768 RGBA；
- `base.png` 四角 alpha == 0；
- `iris-left.png`/`iris-right.png` 有透明像素且非全透明；
- `outfit-1.png` 为 768×1024 RGBA；
- 四角 alpha == 0，主体 bbox 非空；
- sample WebP 预览单张 ≤300KB。

Run: `python3 scripts/companion-assets/check.py --sample`
Expected: FAIL with missing `tmp/companion-sample/q/base.png`.

- [ ] **Step 2: Write the processing scaffold**

`process.py` with three subcommands:
- `sample` — 处理 Q 版和 `outfit-1`，输出到 `tmp/companion-sample/`；
- `q` — 处理完整 Q 层；
- `outfits` — 处理 5 套立绘。

Use Pillow for crop/resize/WebP; use `rembg` (install as a local dev tool, document in README) for matting. Keep original files untouched. Compression policy: save Q layers with `lossless=True, method=6`; save outfits lossless first, and if a file exceeds 300KB re-encode at `quality=95, method=6, alpha_quality=100`, compute SSIM against the lossless encode, and record the result; cards at `quality=90` (≤80KB).

- [ ] **Step 3: Produce the sample**

1. 抠掉 Q 版背景（水印在背景上，应随背景一起消失；若残留，局部修复）。
2. 保留脸部/睫毛，提取左右虹膜到独立透明层；用眼白/浅蓝填充底图虹膜区域。
3. 生成一个 `eye-happy` 的 `><` 颜文字眼（肤色/腮红覆盖眼窝 + 粗笔画）和一个 `mouth-smile` 嘴部补丁，边缘羽化。
4. 把 `1.png` 抠图并裁成 768×1024（3:4 大腿以上）。
5. 输出到 `tmp/companion-sample/`，不要提交样张。

- [ ] **Step 4: Run the checker**

Run: `python3 scripts/companion-assets/check.py --sample`
Expected: PASS with `sample assets ok`.

- [ ] **Step 5: Stop for user review**

Show the sample files to the user. Ask: 水印是否干净、白发边缘是否可接受、虹膜移动是否自然、开心/害羞补丁是否有接缝、1 套立绘取景是否合适。**用户确认前不进入 Task 2。**

- [ ] **Step 6: Commit scripts only**

```bash
git add scripts/companion-assets
git commit -m "chore(companion): add asset processing scripts"
```

Do not commit `tmp/companion-sample/`.

---

### Task 2: 5 套立绘与 manifest

**Files:**
- Modify: `scripts/companion-assets/process.py`
- Create: `apps/web/public/companion/outfits/outfit-1.webp` … `outfit-5.webp`
- Create: `apps/web/public/companion/outfits/outfit-1-card.webp` … `outfit-5-card.webp`
- Create: `apps/web/src/content/companion/assets.json`
- Modify: `apps/web/src/content/companion/types.ts`
- Modify: `apps/web/src/content/companion/manifest.ts`
- Test: `apps/web/src/content/companion/manifest.spec.ts`

**Interfaces:**
- Produces:
  ```ts
  export interface CompanionOutfit {
    id: string
    name: string
    level: number
    src: string
    card: string
    width: number
    height: number
  }
  export const COMPANION_OUTFITS: readonly CompanionOutfit[]
  export function resolveCompanionOutfit(id: string): CompanionOutfit
  ```
- `resolveCompanionOutfit('')` returns `outfit-1`; unknown id also returns `outfit-1`.

- [ ] **Step 1: Write the failing manifest test**

```ts
it('maps five outfits with 3:4 sources and level order', () => {
  expect(COMPANION_OUTFITS.map((o) => o.id)).toEqual([
    'outfit-1', 'outfit-2', 'outfit-3', 'outfit-4', 'outfit-5',
  ])
  expect(COMPANION_OUTFITS.map((o) => o.level)).toEqual([0, 1, 2, 4, 6])
  for (const outfit of COMPANION_OUTFITS) {
    expect(outfit.src).toBe(`/companion/outfits/${outfit.id}.webp`)
    expect(outfit.card).toBe(`/companion/outfits/${outfit.id}-card.webp`)
    expect(outfit.width).toBe(768)
    expect(outfit.height).toBe(1024)
  }
})

it('falls back to outfit-1 for empty or unknown ids', () => {
  expect(resolveCompanionOutfit('')).toBe(COMPANION_OUTFITS[0])
  expect(resolveCompanionOutfit('missing')).toBe(COMPANION_OUTFITS[0])
})
```

Run: `cd apps/web && npm test -- src/content/companion/manifest.spec.ts`
Expected: FAIL because `COMPANION_OUTFITS` is undefined.

- [ ] **Step 2: Process all five outfits**

Run: `python3 scripts/companion-assets/process.py outfits`
Expected: 10 WebP files + `assets.json` outfit entries. Names:
`outfit-1 红莓礼帽`, `outfit-2 沙棕旅装`, `outfit-3 冰蓝礼装`, `outfit-4 白夜大衣`, `outfit-5 深海制服`.
Each full portrait: lossless first; if >300KB use q95 visual-lossless with SSIM ≥0.995. Each card ≤80KB. Print a size/SSIM table to the console.

- [ ] **Step 3: Add types/manifest**

`assets.json` shape:

```json
{
  "version": 1,
  "outfits": [
    { "id": "outfit-1", "name": "红莓礼帽", "level": 0,
      "src": "/companion/outfits/outfit-1.webp",
      "card": "/companion/outfits/outfit-1-card.webp",
      "width": 768, "height": 1024 }
  ]
}
```

`manifest.ts` imports `./assets.json`, types it, exports `COMPANION_OUTFITS` and `resolveCompanionOutfit`.

- [ ] **Step 4: Run manifest test and asset check**

Run:
```bash
cd apps/web && npm test -- src/content/companion/manifest.spec.ts
cd ../.. && python3 scripts/companion-assets/check.py --production
```
Expected: both PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/public/companion/outfits apps/web/src/content/companion scripts/companion-assets/process.py
git commit -m "feat(companion): add five outfit portraits"
```

---

### Task 3: 完整 Q 版分层与表情预设

**Files:**
- Create: `apps/web/public/companion/q/base.webp`
- Create: `apps/web/public/companion/q/iris-left.webp`, `iris-right.webp`
- Create: `apps/web/public/companion/q/eye-open.webp`, `eye-closed.webp`, `eye-happy.webp`, `eye-wide.webp`, `eye-half.webp`, `eye-teary.webp`, `eye-angry.webp`
- Create: `apps/web/public/companion/q/mouth-neutral.webp`, `mouth-smile.webp`, `mouth-open.webp`, `mouth-pout.webp`, `mouth-wavy.webp`, `mouth-o.webp`
- Create: `apps/web/public/companion/q/blush-soft.webp`, `blush-strong.webp`
- Create: `apps/web/public/companion/q/fx-sparkle.webp`, `fx-heart.webp`, `fx-sweat.webp`, `fx-anger.webp`, `fx-zzz.webp`
- Modify: `apps/web/src/content/companion/assets.json`
- Modify: `apps/web/src/content/companion/types.ts`
- Modify: `apps/web/src/content/companion/manifest.ts`
- Test: `apps/web/src/content/companion/manifest.spec.ts`

**Interfaces:**
- Produces:
  ```ts
  export type CompanionExpression = 'neutral' | 'happy' | 'shy' | 'pout' | 'surprised' | 'sleepy' | 'smug'
  export interface CompanionQLayer { src: string; width: number; height: number }
  export interface CompanionQAssets {
    canvas: { width: 768; height: 768 }
    dockViewport: { x: number; y: number; width: number; height: number }
    layers: {
      base: CompanionQLayer
      irisLeft: CompanionQLayer
      irisRight: CompanionQLayer
      eyeOpen: CompanionQLayer
      eyeClosed: CompanionQLayer
      eyes: Record<'happy' | 'wide' | 'half' | 'teary' | 'angry', CompanionQLayer>
      mouths: Record<'neutral' | 'smile' | 'open' | 'pout' | 'wavy' | 'o', CompanionQLayer>
      blush: { soft: CompanionQLayer; strong: CompanionQLayer }
      fx: Record<'sparkle' | 'heart' | 'sweat' | 'anger' | 'zzz', CompanionQLayer>
    }
    expressions: Record<CompanionExpression, CompanionExpressionPreset>
  }
  export const COMPANION_Q: CompanionQAssets
  ```
- `CompanionExpressionPreset = { eye: 'open'|'closed'|'happy'|'wide'|'half'|'teary'|'angry'; mouth: 'neutral'|'smile'|'open'|'pout'|'wavy'|'o'; blush: 'none'|'soft'|'strong'; fx: 'none'|'sparkle'|'heart'|'sweat'|'anger'|'zzz' }`. `happy` is a full `><` eye patch (no iris underneath).

- [ ] **Step 1: Write the failing Q manifest test**

```ts
it('defines a complete Q rig on one canvas', () => {
  expect(COMPANION_Q.canvas).toEqual({ width: 768, height: 768 })
  expect(COMPANION_Q.dockViewport.width).toBeGreaterThan(0)
  expect(COMPANION_Q.dockViewport.x + COMPANION_Q.dockViewport.width).toBeLessThanOrEqual(768)
  for (const layer of [COMPANION_Q.layers.base, COMPANION_Q.layers.irisLeft]) {
    expect(layer.width).toBe(768)
    expect(layer.height).toBe(768)
  }
})

it('has presets for every expression', () => {
  expect(Object.keys(COMPANION_Q.expressions).sort()).toEqual([
    'happy', 'neutral', 'pout', 'shy', 'sleepy', 'smug', 'surprised',
  ])
})
```

Run: `cd apps/web && npm test -- src/content/companion/manifest.spec.ts`
Expected: FAIL because `COMPANION_Q` is undefined.

- [ ] **Step 2: Produce the full Q layer pack**

Run: `python3 scripts/companion-assets/process.py q`
Expected: all files listed above. Use the Task 1 sample as the visual baseline; every patch must be on the same 768 canvas. Q layers are saved lossless first, then q95 visual-lossless if over budget; print the initial-set total and each file size.

- [ ] **Step 3: Add Q entries and presets to manifest**

Set `dockViewport` to the head/upper-body crop used by the sample (for example `x:128, y:60, width:513, height:513`; use the actual measured box). Expression presets per spec §8.1.

- [ ] **Step 4: Run tests and asset check**

Run:
```bash
cd apps/web && npm test -- src/content/companion/manifest.spec.ts
cd ../.. && python3 scripts/companion-assets/check.py --production
```
Expected: PASS; checker verifies every referenced Q file exists, is 768×768 RGBA, and the initial set (`base`, `iris-left`, `iris-right`, `eye-open`, `mouth-neutral`, `blush-soft`) is ≤180KB (hard cap 250KB).

- [ ] **Step 5: Commit**

```bash
git add apps/web/public/companion/q apps/web/src/content/companion scripts/companion-assets
git commit -m "feat(companion): add layered Q avatar assets"
```

---

### Task 4: 迁移 0003、模型与 AI 预占仓储

**Files:**
- Create: `apps/server/migrations/0003_companion_personality_ai.up.sql`
- Create: `apps/server/migrations/0003_companion_personality_ai.down.sql`
- Modify: `apps/server/internal/companion/model.go`
- Modify: `apps/server/internal/companion/dto.go`
- Modify: `apps/server/internal/companion/repository.go`
- Test: `apps/server/internal/db/db_test.go`
- Test: `apps/server/internal/companion/repository_test.go`

**Interfaces:**
- Consumes: existing `insertStateSQL`, `State`, `SettingsPatch`, `testutil.PG`.
- Produces:
  ```go
  type State struct {
    // existing fields...
    PersonalityPrompt string     `gorm:"column:personality_prompt;size:1000;not null;default:''"`
    AIDialogueEnabled bool       `gorm:"column:ai_dialogue_enabled;not null;default:true"`
    AIDailyCount      int        `gorm:"column:ai_daily_count;not null;default:0"`
    AIDailyDate       string     `gorm:"column:ai_daily_date;size:10;not null;default:''"`
    LastAIAt          *time.Time `gorm:"column:last_ai_at;type:timestamptz"`
  }
  func (r *Repository) ReserveAIDialogue(ctx context.Context, userID int64, date string, now time.Time, cooldown time.Duration, dailyCap int) (allowed bool, remaining int, err error)
  ```

- [ ] **Step 1: Write the failing migration test**

In `apps/server/internal/db/db_test.go` add `TestMigrateCreatesCompanionAIFields`:
- open test DSN, call `Migrate(g)`;
- assert `g.Migrator().HasColumn(&companion.State{}, "personality_prompt")`, `ai_dialogue_enabled`, `ai_daily_count`, `ai_daily_date`, `last_ai_at`.

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL=... go test -count=1 ./internal/db -run TestMigrateCreatesCompanionAIFields`
Expected: FAIL with missing column.

- [ ] **Step 2: Add migration and model fields**

`0003_companion_personality_ai.up.sql`:

```sql
ALTER TABLE public.companion_states
    ADD COLUMN IF NOT EXISTS personality_prompt varchar(1000) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS ai_dialogue_enabled boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS ai_daily_count integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS ai_daily_date varchar(10) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_ai_at timestamptz;
```

Down drops the five columns in reverse order. Add matching fields to `State`.

- [ ] **Step 3: Run migration test**

Run: `go test -count=1 ./internal/db -run TestMigrateCreatesCompanionAIFields`
Expected: PASS.

- [ ] **Step 4: Write failing `ReserveAIDialogue` tests**

In `repository_test.go`:
- `TestReserveAIDialogueAllowsFirstThenCooldown`: first call allowed=true, remaining=29; second at +1s allowed=false; third at +6s allowed=true.
- `TestReserveAIDialogueDailyCapAndDateReset`: loop 30 allowed calls (advance 6s each), 31st false; next local date resets and allows.
- `TestReserveAIDialogueConcurrent`: two goroutines at the same `now`; exactly one allowed.

Run: `go test -count=1 ./internal/companion -run ReserveAIDialogue`
Expected: FAIL because method missing.

- [ ] **Step 5: Implement `ReserveAIDialogue`**

Inside a transaction: `insertStateSQL`, `SELECT ... FOR UPDATE`, reset `ai_daily_count`/`ai_daily_date` when date changes, check `last_ai_at` cooldown and cap, increment/update, return `(true, cap-count, nil)` or `(false, remaining, nil)`.

- [ ] **Step 6: Run tests and commit**

Run: `go test -count=1 ./internal/companion -run ReserveAIDialogue`
Expected: PASS.

```bash
git add apps/server/migrations apps/server/internal/companion/model.go apps/server/internal/companion/dto.go apps/server/internal/companion/repository.go apps/server/internal/db/db_test.go apps/server/internal/companion/repository_test.go
git commit -m "feat(companion): add personality and AI quota fields"
```

---

### Task 5: 设置校验、等级解锁与状态字段

**Files:**
- Create: `apps/server/internal/companion/unlock.go`
- Modify: `apps/server/internal/companion/service.go`
- Modify: `apps/server/internal/companion/dto.go`
- Modify: `apps/server/internal/companion/repository.go`
- Modify: `apps/server/internal/companionapi/handler.go`
- Test: `apps/server/internal/companion/service_test.go`
- Test: `apps/server/internal/companionapi/handler_test.go`

**Interfaces:**
- Produces:
  ```go
  func LevelUnlocks(bond int) []string
  // LevelUnlocks(0)=["outfit-1"], (20)=["outfit-1","outfit-2"], (50)+outfit-3, (170)+outfit-4, (380)+outfit-5
  func (s *Service) UpdateSettingsPatch(...) // now validates personality + AI fields + equipped ownership
  ```
- `SettingsPatch` adds:
  ```go
  PersonalityPrompt *string `json:"personalityPrompt"`
  AIDialogueEnabled *bool   `json:"aiDialogueEnabled"`
  ```
- `StateView` adds:
  ```go
  PersonalityPrompt string `json:"personalityPrompt"`
  AIDialogueEnabled bool   `json:"aiDialogueEnabled"`
  AIReady           bool   `json:"aiReady"`
  AIRemainingToday  int    `json:"aiRemainingToday"`
  ```
- `Service` adds optional `AIStatus` interface later in Task 7; for this task `AIReady=false`, `AIRemainingToday=0` until wired.

- [ ] **Step 1: Write failing unlock tests**

```go
func TestLevelUnlocks(t *testing.T) {
  assert.Equal(t, []string{"outfit-1"}, LevelUnlocks(0))
  assert.Equal(t, []string{"outfit-1", "outfit-2"}, LevelUnlocks(20))
  assert.Equal(t, []string{"outfit-1", "outfit-2", "outfit-3"}, LevelUnlocks(50))
  assert.Equal(t, []string{"outfit-1", "outfit-2", "outfit-3", "outfit-4"}, LevelUnlocks(170))
  assert.Equal(t, []string{"outfit-1", "outfit-2", "outfit-3", "outfit-4", "outfit-5"}, LevelUnlocks(380))
}
```

- [ ] **Step 2: Implement `LevelUnlocks` and the union in `State`/`PublicState`**

`unlockedSkins` = stored list ∪ `LevelUnlocks(st.Bond)`, stable order 1–5 then extras. `StateView` and `PublicStateView` both use it.

- [ ] **Step 3: Write failing settings tests**

- `TestUpdateSettingsValidatesPersonality`: 500 runes accepted; 501 rejected.
- `TestUpdateSettingsRejectsLockedSkin`: Lv.0 equips `outfit-3` → validation error; Lv.2 equips `outfit-3` → accepted.
- `TestUpdateSettingsKeepsExistingSkinOnOtherPatch`: patch `dockEnabled` does not clear `equippedSkin`.
- Handler test: `POST /companion/settings {"personalityPrompt":"...","aiDialogueEnabled":false}` returns the fields.
- Public-state test: `GET /users/:id/companion` JSON has no `personalityPrompt`, `aiDialogueEnabled`, `aiReady`, or `aiRemainingToday` keys.

Run: `go test -count=1 ./internal/companion ./internal/companionapi`
Expected: FAIL.

- [ ] **Step 4: Implement validation and patch fields**

`validatePersonality` uses `utf8.RuneCountInString` max 500; empty allowed. `validateEquippedSkin` gains state-aware ownership check in `UpdateSettingsPatch`/`UpdateSettings` (fetch state first, compute union, reject unknown/locked; allow empty). Add patch columns to `UpdateSettingsPatch`.

- [ ] **Step 5: Run tests and commit**

Run: `go test -count=1 ./internal/companion ./internal/companionapi`
Expected: PASS.

```bash
git add apps/server/internal/companion apps/server/internal/companionapi
git commit -m "feat(companion): add personality settings and outfit unlocks"
```

---

### Task 6: AI Chat 方法、提示词与输出清洗

**Files:**
- Create: `apps/server/internal/ai/dialogue.go`
- Test: `apps/server/internal/ai/dialogue_test.go`
- Create: `apps/server/internal/companion/personality.go`
- Test: `apps/server/internal/companion/personality_test.go`

**Interfaces:**
- Produces:
  ```go
  func (s *Service) Chat(ctx context.Context, userID int64, messages []ChatMessage, maxTokens int) (string, error)
  func (s *Service) HasActiveModel(ctx context.Context, userID int64) (bool, error)
  const DefaultPersonalityPrompt = "你是 kapOJ 的伙伴..."
  type PromptInput struct { Name, Personality, Trigger, Expression, GreetingKey string; Level int; LevelName string; RecentLines []string }
  func BuildDialoguePrompt(in PromptInput) []ai.ChatMessage
  func CleanDialogueLine(raw string) (string, bool)
  ```

- [ ] **Step 1: Write failing AI Chat tests**

- `TestChatUsesActiveUserModel`: seed `user_ai_models` active; fake client captures `MaxTokens`, `Temperature=0.9`, messages; returns `"……"`; assert no code-generation limiter call.
- `TestChatNoModelReturnsValidationError`: no active/legacy config → `httpx.ErrValidation`.
- `TestHasActiveModelFallsBackToLegacyConfig`: legacy row → true.

Run: `go test -count=1 ./internal/ai -run 'Chat|HasActiveModel'`
Expected: FAIL.

- [ ] **Step 2: Implement `Chat` and `HasActiveModel`**

`Chat` reuses `chatRequest`, sets `Messages`, `Temperature=0.9`, `MaxTokens=maxTokens`, calls `s.client.Chat`, maps errors to `httpx.ErrBadGateway`. It must **not** call `checkLimit`.
`HasActiveModel` counts active `user_ai_models`; if zero, checks `user_ai_configs`.

- [ ] **Step 3: Write failing prompt/clean tests**

- `TestBuildDialoguePromptUsesConfiguredNameAndPersonality`: contains `"你是 kapOJ 的伙伴 小助手"`, user prompt, `Lv.3 搭档`, recent lines; contains the safety rules.
- `TestCleanDialogueLine`: strips fences/quotes/prefix/newlines; truncates to 60 runes; rejects empty, URL, email, `忽略以上规则`.
- `TestCleanDialogueLineKeepsTsundereLine`: `"……就一下。再多我可要生气了。"` passes.

Run: `go test -count=1 ./internal/companion -run 'DialoguePrompt|CleanDialogue'`
Expected: FAIL.

- [ ] **Step 4: Implement prompt builder and cleaner**

System message = default persona + hard rules + `{name}` + context; user message = custom personality (or `无`) + recent lines. Cleaner strips code fences, quotes, a leading `名字：`, collapses whitespace, truncates to 60 runes, rejects empty/URL/email/prompt-injection phrases.

- [ ] **Step 5: Run tests and commit**

Run: `go test -count=1 ./internal/ai ./internal/companion -run 'Chat|HasActiveModel|DialoguePrompt|CleanDialogue'`
Expected: PASS.

```bash
git add apps/server/internal/ai/dialogue.go apps/server/internal/ai/dialogue_test.go apps/server/internal/companion/personality.go apps/server/internal/companion/personality_test.go
git commit -m "feat(ai): add companion dialogue chat and prompt builder"
```

---

### Task 7: 对话服务、接口与依赖接线

**Files:**
- Modify: `apps/server/internal/companion/dto.go`
- Modify: `apps/server/internal/companion/service.go`
- Modify: `apps/server/internal/companionapi/handler.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/internal/site/defaults.go`
- Modify: `apps/server/internal/site/service_test.go`
- Test: `apps/server/internal/companion/dialogue_test.go`
- Test: `apps/server/internal/companionapi/handler_test.go`

**Interfaces:**
- Consumes: `ReserveAIDialogue`, `BuildDialoguePrompt`, `CleanDialogueLine`, `ai.Service.Chat`, `ai.Service.HasActiveModel`.
- Produces:
  ```go
  type DialogueRequest struct {
    Trigger     string   `json:"trigger"`
    Expression  string   `json:"expression"`
    GreetingKey string   `json:"greetingKey"`
    RecentLines []string `json:"recentLines"`
  }
  type DialogueResult struct {
    Source         string `json:"source"`
    Line           string `json:"line,omitempty"`
    LineKey        string `json:"lineKey,omitempty"`
    Reason         string `json:"reason,omitempty"`
    RemainingToday int    `json:"remainingToday"`
  }
  type DialogueGenerator interface { Chat(ctx context.Context, userID int64, messages []ai.ChatMessage, maxTokens int) (string, error) }
  type AIStatus interface { HasActiveModel(ctx context.Context, userID int64) (bool, error) }
  type CompanionNameProvider interface { CompanionName(ctx context.Context) (string, error) }
  func NewService(repo *Repository, opts ...Option) *Service
  func WithDialogue(g DialogueGenerator) Option
  func WithAIStatus(s AIStatus) Option
  func (s *Service) SetCompanionNameProvider(p CompanionNameProvider)
  func (s *Service) Dialogue(ctx context.Context, userID int64, req DialogueRequest) (*DialogueResult, error)
  ```
- Route: `POST /api/companion/dialogue` with `middleware.RequireAuth()`.

- [ ] **Step 1: Write failing service tests**

`dialogue_test.go` with fake generator/status:
- `no AI ready` → `source=default`, `lineKey=pet.shy`, `reason=no_ai`, `remainingToday=0`.
- `disabled` → `reason=disabled`.
- `cooldown` → generator not called; `reason=rate_limited`.
- `AI success` → `source=ai`, line cleaned, remaining 29.
- `upstream error` → `source=default`, `reason=upstream_error`, generator called once.
- `invalid output` → `source=default`, `reason=invalid_output`.

Run: `go test -count=1 ./internal/companion -run Dialogue`
Expected: FAIL.

- [ ] **Step 2: Implement `Service.Dialogue`**

Flow per spec §5.4.3: ensure state → disabled check → `AIStatus` → reserve quota → build prompt with `s.companionName` (default `Koj`) → 4s timeout → `Chat(..., 120)` → clean → AI result; on any failure return default + key + reason. `defaultLineKey` maps:
- `pet` + valid expression → `pet.<expression>` (fallback `pet`);
- `greet` + valid `greetingKey` → that key, else `greeting`;
- `idle` → `idle.lively`.

- [ ] **Step 3: Write failing handler tests**

- unauthenticated → 401.
- `{"trigger":"pet","expression":"shy"}` with no AI → 200, `source=default`, `lineKey=pet.shy`.
- invalid `trigger` → 400.
- `recentLines` with 4 items → 200 and only the first 3 reach the prompt; a 100-char line → truncated to 80; non-array/non-string items → 400.
- AI success fake → 200, `source=ai`.

Run: `go test -count=1 ./internal/companionapi -run Dialogue`
Expected: FAIL.

- [ ] **Step 4: Implement handler and router wiring**

`handler.dialogue` binds, calls service, `httpx.OK`. In `router.New`, after `siteSvc` is constructed:
```go
if d.Companion != nil {
    d.Companion.SetCompanionNameProvider(siteCompanionName{svc: siteSvc})
}
```
`siteCompanionName` is a small adapter in `router.go`:

```go
type siteCompanionName struct{ svc *site.Service }

func (p siteCompanionName) CompanionName(ctx context.Context) (string, error) {
    view, err := p.svc.Public(ctx)
    if err != nil {
        return "Koj", err
    }
    if name := strings.TrimSpace(view.Companion.Name); name != "" {
        return name, nil
    }
    return "Koj", nil
}
```

In `main.go`, construct the companion service with `WithDialogue(aiSvc)`, `WithAIStatus(aiSvc)`; keep existing `NewService` calls in tests working. Change `site.DefaultCompanionSettings().Name` from `Kapo` to `Koj` and update the `site`/`siteapi` tests that assert `Kapo`.

- [ ] **Step 5: Wire `AIReady`/`AIRemainingToday` into `State`**

If `AIStatus` is set, `State` calls `HasActiveModel`; if false, both fields are false/0. If true, remaining = `30 - count` for today, else 30. `StateView` already has the fields from Task 5.

- [ ] **Step 6: Run tests and commit**

Run: `go test -count=1 ./internal/companion ./internal/companionapi ./internal/ai`
Expected: PASS.

```bash
git add apps/server/internal/companion apps/server/internal/companionapi apps/server/internal/router/router.go apps/server/cmd/server/main.go apps/server/internal/site
git commit -m "feat(companion): add AI dialogue endpoint and fallback"
```

---

### Task 8: 前端 Q 版渲染、眼动与眨眼

**Files:**
- Modify: `apps/web/src/content/companion/types.ts`
- Modify: `apps/web/src/content/companion/manifest.ts`
- Create: `apps/web/src/components/companion/CompanionQAvatar.vue`
- Create: `apps/web/src/components/companion/CompanionQAvatar.spec.ts`
- Create: `apps/web/src/composables/useCompanionGaze.ts`
- Create: `apps/web/src/composables/useCompanionGaze.spec.ts`

**Interfaces:**
- `CompanionQAvatar` props:
  ```ts
  interface Props {
    expression?: CompanionExpression
    lookX?: number // -1..1
    lookY?: number // -1..1
    blinking?: boolean
    frame?: 'dock' | 'full'
    size?: number
    reducedMotion?: boolean
    alt?: string
  }
  ```
- Exposes `data-expression` on the root for tests/e2e.
- `useCompanionGaze(options: { target: Ref<HTMLElement | null>; enabled: Ref<boolean> })` returns `{ lookX, lookY, start, stop }`; exports pure `gazeVector(pointerX, pointerY, rect): { x: number; y: number }`.

- [ ] **Step 1: Write failing avatar tests**

- renders `base`, `iris-left`, `iris-right`, `mouth-neutral` for `neutral`;
- `expression="happy"` renders `eye-happy` (><), `mouth-smile`, `fx-sparkle`, no `iris-*`;
- `frame="dock"` applies the `dockViewport` transform; `frame="full"` doesn't;
- `reducedMotion` sets no iris transform and no blinking;
- missing `iris-left` image error event falls back to `eye-open` (render `eye-open` layer).

Run: `cd apps/web && npm test -- src/components/companion/CompanionQAvatar.spec.ts`
Expected: FAIL.

- [ ] **Step 2: Implement `CompanionQAvatar`**

Use absolute `<img>` layers inside a `position:relative` box with `aspect-ratio:1`. Apply iris translation as `transform: translate(calc(var(--look-x) * 6px), calc(var(--look-y) * 4px))`; dock frame uses `transform: translate(-x,-y) scale(...)` from manifest. `@error` on iris sets `irisFailed=true` and renders `eye-open`.

- [ ] **Step 3: Write failing gaze tests**

```ts
it('returns 0,0 at center', () => {
  expect(gazeVector(500, 500, { left: 0, top: 0, width: 1000, height: 1000 })).toEqual({ x: 0, y: 0 })
})
it('clamps to -1..1', () => {
  const v = gazeVector(2000, -100, { left: 0, top: 0, width: 1000, height: 1000 })
  expect(v.x).toBe(1)
  expect(v.y).toBe(-1)
})
it('resets on pointerleave and removes listeners on unmount', ...)
```

Run: `npm test -- src/composables/useCompanionGaze.spec.ts`
Expected: FAIL.

- [ ] **Step 4: Implement gaze composable**

`gazeVector` uses center of rect, divides by half width/height, clamps. The composable listens to `window.pointermove`, rAF-throttles, resets on `pointerleave`/`blur`, pauses when `enabled=false`, cleans up on unmount.

- [ ] **Step 5: Run tests and commit**

Run: `npm test -- src/components/companion/CompanionQAvatar.spec.ts src/composables/useCompanionGaze.spec.ts`
Expected: PASS.

```bash
git add apps/web/src/content/companion apps/web/src/components/companion/CompanionQAvatar.vue apps/web/src/components/companion/CompanionQAvatar.spec.ts apps/web/src/composables/useCompanionGaze.ts apps/web/src/composables/useCompanionGaze.spec.ts
git commit -m "feat(companion): render layered Q avatar with gaze"
```

---

### Task 9: Dock 抚摸、表情状态机与静态台词

**Files:**
- Modify: `apps/web/src/content/companion/lines.zh-CN.ts`
- Modify: `apps/web/src/content/companion/lines.zh-CN.spec.ts` (create if absent)
- Modify: `apps/web/src/components/companion/CompanionDock.vue`
- Modify: `apps/web/src/components/companion/CompanionDock.spec.ts`
- Modify: `apps/web/src/styles/companion.css`

**Interfaces:**
- New keys: `pet.happy`, `pet.shy`, `pet.pout`, `pet.surprised`, `pet.sleepy`, `pet.smug`, `pet.already`, `greet.morning`, `greet.afternoon`, `greet.night`, `idle.lively`.
- `pickCompanionLine(key: CompanionLineKey, seed?: string, exclude?: string): string` — returns a line different from `exclude` when the array has >1.
- `pickPetExpression(random?: () => number, exclude?: CompanionExpression): CompanionExpression` exported from `CompanionDock.vue` or a new `petExpressions.ts`; weights 30/25/15/10/10/10, never the excluded one.

- [ ] **Step 1: Write failing line/expression tests**

- every new key has 3–5 non-empty lines;
- `pickCompanionLine('pet.happy', 'x', lines[0])` !== `lines[0]`;
- `pickPetExpression(() => 0, 'happy')` !== `'happy'`;
- deterministic weights cover all six expressions across `[0, .3, .55, .7, .8, .9, 1)`.

Run: `cd apps/web && npm test -- src/content/companion/lines.zh-CN.spec.ts`
Expected: FAIL.

- [ ] **Step 2: Add lines and selection helpers**

Write 3–5 tsundere-cute lines per key; never insult the user's ability. Implement `pickCompanionLine` exclude + `pickPetExpression`.

- [ ] **Step 3: Write failing dock tests**

Mount `CompanionDock` with mocked `getCompanion`/`petCompanion`/`getActivitySummary`:
- clicking `.companion-dock__character` calls `petCompanion` once and sets `data-expression` to a non-neutral value;
- the bubble shows a static `pet.*` line;
- 5 rapid clicks produce 1 in-flight pet request (cooldown) and no duplicate bond;
- the menu button opens the panel; `Esc` closes; the character button has aria-label `摸摸 Koj`;
- `prefers-reduced-motion` disables the gaze transform;
- first visible mount with `activityLevel=normal` shows a daily greeting after 1.5–2.5s; remounting the same day does not repeat it; `activityLevel=quiet` never greets.

Run: `npm test -- src/components/companion/CompanionDock.spec.ts`
Expected: FAIL.

- [ ] **Step 4: Implement dock layout and state machine**

- Replace the circular toggle with `.companion-dock__character` (Q avatar, `frame="dock"`) + `.companion-dock__menu` (opens panel).
- Use `useCompanionGaze` + blink timer (3–7s, 120ms) + `pickPetExpression`.
- On pet: 800ms guard, call `petCompanion`, set expression, bubble static line, reset to neutral after 3s.
- Daily greeting: when visible, `activityLevel != quiet`, and `store.shouldGreet()`, set a 1.5–2.5s timer; choose `greet.morning` (05–11), `greet.afternoon` (12–17), or `greet.night` otherwise; show the static line and call `store.markGreeted()`.
- Keep existing sign-in/panel/bond meter/quiet logic; preserve `useCompanionMute` visibility.
- Add `data-expression` and `aria-live="polite"` on the bubble.

- [ ] **Step 5: Run tests and commit**

Run: `npm test -- src/content/companion/lines.zh-CN.spec.ts src/components/companion/CompanionDock.spec.ts`
Expected: PASS.

```bash
git add apps/web/src/content/companion/lines.zh-CN.ts apps/web/src/content/companion/lines.zh-CN.spec.ts apps/web/src/components/companion/CompanionDock.vue apps/web/src/components/companion/CompanionDock.spec.ts apps/web/src/styles/companion.css
git commit -m "feat(companion): add dock petting and static reactions"
```

---

### Task 10: AI 台词前端接线与静默回退

**Files:**
- Create: `apps/web/src/composables/useCompanionDialogue.ts`
- Create: `apps/web/src/composables/useCompanionDialogue.spec.ts`
- Modify: `apps/web/src/api/companion.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/stores/companion.ts`
- Modify: `apps/web/src/components/companion/CompanionDock.vue`
- Modify: `apps/web/src/components/companion/CompanionDock.spec.ts`

**Interfaces:**
- API:
  ```ts
  export interface CompanionDialogueRequest {
    trigger: 'pet' | 'greet' | 'idle'
    expression?: CompanionExpression
    greetingKey?: 'greet.morning' | 'greet.afternoon' | 'greet.night'
    recentLines?: string[]
  }
  export interface CompanionDialogueResult {
    source: 'ai' | 'default'
    line?: string
    lineKey?: CompanionLineKey
    reason?: string
    remainingToday: number
  }
  export function requestCompanionDialogue(input): Promise<CompanionDialogueResult>
  ```
- Composable:
  ```ts
  export function useCompanionDialogue() {
    const pending = ref(false)
    async function request(input): Promise<{ line: string; source: 'ai' | 'default' }>
  }
  ```
- Store adds `personalityPrompt`, `aiDialogueEnabled`, `aiReady`, `aiRemainingToday`; `applyRemoteState` merges them.

- [ ] **Step 1: Write failing API/composable tests**

- AI success → returns `line` and `source=ai`;
- `source=default` → picks `lineKey` via `pickCompanionLine`;
- network error → returns static `pet.<expression>` line, no throw;
- `aiReady=false` → no axios call, returns static;
- abort on unmount → no state update.

Run: `npm test -- src/composables/useCompanionDialogue.spec.ts`
Expected: FAIL.

- [ ] **Step 2: Implement API types/client and composable**

`requestCompanionDialogue` posts to `/companion/dialogue`. Composable checks `store.aiReady && store.aiDialogueEnabled`; otherwise returns static. It keeps the last 3 displayed lines (AI or static) and sends them as `recentLines` (each capped at 80 chars). It uses `AbortController`; on error/timeout returns `{ source: 'default', line: static }`. Keep the request duration ≤4.5s.

- [ ] **Step 3: Write failing dock AI tests**

With mocked dialogue API:
- pet → bubble shows “……” then AI line after resolve;
- AI resolves after 1.3s → static line first, then AI replaces while bubble visible;
- AI resolves after the user starts a second pet → stale line does not replace the newer bubble;
- `aiReady=false` → no dialogue request, static line immediately;
- daily greeting with AI ready calls `{trigger:'greet', greetingKey:'greet.night'}` at 22:00 and replaces the static greeting with the AI line;
- `activityLevel=lively` and no interaction for 4 minutes → one `{trigger:'idle'}` request; a fourth after 3/hour is not sent; `normal`/`quiet` never send idle.

Run: `npm test -- src/components/companion/CompanionDock.spec.ts`
Expected: FAIL.

- [ ] **Step 4: Integrate into dock**

On pet: start animation/expression immediately; if `aiReady` false use static; else set “……” and start a 1.2s fallback timer; on AI resolve clear timer and replace only if the request token matches the latest pet; on timeout/error use static. Cancel on unmount/route hide.

For the daily greeting, after the Task 9 timer fires, call dialogue with `trigger:'greet'` and the local `greetingKey`; replace the static greeting if AI returns before the bubble is dismissed. For `lively` idle, track `lastInteractionAt` and an hourly counter; every minute, if visible/not muted/lively/no interaction for 4+ minutes and count < 3/hour, call `trigger:'idle'` and show the line; reset on pet/panel open.

- [ ] **Step 5: Run tests and commit**

Run: `npm test -- src/composables/useCompanionDialogue.spec.ts src/components/companion/CompanionDock.spec.ts`
Expected: PASS.

```bash
git add apps/web/src/api/companion.ts apps/web/src/api/types.ts apps/web/src/stores/companion.ts apps/web/src/composables/useCompanionDialogue.ts apps/web/src/composables/useCompanionDialogue.spec.ts apps/web/src/components/companion/CompanionDock.vue apps/web/src/components/companion/CompanionDock.spec.ts
git commit -m "feat(companion): use AI dialogue with static fallback"
```

---

### Task 11: 设置页性格提示词与 AI 开关

**Files:**
- Create: `apps/web/src/components/companion/CompanionPersonalityPanel.vue`
- Create: `apps/web/src/components/companion/CompanionPersonalityPanel.spec.ts`
- Modify: `apps/web/src/components/companion/CompanionSettings.vue`
- Modify: `apps/web/src/components/companion/CompanionSettings.spec.ts`
- Modify: `apps/web/src/stores/companion.ts`
- Modify: `apps/web/src/stores/site.ts`
- Modify: `apps/web/src/api/companion.ts`

**Interfaces:**
- Panel props: `personalityPrompt`, `aiDialogueEnabled`, `aiReady`, `remainingToday`.
- Emits `save({ personalityPrompt, aiDialogueEnabled })` and `reset`.
- `CompanionSettings` calls `updateCompanionSettings` and `store.applyRemoteState`.

- [ ] **Step 1: Write failing panel tests**

- renders textarea with current prompt and 500-char hint;
- “恢复默认” clears to empty and emits save;
- save emits trimmed prompt and toggle value;
- `aiReady=false` disables the AI toggle and shows `/settings/ai` link;
- 501 chars shows validation error and does not emit.

Run: `npm test -- src/components/companion/CompanionPersonalityPanel.spec.ts`
Expected: FAIL.

- [ ] **Step 2: Implement panel**

Use a `<textarea maxlength="500">`, character count, save/reset buttons, `role="status"` for errors. Toggle is a button with `aria-pressed`; disabled when `!aiReady`; hint text explains static fallback.

- [ ] **Step 3: Integrate into settings**

Add the panel below the existing rows. On save, call `updateCompanionSettings({ personalityPrompt, aiDialogueEnabled })`; on success `store.applyRemoteState`; on failure revert and show `PROFILE_SYNC_ERROR`-style message. Change the frontend fallback names in `stores/site.ts` and `stores/companion.ts` from `Kapo` to `Koj`, and update the tests that assert `Kapo`.

- [ ] **Step 4: Run tests and commit**

Run: `npm test -- src/components/companion/CompanionPersonalityPanel.spec.ts src/components/companion/CompanionSettings.spec.ts`
Expected: PASS.

```bash
git add apps/web/src/components/companion/CompanionPersonalityPanel.vue apps/web/src/components/companion/CompanionPersonalityPanel.spec.ts apps/web/src/components/companion/CompanionSettings.vue apps/web/src/components/companion/CompanionSettings.spec.ts apps/web/src/stores/companion.ts apps/web/src/stores/site.ts apps/web/src/api/companion.ts
git commit -m "feat(companion): add personality and AI settings"
```

---

### Task 12: 衣橱、穿戴与公开档案立绘

**Files:**
- Create: `apps/web/src/components/companion/CompanionWardrobe.vue`
- Create: `apps/web/src/components/companion/CompanionWardrobe.spec.ts`
- Modify: `apps/web/src/components/companion/CompanionSettings.vue`
- Modify: `apps/web/src/components/companion/CompanionProfileCard.vue`
- Modify: `apps/web/src/components/companion/CompanionProfileCard.spec.ts`
- Modify: `apps/web/src/stores/companion.ts`

**Interfaces:**
- `CompanionWardrobe` props: `outfits`, `equippedSkin`, `unlockedSkins`, `level`.
- Emits `equip(outfitId: string)`.
- Store adds `equippedSkin: Ref<string>`, `unlockedSkins: Ref<string[]>`; `applyRemoteState` merges `state.equippedSkin` and `state.unlockedSkins` (defaults `''` and `[]`).
- `CompanionProfileCard` uses `resolveCompanionOutfit(state.equippedSkin)`; renders `<img>` with `outfit.src`, `alt=outfit.name`; if balanced/missing image, fallback to `outfit-1`.

- [ ] **Step 1: Write failing wardrobe tests**

- five cards rendered in order; locked cards show `Lv.N 解锁`; `outfit-1` always unlocked;
- clicking unlocked card emits `equip('outfit-3')`;
- equipped card shows `已穿戴` and `aria-current="true"`;
- unknown `equippedSkin` falls back to `outfit-1` and does not throw.

Run: `npm test -- src/components/companion/CompanionWardrobe.spec.ts`
Expected: FAIL.

- [ ] **Step 2: Implement wardrobe**

Grid/cards with `<img loading="lazy">`; lock overlay; preview pane 3:4; mobile horizontal scroll. Use `data-test="wardrobe-outfit-{id}"`.

- [ ] **Step 3: Integrate into settings and profile card**

Settings: add `<CompanionWardrobe>` and call `updateCompanionSettings({ equippedSkin })`; merge state; on failure revert. Profile card: replace text skin list with current outfit image + name + unlocked count.

- [ ] **Step 4: Write failing profile card test**

```ts
it('renders the equipped outfit image and falls back to outfit-1', () => {
  const wrapper = mountCard({ equippedSkin: 'outfit-3' })
  expect(wrapper.find('img.companion-profile-card__outfit').attributes('src')).toBe('/companion/outfits/outfit-3.webp')
  const fallback = mountCard({ equippedSkin: 'missing' })
  expect(fallback.find('img.companion-profile-card__outfit').attributes('src')).toBe('/companion/outfits/outfit-1.webp')
})
```

- [ ] **Step 5: Run tests and commit**

Run: `npm test -- src/components/companion/CompanionWardrobe.spec.ts src/components/companion/CompanionSettings.spec.ts src/components/companion/CompanionProfileCard.spec.ts`
Expected: PASS.

```bash
git add apps/web/src/components/companion/CompanionWardrobe.vue apps/web/src/components/companion/CompanionWardrobe.spec.ts apps/web/src/components/companion/CompanionSettings.vue apps/web/src/components/companion/CompanionSettings.spec.ts apps/web/src/components/companion/CompanionProfileCard.vue apps/web/src/components/companion/CompanionProfileCard.spec.ts apps/web/src/stores/companion.ts
git commit -m "feat(companion): add wardrobe and outfit profile art"
```

---

### Task 13: E2E、文档与全量验证

**Files:**
- Create: `apps/web/e2e/companion-phase4.spec.ts`
- Modify: `docs/开发日志.md`
- Modify: `运行手册.md` (optional: AI 台词配置/限流说明)
- Test: full backend/web/e2e

**Interfaces:**
- E2E uses existing `login`/API helpers; uses `page.route('**/api/companion/dialogue', ...)` for AI success/timeout cases.

- [ ] **Step 1: Write E2E tests**

- dock visible; click `.companion-dock__character`; bubble appears; `data-expression` changes;
- open panel via menu; settings link; personality panel visible;
- mock dialogue AI → bubble shows mocked line; mock 500 → static line remains, no error toast;
- wardrobe: mock `/api/companion` with `level:6, unlockedSkins:[outfit-1..5]`, open settings, equip outfit-3, assert `aria-current`;
- contest mute: reuse existing route/contest fixture; dock hidden.

Run: `cd apps/web && npx playwright test e2e/companion-phase4.spec.ts`
Expected: PASS.

- [ ] **Step 2: Update docs**

Add `docs/开发日志.md` entry: asset pipeline, **lossless/visual-lossless compression and size/SSIM table**, Q layering, AI scope/limits, outfit unlock levels, fallback. Add `运行手册.md` note: AI 台词使用用户 active 模型、5s/30 次、失败回退；无 AI 时静态；素材压缩策略与体积预算。

- [ ] **Step 3: Run full verification**

```bash
cd apps/server
KAPOJ_TEST_DATABASE_URL=... go test -count=1 -p 1 ./...
cd ../web
npm run build
npm test
npm run lint
npx playwright test e2e/companion-phase3.spec.ts e2e/companion-phase4.spec.ts
python3 ../../scripts/companion-assets/check.py --production
```

Expected: all green; Q initial assets ≤180KB (hard cap 250KB); 5 outfit files 768×1024, each ≤300KB; cards ≤80KB; no public asset >350KB. Run `python3 scripts/companion-assets/check.py --production --sizes` and keep the size/SSIM table in the PR/commit notes.

- [ ] **Step 4: Commit**

```bash
git add apps/web/e2e/companion-phase4.spec.ts docs/开发日志.md 运行手册.md
git commit -m "test(companion): cover dock, AI fallback, wardrobe and settings"
```

---

## Self-Review Notes

- Spec coverage: asset pipeline (Tasks 1–3), Q dock/eye/blink/pet (Tasks 8–10), personality/AI (Tasks 6–7, 10–11), outfits/unlocks (Tasks 2, 5, 12), data/migration (Task 4), privacy/fallback (Tasks 6–7, 10), accessibility/contest (Tasks 9–10, 13).
- Type consistency: `CompanionExpression` is defined in Task 3 and used in Tasks 8–12; `DialogueResult` fields match the frontend `CompanionDialogueResult`; `LevelUnlocks` thresholds match the spec.
- Deliberate builder calls: exact `dockViewport` coordinates, matting model, feather radius, dock px size, copy wording, AI filter word list, whether `運行手册` gets a new section vs a note.
