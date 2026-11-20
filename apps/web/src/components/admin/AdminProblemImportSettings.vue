<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import { useToast } from 'primevue/usetoast'

import { getAdminProblemImportSettings, updateAdminProblemImportSettings } from '@/api/admin'
import { ApiError } from '@/api/http'
import type { ProblemImportSettings } from '@/api/types'
import AppFormField from '@/components/ui/AppFormField.vue'

const toast = useToast()
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const settings = ref<ProblemImportSettings | null>(null)
const maxPackageMb = ref(200)

const uncompressedText = computed(() => {
  const bytes = settings.value?.maxUncompressedBytes ?? 0
  if (bytes <= 0) return ''
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2).replace(/\.00$/, '')} GB`
})

function message(reason: unknown, fallback: string): string {
  return reason instanceof ApiError ? reason.message : fallback
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    settings.value = await getAdminProblemImportSettings()
    maxPackageMb.value = settings.value.maxPackageMb
  } catch (reason) {
    error.value = message(reason, '加载题包导入设置失败，请稍后再试')
  } finally {
    loading.value = false
  }
}

async function save(): Promise<void> {
  if (saving.value) return
  const value = maxPackageMb.value
  if (!Number.isInteger(value) || value < 1 || value > 2048) {
    error.value = '题包大小上限需为 1–2048 MB'
    return
  }
  saving.value = true
  error.value = ''
  try {
    settings.value = await updateAdminProblemImportSettings({ maxPackageMb: value })
    maxPackageMb.value = settings.value.maxPackageMb
    toast.add({ severity: 'success', summary: '题包导入设置已保存', life: 3000 })
  } catch (reason) {
    error.value = message(reason, '保存题包导入设置失败，请稍后再试')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <form class="problem-import-settings" data-test="problem-import-settings" @submit.prevent="save">
    <Message severity="info" :closable="false">
      改变上限只影响后续导入；已有题包和测试数据不受影响。解压上限会自动取 1 GiB 与 2
      倍包上限中的较大值。
    </Message>

    <p
      v-if="error"
      data-test="problem-import-error"
      class="problem-import-settings__error"
      role="alert"
    >
      {{ error }}
    </p>

    <template v-if="loading">
      <p class="problem-import-settings__hint">正在加载题包导入设置…</p>
    </template>
    <template v-else>
      <AppFormField
        v-slot="{ id }"
        label="压缩包大小上限"
        hint="默认 200 MB；可设置 1–2048 MB，最大支持 2 GB。"
      >
        <InputNumber
          :id="id"
          v-model="maxPackageMb"
          input-id="problem-import-max-mb"
          data-test="problem-import-max-mb"
          :min="1"
          :max="2048"
          suffix=" MB"
          show-buttons
          fluid
        />
      </AppFormField>

      <p class="problem-import-settings__hint">
        当前解压上限约 <strong>{{ uncompressedText || '1 GB' }}</strong
        >。 超大包需要服务器临时目录至少预留 2 GB 空间，并建议在低峰期导入。
      </p>

      <div class="problem-import-settings__actions">
        <Button type="submit" label="保存题包导入设置" :loading="saving" />
      </div>
    </template>
  </form>
</template>

<style scoped>
.problem-import-settings {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  max-width: 620px;
}

.problem-import-settings__hint {
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.84rem;
  line-height: 1.6;
}

.problem-import-settings__error {
  margin: 0;
  color: var(--kapoj-kapo-text);
  font-size: 12px;
}

.problem-import-settings__actions {
  display: flex;
  justify-content: flex-end;
}
</style>
