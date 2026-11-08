<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Paginator from 'primevue/paginator'
import Select from 'primevue/select'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import TabPanel from 'primevue/tabpanel'
import TabPanels from 'primevue/tabpanels'
import Tabs from 'primevue/tabs'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import {
  createAdminCertificateType,
  deleteAdminCertificateType,
  listAdminCertificates,
  listAdminCertificateTypes,
  reviewCertificate,
  updateAdminCertificateType,
} from '@/api/admin'
import { getCertificateImage } from '@/api/certificates'
import { ApiError } from '@/api/http'
import type { AdminCertificateView, CertificateStatus, CertificateTypeView } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { formatDateTime } from '@/utils/format'

const PAGE_SIZE = 20

type ReviewStatus = 'pending' | 'approved' | 'rejected' | 'all'

const STATUS_OPTIONS: { value: ReviewStatus; label: string }[] = [
  { value: 'pending', label: '待审核' },
  { value: 'all', label: '全部' },
  { value: 'approved', label: '已通过' },
  { value: 'rejected', label: '已驳回' },
]

const STATUS_META: Record<CertificateStatus, { label: string; severity: string }> = {
  pending: { label: '待审核', severity: 'warn' },
  approved: { label: '已通过', severity: 'success' },
  rejected: { label: '已驳回', severity: 'danger' },
}

const toast = useToast()
const confirm = useConfirm()

const activeTab = ref('review')

function statusLabel(status: CertificateStatus): string {
  return STATUS_META[status].label
}

function statusSeverity(status: CertificateStatus): string {
  return STATUS_META[status].severity
}

// --- Review list ------------------------------------------------------------

const reviewStatus = ref<ReviewStatus>('pending')
const certs = ref<AdminCertificateView[]>([])
const certTotal = ref(0)
const certPage = ref(1)
const certLoading = ref(false)
const certError = ref('')

let latestCerts = 0

async function loadCerts() {
  const seq = ++latestCerts
  certLoading.value = true
  certError.value = ''
  try {
    const result = await listAdminCertificates({
      status: reviewStatus.value,
      page: certPage.value,
      pageSize: PAGE_SIZE,
    })
    if (seq !== latestCerts) return
    certs.value = result.items
    certTotal.value = result.total
  } catch (e) {
    if (seq !== latestCerts) return
    certError.value = e instanceof ApiError ? e.message : '加载证书失败，请稍后再试'
  } finally {
    if (seq === latestCerts) certLoading.value = false
  }
}

function onStatusChange(value: ReviewStatus) {
  reviewStatus.value = value
  certPage.value = 1
  void loadCerts()
}

function onCertPage(event: { page: number }) {
  certPage.value = event.page + 1
  void loadCerts()
}

// --- Review dialog ----------------------------------------------------------

const reviewDialogVisible = ref(false)
const reviewing = ref<AdminCertificateView | null>(null)
const reviewImageUrl = ref('')
const reviewImageLoading = ref(false)
const reviewImageError = ref('')
const rejectReason = ref('')
const rejectError = ref('')
const reviewingAction = ref(false)

function revokeReviewImage() {
  if (reviewImageUrl.value) {
    URL.revokeObjectURL(reviewImageUrl.value)
    reviewImageUrl.value = ''
  }
}

function openReview(item: AdminCertificateView) {
  reviewing.value = item
  rejectReason.value = ''
  rejectError.value = ''
  reviewDialogVisible.value = true
  void loadReviewImage(item.id)
}

async function loadReviewImage(id: number) {
  reviewImageLoading.value = true
  reviewImageError.value = ''
  revokeReviewImage()
  try {
    const blob = await getCertificateImage(id)
    if (reviewing.value?.id !== id) return
    reviewImageUrl.value = URL.createObjectURL(blob)
  } catch {
    if (reviewing.value?.id !== id) return
    reviewImageError.value = '图片加载失败'
  } finally {
    if (reviewing.value?.id === id) reviewImageLoading.value = false
  }
}

async function approve() {
  if (!reviewing.value || reviewingAction.value) return
  reviewingAction.value = true
  try {
    await reviewCertificate(reviewing.value.id, { action: 'approve' })
    toast.add({ severity: 'success', summary: '已通过', life: 3000 })
    reviewDialogVisible.value = false
    await loadCerts()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '操作失败',
      detail: e instanceof ApiError ? e.message : '审核失败，请稍后再试',
      life: 4000,
    })
  } finally {
    reviewingAction.value = false
  }
}

function reject() {
  if (!reviewing.value || reviewingAction.value) return
  const reason = rejectReason.value.trim()
  if (reason.length < 1) {
    rejectError.value = '请填写驳回原因'
    return
  }
  if (reason.length > 500) {
    rejectError.value = '驳回原因不能超过 500 字'
    return
  }
  rejectError.value = ''
  void doReject(reason)
}

async function doReject(reason: string) {
  if (!reviewing.value) return
  reviewingAction.value = true
  try {
    await reviewCertificate(reviewing.value.id, { action: 'reject', reason })
    toast.add({ severity: 'success', summary: '已驳回', life: 3000 })
    reviewDialogVisible.value = false
    await loadCerts()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '操作失败',
      detail: e instanceof ApiError ? e.message : '审核失败，请稍后再试',
      life: 4000,
    })
  } finally {
    reviewingAction.value = false
  }
}

// --- Types ------------------------------------------------------------------

const types = ref<CertificateTypeView[]>([])
const typesLoading = ref(false)
const typesError = ref('')

async function loadTypes() {
  typesLoading.value = true
  typesError.value = ''
  try {
    types.value = await listAdminCertificateTypes(true)
  } catch (e) {
    typesError.value = e instanceof ApiError ? e.message : '加载证书类型失败，请稍后再试'
  } finally {
    typesLoading.value = false
  }
}

// --- Type dialog ------------------------------------------------------------

const typeDialogVisible = ref(false)
const typeSaving = ref(false)
const editingType = ref<CertificateTypeView | null>(null)
const typeForm = ref({ name: '', description: '', isActive: true, sortOrder: 0 })
const typeErrors = ref({ name: '', description: '', sortOrder: '' })
const deletingTypeId = ref<number | null>(null)

function openCreateType() {
  editingType.value = null
  typeForm.value = { name: '', description: '', isActive: true, sortOrder: 0 }
  typeErrors.value = { name: '', description: '', sortOrder: '' }
  typeDialogVisible.value = true
}

function openEditType(item: CertificateTypeView) {
  editingType.value = item
  typeForm.value = {
    name: item.name,
    description: item.description,
    isActive: item.isActive,
    sortOrder: item.sortOrder,
  }
  typeErrors.value = { name: '', description: '', sortOrder: '' }
  typeDialogVisible.value = true
}

function validateType(): boolean {
  const name = typeForm.value.name.trim()
  const description = typeForm.value.description.trim()
  const sortOrder = typeForm.value.sortOrder

  const nameError =
    name.length < 1 ? '请填写类型名称' : name.length > 50 ? '类型名称不能超过 50 字' : ''
  const descError = description.length > 200 ? '类型描述不能超过 200 字' : ''
  const sortError =
    !Number.isInteger(sortOrder) || sortOrder < 0 || sortOrder > 10000
      ? '排序需为 0–10000 的整数'
      : ''

  typeErrors.value = { name: nameError, description: descError, sortOrder: sortError }
  return !nameError && !descError && !sortError
}

async function saveType() {
  if (typeSaving.value) return
  if (!validateType()) return

  const creating = editingType.value === null
  const input = {
    name: typeForm.value.name.trim(),
    description: typeForm.value.description.trim(),
    isActive: typeForm.value.isActive,
    sortOrder: typeForm.value.sortOrder,
  }

  typeSaving.value = true
  try {
    if (creating) await createAdminCertificateType(input)
    else await updateAdminCertificateType(editingType.value!.id, input)
    toast.add({ severity: 'success', summary: creating ? '类型已创建' : '类型已更新', life: 3000 })
    typeDialogVisible.value = false
    await loadTypes()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '操作失败',
      detail: e instanceof ApiError ? e.message : '保存类型失败，请稍后再试',
      life: 4000,
    })
  } finally {
    typeSaving.value = false
  }
}

function confirmDeleteType(item: CertificateTypeView) {
  confirm.require({
    header: '删除类型',
    message: `确定删除证书类型「${item.name}」吗？已有证书使用该类型时无法删除。`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => void doDeleteType(item),
  })
}

async function doDeleteType(item: CertificateTypeView) {
  deletingTypeId.value = item.id
  try {
    await deleteAdminCertificateType(item.id)
    toast.add({ severity: 'success', summary: '类型已删除', life: 3000 })
    await loadTypes()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '操作失败',
      detail: e instanceof ApiError ? e.message : '删除类型失败，请稍后再试',
      life: 4000,
    })
  } finally {
    deletingTypeId.value = null
  }
}

onMounted(() => {
  void loadCerts()
  void loadTypes()
})

onBeforeUnmount(() => {
  revokeReviewImage()
})
</script>

<template>
  <div class="admin-certificates">
    <AppPageHeader
      eyebrow="ADMIN // CERTIFICATE"
      title="证书审核"
      description="审核用户提交的证书认证申请，并维护可用的证书类型。"
    />

    <AppSurface padding="lg" class="admin-certificates__surface">
      <Tabs v-model:value="activeTab" lazy>
        <TabList>
          <Tab value="review">证书审核</Tab>
          <Tab value="types">证书类型</Tab>
        </TabList>
        <TabPanels>
          <TabPanel value="review">
            <div class="review-panel">
              <div class="toolbar">
                <Select
                  :model-value="reviewStatus"
                  :options="STATUS_OPTIONS"
                  option-label="label"
                  option-value="value"
                  aria-label="按状态筛选"
                  class="status-select"
                  @update:model-value="onStatusChange"
                />
                <span class="count">共 {{ certTotal }} 条申请</span>
              </div>

              <AppErrorState
                v-if="certError && certs.length === 0"
                :description="certError"
                @retry="loadCerts"
              />

              <AppLoadingState v-if="certLoading && certs.length === 0" label="正在加载证书" />

              <DataTable
                v-else-if="!certError || certs.length > 0"
                :value="certs"
                :loading="certLoading"
                data-key="id"
                size="small"
                responsive-layout="scroll"
                class="review-table"
              >
                <Column header="用户" style="min-width: 160px">
                  <template #body="{ data }">
                    <div class="identity">
                      <span class="identity__nickname">{{
                        data.user.nickname || data.user.username
                      }}</span>
                      <span class="identity__handle">@{{ data.user.username }}</span>
                    </div>
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
                <Column header="提交时间" style="min-width: 160px">
                  <template #body="{ data }"
                    ><span class="muted">{{ formatDateTime(data.submittedAt) }}</span></template
                  >
                </Column>
                <Column header="状态" style="width: 96px">
                  <template #body="{ data }">
                    <Tag
                      :value="statusLabel(data.status)"
                      :severity="statusSeverity(data.status)"
                    />
                  </template>
                </Column>
                <Column header="操作" style="width: 96px">
                  <template #body="{ data }">
                    <Button label="审核" text size="small" @click="openReview(data)" />
                  </template>
                </Column>
                <template #empty>
                  <AppEmptyState
                    icon="pi pi-id-card"
                    title="暂无证书"
                    description="当前筛选条件下没有待处理的证书申请。"
                  />
                </template>
              </DataTable>

              <Paginator
                v-if="certTotal > PAGE_SIZE"
                :rows="PAGE_SIZE"
                :total-records="certTotal"
                :first="(certPage - 1) * PAGE_SIZE"
                :page-link-size="5"
                template="PrevPageLink PageLinks NextPageLink"
                class="review-pager"
                @page="onCertPage"
              />
            </div>
          </TabPanel>

          <TabPanel value="types">
            <div class="types-panel">
              <div class="toolbar">
                <span class="count">共 {{ types.length }} 个类型</span>
                <Button label="新建类型" icon="pi pi-plus" size="small" @click="openCreateType" />
              </div>

              <AppErrorState
                v-if="typesError && types.length === 0"
                :description="typesError"
                @retry="loadTypes"
              />

              <AppLoadingState v-if="typesLoading && types.length === 0" label="正在加载类型" />

              <DataTable
                v-else-if="!typesError || types.length > 0"
                :value="types"
                :loading="typesLoading"
                data-key="id"
                size="small"
                responsive-layout="scroll"
                class="types-table"
              >
                <Column header="名称" style="min-width: 140px">
                  <template #body="{ data }"
                    ><span class="title">{{ data.name }}</span></template
                  >
                </Column>
                <Column header="描述" style="min-width: 200px">
                  <template #body="{ data }"
                    ><span class="muted">{{ data.description || '—' }}</span></template
                  >
                </Column>
                <Column header="排序" style="width: 80px">
                  <template #body="{ data }"
                    ><span class="num">{{ data.sortOrder }}</span></template
                  >
                </Column>
                <Column header="启用" style="width: 90px">
                  <template #body="{ data }">
                    <Tag
                      :value="data.isActive ? '已启用' : '已停用'"
                      :severity="data.isActive ? 'success' : 'secondary'"
                    />
                  </template>
                </Column>
                <Column header="操作" style="width: 140px">
                  <template #body="{ data }">
                    <div class="row-actions">
                      <Button label="编辑" text size="small" @click="openEditType(data)" />
                      <Button
                        label="删除"
                        text
                        size="small"
                        severity="danger"
                        :loading="deletingTypeId === data.id"
                        @click="confirmDeleteType(data)"
                      />
                    </div>
                  </template>
                </Column>
                <template #empty>
                  <AppEmptyState
                    icon="pi pi-id-card"
                    title="暂无类型"
                    description="新建证书类型。"
                  />
                </template>
              </DataTable>
            </div>
          </TabPanel>
        </TabPanels>
      </Tabs>
    </AppSurface>

    <!-- Review dialog -->
    <Dialog
      v-model:visible="reviewDialogVisible"
      header="审核证书"
      modal
      :draggable="false"
      :style="{ width: 'min(94vw, 560px)' }"
      @hide="revokeReviewImage"
    >
      <div v-if="reviewing" class="review-dialog">
        <div class="review-image-wrap">
          <div v-if="reviewImageLoading" class="review-image review-image--loading">
            <i class="pi pi-spin pi-spinner" aria-hidden="true" /> 加载图片…
          </div>
          <div v-else-if="reviewImageError" class="review-image review-image--error">
            {{ reviewImageError }}
          </div>
          <img
            v-else-if="reviewImageUrl"
            :src="reviewImageUrl"
            :alt="`${reviewing.type} 证书`"
            class="review-image"
          />
        </div>

        <dl class="review-meta">
          <div class="review-meta__row">
            <dt>用户</dt>
            <dd>
              {{ reviewing.user.nickname || reviewing.user.username }} · @{{
                reviewing.user.username
              }}
            </dd>
          </div>
          <div class="review-meta__row">
            <dt>类型</dt>
            <dd>{{ reviewing.type }}</dd>
          </div>
          <div class="review-meta__row">
            <dt>等级</dt>
            <dd>{{ reviewing.level }}</dd>
          </div>
          <div class="review-meta__row">
            <dt>提交时间</dt>
            <dd>{{ formatDateTime(reviewing.submittedAt) }}</dd>
          </div>
          <div class="review-meta__row">
            <dt>状态</dt>
            <dd>
              <Tag
                :value="statusLabel(reviewing.status)"
                :severity="statusSeverity(reviewing.status)"
              />
            </dd>
          </div>
        </dl>

        <Message
          v-if="reviewing.status === 'rejected' && reviewing.rejectReason"
          severity="error"
          :closable="false"
          role="status"
        >
          驳回原因：{{ reviewing.rejectReason }}
        </Message>

        <AppFormField
          v-if="reviewing.status === 'pending'"
          label="驳回原因"
          :error="rejectError"
          hint="驳回时必填，1–500 字"
        >
          <Textarea
            v-model="rejectReason"
            class="reject-reason"
            rows="3"
            maxlength="500"
            placeholder="填写驳回原因"
            fluid
            @update:model-value="rejectError = ''"
          />
        </AppFormField>
      </div>

      <template #footer>
        <Button label="取消" severity="secondary" text @click="reviewDialogVisible = false" />
        <template v-if="reviewing?.status === 'pending'">
          <Button label="通过" severity="success" :loading="reviewingAction" @click="approve" />
          <Button
            label="驳回"
            severity="danger"
            outlined
            :loading="reviewingAction"
            @click="reject"
          />
        </template>
      </template>
    </Dialog>

    <!-- Type dialog -->
    <Dialog
      v-model:visible="typeDialogVisible"
      :header="editingType ? '编辑类型' : '新建类型'"
      modal
      :draggable="false"
      :style="{ width: 'min(94vw, 480px)' }"
    >
      <div class="type-form">
        <AppFormField
          v-slot="{ id, invalid, describedby }"
          label="名称"
          :error="typeErrors.name"
          required
        >
          <InputText
            :id="id"
            v-model="typeForm.name"
            class="name-input"
            :invalid="invalid"
            :aria-describedby="describedby"
            maxlength="50"
            fluid
            @update:model-value="typeErrors.name = ''"
          />
        </AppFormField>

        <AppFormField
          v-slot="{ id, invalid, describedby }"
          label="描述"
          :error="typeErrors.description"
          hint="最多 200 字"
        >
          <Textarea
            :id="id"
            v-model="typeForm.description"
            class="desc-input"
            :invalid="invalid"
            :aria-describedby="describedby"
            rows="3"
            maxlength="200"
            fluid
            @update:model-value="typeErrors.description = ''"
          />
        </AppFormField>

        <AppFormField
          v-slot="{ id, invalid, describedby }"
          label="排序"
          :error="typeErrors.sortOrder"
          hint="0–10000，数值越小越靠前"
        >
          <InputNumber
            v-model="typeForm.sortOrder"
            :input-id="id"
            :invalid="invalid"
            :aria-describedby="describedby"
            :min="0"
            :max="10000"
            :step="1"
            fluid
            @update:model-value="typeErrors.sortOrder = ''"
          />
        </AppFormField>

        <AppFormField label="启用">
          <div class="toggle-row">
            <ToggleSwitch v-model="typeForm.isActive" input-id="type-active" />
            <label for="type-active">{{ typeForm.isActive ? '已启用' : '已停用' }}</label>
          </div>
        </AppFormField>
      </div>

      <template #footer>
        <Button label="取消" severity="secondary" text @click="typeDialogVisible = false" />
        <Button label="保存" icon="pi pi-check" :loading="typeSaving" @click="saveType" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.admin-certificates {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.admin-certificates__surface {
  min-width: 0;
}

.review-panel,
.types-panel {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
}

.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
}

.status-select {
  min-width: 160px;
}

.count {
  color: var(--kapoj-text-secondary);
  font-size: 13px;
}

.review-table,
.types-table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.review-table :deep(.p-datatable-table) {
  min-width: 560px;
}

.types-table :deep(.p-datatable-table) {
  min-width: 480px;
}

.review-table :deep(.p-datatable-thead > tr > th),
.types-table :deep(.p-datatable-thead > tr > th) {
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

.review-table :deep(.p-datatable-tbody > tr > td),
.types-table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.review-pager {
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.identity {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.identity__nickname {
  color: var(--kapoj-text);
  font-weight: 600;
}

.identity__handle {
  overflow: hidden;
  color: var(--kapoj-text-muted);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.title {
  color: var(--kapoj-text);
  font-weight: 600;
}

.mono {
  font-family: var(--kapoj-font-mono);
}

.muted {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.num {
  font-variant-numeric: tabular-nums;
}

.row-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-1);
}

.review-dialog {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
}

.review-image-wrap {
  display: flex;
  justify-content: center;
}

.review-image {
  display: block;
  max-width: 100%;
  max-height: 260px;
  object-fit: contain;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.review-image--loading,
.review-image--error {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  min-height: 120px;
  color: var(--kapoj-text-muted);
  font-size: 13px;
  background: var(--kapoj-bg);
  border: 1px dashed var(--kapoj-border-strong);
  border-radius: var(--kapoj-radius-sm);
}

.review-image--error {
  color: var(--kapoj-danger);
}

.review-meta {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  margin: 0;
  padding: var(--kapoj-space-3) var(--kapoj-space-4);
  background: var(--kapoj-bg);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.review-meta__row {
  display: flex;
  align-items: baseline;
  gap: var(--kapoj-space-3);
}

.review-meta__row dt {
  flex: none;
  width: 72px;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.review-meta__row dd {
  margin: 0;
  overflow-wrap: anywhere;
  font-size: 13px;
}

.type-form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
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
  cursor: pointer;
}

@media (max-width: 640px) {
  .review-meta__row {
    flex-direction: column;
    gap: var(--kapoj-space-1);
  }

  .review-meta__row dt {
    width: auto;
  }
}
</style>
