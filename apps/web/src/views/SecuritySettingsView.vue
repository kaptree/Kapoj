<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Message from 'primevue/message'
import Password from 'primevue/password'
import Tag from 'primevue/tag'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import { changePassword, listSessions, logoutAll, logoutOthers, revokeSession } from '@/api/auth'
import { ApiError } from '@/api/http'
import type { SessionView } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime } from '@/utils/format'
import { validatePassword } from '@/validation'

const auth = useAuthStore()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()

// --- Device summary ---------------------------------------------------------

/** Simple OS/browser inference from a raw User-Agent string (no extra deps). */
function deviceLabel(userAgent: string): string {
  const ua = userAgent ?? ''
  let os = ''
  if (/Windows/i.test(ua)) os = 'Windows'
  else if (/Android/i.test(ua)) os = 'Android'
  else if (/iPhone|iPad|iPod/i.test(ua)) os = 'iOS'
  else if (/Mac OS X|Macintosh/i.test(ua)) os = 'macOS'
  else if (/Linux/i.test(ua)) os = 'Linux'

  let browser = ''
  if (/Edg\//i.test(ua)) browser = 'Edge'
  else if (/OPR\/|Opera/i.test(ua)) browser = 'Opera'
  else if (/Firefox\//i.test(ua)) browser = 'Firefox'
  else if (/Chrome\//i.test(ua)) browser = 'Chrome'
  else if (/Safari\//i.test(ua)) browser = 'Safari'

  const parts = [os, browser].filter(Boolean)
  if (parts.length > 0) return parts.join(' · ')
  return ua ? ua : '未知设备'
}

// --- Change password --------------------------------------------------------

const passwordFields = ref({ currentPassword: '', newPassword: '', confirmPassword: '' })
const passwordErrors = ref({ currentPassword: '', newPassword: '', confirmPassword: '' })
const changing = ref(false)
const changeError = ref('')

function validateField(field: keyof typeof passwordFields.value) {
  if (field === 'currentPassword') {
    passwordErrors.value.currentPassword =
      passwordFields.value.currentPassword === '' ? '请输入当前密码' : ''
    return
  }
  if (field === 'newPassword') {
    const lengthError = validatePassword(passwordFields.value.newPassword) ?? ''
    if (!lengthError && passwordFields.value.newPassword === passwordFields.value.currentPassword) {
      passwordErrors.value.newPassword = '新密码不能与当前密码相同'
    } else {
      passwordErrors.value.newPassword = lengthError
    }
    return
  }
  if (field === 'confirmPassword') {
    if (passwordFields.value.confirmPassword === '') {
      passwordErrors.value.confirmPassword = '请再次输入新密码'
    } else if (passwordFields.value.confirmPassword !== passwordFields.value.newPassword) {
      passwordErrors.value.confirmPassword = '两次输入的密码不一致'
    } else {
      passwordErrors.value.confirmPassword = ''
    }
  }
}

function validatePasswordForm(): boolean {
  validateField('currentPassword')
  validateField('newPassword')
  validateField('confirmPassword')
  return (
    !passwordErrors.value.currentPassword &&
    !passwordErrors.value.newPassword &&
    !passwordErrors.value.confirmPassword
  )
}

async function submitPassword() {
  if (changing.value) return
  if (!validatePasswordForm()) return

  changing.value = true
  changeError.value = ''
  try {
    await changePassword({
      currentPassword: passwordFields.value.currentPassword,
      newPassword: passwordFields.value.newPassword,
    })
    toast.add({ severity: 'success', summary: '密码已修改，请重新登录', life: 4000 })
    auth.clear()
    await router.push({ name: 'login' })
  } catch (e) {
    const message = e instanceof ApiError ? e.message : '修改失败，请稍后再试'
    changeError.value = message
    toast.add({ severity: 'error', summary: '修改失败', detail: message, life: 4000 })
  } finally {
    changing.value = false
  }
}

// --- Sessions ---------------------------------------------------------------

const sessions = ref<SessionView[]>([])
const loading = ref(true)
const sessionsError = ref('')

const currentSessionId = computed(() => auth.sessionId)

function isCurrent(row: SessionView): boolean {
  return Boolean(currentSessionId.value && row.id === currentSessionId.value)
}

async function loadSessions() {
  loading.value = true
  sessionsError.value = ''
  try {
    sessions.value = await listSessions()
  } catch (e) {
    sessionsError.value = e instanceof ApiError ? e.message : '加载失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

function errorMessage(e: unknown): string {
  return e instanceof ApiError ? e.message : '操作失败，请稍后再试'
}

function confirmRevoke(row: SessionView) {
  confirm.require({
    header: '退出此设备',
    message: `确定退出设备「${deviceLabel(row.userAgent)}」吗？`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '退出',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => {
      void doRevoke(row)
    },
  })
}

async function doRevoke(row: SessionView) {
  try {
    await revokeSession(row.id)
    if (row.id === currentSessionId.value) {
      toast.add({ severity: 'success', summary: '已退出登录', life: 3000 })
      auth.clear()
      await router.push({ name: 'login' })
    } else {
      toast.add({ severity: 'success', summary: '已退出该设备', life: 3000 })
      await loadSessions()
    }
  } catch (e) {
    toast.add({ severity: 'error', summary: '退出失败', detail: errorMessage(e), life: 4000 })
  }
}

function confirmLogoutOthers() {
  const sessionId = currentSessionId.value
  if (!sessionId) return
  confirm.require({
    header: '退出其他设备',
    message: '确定退出除当前设备外的所有设备吗？',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '退出其他设备',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => {
      void doLogoutOthers()
    },
  })
}

async function doLogoutOthers() {
  const sessionId = currentSessionId.value
  if (!sessionId) return
  try {
    await logoutOthers(sessionId)
    toast.add({ severity: 'success', summary: '已退出其他设备', life: 3000 })
    await loadSessions()
  } catch (e) {
    toast.add({ severity: 'error', summary: '退出失败', detail: errorMessage(e), life: 4000 })
  }
}

function confirmLogoutAll() {
  confirm.require({
    header: '退出所有设备',
    message: '确定退出所有设备吗？退出后需要重新登录。',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '退出所有设备',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => {
      void doLogoutAll()
    },
  })
}

async function doLogoutAll() {
  try {
    await logoutAll()
    toast.add({ severity: 'success', summary: '已退出所有设备，请重新登录', life: 3000 })
    auth.clear()
    await router.push({ name: 'login' })
  } catch (e) {
    toast.add({ severity: 'error', summary: '退出失败', detail: errorMessage(e), life: 4000 })
  }
}

onMounted(() => {
  void loadSessions()
})
</script>

<template>
  <div class="security-page">
    <AppPageHeader
      eyebrow="ACCOUNT // SECURITY"
      title="账号安全"
      description="修改密码并管理当前账号的活跃会话。"
    />

    <AppSurface as="section" tone="raised" padding="lg">
      <h2 class="section-title">修改密码</h2>
      <Message
        v-if="changeError"
        severity="error"
        :closable="false"
        class="form-banner"
        role="alert"
      >
        {{ changeError }}
      </Message>
      <form class="password-form" novalidate @submit.prevent="submitPassword">
        <AppFormField
          v-slot="{ id, invalid, describedby }"
          label="当前密码"
          :error="passwordErrors.currentPassword"
          required
        >
          <Password
            :id="id"
            v-model="passwordFields.currentPassword"
            :invalid="invalid"
            :aria-describedby="describedby"
            :feedback="false"
            autocomplete="current-password"
            fluid
            @blur="validateField('currentPassword')"
            @update:model-value="changeError = ''"
          />
        </AppFormField>

        <AppFormField
          v-slot="{ id, invalid, describedby }"
          label="新密码"
          :error="passwordErrors.newPassword"
          hint="8–72 个字节"
          required
        >
          <Password
            :id="id"
            v-model="passwordFields.newPassword"
            :invalid="invalid"
            :aria-describedby="describedby"
            :feedback="false"
            autocomplete="new-password"
            fluid
            @blur="validateField('newPassword')"
            @update:model-value="changeError = ''"
          />
        </AppFormField>

        <AppFormField
          v-slot="{ id, invalid, describedby }"
          label="确认新密码"
          :error="passwordErrors.confirmPassword"
          required
        >
          <Password
            :id="id"
            v-model="passwordFields.confirmPassword"
            :invalid="invalid"
            :aria-describedby="describedby"
            :feedback="false"
            autocomplete="new-password"
            fluid
            @blur="validateField('confirmPassword')"
            @update:model-value="changeError = ''"
          />
        </AppFormField>

        <Button
          type="submit"
          label="修改密码"
          icon="pi pi-check"
          :loading="changing"
          class="password-submit"
        />
      </form>
    </AppSurface>

    <AppSurface as="section" tone="raised" padding="lg">
      <div class="sessions-header">
        <h2 class="section-title">活跃会话</h2>
        <div class="sessions-actions">
          <Button
            label="退出其他设备"
            icon="pi pi-mobile"
            severity="danger"
            outlined
            :disabled="!currentSessionId"
            @click="confirmLogoutOthers"
          />
          <Button
            label="退出所有设备"
            icon="pi pi-sign-out"
            severity="danger"
            @click="confirmLogoutAll"
          />
        </div>
      </div>
      <p v-if="!currentSessionId" class="sessions-hint" role="note">
        当前会话缺少 sessionId，无法标记当前设备或退出其他设备。
      </p>

      <AppLoadingState v-if="loading" label="正在加载会话" />

      <AppErrorState v-else-if="sessionsError" :description="sessionsError" @retry="loadSessions" />

      <AppEmptyState
        v-else-if="sessions.length === 0"
        icon="pi pi-mobile"
        title="暂无活跃会话"
        description="登录其他设备后，会话会显示在这里。"
      />

      <DataTable
        v-else
        :value="sessions"
        data-key="id"
        size="small"
        responsive-layout="scroll"
        class="sessions-table"
      >
        <Column header="设备" style="min-width: 200px">
          <template #body="{ data }">
            <div class="device-cell">
              <span class="device-label">{{ deviceLabel(data.userAgent) }}</span>
              <Tag v-if="isCurrent(data)" value="当前设备" severity="success" />
            </div>
          </template>
        </Column>
        <Column header="IP" style="min-width: 130px">
          <template #body="{ data }"
            ><span class="mono">{{ data.ip || '—' }}</span></template
          >
        </Column>
        <Column header="创建时间" style="min-width: 160px">
          <template #body="{ data }"
            ><span class="muted">{{ formatDateTime(data.createdAt) }}</span></template
          >
        </Column>
        <Column header="最近使用" style="min-width: 160px">
          <template #body="{ data }"
            ><span class="muted">{{ formatDateTime(data.lastUsedAt) }}</span></template
          >
        </Column>
        <Column header="操作" style="width: 120px">
          <template #body="{ data }">
            <Button
              label="退出此设备"
              size="small"
              text
              severity="danger"
              icon="pi pi-sign-out"
              @click="confirmRevoke(data)"
            />
          </template>
        </Column>
      </DataTable>
    </AppSurface>
  </div>
</template>

<style scoped>
.security-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
  max-width: var(--kapoj-content-max);
}

.section-title {
  margin: 0 0 var(--kapoj-space-4);
  color: var(--kapoj-text);
  font-size: 1rem;
  font-weight: 600;
}

.password-form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.form-banner {
  margin-bottom: var(--kapoj-space-4);
}

.password-submit {
  align-self: flex-start;
  margin-top: var(--kapoj-space-1);
}

.sessions-header {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
  margin-bottom: var(--kapoj-space-4);
}

.sessions-header .section-title {
  margin: 0;
}

.sessions-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
}

.sessions-hint {
  margin: 0 0 var(--kapoj-space-3);
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.sessions-table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.sessions-table :deep(.p-datatable-thead > tr > th) {
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

.sessions-table :deep(.p-datatable-tbody > tr > td) {
  padding: 10px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.device-cell {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
}

.device-label {
  color: var(--kapoj-text);
  font-weight: 500;
}

.mono {
  font-family: var(--kapoj-font-mono);
}

.muted {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

@media (max-width: 640px) {
  .sessions-header {
    align-items: flex-start;
    flex-direction: column;
  }

  .sessions-actions {
    width: 100%;
  }

  .sessions-actions :deep(.p-button) {
    flex: 1 1 auto;
  }

  .password-submit {
    width: 100%;
  }

  .sessions-table :deep(.p-datatable-thead > tr > th),
  .sessions-table :deep(.p-datatable-tbody > tr > td) {
    padding: 8px var(--kapoj-space-2);
  }
}
</style>
