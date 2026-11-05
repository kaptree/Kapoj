<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import ColorPicker from 'primevue/colorpicker'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import {
  createTitle,
  deleteTitle,
  listAchievements,
  listTitles,
  updateTitle,
} from '@/api/gamification'
import { ApiError } from '@/api/http'
import type { AchievementItem, TitleInput, TitleItem, TitleUnlockType } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
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

const toast = useToast()
const confirm = useConfirm()
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

const form = ref<FormState>(emptyForm())
const unlockOptions = computed(() =>
  UNLOCK_TYPES.map((type) => ({ value: type, label: titleUnlockLabel(type) })),
)
const achievementOptions = computed<AchievementOption[]>(() => {
  const options = achievements.value.map((item) => ({
    value: item.code,
    label: `${item.code} · ${item.name}${item.isActive ? '' : '（已下线）'}`,
  }))
  const code = form.value.achievementCode.trim()
  if (code && !options.some((option) => option.value === code)) {
    options.push({ value: code, label: `${code}（未找到对应成就）` })
  }
  return options
})

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
  form.value = emptyForm()
  dialogVisible.value = true
  if (achievements.value.length === 0) void loadAchievements()
}

function openEdit(item: TitleItem) {
  editing.value = item
  form.value = {
    name: item.name,
    color: item.color,
    unlockConditionType: item.unlockConditionType,
    rating: item.unlockConditionConfig.rating ?? 1600,
    threshold: item.unlockConditionConfig.threshold ?? 1,
    achievementCode: item.unlockConditionConfig.achievementCode ?? '',
    priceCoins: item.priceCoins,
    sortOrder: item.sortOrder,
    isActive: item.isActive,
  }
  dialogVisible.value = true
  if (achievements.value.length === 0) void loadAchievements()
}

function validate(): string | null {
  const current = form.value
  if (!current.name.trim()) return '请填写称号名称'
  if (current.name.trim().length > 100) return '称号名称不能超过 100 字'
  if (!COLOR_RE.test(current.color)) return '称号颜色必须为 #RRGGBB 格式'
  if (current.unlockConditionType === 'rating') {
    if (!Number.isInteger(current.rating) || current.rating < 1) {
      return '解锁 Rating 必须为正整数'
    }
  } else if (current.unlockConditionType === 'ac_count') {
    if (!Number.isInteger(current.threshold) || current.threshold < 1) {
      return 'AC 题目数阈值必须为正整数'
    }
  } else if (!current.achievementCode.trim()) {
    return '请选择解锁所需的成就'
  }
  if (!Number.isInteger(current.priceCoins) || current.priceCoins < 0) {
    return '称号价格必须为非负整数'
  }
  if (current.priceCoins > 1_000_000) return '称号价格不能超过 1,000,000'
  if (!Number.isInteger(current.sortOrder)) return '排序必须为整数'
  return null
}

function buildInput(): TitleInput {
  const current = form.value
  const unlockConditionConfig =
    current.unlockConditionType === 'rating'
      ? { rating: current.rating }
      : current.unlockConditionType === 'ac_count'
        ? { threshold: current.threshold }
        : { achievementCode: current.achievementCode.trim() }
  return {
    name: current.name.trim(),
    color: current.color,
    unlockConditionType: current.unlockConditionType,
    unlockConditionConfig,
    priceCoins: current.priceCoins,
    sortOrder: current.sortOrder,
    isActive: current.isActive,
  }
}

async function save() {
  const problem = validate()
  if (problem) {
    warn(problem)
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      await updateTitle(editing.value.id, buildInput())
      toast.add({ severity: 'success', summary: '称号已更新', life: 3000 })
    } else {
      await createTitle(buildInput())
      toast.add({ severity: 'success', summary: '称号已创建', life: 3000 })
    }
    dialogVisible.value = false
    await load()
  } catch (e) {
    fail(e, '保存称号失败')
  } finally {
    saving.value = false
  }
}

function remove(item: TitleItem) {
  confirm.require({
    header: '删除称号',
    message: `确定删除称号「${item.name}」吗？删除后称号将下线，已解锁与已购买记录会保留。`,
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

async function doRemove(item: TitleItem) {
  deletingId.value = item.id
  try {
    await deleteTitle(item.id)
    toast.add({ severity: 'success', summary: '称号已删除', life: 3000 })
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
  <div class="admin-panel">
    <div class="toolbar">
      <span class="count">共 {{ titles.length }} 个称号</span>
      <Button label="新建称号" icon="pi pi-plus" size="small" @click="openCreate" />
    </div>

    <AppErrorState v-if="error && !titles.length" :description="error" @retry="load" />
    <template v-else>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <DataTable
        :value="titles"
        :loading="loading"
        data-key="id"
        size="small"
        responsive-layout="scroll"
        class="admin-table"
      >
        <Column header="名称" style="min-width: 140px">
          <template #body="{ data }">
            <span
              class="title-name"
              :style="{ color: safeTitleColor(data.color, 'var(--kapoj-text)') }"
            >
              {{ data.name }}
            </span>
          </template>
        </Column>
        <Column header="解锁条件" style="min-width: 220px">
          <template #body="{ data }">{{ conditionText(data) }}</template>
        </Column>
        <Column header="价格" style="width: 110px">
          <template #body="{ data }">{{ formatCoins(data.priceCoins) }} 金币</template>
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
        <template #empty><AppEmptyState title="暂无称号" /></template>
      </DataTable>
    </template>

    <Dialog
      v-model:visible="dialogVisible"
      :header="editing ? '编辑称号' : '新建称号'"
      modal
      :draggable="false"
      :style="{ width: 'min(94vw, 620px)' }"
    >
      <div class="admin-form">
        <div class="field">
          <label for="title-name">名称 <span class="required">*</span></label>
          <InputText id="title-name" v-model="form.name" maxlength="100" fluid />
        </div>
        <div class="field">
          <span class="field-label">颜色 <span class="required">*</span></span>
          <div class="color-field">
            <ColorPicker v-model="form.color" format="hex" />
            <span
              class="color-preview"
              :style="{ color: safeTitleColor(form.color, 'var(--kapoj-text)') }"
            >
              {{ form.name.trim() || '称号效果预览' }}
            </span>
            <span class="color-value">{{ form.color }}</span>
          </div>
        </div>

        <div class="field">
          <label for="title-unlock">解锁类型 <span class="required">*</span></label>
          <Select
            v-model="form.unlockConditionType"
            input-id="title-unlock"
            :options="unlockOptions"
            option-label="label"
            option-value="value"
            fluid
          />
        </div>

        <div v-if="form.unlockConditionType === 'rating'" class="field">
          <label for="title-rating">Rating <span class="required">*</span></label>
          <InputNumber v-model="form.rating" input-id="title-rating" :min="1" :step="1" fluid />
          <span class="hint">用户当前 Rating ≥ 该值时解锁</span>
        </div>
        <div v-else-if="form.unlockConditionType === 'ac_count'" class="field">
          <label for="title-threshold">AC 题数 <span class="required">*</span></label>
          <InputNumber
            v-model="form.threshold"
            input-id="title-threshold"
            :min="1"
            :step="1"
            fluid
          />
          <span class="hint">按去重后的 AC 题目数计算</span>
        </div>
        <div v-else class="field">
          <label for="title-achievement">所需成就 <span class="required">*</span></label>
          <Select
            v-model="form.achievementCode"
            input-id="title-achievement"
            :options="achievementOptions"
            option-label="label"
            option-value="value"
            filter
            :loading="achievementLoading"
            placeholder="选择已解锁成就"
            fluid
          />
          <div v-if="achievementError" class="achievement-error">
            <span>{{ achievementError }}</span>
            <Button label="重试" text size="small" @click="loadAchievements" />
          </div>
          <span v-else class="hint">用户解锁该成就后即可购买此称号</span>
        </div>

        <div class="field">
          <label for="title-price">价格</label>
          <InputNumber
            v-model="form.priceCoins"
            input-id="title-price"
            :min="0"
            :max="1000000"
            :step="1"
            fluid
          />
          <span class="hint">金币，0 表示免费</span>
        </div>
        <div class="field">
          <label for="title-sort">排序</label>
          <InputNumber v-model="form.sortOrder" input-id="title-sort" :step="1" fluid />
        </div>
        <div class="field toggle-field">
          <span class="field-label">上架</span>
          <div class="toggle-row">
            <ToggleSwitch v-model="form.isActive" input-id="title-active" />
            <label for="title-active">{{ form.isActive ? '已上架' : '已下线' }}</label>
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

.title-name {
  font-weight: 600;
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

.color-field {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.color-preview {
  font-weight: 600;
}

.color-value {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.achievement-error {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-1);
  color: var(--kapoj-danger);
  font-size: 12px;
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
}
</style>
