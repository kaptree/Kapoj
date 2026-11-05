<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'

import {
  createAchievement,
  deleteAchievement,
  listAchievements,
  listBadges,
  updateAchievement,
} from '@/api/gamification'
import { ApiError } from '@/api/http'
import type {
  AchievementConditionType,
  AchievementInput,
  AchievementItem,
  BadgeItem,
} from '@/api/types'
import { achievementConditionLabel, formatCoins } from '@/utils/gamification'

interface ConditionRow {
  type: AchievementConditionType
  threshold: number
}

interface FormState {
  code: string
  name: string
  description: string
  iconUrl: string
  conditionType: AchievementConditionType
  threshold: number
  conditions: ConditionRow[]
  rewardCoins: number
  rewardBadgeId: number | null
  sortOrder: number
  isActive: boolean
}

interface BadgeOption {
  value: number
  label: string
  disabled: boolean
}

const CONDITION_TYPES: AchievementConditionType[] = [
  'ac_count',
  'contest_count',
  'login_streak',
  'created_contest',
  'all_of',
]
const CHILD_TYPES = CONDITION_TYPES.filter((type) => type !== 'all_of')
const CODE_RE = /^[a-z0-9][a-z0-9_-]{1,63}$/

const achievements = ref<AchievementItem[]>([])
const badges = ref<BadgeItem[]>([])
const loading = ref(false)
const error = ref('')
const dialogVisible = ref(false)
const saving = ref(false)
const deletingId = ref<number | null>(null)
const editing = ref<AchievementItem | null>(null)
const originalBadgeId = ref<number | null>(null)

function emptyForm(): FormState {
  return {
    code: '',
    name: '',
    description: '',
    iconUrl: '',
    conditionType: 'ac_count',
    threshold: 1,
    conditions: [],
    rewardCoins: 0,
    rewardBadgeId: null,
    sortOrder: 0,
    isActive: true,
  }
}

const form = reactive<FormState>(emptyForm())

const badgeOptions = computed<BadgeOption[]>(() => {
  const options: BadgeOption[] = badges.value
    .filter((badge) => badge.isActive)
    .map((badge) => ({ value: badge.id, label: badge.name, disabled: false }))
  const original = originalBadgeId.value
  if (original !== null && !options.some((option) => option.value === original)) {
    const badge = badges.value.find((item) => item.id === original)
    options.push({
      value: original,
      label: badge ? `${badge.name}（已下线）` : `#${original}（已下线）`,
      disabled: true,
    })
  }
  return options
})

function badgeName(id: number | null): string {
  if (id === null) return ''
  const badge = badges.value.find((item) => item.id === id)
  return badge ? badge.name : `#${id}`
}

function conditionText(item: AchievementItem): string {
  if (item.conditionType === 'all_of') {
    const rows = item.conditionConfig.conditions ?? []
    if (rows.length === 0) return achievementConditionLabel('all_of')
    return rows
      .map((row) => `${achievementConditionLabel(row.type)} ≥ ${row.threshold}`)
      .join(' 且 ')
  }
  return `${achievementConditionLabel(item.conditionType)} ≥ ${item.conditionConfig.threshold ?? 0}`
}

function fail(e: unknown, fallback: string) {
  ElMessage.error(e instanceof ApiError ? e.message : fallback)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [list, badgeList] = await Promise.all([
      listAchievements({ includeInactive: true }),
      listBadges({ includeInactive: true }),
    ])
    achievements.value = list
    badges.value = badgeList
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '加载成就失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  originalBadgeId.value = null
  Object.assign(form, emptyForm())
  dialogVisible.value = true
}

function openEdit(item: AchievementItem) {
  editing.value = item
  originalBadgeId.value = item.rewardBadgeId
  const conditions = (item.conditionConfig.conditions ?? []).map((row) => ({
    type: row.type === 'all_of' ? 'ac_count' : row.type,
    threshold: row.threshold,
  }))
  while (conditions.length < 2) {
    conditions.push({ type: conditions.length === 0 ? 'ac_count' : 'contest_count', threshold: 1 })
  }
  Object.assign(form, {
    code: item.code,
    name: item.name,
    description: item.description,
    iconUrl: item.iconUrl,
    conditionType: item.conditionType,
    threshold: item.conditionConfig.threshold ?? 1,
    conditions,
    rewardCoins: item.rewardCoins,
    rewardBadgeId: item.rewardBadgeId,
    sortOrder: item.sortOrder,
    isActive: item.isActive,
  })
  dialogVisible.value = true
}

function onConditionTypeChange(type: AchievementConditionType) {
  if (type === 'all_of' && form.conditions.length < 2) {
    form.conditions = [
      { type: 'ac_count', threshold: 1 },
      { type: 'contest_count', threshold: 1 },
    ]
  }
}

function addCondition() {
  if (form.conditions.length >= 8) {
    ElMessage.warning('组合条件最多 8 项')
    return
  }
  form.conditions.push({ type: 'ac_count', threshold: 1 })
}

function removeCondition(index: number) {
  if (form.conditions.length <= 2) {
    ElMessage.warning('组合条件至少 2 项')
    return
  }
  form.conditions.splice(index, 1)
}

function validate(): string | null {
  if (!form.name.trim()) return '请填写成就名称'
  if (!editing.value && !form.code.trim()) return '请填写成就 code'
  if (!editing.value && !CODE_RE.test(form.code.trim())) {
    return 'code 需为 2-64 位小写字母、数字、下划线或连字符，且以字母或数字开头'
  }
  if (form.conditionType === 'all_of') {
    if (form.conditions.length < 2 || form.conditions.length > 8) {
      return '组合条件必须为 2-8 项'
    }
    if (form.conditions.some((row) => row.type === 'all_of')) {
      return '组合条件不能嵌套 all_of'
    }
    if (form.conditions.some((row) => !Number.isInteger(row.threshold) || row.threshold < 1)) {
      return '每个条件阈值必须为正整数'
    }
  } else if (!Number.isInteger(form.threshold) || form.threshold < 1) {
    return '条件阈值必须为正整数'
  }
  if (!Number.isInteger(form.rewardCoins) || form.rewardCoins < 0 || form.rewardCoins > 1_000_000) {
    return '奖励金币必须为 0-1,000,000 的整数'
  }
  if (!Number.isInteger(form.sortOrder)) return '排序必须为整数'
  return null
}

async function save() {
  const problem = validate()
  if (problem) {
    ElMessage.warning(problem)
    return
  }
  const input: AchievementInput = {
    code: form.code.trim(),
    name: form.name.trim(),
    description: form.description.trim(),
    iconUrl: form.iconUrl.trim(),
    conditionType: form.conditionType,
    conditionConfig:
      form.conditionType === 'all_of'
        ? {
            conditions: form.conditions.map((row) => ({
              type: row.type,
              threshold: row.threshold,
            })),
          }
        : { threshold: form.threshold },
    rewardCoins: form.rewardCoins,
    rewardBadgeId: form.rewardBadgeId,
    sortOrder: form.sortOrder,
    isActive: form.isActive,
  }

  saving.value = true
  try {
    if (editing.value) {
      await updateAchievement(editing.value.id, input)
      ElMessage.success('成就已更新')
    } else {
      await createAchievement(input)
      ElMessage.success('成就已创建')
    }
    dialogVisible.value = false
    await load()
  } catch (e) {
    fail(e, '保存成就失败')
  } finally {
    saving.value = false
  }
}

async function remove(item: AchievementItem) {
  try {
    await ElMessageBox.confirm(
      `确定删除成就「${item.name}」吗？删除后不再展示，已解锁记录与奖励会保留。`,
      '删除成就',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  deletingId.value = item.id
  try {
    await deleteAchievement(item.id)
    ElMessage.success('成就已删除')
    await load()
  } catch (e) {
    fail(e, '删除成就失败')
  } finally {
    deletingId.value = null
  }
}

onMounted(load)
</script>

<template>
  <div>
    <div class="toolbar">
      <span class="count">共 {{ achievements.length }} 个成就</span>
      <el-button type="primary" @click="openCreate">新建成就</el-button>
    </div>

    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" class="banner" />

    <el-table v-loading="loading" :data="achievements" empty-text="暂无成就">
      <el-table-column prop="name" label="名称" min-width="140" />
      <el-table-column prop="code" label="code" min-width="140" />
      <el-table-column label="条件" min-width="220">
        <template #default="{ row }">{{ conditionText(row) }}</template>
      </el-table-column>
      <el-table-column label="奖励" min-width="180">
        <template #default="{ row }">
          <div class="rewards">
            <el-tag v-if="row.rewardCoins > 0" type="warning" size="small">
              金币 {{ formatCoins(row.rewardCoins) }}
            </el-tag>
            <el-tag v-if="row.rewardBadgeId !== null" type="primary" size="small">
              徽章 {{ badgeName(row.rewardBadgeId) }}
            </el-tag>
            <span v-if="row.rewardCoins <= 0 && row.rewardBadgeId === null" class="muted">无</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="sortOrder" label="排序" width="80" />
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag :type="row.isActive ? 'success' : 'info'" size="small">
            {{ row.isActive ? '已上架' : '已下线' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="150" fixed="right">
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
      :title="editing ? '编辑成就' : '新建成就'"
      width="680px"
      destroy-on-close
    >
      <el-form label-width="100px">
        <el-form-item label="code" required>
          <el-input
            v-model="form.code"
            :disabled="editing !== null"
            maxlength="64"
            placeholder="例如 ac_100"
          />
        </el-form-item>
        <el-form-item label="名称" required>
          <el-input v-model="form.name" maxlength="100" show-word-limit />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="3" maxlength="500" />
        </el-form-item>
        <el-form-item label="图标 URL">
          <el-input v-model="form.iconUrl" maxlength="512" placeholder="可选，http(s) 图片地址" />
        </el-form-item>
        <el-form-item label="条件类型" required>
          <el-select
            v-model="form.conditionType"
            style="width: 220px"
            @change="onConditionTypeChange"
          >
            <el-option
              v-for="type in CONDITION_TYPES"
              :key="type"
              :label="achievementConditionLabel(type)"
              :value="type"
            />
          </el-select>
        </el-form-item>

        <el-form-item v-if="form.conditionType !== 'all_of'" label="阈值" required>
          <el-input-number v-model="form.threshold" :min="1" :step="1" />
        </el-form-item>

        <template v-else>
          <el-form-item
            v-for="(row, index) in form.conditions"
            :key="index"
            :label="index === 0 ? '组合条件' : ''"
          >
            <div class="condition-row">
              <el-select v-model="row.type" style="width: 180px">
                <el-option
                  v-for="type in CHILD_TYPES"
                  :key="type"
                  :label="achievementConditionLabel(type)"
                  :value="type"
                />
              </el-select>
              <span class="condition-label">≥</span>
              <el-input-number v-model="row.threshold" :min="1" :step="1" />
              <el-button link type="danger" @click="removeCondition(index)">移除</el-button>
            </div>
          </el-form-item>
          <el-form-item label=" ">
            <el-button size="small" @click="addCondition">添加条件（最多 8 项）</el-button>
          </el-form-item>
        </template>

        <el-form-item label="奖励金币">
          <el-input-number v-model="form.rewardCoins" :min="0" :max="1000000" :step="1" />
        </el-form-item>
        <el-form-item label="奖励徽章">
          <el-select
            v-model="form.rewardBadgeId"
            clearable
            placeholder="不配置奖励徽章"
            style="width: 260px"
          >
            <el-option
              v-for="option in badgeOptions"
              :key="option.value"
              :label="option.label"
              :value="option.value"
              :disabled="option.disabled"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sortOrder" :step="1" />
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
.rewards {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.muted {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.condition-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.condition-label {
  color: var(--el-text-color-secondary);
}
@media (max-width: 640px) {
  :deep(.el-dialog) {
    max-width: calc(100vw - 32px);
  }
}
</style>
