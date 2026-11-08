<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import Select from 'primevue/select'
import { useToast } from 'primevue/usetoast'

import { getAIConfig, saveAIConfig } from '@/api/ai'
import { ApiError } from '@/api/http'
import type { AIConfigView } from '@/api/types'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'

const toast = useToast()

const loading = ref(true)
const aiAvailable = ref(true)
const config = ref<AIConfigView | null>(null)

const provider = ref('')
const modelName = ref('')
const apiKey = ref('')
const errors = ref({ provider: '', apiKey: '' })
const saving = ref(false)
const saveError = ref('')

const providers = computed(() => config.value?.providers ?? [])
const configured = computed(() => config.value?.configured === true)
const maskedKey = computed(() => config.value?.apiKeyMasked ?? '')

const currentProvider = computed(
  () => providers.value.find((p) => p.code === provider.value) ?? null,
)

const modelPlaceholder = computed(() => currentProvider.value?.defaultModel ?? '')

function validateAll(): boolean {
  errors.value.provider = provider.value ? '' : '请选择模型服务商'
  errors.value.apiKey =
    !configured.value && apiKey.value.trim() === '' ? '首次使用需填写 API Key' : ''
  return !errors.value.provider && !errors.value.apiKey
}

function applyConfig(view: AIConfigView) {
  config.value = view
  provider.value = view.provider
  modelName.value = view.modelName
  apiKey.value = ''
  saveError.value = ''
}

async function load() {
  loading.value = true
  try {
    const view = await getAIConfig()
    aiAvailable.value = true
    applyConfig(view)
  } catch (e) {
    config.value = null
    if (e instanceof ApiError && e.status === 404) {
      aiAvailable.value = false
    } else {
      aiAvailable.value = true
      saveError.value = e instanceof ApiError ? e.message : '加载配置失败，请稍后再试'
    }
  } finally {
    loading.value = false
  }
}

function onProviderChange(value: string) {
  const previous = providers.value.find((p) => p.code === provider.value)
  // Fill the new provider's default when the model is empty or still on the old
  // default; keep a user-customized model unchanged.
  if (!modelName.value || modelName.value === previous?.defaultModel) {
    modelName.value = providers.value.find((p) => p.code === value)?.defaultModel ?? ''
  }
  provider.value = value
  errors.value.provider = ''
  saveError.value = ''
}

function clearApiKeyError() {
  errors.value.apiKey = ''
  saveError.value = ''
}

async function save() {
  if (!validateAll()) return

  saving.value = true
  saveError.value = ''
  try {
    const input: { provider: string; apiKey?: string; modelName?: string } = {
      provider: provider.value,
    }
    const trimmedKey = apiKey.value.trim()
    if (trimmedKey !== '') input.apiKey = trimmedKey
    if (modelName.value.trim() !== '') input.modelName = modelName.value.trim()

    const view = await saveAIConfig(input)
    applyConfig(view)
    toast.add({
      severity: 'success',
      summary: 'AI 配置已保存',
      detail: 'AI 功能配置已更新',
      life: 3000,
    })
  } catch (e) {
    const message = e instanceof ApiError ? e.message : '保存失败，请稍后再试'
    saveError.value = message
    toast.add({ severity: 'error', summary: '保存失败', detail: message, life: 4000 })
  } finally {
    saving.value = false
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
      description="配置 AI 模型服务的 provider 与 API Key；密钥仅加密存储、不会回显明文，AI 调用使用你自己的 Key。"
    />

    <AppSurface v-if="loading" padding="lg">
      <AppLoadingState label="正在加载 AI 配置" />
    </AppSurface>

    <AppSurface v-else-if="!aiAvailable" padding="lg">
      <Message severity="warn" :closable="false" role="status">
        AI 功能未启用，请联系管理员
      </Message>
    </AppSurface>

    <AppSurface v-else as="section" tone="raised" padding="lg" class="ai-settings-card">
      <Message v-if="saveError" severity="error" :closable="false" class="form-banner" role="alert">
        {{ saveError }}
      </Message>

      <form class="ai-settings-form" novalidate @submit.prevent="save">
        <AppFormField
          v-slot="{ id, invalid, describedby }"
          label="模型服务商"
          :error="errors.provider"
          required
        >
          <Select
            :model-value="provider"
            :input-id="id"
            :options="providers"
            option-label="name"
            option-value="code"
            :invalid="invalid"
            :aria-describedby="describedby"
            placeholder="选择 provider"
            fluid
            @update:model-value="onProviderChange"
          />
        </AppFormField>

        <AppFormField
          v-slot="{ id }"
          label="模型名称"
          :hint="
            modelPlaceholder ? `留空使用默认模型 ${modelPlaceholder}` : '留空使用 provider 默认模型'
          "
          optional
        >
          <InputText
            :id="id"
            v-model="modelName"
            fluid
            :placeholder="modelPlaceholder"
            autocomplete="off"
          />
        </AppFormField>

        <AppFormField
          v-slot="{ id, invalid, describedby }"
          :label="configured ? 'API Key' : 'API Key（首次必填）'"
          :error="errors.apiKey"
          :hint="!configured ? '尚未配置 API Key' : undefined"
          :required="!configured"
        >
          <Password
            v-model="apiKey"
            :input-id="id"
            fluid
            toggle-mask
            :feedback="false"
            :invalid="invalid"
            :input-props="{ autocomplete: 'off', 'aria-describedby': describedby }"
            :placeholder="configured ? '留空保留现有密钥' : '填写你的 API Key'"
            @update:model-value="clearApiKeyError"
          />
          <p v-if="configured && maskedKey" class="masked-key">当前密钥：{{ maskedKey }}</p>
        </AppFormField>

        <Button
          type="submit"
          label="保存配置"
          icon="pi pi-check"
          :loading="saving"
          class="ai-settings-submit"
        />
      </form>
    </AppSurface>
  </div>
</template>

<style scoped>
.ai-settings-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
  max-width: 720px;
}

.ai-settings-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.ai-settings-form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.form-banner {
  margin-bottom: var(--kapoj-space-4);
}

.masked-key {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.ai-settings-submit {
  align-self: flex-start;
  margin-top: var(--kapoj-space-1);
}

@media (max-width: 640px) {
  .ai-settings-submit {
    width: 100%;
  }
}
</style>
