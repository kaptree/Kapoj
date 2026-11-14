<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { useToast } from 'primevue/usetoast'

import {
  createAccountBackup,
  createPlatformBackup,
  downloadBackup,
  getBackupOverview,
  restoreAccountBackup,
  restorePlatformBackup,
  updateBackupSchedule,
} from '@/api/backups'
import { ApiError } from '@/api/http'
import type {
  BackupOverview,
  BackupPackage,
  BackupRestoreJob,
  BackupSchedule,
  BackupScheduleInput,
} from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { formatDateTime } from '@/utils/format'

const FREQUENCIES: { label: string; value: BackupScheduleInput['frequency'] }[] = [
  { label: '每天', value: 'daily' },
  { label: '每周', value: 'weekly' },
  { label: '每月', value: 'monthly' },
]
const WEEKDAYS = [
  { label: '周日', value: 0 },
  { label: '周一', value: 1 },
  { label: '周二', value: 2 },
  { label: '周三', value: 3 },
  { label: '周四', value: 4 },
  { label: '周五', value: 5 },
  { label: '周六', value: 6 },
]

const toast = useToast()
const overview = ref<BackupOverview | null>(null)
const loading = ref(false)
const error = ref('')
const busy = ref('')
const accountUserId = ref<number | null>(null)
const accountAll = ref(false)

const platformSchedule = ref<BackupScheduleInput>(defaultSchedule())
const accountsSchedule = ref<BackupScheduleInput>(defaultSchedule())

const platformRestoreOpen = ref(false)
const accountRestoreOpen = ref(false)
const platformPackageId = ref<number | null>(null)
const platformFile = ref<File | null>(null)
const platformPhrase = ref('')
const accountPackageId = ref<number | null>(null)
const accountFile = ref<File | null>(null)
const restoreUserId = ref<number | null>(null)

let latest = 0
let timer: ReturnType<typeof setInterval> | undefined

const platformPackage = computed(() => overview.value?.platformPackage ?? null)
const accountsPackage = computed(() => overview.value?.accountsPackage ?? null)
const restoreCandidates = computed(() => {
  const list: BackupPackage[] = []
  if (accountsPackage.value) list.push(accountsPackage.value)
  list.push(...(overview.value?.accountPackages ?? []))
  return list
})
const platformOptions = computed(() =>
  platformPackage.value
    ? [{ label: `最新平台备份 #${platformPackage.value.id}`, value: platformPackage.value.id }]
    : [],
)
const accountOptions = computed(() =>
  restoreCandidates.value.map((item) => ({
    label: `#${item.id} · ${item.kind === 'accounts' ? '全部账号' : `用户 ${item.userId}`}`,
    value: item.id,
  })),
)

function defaultSchedule(): BackupScheduleInput {
  return {
    enabled: false,
    frequency: 'daily',
    runHour: 3,
    runMinute: 0,
    weekday: 1,
    dayOfMonth: 1,
  }
}

function syncSchedule(target: BackupScheduleInput, source?: BackupSchedule) {
  if (!source) return
  target.enabled = source.enabled
  target.frequency = source.frequency
  target.runHour = source.runHour
  target.runMinute = source.runMinute
  target.weekday = source.weekday
  target.dayOfMonth = source.dayOfMonth
}

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const data = await getBackupOverview()
    if (seq !== latest) return
    overview.value = data
    syncSchedule(
      platformSchedule.value,
      data.schedules.find((s) => s.kind === 'platform'),
    )
    syncSchedule(
      accountsSchedule.value,
      data.schedules.find((s) => s.kind === 'accounts'),
    )
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载备份状态失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

function formatSize(bytes: number): string {
  if (bytes <= 0) return '—'
  const units = ['B', 'KiB', 'MiB', 'GiB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`
}

function statusSeverity(status: string): 'success' | 'warn' | 'danger' | 'info' {
  if (status === 'ready' || status === 'success') return 'success'
  if (status === 'running' || status === 'queued') return 'info'
  if (status === 'failed') return 'danger'
  return 'warn'
}

function packageLabel(pkg: BackupPackage | null): string {
  if (!pkg) return '暂无备份'
  return `#${pkg.id} · ${formatSize(pkg.sizeBytes)} · ${formatDateTime(pkg.completedAt)}`
}

async function createPlatform() {
  busy.value = 'platform'
  try {
    await createPlatformBackup()
    toast.add({ severity: 'success', summary: '已开始生成全平台备份', life: 3000 })
    await load()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '创建失败',
      detail: e instanceof ApiError ? e.message : '创建备份失败，请稍后再试',
      life: 4000,
    })
  } finally {
    busy.value = ''
  }
}

async function createAccounts() {
  if (!accountAll.value && (!accountUserId.value || accountUserId.value <= 0)) {
    toast.add({ severity: 'warn', summary: '请填写用户 ID 或选择全部账号', life: 3000 })
    return
  }
  busy.value = 'accounts'
  try {
    await createAccountBackup(accountAll.value ? {} : { userId: accountUserId.value ?? undefined })
    toast.add({ severity: 'success', summary: '已开始生成账号备份', life: 3000 })
    await load()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '创建失败',
      detail: e instanceof ApiError ? e.message : '创建备份失败，请稍后再试',
      life: 4000,
    })
  } finally {
    busy.value = ''
  }
}

async function download(pkg: BackupPackage | null) {
  if (!pkg) return
  try {
    const blob = await downloadBackup(pkg.id)
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = pkg.filename.split('/').pop() || `backup-${pkg.id}.zip`
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '下载失败',
      detail: e instanceof ApiError ? e.message : '下载备份失败，请稍后再试',
      life: 4000,
    })
  }
}

async function saveSchedule(kind: 'platform' | 'accounts') {
  const form = kind === 'platform' ? platformSchedule.value : accountsSchedule.value
  busy.value = `schedule-${kind}`
  try {
    await updateBackupSchedule(kind, { ...form })
    toast.add({ severity: 'success', summary: '定时任务已保存', life: 3000 })
    await load()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '保存失败',
      detail: e instanceof ApiError ? e.message : '保存定时任务失败，请稍后再试',
      life: 4000,
    })
  } finally {
    busy.value = ''
  }
}

function openPlatformRestore() {
  platformPackageId.value = platformPackage.value?.id ?? null
  platformFile.value = null
  platformPhrase.value = ''
  platformRestoreOpen.value = true
}

function openAccountRestore() {
  accountPackageId.value = restoreCandidates.value[0]?.id ?? null
  accountFile.value = null
  restoreUserId.value = null
  accountRestoreOpen.value = true
}

function onPlatformFile(event: Event) {
  const input = event.target as HTMLInputElement
  platformFile.value = input.files?.[0] ?? null
  if (platformFile.value) platformPackageId.value = null
}

function onAccountFile(event: Event) {
  const input = event.target as HTMLInputElement
  accountFile.value = input.files?.[0] ?? null
  if (accountFile.value) accountPackageId.value = null
}

async function submitPlatformRestore() {
  if (platformPhrase.value.trim() !== 'RESTORE') {
    toast.add({ severity: 'warn', summary: '请输入 RESTORE 确认恢复', life: 3000 })
    return
  }
  if (!platformFile.value && !platformPackageId.value) {
    toast.add({ severity: 'warn', summary: '请选择服务器备份或上传 ZIP', life: 3000 })
    return
  }
  busy.value = 'restore-platform'
  try {
    const job = await restorePlatformBackup(
      { packageId: platformPackageId.value ?? undefined, confirm: 'RESTORE' },
      platformFile.value ?? undefined,
    )
    toast.add({
      severity: 'success',
      summary: '全平台恢复任务已开始',
      detail: `任务 #${job.id}`,
      life: 4000,
    })
    platformRestoreOpen.value = false
    await load()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '恢复启动失败',
      detail: e instanceof ApiError ? e.message : '恢复失败，请稍后再试',
      life: 4000,
    })
  } finally {
    busy.value = ''
  }
}

async function submitAccountRestore() {
  if (!restoreUserId.value || restoreUserId.value <= 0) {
    toast.add({ severity: 'warn', summary: '请填写目标用户 ID', life: 3000 })
    return
  }
  if (!accountFile.value && !accountPackageId.value) {
    toast.add({ severity: 'warn', summary: '请选择备份包或上传 ZIP', life: 3000 })
    return
  }
  busy.value = 'restore-account'
  try {
    const job = await restoreAccountBackup(
      { packageId: accountPackageId.value ?? undefined, userId: restoreUserId.value },
      accountFile.value ?? undefined,
    )
    toast.add({
      severity: 'success',
      summary: '账号恢复任务已开始',
      detail: `任务 #${job.id}`,
      life: 4000,
    })
    accountRestoreOpen.value = false
    await load()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '恢复启动失败',
      detail: e instanceof ApiError ? e.message : '恢复失败，请稍后再试',
      life: 4000,
    })
  } finally {
    busy.value = ''
  }
}

function scheduleSummary(schedule: BackupScheduleInput): string {
  if (!schedule.enabled) return '已关闭'
  const time = `${String(schedule.runHour).padStart(2, '0')}:${String(schedule.runMinute).padStart(2, '0')}`
  if (schedule.frequency === 'weekly') {
    return `每周${WEEKDAYS.find((d) => d.value === schedule.weekday)?.label ?? ''} ${time}`
  }
  if (schedule.frequency === 'monthly') {
    return `每月 ${schedule.dayOfMonth} 日 ${time}`
  }
  return `每天 ${time}`
}

function latestRestore(): BackupRestoreJob | null {
  return overview.value?.restores[0] ?? null
}

onMounted(() => {
  void load()
  timer = setInterval(() => {
    const running =
      overview.value?.restores.some((job) => job.status === 'running') ||
      [overview.value?.platformPackage, overview.value?.accountsPackage].some(
        (pkg) => pkg?.status === 'running',
      ) ||
      (overview.value?.accountPackages ?? []).some((pkg) => pkg.status === 'running')
    if (running) void load()
  }, 3000)
})

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div class="backup-page">
    <AppPageHeader
      eyebrow="SYSTEM // BACKUP"
      title="数据备份"
      description="管理系统全平台与用户账号备份、定时任务与恢复。备份包含敏感数据，请仅授权可信管理员。"
    >
      <template #actions>
        <Button label="刷新" icon="pi pi-refresh" severity="secondary" outlined @click="load" />
      </template>
    </AppPageHeader>

    <Message severity="warn" :closable="false">
      全平台恢复会替换数据库与业务文件，并自动生成恢复前安全备份；账号恢复按原 user_id
      覆盖关联数据。 恢复期间不要重复提交操作。
    </Message>

    <AppLoadingState v-if="loading && !overview" label="正在加载备份状态" />
    <AppErrorState v-else-if="error && !overview" :description="error" @retry="load" />
    <template v-else-if="overview">
      <div class="backup-grid">
        <AppSurface padding="lg" class="backup-card">
          <div class="card-head">
            <div>
              <h2>全平台备份</h2>
              <p>数据库、业务文件、AI/邮件密钥。</p>
            </div>
            <Tag
              :value="platformPackage?.status ?? '暂无'"
              :severity="statusSeverity(platformPackage?.status ?? '')"
            />
          </div>
          <p class="package-line" data-test="platform-package">
            {{ packageLabel(platformPackage) }}
          </p>
          <div class="card-actions">
            <Button
              label="立即备份"
              icon="pi pi-database"
              :loading="busy === 'platform'"
              @click="createPlatform"
            />
            <Button
              label="下载"
              icon="pi pi-download"
              severity="secondary"
              outlined
              :disabled="!platformPackage"
              @click="download(platformPackage)"
            />
            <Button
              label="恢复"
              icon="pi pi-replay"
              severity="danger"
              outlined
              :disabled="!platformPackage"
              data-test="platform-restore"
              @click="openPlatformRestore"
            />
          </div>
          <div class="schedule">
            <div class="schedule-head">
              <strong>定时任务</strong>
              <span>{{ scheduleSummary(platformSchedule) }}</span>
            </div>
            <div class="schedule-row">
              <ToggleSwitch v-model="platformSchedule.enabled" input-id="platform-enabled" />
              <Select
                v-model="platformSchedule.frequency"
                :options="FREQUENCIES"
                option-label="label"
                option-value="value"
              />
              <InputNumber v-model="platformSchedule.runHour" :min="0" :max="23" suffix=" 时" />
              <InputNumber v-model="platformSchedule.runMinute" :min="0" :max="59" suffix=" 分" />
              <Select
                v-if="platformSchedule.frequency === 'weekly'"
                v-model="platformSchedule.weekday"
                :options="WEEKDAYS"
                option-label="label"
                option-value="value"
              />
              <InputNumber
                v-if="platformSchedule.frequency === 'monthly'"
                v-model="platformSchedule.dayOfMonth"
                :min="1"
                :max="31"
                suffix=" 日"
              />
              <Button
                label="保存"
                size="small"
                :loading="busy === 'schedule-platform'"
                @click="saveSchedule('platform')"
              />
            </div>
          </div>
        </AppSurface>

        <AppSurface padding="lg" class="backup-card">
          <div class="card-head">
            <div>
              <h2>账号备份</h2>
              <p>完整账号快照，不包含用户创建的平台内容。</p>
            </div>
            <Tag
              :value="accountsPackage?.status ?? '暂无'"
              :severity="statusSeverity(accountsPackage?.status ?? '')"
            />
          </div>
          <p class="package-line" data-test="accounts-package">
            {{ packageLabel(accountsPackage) }}
          </p>
          <div class="account-create">
            <ToggleSwitch v-model="accountAll" input-id="account-all" />
            <label for="account-all">全部账号</label>
            <InputNumber
              v-if="!accountAll"
              v-model="accountUserId"
              :min="1"
              placeholder="用户 ID"
              data-test="account-user-id"
            />
            <Button
              label="立即备份"
              icon="pi pi-users"
              :loading="busy === 'accounts'"
              @click="createAccounts"
            />
            <Button
              label="下载"
              icon="pi pi-download"
              severity="secondary"
              outlined
              :disabled="!accountsPackage"
              @click="download(accountsPackage)"
            />
            <Button
              label="恢复账号"
              icon="pi pi-replay"
              severity="danger"
              outlined
              :disabled="restoreCandidates.length === 0"
              data-test="account-restore"
              @click="openAccountRestore"
            />
          </div>
          <div class="schedule">
            <div class="schedule-head">
              <strong>定时任务</strong>
              <span>{{ scheduleSummary(accountsSchedule) }}</span>
            </div>
            <div class="schedule-row">
              <ToggleSwitch v-model="accountsSchedule.enabled" input-id="accounts-enabled" />
              <Select
                v-model="accountsSchedule.frequency"
                :options="FREQUENCIES"
                option-label="label"
                option-value="value"
              />
              <InputNumber v-model="accountsSchedule.runHour" :min="0" :max="23" suffix=" 时" />
              <InputNumber v-model="accountsSchedule.runMinute" :min="0" :max="59" suffix=" 分" />
              <Select
                v-if="accountsSchedule.frequency === 'weekly'"
                v-model="accountsSchedule.weekday"
                :options="WEEKDAYS"
                option-label="label"
                option-value="value"
              />
              <Button
                label="保存"
                size="small"
                :loading="busy === 'schedule-accounts'"
                @click="saveSchedule('accounts')"
              />
            </div>
          </div>
        </AppSurface>
      </div>

      <AppSurface padding="lg">
        <div class="section-head">
          <h2>账号备份包</h2>
          <span>最近 50 个单用户/全部账号包</span>
        </div>
        <DataTable
          v-if="overview.accountPackages.length || accountsPackage"
          :value="[
            ...(accountsPackage ? [accountsPackage] : []),
            ...overview.accountPackages.filter((p) => p.id !== accountsPackage?.id),
          ]"
          size="small"
          responsive-layout="scroll"
        >
          <Column header="包" style="width: 90px">
            <template #body="{ data }">#{{ data.id }}</template>
          </Column>
          <Column header="类型" style="width: 110px">
            <template #body="{ data }">{{
              data.kind === 'accounts' ? '全部账号' : `用户 ${data.userId}`
            }}</template>
          </Column>
          <Column header="大小" style="width: 100px">
            <template #body="{ data }">{{ formatSize(data.sizeBytes) }}</template>
          </Column>
          <Column header="状态" style="width: 90px">
            <template #body="{ data }"
              ><Tag :value="data.status" :severity="statusSeverity(data.status)"
            /></template>
          </Column>
          <Column header="创建时间" style="width: 180px">
            <template #body="{ data }">{{ formatDateTime(data.createdAt) }}</template>
          </Column>
          <Column header="操作" style="width: 100px">
            <template #body="{ data }">
              <Button
                label="下载"
                text
                size="small"
                :disabled="data.status !== 'ready'"
                @click="download(data)"
              />
            </template>
          </Column>
        </DataTable>
        <AppEmptyState
          v-else
          icon="pi pi-database"
          title="暂无账号备份"
          description="创建单个用户或全部账号备份后会显示在这里。"
        />
      </AppSurface>

      <AppSurface padding="lg">
        <div class="section-head">
          <h2>恢复任务</h2>
          <span v-if="latestRestore()">
            最近：#{{ latestRestore()?.id }} · {{ latestRestore()?.status }}
          </span>
        </div>
        <DataTable
          v-if="overview.restores.length"
          :value="overview.restores"
          size="small"
          responsive-layout="scroll"
        >
          <Column header="任务" style="width: 80px">
            <template #body="{ data }">#{{ data.id }}</template>
          </Column>
          <Column header="类型" style="width: 100px">
            <template #body="{ data }">{{
              data.kind === 'platform' ? '全平台' : `账号 ${data.targetUserId}`
            }}</template>
          </Column>
          <Column header="状态" style="width: 90px">
            <template #body="{ data }"
              ><Tag :value="data.status" :severity="statusSeverity(data.status)"
            /></template>
          </Column>
          <Column header="错误" style="min-width: 220px">
            <template #body="{ data }">{{ data.error || '—' }}</template>
          </Column>
          <Column header="完成时间" style="width: 180px">
            <template #body="{ data }">{{ formatDateTime(data.completedAt) }}</template>
          </Column>
        </DataTable>
        <AppEmptyState
          v-else
          icon="pi pi-replay"
          title="暂无恢复任务"
          description="恢复操作会记录在这里。"
        />
      </AppSurface>
    </template>

    <Dialog
      v-model:visible="platformRestoreOpen"
      header="恢复全平台备份"
      modal
      :style="{ width: 'min(92vw, 560px)' }"
    >
      <div class="dialog-form">
        <Message severity="warn" :closable="false"
          >这会替换当前数据库和业务文件，并自动生成恢复前安全备份。</Message
        >
        <label>服务器备份包</label>
        <Select
          v-model="platformPackageId"
          :options="platformOptions"
          option-label="label"
          option-value="value"
          placeholder="选择最新平台备份"
          show-clear
        />
        <label>或上传 ZIP</label>
        <input
          type="file"
          accept=".zip,application/zip"
          data-test="platform-restore-file"
          @change="onPlatformFile"
        />
        <label>输入 RESTORE 确认</label>
        <InputText
          v-model="platformPhrase"
          data-test="platform-restore-phrase"
          placeholder="RESTORE"
        />
        <Button
          label="开始恢复"
          severity="danger"
          :loading="busy === 'restore-platform'"
          data-test="platform-restore-submit"
          @click="submitPlatformRestore"
        />
      </div>
    </Dialog>

    <Dialog
      v-model:visible="accountRestoreOpen"
      header="恢复账号备份"
      modal
      :style="{ width: 'min(92vw, 560px)' }"
    >
      <div class="dialog-form">
        <label>目标用户 ID</label>
        <InputNumber v-model="restoreUserId" :min="1" data-test="account-restore-user" />
        <label>服务器备份包</label>
        <Select
          v-model="accountPackageId"
          :options="accountOptions"
          option-label="label"
          option-value="value"
          placeholder="选择账号备份"
          show-clear
        />
        <label>或上传 ZIP</label>
        <input
          type="file"
          accept=".zip,application/zip"
          data-test="account-restore-file"
          @change="onAccountFile"
        />
        <Button
          label="开始恢复"
          severity="danger"
          :loading="busy === 'restore-account'"
          data-test="account-restore-submit"
          @click="submitAccountRestore"
        />
      </div>
    </Dialog>
  </div>
</template>

<style scoped>
.backup-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.backup-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--kapoj-space-4);
}

.backup-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
}

.card-head,
.section-head,
.schedule-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
}

.card-head h2,
.section-head h2 {
  margin: 0;
  font-size: 1rem;
}

.card-head p,
.section-head span {
  margin: 4px 0 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.package-line {
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.card-actions,
.account-create,
.schedule-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
  align-items: center;
}

.account-create label {
  color: var(--kapoj-text-secondary);
  font-size: 12px;
}

.schedule {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  padding-top: var(--kapoj-space-3);
  border-top: 1px solid var(--kapoj-border);
}

.schedule-head {
  align-items: center;
  color: var(--kapoj-text-secondary);
  font-size: 12px;
}

.schedule-row :deep(.p-inputnumber),
.schedule-row :deep(.p-select) {
  min-width: 110px;
}

.section-head {
  margin-bottom: var(--kapoj-space-3);
}

.dialog-form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
}

.dialog-form label {
  color: var(--kapoj-text-secondary);
  font-size: 12px;
  font-weight: 600;
}

@media (max-width: 900px) {
  .backup-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
