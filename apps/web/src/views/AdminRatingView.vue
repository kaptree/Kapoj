<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import ToggleSwitch from 'primevue/toggleswitch'
import { useToast } from 'primevue/usetoast'

import { getAdminRatingSettings, updateAdminRatingSettings } from '@/api/admin'
import { ApiError } from '@/api/http'
import type { RatingSettingsInput, RatingSettingsView } from '@/api/types'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const toast = useToast()

const canManage = computed(() => auth.can(PERM.rating.settings.manage))

const loading = ref(false)
const error = ref('')
const saving = ref(false)
const settings = ref<RatingSettingsView | null>(null)

const form = ref({
  decayEnabled: true,
  decayThreshold: null as number | null,
  decayInactiveDays: null as number | null,
  decayPercent: null as number | null,
  decayMax: null as number | null,
  seasonEnabled: true,
  seasonMonths: null as number | null,
  seasonResetPercent: null as number | null,
  seasonAnchor: '',
})

const errors = ref({
  decayThreshold: '',
  decayInactiveDays: '',
  decayPercent: '',
  decayMax: '',
  seasonMonths: '',
  seasonResetPercent: '',
  seasonAnchor: '',
})

const nextResetDate = computed(() => settings.value?.currentSeason.nextResetAt.slice(0, 10) ?? '')

function applyView(view: RatingSettingsView) {
  settings.value = view
  form.value.decayEnabled = view.decayEnabled
  form.value.decayThreshold = view.decayThreshold
  form.value.decayInactiveDays = view.decayInactiveDays
  form.value.decayPercent = view.decayPercent
  form.value.decayMax = view.decayMax
  form.value.seasonEnabled = view.seasonEnabled
  form.value.seasonMonths = view.seasonMonths
  form.value.seasonResetPercent = view.seasonResetPercent
  form.value.seasonAnchor = view.seasonAnchor
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    applyView(await getAdminRatingSettings())
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '加载 Rating 设置失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

function intError(value: number | null, min: number, max: number, label: string): string {
  if (value === null || !Number.isInteger(value) || value < min || value > max) {
    return `${label}必须在 ${min} 到 ${max} 之间`
  }
  return ''
}

function anchorError(value: string): string {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return '赛季锚点必须是 YYYY-MM-DD 格式'
  const day = Number(value.slice(8, 10))
  if (day !== 1) return '赛季锚点必须为某月 1 日'
  return ''
}

function validate(): boolean {
  errors.value.decayThreshold = intError(form.value.decayThreshold, 1500, 4000, '衰减阈值')
  errors.value.decayInactiveDays = intError(form.value.decayInactiveDays, 1, 365, '未活跃天数')
  errors.value.decayPercent = intError(form.value.decayPercent, 1, 100, '衰减比例')
  errors.value.decayMax = intError(form.value.decayMax, 1, 1000, '单次衰减上限')
  errors.value.seasonMonths = intError(form.value.seasonMonths, 1, 24, '赛季月数')
  errors.value.seasonResetPercent = intError(form.value.seasonResetPercent, 0, 100, '重置比例')
  errors.value.seasonAnchor = anchorError(form.value.seasonAnchor.trim())
  return Object.values(errors.value).every((message) => message === '')
}

async function save() {
  if (!validate()) return
  saving.value = true
  try {
    const input: RatingSettingsInput = {
      decayEnabled: form.value.decayEnabled,
      decayThreshold: form.value.decayThreshold as number,
      decayInactiveDays: form.value.decayInactiveDays as number,
      decayPercent: form.value.decayPercent as number,
      decayMax: form.value.decayMax as number,
      seasonEnabled: form.value.seasonEnabled,
      seasonMonths: form.value.seasonMonths as number,
      seasonResetPercent: form.value.seasonResetPercent as number,
      seasonAnchor: form.value.seasonAnchor.trim(),
    }
    applyView(await updateAdminRatingSettings(input))
    toast.add({ severity: 'success', summary: '设置已保存', life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '保存失败',
      detail: e instanceof ApiError ? e.message : '保存 Rating 设置失败，请稍后再试',
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
  <div class="admin-rating">
    <AppPageHeader
      eyebrow="ADMIN // RATING"
      title="Rating 设置"
      description="配置高分段 Rating 衰减与赛季软重置策略；修改即时生效，不会追溯已发生的衰减或重置。"
    />

    <AppErrorState v-if="error" :description="error" @retry="load" />

    <AppLoadingState v-else-if="loading && !settings" label="正在加载 Rating 设置" />

    <AppSurface v-else padding="lg" class="admin-rating__surface">
      <div class="season-info" aria-label="当前赛季信息">
        <div class="season-info__item">
          <span class="season-info__label">当前赛季</span>
          <strong class="season-info__value">{{ settings?.currentSeason.name }}</strong>
        </div>
        <div class="season-info__item">
          <span class="season-info__label">下次重置</span>
          <strong class="season-info__value">{{ nextResetDate }}</strong>
        </div>
      </div>

      <form class="admin-rating__form" @submit.prevent="save">
        <section class="settings-group" aria-label="衰减设置">
          <h2 class="settings-group__title">衰减设置</h2>
          <div class="settings-grid">
            <AppFormField label="启用衰减">
              <div class="toggle-row">
                <ToggleSwitch v-model="form.decayEnabled" input-id="decay-enabled" />
                <label for="decay-enabled">{{ form.decayEnabled ? '已启用' : '已关闭' }}</label>
              </div>
            </AppFormField>

            <AppFormField label="衰减阈值" :error="errors.decayThreshold">
              <InputNumber
                v-model="form.decayThreshold"
                input-id="decay-threshold"
                :min="1500"
                :max="4000"
                :step="1"
                :min-fraction-digits="0"
                :max-fraction-digits="0"
                fluid
              />
            </AppFormField>

            <AppFormField label="未活跃天数" :error="errors.decayInactiveDays">
              <InputNumber
                v-model="form.decayInactiveDays"
                input-id="decay-inactive-days"
                :min="1"
                :max="365"
                :step="1"
                :min-fraction-digits="0"
                :max-fraction-digits="0"
                fluid
              />
            </AppFormField>

            <AppFormField label="月衰减比例（%）" :error="errors.decayPercent">
              <InputNumber
                v-model="form.decayPercent"
                input-id="decay-percent"
                :min="1"
                :max="100"
                :step="1"
                :min-fraction-digits="0"
                :max-fraction-digits="0"
                fluid
              />
            </AppFormField>

            <AppFormField label="单次衰减上限" :error="errors.decayMax">
              <InputNumber
                v-model="form.decayMax"
                input-id="decay-max"
                :min="1"
                :max="1000"
                :step="1"
                :min-fraction-digits="0"
                :max-fraction-digits="0"
                fluid
              />
            </AppFormField>
          </div>
        </section>

        <section class="settings-group" aria-label="赛季设置">
          <h2 class="settings-group__title">赛季设置</h2>
          <div class="settings-grid">
            <AppFormField label="启用赛季重置">
              <div class="toggle-row">
                <ToggleSwitch v-model="form.seasonEnabled" input-id="season-enabled" />
                <label for="season-enabled">{{ form.seasonEnabled ? '已启用' : '已关闭' }}</label>
              </div>
            </AppFormField>

            <AppFormField label="赛季月数" :error="errors.seasonMonths">
              <InputNumber
                v-model="form.seasonMonths"
                input-id="season-months"
                :min="1"
                :max="24"
                :step="1"
                :min-fraction-digits="0"
                :max-fraction-digits="0"
                fluid
              />
            </AppFormField>

            <AppFormField label="重置比例（%）" :error="errors.seasonResetPercent">
              <InputNumber
                v-model="form.seasonResetPercent"
                input-id="season-reset-percent"
                :min="0"
                :max="100"
                :step="1"
                :min-fraction-digits="0"
                :max-fraction-digits="0"
                fluid
              />
            </AppFormField>

            <AppFormField
              label="赛季锚点"
              hint="YYYY-MM-DD，必须是某月 1 日"
              :error="errors.seasonAnchor"
            >
              <InputText
                id="season-anchor"
                v-model="form.seasonAnchor"
                class="season-anchor-input"
                maxlength="10"
                placeholder="YYYY-MM-DD"
                fluid
              />
            </AppFormField>
          </div>
        </section>

        <div v-if="canManage" class="admin-rating__actions">
          <Button type="submit" label="保存设置" icon="pi pi-check" :loading="saving" />
        </div>
      </form>
    </AppSurface>
  </div>
</template>

<style scoped>
.admin-rating {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.admin-rating__surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
  min-width: 0;
}

.season-info {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-3) var(--kapoj-space-6);
  padding: var(--kapoj-space-3) var(--kapoj-space-4);
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 26%, var(--kapoj-border));
  border-radius: var(--kapoj-radius-md);
}

.season-info__item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 160px;
}

.season-info__label {
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.season-info__value {
  color: var(--kapoj-text);
  font-family: var(--kapoj-font-mono);
  font-size: 1rem;
}

.admin-rating__form {
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

.admin-rating__actions {
  display: flex;
  gap: var(--kapoj-space-2);
}

@media (max-width: 640px) {
  .settings-grid {
    grid-template-columns: 1fr;
  }

  .admin-rating__actions :deep(.p-button) {
    width: 100%;
  }
}
</style>
