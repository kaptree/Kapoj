<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import { deleteLearningPath, listLearningPaths, updateLearningPath } from '@/api/learning'
import type { LearningPathItem } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime } from '@/utils/format'

type TypeFilter = 'all' | 'official' | 'custom'
type StatusFilter = 'all' | 'active' | 'inactive'

const TYPE_OPTIONS: { label: string; value: TypeFilter }[] = [
  { label: '全部', value: 'all' },
  { label: '官方', value: 'official' },
  { label: '自定义', value: 'custom' },
]

const STATUS_OPTIONS: { label: string; value: StatusFilter }[] = [
  { label: '全部', value: 'all' },
  { label: '上架', value: 'active' },
  { label: '已下架', value: 'inactive' },
]

const auth = useAuthStore()
const toast = useToast()
const confirm = useConfirm()

const canManage = computed(() => auth.can(PERM.learningPath.official.manage))

const items = ref<LearningPathItem[]>([])
const typeFilter = ref<TypeFilter>('all')
const statusFilter = ref<StatusFilter>('all')
const loading = ref(false)
const error = ref('')

const filteredItems = computed<LearningPathItem[]>(() =>
  items.value.filter((item) => {
    if (typeFilter.value === 'official' && !item.isOfficial) return false
    if (typeFilter.value === 'custom' && item.isOfficial) return false
    if (statusFilter.value === 'active' && !item.isActive) return false
    if (statusFilter.value === 'inactive' && item.isActive) return false
    return true
  }),
)

let latest = 0

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const data = await listLearningPaths({ includeInactive: true })
    if (seq !== latest) return
    items.value = data.items
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载学习路线失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

function fail(e: unknown, fallback: string) {
  toast.add({
    severity: 'error',
    summary: '操作失败',
    detail: e instanceof ApiError ? e.message : fallback,
    life: 4000,
  })
}

function typeSeverity(item: LearningPathItem): 'primary' | 'secondary' {
  return item.isOfficial ? 'primary' : 'secondary'
}

function visibilitySeverity(item: LearningPathItem): 'info' | 'warn' {
  return item.isPublic ? 'info' : 'warn'
}

function confirmDown(item: LearningPathItem) {
  confirm.require({
    header: '下架学习路线',
    message: `确定下架学习路线「${item.title}」吗？下架后普通用户将无法查看该路线。`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '下架',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => void doDown(item),
  })
}

async function doDown(item: LearningPathItem) {
  try {
    await deleteLearningPath(item.id)
    toast.add({ severity: 'success', summary: '路线已下架', life: 3000 })
    void load()
  } catch (e) {
    fail(e, '下架失败')
  }
}

function confirmRestore(item: LearningPathItem) {
  confirm.require({
    header: '恢复学习路线',
    message: `确定恢复学习路线「${item.title}」吗？`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '恢复',
    rejectLabel: '取消',
    acceptProps: { severity: 'success' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => void doRestore(item),
  })
}

async function doRestore(item: LearningPathItem) {
  try {
    await updateLearningPath(item.id, {
      title: item.title,
      descriptionMd: item.descriptionMd,
      coverUrl: item.coverUrl,
      isPublic: item.isPublic,
      sortOrder: item.sortOrder,
      isActive: true,
    })
    toast.add({ severity: 'success', summary: '路线已恢复上架', life: 3000 })
    void load()
  } catch (e) {
    fail(e, '恢复失败')
  }
}

onMounted(load)
</script>

<template>
  <div class="admin-learning-paths">
    <AppPageHeader
      eyebrow="ADMIN // LEARNING PATHS"
      title="学习路线管理"
      description="管理全部学习路线（含下架路线），新建官方路线或编辑、下架、恢复任意路线。"
    >
      <template #actions>
        <Button
          v-if="canManage"
          as="router-link"
          to="/learning-paths/new?official=true"
          label="新建官方路线"
          icon="pi pi-plus"
        />
      </template>
    </AppPageHeader>

    <AppSurface padding="lg" class="admin-learning-paths__surface">
      <div class="filters">
        <div class="filters__group">
          <span class="filters__label">类型</span>
          <SelectButton
            v-model="typeFilter"
            :options="TYPE_OPTIONS"
            option-label="label"
            option-value="value"
            :allow-empty="false"
            aria-label="按类型筛选"
          />
        </div>
        <div class="filters__group">
          <span class="filters__label">状态</span>
          <SelectButton
            v-model="statusFilter"
            :options="STATUS_OPTIONS"
            option-label="label"
            option-value="value"
            :allow-empty="false"
            aria-label="按状态筛选"
          />
        </div>
      </div>

      <AppErrorState v-if="error && items.length === 0" :description="error" @retry="load" />

      <AppLoadingState v-if="loading && items.length === 0" label="正在加载学习路线" />

      <DataTable
        v-else-if="!error || items.length > 0"
        :value="filteredItems"
        :loading="loading"
        data-key="id"
        size="small"
        responsive-layout="scroll"
        class="admin-learning-paths__table"
      >
        <Column header="标题" style="min-width: 200px">
          <template #body="{ data }">
            <span class="title">{{ data.title }}</span>
          </template>
        </Column>
        <Column header="类型" style="width: 90px">
          <template #body="{ data }">
            <Tag :value="data.isOfficial ? '官方' : '自定义'" :severity="typeSeverity(data)" />
          </template>
        </Column>
        <Column header="可见性" style="width: 90px">
          <template #body="{ data }">
            <Tag :value="data.isPublic ? '公开' : '私有'" :severity="visibilitySeverity(data)" />
          </template>
        </Column>
        <Column header="状态" style="width: 90px">
          <template #body="{ data }">
            <Tag
              :value="data.isActive ? '上架' : '已下架'"
              :severity="data.isActive ? 'success' : 'danger'"
            />
          </template>
        </Column>
        <Column header="作者" style="width: 130px">
          <template #body="{ data }">
            <span class="muted">{{ data.author.nickname || data.author.username }}</span>
          </template>
        </Column>
        <Column header="节点数" style="width: 80px">
          <template #body="{ data }">
            <span class="num">{{ data.nodeCount }}</span>
          </template>
        </Column>
        <Column header="我的完成" style="width: 90px">
          <template #body="{ data }">
            <span class="num">{{ data.myCompletedCount }}</span>
          </template>
        </Column>
        <Column header="更新时间" style="width: 180px">
          <template #body="{ data }">
            <span class="muted">{{ formatDateTime(data.updatedAt) }}</span>
          </template>
        </Column>
        <Column v-if="canManage" header="操作" style="width: 190px">
          <template #body="{ data }">
            <div class="row-actions">
              <Button
                as="router-link"
                :to="`/learning-paths/${data.id}/edit`"
                label="编辑"
                text
                size="small"
              />
              <Button
                v-if="data.isActive"
                label="下架"
                text
                size="small"
                severity="danger"
                @click="confirmDown(data)"
              />
              <Button
                v-else
                label="恢复"
                text
                size="small"
                severity="success"
                @click="confirmRestore(data)"
              />
            </div>
          </template>
        </Column>
        <template #empty>
          <AppEmptyState
            icon="pi pi-sitemap"
            title="暂无学习路线"
            description="调整筛选条件，或新建官方路线。"
          />
        </template>
      </DataTable>
    </AppSurface>
  </div>
</template>

<style scoped>
.admin-learning-paths {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.admin-learning-paths__surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
}

.filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-4);
}

.filters__group {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.filters__label {
  color: var(--kapoj-text-secondary);
  font-size: 11px;
  font-weight: 600;
}

.admin-learning-paths__table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.admin-learning-paths__table :deep(.p-datatable-table) {
  min-width: 960px;
}

.admin-learning-paths__table :deep(.p-datatable-thead > tr > th) {
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

.admin-learning-paths__table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.title {
  color: var(--kapoj-text);
  font-weight: 600;
}

.row-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 2px;
}

.muted {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.num {
  font-variant-numeric: tabular-nums;
}

@media (max-width: 640px) {
  .filters {
    flex-direction: column;
    align-items: stretch;
  }

  .filters__group {
    flex-wrap: wrap;
  }
}
</style>
