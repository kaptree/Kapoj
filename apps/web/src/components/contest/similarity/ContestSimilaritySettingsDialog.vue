<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import Select from 'primevue/select'
import Textarea from 'primevue/textarea'

import type { ContestSimilarityProblem, ContestSimilaritySettingsInput } from '@/api/types'
import { SUBMISSION_LANGUAGES } from '@/utils/submission'

const MAX_BASE_CODE = 65536

const props = defineProps<{
  visible: boolean
  problem: ContestSimilarityProblem | null
  saving: boolean
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  save: [payload: ContestSimilaritySettingsInput]
}>()

const dialogVisible = computed({
  get: () => props.visible,
  set: (value: boolean) => emit('update:visible', value),
})

const threshold = ref<number | null>(70)
const ignoreCommonCode = ref(false)
const baseLanguage = ref('')
const baseCode = ref('')
const error = ref('')

const languageOptions = computed(() => [
  { value: '', label: '不设置（不使用公共代码）' },
  ...SUBMISSION_LANGUAGES.map((item) => ({ value: item.value, label: item.label })),
])

watch(
  [() => props.visible, () => props.problem],
  ([visible]) => {
    if (!visible) return
    threshold.value = props.problem?.thresholdPercent ?? 70
    ignoreCommonCode.value = props.problem?.ignoreCommonCode ?? false
    baseLanguage.value = props.problem?.baseLanguage ?? ''
    baseCode.value = props.problem?.baseCode ?? ''
    error.value = ''
  },
  { immediate: true },
)

watch([threshold, ignoreCommonCode, baseLanguage, baseCode], () => {
  if (error.value) error.value = ''
})

function submit() {
  if (props.saving) return
  const value = threshold.value
  if (value === null || !Number.isInteger(value) || value < 1 || value > 100) {
    error.value = '阈值需为 1-100 的整数'
    return
  }
  if (baseCode.value.trim() !== '' && baseLanguage.value === '') {
    error.value = '填写公共代码后请选择语言'
    return
  }
  error.value = ''
  emit('save', {
    thresholdPercent: value,
    ignoreCommonCode: ignoreCommonCode.value,
    baseLanguage: baseLanguage.value,
    baseCode: baseCode.value,
  })
}
</script>

<template>
  <Dialog
    v-model:visible="dialogVisible"
    modal
    header="查重设置"
    :style="{ width: 'min(94vw, 640px)' }"
    :draggable="false"
    class="similarity-settings-dialog"
  >
    <div class="similarity-settings">
      <p v-if="problem" class="similarity-settings__problem">
        <span class="similarity-settings__id">{{ problem.displayId }}</span>
        {{ problem.title }}
      </p>

      <div class="field">
        <label for="similarity-threshold">相似度阈值（%）</label>
        <InputNumber
          v-model="threshold"
          input-id="similarity-threshold"
          data-test="similarity-settings-threshold"
          :min="1"
          :max="100"
          :step="1"
          fluid
        />
        <span class="hint">重复率达到该阈值的代码对会被标记，范围为 1-100。</span>
      </div>

      <div class="field field--inline">
        <Checkbox
          v-model="ignoreCommonCode"
          input-id="similarity-ignore-common"
          data-test="similarity-settings-ignore"
          binary
        />
        <div class="field__text">
          <label for="similarity-ignore-common">忽略公共代码</label>
          <span class="hint">移除赛题中常见的模板与固定写法，降低误报。</span>
        </div>
      </div>

      <div class="field">
        <label for="similarity-base-language">公共代码语言</label>
        <Select
          v-model="baseLanguage"
          input-id="similarity-base-language"
          data-test="similarity-settings-language"
          :options="languageOptions"
          option-label="label"
          option-value="value"
          placeholder="不设置"
          fluid
        />
      </div>

      <div class="field">
        <label for="similarity-base-code">公共代码</label>
        <Textarea
          id="similarity-base-code"
          v-model="baseCode"
          data-test="similarity-settings-base-code"
          rows="6"
          :maxlength="MAX_BASE_CODE"
          placeholder="可选：粘贴全站或本赛题通用的代码模板"
          fluid
        />
        <span class="hint">
          填写公共代码后必须选择语言，最多 {{ MAX_BASE_CODE }} 个字符；留空保存会清除已有公共代码。
        </span>
      </div>

      <p v-if="error" class="error" data-test="similarity-settings-error">{{ error }}</p>
    </div>

    <template #footer>
      <Button label="取消" severity="secondary" text @click="dialogVisible = false" />
      <Button
        label="保存"
        data-test="similarity-settings-save"
        icon="pi pi-check"
        :loading="saving"
        @click="submit"
      />
    </template>
  </Dialog>
</template>

<style scoped>
.similarity-settings {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.similarity-settings__problem {
  display: flex;
  align-items: baseline;
  gap: var(--kapoj-space-2);
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
}

.similarity-settings__id {
  color: var(--kapoj-primary);
  font-family: var(--kapoj-font-mono);
  font-size: 0.8125rem;
  font-weight: 600;
}

.field {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
  min-width: 0;
}

.field--inline {
  flex-direction: row;
  align-items: flex-start;
  gap: var(--kapoj-space-2);
}

.field__text {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
}

.field label {
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
  font-weight: 600;
}

.hint {
  color: var(--kapoj-text-muted);
  font-size: 0.75rem;
  line-height: 1.5;
}

.error {
  margin: 0;
  color: var(--kapoj-danger);
  font-size: 0.8125rem;
}
</style>
