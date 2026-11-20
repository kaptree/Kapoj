<script setup lang="ts">
import { computed, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'

import { importAdminUsers, previewAdminUsersImport } from '@/api/admin'
import { ApiError } from '@/api/http'
import type { AdminUserImportResult } from '@/api/types'
import { mappingCsv } from '@/utils/userImport'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{
  'update:visible': [value: boolean]
  imported: []
}>()

const MAX_BYTES = 5 * 1024 * 1024

const SAMPLE_CSV = `_id,uname,mail,hash,salt,hashType
1001,alice,alice@example.com,c519b45b4e118f223b8edfa027e6b07a3446fa506e634ff3f71608fb6abf5d17,aB3xY9mKpQ2rStUvWxYz0123456789AB,hydro
1002,小助手,xiaozhushou@example.com,c519b45b4e118f223b8edfa027e6b07a3446fa506e634ff3f71608fb6abf5d17,aB3xY9mKpQ2rStUvWxYz0123456789AB,hydro`

const open = computed({
  get: () => props.visible,
  set: (value: boolean) => emit('update:visible', value),
})

const file = ref<File | null>(null)
const preview = ref<AdminUserImportResult | null>(null)
const result = ref<AdminUserImportResult | null>(null)
const previewing = ref(false)
const importing = ref(false)
const helpOpen = ref(false)
const error = ref('')

const summary = computed(() => result.value?.summary ?? preview.value?.summary ?? null)
const issues = computed(() => result.value?.issues ?? preview.value?.issues ?? [])
const samples = computed(() => preview.value?.samples ?? [])
const mapping = computed(() => result.value?.mapping ?? preview.value?.mapping ?? [])
const hasMapping = computed(() => mapping.value.length > 0)

const fileLabel = computed(() => {
  if (!file.value) return '未选择文件'
  return `${file.value.name} · ${formatSize(file.value.size)}`
})

const resultText = computed(() => {
  if (!result.value) return ''
  const value = result.value.summary
  return `成功 ${value.imported}，跳过 ${value.skipped}，失败 ${value.failed}`
})

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function onFileChange(event: Event): void {
  const selected = (event.target as HTMLInputElement).files?.[0] ?? null
  preview.value = null
  result.value = null
  error.value = ''
  if (!selected) {
    file.value = null
    return
  }
  if (!selected.name.toLowerCase().endsWith('.csv')) {
    file.value = null
    error.value = '请选择 CSV 文件'
    return
  }
  if (selected.size > MAX_BYTES) {
    file.value = null
    error.value = 'CSV 不能超过 5 MiB'
    return
  }
  file.value = selected
}

function errorMessage(reason: unknown, fallback: string): string {
  return reason instanceof ApiError ? reason.message : fallback
}

async function runPreview(): Promise<void> {
  if (!file.value || previewing.value || importing.value) return
  previewing.value = true
  result.value = null
  error.value = ''
  try {
    preview.value = await previewAdminUsersImport(file.value)
  } catch (reason) {
    preview.value = null
    error.value = errorMessage(reason, '预览失败，请稍后再试')
  } finally {
    previewing.value = false
  }
}

async function confirmImport(): Promise<void> {
  if (!file.value || !preview.value || preview.value.summary.valid <= 0 || importing.value) {
    return
  }
  importing.value = true
  error.value = ''
  try {
    result.value = await importAdminUsers(file.value)
    emit('imported')
  } catch (reason) {
    error.value = errorMessage(reason, '导入失败，请稍后再试')
  } finally {
    importing.value = false
  }
}

function downloadMapping(): void {
  if (!hasMapping.value) return
  const content = mappingCsv(mapping.value)
  const url = URL.createObjectURL(new Blob([content], { type: 'text/csv;charset=utf-8' }))
  const link = document.createElement('a')
  link.href = url
  link.download = 'user-import-mapping.csv'
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

function close(): void {
  if (importing.value) return
  open.value = false
}
</script>

<template>
  <Dialog
    v-model:visible="open"
    header="导入用户"
    modal
    :draggable="false"
    :style="{ width: 'min(94vw, 760px)' }"
  >
    <div class="admin-user-import">
      <p class="admin-user-import__intro">
        上传 HydroOJ 导出的用户 CSV。已存在的账号会跳过，不会修改密码、邮箱或角色。
      </p>

      <div class="admin-user-import__toolbar">
        <label class="admin-user-import__file">
          <span>选择 CSV</span>
          <input
            data-test="import-file"
            type="file"
            accept=".csv,text/csv"
            @change="onFileChange"
          />
        </label>
        <span class="admin-user-import__filename">{{ fileLabel }}</span>
        <Button
          type="button"
          text
          icon="pi pi-question-circle"
          label="CSV 格式说明"
          data-test="import-help-toggle"
          :aria-expanded="helpOpen"
          aria-controls="import-help"
          @click="helpOpen = !helpOpen"
        />
      </div>

      <p v-if="error" class="admin-user-import__error" role="alert">{{ error }}</p>

      <section
        v-if="helpOpen"
        id="import-help"
        data-test="import-help"
        class="admin-user-import__help"
      >
        <h3>支持的字段</h3>
        <table>
          <thead>
            <tr>
              <th>字段</th>
              <th>必填</th>
              <th>说明</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td><code>_id</code></td>
              <td>是</td>
              <td>
                Hydro 旧用户 ID，正整数，同一文件内唯一；用于生成 <code>hydro_&lt;id&gt;</code>
              </td>
            </tr>
            <tr>
              <td><code>uname</code></td>
              <td>是</td>
              <td>
                原用户名，1–32 字符；作为昵称；合法 ASCII 用户名保留，否则生成
                <code>hydro_&lt;id&gt;</code>
              </td>
            </tr>
            <tr>
              <td><code>mail</code></td>
              <td>是</td>
              <td>邮箱，同一文件内唯一；登录时也可以使用邮箱</td>
            </tr>
            <tr>
              <td><code>hash</code></td>
              <td>是</td>
              <td>64 位十六进制 HydroOJ 密码哈希</td>
            </tr>
            <tr>
              <td><code>salt</code></td>
              <td>是</td>
              <td>32 字符密码盐</td>
            </tr>
            <tr>
              <td><code>hashType</code></td>
              <td>是</td>
              <td>目前仅支持 <code>hydro</code></td>
            </tr>
          </tbody>
        </table>

        <h3>文件限制</h3>
        <ul>
          <li>UTF-8（允许 BOM），最大 5 MiB，最多 5000 个数据行。</li>
          <li>表头必须包含上述 6 列；列顺序不限，额外列会忽略。</li>
          <li>已存在的用户名或邮箱会跳过，不修改现有账号。</li>
          <li>不展示、不导出 <code>hash</code>/<code>salt</code>。</li>
        </ul>

        <h3>样例 CSV</h3>
        <pre data-test="import-sample">{{ SAMPLE_CSV }}</pre>
        <p class="admin-user-import__note">
          样例中的 hash/salt 对应演示密码 HelloWorld；真实导入请使用 HydroOJ 导出的值。
          <code>小助手</code> 这类非 ASCII 用户名会生成 <code>hydro_1002</code>，昵称仍为
          <code>小助手</code>。
        </p>
      </section>

      <div v-if="summary" class="admin-user-import__summary" data-test="import-summary">
        <div>
          <span>总数</span>
          <strong data-test="summary-total">{{ summary.total }}</strong>
        </div>
        <div>
          <span>可导入</span>
          <strong data-test="summary-valid">{{ summary.valid }}</strong>
        </div>
        <div>
          <span>冲突</span>
          <strong data-test="summary-conflicts">{{ summary.conflicts }}</strong>
        </div>
        <div>
          <span>无效</span>
          <strong data-test="summary-invalid">{{ summary.invalid }}</strong>
        </div>
        <div>
          <span>生成用户名</span>
          <strong data-test="summary-generated">{{ summary.generatedUsernames }}</strong>
        </div>
      </div>

      <DataTable
        v-if="issues.length > 0"
        :value="issues"
        data-test="import-issues"
        size="small"
        responsive-layout="scroll"
        class="admin-user-import__table"
      >
        <Column field="line" header="行" style="width: 64px" />
        <Column field="legacyId" header="旧 ID" style="width: 80px" />
        <Column field="uname" header="原用户名" style="min-width: 130px" />
        <Column field="email" header="邮箱" style="min-width: 180px" />
        <Column field="reason" header="原因" style="min-width: 140px" />
      </DataTable>

      <DataTable
        v-if="samples.length > 0"
        :value="samples"
        data-test="import-samples"
        size="small"
        responsive-layout="scroll"
        class="admin-user-import__table"
      >
        <Column field="username" header="新用户名" style="min-width: 130px" />
        <Column field="uname" header="昵称" style="min-width: 130px" />
        <Column header="生成" style="width: 72px">
          <template #body="{ data }">{{ data.generated ? '是' : '否' }}</template>
        </Column>
      </DataTable>

      <p v-if="result" data-test="import-result" class="admin-user-import__result">
        {{ resultText }}
      </p>

      <div v-if="hasMapping" class="admin-user-import__mapping">
        <Button
          type="button"
          text
          icon="pi pi-download"
          label="下载账号映射 CSV"
          data-test="import-mapping"
          @click="downloadMapping"
        />
        <span>只包含生成 hydro_&lt;id&gt; 的用户</span>
      </div>

      <div class="admin-user-import__actions">
        <Button
          type="button"
          label="取消"
          severity="secondary"
          text
          :disabled="importing"
          @click="close"
        />
        <Button
          type="button"
          label="预览"
          data-test="import-preview"
          :loading="previewing"
          :disabled="!file || previewing || importing"
          @click="runPreview"
        />
        <Button
          type="button"
          label="确认导入"
          data-test="import-confirm"
          :loading="importing"
          :disabled="!file || !preview || preview.summary.valid <= 0 || importing || !!result"
          @click="confirmImport"
        />
      </div>
    </div>
  </Dialog>
</template>

<style scoped>
.admin-user-import {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.admin-user-import__intro,
.admin-user-import__note {
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.84rem;
  line-height: 1.55;
}

.admin-user-import__toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
  align-items: center;
}

.admin-user-import__file {
  position: relative;
  display: inline-flex;
  align-items: center;
  padding: 7px 12px;
  overflow: hidden;
  font-size: 0.82rem;
  color: var(--kapoj-text);
  cursor: pointer;
  background: var(--kapoj-kapo-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-kapo) 45%, var(--kapoj-border));
  border-radius: var(--kapoj-radius-sm);
}

.admin-user-import__file input {
  position: absolute;
  inset: 0;
  cursor: pointer;
  opacity: 0;
}

.admin-user-import__filename {
  flex: 1;
  min-width: 160px;
  font-size: 12px;
  color: var(--kapoj-text-muted);
}

.admin-user-import__error {
  margin: 0;
  color: var(--kapoj-kapo-text);
  font-size: 12px;
}

.admin-user-import__help {
  padding: 10px 12px;
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.admin-user-import__help h3 {
  margin: 10px 0 6px;
  font-size: 0.82rem;
  color: var(--kapoj-text);
}

.admin-user-import__help h3:first-child {
  margin-top: 0;
}

.admin-user-import__help table {
  width: 100%;
  font-size: 12px;
  border-collapse: collapse;
}

.admin-user-import__help th,
.admin-user-import__help td {
  padding: 5px 6px;
  text-align: left;
  vertical-align: top;
  border-bottom: 1px solid var(--kapoj-border);
}

.admin-user-import__help th {
  color: var(--kapoj-text-secondary);
  font-weight: 600;
}

.admin-user-import__help ul {
  padding-left: 18px;
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
  color: var(--kapoj-text-secondary);
}

.admin-user-import__help pre {
  max-height: 190px;
  padding: 8px 10px;
  overflow: auto;
  font-size: 11px;
  line-height: 1.6;
  color: var(--kapoj-text);
  background: var(--kapoj-surface);
  border: 1px solid var(--kapoj-border);
  border-radius: 4px;
}

.admin-user-import__help code {
  font-family: var(--kapoj-font-mono);
  font-size: 0.92em;
}

.admin-user-import__summary {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 8px;
}

.admin-user-import__summary > div {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 10px;
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.admin-user-import__summary span {
  font-size: 11px;
  color: var(--kapoj-text-muted);
}

.admin-user-import__summary strong {
  font-size: 1.05rem;
  font-variant-numeric: tabular-nums;
  color: var(--kapoj-text);
}

.admin-user-import__table {
  font-size: 12px;
}

.admin-user-import__result {
  margin: 0;
  font-size: 0.86rem;
  color: var(--kapoj-kapo-text);
}

.admin-user-import__mapping {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  font-size: 11px;
  color: var(--kapoj-text-muted);
}

.admin-user-import__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  justify-content: flex-end;
}

@media (max-width: 640px) {
  .admin-user-import__summary {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .admin-user-import__actions {
    justify-content: stretch;
  }

  .admin-user-import__actions :deep(.p-button) {
    flex: 1 1 auto;
  }
}
</style>
