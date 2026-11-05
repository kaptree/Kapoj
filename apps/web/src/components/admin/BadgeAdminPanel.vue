<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { UploadFile, UploadInstance } from 'element-plus'

import {
  createBadge,
  deleteBadge,
  listBadges,
  updateBadge,
  uploadBadgeImage,
} from '@/api/gamification'
import { ApiError } from '@/api/http'
import type { BadgeInput, BadgeItem } from '@/api/types'
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

const badges = ref<BadgeItem[]>([])
const loading = ref(false)
const error = ref('')
const dialogVisible = ref(false)
const saving = ref(false)
const deletingId = ref<number | null>(null)
const editing = ref<BadgeItem | null>(null)
const adjustStock = ref(false)
const uploadRef = ref<UploadInstance>()
const pendingFile = ref<File | null>(null)
const pendingPreview = ref('')
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

const form = reactive<FormState>(emptyForm())

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
    : adminImageSrc(form.imageUrl.trim() || editing.value?.imageUrl || ''),
)

function fail(e: unknown, fallback: string) {
  ElMessage.error(e instanceof ApiError ? e.message : fallback)
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
  uploadRef.value?.clearFiles()
}

function onFileChange(file: UploadFile) {
  const raw = file.raw
  if (!raw) return
  if (raw.size > MAX_IMAGE_BYTES) {
    ElMessage.warning('徽章图片不能超过 2 MiB')
    uploadRef.value?.clearFiles()
    return
  }
  if (!IMAGE_MIME_TYPES.has(raw.type) && !IMAGE_EXT_RE.test(raw.name)) {
    ElMessage.warning('仅支持 PNG、JPEG、WebP 或 GIF 图片')
    uploadRef.value?.clearFiles()
    return
  }
  if (previewObjectUrl) URL.revokeObjectURL(previewObjectUrl)
  previewObjectUrl = URL.createObjectURL(raw)
  pendingFile.value = raw
  pendingPreview.value = previewObjectUrl
}

function onLimitedChange(value: boolean) {
  if (!value) {
    adjustStock.value = false
    return
  }
  // A non-limited badge has no meaningful stock. When it is switched to
  // limited, force the admin to submit an explicit stock value.
  if (editing.value && (!Number.isInteger(form.stock) || form.stock <= 0)) {
    adjustStock.value = true
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    badges.value = await listBadges({ includeInactive: true })
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
  Object.assign(form, emptyForm())
  dialogVisible.value = true
}

function openEdit(item: BadgeItem) {
  clearFile()
  editing.value = item
  adjustStock.value = false
  Object.assign(form, {
    name: item.name,
    description: item.description,
    category: item.category,
    priceCoins: item.priceCoins,
    isLimited: item.isLimited,
    stock: item.stock,
    sortOrder: item.sortOrder,
    isActive: item.isActive,
    imageUrl: '',
  })
  dialogVisible.value = true
}

function validate(): string | null {
  const name = form.name.trim()
  if (!name) return '请填写徽章名称'
  if (name.length > 100) return '徽章名称不能超过 100 字'
  if (form.description.trim().length > 500) return '徽章描述不能超过 500 字'
  const category = form.category.trim()
  if (!category) return '请填写徽章分类'
  if (category.length > 20) return '徽章分类不能超过 20 字'
  if (!Number.isInteger(form.priceCoins) || form.priceCoins < 0) {
    return '徽章价格必须为非负整数'
  }
  if (form.priceCoins > 1_000_000) return '徽章价格不能超过 1,000,000'
  if (!Number.isInteger(form.sortOrder)) return '排序必须为整数'
  const imageUrl = form.imageUrl.trim()
  if (imageUrl.length > 512) return '图片地址不能超过 512 字'
  if (imageUrl && !isValidExternalImageUrl(imageUrl)) {
    return '图片外链必须是带域名的 http(s) 地址'
  }
  const changingStock =
    editing.value === null ? form.isLimited : form.isLimited && adjustStock.value
  if (changingStock && (!Number.isInteger(form.stock) || form.stock < 0)) {
    return '限量库存必须为非负整数'
  }
  return null
}

function buildInput(): BadgeInput {
  const input: BadgeInput = {
    name: form.name.trim(),
    description: form.description.trim(),
    category: form.category.trim(),
    priceCoins: form.priceCoins,
    isLimited: form.isLimited,
    sortOrder: form.sortOrder,
    isActive: form.isActive,
  }
  const imageUrl = form.imageUrl.trim()
  // A pending upload will overwrite imageUrl on the server; an empty value must
  // never be sent so ordinary edits keep the existing image.
  if (!pendingFile.value && imageUrl) input.imageUrl = imageUrl
  if (editing.value) {
    if (form.isLimited && adjustStock.value) input.stock = form.stock
  } else if (form.isLimited) {
    input.stock = form.stock
  }
  return input
}

async function save() {
  const problem = validate()
  if (problem) {
    ElMessage.warning(problem)
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
    ElMessage.success(creating ? '徽章已创建' : '徽章已更新')

    if (pendingFile.value) {
      try {
        await uploadBadgeImage(saved.id, pendingFile.value)
      } catch (e) {
        await load()
        const detail = e instanceof ApiError ? e.message : ''
        ElMessage.error(
          detail
            ? `徽章已保存，但图片上传失败：${detail}`
            : '徽章已保存，但图片上传失败，请重试上传',
        )
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

async function remove(item: BadgeItem) {
  try {
    await ElMessageBox.confirm(
      `确定删除徽章「${item.name}」吗？删除后徽章将下线，已购买记录与公开展示会保留。`,
      '删除徽章',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  deletingId.value = item.id
  try {
    await deleteBadge(item.id)
    ElMessage.success('徽章已删除')
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
  <div>
    <div class="toolbar">
      <span class="count">共 {{ badges.length }} 个徽章</span>
      <el-button type="primary" @click="openCreate">新建徽章</el-button>
    </div>

    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" class="banner" />

    <el-table v-loading="loading" :data="badges" empty-text="暂无徽章" style="width: 100%">
      <el-table-column label="图片" width="72">
        <template #default="{ row }">
          <el-avatar
            :size="40"
            shape="square"
            :src="adminImageSrc(row.imageUrl)"
            class="badge-image"
          >
            {{ badgeInitial(row.name) }}
          </el-avatar>
        </template>
      </el-table-column>
      <el-table-column prop="name" label="名称" min-width="140" />
      <el-table-column prop="category" label="分类" min-width="100" />
      <el-table-column label="价格" width="110">
        <template #default="{ row }">{{ formatCoins(row.priceCoins) }} 金币</template>
      </el-table-column>
      <el-table-column label="库存" width="90">
        <template #default="{ row }">
          <el-tag v-if="row.isLimited" :type="row.stock > 0 ? 'warning' : 'danger'" size="small">
            {{ formatCoins(row.stock) }}
          </el-tag>
          <span v-else class="muted">不限</span>
        </template>
      </el-table-column>
      <el-table-column prop="sortOrder" label="排序" width="70" />
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag :type="row.isActive ? 'success' : 'info'" size="small">
            {{ row.isActive ? '已上架' : '已下线' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="140" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link type="danger" :loading="deletingId === row.id" @click="remove(row)"
            >删除</el-button
          >
        </template>
      </el-table-column>
    </el-table>

    <el-dialog
      v-model="dialogVisible"
      :title="editing ? '编辑徽章' : '新建徽章'"
      width="680px"
      destroy-on-close
      @closed="clearFile"
    >
      <el-form label-width="100px">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" maxlength="100" show-word-limit />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="3" maxlength="500" />
        </el-form-item>
        <el-form-item label="分类" required>
          <el-input v-model="form.category" maxlength="20" show-word-limit />
        </el-form-item>
        <el-form-item label="价格">
          <el-input-number
            v-model="form.priceCoins"
            :min="0"
            :max="1000000"
            :step="1"
            class="number-input"
          />
          <span class="hint">金币，0 表示免费</span>
        </el-form-item>

        <el-form-item label="图片">
          <div class="image-field">
            <el-image
              v-if="previewSrc"
              :src="previewSrc"
              fit="cover"
              class="preview"
              :preview-src-list="[previewSrc]"
              preview-teleported
            />
            <div v-else class="preview preview-empty">无图片</div>
            <div class="image-actions">
              <div class="image-upload-row">
                <el-upload
                  ref="uploadRef"
                  :auto-upload="false"
                  :show-file-list="false"
                  accept=".png,.jpg,.jpeg,.webp,.gif"
                  :on-change="onFileChange"
                >
                  <el-button>选择图片</el-button>
                </el-upload>
                <span class="hint">PNG / JPEG / WebP / GIF，最大 2 MiB</span>
              </div>
              <div v-if="pendingFile" class="pending-file">
                <span>待上传：{{ pendingFile.name }}</span>
                <el-button link type="danger" @click="clearFile">清除</el-button>
              </div>
              <el-input
                v-model="form.imageUrl"
                maxlength="512"
                clearable
                placeholder="可选，http(s) 外链；留空则保留现有图片"
              />
              <span class="hint">选择本地图片后将替换外链；上传会在保存后执行</span>
            </div>
          </div>
        </el-form-item>

        <el-form-item label="限量">
          <el-switch v-model="form.isLimited" @change="onLimitedChange" />
          <span class="hint">限量徽章的库存只影响商城购买，成就奖励不占库存</span>
        </el-form-item>

        <template v-if="editing">
          <el-form-item label="当前库存">
            <span class="stock-readonly">
              {{ editing.isLimited ? `${formatCoins(editing.stock)} 个` : '不限量' }}
            </span>
            <span v-if="editing.isLimited" class="hint">不勾选下方选项时保留该库存</span>
          </el-form-item>
          <el-form-item v-if="form.isLimited" label="库存调整">
            <el-checkbox v-model="adjustStock">调整库存</el-checkbox>
            <el-input-number
              v-if="adjustStock"
              v-model="form.stock"
              :min="0"
              :step="1"
              class="number-input"
            />
            <span v-else class="hint">未勾选时将省略库存，保留服务端当前值</span>
          </el-form-item>
        </template>
        <el-form-item v-else-if="form.isLimited" label="限量库存" required>
          <el-input-number v-model="form.stock" :min="0" :step="1" class="number-input" />
          <span class="hint">限量库存必须显式设置</span>
        </el-form-item>

        <el-form-item label="排序">
          <el-input-number v-model="form.sortOrder" :step="1" class="number-input" />
        </el-form-item>
        <el-form-item label="上架">
          <el-switch v-model="form.isActive" />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
}
.count {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.banner {
  margin-bottom: 12px;
}
.badge-image {
  background: var(--el-fill-color-light);
  color: var(--el-text-color-secondary);
  font-size: 15px;
}
.muted {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.hint {
  margin-left: 8px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.number-input {
  width: 180px;
}
.image-field {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  gap: 12px;
  width: 100%;
}
.preview {
  flex: none;
  width: 88px;
  height: 88px;
  border-radius: 6px;
  background: var(--el-fill-color-light);
}
.preview-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.image-actions {
  display: flex;
  flex: 1 1 240px;
  flex-direction: column;
  gap: 6px;
  min-width: 200px;
}
.image-upload-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}
.image-upload-row .hint {
  margin-left: 0;
}
.pending-file {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  color: var(--el-text-color-regular);
  font-size: 12px;
}
.stock-readonly {
  font-weight: 600;
}
@media (max-width: 640px) {
  :deep(.el-dialog) {
    max-width: calc(100vw - 32px);
  }
  :deep(.el-form-item__label) {
    width: 84px !important;
  }
  .image-actions {
    min-width: 100%;
  }
}
</style>
