<script setup lang="ts">
import { computed, ref } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import Tag from 'primevue/tag'

import { ApiError } from '@/api/http'
import { importProblemPackage, type ProblemImportOptions } from '@/api/problems'
import type { ProblemImportResult } from '@/api/types'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'

const MAX_FILE_MB = 200
const MAX_FILE_BYTES = MAX_FILE_MB * 1024 * 1024

const auth = useAuthStore()

const canEdit = computed(() => auth.can(PERM.problem.update))
const canPublish = computed(() => auth.can(PERM.problem.publish))

const fileInput = ref<HTMLInputElement | null>(null)
const file = ref<File | null>(null)
const uploading = ref(false)
const dragActive = ref(false)
const publishAfterImport = ref(false)
const result = ref<ProblemImportResult | null>(null)
const error = ref('')
const conflictVisible = ref(false)
const overwriteSlugs = ref<string[]>([])

function formatSize(bytes: number) {
  if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  if (bytes >= 1024) return `${Math.round(bytes / 1024)} KB`
  return `${bytes} B`
}

function pickFile(picked: File | null) {
  error.value = ''
  result.value = null
  file.value = null
  if (!picked) return
  if (!picked.name.toLowerCase().endsWith('.zip')) {
    error.value = '请选择 .zip 格式的题包文件'
    if (fileInput.value) fileInput.value.value = ''
    return
  }
  if (picked.size > MAX_FILE_BYTES) {
    error.value = `题包文件不能超过 ${MAX_FILE_MB} MB`
    if (fileInput.value) fileInput.value.value = ''
    return
  }
  file.value = picked
}

function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  pickFile(input.files?.[0] ?? null)
}

function onDrop(event: DragEvent) {
  dragActive.value = false
  pickFile(event.dataTransfer?.files?.[0] ?? null)
}

async function runImport(options: ProblemImportOptions) {
  if (!file.value || uploading.value) return
  uploading.value = true
  error.value = ''
  try {
    const next = await importProblemPackage(file.value, {
      ...options,
      publish: publishAfterImport.value,
    })
    result.value = next
    if (next.conflicts?.length) {
      overwriteSlugs.value = []
      conflictVisible.value = true
    }
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '上传失败，请稍后再试'
  } finally {
    uploading.value = false
  }
}

async function upload() {
  await runImport({ conflictPolicy: 'ask' })
}

async function applyConflicts() {
  conflictVisible.value = false
  const conflicts = result.value?.conflicts ?? []
  const selected = new Set(overwriteSlugs.value)
  const skipSlugs = conflicts.map((item) => item.slug).filter((slug) => !selected.has(slug))
  await runImport({
    conflictPolicy: 'overwrite',
    overwriteSlugs: [...selected],
    skipSlugs,
  })
}

async function skipAllConflicts() {
  conflictVisible.value = false
  await runImport({ conflictPolicy: 'skip' })
}

function reset() {
  file.value = null
  result.value = null
  error.value = ''
  conflictVisible.value = false
  overwriteSlugs.value = []
  publishAfterImport.value = false
  if (fileInput.value) fileInput.value.value = ''
}
</script>

<template>
  <div class="import-page">
    <AppPageHeader
      eyebrow="PROBLEMS // IMPORT"
      title="导入题包"
      description="上传 kapOJ 或 HydroOJ 的 zip 题包，导入后默认下线；有发布权限时可选择立即公开。"
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

      <div
        class="import-drop"
        :class="{ 'import-drop--active': dragActive, 'import-drop--busy': uploading }"
        role="button"
        tabindex="0"
        data-test="import-drop"
        @click="fileInput?.click()"
        @keydown.enter.prevent="fileInput?.click()"
        @keydown.space.prevent="fileInput?.click()"
        @dragover.prevent="dragActive = true"
        @dragleave.prevent="dragActive = false"
        @drop.prevent="onDrop"
      >
        <i class="pi pi-cloud-upload" aria-hidden="true" />
        <strong v-if="file">{{ file.name }}</strong>
        <span v-else>点击或拖拽题包到这里</span>
        <small v-if="file">{{ formatSize(file.size) }}</small>
        <small v-else>支持 .zip，单个题包不超过 {{ MAX_FILE_MB }} MB</small>
        <input
          ref="fileInput"
          type="file"
          accept=".zip,application/zip"
          class="visually-hidden"
          data-test="import-file"
          aria-label="选择题包文件"
          @click.stop
          @change="onFileChange"
        />
      </div>

      <label v-if="canPublish" class="publish-option">
        <Checkbox v-model="publishAfterImport" binary input-id="publish-after-import" />
        <span>导入后立即公开</span>
      </label>

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

      <p class="limit-note">
        导入的题目默认下线，创建者为当前用户；同名题目标识会先询问是否覆盖。勾选立即公开时，每道题独立发布，失败不会回滚已导入内容。
      </p>
    </AppSurface>

    <template v-if="result">
      <AppSurface v-if="result.warnings?.length" padding="lg" class="result-surface">
        <h2 class="result-title">导入提示</h2>
        <Message
          v-for="warning in result.warnings"
          :key="warning"
          severity="warn"
          :closable="false"
        >
          {{ warning }}
        </Message>
      </AppSurface>

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
            <template #body="{ data }">
              <span>{{ data.title }}</span>
              <Tag v-if="data.overwritten" value="已覆盖" severity="warn" class="overwritten-tag" />
            </template>
          </Column>
          <Column header="标识" style="min-width: 120px">
            <template #body="{ data }">
              <span class="mono">{{ data.slug }}</span>
            </template>
          </Column>
          <Column header="测试点数" style="width: 100px">
            <template #body="{ data }">{{ data.testcaseCount }}</template>
          </Column>
          <Column header="可见性" style="min-width: 130px">
            <template #body="{ data }">
              <Tag
                :value="data.published ? '已公开' : '未公开'"
                :severity="data.published ? 'success' : 'secondary'"
              />
              <div v-if="data.publishError" class="publish-error">{{ data.publishError }}</div>
            </template>
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

      <AppSurface v-if="result.skipped?.length" padding="lg" class="result-surface">
        <h2 class="result-title">已跳过 {{ result.skipped.length }} 道</h2>
        <ul class="skipped-list">
          <li v-for="item in result.skipped" :key="item.slug">
            <span class="mono">{{ item.slug }}</span>
            <span>{{ item.folder }}</span>
          </li>
        </ul>
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

    <Dialog
      v-model:visible="conflictVisible"
      header="发现同名题目"
      modal
      :draggable="false"
      :style="{ width: 'min(96vw, 760px)' }"
    >
      <p class="conflict-hint">
        以下题目标识已存在。勾选需要覆盖的题目，未勾选的将跳过；覆盖后题目会强制下线，同时会重置题目难度并重新开启自动难度调整。
      </p>
      <DataTable :value="result?.conflicts ?? []" data-key="slug" size="small">
        <Column header="覆盖" style="width: 70px">
          <template #body="{ data }">
            <Checkbox
              v-model="overwriteSlugs"
              :value="data.slug"
              binary
              :disabled="!data.canOverwrite"
              :aria-label="`覆盖 ${data.slug}`"
            />
          </template>
        </Column>
        <Column header="标识" style="min-width: 130px">
          <template #body="{ data }"
            ><span class="mono">{{ data.slug }}</span></template
          >
        </Column>
        <Column header="已有题目" style="min-width: 160px">
          <template #body="{ data }">{{ data.existingTitle }}</template>
        </Column>
        <Column header="权限" style="width: 100px">
          <template #body="{ data }">
            <Tag
              :value="data.canOverwrite ? '可覆盖' : '无权覆盖'"
              :severity="data.canOverwrite ? 'success' : 'secondary'"
            />
          </template>
        </Column>
      </DataTable>
      <template #footer>
        <Button
          label="全部跳过"
          severity="secondary"
          text
          :disabled="uploading"
          @click="skipAllConflicts"
        />
        <Button
          label="应用选择并导入"
          icon="pi pi-upload"
          :loading="uploading"
          @click="applyConflicts"
        />
      </template>
    </Dialog>
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

.import-drop {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  min-height: 150px;
  color: var(--kapoj-text-secondary);
  text-align: center;
  cursor: pointer;
  border: 1px dashed var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.import-drop:hover,
.import-drop--active {
  color: var(--kapoj-primary);
  border-color: var(--kapoj-primary);
  background: var(--kapoj-primary-soft);
}

.import-drop--busy {
  cursor: wait;
  opacity: 0.7;
}

.import-drop i {
  font-size: 2rem;
}

.import-drop small {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.publish-option {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  color: var(--kapoj-text-secondary);
  font-size: 13px;
  cursor: pointer;
}

.import-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
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
}

.result-table {
  width: 100%;
}

.result-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
}

.mono {
  font-family: var(--kapoj-font-mono);
}

.overwritten-tag {
  margin-left: var(--kapoj-space-1);
}

.skipped-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 0;
  margin: 0;
  list-style: none;
}

.skipped-list li {
  display: flex;
  gap: var(--kapoj-space-2);
  color: var(--kapoj-text-secondary);
  font-size: 12px;
}

.empty-result {
  margin: 0;
  color: var(--kapoj-text-muted);
}

.conflict-hint {
  margin: 0 0 var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
}

.edit-link {
  color: var(--kapoj-primary);
}

.offline-note {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.publish-error {
  margin-top: 2px;
  color: var(--kapoj-danger);
  font-size: 11px;
}
</style>
