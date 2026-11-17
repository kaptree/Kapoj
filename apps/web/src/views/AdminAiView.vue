<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import { copyText } from '@/utils/clipboard'
import { formatDateTime } from '@/utils/format'

import {
  createAdminAIPlatformModel,
  deleteAdminAIPlatformModel,
  generateAdminAIKey,
  getAdminAIKey,
  getAdminAISettings,
  listAdminAIPlatformModels,
  resetAdminAIKey,
  selectAdminAIPlatformModel,
  setAdminAIKey,
  testAdminAIPlatformModel,
  updateAdminAIPlatformModel,
  updateAdminAISettings,
} from '@/api/admin'
import { ApiError } from '@/api/http'
import type {
  AdminAISettingsView,
  AIKeyView,
  PlatformModelInput,
  PlatformModelView,
} from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const toast = useToast()
const confirm = useConfirm()

const canManage = computed(() => auth.can(PERM.ai.settings.manage))

const loading = ref(false)
const error = ref('')
const saving = ref(false)
const settings = ref<AdminAISettingsView | null>(null)
const enabled = ref(false)

const keyLoading = ref(false)
const keyError = ref('')
const keySaving = ref(false)
const key = ref<AIKeyView | null>(null)
const generatedKey = ref('')
const manualKey = ref('')

const keyConfigured = computed(() => settings.value?.keyConfigured ?? false)

/** Effective status: without a configured key AI is unavailable regardless of the flag. */
const statusLabel = computed(() => {
  if (!keyConfigured.value) return '不可用'
  return enabled.value ? '已启用' : '已关闭'
})

const statusSeverity = computed<'secondary' | 'success' | 'danger'>(() => {
  if (!keyConfigured.value) return 'secondary'
  return enabled.value ? 'success' : 'danger'
})

/** Saving is only blocked when the toggle is on while no key is configured (enabling is impossible). */
const disableSave = computed(() => (!keyConfigured.value && enabled.value) || saving.value)

const keySource = computed(() => key.value?.source ?? 'none')
const isEnvSource = computed(() => keySource.value === 'env')
const hasFileKey = computed(() => key.value?.configured === true && keySource.value === 'file')

const keyStatusLabel = computed(() => {
  if (isEnvSource.value) return '由部署配置管理'
  if (hasFileKey.value) return '已配置（文件）'
  return '未配置'
})

const keyStatusSeverity = computed<'secondary' | 'success' | 'info' | 'warn' | 'danger'>(() => {
  if (isEnvSource.value) return 'info'
  if (hasFileKey.value) return 'success'
  return 'secondary'
})

// --- Platform models --------------------------------------------------------

interface PlatformModelForm {
  name: string
  baseUrl: string
  modelName: string
  apiKey: string
  enabled: boolean
}

function emptyModelForm(): PlatformModelForm {
  return { name: '', baseUrl: '', modelName: '', apiKey: '', enabled: true }
}

const models = ref<PlatformModelView[]>([])
const modelsLoading = ref(false)
const modelsError = ref('')
/** Server-persisted selection; drives the “当前使用” tag and delete guard. */
const selectedModelId = ref<number | null>(null)
/** Unsaved Select draft; only becomes the persisted selection after a successful save. */
const selectionDraft = ref<number | null>(null)
const selectionSaving = ref(false)

/** Kept in script so the `://` literal never appears in the template. */
const BASE_URL_PLACEHOLDER = 'https://api.example.com/v1'

const modelDialogVisible = ref(false)
const modelDialogMode = ref<'create' | 'edit'>('create')
const editingModelId = ref<number | null>(null)
const editingModel = ref<PlatformModelView | null>(null)
const modelForm = ref<PlatformModelForm>(emptyModelForm())
const modelFormErrors = ref({ name: '', baseUrl: '', modelName: '', apiKey: '' })
const modelSaving = ref(false)
const testingModelId = ref<number | null>(null)

/** Tokens are encrypted with the platform key, so adds/edits need one configured. */
const canEditModels = computed(() => canManage.value && keyConfigured.value)
const isEditingModel = computed(() => modelDialogMode.value === 'edit')

const apiKeyHint = computed(() => {
  if (!isEditingModel.value) return 'Token 由服务端加密保存，页面不会显示明文'
  return editingModel.value?.apiKeyMasked
    ? `留空则保留原 Token（当前：${editingModel.value.apiKeyMasked}）`
    : '留空则保留原 Token'
})

/** Enabled models plus "未选择"; keep the current pick visible even if it was disabled. */
const modelOptions = computed<{ label: string; value: number | null }[]>(() => {
  const options = models.value
    .filter((model) => model.enabled)
    .map((model) => ({ label: model.name, value: model.id }))
  const current = models.value.find((model) => model.id === selectedModelId.value)
  if (current && !current.enabled) {
    options.push({ label: `${current.name}（已停用）`, value: current.id })
  }
  return [{ label: '未选择', value: null }, ...options]
})

/** The persisted selection may point at a model that was disabled later. */
const selectedModelDisabled = computed(() => {
  if (selectedModelId.value === null) return false
  const current = models.value.find((model) => model.id === selectedModelId.value)
  return current !== undefined && !current.enabled
})

function modelStatusSeverity(model: PlatformModelView): 'secondary' | 'success' | 'danger' {
  if (!model.lastTestAt) return 'secondary'
  return model.lastTestOk ? 'success' : 'danger'
}

function modelStatusLabel(model: PlatformModelView): string {
  if (!model.lastTestAt) return '未测试'
  return model.lastTestOk ? '成功' : '失败'
}

function applyView(view: AdminAISettingsView) {
  settings.value = view
  enabled.value = view.enabled
  selectedModelId.value = view.platformModelId ?? null
  selectionDraft.value = selectedModelId.value
}

/** Applies only the selection fields so an unsaved enable-toggle draft is preserved. */
function applySelection(view: AdminAISettingsView) {
  settings.value = {
    ...(settings.value ?? view),
    platformModelId: view.platformModelId ?? null,
    platformModelName: view.platformModelName ?? '',
  }
  selectedModelId.value = settings.value.platformModelId ?? null
  selectionDraft.value = selectedModelId.value
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    applyView(await getAdminAISettings())
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '加载 AI 设置失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

/** Refresh the settings view after a key change so the toggle stays in sync. */
async function refreshSettings() {
  try {
    applyView(await getAdminAISettings())
  } catch {
    // A failed refresh must not mask the key operation result.
  }
}

async function loadKey() {
  keyLoading.value = true
  keyError.value = ''
  try {
    key.value = await getAdminAIKey()
  } catch (e) {
    keyError.value = e instanceof ApiError ? e.message : '加载密钥状态失败，请稍后再试'
  } finally {
    keyLoading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    applyView(await updateAdminAISettings({ enabled: enabled.value }))
    toast.add({ severity: 'success', summary: '设置已保存', life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '保存失败',
      detail: e instanceof ApiError ? e.message : '保存 AI 设置失败，请稍后再试',
      life: 4000,
    })
  } finally {
    saving.value = false
  }
}

function confirmGenerate() {
  if (hasFileKey.value) {
    confirm.require({
      header: '重新生成密钥',
      message: '重新生成会立即轮换密钥并重加密所有用户 AI Key，请确保已备份现有密钥。',
      icon: 'pi pi-exclamation-triangle',
      acceptLabel: '重新生成',
      rejectLabel: '取消',
      accept: () => void doGenerate(),
    })
    return
  }
  void doGenerate()
}

async function doGenerate() {
  keySaving.value = true
  try {
    const result = await generateAdminAIKey()
    key.value = result
    generatedKey.value = result.key
    manualKey.value = ''
    await refreshSettings()
    toast.add({
      severity: 'success',
      summary: '密钥已生成',
      detail: '请立即备份下方一次性密钥',
      life: 4000,
    })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '生成失败',
      detail: e instanceof ApiError ? e.message : '生成密钥失败，请稍后再试',
      life: 4000,
    })
  } finally {
    keySaving.value = false
  }
}

function confirmResetKey() {
  confirm.require({
    header: '清除旧密钥并生成新密钥',
    message:
      '将删除所有用户已保存的 AI API Key，清空所有平台模型 Token 并重置当前模型选择，然后生成新的平台加密密钥。此操作不可撤销，用户需要重新填写自己的 API Key，平台模型需要重新填写 Token。',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '清除并生成',
    rejectLabel: '取消',
    accept: () => void doResetKey(),
  })
}

async function doResetKey() {
  keySaving.value = true
  generatedKey.value = ''
  try {
    const result = await resetAdminAIKey()
    key.value = result
    generatedKey.value = result.key
    manualKey.value = ''
    await refreshSettings()
    // The reset also disables every platform model and clears the selection;
    // refresh the table so enabled/masked state is not stale.
    await loadModels()
    toast.add({
      severity: 'success',
      summary: '旧密钥已清除',
      detail:
        '新密钥已生成；请立即备份。用户需重新填写 AI API Key，平台模型 Token 与当前选择已清空',
      life: 4000,
    })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '重置失败',
      detail: e instanceof ApiError ? e.message : '重置密钥失败，请稍后再试',
      life: 4000,
    })
  } finally {
    keySaving.value = false
  }
}

function confirmSaveManual() {
  const value = manualKey.value.trim()
  if (value === '') {
    toast.add({ severity: 'warn', summary: '请填写密钥', life: 3000 })
    return
  }
  if (hasFileKey.value) {
    confirm.require({
      header: '手动替换密钥',
      message: '替换密钥会立即轮换并重加密所有用户 AI Key，请确保已备份现有密钥。',
      icon: 'pi pi-exclamation-triangle',
      acceptLabel: '替换',
      rejectLabel: '取消',
      accept: () => void doSaveManual(value),
    })
    return
  }
  void doSaveManual(value)
}

async function doSaveManual(value: string) {
  keySaving.value = true
  try {
    const view = await setAdminAIKey({ key: value })
    key.value = view
    generatedKey.value = ''
    manualKey.value = ''
    await refreshSettings()
    toast.add({ severity: 'success', summary: '密钥已保存', life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '保存失败',
      detail: e instanceof ApiError ? e.message : '保存密钥失败，请稍后再试',
      life: 4000,
    })
  } finally {
    keySaving.value = false
  }
}

async function copyGeneratedKey() {
  if (!generatedKey.value) return
  try {
    await copyText(generatedKey.value)
    toast.add({ severity: 'success', summary: '已复制', life: 2000 })
  } catch {
    toast.add({ severity: 'error', summary: '复制失败', detail: '请手动选择文本', life: 3000 })
  }
}

function dismissGeneratedKey() {
  generatedKey.value = ''
}

async function loadModels() {
  modelsLoading.value = true
  modelsError.value = ''
  try {
    models.value = await listAdminAIPlatformModels()
  } catch (e) {
    modelsError.value = e instanceof ApiError ? e.message : '加载平台模型失败，请稍后再试'
  } finally {
    modelsLoading.value = false
  }
}

async function saveSelection() {
  selectionSaving.value = true
  try {
    applySelection(await selectAdminAIPlatformModel(selectionDraft.value))
    toast.add({ severity: 'success', summary: '当前模型已更新', life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '保存失败',
      detail: e instanceof ApiError ? e.message : '保存当前模型失败，请稍后再试',
      life: 4000,
    })
  } finally {
    selectionSaving.value = false
  }
}

function openCreateModel() {
  if (!canEditModels.value) return
  modelDialogMode.value = 'create'
  editingModelId.value = null
  editingModel.value = null
  modelForm.value = emptyModelForm()
  modelFormErrors.value = { name: '', baseUrl: '', modelName: '', apiKey: '' }
  modelDialogVisible.value = true
}

function openEditModel(model: PlatformModelView) {
  if (!canEditModels.value) return
  modelDialogMode.value = 'edit'
  editingModelId.value = model.id
  editingModel.value = model
  modelForm.value = {
    name: model.name,
    baseUrl: model.baseUrl,
    modelName: model.modelName,
    apiKey: '',
    enabled: model.enabled,
  }
  modelFormErrors.value = { name: '', baseUrl: '', modelName: '', apiKey: '' }
  modelDialogVisible.value = true
}

/** Client-side mirror of the server rules; the server remains authoritative. */
function validateModelForm(): boolean {
  const form = modelForm.value
  const errors = { name: '', baseUrl: '', modelName: '', apiKey: '' }
  if (form.name.trim() === '') errors.name = '请填写模型名称'
  const baseUrl = form.baseUrl.trim()
  if (baseUrl === '') errors.baseUrl = '请填写接口地址'
  else if (!/^https?:\/\//i.test(baseUrl))
    errors.baseUrl = '接口地址必须以 http:// 或 https:// 开头'
  else if (/\/chat\/completions\/?$/i.test(baseUrl)) {
    errors.baseUrl = '接口地址不能以 /chat/completions 结尾'
  }
  if (form.modelName.trim() === '') errors.modelName = '请填写模型名'
  if (!isEditingModel.value && form.apiKey.trim() === '') errors.apiKey = '请填写 API Key'
  modelFormErrors.value = errors
  return !errors.name && !errors.baseUrl && !errors.modelName && !errors.apiKey
}

async function saveModel() {
  if (modelSaving.value || !validateModelForm()) return
  modelSaving.value = true
  try {
    const input: PlatformModelInput = {
      name: modelForm.value.name.trim(),
      baseUrl: modelForm.value.baseUrl.trim(),
      modelName: modelForm.value.modelName.trim(),
      enabled: modelForm.value.enabled,
    }
    const token = modelForm.value.apiKey.trim()
    if (token !== '') input.apiKey = token

    if (isEditingModel.value && editingModelId.value !== null) {
      await updateAdminAIPlatformModel(editingModelId.value, input)
      toast.add({ severity: 'success', summary: '模型已保存', life: 3000 })
    } else {
      await createAdminAIPlatformModel(input)
      toast.add({ severity: 'success', summary: '模型已创建', life: 3000 })
    }
    modelDialogVisible.value = false
    await loadModels()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '保存失败',
      detail: e instanceof ApiError ? e.message : '保存平台模型失败，请稍后再试',
      life: 4000,
    })
  } finally {
    modelSaving.value = false
  }
}

async function testModel(model: PlatformModelView) {
  if (testingModelId.value !== null) return
  testingModelId.value = model.id
  try {
    const result = await testAdminAIPlatformModel(model.id)
    toast.add({
      severity: result.ok ? 'success' : 'error',
      summary: result.ok ? '测试通过' : '测试失败',
      detail: `${result.message}（${result.latencyMs} ms）`,
      life: 4000,
    })
    await loadModels()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '测试失败',
      detail: e instanceof ApiError ? e.message : '测试平台模型失败，请稍后再试',
      life: 4000,
    })
  } finally {
    testingModelId.value = null
  }
}

function confirmDeleteModel(model: PlatformModelView) {
  if (model.id === selectedModelId.value) {
    toast.add({
      severity: 'warn',
      summary: '请先切换当前模型',
      detail: '当前使用的模型不能删除',
      life: 3000,
    })
    return
  }
  confirm.require({
    header: '删除平台模型',
    message: `确定删除平台模型「${model.name}」吗？此操作不可撤销。`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => void doDeleteModel(model),
  })
}

async function doDeleteModel(model: PlatformModelView) {
  try {
    await deleteAdminAIPlatformModel(model.id)
    toast.add({ severity: 'success', summary: '模型已删除', life: 3000 })
    await loadModels()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '删除失败',
      detail: e instanceof ApiError ? e.message : '删除平台模型失败，请稍后再试',
      life: 4000,
    })
  }
}

onMounted(() => {
  void load()
  void loadKey()
  void loadModels()
})
</script>

<template>
  <div class="admin-ai">
    <AppPageHeader
      eyebrow="ADMIN // AI"
      title="AI 设置"
      description="控制平台 AI 功能的启用与关闭；关闭后所有 AI 接口返回 404，用户侧显示“AI 功能未启用”。下方可自动生成或手动填写 AI 加密密钥。"
    />

    <AppErrorState v-if="error" :description="error" @retry="load" />

    <AppLoadingState v-else-if="loading && !settings" label="正在加载 AI 设置" />

    <template v-else>
      <AppSurface padding="lg" class="admin-ai__surface">
        <div class="status-row" aria-label="AI 当前状态">
          <span class="status-row__label">当前状态</span>
          <Tag :value="statusLabel" :severity="statusSeverity" />
        </div>

        <Message v-if="!keyConfigured" severity="warn" :closable="false" role="status">
          未配置 AI 加密密钥，无法启用；关闭仍可保存。
        </Message>

        <form class="admin-ai__form" @submit.prevent="save">
          <AppFormField label="启用 AI">
            <div class="toggle-row">
              <ToggleSwitch v-model="enabled" input-id="ai-enabled" />
              <label for="ai-enabled">{{ enabled ? '已启用' : '已关闭' }}</label>
            </div>
          </AppFormField>

          <div v-if="canManage" class="admin-ai__actions">
            <Button
              type="submit"
              label="保存设置"
              icon="pi pi-check"
              :loading="saving"
              :disabled="disableSave"
            />
          </div>
        </form>
      </AppSurface>

      <AppSurface padding="lg" class="admin-ai__surface">
        <div class="key-header">
          <h2 class="key-title">平台模型</h2>
          <Button
            v-if="canManage"
            label="新增模型"
            icon="pi pi-plus"
            size="small"
            data-test="add-platform-model"
            :disabled="!canEditModels"
            @click="openCreateModel"
          />
        </div>

        <Message v-if="!keyConfigured" severity="warn" :closable="false" role="status">
          未配置 AI 加密密钥，无法新增或编辑平台模型。请先在下方配置加密密钥。
        </Message>

        <p class="platform-hint">
          平台模型供系统任务使用；Token 只保存在服务端，列表与测试结果均不显示明文。
        </p>

        <Message
          v-if="selectedModelDisabled"
          severity="warn"
          :closable="false"
          role="status"
          data-test="selected-model-disabled"
        >
          当前选择的平台模型已停用，系统任务不会使用它。请启用该模型或重新选择。
        </Message>

        <AppFormField v-slot="{ id }" label="当前使用模型">
          <div class="platform-select__row">
            <Select
              :model-value="selectionDraft"
              :input-id="id"
              :options="modelOptions"
              option-label="label"
              option-value="value"
              :disabled="!canManage"
              fluid
              @update:model-value="selectionDraft = $event"
            />
            <Button
              v-if="canManage"
              label="保存选择"
              icon="pi pi-check"
              data-test="save-platform-selection"
              :loading="selectionSaving"
              @click="saveSelection"
            />
          </div>
        </AppFormField>

        <AppErrorState v-if="modelsError" :description="modelsError" @retry="loadModels" />

        <AppLoadingState
          v-else-if="modelsLoading && models.length === 0"
          label="正在加载平台模型"
        />

        <DataTable
          v-else
          :value="models"
          :loading="modelsLoading"
          data-key="id"
          size="small"
          responsive-layout="scroll"
          class="platform-table"
        >
          <Column header="名称" style="min-width: 150px">
            <template #body="{ data }">
              <div class="model-name">
                <span>{{ data.name }}</span>
                <Tag v-if="data.id === selectedModelId" value="当前使用" severity="info" />
              </div>
            </template>
          </Column>
          <Column header="接口地址" style="min-width: 200px">
            <template #body="{ data }">
              <code class="mono">{{ data.baseUrl }}</code>
            </template>
          </Column>
          <Column header="模型" style="min-width: 130px">
            <template #body="{ data }">
              <code class="mono">{{ data.modelName }}</code>
            </template>
          </Column>
          <Column header="状态" style="width: 90px">
            <template #body="{ data }">
              <Tag
                :value="data.enabled ? '已启用' : '已停用'"
                :severity="data.enabled ? 'success' : 'secondary'"
              />
            </template>
          </Column>
          <Column header="最近测试" style="min-width: 170px">
            <template #body="{ data }">
              <div class="test-cell">
                <Tag :value="modelStatusLabel(data)" :severity="modelStatusSeverity(data)" />
                <span v-if="data.lastTestAt" class="muted">
                  {{ formatDateTime(data.lastTestAt) }} · {{ data.lastTestLatencyMs }} ms
                </span>
                <span
                  v-if="data.lastTestError"
                  class="muted test-cell__error"
                  :title="data.lastTestError"
                >
                  {{ data.lastTestError }}
                </span>
              </div>
            </template>
          </Column>
          <Column v-if="canManage" header="操作" style="min-width: 200px">
            <template #body="{ data }">
              <div class="row-actions">
                <Button
                  label="编辑"
                  text
                  size="small"
                  :disabled="!canEditModels"
                  :data-test="`edit-platform-model-${data.id}`"
                  @click="openEditModel(data)"
                />
                <Button
                  label="测试"
                  text
                  size="small"
                  :loading="testingModelId === data.id"
                  :disabled="testingModelId !== null && testingModelId !== data.id"
                  :data-test="`test-platform-model-${data.id}`"
                  @click="testModel(data)"
                />
                <Button
                  label="删除"
                  text
                  size="small"
                  severity="danger"
                  :disabled="data.id === selectedModelId"
                  :data-test="`delete-platform-model-${data.id}`"
                  @click="confirmDeleteModel(data)"
                />
                <span v-if="data.id === selectedModelId" class="row-hint">先切换</span>
              </div>
            </template>
          </Column>
          <template #empty>
            <AppEmptyState
              icon="pi pi-server"
              title="暂无平台模型"
              description="新增平台模型并选择后，系统任务即可使用该模型。"
            />
          </template>
        </DataTable>
      </AppSurface>

      <AppSurface padding="lg" class="admin-ai__surface">
        <div class="key-header">
          <h2 class="key-title">加密密钥</h2>
          <Tag :value="keyStatusLabel" :severity="keyStatusSeverity" />
        </div>

        <AppErrorState v-if="keyError" :description="keyError" @retry="loadKey" />

        <AppLoadingState v-else-if="keyLoading && !key" label="正在加载密钥状态" />

        <template v-else>
          <dl v-if="key?.configured" class="key-meta">
            <div class="key-meta__row">
              <dt>脱敏值</dt>
              <dd>
                <code>{{ key.masked }}</code>
              </dd>
            </div>
            <div class="key-meta__row">
              <dt>指纹</dt>
              <dd>
                <code>{{ key.fingerprint }}</code>
              </dd>
            </div>
          </dl>

          <Message v-if="isEnvSource" severity="info" :closable="false" role="status">
            由 KAPOJ_AI_ENCRYPTION_KEY 管理，后台只读。
          </Message>

          <template v-if="canManage && !isEnvSource">
            <div class="key-actions">
              <Button
                type="button"
                :label="hasFileKey ? '重新生成' : '自动生成密钥'"
                icon="pi pi-refresh"
                :loading="keySaving"
                @click="confirmGenerate"
              />
              <Button
                type="button"
                label="清除旧密钥并生成新密钥"
                icon="pi pi-trash"
                severity="danger"
                outlined
                :loading="keySaving"
                data-test="reset-ai-key"
                @click="confirmResetKey"
              />
            </div>
            <p class="key-actions__hint">
              旧密钥丢失时使用此操作；会删除所有用户已保存的 AI Key，并清空平台模型 Token
              与当前模型选择。
            </p>

            <form class="admin-ai__form" @submit.prevent="confirmSaveManual">
              <AppFormField
                v-slot="{ id }"
                label="手动填写密钥"
                hint="粘贴 base64 编码的 32 字节密钥"
              >
                <Password
                  v-model="manualKey"
                  :input-id="id"
                  fluid
                  toggle-mask
                  :feedback="false"
                  :input-props="{ autocomplete: 'off' }"
                  placeholder="粘贴 base64 32 字节密钥"
                />
              </AppFormField>
              <div class="admin-ai__actions">
                <Button type="submit" label="保存" icon="pi pi-check" :loading="keySaving" />
              </div>
            </form>
          </template>

          <div v-if="generatedKey" class="generated-key">
            <Message severity="warn" :closable="false" role="status">
              请立即备份，丢失后用户 AI Key 无法解密。
            </Message>
            <div class="generated-key__row">
              <InputText :model-value="generatedKey" readonly fluid />
              <Button type="button" label="复制" icon="pi pi-copy" @click="copyGeneratedKey" />
              <Button
                type="button"
                label="隐藏"
                icon="pi pi-times"
                severity="secondary"
                text
                @click="dismissGeneratedKey"
              />
            </div>
          </div>
        </template>
      </AppSurface>

      <Dialog
        v-model:visible="modelDialogVisible"
        :header="isEditingModel ? '编辑平台模型' : '新增平台模型'"
        modal
        :draggable="false"
        :style="{ width: 'min(94vw, 560px)' }"
      >
        <div class="admin-ai__form">
          <AppFormField
            v-slot="field"
            input-id="platform-model-name"
            label="名称"
            :error="modelFormErrors.name"
            required
          >
            <InputText
              :id="field.id"
              v-model="modelForm.name"
              :invalid="field.invalid"
              :aria-describedby="field.describedby"
              maxlength="100"
              placeholder="例如：DeepSeek 官方"
              fluid
              @update:model-value="modelFormErrors.name = ''"
            />
          </AppFormField>

          <AppFormField
            v-slot="field"
            input-id="platform-model-base-url"
            label="接口地址"
            :error="modelFormErrors.baseUrl"
            hint="以 http:// 或 https:// 开头，不要包含 /chat/completions"
            required
          >
            <InputText
              :id="field.id"
              v-model="modelForm.baseUrl"
              :invalid="field.invalid"
              :aria-describedby="field.describedby"
              maxlength="512"
              :placeholder="BASE_URL_PLACEHOLDER"
              fluid
              @update:model-value="modelFormErrors.baseUrl = ''"
            />
          </AppFormField>

          <AppFormField
            v-slot="field"
            input-id="platform-model-model-name"
            label="模型名"
            :error="modelFormErrors.modelName"
            required
          >
            <InputText
              :id="field.id"
              v-model="modelForm.modelName"
              :invalid="field.invalid"
              :aria-describedby="field.describedby"
              maxlength="100"
              placeholder="例如：deepseek-chat"
              fluid
              @update:model-value="modelFormErrors.modelName = ''"
            />
          </AppFormField>

          <AppFormField
            v-slot="field"
            input-id="platform-model-api-key"
            label="API Key"
            :error="modelFormErrors.apiKey"
            :hint="apiKeyHint"
            :required="!isEditingModel"
          >
            <Password
              v-model="modelForm.apiKey"
              :input-id="field.id"
              :invalid="field.invalid"
              :aria-describedby="field.describedby"
              fluid
              toggle-mask
              :feedback="false"
              :input-props="{ autocomplete: 'off' }"
              placeholder="粘贴模型 Token"
              @update:model-value="modelFormErrors.apiKey = ''"
            />
          </AppFormField>

          <AppFormField label="启用">
            <div class="toggle-row">
              <ToggleSwitch v-model="modelForm.enabled" input-id="platform-model-enabled" />
              <label for="platform-model-enabled">
                {{ modelForm.enabled ? '已启用' : '已停用' }}
              </label>
            </div>
          </AppFormField>
        </div>

        <template #footer>
          <Button label="取消" severity="secondary" text @click="modelDialogVisible = false" />
          <Button
            :label="isEditingModel ? '保存' : '创建'"
            icon="pi pi-check"
            data-test="save-platform-model"
            :loading="modelSaving"
            @click="saveModel"
          />
        </template>
      </Dialog>
    </template>
  </div>
</template>

<style scoped>
.admin-ai {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.admin-ai__surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
  min-width: 0;
}

.status-row {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-3) var(--kapoj-space-4);
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 26%, var(--kapoj-border));
  border-radius: var(--kapoj-radius-md);
}

.status-row__label {
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.admin-ai__form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.toggle-row {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  min-height: 40px;
}

.toggle-row label {
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
}

.admin-ai__actions {
  display: flex;
  gap: var(--kapoj-space-2);
}

.key-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
}

.key-title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  font-weight: 600;
}

.key-meta {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  margin: 0;
  padding: var(--kapoj-space-3) var(--kapoj-space-4);
  background: var(--kapoj-bg);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.key-meta__row {
  display: flex;
  align-items: baseline;
  gap: var(--kapoj-space-3);
}

.key-meta__row dt {
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.key-meta__row dd {
  margin: 0;
  overflow-wrap: anywhere;
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.key-actions {
  display: flex;
  gap: var(--kapoj-space-2);
  flex-wrap: wrap;
}

.key-actions__hint {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.platform-hint {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.platform-select__row {
  display: flex;
  gap: var(--kapoj-space-2);
  align-items: center;
}

.platform-select__row :deep(.p-select) {
  flex: 1;
}

.platform-table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.platform-table :deep(.p-datatable-thead > tr > th) {
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

.platform-table :deep(.p-datatable-tbody > tr > td) {
  padding: 10px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.model-name,
.row-actions,
.test-cell {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.row-actions {
  flex-wrap: wrap;
  gap: var(--kapoj-space-1);
}

.test-cell {
  flex-direction: column;
  align-items: flex-start;
  gap: var(--kapoj-space-1);
}

.mono {
  overflow-wrap: anywhere;
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.muted {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.test-cell__error {
  color: var(--kapoj-danger);
}

.row-hint {
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.generated-key {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.generated-key__row {
  display: flex;
  gap: var(--kapoj-space-2);
  align-items: center;
}

.generated-key__row :deep(.p-inputtext) {
  font-family: var(--kapoj-font-mono);
}

@media (max-width: 640px) {
  .admin-ai__actions :deep(.p-button) {
    width: 100%;
  }

  .key-actions :deep(.p-button) {
    width: 100%;
  }

  .platform-select__row {
    flex-direction: column;
    align-items: stretch;
  }

  .platform-select__row :deep(.p-button) {
    width: 100%;
  }

  .generated-key__row {
    flex-wrap: wrap;
  }
}
</style>
