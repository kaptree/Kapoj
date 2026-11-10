<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import {
  generateAdminAIKey,
  getAdminAIKey,
  getAdminAISettings,
  resetAdminAIKey,
  setAdminAIKey,
  updateAdminAISettings,
} from '@/api/admin'
import { ApiError } from '@/api/http'
import type { AdminAISettingsView, AIKeyView } from '@/api/types'
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
  if (isEnvSource.value) return '由环境变量管理'
  if (hasFileKey.value) return '已配置（文件）'
  return '未配置'
})

const keyStatusSeverity = computed<'secondary' | 'success' | 'info' | 'warn' | 'danger'>(() => {
  if (isEnvSource.value) return 'info'
  if (hasFileKey.value) return 'success'
  return 'secondary'
})

function applyView(view: AdminAISettingsView) {
  settings.value = view
  enabled.value = view.enabled
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
      '将删除所有用户已保存的 AI API Key，并生成新的平台加密密钥。此操作不可撤销，用户需要重新填写自己的 API Key。',
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
    toast.add({
      severity: 'success',
      summary: '旧密钥已清除',
      detail: '新密钥已生成；请立即备份，用户需重新填写 AI API Key',
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
    await navigator.clipboard.writeText(generatedKey.value)
    toast.add({ severity: 'success', summary: '已复制', life: 2000 })
  } catch {
    toast.add({ severity: 'error', summary: '复制失败', detail: '请手动选择文本', life: 3000 })
  }
}

function dismissGeneratedKey() {
  generatedKey.value = ''
}

onMounted(() => {
  void load()
  void loadKey()
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
            <p class="key-actions__hint">旧密钥丢失时使用此操作；会删除所有用户已保存的 AI Key。</p>

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

  .generated-key__row {
    flex-wrap: wrap;
  }
}
</style>
