<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import {
  createAIUserModel,
  deleteAIUserModel,
  listAIUserModels,
  setActiveAIUserModel,
  testAIUserModel,
  updateAIUserModel,
} from '@/api/ai'
import { ApiError } from '@/api/http'
import type { AIUserModelInput, AIUserModelView, AIUserModelsView } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { formatDateTime } from '@/utils/format'

type Protocol = AIUserModelView['protocol']

const PROTOCOL_OPTIONS: { label: string; value: Protocol }[] = [
  { label: 'OpenAI 兼容', value: 'openai' },
  { label: 'Anthropic', value: 'anthropic' },
]

const toast = useToast()
const confirm = useConfirm()

const loading = ref(true)
const aiAvailable = ref(true)
const loadError = ref('')
const view = ref<AIUserModelsView | null>(null)

const models = computed(() => view.value?.models ?? [])
const providers = computed(() => view.value?.providers ?? [])
const keyMissing = computed(() => view.value !== null && view.value.keyConfigured === false)
const canManage = computed(() => view.value !== null && view.value.keyConfigured)

interface ModelForm {
  name: string
  protocol: Protocol
  baseUrl: string
  modelName: string
  apiKey: string
  isActive: boolean
  providerCode: string
}

function emptyForm(): ModelForm {
  return {
    name: '',
    protocol: 'openai',
    baseUrl: '',
    modelName: '',
    apiKey: '',
    isActive: false,
    providerCode: '',
  }
}

const dialogVisible = ref(false)
const dialogMode = ref<'create' | 'edit'>('create')
const editingModel = ref<AIUserModelView | null>(null)
const form = ref<ModelForm>(emptyForm())
const formErrors = ref({ name: '', baseUrl: '', modelName: '', apiKey: '' })
const formError = ref('')
const saving = ref(false)
const settingActiveId = ref<number | null>(null)
const testingModelId = ref<number | null>(null)

const isEditing = computed(() => dialogMode.value === 'edit')

/** Kept in script so the `://` literal never appears in the template. */
const BASE_URL_PLACEHOLDER = 'https://api.example.com/v1'

const apiKeyHint = computed(() => {
  if (!isEditing.value) return 'Token 由服务端加密保存，页面不会显示明文'
  const masked = editingModel.value?.apiKeyMasked
  return masked ? `留空则保留原 Token（当前：${masked}）` : '留空则保留原 Token'
})

const baseUrlHint = computed(() =>
  form.value.protocol === 'anthropic'
    ? '以 http:// 或 https:// 开头，填写到域名或 /v1，不要包含 /messages'
    : '以 http:// 或 https:// 开头，不要包含 /chat/completions',
)

function protocolLabel(protocol: Protocol): string {
  return protocol === 'anthropic' ? 'Anthropic' : 'OpenAI'
}

function protocolSeverity(protocol: Protocol): 'secondary' | 'warn' {
  return protocol === 'anthropic' ? 'warn' : 'secondary'
}

function testSeverity(model: AIUserModelView): 'secondary' | 'success' | 'danger' {
  if (!model.lastTestAt) return 'secondary'
  return model.lastTestOk ? 'success' : 'danger'
}

function testLabel(model: AIUserModelView): string {
  if (!model.lastTestAt) return '未测试'
  return model.lastTestOk ? '成功' : '失败'
}

function errorMessage(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

async function load() {
  loading.value = true
  loadError.value = ''
  aiAvailable.value = true
  view.value = null
  try {
    view.value = await listAIUserModels()
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) {
      aiAvailable.value = false
    } else {
      loadError.value = errorMessage(e, '加载 AI 模型失败，请稍后再试')
    }
  } finally {
    loading.value = false
  }
}

/** Refresh after a mutation without replacing the page with an error state. */
async function refreshModels() {
  try {
    view.value = await listAIUserModels()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '刷新失败',
      detail: errorMessage(e, '刷新 AI 模型列表失败，请稍后再试'),
      life: 4000,
    })
  }
}

function requireManage(): boolean {
  if (canManage.value) return true
  toast.add({
    severity: 'warn',
    summary: '无法管理模型',
    detail: '管理员尚未配置 AI 加密密钥',
    life: 3000,
  })
  return false
}

function openCreate() {
  if (!requireManage()) return
  dialogMode.value = 'create'
  editingModel.value = null
  form.value = emptyForm()
  formErrors.value = { name: '', baseUrl: '', modelName: '', apiKey: '' }
  formError.value = ''
  dialogVisible.value = true
}

function openEdit(model: AIUserModelView) {
  if (!requireManage()) return
  dialogMode.value = 'edit'
  editingModel.value = model
  form.value = {
    name: model.name,
    protocol: model.protocol,
    baseUrl: model.baseUrl,
    modelName: model.modelName,
    apiKey: '',
    isActive: model.isActive,
    providerCode: '',
  }
  formErrors.value = { name: '', baseUrl: '', modelName: '', apiKey: '' }
  formError.value = ''
  dialogVisible.value = true
}

/** Presets are OpenAI-compatible providers; fill protocol, base URL and default model. */
function applyProviderPreset(code: string) {
  const preset = providers.value.find((provider) => provider.code === code)
  form.value.providerCode = code
  if (!preset) return
  form.value.protocol = 'openai'
  if (preset.baseUrl) form.value.baseUrl = preset.baseUrl
  if (preset.defaultModel) form.value.modelName = preset.defaultModel
  formErrors.value.baseUrl = ''
  formErrors.value.modelName = ''
  formError.value = ''
}

function onProtocolChange(value: Protocol) {
  form.value.protocol = value
  formErrors.value.baseUrl = ''
  formError.value = ''
}

/** Client-side mirror of the server rules; the server remains authoritative. */
function validateForm(): boolean {
  const errors = { name: '', baseUrl: '', modelName: '', apiKey: '' }
  const name = form.value.name.trim()
  if (name === '') errors.name = '请填写模型名称'
  else if ([...name].length > 100) errors.name = '模型名称长度须为 1-100 个字符'

  const baseUrl = form.value.baseUrl.trim()
  const normalized = baseUrl.replace(/\/+$/, '').toLowerCase()
  if (baseUrl === '') {
    errors.baseUrl = '请填写接口地址'
  } else if (!/^https?:\/\//i.test(baseUrl)) {
    errors.baseUrl = '接口地址必须以 http:// 或 https:// 开头'
  } else if (
    form.value.protocol === 'anthropic' &&
    (normalized.endsWith('/messages') || normalized.endsWith('/v1/messages'))
  ) {
    errors.baseUrl = '接口地址只填到域名或 /v1'
  } else if (form.value.protocol !== 'anthropic' && normalized.endsWith('/chat/completions')) {
    errors.baseUrl = '接口地址不能以 /chat/completions 结尾'
  }

  const modelName = form.value.modelName.trim()
  if (modelName === '') errors.modelName = '请填写模型名'
  else if ([...modelName].length > 100) errors.modelName = '模型名长度须为 1-100 个字符'

  if (!isEditing.value && form.value.apiKey.trim() === '') {
    errors.apiKey = '请填写 API Key'
  }

  formErrors.value = errors
  return !errors.name && !errors.baseUrl && !errors.modelName && !errors.apiKey
}

async function saveModel() {
  if (saving.value || !validateForm()) return
  saving.value = true
  formError.value = ''
  try {
    const input: AIUserModelInput = {
      name: form.value.name.trim(),
      protocol: form.value.protocol,
      baseUrl: form.value.baseUrl.trim().replace(/\/+$/, ''),
      modelName: form.value.modelName.trim(),
    }
    const token = form.value.apiKey.trim()
    if (token !== '') input.apiKey = token
    if (!isEditing.value && form.value.isActive) input.isActive = true

    if (isEditing.value && editingModel.value !== null) {
      await updateAIUserModel(editingModel.value.id, input)
      toast.add({ severity: 'success', summary: '模型已保存', life: 3000 })
    } else {
      await createAIUserModel(input)
      toast.add({ severity: 'success', summary: '模型已创建', life: 3000 })
    }
    dialogVisible.value = false
    await refreshModels()
  } catch (e) {
    const message = errorMessage(e, '保存模型失败，请稍后再试')
    formError.value = message
    toast.add({ severity: 'error', summary: '保存失败', detail: message, life: 4000 })
  } finally {
    saving.value = false
  }
}

async function setActive(model: AIUserModelView) {
  if (model.isActive || settingActiveId.value !== null) return
  settingActiveId.value = model.id
  try {
    await setActiveAIUserModel(model.id)
    toast.add({
      severity: 'success',
      summary: '已设为当前模型',
      detail: model.name,
      life: 3000,
    })
    await refreshModels()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '切换失败',
      detail: errorMessage(e, '切换当前模型失败，请稍后再试'),
      life: 4000,
    })
  } finally {
    settingActiveId.value = null
  }
}

async function testModel(model: AIUserModelView) {
  if (testingModelId.value !== null) return
  testingModelId.value = model.id
  try {
    const result = await testAIUserModel(model.id)
    toast.add({
      severity: result.ok ? 'success' : 'error',
      summary: result.ok ? '测试通过' : '测试失败',
      detail: `${result.message}（${result.latencyMs} ms）`,
      life: 4000,
    })
    await refreshModels()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '测试失败',
      detail: errorMessage(e, '测试 AI 模型失败，请稍后再试'),
      life: 4000,
    })
  } finally {
    testingModelId.value = null
  }
}

function confirmDelete(model: AIUserModelView) {
  if (model.isActive) {
    toast.add({
      severity: 'warn',
      summary: '请先切换当前模型',
      detail: '当前使用的模型不能删除',
      life: 3000,
    })
    return
  }
  confirm.require({
    header: '删除 AI 模型',
    message: `确定删除模型「${model.name}」吗？此操作不可撤销。`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => void doDelete(model),
  })
}

async function doDelete(model: AIUserModelView) {
  try {
    await deleteAIUserModel(model.id)
    toast.add({ severity: 'success', summary: '模型已删除', life: 3000 })
    await refreshModels()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '删除失败',
      detail: errorMessage(e, '删除 AI 模型失败，请稍后再试'),
      life: 4000,
    })
  }
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="ai-settings-page">
    <AppPageHeader
      eyebrow="ACCOUNT // AI"
      title="AI 设置"
      description="管理你自己的 AI 模型：每个模型可以独立选择 OpenAI 兼容或 Anthropic 协议、接口地址与 API Key。Token 仅加密存储、不会回显明文。"
    >
      <template #actions>
        <Button
          v-if="canManage && !loading"
          label="新增模型"
          icon="pi pi-plus"
          data-test="add-user-model"
          @click="openCreate"
        />
      </template>
    </AppPageHeader>

    <AppSurface v-if="loading" padding="lg">
      <AppLoadingState label="正在加载 AI 模型" />
    </AppSurface>

    <AppSurface v-else-if="!aiAvailable" padding="lg">
      <Message severity="warn" :closable="false" role="status">
        AI 功能未启用，请联系管理员
      </Message>
    </AppSurface>

    <AppSurface v-else-if="keyMissing" padding="lg">
      <Message severity="warn" :closable="false" role="status">
        管理员尚未配置 AI 加密密钥，请联系管理员
      </Message>
    </AppSurface>

    <AppErrorState v-else-if="loadError" :description="loadError" @retry="load" />

    <template v-else>
      <AppSurface as="section" tone="raised" padding="lg" class="ai-settings-card">
        <p class="models-hint">
          当前模型用于代码补全、格式化、优化与提示；切换后立即生效。列表只显示脱敏 Token。
        </p>

        <DataTable
          :value="models"
          data-key="id"
          size="small"
          responsive-layout="scroll"
          class="model-table"
        >
          <Column header="名称" style="min-width: 150px">
            <template #body="{ data }">
              <div class="model-name">
                <span>{{ data.name }}</span>
                <Tag v-if="data.isActive" value="当前" severity="info" />
              </div>
            </template>
          </Column>
          <Column header="协议" style="width: 120px">
            <template #body="{ data }">
              <Tag
                :value="protocolLabel(data.protocol)"
                :severity="protocolSeverity(data.protocol)"
              />
            </template>
          </Column>
          <Column header="接口地址" style="min-width: 200px">
            <template #body="{ data }">
              <code class="mono">{{ data.baseUrl }}</code>
            </template>
          </Column>
          <Column header="模型" style="min-width: 140px">
            <template #body="{ data }">
              <code class="mono">{{ data.modelName }}</code>
            </template>
          </Column>
          <Column header="Token" style="min-width: 130px">
            <template #body="{ data }">
              <code class="mono">{{ data.apiKeyMasked || '未保存' }}</code>
            </template>
          </Column>
          <Column header="最近测试" style="min-width: 170px">
            <template #body="{ data }">
              <div class="test-cell">
                <Tag :value="testLabel(data)" :severity="testSeverity(data)" />
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
          <Column header="操作" style="min-width: 290px">
            <template #body="{ data }">
              <div class="row-actions">
                <Button
                  label="设为当前"
                  text
                  size="small"
                  :loading="settingActiveId === data.id"
                  :disabled="data.isActive || settingActiveId !== null"
                  :data-test="`set-active-model-${data.id}`"
                  @click="setActive(data)"
                />
                <Button
                  label="编辑"
                  text
                  size="small"
                  :data-test="`edit-user-model-${data.id}`"
                  @click="openEdit(data)"
                />
                <Button
                  label="测试"
                  text
                  size="small"
                  :loading="testingModelId === data.id"
                  :disabled="testingModelId !== null && testingModelId !== data.id"
                  :data-test="`test-user-model-${data.id}`"
                  @click="testModel(data)"
                />
                <Button
                  label="删除"
                  text
                  size="small"
                  severity="danger"
                  :disabled="data.isActive"
                  :data-test="`delete-user-model-${data.id}`"
                  @click="confirmDelete(data)"
                />
                <span v-if="data.isActive" class="row-hint">先切换</span>
              </div>
            </template>
          </Column>
          <template #empty>
            <AppEmptyState
              icon="pi pi-sparkles"
              title="暂无 AI 模型"
              description="新增模型并填写 API Key 后即可使用 AI 功能；第一个模型会自动成为当前模型。"
            >
              <template #actions>
                <Button
                  label="新增模型"
                  icon="pi pi-plus"
                  data-test="add-user-model-empty"
                  @click="openCreate"
                />
              </template>
            </AppEmptyState>
          </template>
        </DataTable>
      </AppSurface>

      <Dialog
        v-model:visible="dialogVisible"
        :header="isEditing ? '编辑模型' : '新增模型'"
        modal
        :draggable="false"
        :style="{ width: 'min(94vw, 560px)' }"
      >
        <div class="model-form">
          <Message v-if="formError" severity="error" :closable="false" role="alert">
            {{ formError }}
          </Message>

          <AppFormField
            v-if="!isEditing"
            v-slot="field"
            input-id="user-model-provider"
            label="服务商预填"
            hint="选择内置服务商可自动填写接口地址与默认模型"
            optional
          >
            <Select
              :model-value="form.providerCode"
              :input-id="field.id"
              :options="providers"
              option-label="name"
              option-value="code"
              placeholder="选择服务商（可选）"
              fluid
              @update:model-value="applyProviderPreset"
            />
          </AppFormField>

          <AppFormField
            v-slot="field"
            input-id="user-model-name"
            label="名称"
            :error="formErrors.name"
            required
          >
            <InputText
              :id="field.id"
              v-model="form.name"
              :invalid="field.invalid"
              :aria-describedby="field.describedby"
              maxlength="100"
              placeholder="例如：Anthropic 主力"
              fluid
              @update:model-value="formErrors.name = ''"
            />
          </AppFormField>

          <AppFormField v-slot="field" input-id="user-model-protocol" label="协议" required>
            <Select
              :model-value="form.protocol"
              :input-id="field.id"
              :options="PROTOCOL_OPTIONS"
              option-label="label"
              option-value="value"
              fluid
              @update:model-value="onProtocolChange"
            />
          </AppFormField>

          <AppFormField
            v-slot="field"
            input-id="user-model-base-url"
            label="接口地址"
            :error="formErrors.baseUrl"
            :hint="baseUrlHint"
            required
          >
            <InputText
              :id="field.id"
              v-model="form.baseUrl"
              :invalid="field.invalid"
              :aria-describedby="field.describedby"
              maxlength="512"
              :placeholder="BASE_URL_PLACEHOLDER"
              fluid
              @update:model-value="formErrors.baseUrl = ''"
            />
          </AppFormField>

          <AppFormField
            v-slot="field"
            input-id="user-model-model-name"
            label="模型名"
            :error="formErrors.modelName"
            required
          >
            <InputText
              :id="field.id"
              v-model="form.modelName"
              :invalid="field.invalid"
              :aria-describedby="field.describedby"
              maxlength="100"
              placeholder="例如：claude-sonnet-4-5"
              fluid
              @update:model-value="formErrors.modelName = ''"
            />
          </AppFormField>

          <AppFormField
            v-slot="field"
            input-id="user-model-api-key"
            label="API Key"
            :error="formErrors.apiKey"
            :hint="apiKeyHint"
            :required="!isEditing"
          >
            <Password
              v-model="form.apiKey"
              :input-id="field.id"
              :invalid="field.invalid"
              :aria-describedby="field.describedby"
              fluid
              toggle-mask
              :feedback="false"
              :input-props="{ autocomplete: 'off' }"
              :placeholder="isEditing ? '留空保留现有 Token' : '填写你的 API Key'"
              @update:model-value="formErrors.apiKey = ''"
            />
          </AppFormField>

          <AppFormField v-if="!isEditing" label="设为当前模型">
            <div class="toggle-row">
              <Checkbox v-model="form.isActive" binary input-id="user-model-active" />
              <label for="user-model-active">
                {{ form.isActive ? '创建后立即使用该模型' : '创建后不切换当前模型' }}
              </label>
            </div>
          </AppFormField>
        </div>

        <template #footer>
          <Button label="取消" severity="secondary" text @click="dialogVisible = false" />
          <Button
            :label="isEditing ? '保存' : '创建'"
            icon="pi pi-check"
            data-test="save-user-model"
            :loading="saving"
            @click="saveModel"
          />
        </template>
      </Dialog>
    </template>
  </div>
</template>

<style scoped>
.ai-settings-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
  max-width: 1080px;
}

.ai-settings-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
}

.models-hint {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.model-table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.model-table :deep(.p-datatable-thead > tr > th) {
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

.model-table :deep(.p-datatable-tbody > tr > td) {
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

.model-form {
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
</style>
