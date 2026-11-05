<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'

import {
  createTitle,
  deleteTitle,
  listAchievements,
  listTitles,
  updateTitle,
} from '@/api/gamification'
import { ApiError } from '@/api/http'
import type { AchievementItem, TitleInput, TitleItem, TitleUnlockType } from '@/api/types'
import {
  DEFAULT_TITLE_COLOR,
  formatCoins,
  safeTitleColor,
  titleUnlockLabel,
} from '@/utils/gamification'

interface FormState {
  name: string
  color: string
  unlockConditionType: TitleUnlockType
  rating: number
  threshold: number
  achievementCode: string
  priceCoins: number
  sortOrder: number
  isActive: boolean
}

interface AchievementOption {
  value: string
  label: string
}

const UNLOCK_TYPES: TitleUnlockType[] = ['rating', 'ac_count', 'achievement']
const COLOR_RE = /^#[0-9A-Fa-f]{6}$/

const titles = ref<TitleItem[]>([])
const achievements = ref<AchievementItem[]>([])
const loading = ref(false)
const error = ref('')
const achievementError = ref('')
const achievementLoading = ref(false)
const dialogVisible = ref(false)
const saving = ref(false)
const deletingId = ref<number | null>(null)
const editing = ref<TitleItem | null>(null)

function emptyForm(): FormState {
  return {
    name: '',
    color: DEFAULT_TITLE_COLOR,
    unlockConditionType: 'ac_count',
    rating: 1600,
    threshold: 1,
    achievementCode: '',
    priceCoins: 0,
    sortOrder: 0,
    isActive: true,
  }
}

const form = reactive<FormState>(emptyForm())

const achievementOptions = computed<AchievementOption[]>(() => {
  const options = achievements.value.map((item) => ({
    value: item.code,
    label: `${item.code} · ${item.name}${item.isActive ? '' : '（已下线）'}`,
  }))
  const code = form.achievementCode.trim()
  if (code && !options.some((option) => option.value === code)) {
    options.push({ value: code, label: `${code}（未找到对应成就）` })
  }
  return options
})

function fail(e: unknown, fallback: string) {
  ElMessage.error(e instanceof ApiError ? e.message : fallback)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    titles.value = await listTitles({ includeInactive: true })
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '加载称号失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

async function loadAchievements() {
  achievementLoading.value = true
  achievementError.value = ''
  try {
    achievements.value = await listAchievements({ includeInactive: true })
  } catch (e) {
    achievementError.value = e instanceof ApiError ? e.message : '加载成就列表失败，请稍后再试'
  } finally {
    achievementLoading.value = false
  }
}

function conditionText(item: TitleItem): string {
  const config = item.unlockConditionConfig
  if (item.unlockConditionType === 'rating') {
    return `Rating ≥ ${config.rating ?? 0}`
  }
  if (item.unlockConditionType === 'ac_count') {
    return `AC 题目数 ≥ ${config.threshold ?? 0}`
  }
  const code = config.achievementCode ?? ''
  const achievement = achievements.value.find((option) => option.code === code)
  return achievement ? `成就 ${code}（${achievement.name}）` : `成就 ${code}`
}

function openCreate() {
  editing.value = null
  Object.assign(form, emptyForm())
  dialogVisible.value = true
  if (achievements.value.length === 0) void loadAchievements()
}

function openEdit(item: TitleItem) {
  editing.value = item
  Object.assign(form, {
    name: item.name,
    color: item.color,
    unlockConditionType: item.unlockConditionType,
    rating: item.unlockConditionConfig.rating ?? 1600,
    threshold: item.unlockConditionConfig.threshold ?? 1,
    achievementCode: item.unlockConditionConfig.achievementCode ?? '',
    priceCoins: item.priceCoins,
    sortOrder: item.sortOrder,
    isActive: item.isActive,
  })
  dialogVisible.value = true
  if (achievements.value.length === 0) void loadAchievements()
}

function onColorChange(value: string | null) {
  form.color = value ?? DEFAULT_TITLE_COLOR
}

function validate(): string | null {
  const name = form.name.trim()
  if (!name) return '请填写称号名称'
  if (name.length > 100) return '称号名称不能超过 100 字'
  if (!COLOR_RE.test(form.color)) return '称号颜色必须为 #RRGGBB 格式'
  if (form.unlockConditionType === 'rating') {
    if (!Number.isInteger(form.rating) || form.rating < 1) return '解锁 Rating 必须为正整数'
  } else if (form.unlockConditionType === 'ac_count') {
    if (!Number.isInteger(form.threshold) || form.threshold < 1) {
      return 'AC 题目数阈值必须为正整数'
    }
  } else if (!form.achievementCode.trim()) {
    return '请选择解锁所需的成就'
  }
  if (!Number.isInteger(form.priceCoins) || form.priceCoins < 0) {
    return '称号价格必须为非负整数'
  }
  if (form.priceCoins > 1_000_000) return '称号价格不能超过 1,000,000'
  if (!Number.isInteger(form.sortOrder)) return '排序必须为整数'
  return null
}

function buildInput(): TitleInput {
  const unlockConditionConfig =
    form.unlockConditionType === 'rating'
      ? { rating: form.rating }
      : form.unlockConditionType === 'ac_count'
        ? { threshold: form.threshold }
        : { achievementCode: form.achievementCode.trim() }
  return {
    name: form.name.trim(),
    color: form.color,
    unlockConditionType: form.unlockConditionType,
    unlockConditionConfig,
    priceCoins: form.priceCoins,
    sortOrder: form.sortOrder,
    isActive: form.isActive,
  }
}

async function save() {
  const problem = validate()
  if (problem) {
    ElMessage.warning(problem)
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      await updateTitle(editing.value.id, buildInput())
      ElMessage.success('称号已更新')
    } else {
      await createTitle(buildInput())
      ElMessage.success('称号已创建')
    }
    dialogVisible.value = false
    await load()
  } catch (e) {
    fail(e, '保存称号失败')
  } finally {
    saving.value = false
  }
}

async function remove(item: TitleItem) {
  try {
    await ElMessageBox.confirm(
      `确定删除称号「${item.name}」吗？删除后称号将下线，已解锁与已购买记录会保留。`,
      '删除称号',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  deletingId.value = item.id
  try {
    await deleteTitle(item.id)
    ElMessage.success('称号已删除')
    await load()
  } catch (e) {
    fail(e, '删除称号失败')
  } finally {
    deletingId.value = null
  }
}

onMounted(() => {
  void load()
  void loadAchievements()
})
</script>

<template>
  <div>
    <div class="toolbar">
      <span class="count">共 {{ titles.length }} 个称号</span>
      <el-button type="primary" @click="openCreate">新建称号</el-button>
    </div>

    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" class="banner" />

    <el-table v-loading="loading" :data="titles" empty-text="暂无称号" style="width: 100%">
      <el-table-column label="名称" min-width="140">
        <template #default="{ row }">
          <span class="title-name" :style="{ color: safeTitleColor(row.color) }">
            {{ row.name }}
          </span>
        </template>
      </el-table-column>
      <el-table-column label="解锁条件" min-width="220">
        <template #default="{ row }">{{ conditionText(row) }}</template>
      </el-table-column>
      <el-table-column label="价格" width="110">
        <template #default="{ row }">{{ formatCoins(row.priceCoins) }} 金币</template>
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
      :title="editing ? '编辑称号' : '新建称号'"
      width="620px"
      destroy-on-close
    >
      <el-form label-width="100px">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" maxlength="100" show-word-limit />
        </el-form-item>
        <el-form-item label="颜色" required>
          <div class="color-field">
            <el-color-picker
              :model-value="form.color"
              color-format="hex"
              @update:model-value="onColorChange"
            />
            <span class="color-preview" :style="{ color: safeTitleColor(form.color) }">
              {{ form.name.trim() || '称号效果预览' }}
            </span>
            <span class="color-value">{{ form.color }}</span>
          </div>
        </el-form-item>
        <el-form-item label="解锁类型" required>
          <el-select v-model="form.unlockConditionType" class="select-input">
            <el-option
              v-for="type in UNLOCK_TYPES"
              :key="type"
              :label="titleUnlockLabel(type)"
              :value="type"
            />
          </el-select>
        </el-form-item>

        <el-form-item v-if="form.unlockConditionType === 'rating'" label="Rating" required>
          <el-input-number v-model="form.rating" :min="1" :step="1" class="number-input" />
          <span class="hint">用户当前 Rating ≥ 该值时解锁，M4 起实时生效</span>
        </el-form-item>
        <el-form-item v-else-if="form.unlockConditionType === 'ac_count'" label="AC 题数" required>
          <el-input-number v-model="form.threshold" :min="1" :step="1" class="number-input" />
          <span class="hint">按去重后的 AC 题目数计算</span>
        </el-form-item>
        <el-form-item v-else label="所需成就" required>
          <el-select
            v-model="form.achievementCode"
            filterable
            :loading="achievementLoading"
            placeholder="选择已解锁成就"
            class="select-input"
          >
            <el-option
              v-for="option in achievementOptions"
              :key="option.value"
              :label="option.label"
              :value="option.value"
            />
          </el-select>
          <div v-if="achievementError" class="achievement-error">
            <span>{{ achievementError }}</span>
            <el-button link type="primary" @click="loadAchievements">重试</el-button>
          </div>
          <span v-else class="hint">用户解锁该成就后即可购买此称号</span>
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
.title-name {
  font-weight: 600;
}
.number-input {
  width: 180px;
}
.select-input {
  width: 280px;
}
.hint {
  margin-left: 8px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.color-field {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
}
.color-preview {
  padding: 0 4px;
  font-weight: 600;
}
.color-value {
  color: var(--el-text-color-secondary);
  font-family: var(--el-font-family-mono, monospace);
  font-size: 12px;
}
.achievement-error {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  width: 100%;
  margin-top: 4px;
  color: var(--el-color-danger);
  font-size: 12px;
}
@media (max-width: 640px) {
  :deep(.el-dialog) {
    max-width: calc(100vw - 32px);
  }
  :deep(.el-form-item__label) {
    width: 84px !important;
  }
  .select-input {
    width: 100%;
  }
  .number-input {
    width: 160px;
  }
}
</style>
