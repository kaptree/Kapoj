<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Paginator from 'primevue/paginator'
import Tag from 'primevue/tag'

import { listContests } from '@/api/contests'
import { ApiError } from '@/api/http'
import type { ContestListItem, ContestStatus } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { contestModeLabel, contestStatusLabel, registerTypeLabel } from '@/utils/contest'
import { formatDateTime } from '@/utils/format'

const PAGE_SIZE = 20
const route = useRoute()

const problemId = computed(() => {
  const id = route.params.id
  return typeof id === 'string' ? id : ''
})

const items = ref<ContestListItem[]>([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const error = ref('')

const first = computed(() => (page.value - 1) * PAGE_SIZE)

let latest = 0
let loadedProblemId = ''

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  if (problemId.value !== loadedProblemId) {
    items.value = []
    total.value = 0
  }
  try {
    const data = await listContests({
      problemId: problemId.value,
      page: page.value,
      pageSize: PAGE_SIZE,
    })
    if (seq !== latest) return
    loadedProblemId = problemId.value
    items.value = data.items
    total.value = data.total
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载比赛失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

watch(
  () => route.params.id,
  () => {
    page.value = 1
    void load()
  },
  { immediate: true },
)

function onPage(event: { page: number }) {
  page.value = event.page + 1
  void load()
}

function statusSeverity(status: ContestStatus): 'success' | 'warn' | 'info' {
  if (status === 'running') return 'success'
  if (status === 'upcoming') return 'warn'
  return 'info'
}
</script>

<template>
  <div class="contests-page">
    <AppPageHeader
      eyebrow="PROBLEM // CONTESTS"
      title="关联比赛"
      :description="`包含题目 #${problemId} 的比赛。`"
    >
      <template #actions>
        <Button
          as="router-link"
          :to="`/problems/${problemId}`"
          label="返回题目"
          icon="pi pi-arrow-left"
          severity="secondary"
          outlined
        />
      </template>
    </AppPageHeader>

    <AppSurface padding="lg" class="contests-surface">
      <AppErrorState v-if="error && !items.length" :description="error" @retry="load" />

      <AppLoadingState v-if="loading && !items.length" label="正在加载关联比赛" />

      <AppEmptyState
        v-else-if="!error && !items.length"
        icon="pi pi-trophy"
        title="暂无关联比赛"
        description="该题目还没有被收录到任何比赛。"
      />

      <template v-else>
        <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>

        <div class="contest-grid">
          <RouterLink
            v-for="contest in items"
            :key="contest.id"
            class="contest-link"
            :to="`/contests/${contest.id}`"
          >
            <AppSurface as="article" padding="md" interactive class="contest-card">
              <div class="card-head">
                <h2 class="contest-title">{{ contest.title }}</h2>
                <Tag
                  :value="contestStatusLabel(contest.status)"
                  :severity="statusSeverity(contest.status)"
                />
              </div>
              <div class="card-tags">
                <span class="chip">{{ contestModeLabel(contest.mode) }}</span>
                <span class="chip">{{ registerTypeLabel(contest.registerType) }}</span>
                <Tag v-if="contest.registered" value="已报名" severity="success" />
              </div>
              <p class="card-time">
                {{ formatDateTime(contest.startTime) }} 至 {{ formatDateTime(contest.endTime) }}
              </p>
              <p class="card-meta">
                {{ contest.problemCount }} 题 / {{ contest.participantCount }} 人
              </p>
            </AppSurface>
          </RouterLink>
        </div>

        <Paginator
          v-if="total > 0"
          :rows="PAGE_SIZE"
          :total-records="total"
          :first="first"
          :page-link-size="5"
          template="PrevPageLink PageLinks NextPageLink"
          class="pager"
          @page="onPage"
        />
      </template>
    </AppSurface>
  </div>
</template>

<style scoped>
.contests-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.contests-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.contest-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: var(--kapoj-space-4);
}

.contest-link {
  display: block;
  color: inherit;
  text-decoration: none;
}

.contest-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
}

.card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
}

.contest-title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  line-height: 1.35;
  word-break: break-word;
}

.card-tags {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.chip {
  padding: 1px 7px;
  color: var(--kapoj-text-secondary);
  font-size: 11px;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-xs);
}

.card-time {
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 12px;
  line-height: 1.6;
}

.card-meta {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.pager {
  align-self: flex-end;
}

@media (max-width: 640px) {
  .contest-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .pager {
    align-self: stretch;
  }
}
</style>
