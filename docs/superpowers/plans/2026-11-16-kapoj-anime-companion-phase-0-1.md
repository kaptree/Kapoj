# KapOJ Anime Companion Phase 0–1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the KapOJ companion's visual language and character presence across the web app without backend changes: tokens, avatar/placeholder, dock, page notes, auth static art, home hero, 404/403, and local preferences.

**Architecture:** A small frontend-only companion module (asset manifest, lines, page-note map, Pinia store, composables) feeds presentational components. `DefaultLayout` mounts the logged-in dock and a route-keyed `CompanionPageNote`; auth pages get static art through `AuthLayout`. No backend, API, database or route changes.

**Tech Stack:** Vue 3, TypeScript, Pinia, PrimeVue 4, Vitest + Vue Test Utils, Playwright.

**Spec:** `docs/superpowers/specs/2026-11-16-kapoj-anime-companion-design.md`

## Global Constraints

- Scope is **Phase 0–1 only**. Do not add backend routes, tables, migrations, or API clients.
- Do not add npm dependencies. Use Vue, Pinia, PrimeVue, Vitest and existing utilities.
- Keep `--kapoj-primary` teal for buttons, links, focus and judge interaction. Crimson is only for the companion/brand accents and decorative labels.
- Danger/WA colors stay unchanged; never use crimson for validation errors or destructive actions.
- Every new color must exist in both dark (`:root`) and light (`:root:not(.app-dark)`) blocks in `apps/web/src/styles/tokens.css`.
- Character copy is Chinese, tsundere but never attacks the user; max 40 characters per line; failure lines must suggest a next action.
- The character is all-ages and non-sexualized. Placeholder art must not imply otherwise.
- Dock is logged-in only. No dock in `AdminLayout`; auth pages use static art only.
- Running contest (`contestId` + status `running`) hides the dock and page notes.
- Disabling the companion removes character elements but keeps the new visual language (crimson/silver tokens, manga labels, sticker cards).
- No layout shift: avatars have fixed `width`/`height` or `aspect-ratio`; notes have fixed min-height.
- Local storage keys: `kapoj.companion.prefs`, `kapoj.companion.notes`, `kapoj.companion.greeting`.
- Commands run from `apps/web`: `corepack pnpm test -- <path>`, `corepack pnpm lint`, `corepack pnpm build`.

## Review Focus

- **Corrupt or unavailable localStorage** → companion loads defaults and never crashes; next write repairs storage. Tested in Task 2.
- **Missing or broken mascot image** → `CompanionAvatar` shows the inline placeholder without layout jump. Tested in Task 3.
- **Contest API failure or rapid route change** → mute resolves to `false` on error and never stays stuck; leaving a contest restores the dock. Tested in Task 4.
- **Disabling the companion while dock is open, or visiting auth/admin pages** → no dock/note residue and no trapped focus. Tested in Tasks 6 and 7.
- **Reduced motion preference** → no transitions or idle effects; store override wins over system. Tested in Task 4.
- **Local-day rollover** → dismissed notes and daily greeting return on the next local day, not on a UTC boundary. Tested in Tasks 2 and 5.

---

### Task 1: Companion Tokens and MangaLabel

**Files:**
- Modify: `apps/web/src/styles/tokens.css`
- Create: `apps/web/src/styles/companion.css`
- Modify: `apps/web/src/main.ts`
- Create: `apps/web/src/components/companion/MangaLabel.vue`
- Test: `apps/web/src/styles/tokens.spec.ts`
- Test: `apps/web/src/components/companion/MangaLabel.spec.ts`

**Interfaces:**
- Produces: CSS variables `--kapoj-kapo`, `--kapoj-kapo-strong`, `--kapoj-kapo-text`, `--kapoj-kapo-soft`, `--kapoj-silver`, `--kapoj-silver-strong`, `--kapoj-ink`, `--kapoj-manga-line`, `--kapoj-manga-dot`, `--kapoj-sticker-shadow`.
- Produces: `<MangaLabel :index="'02'" label="学习概览" kana="学習記録" />`.

- [ ] **Step 1: Write the failing token test**

```ts
// apps/web/src/styles/tokens.spec.ts
import { describe, expect, it } from 'vitest'
import tokens from './tokens.css?raw'

const required = [
  '--kapoj-kapo:',
  '--kapoj-kapo-strong:',
  '--kapoj-kapo-text:',
  '--kapoj-kapo-soft:',
  '--kapoj-silver:',
  '--kapoj-silver-strong:',
  '--kapoj-ink:',
  '--kapoj-manga-line:',
  '--kapoj-manga-dot:',
  '--kapoj-sticker-shadow:',
]

describe('companion tokens', () => {
  it('defines every companion token in both themes', () => {
    for (const token of required) {
      expect(tokens.split(token).length - 1).toBeGreaterThanOrEqual(2)
    }
  })
})
```

- [ ] **Step 2: Run it to verify it fails**

Run: `corepack pnpm test -- src/styles/tokens.spec.ts`
Expected: FAIL because the tokens are missing.

- [ ] **Step 3: Add the dark tokens after `--kapoj-re` / `--kapoj-se`**

```css
--kapoj-kapo: #e23a4e;
--kapoj-kapo-strong: #ff5c70;
--kapoj-kapo-text: #ff7a88;
--kapoj-kapo-soft: rgb(226 58 78 / 12%);
--kapoj-silver: #c9d1dc;
--kapoj-silver-strong: #eef2f7;
--kapoj-ink: #0b0b10;
--kapoj-manga-line: rgb(238 242 247 / 18%);
--kapoj-manga-dot: rgb(226 58 78 / 14%);
--kapoj-sticker-shadow: 4px 4px 0 rgb(0 0 0 / 45%);
```

Add the light overrides inside `:root:not(.app-dark)`:

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

- [ ] **Step 4: Run the token test to verify it passes**

Run: `corepack pnpm test -- src/styles/tokens.spec.ts`
Expected: PASS.

- [ ] **Step 5: Write the failing MangaLabel test**

```vue
// apps/web/src/components/companion/MangaLabel.spec.ts
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import MangaLabel from './MangaLabel.vue'

describe('MangaLabel', () => {
  it('renders the index, label and decorative kana', () => {
    const wrapper = mount(MangaLabel, {
      props: { index: '02', label: '学习概览', kana: '学習記録' },
    })
    expect(wrapper.text()).toContain('02')
    expect(wrapper.text()).toContain('学习概览')
    expect(wrapper.find('.manga-label__kana').attributes('aria-hidden')).toBe('true')
  })
})
```

- [ ] **Step 6: Run it to verify it fails**

Run: `corepack pnpm test -- src/components/companion/MangaLabel.spec.ts`
Expected: FAIL because the component does not exist.

- [ ] **Step 7: Implement `MangaLabel.vue` and `companion.css`**

`MangaLabel.vue` props: `index?: string` (default `''`), `label: string`, `kana?: string` (default `''`). Render:

```vue
<p class="manga-label">
  <span v-if="index" class="manga-label__index">{{ index }}</span>
  <span class="manga-label__label">{{ label }}</span>
  <span v-if="kana" class="manga-label__kana" aria-hidden="true">{{ kana }}</span>
</p>
```

Styles: left 3px crimson bar, uppercase mono index, 600-weight label, muted kana, bottom hairline using `--kapoj-manga-line`.

Create `companion.css` with a `.companion-sticker` class:

```css
.companion-sticker {
  border: 1px solid var(--kapoj-manga-line);
  box-shadow: var(--kapoj-sticker-shadow);
}
```

Import `./styles/companion.css` in `main.ts` after `styles.css`.

- [ ] **Step 8: Run both tests**

Run: `corepack pnpm test -- src/styles/tokens.spec.ts src/components/companion/MangaLabel.spec.ts`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add apps/web/src/styles/tokens.css apps/web/src/styles/tokens.spec.ts apps/web/src/styles/companion.css apps/web/src/components/companion/MangaLabel.vue apps/web/src/components/companion/MangaLabel.spec.ts apps/web/src/main.ts
git commit -m "feat: add companion visual tokens and manga label"
```

---

### Task 2: Companion Store, Date Helper and Content

**Files:**
- Create: `apps/web/src/utils/date.ts`
- Create: `apps/web/src/stores/companion.ts`
- Create: `apps/web/src/content/companion/types.ts`
- Create: `apps/web/src/content/companion/lines.zh-CN.ts`
- Create: `apps/web/src/content/companion/pageNotes.ts`
- Test: `apps/web/src/utils/date.spec.ts`
- Test: `apps/web/src/stores/companion.spec.ts`
- Test: `apps/web/src/content/companion/lines.spec.ts`

**Interfaces:**
- Produces: `localDateKey(date?: Date): string` → `YYYY-MM-DD` in local time.
- Produces: `useCompanionStore()` with `load()`, `setEnabled(value: boolean)`, `setActivityLevel(value: CompanionActivityLevel)`, `setMotion(value: CompanionMotionPreference)`, `reset()`, `dismissNote(key: string, date?: string)`, `isNoteDismissed(key: string, date?: string): boolean`, `shouldGreet(date?: string): boolean`, `markGreeted(date?: string)`, and refs `enabled`, `activityLevel`, `motion`, `loaded`.
- Produces: `CompanionPose = 'chibi' | 'bust' | 'full'` and `CompanionExpression` with `neutral | smug | pout | blush | surprised | annoyed | thinking | sleepy | victory | panic | lost`.
- Produces: `CompanionLineKey`, `COMPANION_LINES`, `pickCompanionLine(key, seed = ''): string`.
- Produces: `COMPANION_PAGE_NOTES: Record<string, { lineKey: CompanionLineKey; expression: CompanionExpression }>`.
- Produces: storage keys `COMPANION_PREFS_KEY`, `COMPANION_NOTES_KEY`, `COMPANION_GREETING_KEY`.

- [ ] **Step 1: Write the failing date test**

```ts
import { describe, expect, it } from 'vitest'
import { localDateKey } from './date'

describe('localDateKey', () => {
  it('uses local calendar fields, not UTC', () => {
    const date = new Date(2026, 10, 16, 23, 30)
    expect(localDateKey(date)).toBe('2026-11-16')
  })
})
```

- [ ] **Step 2: Run it to verify it fails**

Run: `corepack pnpm test -- src/utils/date.spec.ts`
Expected: FAIL because the module does not exist.

- [ ] **Step 3: Implement `localDateKey`**

```ts
export function localDateKey(date = new Date()): string {
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${date.getFullYear()}-${month}-${day}`
}
```

- [ ] **Step 4: Write the failing store test**

```ts
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import {
  COMPANION_NOTES_KEY,
  COMPANION_PREFS_KEY,
  useCompanionStore,
} from './companion'

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})

describe('companion store', () => {
  it('defaults to enabled, normal activity and system motion', () => {
    const store = useCompanionStore()
    store.load()
    expect(store.enabled).toBe(true)
    expect(store.activityLevel).toBe('normal')
    expect(store.motion).toBe('system')
  })

  it('persists preferences and tolerates corrupt storage', () => {
    localStorage.setItem(COMPANION_PREFS_KEY, '{not json')
    const store = useCompanionStore()
    store.load()
    store.setEnabled(false)
    store.setActivityLevel('quiet')
    store.setMotion('reduce')
    const saved = JSON.parse(localStorage.getItem(COMPANION_PREFS_KEY)!)
    expect(saved).toEqual({ enabled: false, activityLevel: 'quiet', motion: 'reduce' })
  })

  it('dismisses a note only for the given local day', () => {
    const store = useCompanionStore()
    store.load()
    store.dismissNote('problems', '2026-11-16')
    expect(store.isNoteDismissed('problems', '2026-11-16')).toBe(true)
    expect(store.isNoteDismissed('problems', '2026-11-17')).toBe(false)
    expect(localStorage.getItem(COMPANION_NOTES_KEY)).toContain('problems')
  })

  it('greets once per local day', () => {
    const store = useCompanionStore()
    store.load()
    expect(store.shouldGreet('2026-11-16')).toBe(true)
    store.markGreeted('2026-11-16')
    expect(store.shouldGreet('2026-11-16')).toBe(false)
    expect(store.shouldGreet('2026-11-17')).toBe(true)
  })
})
```

- [ ] **Step 5: Run it to verify it fails**

Run: `corepack pnpm test -- src/stores/companion.spec.ts`
Expected: FAIL because the store does not exist.

- [ ] **Step 6: Implement the store**

Use `defineStore('companion', () => { ... })`. `load()` is idempotent, wraps every `JSON.parse` in `try/catch`, and writes defaults back when storage is corrupt. `reset()` restores defaults and clears the three keys. `dismissNote` stores `{ [key]: date }`. `set*` methods call `persistPrefs()`.

- [ ] **Step 7: Write the failing lines test**

```ts
import { describe, expect, it } from 'vitest'
import { COMPANION_LINES, pickCompanionLine } from './lines.zh-CN'

describe('companion lines', () => {
  it('is deterministic for the same key and day', () => {
    const first = pickCompanionLine('greeting', '2026-11-16')
    const second = pickCompanionLine('greeting', '2026-11-16')
    expect(first).toBe(second)
    expect(COMPANION_LINES.greeting).toContain(first)
  })

  it('keeps every line short enough for a bubble', () => {
    for (const lines of Object.values(COMPANION_LINES)) {
      for (const line of lines) expect(Array.from(line).length).toBeLessThanOrEqual(40)
    }
  })
})
```

- [ ] **Step 8: Run it to verify it fails**

Run: `corepack pnpm test -- src/content/companion/lines.spec.ts`
Expected: FAIL because the module does not exist.

- [ ] **Step 9: Implement types, lines and page notes**

Create `types.ts` first:

```ts
export type CompanionPose = 'chibi' | 'bust' | 'full'
export type CompanionExpression =
  | 'neutral' | 'smug' | 'pout' | 'blush' | 'surprised'
  | 'annoyed' | 'thinking' | 'sleepy' | 'victory' | 'panic' | 'lost'
export interface CompanionAsset { src: string; width: number; height: number }
export type CompanionAssetPack = Partial<Record<CompanionPose, Partial<Record<CompanionExpression, CompanionAsset>>>>
```

`lines.zh-CN.ts` exports these exact keys with at least the shown copy:

```ts
export type CompanionLineKey =
  | 'greeting' | 'signin' | 'ac' | 'wa' | 'ce' | 'tle' | 're' | 'empty' | 'lost'
  | 'note.problems' | 'note.problem' | 'note.submit' | 'note.solutions'
  | 'note.contests' | 'note.contest' | 'note.submissions' | 'note.leaderboard'
  | 'note.learning' | 'note.tasks' | 'note.achievements' | 'note.shop'
  | 'note.notifications' | 'note.certificate' | 'note.profile' | 'note.appearance'
  | 'note.statistics'

export const COMPANION_LINES: Record<CompanionLineKey, readonly string[]> = {
  greeting: ['又来了？……题可不会自己变简单。', '别误会，我只是刚好在看你提交。'],
  signin: ['签到了。今天也别让我记太多 WA。', '嗯，签到。……别以为我在等你。'],
  ac: ['过了。下一题，别得意。', '……不错。我说了不错，别让我重复。'],
  wa: ['第 3 个点。别看我，看代码。', '又 WA 了？……再读一遍题面。'],
  ce: ['编译器都看不下去了。', '先让它编译通过。……我等你。'],
  tle: ['跑得比我还慢。', '超时了。……优化一下。'],
  re: ['数组越界？……真拿你没办法。', '运行时崩了。检查边界。'],
  empty: ['这里空空的。……要一起填满吗？'],
  lost: ['这里什么都没有。……跟着我。'],
  'note.problems': ['今天的题……自己挑。别贪多。'],
  'note.problem': ['先读题。别急着写。'],
  'note.submit': ['代码写完再看一眼。……我可不帮你 debug。'],
  'note.solutions': ['看题解可以，别直接抄。'],
  'note.contests': ['比赛很多。选一个，认真打。'],
  'note.contest': ['开始了就专注点。……我在外面等。'],
  'note.submissions': ['记录都在这里。……别翻太多。'],
  'note.leaderboard': ['想上榜？先 AC 一道。'],
  'note.learning': ['路线在这。走不走随你。'],
  'note.tasks': ['任务刷新了。做不做随你。'],
  'note.achievements': ['还差几个。……我记着。'],
  'note.shop': ['看看就好，别乱花。'],
  'note.notifications': ['有人找你。……去看。'],
  'note.certificate': ['证书这东西……自己收好。'],
  'note.profile': ['你的主页。……还行。'],
  'note.appearance': ['换装？……先把我放出来。'],
  'note.statistics': ['数据不会骗人。……你自己看。'],
}
```

`pickCompanionLine` hashes `${key}:${seed}` into the array index with a small deterministic hash.

`pageNotes.ts` maps route names exactly: `problems`, `problem-detail`, `problem-submit`, `problem-official-solution`, `problem-solutions`, `problem-solution-detail`, `problem-statistics`, `problem-contests`, `contests`, `contest-detail`, `submissions`, `leaderboard`, `learning-paths`, `learning-path-detail`, `tasks`, `achievements`, `badge-shop`, `title-shop`, `notifications`, `certificate-verify`, `certificate-settings`, `user-profile`, `appearance`. Import `CompanionExpression` from `./types` (created in this task).

- [ ] **Step 10: Run the three tests**

Run: `corepack pnpm test -- src/utils/date.spec.ts src/stores/companion.spec.ts src/content/companion/lines.spec.ts`
Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add apps/web/src/utils/date.ts apps/web/src/utils/date.spec.ts apps/web/src/stores/companion.ts apps/web/src/stores/companion.spec.ts apps/web/src/content/companion
git commit -m "feat: add companion store, types, lines and page note map"
```

---

### Task 3: Asset Manifest, Avatar and Bubble

**Files:**
- Create: `apps/web/src/content/companion/manifest.ts`
- Create: `apps/web/src/components/companion/CompanionAvatar.vue`
- Create: `apps/web/src/components/companion/CompanionBubble.vue`
- Test: `apps/web/src/content/companion/manifest.spec.ts`
- Test: `apps/web/src/components/companion/CompanionAvatar.spec.ts`
- Test: `apps/web/src/components/companion/CompanionBubble.spec.ts`

**Interfaces:**
- Consumes: `CompanionPose`, `CompanionExpression`, `CompanionAsset`, `CompanionAssetPack` from `src/content/companion/types.ts` (Task 2).
- Produces: `resolveCompanionAsset(pose, expression, pack?): CompanionAsset | null`.
- Produces: `<CompanionAvatar pose?, expression?, size?, eager?, alt? />` with sizes `xs | sm | md | lg`.
- Produces: `<CompanionBubble text, tone? />` with tone `tsun | neutral`.

- [ ] **Step 1: Write the failing manifest test**

```ts
import { describe, expect, it } from 'vitest'
import { resolveCompanionAsset } from './manifest'
import type { CompanionAssetPack } from './types'

const pack: CompanionAssetPack = {
  chibi: {
    neutral: { src: '/mascot/chibi/neutral@2x.webp', width: 512, height: 512 },
    pout: { src: '/mascot/chibi/pout@2x.webp', width: 512, height: 512 },
  },
}

describe('resolveCompanionAsset', () => {
  it('falls back to neutral for an unknown expression', () => {
    expect(resolveCompanionAsset('chibi', 'panic', pack)?.src).toContain('neutral')
  })

  it('returns null when the pose is missing', () => {
    expect(resolveCompanionAsset('full', 'neutral', pack)).toBeNull()
  })
})
```

- [ ] **Step 2: Run it to verify it fails**

Run: `corepack pnpm test -- src/content/companion/manifest.spec.ts`
Expected: FAIL because the module does not exist.

- [ ] **Step 3: Implement the manifest**

`COMPANION_ASSETS` is an empty typed pack and `COMPANION_PACK_AVAILABLE = false` until the user supplies art. `resolveCompanionAsset` returns `null` when the pack is empty, otherwise `pack[pose]?.[expression] ?? pack[pose]?.neutral ?? null`.

- [ ] **Step 4: Write the failing avatar test**

```ts
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import CompanionAvatar from './CompanionAvatar.vue'

describe('CompanionAvatar', () => {
  it('renders the inline placeholder when no art pack is available', () => {
    const wrapper = mount(CompanionAvatar, { props: { pose: 'bust', expression: 'pout' } })
    expect(wrapper.find('[data-testid="companion-placeholder"]').exists()).toBe(true)
    expect(wrapper.find('img').exists()).toBe(false)
  })

  it('applies pose and size classes', () => {
    const wrapper = mount(CompanionAvatar, {
      props: { pose: 'full', size: 'lg', expression: 'smug' },
    })
    expect(wrapper.classes()).toContain('companion-avatar--full')
    expect(wrapper.classes()).toContain('companion-avatar--lg')
  })
})
```

- [ ] **Step 5: Run it to verify it fails**

Run: `corepack pnpm test -- src/components/companion/CompanionAvatar.spec.ts`
Expected: FAIL because the component does not exist.

- [ ] **Step 6: Implement `CompanionAvatar.vue`**

Props with defaults: `pose: 'chibi'`, `expression: 'neutral'`, `size: 'md'`, `eager: false`, `alt: ''`. Compute `asset = resolveCompanionAsset(pose, expression)`. Use an internal `failed` ref; reset it when `asset` changes. Render `<img>` only when `asset && !failed`, with `:src`, `:width`, `:height`, `:alt`, `:loading="eager ? 'eager' : 'lazy'"`, `decoding="async"`, and `@error="failed = true"`.

Placeholder is inline SVG with `data-testid="companion-placeholder"`: a silver head circle, ink shoulders, two crimson eye dots, `aria-hidden="true"`, `focusable="false"`. Use `aspect-ratio` per pose (`chibi` 1/1, `bust` 6/7, `full` 2/3) and fixed size classes:

```css
.companion-avatar--xs { width: 24px; }
.companion-avatar--sm { width: 44px; }
.companion-avatar--md { width: 96px; }
.companion-avatar--lg { width: min(240px, 40vw); }
```

- [ ] **Step 7: Write the failing bubble test**

```ts
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import CompanionBubble from './CompanionBubble.vue'

describe('CompanionBubble', () => {
  it('renders the line and tone', () => {
    const wrapper = mount(CompanionBubble, { props: { text: '又 WA 了？', tone: 'tsun' } })
    expect(wrapper.text()).toContain('又 WA 了？')
    expect(wrapper.classes()).toContain('companion-bubble--tsun')
  })
})
```

- [ ] **Step 8: Implement `CompanionBubble.vue`**

Props: `text: string`, `tone: 'tsun' | 'neutral'` default `'tsun'`. Render a `<p class="companion-bubble">` with a small tail pseudo-element. Silver background, 1px manga line, text color `--kapoj-text`, max-width 34ch.

- [ ] **Step 9: Run the three test files**

Run: `corepack pnpm test -- src/content/companion/manifest.spec.ts src/components/companion/CompanionAvatar.spec.ts src/components/companion/CompanionBubble.spec.ts`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add apps/web/src/content/companion/manifest.ts apps/web/src/content/companion/manifest.spec.ts apps/web/src/components/companion/CompanionAvatar.vue apps/web/src/components/companion/CompanionAvatar.spec.ts apps/web/src/components/companion/CompanionBubble.vue apps/web/src/components/companion/CompanionBubble.spec.ts
git commit -m "feat: add companion avatar, bubble and asset manifest"
```

---

### Task 4: Reduced Motion and Contest Mute

**Files:**
- Create: `apps/web/src/composables/useReducedMotion.ts`
- Create: `apps/web/src/composables/useCompanionMute.ts`
- Test: `apps/web/src/composables/useReducedMotion.spec.ts`
- Test: `apps/web/src/composables/useCompanionMute.spec.ts`

**Interfaces:**
- Produces: `useReducedMotion(): ComputedRef<boolean>`; true when the store motion is `reduce` or the OS media query matches.
- Produces: `useCompanionMute(): { muted: ComputedRef<boolean>; checking: Ref<boolean> }`.
- Produces: `clearCompanionMuteCache(): void` for tests.

- [ ] **Step 1: Write the failing reduced-motion test**

```ts
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useCompanionStore } from '@/stores/companion'
import { useReducedMotion } from './useReducedMotion'

let result: ReturnType<typeof useReducedMotion>

function mountComposable() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const Comp = defineComponent({
    setup() {
      result = useReducedMotion()
      return () => null
    },
  })
  return mount(Comp, { global: { plugins: [pinia] } })
}

beforeEach(() => {
  vi.mocked(window.matchMedia).mockReturnValue({
    matches: false, media: '', onchange: null,
    addListener: () => undefined, removeListener: () => undefined,
    addEventListener: () => undefined, removeEventListener: () => undefined,
    dispatchEvent: () => false,
  } as MediaQueryList)
})

describe('useReducedMotion', () => {
  it('honors the store override and the system preference', async () => {
    mountComposable()
    const store = useCompanionStore()
    expect(result.value).toBe(false)
    store.setMotion('reduce')
    await Promise.resolve()
    expect(result.value).toBe(true)
  })
})
```

- [ ] **Step 2: Run it to verify it fails**

Run: `corepack pnpm test -- src/composables/useReducedMotion.spec.ts`
Expected: FAIL because the composable does not exist.

- [ ] **Step 3: Implement `useReducedMotion`**

Read `window.matchMedia('(prefers-reduced-motion: reduce)')` in a `ref`, subscribe with `addEventListener` when available, and clean up with `onScopeDispose` only when `getCurrentScope()` is set (so the composable also works outside a component in tests). Return `computed(() => store.motion === 'reduce' || media.matches)`.

- [ ] **Step 4: Write the failing mute test**

```ts
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getContest } from '@/api/contests'
import { clearCompanionMuteCache, useCompanionMute } from './useCompanionMute'

vi.mock('@/api/contests', () => ({ getContest: vi.fn() }))

const routes = [
  { path: '/contests/:id(\\d+)', name: 'contest-detail', component: { template: '<div />' } },
  { path: '/problems/:id(\\d+)', name: 'problem-detail', component: { template: '<div />' } },
]

async function mountAt(path: string) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  let result!: ReturnType<typeof useCompanionMute>
  const Comp = defineComponent({
    setup() {
      result = useCompanionMute()
      return () => null
    },
  })
  mount(Comp, { global: { plugins: [router] } })
  await flushPromises()
  return result
}

beforeEach(() => {
  clearCompanionMuteCache()
  vi.mocked(getContest).mockReset()
})

describe('useCompanionMute', () => {
  it('mutes a running contest detail', async () => {
    vi.mocked(getContest).mockResolvedValue({ status: 'running' } as never)
    const { muted } = await mountAt('/contests/7')
    expect(muted.value).toBe(true)
  })

  it('mutes a problem opened with a running contest query', async () => {
    vi.mocked(getContest).mockResolvedValue({ status: 'running' } as never)
    const { muted } = await mountAt('/problems/3?contestId=7')
    expect(muted.value).toBe(true)
  })

  it('does not stay muted when the contest request fails', async () => {
    vi.mocked(getContest).mockRejectedValue(new Error('offline'))
    const { muted } = await mountAt('/contests/7')
    expect(muted.value).toBe(false)
  })

  it('does not mute ordinary practice pages', async () => {
    const { muted } = await mountAt('/problems/3')
    expect(muted.value).toBe(false)
    expect(getContest).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 5: Run it to verify it fails**

Run: `corepack pnpm test -- src/composables/useCompanionMute.spec.ts`
Expected: FAIL because the composable does not exist.

- [ ] **Step 6: Implement `useCompanionMute`**

Read `useRoute()`. Compute `contestId`:

```ts
const contestId = computed(() => {
  if (route.name === 'contest-detail') {
    const id = Number(route.params.id)
    return Number.isFinite(id) && id > 0 ? id : 0
  }
  if (route.name === 'problem-detail' || route.name === 'problem-submit') {
    const id = Number(route.query.contestId)
    return Number.isFinite(id) && id > 0 ? id : 0
  }
  return 0
})
```

Module-level cache `Map<number, { status: string; at: number }>` with 15 seconds TTL. Watch `contestId` immediately; set `checking`, fetch `getContest(id)`, set `status`, catch to `null`. `muted = computed(() => contestId.value > 0 && status.value === 'running')`. `clearCompanionMuteCache()` clears the map.

- [ ] **Step 7: Run both tests**

Run: `corepack pnpm test -- src/composables/useReducedMotion.spec.ts src/composables/useCompanionMute.spec.ts`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/web/src/composables/useReducedMotion.ts apps/web/src/composables/useReducedMotion.spec.ts apps/web/src/composables/useCompanionMute.ts apps/web/src/composables/useCompanionMute.spec.ts
git commit -m "feat: add companion motion and contest mute composables"
```

---

### Task 5: CompanionNote and CompanionPageNote

**Files:**
- Create: `apps/web/src/components/companion/CompanionNote.vue`
- Create: `apps/web/src/components/companion/CompanionPageNote.vue`
- Test: `apps/web/src/components/companion/CompanionNote.spec.ts`
- Test: `apps/web/src/components/companion/CompanionPageNote.spec.ts`

**Interfaces:**
- Consumes: `useCompanionStore`, `useCompanionMute`, `COMPANION_PAGE_NOTES`, `pickCompanionLine`, `localDateKey`.
- Produces: `<CompanionNote :line :expression? :dismissible? @dismiss />`.
- Produces: `<CompanionPageNote />` with no props; renders only when the current route has a note and the companion is enabled, activity is not `quiet`, not muted, and not dismissed today.

- [ ] **Step 1: Write the failing CompanionNote test**

```ts
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import CompanionNote from './CompanionNote.vue'

describe('CompanionNote', () => {
  it('renders a chibi, line and close button', async () => {
    const wrapper = mount(CompanionNote, { props: { line: '先读题。别急着写。' } })
    expect(wrapper.text()).toContain('先读题')
    expect(wrapper.find('[data-testid="companion-placeholder"]').exists()).toBe(true)
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('dismiss')).toHaveLength(1)
  })

  it('can hide the close button', () => {
    const wrapper = mount(CompanionNote, { props: { line: '专注点。', dismissible: false } })
    expect(wrapper.find('button').exists()).toBe(false)
  })
})
```

- [ ] **Step 2: Run it to verify it fails**

Run: `corepack pnpm test -- src/components/companion/CompanionNote.spec.ts`
Expected: FAIL because the component does not exist.

- [ ] **Step 3: Implement `CompanionNote.vue`**

Fixed `min-height: 44px`, horizontal layout: 28px chibi avatar, text, optional close button with `aria-label="关闭伙伴提示"`. Props: `line: string`, `expression: CompanionExpression` default `'smug'`, `dismissible: boolean` default `true`. Emit `dismiss`. Background `--kapoj-kapo-soft`, 1px `--kapoj-manga-line`, left 3px crimson bar, `border-radius: var(--kapoj-radius-sm)`.

- [ ] **Step 4: Write the failing CompanionPageNote test**

```ts
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useCompanionStore } from '@/stores/companion'
import CompanionPageNote from './CompanionPageNote.vue'

vi.mock('@/composables/useCompanionMute', () => ({
  useCompanionMute: () => ({ muted: { value: false }, checking: { value: false } }),
}))

const routes = [
  { path: '/problems', name: 'problems', component: { template: '<div />' } },
  { path: '/privacy', name: 'privacy', component: { template: '<div />' } },
]

let pinia: ReturnType<typeof createPinia>

async function mountAt(path: string) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  return mount(CompanionPageNote, { global: { plugins: [router, pinia] } })
}

beforeEach(() => {
  localStorage.clear()
  pinia = createPinia()
  setActivePinia(pinia)
})

describe('CompanionPageNote', () => {
  it('renders the mapped note and dismisses it for today', async () => {
    const wrapper = await mountAt('/problems')
    expect(wrapper.text()).toContain('今天的题')
    await wrapper.get('button').trigger('click')
    expect(wrapper.find('.companion-note').exists()).toBe(false)
    const store = useCompanionStore()
    store.load()
    expect(store.isNoteDismissed('problems')).toBe(true)
  })

  it('renders nothing on routes without a note', async () => {
    const wrapper = await mountAt('/privacy')
    expect(wrapper.find('.companion-note').exists()).toBe(false)
  })

  it('renders nothing when the companion is disabled or quiet', async () => {
    const store = useCompanionStore()
    store.load()
    store.setEnabled(false)
    expect((await mountAt('/problems')).find('.companion-note').exists()).toBe(false)
    store.setEnabled(true)
    store.setActivityLevel('quiet')
    expect((await mountAt('/problems')).find('.companion-note').exists()).toBe(false)
  })
})
```

- [ ] **Step 5: Run it to verify it fails**

Run: `corepack pnpm test -- src/components/companion/CompanionPageNote.spec.ts`
Expected: FAIL because the component does not exist.

- [ ] **Step 6: Implement `CompanionPageNote.vue`**

Use `route.name` (string) as the note key. Look up `COMPANION_PAGE_NOTES[key]`. Compute `line = pickCompanionLine(note.lineKey, localDateKey())`. Call `store.load()` in setup. Render `<CompanionNote>` only when `note && store.enabled && store.activityLevel !== 'quiet' && !muted && !store.isNoteDismissed(key)`. On dismiss call `store.dismissNote(key)`.

- [ ] **Step 7: Run both tests**

Run: `corepack pnpm test -- src/components/companion/CompanionNote.spec.ts src/components/companion/CompanionPageNote.spec.ts`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/web/src/components/companion/CompanionNote.vue apps/web/src/components/companion/CompanionNote.spec.ts apps/web/src/components/companion/CompanionPageNote.vue apps/web/src/components/companion/CompanionPageNote.spec.ts
git commit -m "feat: add companion page notes"
```

---

### Task 6: CompanionDock

**Files:**
- Create: `apps/web/src/components/companion/CompanionDock.vue`
- Test: `apps/web/src/components/companion/CompanionDock.spec.ts`

**Interfaces:**
- Consumes: `useAuthStore`, `useCompanionStore`, `useCompanionMute`, `getActivitySummary`, `signInActivity`, `pickCompanionLine`, `localDateKey`.
- Produces: `.companion-dock` fixed widget; hidden unless logged in, enabled and not muted.

- [ ] **Step 1: Write the failing dock test**

```ts
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { getActivitySummary, signInActivity } from '@/api/leaderboard'
import { useCompanionStore } from '@/stores/companion'
import CompanionDock from './CompanionDock.vue'

const auth = vi.hoisted(() => ({ isLoggedIn: true, user: { id: 7 } }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/leaderboard', () => ({
  getActivitySummary: vi.fn().mockResolvedValue({ signedInToday: true }),
  signInActivity: vi.fn(),
}))

const routes = [
  { path: '/', name: 'home', component: { template: '<div />' } },
  { path: '/login', name: 'login', component: { template: '<div />' } },
]

async function mountDock(path = '/') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const pinia = createPinia()
  setActivePinia(pinia)
  return mount(CompanionDock, {
    global: { plugins: [router, PrimeVue, pinia] },
    attachTo: document.body,
  })
}

beforeEach(() => {
  localStorage.clear()
  auth.isLoggedIn = true
  vi.mocked(getActivitySummary).mockResolvedValue({ signedInToday: true } as never)
})

afterEach(() => {
  document.body.innerHTML = ''
})

describe('CompanionDock', () => {
  it('hides for guests', async () => {
    auth.isLoggedIn = false
    const wrapper = await mountDock()
    expect(wrapper.find('.companion-dock').exists()).toBe(false)
  })

  it('hides on auth routes even when a session exists', async () => {
    const wrapper = await mountDock('/login')
    expect(wrapper.find('.companion-dock').exists()).toBe(false)
  })

  it('opens and closes with Escape', async () => {
    const wrapper = await mountDock()
    await flushPromises()
    await wrapper.get('.companion-dock__toggle').trigger('click')
    expect(wrapper.get('.companion-dock__toggle').attributes('aria-expanded')).toBe('true')
    await wrapper.get('.companion-dock__panel').trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('.companion-dock__panel').exists()).toBe(false)
  })

  it('offers sign-in when today is not signed', async () => {
    vi.mocked(getActivitySummary).mockResolvedValue({ signedInToday: false } as never)
    const wrapper = await mountDock()
    await flushPromises()
    expect(wrapper.text()).toContain('签到')
    await wrapper.get('.companion-dock__signin').trigger('click')
    expect(signInActivity).toHaveBeenCalled()
  })

  it('can switch to quiet mode from the panel', async () => {
    const wrapper = await mountDock()
    await flushPromises()
    await wrapper.get('.companion-dock__toggle').trigger('click')
    await wrapper.get('.companion-dock__quiet').trigger('click')
    expect(useCompanionStore().activityLevel).toBe('quiet')
  })

  it('disappears when the companion is disabled', async () => {
    const wrapper = await mountDock()
    const store = useCompanionStore()
    store.load()
    store.setEnabled(false)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.companion-dock').exists()).toBe(false)
  })
})
```

- [ ] **Step 2: Run it to verify it fails**

Run: `corepack pnpm test -- src/components/companion/CompanionDock.spec.ts`
Expected: FAIL because the component does not exist.

- [ ] **Step 3: Implement `CompanionDock.vue`**

Setup: `const auth = useAuthStore(); const store = useCompanionStore(); store.load(); const { muted } = useCompanionMute(); const route = useRoute()`. `const AUTH_ROUTES = new Set(['login', 'register', 'forgot-password', 'reset-password', 'verify-email'])`; `visible = computed(() => auth.isLoggedIn && store.enabled && !muted.value && !AUTH_ROUTES.has(String(route.name)))`.

Collapsed button `.companion-dock__toggle` 56px, `aria-expanded`, `aria-controls="companion-dock-panel"`, chibi avatar + a small crimson dot. On click toggle `open`. When open, render `.companion-dock__panel` with `role="region"`, `aria-label="KapOJ 伙伴"`, bust avatar, bubble line, activity summary, and actions:

- If `signedInToday === false`, button `.companion-dock__signin` calls `signInActivity()`, then sets `signedInToday = true`; on error leaves state and shows "……线路断了。".
- Link to `/problems` "去刷题".
- Link to `/appearance` "伙伴设置".
- Button `.companion-dock__quiet` "安静" sets `store.setActivityLevel('quiet')`.
- Close button.

Load summary on mount only when visible: `getActivitySummary().then(r => signedInToday.value = r.signedInToday).catch(() => {})`. Watch `visible` to load when it becomes true. Add `keydown` Escape listener while open; return focus to the toggle. Close on route change via `watch(() => route.fullPath, () => { open.value = false })`.

Line: `pickCompanionLine(signedInToday.value === false ? 'signin' : 'greeting', localDateKey())`.

Styles: `position: fixed; right: var(--kapoj-space-5); bottom: var(--kapoj-space-5); z-index: 25;` panel width `min(320px, calc(100vw - 32px))`, max-height 60vh. On mobile right/bottom 16px.

- [ ] **Step 4: Run the dock test**

Run: `corepack pnpm test -- src/components/companion/CompanionDock.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/companion/CompanionDock.vue apps/web/src/components/companion/CompanionDock.spec.ts
git commit -m "feat: add companion dock"
```

---

### Task 7: Layout Integration

**Files:**
- Modify: `apps/web/src/layouts/DefaultLayout.vue`
- Modify: `apps/web/src/App.vue`
- Test: `apps/web/src/layouts/DefaultLayout.spec.ts`
- Test: `apps/web/src/App.spec.ts`
- Modify: `apps/web/src/layouts/AdminLayout.spec.ts`

**Interfaces:**
- Consumes: `CompanionDock`, `CompanionPageNote`, `useCompanionStore`, `useCompanionMute`.
- Produces: exactly one dock per default-layout page; zero in admin; companion preferences loaded once at app start; a static "专注模式" chip while a contest is running.

- [ ] **Step 1: Write the failing DefaultLayout test**

```ts
import { mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'
import DefaultLayout from './DefaultLayout.vue'

const mute = vi.hoisted(() => ({ muted: { value: false } }))

vi.mock('@/stores/site', () => ({
  useSiteStore: () => ({ recordVisit: vi.fn().mockResolvedValue(undefined) }),
}))
vi.mock('@/composables/useCompanionMute', () => ({
  useCompanionMute: () => ({ muted: mute.muted, checking: { value: false } }),
}))

const routes = [{ path: '/', component: DefaultLayout, children: [{ path: '', component: { template: '<div />' } }] }]

describe('DefaultLayout', () => {
  it('mounts the dock and page note for default pages', async () => {
    const router = createRouter({ history: createMemoryHistory(), routes })
    await router.push('/')
    await router.isReady()
    const wrapper = mount(DefaultLayout, {
      global: {
        plugins: [router, PrimeVue, createPinia()],
        stubs: {
          AppShell: { template: '<div><slot name="nav" /><slot /></div>' },
          AppNav: true, SiteFooter: true, CompanionDock: true, CompanionPageNote: true,
        },
      },
    })
    expect(wrapper.find('companion-dock-stub').exists()).toBe(true)
    expect(wrapper.find('companion-page-note-stub').exists()).toBe(true)
  })

  it('shows a static focus chip while a contest is running', async () => {
    mute.muted.value = true
    const router = createRouter({ history: createMemoryHistory(), routes })
    await router.push('/')
    await router.isReady()
    const wrapper = mount(DefaultLayout, {
      global: {
        plugins: [router, PrimeVue, createPinia()],
        stubs: {
          AppShell: { template: '<div><slot name="nav" /><slot /></div>' },
          AppNav: true, SiteFooter: true, CompanionDock: true, CompanionPageNote: true,
        },
      },
    })
    expect(wrapper.text()).toContain('专注模式')
    mute.muted.value = false
  })
})
```

- [ ] **Step 2: Run it to verify it fails**

Run: `corepack pnpm test -- src/layouts/DefaultLayout.spec.ts`
Expected: FAIL because the layout has no companion children.

- [ ] **Step 3: Modify `DefaultLayout.vue`**

Import both components and `useCompanionMute`. In setup: `const { muted } = useCompanionMute()`. Render `<CompanionPageNote />` as the first child inside `AppShell` before `<RouterView />`, add a static focus chip when muted, and `<CompanionDock />` after `</AppShell>`:

```vue
<AppShell ...>
  <template #nav="{ navigate }">...</template>
  <p v-if="muted" class="companion-focus-chip">专注模式 · 伙伴已静默</p>
  <CompanionPageNote />
  <RouterView />
  <template #footer><SiteFooter /></template>
</AppShell>
<CompanionDock />
```

Style `.companion-focus-chip` as a small mono label with a muted silver border, no character art.

- [ ] **Step 4: Load companion preferences once at app start**

In `App.vue` script setup, import `useCompanionStore` and call `useCompanionStore().load()` once. This makes disabled/quiet preferences apply on auth and error pages too, before any dock or note mounts.

Add an `App.spec.ts` assertion using the existing `mountApp()` helper:

```ts
it('loads companion preferences on startup', async () => {
  localStorage.setItem(
    'kapoj.companion.prefs',
    JSON.stringify({ enabled: false, activityLevel: 'quiet', motion: 'reduce' }),
  )
  await mountApp()
  const store = useCompanionStore()
  expect(store.enabled).toBe(false)
  expect(store.activityLevel).toBe('quiet')
})
```

Import `useCompanionStore` in the spec.

- [ ] **Step 5: Add the admin assertion**

In `AdminLayout.spec.ts`, in the first test add:

```ts
expect(wrapper.find('.companion-dock').exists()).toBe(false)
expect(wrapper.find('.companion-note').exists()).toBe(false)
```

- [ ] **Step 6: Run both layout tests**

Run: `corepack pnpm test -- src/layouts/DefaultLayout.spec.ts src/layouts/AdminLayout.spec.ts src/App.spec.ts`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/layouts/DefaultLayout.vue apps/web/src/layouts/DefaultLayout.spec.ts apps/web/src/layouts/AdminLayout.spec.ts apps/web/src/App.vue apps/web/src/App.spec.ts
git commit -m "feat: mount companion in default layout only"
```

---

### Task 8: Empty, Loading and Error States

**Files:**
- Modify: `apps/web/src/components/ui/AppEmptyState.vue`
- Modify: `apps/web/src/components/ui/AppLoadingState.vue`
- Modify: `apps/web/src/components/ui/AppErrorState.vue`
- Test: `apps/web/src/components/ui/AppEmptyState.spec.ts`
- Test: `apps/web/src/components/ui/AppLoadingState.spec.ts`
- Test: `apps/web/src/components/ui/AppErrorState.spec.ts`

**Interfaces:**
- `AppEmptyState` adds `mascot?: CompanionExpression | ''` and `companionLine?: string`.
- `AppLoadingState` adds `mascot?: CompanionExpression | ''`.
- `AppErrorState` adds `mascot?: CompanionExpression | ''`.
- All three consume `useCompanionStore` and hide the mascot when `enabled` is false; the surrounding empty/loading/error UI remains unchanged.

- [ ] **Step 1: Write the failing empty-state test**

```ts
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { useCompanionStore } from '@/stores/companion'
import AppEmptyState from './AppEmptyState.vue'

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})

describe('AppEmptyState', () => {
  it('renders the companion mascot and line when provided', () => {
    const wrapper = mount(AppEmptyState, {
      props: { title: '暂无记录', mascot: 'lost', companionLine: '这里什么都没有。' },
    })
    expect(wrapper.find('[data-testid="companion-placeholder"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('这里什么都没有。')
  })

  it('keeps the icon when no mascot is set', () => {
    const wrapper = mount(AppEmptyState, { props: { title: '空', icon: 'pi pi-inbox' } })
    expect(wrapper.find('.pi-inbox').exists()).toBe(true)
  })

  it('hides the mascot when the companion is disabled', () => {
    const store = useCompanionStore()
    store.load()
    store.setEnabled(false)
    const wrapper = mount(AppEmptyState, { props: { title: '空', mascot: 'lost' } })
    expect(wrapper.find('[data-testid="companion-placeholder"]').exists()).toBe(false)
  })
})
```

- [ ] **Step 2: Run it to verify it fails**

Run: `corepack pnpm test -- src/components/ui/AppEmptyState.spec.ts`
Expected: FAIL because the props do not exist.

- [ ] **Step 3: Implement the three prop additions**

`AppEmptyState`: when `mascot` is non-empty and the companion is enabled, render `<CompanionAvatar pose="bust" :expression="mascot" size="md" />` above the title and hide the icon box; when `companionLine` is set and the mascot is visible, render it as `<p class="app-empty__companion">`.

`AppLoadingState`: when `mascot` is non-empty and the companion is enabled, render `<CompanionAvatar pose="chibi" :expression="mascot" size="xs" />` before the label.

`AppErrorState`: when `mascot` is non-empty and the companion is enabled, render `<CompanionAvatar pose="bust" :expression="mascot" size="sm" />` above the title.

Each component resolves the store defensively so existing specs that mount them without Pinia keep working:

```ts
import { getActivePinia } from 'pinia'
const companion = getActivePinia() ? useCompanionStore() : null
companion?.load()
const showMascot = computed(() => Boolean(props.mascot) && (companion?.enabled ?? true))
```

In the running app Pinia is always active; the fallback only keeps isolated component tests and storybook-like mounts from crashing.

- [ ] **Step 4: Write the loading and error tests**

```ts
// Both specs use the same beforeEach as AppEmptyState:
// localStorage.clear(); setActivePinia(createPinia())

// AppLoadingState.spec.ts
it('shows a chibi when a mascot is provided', () => {
  const wrapper = mount(AppLoadingState, { props: { mascot: 'sleepy' } })
  expect(wrapper.find('[data-testid="companion-placeholder"]').exists()).toBe(true)
})

// AppErrorState.spec.ts
it('shows an annoyed mascot when requested', () => {
  const wrapper = mount(AppErrorState, { props: { mascot: 'annoyed' } })
  expect(wrapper.find('[data-testid="companion-placeholder"]').exists()).toBe(true)
})
```

- [ ] **Step 5: Run all three tests**

Run: `corepack pnpm test -- src/components/ui/AppEmptyState.spec.ts src/components/ui/AppLoadingState.spec.ts src/components/ui/AppErrorState.spec.ts`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/components/ui/AppEmptyState.vue apps/web/src/components/ui/AppEmptyState.spec.ts apps/web/src/components/ui/AppLoadingState.vue apps/web/src/components/ui/AppLoadingState.spec.ts apps/web/src/components/ui/AppErrorState.vue apps/web/src/components/ui/AppErrorState.spec.ts
git commit -m "feat: add optional companion art to state components"
```

---

### Task 9: Home Hero

**Files:**
- Modify: `apps/web/src/views/HomeView.vue`
- Test: `apps/web/src/views/HomeView.spec.ts`

**Interfaces:**
- Consumes: `CompanionAvatar`, `CompanionBubble`, `MangaLabel`, `useCompanionStore`, `pickCompanionLine`, `localDateKey`.

- [ ] **Step 1: Write the failing HomeView test**

In `HomeView.spec.ts`, add `localStorage.removeItem('kapoj.companion.prefs')` to the existing `beforeEach` so the disabled test cannot leak into later tests. Then add:

```ts
it('shows the companion and greeting in the hero', async () => {
  const { wrapper } = await mountHome()
  expect(wrapper.find('.hero__companion').exists()).toBe(true)
  expect(wrapper.find('.companion-bubble').exists()).toBe(true)
  expect(wrapper.find('.hero__companion').text().length).toBeGreaterThan(0)
})

it('shows the abstract fallback when the companion is disabled', async () => {
  const { wrapper } = await mountHome()
  const store = useCompanionStore()
  store.load()
  store.setEnabled(false)
  await wrapper.vm.$nextTick()
  expect(wrapper.find('.hero__companion').exists()).toBe(false)
  expect(wrapper.find('.hero__signal').exists()).toBe(true)
  store.reset()
})
```

Import `useCompanionStore` in the spec.

- [ ] **Step 2: Run it to verify it fails**

Run: `corepack pnpm test -- src/views/HomeView.spec.ts`
Expected: FAIL because `.hero__companion` does not exist.

- [ ] **Step 3: Modify the hero template**

Inside the hero, after `.hero__copy` and before `.hero__status`, add:

```vue
<div v-if="companion.enabled" class="hero__companion" aria-hidden="false">
  <CompanionAvatar pose="bust" expression="smug" size="lg" />
  <CompanionBubble :text="companionGreeting" />
</div>
<div v-else class="hero__signal" aria-hidden="true">
  <span class="hero__signal-ring" />
  <span class="hero__signal-ring" />
  <span class="hero__signal-mark">K</span>
</div>
```

In script: `const companion = useCompanionStore(); companion.load();` and `const companionGreeting = computed(() => pickCompanionLine('greeting', localDateKey()))`.

- [ ] **Step 4: Update the hero grid**

Change `.hero` to `grid-template-columns: minmax(0, 1.15fr) minmax(220px, 0.55fr) minmax(300px, 0.8fr);`. Add `.hero__companion` flex column, centered, `min-width: 0`, with the bubble aligned right. Add `.hero__signal` abstract rings using `--kapoj-kapo-soft` and `--kapoj-manga-line`.

At `max-width: 1100px`, set `.hero__companion` / `.hero__signal` to `display: none` **and** change `.hero` back to `grid-template-columns: minmax(0, 1.35fr) minmax(300px, 0.9fr);` so no empty third track remains. At `max-width: 900px`, collapse to one column as today.

- [ ] **Step 5: Apply the manga label and sticker treatment**

Add `companion-sticker` to the hero's `AppSurface` class list. Replace the `section-eyebrow` line in the three main content sections with `MangaLabel`:

```vue
<MangaLabel index="01" label="LEARNING OVERVIEW" kana="学習記録" />
<MangaLabel index="02" label="QUICK START" kana="練習開始" />
<MangaLabel index="03" label="GROWTH" kana="成長記録" />
```

Keep the existing Chinese `<h2>` directly below each label. Remove only the old `section-eyebrow` paragraph for those three sections; other sections keep their existing markup.

- [ ] **Step 6: Run the HomeView test**

Run: `corepack pnpm test -- src/views/HomeView.spec.ts`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/views/HomeView.vue apps/web/src/views/HomeView.spec.ts
git commit -m "feat: add companion to home hero"
```

---

### Task 10: Auth Static Art and 404/403

**Files:**
- Modify: `apps/web/src/components/auth/AuthLayout.vue`
- Modify: `apps/web/src/views/NotFoundView.vue`
- Modify: `apps/web/src/views/ForbiddenView.vue`
- Test: `apps/web/src/components/auth/AuthLayout.spec.ts`
- Test: `apps/web/src/views/NotFoundView.spec.ts`
- Test: `apps/web/src/views/ForbiddenView.spec.ts`

**Interfaces:**
- Auth pages keep using `AuthLayout`; the layout adds a static `CompanionAvatar pose="full"` and one greeting line, with no dock and no interactive character behavior. It consumes `useCompanionStore` and hides the block when the companion is disabled.
- 404 passes `mascot="lost"`; 403 passes `mascot="smug"`; both rely on Task 8's store check.

- [ ] **Step 1: Write the failing AuthLayout test**

```ts
import { mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it } from 'vitest'
import AuthLayout from './AuthLayout.vue'

describe('AuthLayout', () => {
  it('renders static companion art without a dock', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/', component: { template: '<div />' } }],
    })
    await router.push('/')
    await router.isReady()
    const wrapper = mount(AuthLayout, {
      props: { title: '登录' },
      global: { plugins: [router, createPinia()] },
    })
    expect(wrapper.find('.auth-intro__companion').exists()).toBe(true)
    expect(wrapper.find('.auth-panel__companion').exists()).toBe(true)
    expect(wrapper.find('[data-testid="companion-placeholder"]').exists()).toBe(true)
    expect(wrapper.find('.companion-dock').exists()).toBe(false)
    expect(wrapper.find('.auth-intro__companion button').exists()).toBe(false)
  })
})
```

- [ ] **Step 2: Run it to verify it fails**

Run: `corepack pnpm test -- src/components/auth/AuthLayout.spec.ts`
Expected: FAIL because the companion block does not exist.

- [ ] **Step 3: Add the static art to AuthLayout**

Import `useCompanionStore`, call `companion.load()` in setup, and wrap the block with `v-if="companion.enabled"`. Inside `.auth-intro`, after the features list and before the status row:

```vue
<div v-if="companion.enabled" class="auth-intro__companion" aria-hidden="true">
  <CompanionAvatar pose="full" expression="smug" size="lg" />
  <CompanionBubble text="登录而已……别紧张。" />
</div>
```

Use `aria-hidden="true"` on the decorative image; the bubble text is visible but not interactive. Style `.auth-intro` as `position: relative`; `.auth-intro__companion` absolute right/bottom, width `min(46%, 280px)`, pointer-events none, opacity 0.95.

Because `.auth-intro` is hidden at `max-width: 900px`, add a mobile-only static block inside the auth panel header:

```vue
<div v-if="companion.enabled" class="auth-panel__companion" aria-hidden="true">
  <CompanionAvatar pose="chibi" expression="pout" size="sm" />
  <span>登录而已……别紧张。</span>
</div>
```

`.auth-panel__companion` is `display: none` above 900px and `display: flex` below it, so mobile still sees her without a dock.

- [ ] **Step 4: Update 404/403**

`NotFoundView`: add `mascot="lost"` and `companion-line="这里什么都没有。……跟着我。"` to `AppEmptyState`.

`ForbiddenView`: add `mascot="smug"` and `companion-line="这里不让你进。……别硬闯。"` to `AppEmptyState`.

- [ ] **Step 5: Write the 404/403 assertions**

Add to the existing specs:

```ts
expect(wrapper.find('[data-testid="companion-placeholder"]').exists()).toBe(true)
expect(wrapper.text()).toContain('跟着我')
```

Use the matching line for 403.

- [ ] **Step 6: Run the three tests**

Run: `corepack pnpm test -- src/components/auth/AuthLayout.spec.ts src/views/NotFoundView.spec.ts src/views/ForbiddenView.spec.ts`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/components/auth/AuthLayout.vue apps/web/src/components/auth/AuthLayout.spec.ts apps/web/src/views/NotFoundView.vue apps/web/src/views/NotFoundView.spec.ts apps/web/src/views/ForbiddenView.vue apps/web/src/views/ForbiddenView.spec.ts
git commit -m "feat: add static companion art to auth and error pages"
```

---

### Task 11: Appearance Settings

**Files:**
- Create: `apps/web/src/components/companion/CompanionSettings.vue`
- Modify: `apps/web/src/views/AppearanceView.vue`
- Test: `apps/web/src/components/companion/CompanionSettings.spec.ts`
- Test: `apps/web/src/views/AppearanceView.spec.ts`

**Interfaces:**
- Consumes: `useCompanionStore`.
- Produces: `<CompanionSettings />` with enable toggle, activity level, reduced motion and reset.

- [ ] **Step 1: Write the failing settings test**

```ts
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it } from 'vitest'
import { useCompanionStore } from '@/stores/companion'
import CompanionSettings from './CompanionSettings.vue'

let pinia: ReturnType<typeof createPinia>

function mountSettings() {
  pinia = createPinia()
  setActivePinia(pinia)
  return mount(CompanionSettings, { global: { plugins: [PrimeVue, pinia] } })
}

beforeEach(() => {
  localStorage.clear()
})

describe('CompanionSettings', () => {
  it('toggles the companion and persists the choice', async () => {
    const wrapper = mountSettings()
    await wrapper.get('[data-test="companion-enabled"]').trigger('click')
    const store = useCompanionStore()
    expect(store.enabled).toBe(false)
    expect(localStorage.getItem('kapoj.companion.prefs')).toContain('false')
  })

  it('selects quiet activity and reduced motion', async () => {
    const wrapper = mountSettings()
    await wrapper.get('[data-test="activity-quiet"]').trigger('click')
    await wrapper.get('[data-test="motion-reduce"]').trigger('click')
    const store = useCompanionStore()
    expect(store.activityLevel).toBe('quiet')
    expect(store.motion).toBe('reduce')
  })
})
```

- [ ] **Step 2: Run it to verify it fails**

Run: `corepack pnpm test -- src/components/companion/CompanionSettings.spec.ts`
Expected: FAIL because the component does not exist.

- [ ] **Step 3: Implement `CompanionSettings.vue`**

Use native buttons for the three test hooks to keep the component simple and accessible:

- `[data-test="companion-enabled"]` — a button with `aria-pressed` and label "启用伙伴 / 已关闭".
- `[data-test="activity-quiet"]`, `[data-test="activity-normal"]`, `[data-test="activity-lively"]` — a segmented button group with `aria-pressed`.
- `[data-test="motion-reduce"]` — a button toggling between "跟随系统" and "减少动效".
- Reset button calls `store.reset()`.

Section layout: heading "伙伴", description "她的出现频率、动效与显示方式。当前为本地偏好，仅本浏览器生效。" Use `AppSurface` if imported, or a plain `.companion-settings` section with token styles.

- [ ] **Step 4: Insert into AppearanceView**

After the overview `AppSurface` and before the badges section, add `<CompanionSettings />`. Import it in the script.

- [ ] **Step 5: Add the AppearanceView assertion**

In `AppearanceView.spec.ts`, add:

```ts
expect(wrapper.find('.companion-settings').exists()).toBe(true)
expect(wrapper.text()).toContain('伙伴')
```

- [ ] **Step 6: Run both tests**

Run: `corepack pnpm test -- src/components/companion/CompanionSettings.spec.ts src/views/AppearanceView.spec.ts`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/components/companion/CompanionSettings.vue apps/web/src/components/companion/CompanionSettings.spec.ts apps/web/src/views/AppearanceView.vue apps/web/src/views/AppearanceView.spec.ts
git commit -m "feat: add companion appearance settings"
```

---

### Task 12: Route Coverage Audit

**Files:**
- Test: `apps/web/src/content/companion/pageNotes.spec.ts`

**Interfaces:**
- Consumes: `COMPANION_PAGE_NOTES`.
- Produces: a test that fails if a B-tier route loses its note or a quiet/admin/auth route gains one.

- [ ] **Step 1: Write the failing coverage test**

```ts
import { describe, expect, it } from 'vitest'
import { COMPANION_PAGE_NOTES } from './pageNotes'

const expected = [
  'problems', 'problem-detail', 'problem-submit', 'problem-official-solution',
  'problem-solutions', 'problem-solution-detail', 'problem-statistics',
  'problem-contests', 'contests', 'contest-detail', 'submissions', 'leaderboard',
  'learning-paths', 'learning-path-detail', 'tasks', 'achievements', 'badge-shop',
  'title-shop', 'notifications', 'certificate-verify', 'certificate-settings',
  'user-profile', 'appearance',
]

const excluded = [
  'home', 'login', 'register', 'forgot-password', 'reset-password', 'verify-email',
  'profile-edit', 'security-settings', 'ai-settings', 'problem-create', 'problem-edit',
  'problem-import', 'problem-testcase-generator', 'contest-create', 'contest-edit',
  'learning-path-create', 'learning-path-edit', 'forbidden', 'not-found', 'admin-dashboard',
]

describe('companion page note map', () => {
  it('covers every B-tier route', () => {
    for (const route of expected) expect(COMPANION_PAGE_NOTES[route]).toBeTruthy()
  })

  it('leaves A/C/admin/auth routes out', () => {
    for (const route of excluded) expect(COMPANION_PAGE_NOTES[route]).toBeUndefined()
  })
})
```

- [ ] **Step 2: Run it**

Run: `corepack pnpm test -- src/content/companion/pageNotes.spec.ts`
Expected: PASS if Task 2 was implemented correctly. If a route is missing, add it to `pageNotes.ts` using the nearest line key from Task 2.

- [ ] **Step 3: Commit**

```bash
git add apps/web/src/content/companion/pageNotes.spec.ts apps/web/src/content/companion/pageNotes.ts
git commit -m "test: lock companion page note coverage"
```

---

### Task 13: E2E Smoke and Final Verification

**Files:**
- Create: `apps/web/e2e/companion.spec.ts`

**Interfaces:**
- Consumes the real app and existing E2E credentials.
- Verifies static auth art, logged-in dock, note dismissal and admin exclusion.

- [ ] **Step 1: Write the E2E spec**

```ts
import { expect, test, type Page } from '@playwright/test'

const limitedUser = process.env.E2E_LIMITED_USER ?? 'm2bsolver1'
const limitedPassword = process.env.E2E_LIMITED_PASSWORD ?? 'correct-horse'
const adminUser = process.env.E2E_ADMIN_USER ?? 'm2badmin'
const adminPassword = process.env.E2E_ADMIN_PASSWORD ?? 'correct-horse'

async function login(page: Page, account: string, password: string) {
  await page.goto('/login')
  await page.getByLabel('用户名或邮箱').fill(account)
  await page.getByLabel('密码').fill(password)
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL('/')
}

test.describe('companion phase 0-1', () => {
  test('login shows static art and no dock', async ({ page }) => {
    await page.goto('/login')
    await expect(page.locator('.auth-intro__companion')).toBeVisible()
    await expect(page.locator('.companion-dock')).toHaveCount(0)
  })

  test('logged-in home and problems show the dock and a note', async ({ page }) => {
    await login(page, limitedUser, limitedPassword)
    await expect(page.locator('.companion-dock__toggle')).toBeVisible()
    await page.locator('.companion-dock__toggle').click()
    await expect(page.locator('.companion-dock__panel')).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(page.locator('.companion-dock__panel')).toHaveCount(0)
    await page.goto('/problems')
    await expect(page.locator('.companion-note')).toBeVisible()
    await page.getByRole('button', { name: '关闭伙伴提示' }).click()
    await page.reload()
    await expect(page.locator('.companion-note')).toHaveCount(0)
  })

  test('admin does not get the companion', async ({ page }) => {
    await login(page, adminUser, adminPassword)
    await page.goto('/admin')
    await expect(page.locator('.companion-dock')).toHaveCount(0)
    await expect(page.locator('.companion-note')).toHaveCount(0)
  })
})
```

- [ ] **Step 2: Run the E2E spec if the backend is available**

Run: `E2E_API_URL=http://127.0.0.1:18080 corepack pnpm test:e2e -- e2e/companion.spec.ts`
Expected: PASS, or a clear backend-not-running message from `global-setup.ts`. Do not weaken the global setup.

- [ ] **Step 3: Run the full unit suite**

Run: `corepack pnpm test`
Expected: PASS.

- [ ] **Step 4: Run lint and build**

Run: `corepack pnpm lint`
Expected: PASS.

Run: `corepack pnpm build`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/e2e/companion.spec.ts
git commit -m "test: add companion browser smoke tests"
```

---

## Self-Review Notes

- Spec coverage: Phase 0–1 tokens, avatar/placeholder, dock, page notes, auth static art, home hero, 404/403, state components, appearance settings, local preferences, contest mute, reduced motion and E2E smoke each have a task.
- Not in this plan (Phase 2+): backend, bond, skins, profile archive, AI persona, shop, admin settings.
- Type consistency: `CompanionLineKey`, `CompanionExpression`, `CompanionPose`, `CompanionAsset`, `COMPANION_PAGE_NOTES`, `useCompanionStore`, `useCompanionMute`, `useReducedMotion` names are used consistently across tasks.
- Review Focus tests are mapped to Tasks 2, 3, 4, 6 and 7.
- No task adds a backend file or API client.
