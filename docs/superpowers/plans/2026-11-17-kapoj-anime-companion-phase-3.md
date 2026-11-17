# KapOJ Anime Companion Phase 3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the companion visible beyond the dock: a privacy-aware companion card on user profiles, and the companion as the presentation layer for AI hints/code actions.

**Architecture:** The backend exposes a read-only public companion summary (`GET /api/users/:id/companion`) that is visible when `publicProfile` is true or the viewer is the owner. The profile page renders a `CompanionProfileCard` below the existing hero/word cloud. AI surfaces reuse `CompanionAvatar` and the store name without changing prompts, caches, or AI contracts.

**Tech Stack:** Go + Gin + GORM, Vue 3 + Pinia + PrimeVue, Vitest, Playwright.

**Spec:** `docs/superpowers/specs/2026-11-16-kapoj-anime-companion-design.md` (Phase 3 sections).

## Global Constraints

- Phase 3 only: profile companion card + AI presentation. No prompt/cache changes, no skins/shop, no new artwork requirements.
- Do not add npm or Go dependencies.
- Preserve the profile hero's evaluation word cloud; the companion card goes below it, never replaces it.
- Privacy: a non-owner sees the companion summary only when `publicProfile` is true; otherwise 404. The owner always sees their own summary, including a "仅自己可见" hint.
- All companion surfaces respect `effectiveEnabled` (local opt-out + global switch).
- Character copy stays Chinese, tsundere-safe, ≤40 characters.
- Keep Phase 0–2 behavior when the API fails; profile page still renders without the companion card.
- Backend tests use `kapoj_test`, `-p 1`; frontend commands from `apps/web`.

## Review Focus

- **Privacy leak** → a private profile must not expose bond/level to guests or other users; owner still sees it. Tested in Tasks 1/2.
- **Word-cloud regression** → the profile card must not displace or hide the evaluation word cloud. Tested in Task 2.
- **Global/local disable** → AI avatars and profile card disappear when the companion is disabled. Tested in Tasks 2/3.
- **AI contract drift** → no request/response/prompt/cache changes. Tested in Task 3.
- **Profile API failure** → profile page still renders; no unhandled rejection. Tested in Task 2.

---

### Task 1: Public Companion Summary API

**Files:**
- Modify: `apps/server/internal/companion/repository.go` + `repository_test.go`
- Modify: `apps/server/internal/companion/service.go` + `service_test.go`
- Modify: `apps/server/internal/companion/dto.go`
- Modify: `apps/server/internal/companionapi/handler.go` + `handler_test.go`
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`

**Interfaces:**
- `Repository.GetState(ctx, userID) (*State, error)` — non-creating; returns `gorm.ErrRecordNotFound` when absent.
- `Service.PublicState(ctx, userID, viewerID int64) (*PublicStateView, error)`.
- `PublicStateView{UserID, Bond, Level, LevelName, NextThreshold, EquippedSkin, UnlockedSkins, PublicProfile, DaysTogether}`.
- Route `GET /api/users/:id/companion` on the optional-auth `api` group; no `RequireAuth`.

- [ ] **Step 1: Write failing repository/service tests**

Cover: missing state returns not found; private state returns not found for another viewer; owner gets it; public state returns for guests; `DaysTogether` is at least 1 and stable for a fixed `time.Local`.

- [ ] **Step 2: Run RED**  
`go test ./internal/companion -run 'TestGetState|TestPublicState' -count=1 -p 1`

- [ ] **Step 3: Implement GetState + PublicState + DTO**

`GetState` uses `First` without `EnsureState`. `PublicState`:
```go
st, err := s.repo.GetState(ctx, userID)
if err != nil { return nil, err }
if !st.PublicProfile && viewerID != userID { return nil, httpx.ErrNotFound }
days := int(time.Since(st.CreatedAt.In(time.Local)).Hours()/24) + 1
if days < 1 { days = 1 }
return &PublicStateView{...}, nil
```
Use `time.Local` date arithmetic; no `time.Now().UTC()` date.

- [ ] **Step 4: Add handler and route**

`companionapi` adds:
```go
r.GET("/users/:id/companion", h.publicState)
```
Parse `:id` with `strconv.ParseInt` (>0). Read optional claims via `middleware.CurrentClaims`; viewerID=0 for guests. Return `httpx.OK`.

- [ ] **Step 5: OpenAPI + generate**

Add the path and `PublicStateView` schema; run `corepack pnpm gen:api`.

- [ ] **Step 6: Run tests**  
`go test ./internal/companion ./internal/companionapi ./internal/router -count=1 -p 1`

- [ ] **Step 7: Commit**

```bash
git add apps/server/internal/companion apps/server/internal/companionapi packages/api apps/web/src/api/schema.d.ts
git commit -m "feat(companion): expose public profile summary"
```

---

### Task 2: Profile Companion Card

**Files:**
- Create: `apps/web/src/components/companion/CompanionProfileCard.vue` + spec
- Modify: `apps/web/src/api/companion.ts`
- Modify: `apps/web/src/views/UserProfileView.vue` + spec
- Modify: `apps/web/src/api/types.ts` if needed

**Interfaces:**
- `getUserCompanion(userID: number): Promise<UserCompanion>`.
- Props: `state: UserCompanion`, `isSelf: boolean`, `name: string`.
- The card renders `CompanionAvatar pose="bust" size="md"`, level name, bond progress (`bond / nextThreshold`), days together, unlocked skins (empty state: "还没有换装"), and a "仅自己可见" hint when `isSelf && !state.publicProfile`.
- `UserProfileView` loads it after the profile; 404/error => card hidden; no page error.

- [ ] **Step 1: Write failing component/profile tests**

Component: renders level/bond/days/private hint; hides when no state. Profile: card appears below the hero and the word cloud still exists; a 404 keeps the card hidden; disabled companion hides the card.

- [ ] **Step 2: Run RED**  
`corepack pnpm test -- src/components/companion/CompanionProfileCard.spec.ts src/views/UserProfileView.spec.ts`

- [ ] **Step 3: Implement API + card + profile integration**

Place the card between `</section>` (hero) and the `profile-stats` `<dl>` so the word cloud stays inside the hero. Use `store.effectiveEnabled`; if false, do not fetch/render.

- [ ] **Step 4: Run tests**  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/companion apps/web/src/api/companion.ts apps/web/src/views/UserProfileView.vue apps/web/src/views/UserProfileView.spec.ts
git commit -m "feat(companion): add profile companion card"
```

---

### Task 3: AI Presentation Layer

**Files:**
- Modify: `apps/web/src/views/ProblemDetailView.vue` + spec
- Modify: `apps/web/src/views/SubmitView.vue` + spec

**Interfaces:**
- Problem hint header uses `CompanionAvatar pose="chibi" size="xs"` + `{{ store.name }} 的提示` when `store.effectiveEnabled`.
- Loading text `{{ store.name }} 正在生成提示…` when enabled, else existing copy.
- Submit AI toolbar adds a 24px chibi before the three AI buttons when enabled; labels/contracts unchanged.
- No API/prompt/cache changes.

- [ ] **Step 1: Write failing tests**

ProblemDetail: with companion enabled, the hint header contains the avatar and `Kapo 的提示`; when disabled, no avatar and the neutral copy. Submit: AI toolbar has the avatar when enabled, absent when disabled, and the three buttons still call the same handlers.

- [ ] **Step 2: Run RED**  
`corepack pnpm test -- src/views/ProblemDetailView.spec.ts src/views/SubmitView.spec.ts`

- [ ] **Step 3: Implement presentation only**

Use the store loaded by `App.vue`; call `store.load()` if a spec mounts the view standalone. Do not modify `api/ai.ts`, `getAIHint`, or prompts.

- [ ] **Step 4: Run tests**  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/ProblemDetailView.vue apps/web/src/views/SubmitView.vue apps/web/src/views/ProblemDetailView.spec.ts apps/web/src/views/SubmitView.spec.ts
git commit -m "feat(companion): add AI presentation layer"
```

---

### Task 4: E2E and Final Verification

**Files:**
- Create: `apps/web/e2e/companion-phase3.spec.ts`

**Interfaces:**
- Login, fetch `/api/users/me` via `APIRequestContext`, visit `/users/{id}`, assert `.companion-profile` visible for self; assert the word cloud still exists.
- Set `publicProfile` true via API, then visit the profile as a guest in a fresh context and assert the card is visible; restore `publicProfile` false in `finally`.
- Do not run the AI flow (requires an admin AI key).

- [ ] **Step 1: Write the E2E spec**

Use the existing `m2bsolver1` login and API helpers. The self profile is visible even when private; the public check proves the privacy branch.

- [ ] **Step 2: Run the focused E2E against the Phase 3 backend**  
Use `E2E_API_URL=http://127.0.0.1:8080 E2E_BASE_URL=http://127.0.0.1:5173 corepack pnpm exec playwright test e2e/companion-phase3.spec.ts` (the worktree backend must be deployed first; if not, record the exact skip).

- [ ] **Step 3: Full verification**

From `apps/web`: `corepack pnpm test`, `corepack pnpm lint`, `corepack pnpm build`.
From `apps/server` with `kapoj_test`: `gofmt -l .`, `go vet ./...`, `go test -p 1 -count=1 ./internal/companion ./internal/companionapi ./internal/router`.

- [ ] **Step 4: Commit**

```bash
git add apps/web/e2e/companion-phase3.spec.ts
git commit -m "test(companion): add phase 3 profile smoke test"
```

---

## Self-Review Notes

- Spec coverage: public profile summary, privacy, profile card, word-cloud preservation, AI presentation, global/local disable, E2E.
- Skins/expression assets remain deferred until art exists; the state fields are already in place.
- Type consistency: `PublicStateView`, `UserCompanion`, `CompanionProfileCard`, `getUserCompanion`.
- Review Focus maps to Tasks 1/2/3.
