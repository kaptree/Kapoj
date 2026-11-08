<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import { useToast } from 'primevue/usetoast'

import {
  getCertificateImage,
  listCertificateTypes,
  listMyCertificates,
  submitCertificate,
} from '@/api/certificates'
import { ApiError } from '@/api/http'
import type { CertificateStatus, CertificateTypeView, CertificateView } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { formatDateTime } from '@/utils/format'

const MAX_FILE_MB = 5
const MAX_FILE_BYTES = MAX_FILE_MB * 1024 * 1024
const IMAGE_TYPES = ['image/png', 'image/jpeg', 'image/webp']

const STATUS_META: Record<CertificateStatus, { label: string; severity: string }> = {
  pending: { label: '待审核', severity: 'warn' },
  approved: { label: '已通过', severity: 'success' },
  rejected: { label: '已驳回', severity: 'danger' },
}

const toast = useToast()

const loading = ref(true)
const error = ref('')
const types = ref<CertificateTypeView[]>([])
const certs = ref<CertificateView[]>([])

const thumbUrls = ref<Record<number, string>>({})
const objectUrls: string[] = []

const dialogOpen = ref(false)
const typeId = ref<number | null>(null)
const level = ref('')
const file = ref<File | null>(null)
const localPreviewUrl = ref('')
const errors = ref({ typeId: '', level: '', file: '' })
const submitting = ref(false)
const submitError = ref('')

function statusLabel(status: CertificateStatus): string {
  return STATUS_META[status].label
}

function statusSeverity(status: CertificateStatus): string {
  return STATUS_META[status].severity
}

function revokeAllThumbs() {
  for (const url of objectUrls) URL.revokeObjectURL(url)
  objectUrls.length = 0
  thumbUrls.value = {}
}

function revokeLocalPreview() {
  if (localPreviewUrl.value) URL.revokeObjectURL(localPreviewUrl.value)
  localPreviewUrl.value = ''
}

async function loadImage(cert: CertificateView) {
  try {
    const blob = await getCertificateImage(cert.id)
    const url = URL.createObjectURL(blob)
    thumbUrls.value = { ...thumbUrls.value, [cert.id]: url }
    objectUrls.push(url)
  } catch {
    // Failed images fall back to the placeholder rendered when no URL is set.
  }
}

async function load() {
  loading.value = true
  error.value = ''
  revokeAllThumbs()
  try {
    const [typesResult, page] = await Promise.all([
      listCertificateTypes(),
      listMyCertificates(1, 100),
    ])
    types.value = typesResult
    certs.value = page.items
    for (const cert of page.items) void loadImage(cert)
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '加载失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

function openDialog() {
  resetForm()
  dialogOpen.value = true
}

function resetForm() {
  typeId.value = null
  level.value = ''
  file.value = null
  errors.value = { typeId: '', level: '', file: '' }
  submitError.value = ''
  revokeLocalPreview()
}

function onTypeChange(value: number) {
  typeId.value = value
  errors.value.typeId = ''
}

function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const picked = input.files?.[0] ?? null
  errors.value.file = ''
  file.value = null
  revokeLocalPreview()
  if (!picked) return
  if (!IMAGE_TYPES.includes(picked.type)) {
    errors.value.file = '请选择 PNG、JPEG 或 WebP 格式的图片'
    input.value = ''
    return
  }
  if (picked.size > MAX_FILE_BYTES) {
    errors.value.file = `证书图片不能超过 ${MAX_FILE_MB} MiB`
    input.value = ''
    return
  }
  file.value = picked
  localPreviewUrl.value = URL.createObjectURL(picked)
}

function validate(): boolean {
  errors.value.typeId = typeId.value == null ? '请选择证书类型' : ''
  errors.value.level = level.value.trim() === '' ? '请填写等级' : ''
  errors.value.file = file.value ? '' : '请选择证书图片'
  return !errors.value.typeId && !errors.value.level && !errors.value.file
}

async function submit() {
  if (submitting.value) return
  if (!validate()) return

  submitting.value = true
  submitError.value = ''
  try {
    await submitCertificate({ typeId: typeId.value!, level: level.value.trim(), file: file.value! })
    toast.add({
      severity: 'success',
      summary: '提交成功',
      detail: '证书已提交，等待管理员审核',
      life: 3000,
    })
    dialogOpen.value = false
    resetForm()
    await load()
  } catch (e) {
    const message = e instanceof ApiError ? e.message : '提交失败，请稍后再试'
    submitError.value = message
    toast.add({ severity: 'error', summary: '提交失败', detail: message, life: 4000 })
  } finally {
    submitting.value = false
  }
}

async function copyVerifyLink(cert: CertificateView) {
  const url = `${window.location.origin}/verify?code=${cert.verifyCode}`
  await navigator.clipboard.writeText(url)
  toast.add({ severity: 'success', summary: '已复制', detail: '验证链接已复制', life: 3000 })
}

onMounted(() => {
  void load()
})

onUnmounted(() => {
  revokeAllThumbs()
  revokeLocalPreview()
})
</script>

<template>
  <div class="cert-settings-page">
    <AppPageHeader
      eyebrow="ACCOUNT // CERTIFICATE"
      title="我的证书"
      description="提交证书图片并填写等级，管理员审核通过后即可获得唯一验证码；任何人可用验证码公开核验。"
    >
      <template #actions>
        <Button label="申请认证" icon="pi pi-plus" @click="openDialog" />
      </template>
    </AppPageHeader>

    <AppSurface v-if="loading" padding="lg">
      <AppLoadingState label="正在加载证书" />
    </AppSurface>

    <AppSurface v-else-if="error" padding="lg">
      <AppErrorState :description="error" @retry="load" />
    </AppSurface>

    <AppSurface v-else-if="certs.length === 0" padding="lg">
      <AppEmptyState
        icon="pi pi-id-card"
        title="还没有证书"
        description="提交证书图片并选择类型、填写等级，审核通过后即可获得验证码。"
      >
        <template #actions>
          <Button label="申请认证" icon="pi pi-plus" @click="openDialog" />
        </template>
      </AppEmptyState>
    </AppSurface>

    <AppSurface v-else as="section" tone="raised" padding="lg" class="cert-list-surface">
      <DataTable
        :value="certs"
        data-key="id"
        size="small"
        responsive-layout="scroll"
        class="cert-table"
      >
        <Column header="图片" style="width: 64px">
          <template #body="{ data }">
            <img
              v-if="thumbUrls[data.id]"
              :src="thumbUrls[data.id]"
              :alt="`${data.type} 证书`"
              class="cert-thumb"
            />
            <span v-else class="thumb-placeholder" aria-hidden="true"
              ><i class="pi pi-image"
            /></span>
          </template>
        </Column>
        <Column header="类型" style="min-width: 120px">
          <template #body="{ data }"
            ><strong>{{ data.type }}</strong></template
          >
        </Column>
        <Column header="等级" style="width: 100px">
          <template #body="{ data }"
            ><span class="mono">{{ data.level }}</span></template
          >
        </Column>
        <Column header="状态" style="width: 96px">
          <template #body="{ data }">
            <Tag :value="statusLabel(data.status)" :severity="statusSeverity(data.status)" />
          </template>
        </Column>
        <Column header="提交时间" style="min-width: 160px">
          <template #body="{ data }"
            ><span class="muted">{{ formatDateTime(data.submittedAt) }}</span></template
          >
        </Column>
        <Column header="审核时间" style="min-width: 160px">
          <template #body="{ data }">
            <span class="muted">{{ data.reviewedAt ? formatDateTime(data.reviewedAt) : '—' }}</span>
          </template>
        </Column>
        <Column header="备注" style="min-width: 240px">
          <template #body="{ data }">
            <div v-if="data.status === 'rejected' && data.rejectReason" class="reject-reason">
              {{ data.rejectReason }}
            </div>
            <div v-else-if="data.status === 'approved' && data.verifyCode" class="verify-code">
              <span class="mono">{{ data.verifyCode }}</span>
              <Button
                label="复制链接"
                size="small"
                text
                severity="secondary"
                icon="pi pi-copy"
                @click="copyVerifyLink(data)"
              />
            </div>
            <span v-else class="muted">—</span>
          </template>
        </Column>
      </DataTable>
    </AppSurface>

    <Dialog
      v-model:visible="dialogOpen"
      header="申请认证"
      modal
      :draggable="false"
      :style="{ width: 'min(92vw, 440px)' }"
      @hide="resetForm"
    >
      <Message
        v-if="submitError"
        severity="error"
        :closable="false"
        class="form-banner"
        role="alert"
      >
        {{ submitError }}
      </Message>

      <AppFormField
        v-slot="{ id, invalid, describedby }"
        label="证书类型"
        :error="errors.typeId"
        required
      >
        <Select
          :model-value="typeId"
          :input-id="id"
          :options="types"
          option-label="name"
          option-value="id"
          :invalid="invalid"
          :aria-describedby="describedby"
          placeholder="选择证书类型"
          fluid
          @update:model-value="onTypeChange"
        />
      </AppFormField>

      <AppFormField
        v-slot="{ id, invalid, describedby }"
        label="等级"
        :error="errors.level"
        hint="1–50 个字符，例如「省一」「国二」"
        required
      >
        <InputText
          :id="id"
          v-model="level"
          :invalid="invalid"
          :aria-describedby="describedby"
          fluid
          maxlength="50"
          placeholder="填写等级"
          @update:model-value="errors.level = ''"
        />
      </AppFormField>

      <AppFormField
        v-slot="{ id, invalid, describedby }"
        label="证书图片"
        :error="errors.file"
        :hint="`PNG/JPEG/WebP，不超过 ${MAX_FILE_MB} MiB`"
        required
      >
        <input
          :id="id"
          type="file"
          accept="image/png,image/jpeg,image/webp"
          class="file-input"
          :aria-invalid="invalid"
          :aria-describedby="describedby"
          @change="onFileChange"
        />
        <div v-if="localPreviewUrl" class="file-preview">
          <img :src="localPreviewUrl" alt="证书预览" />
        </div>
      </AppFormField>

      <template #footer>
        <Button label="取消" severity="secondary" text @click="dialogOpen = false" />
        <Button label="提交" icon="pi pi-check" :loading="submitting" @click="submit" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.cert-settings-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
  max-width: var(--kapoj-content-max);
}

.cert-list-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.cert-table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.cert-table :deep(.p-datatable-thead > tr > th) {
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

.cert-table :deep(.p-datatable-tbody > tr > td) {
  padding: 10px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.cert-thumb {
  display: block;
  width: 44px;
  height: 44px;
  object-fit: cover;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.thumb-placeholder {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  color: var(--kapoj-text-muted);
  background: var(--kapoj-bg);
  border: 1px dashed var(--kapoj-border-strong);
  border-radius: var(--kapoj-radius-sm);
}

.mono {
  font-family: var(--kapoj-font-mono);
}

.muted {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.reject-reason {
  color: var(--kapoj-danger);
  font-size: 12px;
  line-height: 1.5;
}

.verify-code {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.verify-code .mono {
  color: var(--kapoj-primary);
  font-weight: 600;
}

.form-banner {
  margin-bottom: var(--kapoj-space-4);
}

.file-input {
  width: 100%;
  color: var(--kapoj-text-secondary);
  font-size: 13px;
}

.file-preview {
  margin-top: var(--kapoj-space-2);
}

.file-preview img {
  display: block;
  max-width: 100%;
  max-height: 220px;
  object-fit: contain;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

@media (max-width: 640px) {
  .cert-table :deep(.p-datatable-thead > tr > th),
  .cert-table :deep(.p-datatable-tbody > tr > td) {
    padding: 8px var(--kapoj-space-2);
  }
}
</style>
