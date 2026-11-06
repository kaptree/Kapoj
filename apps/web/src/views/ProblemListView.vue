<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import InputText from 'primevue/inputtext'
import Paginator from 'primevue/paginator'
import Select from 'primevue/select'
import ToggleSwitch from 'primevue/toggleswitch'

import { ApiError } from '@/api/http'
import { listProblems } from '@/api/problems'
import type { ProblemListItem } from '@/api/types'
import AppDifficultyTag from '@/components/ui/AppDifficultyTag.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'
import { DIFFICULTIES } from '@/utils/difficulty'
import { parseProblemQuery, toRouteQuery } from '@/utils/problemQuery'

const PAGE_SIZE = 20

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const canCreate = computed(() => auth.can(PERM.problem.create))
const canViewHidden = computed(() => auth.can(PERM.problem.viewHidden))
const query = computed(() => parseProblemQuery(route.query))

const filters = ref({ keyword: '', tag: '', difficulty: null as number | null })
const showHidden = ref(false)
const items = ref<ProblemListItem[]>([])
const total = ref(0)
const loading = ref(false)
const error = ref('')

const first = computed(() => (query.value.page - 1) * PAGE_SIZE)

// Only the newest request may update the page; slow older ones are ignored.
let latest = 0

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const page = await listProblems({
      ...query.value,
      includeHidden: canViewHidden.value && showHidden.value,
      pageSize: PAGE_SIZE,
    })
    if (seq !== latest) return
    items.value = page.items
    total.value = page.total
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载题目失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

watch(
  () => route.query,
  () => {
    const q = query.value
    filters.value.keyword = q.keyword
    filters.value.tag = q.tag
    filters.value.difficulty = q.difficulty
    void load()
  },
  { immediate: true },
)
watch(showHidden, () => void load())

function go(next: Partial<ReturnType<typeof parseProblemQuery>>) {
  void router.push({ name: 'problems', query: toRouteQuery({ ...query.value, ...next }) })
}

function search() {
  go({
    keyword: filters.value.keyword.trim(),
    tag: filters.value.tag.trim(),
    difficulty: typeof filters.value.difficulty === 'number' ? filters.value.difficulty : null,
    page: 1,
  })
}

function reset() {
  go({ keyword: '', tag: '', difficulty: null, page: 1 })
}

function onPage(event: { page: number }) {
  go({ page: event.page + 1 })
}

function rate(p: ProblemListItem) {
  return p.submitCount === 0 ? '-' : `${Math.round((p.acceptCount / p.submitCount) * 100)}%`
}
</script>

<template>
  <div class="problem-page">
    <AppPageHeader
      eyebrow="PROBLEMS // ARCHIVE"
      title="题库"
      description="筛选题目、查看难度与通过率，开始一次练习。"
    >
      <template #actions>
        <span v-if="!loading || total > 0" class="result-count">{{ total }} 道题</span>
        <Button
          v-if="canCreate"
          as="router-link"
          :to="{ name: 'problem-create' }"
          label="新建题目"
          icon="pi pi-plus"
        />
      </template>
    </AppPageHeader>

    <AppSurface padding="lg" class="problem-surface">
      <form class="filter-bar" role="search" @submit.prevent="search">
        <div class="filter-fields">
          <InputText
            v-model="filters.keyword"
            fluid
            placeholder="搜索标题或标识"
            aria-label="搜索标题或标识"
          />
          <InputText v-model="filters.tag" fluid placeholder="标签" aria-label="按标签筛选" />
          <Select
            v-model="filters.difficulty"
            fluid
            :options="DIFFICULTIES"
            option-label="label"
            option-value="value"
            placeholder="难度"
            show-clear
            aria-label="按难度筛选"
          />
        </div>
        <div class="filter-actions">
          <Button type="submit" label="搜索" icon="pi pi-search" />
          <Button type="button" label="重置" severity="secondary" text @click="reset" />
        </div>
        <div v-if="canViewHidden" class="filter-admin">
          <ToggleSwitch v-model="showHidden" input-id="show-hidden" />
          <label for="show-hidden">显示未公开</label>
        </div>
      </form>

      <AppErrorState v-if="error" :description="error" class="page-error" @retry="load" />

      <AppLoadingState v-if="loading && items.length === 0" label="正在加载题目" />
      <DataTable
        v-else-if="!error || items.length > 0"
        :value="items"
        :loading="loading"
        data-key="id"
        size="small"
        row-hover
        responsive-layout="scroll"
        class="problem-table"
      >
        <Column header="编号" style="width: 64px">
          <template #body="{ data }">
            <span class="num">{{ data.id }}</span>
          </template>
        </Column>
        <Column header="题目" style="min-width: 190px">
          <template #body="{ data }">
            <RouterLink
              :to="{ name: 'problem-detail', params: { id: data.id } }"
              class="problem-link"
            >
              {{ data.title }}
            </RouterLink>
            <span v-if="!data.isPublic" class="private-tag">未公开</span>
          </template>
        </Column>
        <Column header="难度" style="width: 84px">
          <template #body="{ data }">
            <AppDifficultyTag :difficulty="data.difficulty" />
          </template>
        </Column>
        <Column header="标签" style="min-width: 120px">
          <template #body="{ data }">
            <div class="tag-list">
              <button
                v-for="t in data.tags"
                :key="t"
                type="button"
                class="tag-chip"
                @click="go({ tag: t, page: 1 })"
              >
                {{ t }}
              </button>
            </div>
          </template>
        </Column>
        <Column header="通过 / 提交" style="width: 140px">
          <template #body="{ data }">
            <span class="num">{{ data.acceptCount }} / {{ data.submitCount }}</span>
            <span class="rate">（{{ rate(data) }}）</span>
          </template>
        </Column>
        <template #empty>
          <AppEmptyState
            icon="pi pi-search"
            title="没有找到符合条件的题目"
            description="调整筛选条件，或清空搜索后重试。"
          >
            <template #actions>
              <Button label="重置筛选" severity="secondary" outlined @click="reset" />
            </template>
          </AppEmptyState>
        </template>
      </DataTable>

      <Paginator
        v-if="total > PAGE_SIZE"
        :rows="PAGE_SIZE"
        :total-records="total"
        :first="first"
        :page-link-size="5"
        template="PrevPageLink PageLinks NextPageLink"
        class="pager"
        @page="onPage"
      />
    </AppSurface>
  </div>
</template>

<style scoped>
.problem-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
  max-width: var(--kapoj-content-max);
}

.problem-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.result-count {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
}

.filter-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: var(--kapoj-space-3);
}

.filter-fields {
  display: grid;
  flex: 1 1 520px;
  grid-template-columns: minmax(180px, 1.4fr) minmax(140px, 0.8fr) minmax(140px, 0.6fr);
  gap: var(--kapoj-space-2);
}

.filter-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-1);
}

.filter-admin {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  margin-left: auto;
  color: var(--kapoj-text-secondary);
  font-size: 13px;
}

.filter-admin label {
  cursor: pointer;
}

.page-error {
  margin-bottom: var(--kapoj-space-1);
}

.problem-table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.problem-table :deep(.p-datatable-table) {
  min-width: 610px;
}

.problem-table :deep(.p-datatable-thead > tr > th) {
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

.problem-table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.problem-link {
  color: var(--kapoj-text);
  font-weight: 600;
}

.problem-link:hover {
  color: var(--kapoj-primary);
}

.private-tag {
  display: inline-flex;
  margin-left: var(--kapoj-space-2);
  padding: 1px 6px;
  color: var(--kapoj-warning);
  font-size: 10px;
  border: 1px solid color-mix(in srgb, var(--kapoj-warning) 32%, transparent);
  border-radius: var(--kapoj-radius-xs);
}

.tag-list {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-1);
}

.tag-chip {
  padding: 1px 7px;
  color: var(--kapoj-primary);
  font: inherit;
  font-size: 11px;
  cursor: pointer;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 28%, transparent);
  border-radius: var(--kapoj-radius-xs);
  transition:
    color var(--kapoj-transition),
    background var(--kapoj-transition),
    border-color var(--kapoj-transition);
}

.tag-chip:hover {
  color: var(--kapoj-text);
  background: color-mix(in srgb, var(--kapoj-primary) 18%, transparent);
  border-color: var(--kapoj-primary);
}

.rate {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.pager {
  align-self: flex-end;
  margin-top: var(--kapoj-space-1);
}

@media (max-width: 900px) {
  .filter-fields {
    flex-basis: 100%;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .filter-fields > :nth-child(3) {
    grid-column: span 2;
  }

  .filter-admin {
    margin-left: 0;
  }
}

@media (max-width: 600px) {
  .problem-page {
    gap: var(--kapoj-space-4);
  }

  .filter-fields {
    grid-template-columns: minmax(0, 1fr);
  }

  .filter-fields > :nth-child(3) {
    grid-column: auto;
  }

  .filter-actions {
    width: 100%;
  }

  .filter-actions :deep(.p-button) {
    flex: 1 1 auto;
  }

  .filter-admin {
    width: 100%;
  }

  .pager {
    align-self: stretch;
  }
}

@media (prefers-reduced-motion: reduce) {
  .tag-chip {
    transition: none;
  }
}
</style>
