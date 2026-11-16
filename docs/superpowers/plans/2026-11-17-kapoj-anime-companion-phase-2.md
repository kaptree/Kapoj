# KapOJ Anime Companion Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the server-backed companion: bond/level progression, idempotent event outbox, settings sync, event reactions, pet interaction, admin companion defaults, and the dock bond meter.

**Architecture:** A new `internal/companion` module owns `companion_states` and `companion_events`. Core actions (sign-in, AC, task claim, achievement unlock, contest first submission) call a shared recorder that applies daily caps in one state-row transaction. The API exposes state/settings/pet/events; the frontend syncs state and polls unread events, while preserving the existing local fallback. Site settings gain companion defaults for global enable/name/activity/contest mute.

**Tech Stack:** Go 1.24 + Gin + GORM + PostgreSQL + Asynq, Vue 3 + Pinia + PrimeVue, Vitest, Playwright.

**Spec:** `docs/superpowers/specs/2026-11-16-kapoj-anime-companion-design.md` (Phase 2 sections).

## Global Constraints

- Scope is Phase 2 only: bond, events, settings sync, pet, admin defaults. No skins/shop, profile archive, AI persona, or share cards.
- Do not add npm or Go dependencies.
- Use the repo's local-day convention: `time.Local`, date keys `YYYY-MM-DD`, and day bounds computed with `time.Date(...).AddDate(0,0,1)`; never use `judged_at::date` or fixed 24-hour arithmetic.
- Bond rules: sign-in +3; daily first AC +2; per-problem first AC +1 (3/day); task claim +1 (2/day); achievement +1 (2/day); contest first submission +5; pet +1; streak 7/30/100 +5/+10/+20. Daily natural cap 15; streak bypasses it.
- Event keys are idempotent per user: `signin:{date}`, `ac:daily:{date}`, `ac:first:{problemID}`, `task:{taskID}`, `achievement:{achievementID}`, `contest:{contestID}`, `pet:{date}`, `streak:{days}`. Unique `(user_id, key)`.
- Companion write failures must never fail sign-in, submission, task claim, or achievement unlock; log and continue.
- All companion data is private by default. `PublicProfile` defaults false; no public endpoint exposes bond/events.
- Add `companion_states` and `companion_events` to account backup export/restore tables.
- Backend commands run from `apps/server`; `KAPOJ_TEST_DATABASE_URL` must point at `kapoj_test`, never the dev DB; use `-p 1`.
- Frontend commands run from `apps/web`: `corepack pnpm test`, `corepack pnpm lint`, `corepack pnpm build`.
- Keep Phase 0–1 behavior when the API is unavailable: local prefs and local lines still work.

## Review Focus

- **Local midnight rollover** → daily cap and `ac:daily` use the event's local date, not processing time. Tested in Tasks 2/3.
- **Concurrent events** → two ACs or task claims at once cannot overrun the cap. Tested in Task 2.
- **Rejudge / duplicate AC** → `ac:first` and `ac:daily` stay idempotent; an old AC rejudged today does not double-award. Tested in Task 3.
- **Core action survives companion failure** → sign-in/submission/task/achievement still succeed when the companion recorder errors. Tested in Tasks 5/6.
- **Contest first submission non-AC** → +5 triggers on submission creation, not only accepted verdict. Tested in Task 5.
- **Account backup/restore** → companion rows are exported and cleared on restore. Tested in Task 8.
- **Global disable** → admin `enabled=false` hides dock/notes/art but leaves settings reachable. Tested in Task 10.

---

### Task 1: Companion Model and Level Rules

**Files:**
- Create: `apps/server/internal/companion/model.go`
- Create: `apps/server/internal/companion/level.go`
- Modify: `apps/server/internal/db/db.go`
- Test: `apps/server/internal/companion/level_test.go`
- Modify: `apps/server/internal/db/db_test.go`

**Interfaces:**
- Produces `State`, `Event` GORM models and `LevelForBond(bond int) LevelInfo` with `LevelInfo{Level int, Name string, Next int}`.
- Adds both models to `db.Migrate` AutoMigrate.

- [ ] **Step 1: Write the failing level test**

```go
func TestLevelForBond(t *testing.T) {
	cases := []struct{ bond int; level int; name string; next int }{
		{0, 0, "陌生", 20}, {19, 0, "陌生", 20}, {20, 1, "认识", 50},
		{100, 3, "搭档", 170}, {380, 6, "只对你", 380},
	}
	for _, tc := range cases {
		got := LevelForBond(tc.bond)
		assert.Equal(t, tc.level, got.Level)
		assert.Equal(t, tc.name, got.Name)
		assert.Equal(t, tc.next, got.Next)
	}
}
```

- [ ] **Step 2: Run to verify RED**  
`go test ./internal/companion -run TestLevelForBond -count=1`

- [ ] **Step 3: Implement models and level rules**

```go
type State struct {
	UserID        int64  `gorm:"primaryKey"`
	Bond          int    `gorm:"not null;default:0"`
	Level         int    `gorm:"not null;default:0"`
	DailyGain     int    `gorm:"not null;default:0"`
	DailyDate     string `gorm:"size:10;not null;default:''"`
	ActivityLevel string `gorm:"size:10;not null;default:'normal'"`
	DockEnabled   bool   `gorm:"not null;default:true"`
	PublicProfile bool   `gorm:"not null;default:false"`
	EquippedSkin  string `gorm:"size:40;not null;default:''"`
	UnlockedSkins dbtypes.List[string] `gorm:"column:unlocked_skins_json;type:jsonb;not null;default:'[]'"`
	LastPetDate   string `gorm:"size:10;not null;default:''"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
func (State) TableName() string { return "companion_states" }

type Event struct {
	ID        int64  `gorm:"primaryKey"`
	UserID    int64  `gorm:"not null;uniqueIndex:uniq_companion_events_user_key,priority:1;index:idx_companion_events_user_unread,priority:1;index:idx_companion_events_user_date,priority:1"`
	Key       string `gorm:"size:64;not null;uniqueIndex:uniq_companion_events_user_key,priority:2"`
	Type      string `gorm:"size:20;not null;index:idx_companion_events_user_unread,priority:2;index:idx_companion_events_user_date,priority:2"`
	Bond      int    `gorm:"not null;default:0"`
	EventDate string `gorm:"size:10;not null;default:'';index:idx_companion_events_user_date,priority:3"`
	Payload   dbtypes.JSON[map[string]any] `gorm:"type:jsonb;not null;default:'{}'"`
	ReadAt    *time.Time `gorm:"index:idx_companion_events_user_unread,priority:3"`
	CreatedAt time.Time  `gorm:"type:timestamptz;not null"`
}
func (Event) TableName() string { return "companion_events" }
```

Add `&companion.State{}, &companion.Event{}` to `db.Migrate` and a `TestMigrateCreatesCompanionSchema` asserting tables, the `(user_id,key)` unique index, and the unread/date indexes.

- [ ] **Step 4: Run RED/GREEN tests**  
`go test ./internal/companion ./internal/db -count=1`

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/companion apps/server/internal/db
git commit -m "feat(companion): add state, event and level models"
```

---

### Task 2: Companion Repository and Event Engine

**Files:**
- Create: `apps/server/internal/companion/repository.go`
- Create: `apps/server/internal/companion/dto.go` (input DTOs used by Tasks 2–3)
- Test: `apps/server/internal/companion/repository_test.go`

**Interfaces:**
- Produces:
  - `SettingsInput{ActivityLevel string, DockEnabled bool, PublicProfile bool, EquippedSkin string}`
  - `EventInput{UserID int64, Key string, Type string, Bond int, EventDate string, Payload map[string]any, DailyCapBypass bool, SubCap int}`
  - `NewGormRepository(db *gorm.DB) *Repository`
  - `EnsureState(ctx, userID) (*State, error)`
  - `UpdateSettings(ctx, userID, SettingsInput) (*State, error)`
  - `ApplyEvent(ctx, EventInput) (*Event, bool, error)` (event, created, error)
  - `UnreadEvents(ctx, userID, limit) ([]Event, error)`
  - `MarkRead(ctx, userID, ids []int64) error`
  - `Pet(ctx, userID, date string) (*Event, bool, error)`
  - `SubmissionInfo(ctx, submissionID) (*SubmissionInfo, error)`
  - `HasEarlierAccepted(ctx, userID, problemID int64, createdAt time.Time, submissionID int64) (bool, error)`
  - `LoginStreak(ctx, userID int64, date string) (int, error)`

- [ ] **Step 1: Write failing repository tests**

Cover:
1. `EnsureState` creates defaults and is idempotent.
2. `ApplyEvent` awards once, duplicate key returns `created=false` and no second bond.
3. Daily cap: two +10 events on one date only award 10 total; next local date resets.
4. Sub-cap: three `ac_first` +1 events award 3; fourth awards 0.
5. `Pet` awards once per date and updates `LastPetDate`.
6. `UnreadEvents` returns only unread ordered by id; `MarkRead` removes them.
7. Concurrent goroutines applying two events cannot exceed the cap (use `sync.WaitGroup`; state row lock).
8. `HasEarlierAccepted` ignores the same submission and honors `created_at`/id ordering.
9. `LoginStreak` counts consecutive local dates ending at the given date.

Use `testutil.PG(t)`; create a user first (`user.NewMemoryRepository`? No, PG tests insert a user row via GORM or use an existing helper). Check the repo's existing tests for a user fixture; if none, insert `user.User{...}` with a unique username.

- [ ] **Step 2: Run to verify RED**  
`KAPOJ_TEST_DATABASE_URL=... go test ./internal/companion -run TestRepository -count=1 -p 1`

- [ ] **Step 3: Implement transaction engine**

`ApplyEvent` must:
1. Begin transaction.
2. `ensureState` with `INSERT ... ON CONFLICT DO NOTHING`; then `SELECT ... FOR UPDATE`.
3. Reset `DailyGain` when `State.DailyDate != EventDate`.
4. Compute `bond := input.Bond`; if `!DailyCapBypass`, clamp to `15 - DailyGain`; if `SubCap > 0`, clamp to `SubCap - count(type, eventDate)`.
5. Insert event with `ON CONFLICT (user_id,key) DO NOTHING`; if `RowsAffected == 0`, return the existing event with `created=false`.
6. Update state bond, level via `LevelForBond`, daily gain/date.
7. Commit and return.

Use `clause.Locking{Strength: "UPDATE"}` and `clause.OnConflict`. Do not call `time.Now()` for `EventDate`; the caller supplies it.

- [ ] **Step 4: Run the repository tests**  
Expected: PASS, including the concurrency test.

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/companion
git commit -m "feat(companion): add transactional event repository"
```

---

### Task 3: Companion Service and API DTOs

**Files:**
- Create: `apps/server/internal/companion/service.go`
- Modify: `apps/server/internal/companion/dto.go` (add view/result DTOs)
- Test: `apps/server/internal/companion/service_test.go`

**Interfaces:**
- Produces `NewService(repo *Repository) *Service` and:
  - `State(ctx, userID) (*StateView, error)`
  - `UpdateSettings(ctx, userID, SettingsInput) (*StateView, error)`
  - `Pet(ctx, userID, date string) (*PetResult, error)`
  - `RecordSignIn(ctx, userID int64, date string) error`
  - `RecordAC(ctx, userID, submissionID int64) error`
  - `RecordTaskClaim(ctx, userID, taskID int64) error`
  - `RecordAchievement(ctx, userID, achievementID int64) error`
  - `RecordContestSubmission(ctx, userID, contestID int64) error`
  - `HandleGameEvent(ctx, ev gamequeue.Event) error`
  - `UnreadEvents(ctx, userID int64, limit int) ([]EventView, error)`
  - `MarkEventsRead(ctx, userID int64, ids []int64) error`
- `StateView` includes bond, level, levelName, nextThreshold, dailyGain, dailyCap, activityLevel, dockEnabled, publicProfile, equippedSkin, unlockedSkins, unreadCount, todayEvents.
- `EventView` includes id, type, bond, payload, readAt, createdAt.

- [ ] **Step 1: Write failing service tests**

Cover:
1. `RecordSignIn` writes `signin:{date}` +3; repeated call does not double.
2. `RecordAC` for an accepted submission writes `ac:daily:{localDate}` +2 and `ac:first:{problemID}` +1; repeated/rejudged event does not double.
3. A non-accepted submission writes nothing.
4. `RecordTaskClaim` / `RecordAchievement` cap at 2/day.
5. `RecordContestSubmission` writes `contest:{id}` +5 once.
6. `Pet` awards once/day and returns `awarded=false` on repeat.
7. `State` reports the correct level/next threshold and unread count.
8. `RecordStreak` (if implemented via `RecordSignIn`) awards 7/30/100 milestones once and bypasses the 15 cap.

Use a PG DB and insert a user + submissions with `judged_at` in a fixed `time.Local` (save/restore `time.Local` as existing activity tests do).

- [ ] **Step 2: Run to verify RED**  
`go test ./internal/companion -run TestService -count=1 -p 1`

- [ ] **Step 3: Implement the service**

- `RecordSignIn`: `ApplyEvent` key `signin:{date}`, type `signin`, bond 3; then `RecordStreak` (loads `LoginStreak`, records 7/30/100 if reached, `DailyCapBypass=true`).
- `RecordAC`: read `SubmissionInfo`; if `Status != "accepted"` return nil; `date := JudgedAt.In(time.Local).Format("2006-01-02")`; `ac:daily` +2; if `!HasEarlierAccepted` then `ac:first` +1 with `SubCap: 3`.
- `RecordTaskClaim`: key `task:{id}`, +1, `SubCap: 2`.
- `RecordAchievement`: key `achievement:{id}`, +1, `SubCap: 2`.
- `RecordContestSubmission`: key `contest:{id}`, +5, positive safe id only.
- `Pet`: delegate to repository `Pet`.
- `HandleGameEvent`: only `gamequeue.EventSubmissionAccepted`; call `RecordAC`.
- `State`: `EnsureState`, `UnreadEvents` count, today's events by `EventDate`.

- [ ] **Step 4: Run service tests**  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/companion
git commit -m "feat(companion): add bond service and event rules"
```

---

### Task 4: Companion HTTP API and Router Wiring

**Files:**
- Create: `apps/server/internal/companionapi/handler.go`
- Test: `apps/server/internal/companionapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/cmd/server/main.go`

**Interfaces:**
- `RegisterRoutes(r gin.IRouter, svc *companion.Service)` mounts:
  - `GET /api/companion`
  - `GET /api/companion/events`
  - `POST /api/companion/events/read`
  - `POST /api/companion/settings`
  - `POST /api/companion/pet`
- All routes use `middleware.RequireAuth()`.
- `router.Deps` gains `Companion *companion.Service`; `main.go` constructs `companion.NewService(companion.NewGormRepository(g))`.

- [ ] **Step 1: Write the failing handler test**

Use the existing `gamificationapi/handler_test.go` fixture pattern (PG + miniredis + router + JWT token). Assert:
1. `GET /api/companion` returns defaults for a fresh user.
2. `POST /api/companion/pet` awards once, second call `awarded=false`.
3. `POST /api/companion/settings` persists `activityLevel:"quiet"`, `dockEnabled:false`, `publicProfile:true`.
4. `GET /api/companion/events?unread=1` returns events; `POST /events/read` empties them.
5. Unauthenticated requests return 401.

- [ ] **Step 2: Run to verify RED**  
`go test ./internal/companionapi -count=1 -p 1`

- [ ] **Step 3: Implement handler and wiring**

Use `httpx.OK`, `httpx.BindJSON`, `middleware.CurrentClaims`. Parse `limit` with `httpx.ParsePage` or a 1–20 clamp. Return `{"items": [...]}` for events. Add `Companion` to `router.Deps` and mount only when non-nil. Construct the service in `main.go` before `gameSvc` and pass it to `router.Deps`.

- [ ] **Step 4: Run handler + router tests**  
`go test ./internal/companionapi ./internal/router -count=1 -p 1`

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/companionapi apps/server/internal/router apps/server/cmd/server
git commit -m "feat(companion): add API and router wiring"
```

---

### Task 5: Sign-in, Task Claim and Contest Submission Hooks

**Files:**
- Modify: `apps/server/internal/activityapi/handler.go`
- Modify: `apps/server/internal/gamificationapi/handler.go`
- Modify: `apps/server/internal/submissionapi/handler.go`
- Modify: `apps/server/internal/router/router.go`
- Test: `apps/server/internal/activityapi/handler_test.go`
- Test: `apps/server/internal/gamificationapi/handler_test.go`
- Test: `apps/server/internal/submissionapi/handler_test.go`

**Interfaces:**
- `activityapi.Options` adds `Companion *companion.Service`; after `SignIn` returns `result.Awarded`, call `RecordSignIn(ctx, claims.UserID, result.Date)` and log errors.
- `gamificationapi.Options` adds `Companion *companion.Service`; after `ClaimTask` succeeds, call `RecordTaskClaim(ctx, claims.UserID, id)`.
- `submissionapi.Options` adds `Companion *companion.Service`; after `Create` succeeds and `sub.ContestID != nil`, call `RecordContestSubmission(ctx, sub.UserID, *sub.ContestID)`.

- [ ] **Step 1: Write failing hook tests**

For each handler, use a real companion service with a PG DB and assert the expected event row after the action. Also test that a companion error does not change the HTTP success response (inject a service pointed at a closed DB or wrap a recorder that returns an error).

- [ ] **Step 2: Run to verify RED**  
`go test ./internal/activityapi ./internal/gamificationapi ./internal/submissionapi -count=1 -p 1`

- [ ] **Step 3: Implement hooks and wire options**

Add `Companion` to each `Options`; store on handler; call after success with `slog.Warn` on error. Pass `d.Companion` in `router.New` for all three.

- [ ] **Step 4: Run hook tests**  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/activityapi apps/server/internal/gamificationapi apps/server/internal/submissionapi apps/server/internal/router
git commit -m "feat(companion): record sign-in, task and contest events"
```

---

### Task 6: AC and Achievement Hooks

**Files:**
- Modify: `apps/server/internal/gamification/service.go`
- Modify: `apps/server/internal/gamification/achievement.go`
- Modify: `apps/server/cmd/server/main.go`
- Test: `apps/server/internal/gamification/achievement_test.go`

**Interfaces:**
- `gamification` adds `type BondRecorder interface { RecordAchievement(ctx context.Context, userID, achievementID int64) error }`.
- `ServiceOptions` adds `BondRecorder BondRecorder`; `Service` stores it.
- `EvaluateUser` captures `unlocked` from `GrantAchievement`; when true, calls `BondRecorder` and logs errors without failing evaluation.
- `main.go` constructs companion before `gameSvc`, passes it as `BondRecorder`, and appends it to `gameHandlers` after `learningSvc`.

- [ ] **Step 1: Write failing achievement hook test**

Create an achievement with a simple condition, call `EvaluateUser`, assert `companion_events` contains `achievement:{id}`; call `EvaluateUser` again and assert no duplicate. Add a recorder stub that returns an error and assert `EvaluateUser` still returns nil and unlocks the achievement.

- [ ] **Step 2: Run to verify RED**  
`go test ./internal/gamification -run TestAchievementCompanion -count=1 -p 1`

- [ ] **Step 3: Implement the recorder and gamequeue chain**

Capture the bool in `achievement.go`, call the recorder best-effort. In `main.go`, append `companionSvc` to `gameHandlers` and set `gamification.ServiceOptions{BondRecorder: companionSvc}`.

- [ ] **Step 4: Run tests**  
`go test ./internal/gamification ./internal/companion -count=1 -p 1`

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/gamification apps/server/cmd/server
git commit -m "feat(companion): record achievement and AC events"
```

---

### Task 7: Site Admin Companion Settings

**Files:**
- Modify: `apps/server/internal/site/model.go`
- Modify: `apps/server/internal/site/dto.go`
- Modify: `apps/server/internal/site/repository.go`
- Modify: `apps/server/internal/site/service.go`
- Create: `apps/server/internal/site/companion.go`
- Modify: `apps/server/internal/siteapi/handler.go`
- Test: `apps/server/internal/site/service_test.go`
- Test: `apps/server/internal/siteapi/handler_test.go`

**Interfaces:**
- `CompanionSettings{Enabled bool, Name string, DefaultActivityLevel string, ContestMute bool, AllowUserDisable bool}`.
- `site.Settings` adds `Companion dbtypes.JSON[CompanionSettings]` column `companion_json`.
- `PublicView`/`AdminView` expose `companion`.
- `site.Service.UpdateCompanion(ctx, actorID, in) (*AdminView, error)`.
- Routes: `GET/PUT /api/admin/site/companion`.

- [ ] **Step 1: Write failing tests**

Defaults: enabled true, name "Kapo", defaultActivityLevel "normal", contestMute true, allowUserDisable true. PUT persists; invalid activity/name returns validation error; public `/api/site` includes the safe companion defaults. Existing rows with empty `companion_json` fall back to defaults.

- [ ] **Step 2: Run to verify RED**  
`go test ./internal/site ./internal/siteapi -count=1 -p 1`

- [ ] **Step 3: Implement model, defaults, validation, routes**

Add the JSONB column and `"companion_json"` to the repository `Updates` map. Add defaults in `ensureSettings` and in `view` when the JSON is empty. Validate: name 1–20 runes, activity in `quiet|normal|lively`. Add `recordAudit` on PUT.

- [ ] **Step 4: Run tests**  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/site apps/server/internal/siteapi
git commit -m "feat(companion): add admin companion defaults"
```

---

### Task 8: OpenAPI, Generated Types and Account Backup

**Files:**
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts` (the `@kapoj/api` gen script output)
- Modify: `apps/server/internal/backup/account.go`
- Test: `apps/server/internal/backup/account_test.go`

**Interfaces:**
- Add paths/schemas for companion state, events, settings, pet, and admin site companion settings.
- Add `companion_states` and `companion_events` to `accountTables` and the restore `deleteStatements` list.

- [ ] **Step 1: Update OpenAPI and run generation**

Add schemas mirroring the DTOs; run `corepack pnpm gen:api`. Confirm generated types include `CompanionState`, `CompanionEvent`, `CompanionSettingsInput`, `SiteCompanionSettings`.

- [ ] **Step 2: Write failing backup test**

Create a user with a companion state/event, export the account, assert both CSVs exist; restore into a fresh user and assert the rows are recreated, and the target's old companion rows are removed first.

- [ ] **Step 3: Implement backup tables and run tests**

`go test ./internal/backup -count=1 -p 1`

- [ ] **Step 4: Commit**

```bash
git add packages/api apps/server/internal/backup
git commit -m "feat(companion): expose API types and account backup"
```

---

### Task 9: Frontend API, Store Sync and Event Polling

**Files:**
- Create: `apps/web/src/api/companion.ts`
- Modify: `apps/web/src/api/types.ts` if needed
- Modify: `apps/web/src/stores/companion.ts`
- Modify: `apps/web/src/stores/site.ts`
- Create: `apps/web/src/composables/useCompanionSync.ts`
- Modify: `apps/web/src/layouts/DefaultLayout.vue`
- Test: `apps/web/src/stores/companion.spec.ts`
- Test: `apps/web/src/composables/useCompanionSync.spec.ts`

**Interfaces:**
- API: `getCompanion()`, `listCompanionEvents()`, `markCompanionEventsRead(ids)`, `updateCompanionSettings(input)`, `petCompanion()`.
- Site store exposes `companion` defaults from `SitePublicView.companion` and passes them to `useCompanionStore.applyGlobalConfig()`.
- Store adds `bond`, `level`, `levelName`, `nextThreshold`, `dailyGain`, `dailyCap`, `unreadCount`, `latestEventLine`, `globalEnabled`, `applyRemoteState`, `applyGlobalConfig`, `setLatestEventLine`.
- `useCompanionSync()` calls `getCompanion` on login, polls unread events every 60s while visible, marks them read, and sets the latest line. It never throws.

- [ ] **Step 1: Write failing store/sync tests**

Mock the API. Assert remote state applies, unread events mark read, poll stops on unmount, and a failed request leaves local state intact. Test `globalEnabled=false` hides the effective companion.

- [ ] **Step 2: Run to verify RED**  
`corepack pnpm test -- src/stores/companion.spec.ts src/composables/useCompanionSync.spec.ts`

- [ ] **Step 3: Implement API/store/sync**

Keep the existing local `load()` and setters. Add remote fields; do not remove local fallback. Use `useAuthStore` in the composable; call it from `DefaultLayout` setup. Poll only when `document.visibilityState === 'visible'` and logged in.

- [ ] **Step 4: Run tests**  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/api apps/web/src/stores/companion.ts apps/web/src/composables apps/web/src/layouts/DefaultLayout.vue apps/web/src/stores/companion.spec.ts apps/web/src/composables/useCompanionSync.spec.ts
git commit -m "feat(companion): sync state and poll events"
```

---

### Task 10: Dock Bond Meter, Pet and Settings Sync

**Files:**
- Create: `apps/web/src/components/companion/CompanionBondMeter.vue`
- Modify: `apps/web/src/components/companion/CompanionDock.vue`
- Modify: `apps/web/src/components/companion/CompanionSettings.vue`
- Modify: `apps/web/src/components/companion/CompanionPageNote.vue`
- Modify: `apps/web/src/views/HomeView.vue`
- Modify: `apps/web/src/components/auth/AuthLayout.vue`
- Modify: `apps/web/src/components/ui/AppEmptyState.vue` / `AppLoadingState.vue` / `AppErrorState.vue`
- Test: corresponding specs.

**Interfaces:**
- `CompanionBondMeter` props `bond`, `level`, `levelName`, `nextThreshold`, `dailyGain`, `dailyCap`.
- Dock shows the meter when `bond > 0`, a `.companion-dock__pet` button, and `store.latestEventLine` in the bubble.
- Settings calls `updateCompanionSettings` when logged in; adds a `publicProfile` toggle.
- All visibility gates use `store.enabled && store.globalEnabled`.

- [ ] **Step 1: Write failing component tests**

Meter renders `Lv.3 搭档 120/170`; pet calls API and updates store; settings persists remotely; global disable hides dock/note/home/auth mascots.

- [ ] **Step 2: Run to verify RED**  
`corepack pnpm test -- src/components/companion`

- [ ] **Step 3: Implement UI and gates**

Use the existing tokens and `CompanionAvatar`. Keep the collapsed dock 56px; add the meter inside the expanded panel. `petCompanion()` is disabled while `dailyGain >= dailyCap`? No, pet is still allowed but bond may be 0; show the response line.

- [ ] **Step 4: Run tests**  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components apps/web/src/views/HomeView.vue
git commit -m "feat(companion): add bond meter, pet and remote settings"
```

---

### Task 11: Admin Companion Settings UI

**Files:**
- Modify: `apps/web/src/api/site.ts`
- Modify: `apps/web/src/views/AdminSiteView.vue`
- Test: `apps/web/src/views/AdminSiteView.spec.ts`

**Interfaces:**
- Add a "伙伴设置" card with enabled toggle, name input, default activity select, contest mute toggle, allow-user-disable toggle, save button.
- Use `getSiteAdmin`/`saveAdminSiteCompanion` (or the generated API client) and the existing AdminSiteView save pattern.

- [ ] **Step 1: Write failing UI test**

Mount with a site admin payload containing companion defaults; assert the fields render; change enabled/name/activity and save; assert the API call payload.

- [ ] **Step 2: Run to verify RED**  
`corepack pnpm test -- src/views/AdminSiteView.spec.ts`

- [ ] **Step 3: Implement the card**

Follow the existing `AdminSiteView` card/form patterns; use `Message` for validation errors. Do not add new dependencies.

- [ ] **Step 4: Run tests**  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/AdminSiteView.vue apps/web/src/views/AdminSiteView.spec.ts
git commit -m "feat(companion): add admin companion settings UI"
```

---

### Task 12: E2E Smoke and Final Verification

**Files:**
- Create: `apps/web/e2e/companion-phase2.spec.ts`

**Interfaces:**
- E2E: login, open dock, assert bond meter/level; click pet and assert the bubble changes; open appearance, toggle quiet, reload, assert persisted; admin site page shows the companion card.

- [ ] **Step 1: Write the E2E spec**

Use the existing `m2bsolver1` / `m2badmin` credentials and login helper. The pet assertion should allow `awarded=false` if already petted today; assert the UI line changes or shows the daily limit message.

- [ ] **Step 2: Run the focused E2E if the backend is available**

`E2E_API_URL=http://127.0.0.1:18080 corepack pnpm test:e2e -- e2e/companion-phase2.spec.ts`

If the backend is unavailable, record the health-check failure; do not weaken global setup.

- [ ] **Step 3: Run all verification**

From `apps/server` with the test DB:
```bash
gofmt -l .
go vet ./...
go test -p 1 -count=1 ./...
```
From `apps/web`:
```bash
corepack pnpm test
corepack pnpm lint
corepack pnpm build
```

- [ ] **Step 4: Commit**

```bash
git add apps/web/e2e/companion-phase2.spec.ts
git commit -m "test(companion): add phase 2 browser smoke tests"
```

---

## Self-Review Notes

- Spec coverage: state/level, event keys, daily caps, sign-in/AC/task/achievement/contest/pet, settings sync, admin defaults, event feed, frontend dock meter and admin UI all have tasks.
- No skins, shop, profile archive or AI persona are included.
- Type consistency: `EventInput`, `StateView`, `EventView`, `SettingsInput`, `PetResult`, `CompanionSettings` are used across tasks.
- Review Focus items map to Tasks 2, 3, 5/6, 8 and 10.
