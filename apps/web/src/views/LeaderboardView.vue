<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Message from 'primevue/message'
import Paginator from 'primevue/paginator'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import TabPanel from 'primevue/tabpanel'
import TabPanels from 'primevue/tabpanels'
import Tabs from 'primevue/tabs'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import {
  getActivitySummary,
  listActivityLeaderboard,
  listRatingLeaderboard,
  listSharingLeaderboard,
  signInActivity,
} from '@/api/leaderboard'
import type { ActivitySummary, EquippedBadge, EquippedTitle, RatingLevel } from '@/api/types'
import IdentityShowcase from '@/components/IdentityShowcase.vue'
import RatingLevelName from '@/components/RatingLevelName.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useAuthStore } from '@/stores/auth'

const PAGE_SIZE = 20

type TabKey = 'rating' | 'activity' | 'sharing'

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

interface TabState {
  page: number
  rows: LeaderboardRow[]
  total: number
  loading: boolean
  loaded: boolean
  error: string
  seq: number
}

interface TabDef {
  key: TabKey
  label: string
  emptyTitle: string
  emptyDescription: string
}

const tabDefs: TabDef[] = [
  {
    key: 'rating',
    label: 'Rating',
    emptyTitle: '暂无 Rating 排行',
    emptyDescription: '参与并完成 Rated 比赛后即可登上 Rating 榜。',
  },
  {
    key: 'activity',
    label: '活跃度',
    emptyTitle: '暂无活跃度排行',
    emptyDescription: '提交代码、参加比赛或每日签到即可获得活跃度。',
  },
  {
    key: 'sharing',
    label: '共享',
    emptyTitle: '暂无共享排行',
    emptyDescription: '创建或克隆比赛即可获得共享积分。',
  },
]

function createState(): TabState {
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

const route = useRoute()
const auth = useAuthStore()
const toast = useToast()

const activeTab = ref<TabKey>('rating')
const states: Record<TabKey, TabState> = {
  rating: reactive(createState()),
  activity: reactive(createState()),
  sharing: reactive(createState()),
}

const summary = ref<ActivitySummary | null>(null)
const summaryLoading = ref(false)
const summaryError = ref('')
const signingIn = ref(false)
let summarySeq = 0

const isLoggedIn = computed(() => auth.isLoggedIn)
const loginTarget = computed(() => ({ name: 'login', query: { redirect: route.fullPath } }))

async function fetchPage(kind: TabKey, page: number) {
  const state = states[kind]
  const seq = ++state.seq
  state.page = page
  state.loading = true
  state.error = ''
  try {
    const data =
      kind === 'rating'
        ? await listRatingLeaderboard({ page, pageSize: PAGE_SIZE })
        : kind === 'activity'
          ? await listActivityLeaderboard({ page, pageSize: PAGE_SIZE })
          : await listSharingLeaderboard({ page, pageSize: PAGE_SIZE })
    if (seq !== state.seq) return
    state.rows = data.items
    state.total = data.total
    state.loaded = true
  } catch (e) {
    if (seq !== state.seq) return
    state.error = e instanceof ApiError ? e.message : '加载排行榜失败，请稍后再试'
  } finally {
    if (seq === state.seq) state.loading = false
  }
}

function stateOf(kind: TabKey): TabState {
  return states[kind]
}

function onPageChange(kind: TabKey, event: { page: number }) {
  void fetchPage(kind, event.page + 1)
}

function formatScore(kind: TabKey, score: number): string {
  return kind === 'rating' ? String(score) : score.toLocaleString('zh-CN')
}

async function loadSummary() {
  if (!auth.isLoggedIn) {
    summarySeq += 1
    summary.value = null
    summaryError.value = ''
    summaryLoading.value = false
    return
  }
  const seq = ++summarySeq
  summaryLoading.value = true
  summaryError.value = ''
  try {
    const data = await getActivitySummary()
    if (seq !== summarySeq) return
    summary.value = data
  } catch (e) {
    if (seq !== summarySeq) return
    summaryError.value = e instanceof ApiError ? e.message : '加载活跃度摘要失败，请稍后再试'
  } finally {
    if (seq === summarySeq) summaryLoading.value = false
  }
}

async function signIn() {
  if (!auth.isLoggedIn || signingIn.value || summary.value?.signedInToday) return
  signingIn.value = true
  try {
    const result = await signInActivity()
    if (result.awarded) {
      toast.add({
        severity: 'success',
        summary: '签到成功',
        detail: `+${result.activityPoints} 活跃度 / +${result.coins} 金币`,
        life: 4000,
      })
    } else {
      toast.add({
        severity: 'info',
        summary: '今日已签到',
        life: 3000,
      })
    }
    // The server is authoritative for coins and points; refresh the cached user
    // instead of adding balances locally.
    await auth.fetchMe().catch(() => undefined)
    await Promise.all([loadSummary(), fetchPage('activity', 1)])
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '签到失败',
      detail: e instanceof ApiError ? e.message : '签到失败，请稍后再试',
      life: 4000,
    })
  } finally {
    signingIn.value = false
  }
}

onMounted(() => void fetchPage('rating', states.rating.page))

// Tabs load lazily: the first visit to a tab fetches its own first page.
watch(activeTab, (kind) => {
  const state = states[kind]
  if (!state.loaded && !state.loading) void fetchPage(kind, state.page)
})

watch(
  () => auth.isLoggedIn,
  () => void loadSummary(),
  { immediate: true },
)
</script>

<template>
  <div class="leaderboard-page">
    <AppPageHeader
      eyebrow="RANKS // LEADERBOARD"
      title="排行榜"
      description="Rating、活跃度与共享积分总览；游客也可以查看全部榜单。"
    />

    <AppSurface padding="lg" class="leaderboard-surface">
      <Tabs v-model:value="activeTab" lazy>
        <TabList>
          <Tab v-for="tab in tabDefs" :key="tab.key" :value="tab.key">{{ tab.label }}</Tab>
        </TabList>

        <TabPanels>
          <TabPanel v-for="tab in tabDefs" :key="tab.key" :value="tab.key">
            <section v-if="tab.key === 'activity'" class="signin-card" aria-label="每日签到">
              <template v-if="!isLoggedIn">
                <div class="signin-copy">
                  <p class="signin-title">登录后签到领金币</p>
                  <p class="signin-hint">每日签到可获得 5 活跃度与 5 金币。</p>
                </div>
                <RouterLink :to="loginTarget">
                  <Button label="登录 / 注册" icon="pi pi-sign-in" size="small" outlined />
                </RouterLink>
              </template>

              <AppLoadingState
                v-else-if="summaryLoading && !summary"
                label="正在加载活跃度"
                :rows="1"
              />

              <AppErrorState
                v-else-if="summaryError && !summary"
                :description="summaryError"
                @retry="loadSummary"
              />

              <template v-else>
                <div class="signin-stats">
                  <div class="signin-stat">
                    <span>今日活跃度</span>
                    <strong>{{ summary?.todayPoints ?? 0 }}</strong>
                  </div>
                  <div class="signin-stat">
                    <span>总活跃度</span>
                    <strong>{{ summary?.totalPoints ?? 0 }}</strong>
                  </div>
                </div>
                <div class="signin-action">
                  <Button
                    :label="summary?.signedInToday ? '今日已签到' : '签到 +5 金币'"
                    :icon="summary?.signedInToday ? 'pi pi-check' : 'pi pi-calendar-plus'"
                    size="small"
                    :loading="signingIn"
                    :disabled="!summary || summary.signedInToday || signingIn"
                    @click="signIn"
                  />
                  <span v-if="summary?.signedInToday" class="signed-hint">
                    今天已经签到过了，明天再来。
                  </span>
                </div>
              </template>
            </section>

            <AppErrorState
              v-if="stateOf(tab.key).error && stateOf(tab.key).rows.length === 0"
              :description="stateOf(tab.key).error"
              @retry="fetchPage(tab.key, stateOf(tab.key).page)"
            />

            <template v-else>
              <Message
                v-if="stateOf(tab.key).error"
                severity="error"
                :closable="false"
                class="page-error"
              >
                {{ stateOf(tab.key).error }}
              </Message>

              <AppLoadingState
                v-if="stateOf(tab.key).loading && stateOf(tab.key).rows.length === 0"
                :label="`正在加载${tab.label}排行`"
              />

              <template v-else>
                <div class="table-scroll">
                  <DataTable
                    :value="stateOf(tab.key).rows"
                    :loading="stateOf(tab.key).loading"
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
                          >
                            {{ data.nickname || data.username }}
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
                        <span class="score">{{ formatScore(tab.key, data.score) }}</span>
                      </template>
                    </Column>

                    <Column v-if="tab.key === 'rating'" header="等级" style="width: 120px">
                      <template #body="{ data }">
                        <RatingLevelName
                          v-if="data.level"
                          :name="data.nickname"
                          :level="data.level"
                        />
                      </template>
                    </Column>

                    <template #empty>
                      <AppEmptyState
                        icon="pi pi-chart-line"
                        :title="tab.emptyTitle"
                        :description="tab.emptyDescription"
                      />
                    </template>
                  </DataTable>
                </div>

                <Paginator
                  v-if="stateOf(tab.key).total > PAGE_SIZE"
                  :rows="PAGE_SIZE"
                  :total-records="stateOf(tab.key).total"
                  :first="(stateOf(tab.key).page - 1) * PAGE_SIZE"
                  :page-link-size="5"
                  template="PrevPageLink PageLinks NextPageLink"
                  class="pager"
                  @page="onPageChange(tab.key, $event)"
                />
              </template>
            </template>
          </TabPanel>
        </TabPanels>
      </Tabs>
    </AppSurface>
  </div>
</template>

<style scoped>
.leaderboard-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
  max-width: var(--kapoj-content-max);
}

.leaderboard-surface {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.signin-card {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-3) var(--kapoj-space-4);
  margin-bottom: var(--kapoj-space-4);
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 26%, var(--kapoj-border));
  border-radius: var(--kapoj-radius-md);
}

.signin-copy {
  min-width: 0;
}

.signin-title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 0.9rem;
  font-weight: 600;
}

.signin-hint {
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
}

.signin-stats {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-5);
}

.signin-stat {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 88px;
}

.signin-stat span {
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.signin-stat strong {
  color: var(--kapoj-text);
  font-family: var(--kapoj-font-mono);
  font-size: 1.15rem;
  line-height: 1.2;
}

.signin-action {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.signed-hint {
  color: var(--kapoj-text-secondary);
  font-size: 12px;
}

.page-error {
  margin-bottom: var(--kapoj-space-3);
}

.table-scroll {
  overflow-x: auto;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
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

@media (max-width: 640px) {
  .leaderboard-page {
    gap: var(--kapoj-space-4);
  }

  .signin-card {
    align-items: flex-start;
  }

  .signin-card :deep(.p-button) {
    width: 100%;
  }

  .pager {
    align-self: stretch;
  }
}
</style>
