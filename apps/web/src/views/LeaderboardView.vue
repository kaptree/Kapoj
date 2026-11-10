<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import type { ComponentPublicInstance } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import Paginator from 'primevue/paginator'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import TabPanel from 'primevue/tabpanel'
import TabPanels from 'primevue/tabpanels'
import Tabs from 'primevue/tabs'

import { ApiError } from '@/api/http'
import {
  getRatingSeason,
  listActivityLeaderboard,
  listRatingLeaderboard,
  listSharingLeaderboard,
  listWealthLeaderboard,
} from '@/api/leaderboard'
import type { EquippedBadge, EquippedTitle, RatingLevel, RatingSeasonView } from '@/api/types'
import IdentityShowcase from '@/components/IdentityShowcase.vue'
import RatingLevelName from '@/components/RatingLevelName.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { formatCoins } from '@/utils/gamification'

const TOP_SIZE = 10
const PAGE_SIZE = 20

type BoardKey = 'rating' | 'activity' | 'wealth' | 'contribution'

interface BoardDef {
  key: BoardKey
  label: string
  english: string
  icon: string
  accent: string
  unit: string
  emptyTitle: string
  emptyDescription: string
}

interface RulesSection {
  key: BoardKey
  label: string
  accent: string
  icon: string
  items: string[]
}

/** Cards and full tables share one row shape; only Rating rows carry a level. */
interface LeaderboardRow {
  rank: number
  userId: number
  username: string
  nickname: string
  score: number
  level?: RatingLevel
  equippedBadges: EquippedBadge[]
  equippedTitle: EquippedTitle | null
}

interface LeaderboardPage {
  total: number
  items: LeaderboardRow[]
}

interface RowState {
  page: number
  rows: LeaderboardRow[]
  total: number
  loading: boolean
  loaded: boolean
  error: string
  seq: number
}

const boardDefs: BoardDef[] = [
  {
    key: 'rating',
    label: 'Rating 榜',
    english: 'RATING · TOP 10',
    icon: 'pi pi-star',
    accent: '#22d3ee',
    unit: 'Rating',
    emptyTitle: '暂无 Rating 排行',
    emptyDescription: '参与并完成 Rated 比赛后即可登上 Rating 榜。',
  },
  {
    key: 'activity',
    label: '活跃榜',
    english: 'ACTIVITY · TOP 10',
    icon: 'pi pi-bolt',
    accent: '#34d399',
    unit: '活跃度',
    emptyTitle: '暂无活跃度排行',
    emptyDescription: '提交代码、参加比赛或完成每日签到即可获得活跃度。',
  },
  {
    key: 'wealth',
    label: '富豪榜',
    english: 'WEALTH · TOP 10',
    icon: 'pi pi-wallet',
    accent: '#fbbf24',
    unit: '金币',
    emptyTitle: '暂无富豪排行',
    emptyDescription: '通过签到、成就和任务积累金币，即可登上富豪榜。',
  },
  {
    key: 'contribution',
    label: '贡献榜',
    english: 'CONTRIBUTION · TOP 10',
    icon: 'pi pi-heart',
    accent: '#a78bfa',
    unit: '贡献分',
    emptyTitle: '暂无贡献排行',
    emptyDescription: '创建或克隆比赛、题解被点赞即可获得贡献分。',
  },
]

/**
 * Scoring rules mirror the backend award table. Contribution is a current
 * balance: unlikes and deleted solution posts can lower it, so the copy must
 * never describe it as a lifetime total.
 */
const rulesSections: RulesSection[] = [
  {
    key: 'rating',
    label: 'Rating 榜',
    accent: '#22d3ee',
    icon: 'pi pi-star',
    items: [
      '初始 Rating 为 1500。',
      'Rated 比赛采用 Elo v1 计分，K = 32。',
      '单场比赛的 Rating 变动最多 ±100。',
      '赛季结束时按设置进行衰减或软重置。',
    ],
  },
  {
    key: 'activity',
    label: '活跃榜',
    accent: '#34d399',
    icon: 'pi pi-bolt',
    items: [
      '提交代码 +2，累计上限 40。',
      '参加比赛 +10，累计上限 50。',
      '每日签到 +5，每天仅一次。',
      '题解回复 +3，累计上限 20。',
      '点赞题解 +1 记点赞者，累计上限 20。',
      '关注用户 +2，累计上限 20。',
    ],
  },
  {
    key: 'wealth',
    label: '富豪榜',
    accent: '#fbbf24',
    icon: 'pi pi-wallet',
    items: [
      '按当前金币余额排名，而不是历史累计获得。',
      '每日签到、成就、任务与活动可获得金币。',
      '购买徽章、称号等会直接扣除余额。',
      '仅展示状态正常且余额大于 0 的用户。',
    ],
  },
  {
    key: 'contribution',
    label: '贡献榜',
    accent: '#a78bfa',
    icon: 'pi pi-heart',
    items: [
      '创建或克隆比赛 +20。',
      '题解被点赞 +5，记给题解作者。',
      '取消点赞会扣回 5 分；删除题解会移除该题解带来的贡献。',
      '删除比赛不会扣回创建时获得的 +20。',
      '贡献分是当前值而非累计值，可能因取消点赞或删除题解而下降。',
    ],
  },
]

const rulesVisible = ref(false)

function createState(): RowState {
  return {
    page: 1,
    rows: [],
    total: 0,
    loading: false,
    loaded: false,
    error: '',
    seq: 0,
  }
}

const topStates: Record<BoardKey, RowState> = {
  rating: reactive(createState()),
  activity: reactive(createState()),
  wealth: reactive(createState()),
  contribution: reactive(createState()),
}

const tableStates: Record<BoardKey, RowState> = {
  rating: reactive(createState()),
  activity: reactive(createState()),
  wealth: reactive(createState()),
  contribution: reactive(createState()),
}

const activeTab = ref<BoardKey>('rating')
const fullRankingsRef = ref<ComponentPublicInstance | null>(null)

const season = ref<RatingSeasonView | null>(null)
const nextResetDate = computed(() => season.value?.nextResetAt.slice(0, 10) ?? '')

const activeAccent = computed(
  () => boardDefs.find((board) => board.key === activeTab.value)?.accent ?? '#22d3ee',
)

/** Season info is best-effort: a failed fetch silently leaves it hidden. */
async function loadSeason() {
  try {
    season.value = await getRatingSeason()
  } catch {
    season.value = null
  }
}

function loadRows(key: BoardKey, page: number, pageSize: number): Promise<LeaderboardPage> {
  switch (key) {
    case 'rating':
      return listRatingLeaderboard({ page, pageSize })
    case 'activity':
      return listActivityLeaderboard({ page, pageSize })
    case 'wealth':
      return listWealthLeaderboard({ page, pageSize })
    case 'contribution':
      return listSharingLeaderboard({ page, pageSize })
  }
}

function errorMessage(e: unknown): string {
  return e instanceof ApiError ? e.message : '加载排行榜失败，请稍后再试'
}

async function fetchTop(key: BoardKey) {
  const state = topStates[key]
  const seq = ++state.seq
  state.loading = true
  state.error = ''
  try {
    const data = await loadRows(key, 1, TOP_SIZE)
    if (seq !== state.seq) return
    state.rows = data.items
    state.total = data.total
    state.loaded = true
  } catch (e) {
    if (seq !== state.seq) return
    state.error = errorMessage(e)
  } finally {
    if (seq === state.seq) state.loading = false
  }
}

async function fetchTable(key: BoardKey, page: number) {
  const state = tableStates[key]
  const seq = ++state.seq
  state.page = page
  state.loading = true
  state.error = ''
  try {
    const data = await loadRows(key, page, PAGE_SIZE)
    if (seq !== state.seq) return
    state.rows = data.items
    state.total = data.total
    state.loaded = true
  } catch (e) {
    if (seq !== state.seq) return
    state.error = errorMessage(e)
  } finally {
    if (seq === state.seq) state.loading = false
  }
}

function topState(key: BoardKey): RowState {
  return topStates[key]
}

function tableState(key: BoardKey): RowState {
  return tableStates[key]
}

function onPageChange(key: BoardKey, event: { page: number }) {
  void fetchTable(key, event.page + 1)
}

function formatScore(key: BoardKey, score: number): string {
  if (key === 'rating') return String(score)
  if (key === 'wealth') return formatCoins(score)
  return score.toLocaleString('zh-CN')
}

function prefersReducedMotion(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  )
}

function scrollToFullRankings() {
  void nextTick(() => {
    const el = fullRankingsRef.value?.$el as HTMLElement | undefined
    if (!el || typeof el.scrollIntoView !== 'function') return
    el.scrollIntoView({
      behavior: prefersReducedMotion() ? 'auto' : 'smooth',
      block: 'start',
    })
  })
}

function openBoard(key: BoardKey) {
  activeTab.value = key
  const state = tableStates[key]
  if (!state.loaded && !state.loading) void fetchTable(key, 1)
  scrollToFullRankings()
}

// Server colors go through style bindings only; the level name itself stays
// in the normal text color so it is readable with or without color.
function levelBadgeStyle(level: RatingLevel): Record<string, string> {
  return { borderColor: level.accentColor || level.color }
}

function levelDotStyle(level: RatingLevel): Record<string, string> {
  return {
    backgroundColor: level.color,
    borderColor: level.accentColor || level.color,
  }
}

onMounted(() => {
  void loadSeason()
  for (const board of boardDefs) void fetchTop(board.key)
  void fetchTable('rating', 1)
})

// The complete table loads lazily on first visit, whether the tab is opened by
// a card (openBoard) or by clicking the tab directly.
watch(activeTab, (key) => {
  const state = tableStates[key]
  if (!state.loaded && !state.loading) void fetchTable(key, 1)
})
</script>

<template>
  <div class="leaderboard-page">
    <AppPageHeader
      eyebrow="RANKS // HALL OF FAME"
      title="排行榜"
      description="四榜同辉：Rating、活跃、富豪与贡献总览，点击卡片查看完整排名。"
    >
      <template #actions>
        <Button
          data-test="rules-button"
          label="计分规则"
          icon="pi pi-question-circle"
          outlined
          @click="rulesVisible = true"
        />
      </template>
    </AppPageHeader>

    <p v-if="season" class="season-banner" aria-label="当前赛季">
      当前赛季 {{ season.name }} · 下次重置 {{ nextResetDate }}
    </p>

    <section class="boards-grid" aria-label="四榜 Top 10">
      <article
        v-for="(board, index) in boardDefs"
        :key="board.key"
        class="board-card"
        :data-board="board.key"
        :style="{ '--board-accent': board.accent, '--board-delay': `${index * 40}ms` }"
        role="button"
        tabindex="0"
        :aria-label="`${board.label} Top 10，查看完整排名`"
        @click="openBoard(board.key)"
        @keydown.enter.prevent="openBoard(board.key)"
        @keydown.space.prevent="openBoard(board.key)"
      >
        <header class="board-card__head">
          <span class="board-card__icon" aria-hidden="true">
            <i :class="board.icon" />
          </span>
          <div class="board-card__titles">
            <h2 class="board-card__title">{{ board.label }}</h2>
            <p class="board-card__english">{{ board.english }}</p>
          </div>
          <span class="board-card__more" aria-hidden="true">查看全部 →</span>
        </header>

        <AppLoadingState
          v-if="topState(board.key).loading && topState(board.key).rows.length === 0"
          :label="`正在加载${board.label}`"
          :rows="5"
          class="board-card__loading"
        />

        <div
          v-else-if="topState(board.key).error"
          class="board-card__error"
          @click.stop
          @keydown.enter.stop
          @keydown.space.stop
        >
          <AppErrorState :description="topState(board.key).error" @retry="fetchTop(board.key)" />
        </div>

        <AppEmptyState
          v-else-if="topState(board.key).rows.length === 0"
          :icon="board.icon"
          :title="board.emptyTitle"
          :description="board.emptyDescription"
          class="board-card__empty"
        />

        <ol v-else class="board-card__list">
          <li
            v-for="row in topState(board.key).rows"
            :key="row.userId"
            class="board-row"
            :class="{ 'is-podium': row.rank <= 3, 'is-first': row.rank === 1 }"
          >
            <span class="board-row__rank">{{ row.rank }}</span>
            <span class="board-row__user">
              <RatingLevelName
                v-if="board.key === 'rating' && row.level"
                :name="row.nickname || row.username"
                :level="row.level"
              />
              <span v-else class="board-row__name">{{ row.nickname || row.username }}</span>
              <IdentityShowcase
                :badges="row.equippedBadges"
                :title="row.equippedTitle"
                size="small"
              />
            </span>
            <span class="board-row__score" :title="board.unit">
              {{ formatScore(board.key, row.score) }}
            </span>
          </li>
        </ol>

        <p v-if="board.key === 'rating' && season" class="board-card__season">
          本赛季 {{ season.name }} · {{ nextResetDate }} 重置
        </p>
      </article>
    </section>

    <AppSurface
      ref="fullRankingsRef"
      data-test="full-rankings"
      padding="lg"
      class="full-rankings"
      :style="{ '--board-accent': activeAccent }"
    >
      <header class="full-rankings__head">
        <h2 class="full-rankings__title">完整排名</h2>
        <p class="full-rankings__description">四个榜单的全部上榜用户</p>
      </header>

      <Tabs v-model:value="activeTab" lazy>
        <TabList>
          <Tab v-for="board in boardDefs" :key="board.key" :value="board.key">
            {{ board.label }}
          </Tab>
        </TabList>

        <TabPanels>
          <TabPanel v-for="board in boardDefs" :key="board.key" :value="board.key">
            <AppErrorState
              v-if="tableState(board.key).error && tableState(board.key).rows.length === 0"
              :description="tableState(board.key).error"
              @retry="fetchTable(board.key, tableState(board.key).page)"
            />

            <template v-else>
              <Message
                v-if="tableState(board.key).error"
                severity="error"
                :closable="false"
                class="page-error"
              >
                {{ tableState(board.key).error }}
              </Message>

              <AppLoadingState
                v-if="tableState(board.key).loading && tableState(board.key).rows.length === 0"
                :label="`正在加载${board.label}`"
              />

              <template v-else>
                <div
                  class="table-scroll"
                  tabindex="0"
                  role="region"
                  aria-label="排行榜表格，可横向滚动"
                >
                  <DataTable
                    :value="tableState(board.key).rows"
                    :loading="tableState(board.key).loading"
                    data-key="userId"
                    size="small"
                    row-hover
                    class="leaderboard-table"
                  >
                    <Column header="排名" style="width: 72px">
                      <template #body="{ data }">
                        <span class="rank" :class="{ 'rank--top': data.rank <= 3 }">
                          {{ data.rank }}
                        </span>
                      </template>
                    </Column>

                    <Column header="用户" style="min-width: 180px">
                      <template #body="{ data }">
                        <div class="user-cell">
                          <RouterLink
                            :to="{ name: 'user-profile', params: { id: data.userId } }"
                            class="user-link"
                            :class="{ 'user-link--rating': board.key === 'rating' }"
                          >
                            <RatingLevelName
                              v-if="board.key === 'rating' && data.level"
                              :name="data.nickname || data.username"
                              :level="data.level"
                            />
                            <template v-else>{{ data.nickname || data.username }}</template>
                          </RouterLink>
                          <IdentityShowcase
                            :badges="data.equippedBadges"
                            :title="data.equippedTitle"
                            size="small"
                          />
                        </div>
                      </template>
                    </Column>

                    <Column header="分数" style="width: 120px">
                      <template #body="{ data }">
                        <span class="score">{{ formatScore(board.key, data.score) }}</span>
                      </template>
                    </Column>

                    <Column v-if="board.key === 'rating'" header="等级" style="width: 132px">
                      <template #body="{ data }">
                        <span
                          v-if="data.level"
                          class="level-badge"
                          :style="levelBadgeStyle(data.level)"
                        >
                          <span
                            class="level-badge__dot"
                            :style="levelDotStyle(data.level)"
                            aria-hidden="true"
                          ></span>
                          <span class="level-badge__name">{{ data.level.name }}</span>
                        </span>
                      </template>
                    </Column>

                    <template #empty>
                      <AppEmptyState
                        icon="pi pi-chart-line"
                        :title="board.emptyTitle"
                        :description="board.emptyDescription"
                      />
                    </template>
                  </DataTable>
                </div>

                <Paginator
                  v-if="tableState(board.key).total > PAGE_SIZE"
                  :rows="PAGE_SIZE"
                  :total-records="tableState(board.key).total"
                  :first="(tableState(board.key).page - 1) * PAGE_SIZE"
                  :page-link-size="5"
                  template="PrevPageLink PageLinks NextPageLink"
                  class="pager"
                  @page="onPageChange(board.key, $event)"
                />
              </template>
            </template>
          </TabPanel>
        </TabPanels>
      </Tabs>
    </AppSurface>

    <Dialog
      v-model:visible="rulesVisible"
      modal
      header="四榜计分规则"
      data-test="rules-dialog"
      class="rules-dialog"
      :draggable="false"
    >
      <div class="rules-sections">
        <section
          v-for="section in rulesSections"
          :key="section.key"
          class="rules-section"
          :style="{ '--rule-accent': section.accent }"
        >
          <header class="rules-section__head">
            <span class="rules-section__icon" aria-hidden="true">
              <i :class="section.icon" />
            </span>
            <h3 class="rules-section__title">{{ section.label }}</h3>
          </header>
          <ul class="rules-section__items">
            <li v-for="item in section.items" :key="item">{{ item }}</li>
          </ul>
        </section>
      </div>
      <template #footer>
        <Button label="知道了" icon="pi pi-check" @click="rulesVisible = false" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.leaderboard-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
  max-width: var(--kapoj-content-max);
}

.season-banner {
  margin: 0;
  padding: var(--kapoj-space-2) var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 0.8125rem;
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

/* Four Top10 boards */
.boards-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--kapoj-space-4);
  align-items: stretch;
}

.board-card {
  --board-accent: var(--kapoj-primary);

  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
  padding: var(--kapoj-space-4);
  cursor: pointer;
  background:
    radial-gradient(
      120% 78% at 50% 0%,
      color-mix(in srgb, var(--board-accent) 12%, transparent) 0%,
      transparent 66%
    ),
    var(--kapoj-surface);
  border: 1px solid color-mix(in srgb, var(--board-accent) 24%, var(--kapoj-border));
  border-radius: var(--kapoj-radius-md);
  box-shadow:
    var(--kapoj-shadow-sm),
    inset 0 1px 0 color-mix(in srgb, var(--board-accent) 24%, transparent);
  transition:
    transform var(--kapoj-transition),
    border-color var(--kapoj-transition),
    box-shadow var(--kapoj-transition);
  animation: board-card-in var(--kapoj-transition-slow) ease-out both;
  animation-delay: var(--board-delay, 0ms);
}

.board-card:hover,
.board-card:focus-visible {
  border-color: color-mix(in srgb, var(--board-accent) 55%, var(--kapoj-border));
  box-shadow:
    0 12px 26px color-mix(in srgb, var(--board-accent) 16%, transparent),
    var(--kapoj-shadow-sm),
    inset 0 1px 0 color-mix(in srgb, var(--board-accent) 34%, transparent);
  transform: translateY(-2px);
}

.board-card:focus-visible {
  outline: 2px solid var(--board-accent);
  outline-offset: 2px;
}

@keyframes board-card-in {
  from {
    opacity: 0;
    transform: translateY(6px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.board-card__head {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  min-width: 0;
}

.board-card__icon {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  color: var(--board-accent);
  font-size: 0.9rem;
  background: color-mix(in srgb, var(--board-accent) 14%, transparent);
  border: 1px solid color-mix(in srgb, var(--board-accent) 34%, transparent);
  border-radius: var(--kapoj-radius-sm);
}

.board-card__titles {
  flex: 1 1 auto;
  min-width: 0;
}

.board-card__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 0.95rem;
  font-weight: 600;
  line-height: 1.2;
}

.board-card__english {
  margin: 2px 0 0;
  color: color-mix(in srgb, var(--board-accent) 62%, var(--kapoj-text-secondary));
  font-family: var(--kapoj-font-mono);
  font-size: 9px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.board-card__more {
  flex: none;
  color: var(--kapoj-text-muted);
  font-size: 11px;
  white-space: nowrap;
  transition: color var(--kapoj-transition);
}

.board-card:hover .board-card__more {
  color: var(--board-accent);
}

.board-card__list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 0;
  margin: 0;
  list-style: none;
}

.board-row {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  min-height: 32px;
  padding: 2px var(--kapoj-space-2);
  border: 1px solid transparent;
  border-radius: var(--kapoj-radius-sm);
}

.board-row.is-first {
  background: color-mix(in srgb, var(--board-accent) 9%, transparent);
  border-color: color-mix(in srgb, var(--board-accent) 24%, transparent);
}

.board-row__rank {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
  border-radius: 50%;
}

.board-row.is-podium .board-row__rank {
  color: #0b1120;
  font-weight: 700;
}

.board-row.is-podium:nth-child(1) .board-row__rank {
  background: linear-gradient(135deg, #fde68a, #d97706);
}

.board-row.is-podium:nth-child(2) .board-row__rank {
  background: linear-gradient(135deg, #e2e8f0, #94a3b8);
}

.board-row.is-podium:nth-child(3) .board-row__rank {
  background: linear-gradient(135deg, #fdba74, #b45309);
}

.board-row__user {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  align-items: flex-start;
  gap: 1px;
  min-width: 0;
}

.board-row__name {
  max-width: 100%;
  overflow: hidden;
  color: var(--kapoj-text);
  font-size: 12px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.board-row__user :deep(.rating-level-name) {
  max-width: 100%;
  overflow: hidden;
  font-size: 12px;
  text-overflow: ellipsis;
}

.board-row__score {
  flex: none;
  margin-left: auto;
  color: var(--kapoj-text);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  text-align: right;
}

.board-card__season {
  margin: auto 0 0;
  padding-top: var(--kapoj-space-2);
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  border-top: 1px solid var(--kapoj-border);
}

.board-card__loading :deep(.app-loading),
.board-card__error :deep(.app-error),
.board-card__empty :deep(.app-empty) {
  padding: var(--kapoj-space-2);
  background: transparent;
  border-color: transparent;
}

.board-card__error :deep(.app-error) {
  border-color: color-mix(in srgb, var(--kapoj-danger) 24%, transparent);
}

.board-card__error :deep(.app-error__icon) {
  display: none;
}

.board-card__error :deep(.app-error__title) {
  font-size: 0.8125rem;
}

.board-card__error {
  cursor: default;
}

/* Complete rankings */
.full-rankings {
  display: flex;
  flex-direction: column;
  min-width: 0;
  scroll-margin-top: calc(var(--kapoj-header-height) + var(--kapoj-space-4));
}

.full-rankings__head {
  margin-bottom: var(--kapoj-space-3);
}

.full-rankings__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.05rem;
  font-weight: 600;
}

.full-rankings__description {
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
}

.full-rankings :deep(.p-tablist-active-bar) {
  background: var(--board-accent);
}

.full-rankings :deep(.p-tab) {
  color: var(--kapoj-text-secondary);
}

.full-rankings :deep(.p-tab[data-p-active='true']) {
  color: var(--board-accent);
}

.full-rankings .rank--top {
  color: color-mix(in srgb, var(--board-accent) 65%, var(--kapoj-text));
  background: color-mix(in srgb, var(--board-accent) 14%, transparent);
}

.full-rankings .score {
  color: color-mix(in srgb, var(--board-accent) 62%, var(--kapoj-text));
}

.page-error {
  margin-bottom: var(--kapoj-space-3);
}

.table-scroll {
  overflow-x: auto;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.table-scroll:focus-visible {
  outline: 2px solid var(--kapoj-primary);
  outline-offset: 2px;
}

/* Let the outer .table-scroll own horizontal scrolling on narrow screens
   instead of nesting a second scroll container inside DataTable. */
.table-scroll :deep(.p-datatable-table-container) {
  overflow: visible !important;
}

.leaderboard-table :deep(.p-datatable-table) {
  min-width: 480px;
}

.leaderboard-table :deep(.p-datatable-thead > tr > th) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  background: var(--kapoj-bg-elevated);
  border-color: var(--kapoj-border);
}

.leaderboard-table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.rank {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 26px;
  height: 22px;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  border-radius: var(--kapoj-radius-xs);
}

.rank--top {
  color: var(--kapoj-primary);
  background: var(--kapoj-primary-soft);
}

.user-cell {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  min-width: 0;
}

.user-link {
  color: var(--kapoj-text);
  font-weight: 600;
  word-break: break-word;
}

.user-link:hover {
  color: var(--kapoj-primary);
}

/* The rating palette includes true black, so the rating username gets a light
   chip to stay readable on the dark-first theme. Activity/sharing links keep
   the plain treatment. */
.user-link--rating {
  padding: 0 6px;
  color: inherit;
  background: #f3f4f6;
  border-radius: var(--kapoj-radius-xs);
}

.user-link--rating:hover {
  color: inherit;
}

.level-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 8px;
  color: var(--kapoj-text);
  font-size: 12px;
  font-weight: 600;
  line-height: 1.4;
  white-space: nowrap;
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: 999px;
}

.level-badge__dot {
  flex: none;
  width: 8px;
  height: 8px;
  border: 2px solid transparent;
  border-radius: 50%;
}

.score {
  color: var(--kapoj-text);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  font-weight: 600;
}

.pager {
  align-self: flex-end;
  margin-top: var(--kapoj-space-3);
}

/* Scoring rules dialog (teleported, so the shell width is global). */
:global(.rules-dialog) {
  width: min(92vw, 720px);
}

.rules-sections {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.rules-section {
  --rule-accent: var(--kapoj-primary);

  padding: var(--kapoj-space-3) var(--kapoj-space-4);
  background: color-mix(in srgb, var(--rule-accent) 7%, var(--kapoj-surface));
  border: 1px solid color-mix(in srgb, var(--rule-accent) 22%, var(--kapoj-border));
  border-left: 3px solid var(--rule-accent);
  border-radius: var(--kapoj-radius-md);
}

.rules-section__head {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.rules-section__icon {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  color: var(--rule-accent);
  font-size: 0.8rem;
  background: color-mix(in srgb, var(--rule-accent) 14%, transparent);
  border: 1px solid color-mix(in srgb, var(--rule-accent) 32%, transparent);
  border-radius: var(--kapoj-radius-sm);
}

.rules-section__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 0.95rem;
  font-weight: 600;
}

.rules-section__items {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-left: 1.15rem;
  margin: var(--kapoj-space-2) 0 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
  line-height: 1.55;
}

.rules-section__items li::marker {
  color: var(--rule-accent);
}

@media (max-width: 640px) {
  :global(.rules-dialog) {
    width: 100%;
  }

  .rules-section {
    padding: var(--kapoj-space-3);
  }
}

@media (max-width: 1199px) {
  .boards-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 767px) {
  .boards-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 640px) {
  .leaderboard-page {
    gap: var(--kapoj-space-4);
  }

  .board-card__english {
    display: none;
  }

  .board-row__score {
    text-align: right;
  }

  .pager {
    align-self: stretch;
  }
}

@media (prefers-reduced-motion: reduce) {
  .board-card {
    animation: none;
    transition: none;
  }

  .board-card:hover,
  .board-card:focus-visible {
    transform: none;
  }
}
</style>
