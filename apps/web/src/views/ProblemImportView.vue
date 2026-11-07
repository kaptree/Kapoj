<script setup lang="ts">
import { computed, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Message from 'primevue/message'

import { ApiError } from '@/api/http'
import { importHydroProblems } from '@/api/problems'
import type { HydroImportResult } from '@/api/types'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'

const MAX_FILE_MB = 200
const MAX_FILE_BYTES = MAX_FILE_MB * 1024 * 1024

const auth = useAuthStore()

const canEdit = computed(() => auth.can(PERM.problem.update))

const fileInput = ref<HTMLInputElement | null>(null)
const file = ref<File | null>(null)
const uploading = ref(false)
const result = ref<HydroImportResult | null>(null)
const error = ref('')

function formatSize(bytes: number) {
  if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  if (bytes >= 1024) return `${Math.round(bytes / 1024)} KB`
  return `${bytes} B`
}

function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const picked = input.files?.[0] ?? null
  error.value = ''
  file.value = null
  if (!picked) return
  if (!picked.name.toLowerCase().endsWith('.zip')) {
    error.value = '请选择 .zip 格式的题包文件'
    input.value = ''
    return
  }
  if (picked.size > MAX_FILE_BYTES) {
    error.value = `题包文件不能超过 ${MAX_FILE_MB} MB`
    input.value = ''
    return
  }
  file.value = picked
}

async function upload() {
  if (!file.value || uploading.value) return
  uploading.value = true
  error.value = ''
  try {
    result.value = await importHydroProblems(file.value)
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '上传失败，请稍后再试'
  } finally {
    uploading.value = false
  }
}

function reset() {
  file.value = null
  result.value = null
  error.value = ''
  if (fileInput.value) fileInput.value.value = ''
}
</script>

<template>
  <div class="import-page">
    <AppPageHeader
      eyebrow="PROBLEMS // IMPORT"
      title="导入题包"
      description="上传 HydroOJ 导出的 zip 题包，批量导入为下线题目，稍后可编辑并发布。"
    >
      <template #actions>
        <Button
          as="router-link"
          :to="{ name: 'problems' }"
          label="返回题库"
          icon="pi pi-arrow-left"
          severity="secondary"
          outlined
        />
      </template>
    </AppPageHeader>

    <AppSurface padding="lg" class="import-surface">
      <Message v-if="error" severity="error" :closable="false" class="import-error">
        {{ error }}
      </Message>

      <div class="import-controls">
        <input
          ref="fileInput"
          type="file"
          accept=".zip,application/zip"
          class="file-input"
          aria-label="选择题包文件"
          @change="onFileChange"
        />
        <span v-if="file" class="file-name">{{ file.name }}（{{ formatSize(file.size) }}）</span>
        <span v-else class="file-hint">请选择 .zip 格式的题包文件</span>
        <div class="import-actions">
          <Button
            label="开始导入"
            icon="pi pi-upload"
            :loading="uploading"
            :disabled="!file || uploading"
            @click="upload"
          />
          <Button
            v-if="file && !uploading"
            label="重新选择"
            severity="secondary"
            text
            @click="reset"
          />
        </div>
      </div>

      <p class="limit-note">
        单个题包不超过 {{ MAX_FILE_MB }} MB；导入的题目默认下线，创建者为当前用户。
      </p>
    </AppSurface>

    <template v-if="result">
      <AppSurface padding="lg" class="result-surface">
        <h2 class="result-title">导入成功 {{ result.imported.length }} 道</h2>
        <DataTable
          v-if="result.imported.length"
          :value="result.imported"
          data-key="id"
          size="small"
          responsive-layout="scroll"
          class="result-table"
        >
          <Column header="标题" style="min-width: 140px">
            <template #body="{ data }">{{ data.title }}</template>
          </Column>
          <Column header="标识" style="min-width: 120px">
            <template #body="{ data }">
              <span class="mono">{{ data.slug }}</span>
            </template>
          </Column>
          <Column header="测试点数" style="width: 100px">
            <template #body="{ data }">{{ data.testcaseCount }}</template>
          </Column>
          <Column header="操作" style="width: 160px">
            <template #body="{ data }">
              <RouterLink
                v-if="canEdit"
                :to="{ name: 'problem-edit', params: { id: data.id } }"
                class="edit-link"
              >
                编辑
              </RouterLink>
              <span v-else class="offline-note">已导入（下线）</span>
            </template>
          </Column>
        </DataTable>
        <p v-else class="empty-result">没有成功导入的题目。</p>
      </AppSurface>

      <AppSurface v-if="result.failed.length" padding="lg" class="result-surface">
        <h2 class="result-title">导入失败 {{ result.failed.length }} 道</h2>
        <DataTable
          :value="result.failed"
          size="small"
          responsive-layout="scroll"
          class="result-table"
        >
          <Column header="文件夹" style="width: 120px">
            <template #body="{ data }">
              <span class="mono">{{ data.folder }}</span>
            </template>
          </Column>
          <Column header="错误" style="min-width: 180px">
            <template #body="{ data }">{{ data.error }}</template>
          </Column>
        </DataTable>
      </AppSurface>

      <div class="result-actions">
        <Button
          as="router-link"
          :to="{ name: 'problems' }"
          label="返回题库"
          icon="pi pi-arrow-left"
          severity="secondary"
          outlined
        />
        <Button label="继续导入" icon="pi pi-upload" @click="reset" />
      </div>
    </template>
  </div>
</template>

<style scoped>
.import-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
  max-width: var(--kapoj-content-max);
}

.import-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.import-error {
  margin: 0;
}

.import-controls {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-3);
}

.file-input {
  flex: 0 0 auto;
}

.file-name {
  color: var(--kapoj-text);
  font-weight: 500;
}

.file-hint {
  color: var(--kapoj-text-muted);
  font-size: 13px;
}

.import-actions {
  display: flex;
  gap: var(--kapoj-space-2);
  margin-left: auto;
}

.limit-note {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.result-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.result-title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  font-weight: 600;
}

.result-table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.result-table :deep(.p-datatable-thead > tr > th) {
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

.result-table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.mono {
  font-family: var(--kapoj-font-mono);
}

.edit-link {
  color: var(--kapoj-primary);
  font-weight: 600;
}

.offline-note {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.empty-result {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 13px;
}

.result-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
}

@media (max-width: 600px) {
  .import-actions {
    width: 100%;
    margin-left: 0;
  }

  .import-actions :deep(.p-button) {
    flex: 1 1 auto;
  }
}
</style>
