<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import { useToast } from 'primevue/usetoast'

import { getAdminJudgeSettings, updateAdminJudgeSettings } from '@/api/admin'
import { ApiError } from '@/api/http'
import type { JudgeSettingsInput, JudgeSettingsView } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'

const MIB = 1024 * 1024
const FLOOR_MIN_MIB = 1 / 1024

const auth = useAuthStore()
const toast = useToast()

const canManage = computed(() => auth.can(PERM.system.settings.manage))

const loading = ref(false)
const error = ref('')
const saving = ref(false)
const saveError = ref('')
const settings = ref<JudgeSettingsView | null>(null)

const form = ref({
  outputFloorMiB: null as number | null,
  outputSlackMiB: null as number | null,
  maxOutputMiB: null as number | null,
  maxInputFileMiB: null as number | null,
  maxOutputFileMiB: null as number | null,
  maxBatchTotalMiB: null as number | null,
  maxProblemTotalMiB: null as number | null,
})

const errors = ref({
  outputFloorMiB: '',
  outputSlackMiB: '',
  maxOutputMiB: '',
  maxInputFileMiB: '',
  maxOutputFileMiB: '',
  maxBatchTotalMiB: '',
  maxProblemTotalMiB: '',
})

function bytesToMiB(bytes: number): number {
  return bytes / MIB
}

function mibToBytes(mib: number): number {
  return Math.round(mib * MIB)
}

function applyView(view: JudgeSettingsView) {
  settings.value = view
  form.value.outputFloorMiB = bytesToMiB(view.outputFloorBytes)
  form.value.outputSlackMiB = bytesToMiB(view.outputSlackBytes)
  form.value.maxOutputMiB = bytesToMiB(view.maxOutputBytes)
  form.value.maxInputFileMiB = bytesToMiB(view.maxInputFileBytes)
  form.value.maxOutputFileMiB = bytesToMiB(view.maxOutputFileBytes)
  form.value.maxBatchTotalMiB = bytesToMiB(view.maxBatchTotalBytes)
  form.value.maxProblemTotalMiB = bytesToMiB(view.maxProblemTotalBytes)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    applyView(await getAdminJudgeSettings())
  } catch (e) {
    settings.value = null
    error.value = e instanceof ApiError ? e.message : '加载评测设置失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

function rangeError(
  value: number | null,
  min: number,
  max: number,
  label: string,
  minText: string,
  maxText: string,
): string {
  if (value === null || !Number.isFinite(value) || value < min || value > max) {
    return `${label}必须在 ${minText} 到 ${maxText} 之间`
  }
  return ''
}

function validate(): boolean {
  const values = form.value
  const next = {
    outputFloorMiB: rangeError(
      values.outputFloorMiB,
      FLOOR_MIN_MIB,
      64,
      '输出下限',
      '1 KiB',
      '64 MiB',
    ),
    outputSlackMiB: rangeError(values.outputSlackMiB, 0, 64, '输出余量', '0', '64 MiB'),
    maxOutputMiB: rangeError(values.maxOutputMiB, 1, 64, '最大输出', '1', '64 MiB'),
    maxInputFileMiB: rangeError(values.maxInputFileMiB, 1, 256, '单个输入文件上限', '1', '256 MiB'),
    maxOutputFileMiB: rangeError(values.maxOutputFileMiB, 1, 64, '单个输出文件上限', '1', '64 MiB'),
    maxBatchTotalMiB: rangeError(values.maxBatchTotalMiB, 1, 2048, '批量上传总量', '1', '2048 MiB'),
    maxProblemTotalMiB: rangeError(
      values.maxProblemTotalMiB,
      1,
      2048,
      '单题导入总量',
      '1',
      '2048 MiB',
    ),
  }

  if (!next.maxOutputMiB && !next.outputFloorMiB && values.outputFloorMiB! > values.maxOutputMiB!) {
    next.maxOutputMiB = '最大输出不能小于输出下限'
  }
  if (!next.maxOutputMiB && !next.outputSlackMiB && values.outputSlackMiB! > values.maxOutputMiB!) {
    next.outputSlackMiB = '输出余量不能超过最大输出'
  }
  if (
    !next.maxOutputMiB &&
    !next.maxOutputFileMiB &&
    values.maxOutputFileMiB! > values.maxOutputMiB!
  ) {
    next.maxOutputFileMiB = '单个输出文件上限不能超过最大输出'
  }

  errors.value = next
  return Object.values(next).every((message) => message === '')
}

async function save() {
  if (!canManage.value || !validate()) return
  saving.value = true
  saveError.value = ''
  try {
    const input: JudgeSettingsInput = {
      outputFloorBytes: mibToBytes(form.value.outputFloorMiB as number),
      outputSlackBytes: mibToBytes(form.value.outputSlackMiB as number),
      maxOutputBytes: mibToBytes(form.value.maxOutputMiB as number),
      maxInputFileBytes: mibToBytes(form.value.maxInputFileMiB as number),
      maxOutputFileBytes: mibToBytes(form.value.maxOutputFileMiB as number),
      maxBatchTotalBytes: mibToBytes(form.value.maxBatchTotalMiB as number),
      maxProblemTotalBytes: mibToBytes(form.value.maxProblemTotalMiB as number),
    }
    await updateAdminJudgeSettings(input)
    // Re-read so server-side rounding and clamping are what the form shows.
    applyView(await getAdminJudgeSettings())
    toast.add({ severity: 'success', summary: '评测设置已保存', life: 3000 })
  } catch (e) {
    const message = e instanceof ApiError ? e.message : '保存评测设置失败，请稍后再试'
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
  <div class="admin-judge">
    <AppPageHeader
      eyebrow="ADMIN // JUDGE"
      title="评测设置"
      description="配置评测输出与测试数据的大小上限；保存后对后续提交与上传生效。"
    />

    <AppErrorState v-if="error" :description="error" @retry="load" />

    <AppLoadingState v-else-if="loading && !settings" label="正在加载评测设置" />

    <AppEmptyState
      v-else-if="!settings"
      title="暂无评测设置"
      description="服务器尚未返回评测设置，请稍后重试。"
    >
      <template #actions>
        <Button label="重新加载" icon="pi pi-refresh" severity="secondary" outlined @click="load" />
      </template>
    </AppEmptyState>

    <AppSurface v-else padding="lg" class="admin-judge__surface">
      <Message severity="info" :closable="false" role="status">
        这些限制只影响后续提交与上传；已有数据不会被修改；降低最大输出可能导致已有的大输出被截断。
      </Message>

      <form class="admin-judge__form" @submit.prevent="save">
        <section class="settings-group" aria-label="评测运行">
          <h2 class="settings-group__title">评测运行</h2>
          <div class="settings-grid">
            <AppFormField label="输出下限（MiB）" :error="errors.outputFloorMiB">
              <InputNumber
                v-model="form.outputFloorMiB"
                input-id="output-floor"
                :min="FLOOR_MIN_MIB"
                :max="64"
                :step="0.5"
                :max-fraction-digits="10"
                fluid
              />
            </AppFormField>

            <AppFormField label="输出余量（MiB）" :error="errors.outputSlackMiB">
              <InputNumber
                v-model="form.outputSlackMiB"
                input-id="output-slack"
                :min="0"
                :max="64"
                :step="0.5"
                :max-fraction-digits="10"
                fluid
              />
            </AppFormField>

            <AppFormField label="最大输出（MiB）" :error="errors.maxOutputMiB">
              <InputNumber
                v-model="form.maxOutputMiB"
                input-id="max-output"
                :min="1"
                :max="64"
                :step="0.5"
                :max-fraction-digits="10"
                fluid
              />
            </AppFormField>
          </div>
        </section>

        <section class="settings-group" aria-label="测试数据">
          <h2 class="settings-group__title">测试数据</h2>
          <div class="settings-grid">
            <AppFormField label="单个输入文件（MiB）" :error="errors.maxInputFileMiB">
              <InputNumber
                v-model="form.maxInputFileMiB"
                input-id="max-input-file"
                :min="1"
                :max="256"
                :step="0.5"
                :max-fraction-digits="10"
                fluid
              />
            </AppFormField>

            <AppFormField label="单个输出文件（MiB）" :error="errors.maxOutputFileMiB">
              <InputNumber
                v-model="form.maxOutputFileMiB"
                input-id="max-output-file"
                :min="1"
                :max="64"
                :step="0.5"
                :max-fraction-digits="10"
                fluid
              />
            </AppFormField>

            <AppFormField label="批量上传总量（MiB）" :error="errors.maxBatchTotalMiB">
              <InputNumber
                v-model="form.maxBatchTotalMiB"
                input-id="max-batch-total"
                :min="1"
                :max="2048"
                :step="0.5"
                :max-fraction-digits="10"
                fluid
              />
            </AppFormField>

            <AppFormField label="单题导入总量（MiB）" :error="errors.maxProblemTotalMiB">
              <InputNumber
                v-model="form.maxProblemTotalMiB"
                input-id="max-problem-total"
                :min="1"
                :max="2048"
                :step="0.5"
                :max-fraction-digits="10"
                fluid
              />
            </AppFormField>
          </div>
        </section>

        <div v-if="canManage" class="admin-judge__actions">
          <Message v-if="saveError" severity="error" :closable="false" role="alert">
            {{ saveError }}
          </Message>
          <Button type="submit" label="保存设置" icon="pi pi-check" :loading="saving" />
        </div>
      </form>
    </AppSurface>
  </div>
</template>

<style scoped>
.admin-judge {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.admin-judge__surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
}

.admin-judge__form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
}

.settings-group {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.settings-group__title {
  margin: 0;
  padding-bottom: var(--kapoj-space-2);
  color: var(--kapoj-text);
  font-size: 0.95rem;
  font-weight: 600;
  border-bottom: 1px solid var(--kapoj-border);
}

.settings-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--kapoj-space-3) var(--kapoj-space-4);
}

.admin-judge__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-3);
}

.admin-judge__actions :deep(.p-message) {
  flex: 1 1 320px;
}

@media (max-width: 640px) {
  .settings-grid {
    grid-template-columns: 1fr;
  }

  .admin-judge__actions :deep(.p-button) {
    width: 100%;
  }
}
</style>
