<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import {
  createBadge,
  deleteBadge,
  listBadges,
  updateBadge,
  uploadBadgeImage,
} from '@/api/gamification'
import { ApiError } from '@/api/http'
import type { BadgeInput, BadgeItem } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import { badgeInitial, formatCoins } from '@/utils/gamification'

interface FormState {
  name: string
  description: string
  category: string
  priceCoins: number
  isLimited: boolean
  stock: number
  sortOrder: number
  isActive: boolean
  imageUrl: string
}

const MAX_IMAGE_BYTES = 2 * 1024 * 1024
const IMAGE_MIME_TYPES = new Set(['image/png', 'image/jpeg', 'image/webp', 'image/gif'])
const IMAGE_EXT_RE = /\.(png|jpe?g|webp|gif)$/i

const toast = useToast()
const confirm = useConfirm()
const badges = ref<BadgeItem[]>([])
const loading = ref(false)
const error = ref('')
const dialogVisible = ref(false)
const saving = ref(false)
const deletingId = ref<number | null>(null)
const editing = ref<BadgeItem | null>(null)
const adjustStock = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)
const pendingFile = ref<File | null>(null)
const pendingPreview = ref('')
const previewFailed = ref(false)
const failedImages = ref<number[]>([])
const cacheBust = ref(Date.now())
let previewObjectUrl = ''

function emptyForm(): FormState {
  return {
    name: '',
    description: '',
    category: '其他',
    priceCoins: 0,
    isLimited: false,
    stock: 0,
    sortOrder: 0,
    isActive: true,
    imageUrl: '',
  }
}

const form = ref<FormState>(emptyForm())

/** Internal badge images keep a stable URL, so the admin view must bust its own cache. */
function adminImageSrc(url: string): string {
  const raw = url.trim()
  if (!raw || !raw.startsWith('/api/badges/')) return raw
  const separator = raw.includes('?') ? '&' : '?'
  return `${raw}${separator}v=${cacheBust.value}`
}

const previewSrc = computed(() =>
  pendingPreview.value
    ? pendingPreview.value
    : adminImageSrc(form.value.imageUrl.trim() || editing.value?.imageUrl || ''),
)

function rowImage(item: BadgeItem): string | undefined {
  if (failedImages.value.includes(item.id)) return undefined
  return adminImageSrc(item.imageUrl) || undefined
}

function rowLabel(item: BadgeItem): string | undefined {
  return rowImage(item) ? undefined : badgeInitial(item.name)
}

function onRowImageError(id: number) {
  if (!failedImages.value.includes(id)) failedImages.value.push(id)
}

function fail(e: unknown, fallback: string) {
  toast.add({
    severity: 'error',
    summary: '操作失败',
    detail: e instanceof ApiError ? e.message : fallback,
    life: 4000,
  })
}

function warn(summary: string) {
  toast.add({ severity: 'warn', summary, life: 3000 })
}

/** Mirrors the backend: only absolute http(s) URLs with a host are accepted. */
function isValidExternalImageUrl(raw: string): boolean {
  try {
    const url = new URL(raw)
    return (url.protocol === 'http:' || url.protocol === 'https:') && url.hostname !== ''
  } catch {
    return false
  }
}

function clearFile() {
  if (previewObjectUrl) {
    URL.revokeObjectURL(previewObjectUrl)
    previewObjectUrl = ''
  }
  pendingFile.value = null
  pendingPreview.value = ''
  previewFailed.value = false
  if (fileInput.value) fileInput.value.value = ''
}

function chooseImage() {
  fileInput.value?.click()
}

function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const raw = input.files?.[0]
  if (!raw) return
  if (raw.size > MAX_IMAGE_BYTES) {
    warn('徽章图片不能超过 2 MiB')
    input.value = ''
    return
  }
  if (!IMAGE_MIME_TYPES.has(raw.type) && !IMAGE_EXT_RE.test(raw.name)) {
    warn('仅支持 PNG、JPEG、WebP 或 GIF 图片')
    input.value = ''
    return
  }
  if (previewObjectUrl) URL.revokeObjectURL(previewObjectUrl)
  previewObjectUrl = URL.createObjectURL(raw)
  previewFailed.value = false
  pendingFile.value = raw
  pendingPreview.value = previewObjectUrl
  input.value = ''
}

function onLimitedChange(value: boolean) {
  if (!value) {
    adjustStock.value = false
    return
  }
  // A non-limited badge has no meaningful stock. When it is switched to
  // limited, force the admin to submit an explicit stock value.
  if (editing.value && (!Number.isInteger(form.value.stock) || form.value.stock <= 0)) {
    adjustStock.value = true
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    badges.value = await listBadges({ includeInactive: true })
    failedImages.value = []
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '加载徽章失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

function openCreate() {
  clearFile()
  editing.value = null
  adjustStock.value = false
  form.value = emptyForm()
  dialogVisible.value = true
}

function openEdit(item: BadgeItem) {
  clearFile()
  editing.value = item
  adjustStock.value = false
  form.value = {
    name: item.name,
    description: item.description,
    category: item.category,
    priceCoins: item.priceCoins,
    isLimited: item.isLimited,
    stock: item.stock,
    sortOrder: item.sortOrder,
    isActive: item.isActive,
    imageUrl: '',
  }
  dialogVisible.value = true
}

function validate(): string | null {
  const current = form.value
  const name = current.name.trim()
  if (!name) return '请填写徽章名称'
  if (name.length > 100) return '徽章名称不能超过 100 字'
  if (current.description.trim().length > 500) return '徽章描述不能超过 500 字'
  const category = current.category.trim()
  if (!category) return '请填写徽章分类'
  if (category.length > 20) return '徽章分类不能超过 20 字'
  if (!Number.isInteger(current.priceCoins) || current.priceCoins < 0) {
    return '徽章价格必须为非负整数'
  }
  if (current.priceCoins > 1_000_000) return '徽章价格不能超过 1,000,000'
  if (!Number.isInteger(current.sortOrder)) return '排序必须为整数'
  const imageUrl = current.imageUrl.trim()
  if (imageUrl.length > 512) return '图片地址不能超过 512 字'
  if (imageUrl && !isValidExternalImageUrl(imageUrl)) {
    return '图片外链必须是带域名的 http(s) 地址'
  }
  const changingStock =
    editing.value === null ? current.isLimited : current.isLimited && adjustStock.value
  if (changingStock && (!Number.isInteger(current.stock) || current.stock < 0)) {
    return '限量库存必须为非负整数'
  }
  return null
}

function buildInput(): BadgeInput {
  const current = form.value
  const input: BadgeInput = {
    name: current.name.trim(),
    description: current.description.trim(),
    category: current.category.trim(),
    priceCoins: current.priceCoins,
    isLimited: current.isLimited,
    sortOrder: current.sortOrder,
    isActive: current.isActive,
  }
  const imageUrl = current.imageUrl.trim()
  // A pending upload will overwrite imageUrl on the server; an empty value must
  // never be sent so ordinary edits keep the existing image.
  if (!pendingFile.value && imageUrl) input.imageUrl = imageUrl
  if (editing.value) {
    if (current.isLimited && adjustStock.value) input.stock = current.stock
  } else if (current.isLimited) {
    input.stock = current.stock
  }
  return input
}

async function save() {
  const problem = validate()
  if (problem) {
    warn(problem)
    return
  }
  const creating = editing.value === null
  saving.value = true
  try {
    let saved: BadgeItem
    try {
      saved = creating
        ? await createBadge(buildInput())
        : await updateBadge(editing.value!.id, buildInput())
    } catch (e) {
      fail(e, creating ? '创建徽章失败' : '保存徽章失败')
      return
    }

    // Keep the created badge as the edit target so a failed image upload can be
    // retried without creating a duplicate badge.
    editing.value = saved
    toast.add({
      severity: 'success',
      summary: creating ? '徽章已创建' : '徽章已更新',
      life: 3000,
    })

    if (pendingFile.value) {
      try {
        await uploadBadgeImage(saved.id, pendingFile.value)
      } catch (e) {
        await load()
        const detail = e instanceof ApiError ? e.message : ''
        toast.add({
          severity: 'error',
          summary: '图片上传失败',
          detail: detail
            ? `徽章已保存，但图片上传失败：${detail}`
            : '徽章已保存，但图片上传失败，请重试上传',
          life: 5000,
        })
        // Keep the dialog open in edit mode with the pending file so the upload
        // can be retried without saving the badge again from scratch.
        return
      }
      cacheBust.value = Date.now()
    }

    dialogVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

function remove(item: BadgeItem) {
  confirm.require({
    header: '删除徽章',
    message: `确定删除徽章「${item.name}」吗？删除后徽章将下线，已购买记录与公开展示会保留。`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => {
      void doRemove(item)
    },
  })
}

async function doRemove(item: BadgeItem) {
  deletingId.value = item.id
  try {
    await deleteBadge(item.id)
    toast.add({ severity: 'success', summary: '徽章已删除', life: 3000 })
    await load()
  } catch (e) {
    fail(e, '删除徽章失败')
  } finally {
    deletingId.value = null
  }
}

onMounted(load)
onBeforeUnmount(clearFile)
</script>

<template>
  <div class="admin-panel">
    <div class="toolbar">
      <span class="count">共 {{ badges.length }} 个徽章</span>
      <Button label="新建徽章" icon="pi pi-plus" size="small" @click="openCreate" />
    </div>

    <AppErrorState v-if="error && !badges.length" :description="error" @retry="load" />
    <template v-else>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <DataTable
        :value="badges"
        :loading="loading"
        data-key="id"
        size="small"
        responsive-layout="scroll"
        class="admin-table"
      >
        <Column header="图片" style="width: 72px">
          <template #body="{ data }">
            <Avatar
              :image="rowImage(data)"
              :label="rowLabel(data)"
              size="large"
              shape="square"
              class="badge-image"
              @error="onRowImageError(data.id)"
            />
          </template>
        </Column>
        <Column header="名称" style="min-width: 140px">
          <template #body="{ data }">{{ data.name }}</template>
        </Column>
        <Column header="分类" style="min-width: 100px">
          <template #body="{ data }">{{ data.category }}</template>
        </Column>
        <Column header="价格" style="width: 110px">
          <template #body="{ data }">{{ formatCoins(data.priceCoins) }} 金币</template>
        </Column>
        <Column header="库存" style="width: 90px">
          <template #body="{ data }">
            <Tag
              v-if="data.isLimited"
              :severity="data.stock > 0 ? 'warn' : 'danger'"
              :value="formatCoins(data.stock)"
            />
            <span v-else class="muted">不限</span>
          </template>
        </Column>
        <Column header="排序" style="width: 70px">
          <template #body="{ data }"
            ><span class="num">{{ data.sortOrder }}</span></template
          >
        </Column>
        <Column header="状态" style="width: 90px">
          <template #body="{ data }">
            <Tag
              :value="data.isActive ? '已上架' : '已下线'"
              :severity="data.isActive ? 'success' : 'secondary'"
            />
          </template>
        </Column>
        <Column header="操作" style="width: 140px">
          <template #body="{ data }">
            <div class="row-actions">
              <Button label="编辑" text size="small" @click="openEdit(data)" />
              <Button
                label="删除"
                text
                size="small"
                severity="danger"
                :loading="deletingId === data.id"
                @click="remove(data)"
              />
            </div>
          </template>
        </Column>
        <template #empty><AppEmptyState title="暂无徽章" /></template>
      </DataTable>
    </template>

    <Dialog
      v-model:visible="dialogVisible"
      :header="editing ? '编辑徽章' : '新建徽章'"
      modal
      :draggable="false"
      :style="{ width: 'min(94vw, 680px)' }"
      @hide="clearFile"
    >
      <div class="admin-form">
        <div class="field">
          <label for="badge-name">名称 <span class="required">*</span></label>
          <InputText
            id="badge-name"
            v-model="form.name"
            class="badge-name-input"
            maxlength="100"
            fluid
          />
        </div>
        <div class="field">
          <label for="badge-category">分类 <span class="required">*</span></label>
          <InputText
            id="badge-category"
            v-model="form.category"
            class="badge-category-input"
            maxlength="20"
            fluid
          />
        </div>
        <div class="field span-2">
          <label for="badge-description">描述</label>
          <Textarea
            id="badge-description"
            v-model="form.description"
            class="badge-description-input"
            rows="3"
            maxlength="500"
            fluid
          />
        </div>
        <div class="field">
          <label for="badge-price">价格</label>
          <InputNumber
            v-model="form.priceCoins"
            input-id="badge-price"
            :min="0"
            :max="1000000"
            :step="1"
            fluid
          />
          <span class="hint">金币，0 表示免费</span>
        </div>
        <div class="field">
          <label for="badge-sort">排序</label>
          <InputNumber v-model="form.sortOrder" input-id="badge-sort" :step="1" fluid />
        </div>

        <div class="field span-2">
          <span class="field-label">图片</span>
          <div class="image-field">
            <Avatar
              v-if="previewSrc && !previewFailed"
              :image="previewSrc"
              size="xlarge"
              shape="square"
              class="preview"
              @error="previewFailed = true"
            />
            <div v-else class="preview preview-empty">无图片</div>
            <div class="image-actions">
              <div class="image-upload-row">
                <input
                  ref="fileInput"
                  type="file"
                  class="badge-file-input"
                  accept=".png,.jpg,.jpeg,.webp,.gif"
                  @change="onFileChange"
                />
                <Button label="选择图片" size="small" outlined @click="chooseImage" />
                <span class="hint">PNG / JPEG / WebP / GIF，最大 2 MiB</span>
              </div>
              <div v-if="pendingFile" class="pending-file">
                <span>待上传：{{ pendingFile.name }}</span>
                <Button label="清除" text size="small" severity="danger" @click="clearFile" />
              </div>
              <InputText
                v-model="form.imageUrl"
                class="badge-image-url"
                maxlength="512"
                placeholder="可选，http(s) 外链；留空则保留现有图片"
                fluid
              />
              <span class="hint">选择本地图片后将替换外链；上传会在保存后执行</span>
            </div>
          </div>
        </div>

        <div class="field">
          <span class="field-label">限量</span>
          <div class="toggle-row">
            <ToggleSwitch
              v-model="form.isLimited"
              input-id="badge-limited"
              @update:model-value="onLimitedChange"
            />
            <label for="badge-limited">{{ form.isLimited ? '限量' : '不限量' }}</label>
          </div>
          <span class="hint">限量徽章的库存只影响商城购买，成就奖励不占库存</span>
        </div>

        <div v-if="editing" class="field">
          <span class="field-label">当前库存</span>
          <span class="stock-readonly">
            {{ editing.isLimited ? `${formatCoins(editing.stock)} 个` : '不限量' }}
          </span>
          <span v-if="editing.isLimited" class="hint">不勾选下方选项时保留该库存</span>
        </div>
        <div v-if="editing && form.isLimited" class="field span-2">
          <span class="field-label">库存调整</span>
          <div class="toggle-row">
            <Checkbox v-model="adjustStock" input-id="badge-adjust-stock" binary />
            <label for="badge-adjust-stock">调整库存</label>
            <InputNumber
              v-if="adjustStock"
              v-model="form.stock"
              input-id="badge-stock"
              :min="0"
              :step="1"
            />
          </div>
          <span v-if="!adjustStock" class="hint"> 未勾选时将省略库存，保留服务端当前值 </span>
        </div>
        <div v-else-if="!editing && form.isLimited" class="field">
          <label for="badge-stock">限量库存 <span class="required">*</span></label>
          <InputNumber v-model="form.stock" input-id="badge-stock" :min="0" :step="1" fluid />
          <span class="hint">限量库存必须显式设置</span>
        </div>

        <div class="field toggle-field">
          <span class="field-label">上架</span>
          <div class="toggle-row">
            <ToggleSwitch v-model="form.isActive" input-id="badge-active" />
            <label for="badge-active">{{ form.isActive ? '已上架' : '已下线' }}</label>
          </div>
        </div>
      </div>

      <template #footer>
        <Button label="取消" severity="secondary" text @click="dialogVisible = false" />
        <Button label="保存" icon="pi pi-check" :loading="saving" @click="save" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.admin-panel {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
}

.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
}

.count {
  color: var(--kapoj-text-secondary);
  font-size: 13px;
}

.admin-table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.admin-table :deep(.p-datatable-thead > tr > th) {
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

.admin-table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.badge-image {
  color: var(--kapoj-primary);
  background: var(--kapoj-primary-soft);
}

.muted {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.row-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-1);
}

.admin-form {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--kapoj-space-3);
}

.field {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
  min-width: 0;
}

.span-2 {
  grid-column: 1 / -1;
}

.field label,
.field-label {
  color: var(--kapoj-text-secondary);
  font-size: 12px;
  font-weight: 600;
}

.required {
  color: var(--kapoj-danger);
}

.hint {
  color: var(--kapoj-text-muted);
  font-size: 11px;
  line-height: 1.4;
}

.image-field {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  gap: var(--kapoj-space-3);
  width: 100%;
}

.preview {
  flex: none;
  width: 88px;
  height: 88px;
  background: var(--kapoj-bg-elevated);
}

.preview-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--kapoj-text-muted);
  font-size: 12px;
  border: 1px dashed var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.image-actions {
  display: flex;
  flex: 1 1 240px;
  flex-direction: column;
  gap: var(--kapoj-space-1);
  min-width: 200px;
}

.image-upload-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.badge-file-input {
  display: none;
}

.pending-file {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
  color: var(--kapoj-text);
  font-size: 12px;
}

.toggle-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.toggle-row label {
  color: var(--kapoj-text);
  cursor: pointer;
}

.stock-readonly {
  color: var(--kapoj-text);
  font-weight: 600;
}

@media (max-width: 640px) {
  .admin-form {
    grid-template-columns: minmax(0, 1fr);
  }

  .image-actions {
    min-width: 100%;
  }
}
</style>
