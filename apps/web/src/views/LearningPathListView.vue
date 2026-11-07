<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'

import { ApiError } from '@/api/http'
import { listLearningPaths } from '@/api/learning'
import type { LearningPathItem } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'
import { learningPathVisibilityLabel } from '@/utils/learning'

type Filter = 'all' | 'official' | 'custom' | 'mine'

const auth = useAuthStore()

const items = ref<LearningPathItem[]>([])
const mineItems = ref<LearningPathItem[]>([])
const filter = ref<Filter>('all')
const loading = ref(false)
const error = ref('')

const isLoggedIn = computed(() => auth.isLoggedIn)
const canManage = computed(() => auth.can(PERM.learningPath.official.manage))

const filterOptions = computed(() => {
  const options: { label: string; value: Filter }[] = [
    { label: '全部', value: 'all' },
    { label: '官方', value: 'official' },
    { label: '自定义', value: 'custom' },
  ]
  if (isLoggedIn.value) options.push({ label: '我创建的', value: 'mine' })
  return options
})

const visibleItems = computed<LearningPathItem[]>(() => {
  if (filter.value === 'mine') return mineItems.value
  if (filter.value === 'official') return items.value.filter((item) => item.isOfficial)
  if (filter.value === 'custom') return items.value.filter((item) => !item.isOfficial)
  return items.value
})

const total = computed(() => visibleItems.value.length)

let latest = 0

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  const mine = filter.value === 'mine'
  try {
    const data = await listLearningPaths(mine ? { mine: true } : {})
    if (seq !== latest) return
    if (mine) mineItems.value = data.items
    else items.value = data.items
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载学习路线失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

function canEdit(item: LearningPathItem): boolean {
  return item.author.id === auth.user?.id || canManage.value
}

function visibilitySeverity(item: LearningPathItem): 'info' | 'warn' {
  if (item.isPublic) return 'info'
  return 'warn'
}

onMounted(load)

watch(filter, (next, prev) => {
  // Only re-fetch when the data source changes (all <-> mine); 官方/自定义 are
  // filtered client-side from the already loaded list.
  if (next === 'mine' || prev === 'mine') void load()
})
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

    <AppSurface padding="lg" class="learning-paths-surface">
      <AppErrorState v-if="error && !visibleItems.length" :description="error" @retry="load" />

      <AppLoadingState v-if="loading && !visibleItems.length" label="正在加载学习路线" />

      <AppEmptyState
        v-else-if="!error && !visibleItems.length"
        icon="pi pi-sitemap"
        title="暂无学习路线"
        description="当前筛选条件下还没有可展示的学习路线。"
      />

      <template v-else>
        <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>

        <div class="path-grid">
          <AppSurface
            v-for="item in visibleItems"
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
                <Tag
                  :value="learningPathVisibilityLabel(item)"
                  :severity="visibilitySeverity(item)"
                />
              </div>

              <p class="path-desc">{{ item.descriptionMd }}</p>

              <div class="path-meta">
                <span class="path-author">{{ item.author.nickname || item.author.username }}</span>
                <span class="path-progress"
                  >已完成 {{ item.myCompletedCount }} / {{ item.nodeCount }} 节点</span
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

@media (max-width: 640px) {
  .path-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
