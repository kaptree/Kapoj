<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { useToast } from 'primevue/usetoast'

import { getAdminAISettings, updateAdminAISettings } from '@/api/admin'
import { ApiError } from '@/api/http'
import type { AdminAISettingsView } from '@/api/types'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const toast = useToast()

const canManage = computed(() => auth.can(PERM.ai.settings.manage))

const loading = ref(false)
const error = ref('')
const saving = ref(false)
const settings = ref<AdminAISettingsView | null>(null)
const enabled = ref(false)

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

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="admin-ai">
    <AppPageHeader
      eyebrow="ADMIN // AI"
      title="AI 设置"
      description="控制平台 AI 功能的启用与关闭；关闭后所有 AI 接口返回 404，用户侧显示“AI 功能未启用”。加密密钥仍由部署环境配置。"
    />

    <AppErrorState v-if="error" :description="error" @retry="load" />

    <AppLoadingState v-else-if="loading && !settings" label="正在加载 AI 设置" />

    <AppSurface v-else padding="lg" class="admin-ai__surface">
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

@media (max-width: 640px) {
  .admin-ai__actions :deep(.p-button) {
    width: 100%;
  }
}
</style>
