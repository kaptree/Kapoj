<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

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
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
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

const toast = useToast()
const confirm = useConfirm()
const achievements = ref<AchievementItem[]>([])
const badges = ref<BadgeItem[]>([])
const loading = ref(false)
const error = ref('')
const dialogVisible = ref(false)
const saving = ref(false)
const deletingId = ref<number | null>(null)
const editing = ref<AchievementItem | null>(null)
const originalBadgeId = ref<number | null>(null)

const conditionOptions = computed(() =>
  CONDITION_TYPES.map((type) => ({ value: type, label: achievementConditionLabel(type) })),
)
const childOptions = computed(() =>
  CHILD_TYPES.map((type) => ({ value: type, label: achievementConditionLabel(type) })),
)

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

const form = ref<FormState>(emptyForm())

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
  form.value = emptyForm()
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
  form.value = {
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
  }
  dialogVisible.value = true
}

function onConditionTypeChange(type: AchievementConditionType) {
  if (type === 'all_of' && form.value.conditions.length < 2) {
    form.value.conditions = [
      { type: 'ac_count', threshold: 1 },
      { type: 'contest_count', threshold: 1 },
    ]
  }
}

function addCondition() {
  if (form.value.conditions.length >= 8) {
    warn('组合条件最多 8 项')
    return
  }
  form.value.conditions.push({ type: 'ac_count', threshold: 1 })
}

function removeCondition(index: number) {
  if (form.value.conditions.length <= 2) {
    warn('组合条件至少 2 项')
    return
  }
  form.value.conditions.splice(index, 1)
}

function validate(): string | null {
  const current = form.value
  if (!current.name.trim()) return '请填写成就名称'
  if (!editing.value && !current.code.trim()) return '请填写成就 code'
  if (!editing.value && !CODE_RE.test(current.code.trim())) {
    return 'code 需为 2-64 位小写字母、数字、下划线或连字符，且以字母或数字开头'
  }
  if (current.conditionType === 'all_of') {
    if (current.conditions.length < 2 || current.conditions.length > 8) {
      return '组合条件必须为 2-8 项'
    }
    if (current.conditions.some((row) => row.type === 'all_of')) {
      return '组合条件不能嵌套 all_of'
    }
    if (current.conditions.some((row) => !Number.isInteger(row.threshold) || row.threshold < 1)) {
      return '每个条件阈值必须为正整数'
    }
  } else if (!Number.isInteger(current.threshold) || current.threshold < 1) {
    return '条件阈值必须为正整数'
  }
  if (
    !Number.isInteger(current.rewardCoins) ||
    current.rewardCoins < 0 ||
    current.rewardCoins > 1_000_000
  ) {
    return '奖励金币必须为 0-1,000,000 的整数'
  }
  if (!Number.isInteger(current.sortOrder)) return '排序必须为整数'
  return null
}

async function save() {
  const problem = validate()
  if (problem) {
    warn(problem)
    return
  }
  const current = form.value
  const input: AchievementInput = {
    code: current.code.trim(),
    name: current.name.trim(),
    description: current.description.trim(),
    iconUrl: current.iconUrl.trim(),
    conditionType: current.conditionType,
    conditionConfig:
      current.conditionType === 'all_of'
        ? {
            conditions: current.conditions.map((row) => ({
              type: row.type,
              threshold: row.threshold,
            })),
          }
        : { threshold: current.threshold },
    rewardCoins: current.rewardCoins,
    rewardBadgeId: current.rewardBadgeId,
    sortOrder: current.sortOrder,
    isActive: current.isActive,
  }

  saving.value = true
  try {
    if (editing.value) {
      await updateAchievement(editing.value.id, input)
      toast.add({ severity: 'success', summary: '成就已更新', life: 3000 })
    } else {
      await createAchievement(input)
      toast.add({ severity: 'success', summary: '成就已创建', life: 3000 })
    }
    dialogVisible.value = false
    await load()
  } catch (e) {
    fail(e, '保存成就失败')
  } finally {
    saving.value = false
  }
}

function remove(item: AchievementItem) {
  confirm.require({
    header: '删除成就',
    message: `确定删除成就「${item.name}」吗？删除后不再展示，已解锁记录与奖励会保留。`,
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

async function doRemove(item: AchievementItem) {
  deletingId.value = item.id
  try {
    await deleteAchievement(item.id)
    toast.add({ severity: 'success', summary: '成就已删除', life: 3000 })
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
  <div class="admin-panel">
    <div class="toolbar">
      <span class="count">共 {{ achievements.length }} 个成就</span>
      <Button label="新建成就" icon="pi pi-plus" size="small" @click="openCreate" />
    </div>

    <AppErrorState v-if="error && !achievements.length" :description="error" @retry="load" />
    <template v-else>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <DataTable
        :value="achievements"
        :loading="loading"
        data-key="id"
        size="small"
        responsive-layout="scroll"
        class="admin-table"
      >
        <Column header="名称" style="min-width: 140px">
          <template #body="{ data }">{{ data.name }}</template>
        </Column>
        <Column header="code" style="min-width: 140px">
          <template #body="{ data }">{{ data.code }}</template>
        </Column>
        <Column header="条件" style="min-width: 220px">
          <template #body="{ data }">{{ conditionText(data) }}</template>
        </Column>
        <Column header="奖励" style="min-width: 180px">
          <template #body="{ data }">
            <div class="rewards">
              <Tag
                v-if="data.rewardCoins > 0"
                severity="warn"
                :value="`金币 ${formatCoins(data.rewardCoins)}`"
              />
              <Tag
                v-if="data.rewardBadgeId !== null"
                severity="info"
                :value="`徽章 ${badgeName(data.rewardBadgeId)}`"
              />
              <span v-if="data.rewardCoins <= 0 && data.rewardBadgeId === null" class="muted"
                >无</span
              >
            </div>
          </template>
        </Column>
        <Column header="排序" style="width: 80px">
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
        <Column header="操作" style="width: 150px">
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
        <template #empty><AppEmptyState title="暂无成就" /></template>
      </DataTable>
    </template>

    <Dialog
      v-model:visible="dialogVisible"
      :header="editing ? '编辑成就' : '新建成就'"
      modal
      :draggable="false"
      :style="{ width: 'min(94vw, 680px)' }"
    >
      <div class="admin-form">
        <div class="field">
          <label for="achievement-code">code <span class="required">*</span></label>
          <InputText
            id="achievement-code"
            v-model="form.code"
            class="code-input"
            :disabled="editing !== null"
            maxlength="64"
            placeholder="例如 ac_100"
            fluid
          />
        </div>
        <div class="field">
          <label for="achievement-name">名称 <span class="required">*</span></label>
          <InputText
            id="achievement-name"
            v-model="form.name"
            class="name-input"
            maxlength="100"
            fluid
          />
        </div>
        <div class="field span-2">
          <label for="achievement-description">描述</label>
          <Textarea
            id="achievement-description"
            v-model="form.description"
            class="description-input"
            rows="3"
            maxlength="500"
            fluid
          />
        </div>
        <div class="field span-2">
          <label for="achievement-icon">图标 URL</label>
          <InputText
            id="achievement-icon"
            v-model="form.iconUrl"
            maxlength="512"
            placeholder="可选，http(s) 图片地址"
            fluid
          />
        </div>
        <div class="field">
          <label for="achievement-condition">条件类型 <span class="required">*</span></label>
          <Select
            v-model="form.conditionType"
            input-id="achievement-condition"
            :options="conditionOptions"
            option-label="label"
            option-value="value"
            fluid
            @update:model-value="onConditionTypeChange"
          />
        </div>
        <div v-if="form.conditionType !== 'all_of'" class="field">
          <label for="achievement-threshold">阈值 <span class="required">*</span></label>
          <InputNumber
            v-model="form.threshold"
            input-id="achievement-threshold"
            :min="1"
            :step="1"
            fluid
          />
        </div>

        <div v-else class="conditions span-2">
          <span class="field-label">组合条件 <span class="required">*</span></span>
          <div v-for="(row, index) in form.conditions" :key="index" class="condition-row">
            <Select
              v-model="row.type"
              :options="childOptions"
              option-label="label"
              option-value="value"
              fluid
            />
            <span class="condition-label">≥</span>
            <InputNumber v-model="row.threshold" :min="1" :step="1" fluid />
            <Button
              label="移除"
              text
              size="small"
              severity="danger"
              @click="removeCondition(index)"
            />
          </div>
          <Button
            label="添加条件（最多 8 项）"
            text
            size="small"
            icon="pi pi-plus"
            @click="addCondition"
          />
        </div>

        <div class="field">
          <label for="achievement-reward">奖励金币</label>
          <InputNumber
            v-model="form.rewardCoins"
            input-id="achievement-reward"
            :min="0"
            :max="1000000"
            :step="1"
            fluid
          />
        </div>
        <div class="field">
          <label for="achievement-badge">奖励徽章</label>
          <Select
            v-model="form.rewardBadgeId"
            input-id="achievement-badge"
            :options="badgeOptions"
            option-label="label"
            option-value="value"
            option-disabled="disabled"
            show-clear
            placeholder="不配置奖励徽章"
            fluid
          />
          <span class="hint">奖励徽章只能更换，不能清除；创建时可留空不配置。</span>
        </div>

        <div class="field">
          <label for="achievement-sort">排序</label>
          <InputNumber v-model="form.sortOrder" input-id="achievement-sort" :step="1" fluid />
        </div>
        <div class="field toggle-field">
          <span class="field-label">上架</span>
          <div class="toggle-row">
            <ToggleSwitch v-model="form.isActive" input-id="achievement-active" />
            <label for="achievement-active">{{ form.isActive ? '已上架' : '已下线' }}</label>
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

.rewards,
.row-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-1);
}

.muted {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.admin-form {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--kapoj-space-3);
}

.field,
.conditions {
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

.condition-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto minmax(110px, 0.5fr) auto;
  gap: var(--kapoj-space-2);
  align-items: center;
}

.condition-label {
  color: var(--kapoj-text-secondary);
}

.toggle-row {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.toggle-row label {
  color: var(--kapoj-text);
  cursor: pointer;
}

@media (max-width: 640px) {
  .admin-form {
    grid-template-columns: minmax(0, 1fr);
  }

  .condition-row {
    grid-template-columns: minmax(0, 1fr) auto minmax(90px, 0.6fr) auto;
  }
}
</style>
