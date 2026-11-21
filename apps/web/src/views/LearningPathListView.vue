<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import DatePicker from 'primevue/datepicker'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Paginator from 'primevue/paginator'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'

import { ApiError } from '@/api/http'
import { listLearningPaths, type LearningPathScope } from '@/api/learning'
import type { LearningPathItem } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import RatingLevelName from '@/components/RatingLevelName.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'
import { learningPathVisibilityLabel } from '@/utils/learning'

const PAGE_SIZE = 12

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const items = ref<LearningPathItem[]>([])
const filter = ref<LearningPathScope>(scopeFromQuery())
const keyword = ref(typeof route.query.keyword === 'string' ? route.query.keyword : '')
const createdFrom = ref<Date | null>(dateFromQuery(route.query.createdFrom))
const createdTo = ref<Date | null>(dateFromQuery(route.query.createdTo))
const page = ref(positivePage(route.query.page))
const total = ref(0)
const loading = ref(false)
const error = ref('')

const isLoggedIn = computed(() => auth.isLoggedIn)
const canManage = computed(() => auth.can(PERM.learningPath.official.manage))

const filterOptions = computed(() => {
  const options: { label: string; value: LearningPathScope }[] = [
    { label: '全部', value: 'all' },
    { label: '官方', value: 'official' },
    { label: '自定义', value: 'custom' },
  ]
  if (isLoggedIn.value) options.push({ label: '我创建的', value: 'mine' })
  return options
})

let latest = 0

function scopeFromQuery(): LearningPathScope {
  const value = route.query.scope
  return value === 'official' || value === 'custom' || value === 'mine' ? value : 'all'
}

function positivePage(raw: unknown): number {
  const value = Number(typeof raw === 'string' ? raw : 1)
  return Number.isInteger(value) && value > 0 ? value : 1
}

function dateFromQuery(raw: unknown): Date | null {
  if (typeof raw !== 'string' || !/^\d{4}-\d{2}-\d{2}$/.test(raw)) return null
  const [year, month, day] = raw.split('-').map(Number)
  const date = new Date(year!, month! - 1, day!)
  return Number.isNaN(date.getTime()) ? null : date
}

function dateParam(value: Date | null): string | undefined {
  if (!value) return undefined
  const year = value.getFullYear()
  const month = String(value.getMonth() + 1).padStart(2, '0')
  const day = String(value.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function syncQuery() {
  void router.replace({
    name: 'learning-paths',
    query: {
      scope: filter.value === 'all' ? undefined : filter.value,
      keyword: keyword.value.trim() || undefined,
      createdFrom: dateParam(createdFrom.value),
      createdTo: dateParam(createdTo.value),
      page: page.value > 1 ? String(page.value) : undefined,
    },
  })
}

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const data = await listLearningPaths({
      scope: filter.value,
      keyword: keyword.value.trim() || undefined,
      createdFrom: dateParam(createdFrom.value),
      createdTo: dateParam(createdTo.value),
      page: page.value,
      pageSize: PAGE_SIZE,
    })
    if (seq !== latest) return
    items.value = data.items
    total.value = data.total
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载学习路线失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

function applyFilters() {
  page.value = 1
  syncQuery()
  void load()
}

function resetFilters() {
  filter.value = 'all'
  keyword.value = ''
  createdFrom.value = null
  createdTo.value = null
  applyFilters()
}

function onPage(event: { page: number }) {
  page.value = event.page + 1
  syncQuery()
  void load()
}

function canEdit(item: LearningPathItem): boolean {
  return item.canEdit || item.author.id === auth.user?.id || canManage.value
}

function visibilitySeverity(item: LearningPathItem): 'info' | 'warn' {
  if (item.isPublic) return 'info'
  return 'warn'
}

function priceLabel(item: LearningPathItem): string {
  return item.priceCoins > 0 ? `${item.priceCoins} 金币` : '免费'
}

watch(filter, () => applyFilters())

onMounted(load)
</script>

<template>
  <div class="learning-paths-page">
    <AppPageHeader
      eyebrow="LEARNING // PATHS"
      title="学习路线"
      description="跟随官方或社区路线，按阶段循序渐进地掌握算法与数据结构。"
    >
      <template #actions>
        <span v-if="!loading || total > 0" class="result-count">共 {{ total }} 条路线</span>
        <Button
          v-if="isLoggedIn"
          as="router-link"
          to="/learning-paths/new"
          label="新建路线"
          icon="pi pi-plus"
        />
      </template>
    </AppPageHeader>

    <div class="toolbar">
      <SelectButton
        v-model="filter"
        :options="filterOptions"
        option-label="label"
        option-value="value"
        :allow-empty="false"
      />
    </div>

    <form class="filters" @submit.prevent="applyFilters">
      <InputText
        v-model="keyword"
        data-test="path-keyword"
        placeholder="搜索标题或简介"
        maxlength="100"
        fluid
      />
      <DatePicker
        v-model="createdFrom"
        data-test="path-created-from"
        placeholder="创建起始日"
        date-format="yy-mm-dd"
        show-icon
        fluid
      />
      <DatePicker
        v-model="createdTo"
        data-test="path-created-to"
        placeholder="创建结束日"
        date-format="yy-mm-dd"
        show-icon
        fluid
      />
      <div class="filters__actions">
        <Button type="submit" label="搜索" icon="pi pi-search" />
        <Button type="button" label="重置" severity="secondary" text @click="resetFilters" />
      </div>
    </form>

    <AppSurface padding="lg" class="learning-paths-surface">
      <AppErrorState v-if="error && !items.length" :description="error" @retry="load" />

      <AppLoadingState v-else-if="loading && !items.length" label="正在加载学习路线" />

      <AppEmptyState
        v-else-if="!error && !items.length"
        icon="pi pi-sitemap"
        title="暂无学习路线"
        description="当前筛选条件下还没有可展示的学习路线。"
      />

      <template v-else>
        <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>

        <div class="path-grid">
          <AppSurface
            v-for="item in items"
            :key="item.id"
            as="article"
            padding="md"
            interactive
            class="path-card"
          >
            <RouterLink :to="`/learning-paths/${item.id}`" class="path-cover">
              <img
                v-if="item.coverUrl"
                :src="item.coverUrl"
                :alt="item.title"
                class="path-cover__img"
              />
              <span v-else class="path-cover__placeholder" aria-hidden="true">
                <i class="pi pi-sitemap" />
              </span>
            </RouterLink>

            <div class="path-body">
              <div class="path-head">
                <RouterLink :to="`/learning-paths/${item.id}`" class="path-title">
                  {{ item.title }}
                </RouterLink>
                <div class="path-tags">
                  <Tag
                    :value="learningPathVisibilityLabel(item)"
                    :severity="visibilitySeverity(item)"
                  />
                  <Tag
                    :value="priceLabel(item)"
                    :severity="item.priceCoins > 0 ? 'warn' : 'success'"
                    data-test="path-price"
                  />
                  <Tag
                    v-if="item.priceCoins > 0"
                    :value="item.purchased ? '已购买' : '未购买'"
                    :severity="item.purchased ? 'success' : 'secondary'"
                    data-test="path-purchased"
                  />
                </div>
              </div>

              <p class="path-desc">{{ item.descriptionMd }}</p>

              <div class="path-meta">
                <span class="path-author"><RatingLevelName :name="item.author.nickname || item.author.username" :level="item.author.level" /></span>
                <span class="path-progress"
                  >已完成 {{ item.myCompletedCount }} / {{ item.nodeCount }} 节点</span
                >
                <span class="path-social" data-test="path-social"
                  >参与 {{ item.participantCount }} · 推荐 {{ item.recommendCount }}</span
                >
              </div>

              <RouterLink
                v-if="canEdit(item)"
                :to="`/learning-paths/${item.id}/edit`"
                class="path-edit"
              >
                编辑
              </RouterLink>
            </div>
          </AppSurface>
        </div>

        <Paginator
          v-if="total > PAGE_SIZE"
          :rows="PAGE_SIZE"
          :total-records="total"
          :first="(page - 1) * PAGE_SIZE"
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
.learning-paths-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.toolbar {
  display: flex;
}

.filters {
  display: grid;
  grid-template-columns: minmax(180px, 1fr) repeat(2, minmax(140px, 180px)) auto;
  gap: var(--kapoj-space-2);
  align-items: center;
}

.filters__actions {
  display: flex;
  gap: var(--kapoj-space-1);
  align-items: center;
}

.path-tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-1);
  justify-content: flex-end;
}

.path-social {
  color: var(--kapoj-text-muted);
}

.pager {
  margin-top: var(--kapoj-space-3);
}

.result-count {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
}

.learning-paths-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.path-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: var(--kapoj-space-4);
}

.path-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
  overflow: hidden;
}

.path-cover {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 120px;
  margin: calc(var(--kapoj-space-4) * -1) calc(var(--kapoj-space-4) * -1) 0;
  color: inherit;
  text-decoration: none;
  background: linear-gradient(135deg, var(--kapoj-primary-soft), var(--kapoj-bg-elevated));
  overflow: hidden;
}

.path-cover__img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.path-cover__placeholder {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  color: var(--kapoj-primary);
  font-size: 1.5rem;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 28%, transparent);
  border-radius: var(--kapoj-radius-md);
}

.path-body {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  min-width: 0;
}

.path-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
}

.path-title {
  min-width: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  font-weight: 600;
  line-height: 1.35;
  text-decoration: none;
  word-break: break-word;
}

.path-title:hover {
  color: var(--kapoj-primary);
  text-decoration: underline;
}

.path-desc {
  display: -webkit-box;
  margin: 0;
  overflow: hidden;
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
  line-height: 1.6;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.path-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.path-author {
  color: var(--kapoj-text-secondary);
}

.path-edit {
  align-self: flex-start;
  color: var(--kapoj-primary);
  font-size: 12px;
  font-weight: 600;
  text-decoration: none;
}

.path-edit:hover {
  text-decoration: underline;
}

@media (max-width: 860px) {
  .filters {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  }

  .filters__actions {
    grid-column: 1 / -1;
  }
}

@media (max-width: 640px) {
  .path-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .filters {
    grid-template-columns: minmax(0, 1fr);
  }

  .path-head {
    flex-direction: column;
  }

  .path-tags {
    justify-content: flex-start;
  }
}
</style>
