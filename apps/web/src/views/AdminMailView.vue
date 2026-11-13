<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import { getAdminMail, testAdminMail, updateAdminMail } from '@/api/mail'
import type { AdminMailView, MailSettingsInput, PasswordState } from '@/api/types'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { validateEmail } from '@/validation'

type ProviderValue = MailSettingsInput['provider']

interface ProviderOption {
  label: string
  value: ProviderValue
  host: string
  port: number
  tls: MailSettingsInput['smtpTls']
}

const PROVIDERS: ProviderOption[] = [
  { label: 'QQ 邮箱', value: 'qq', host: 'smtp.qq.com', port: 465, tls: 'ssl' },
  {
    label: '腾讯云 SES',
    value: 'tencent_ses',
    host: 'smtp.qcloudmail.com',
    port: 465,
    tls: 'ssl',
  },
  {
    label: '阿里云邮件推送',
    value: 'aliyun_dm',
    host: 'smtpdm.aliyun.com',
    port: 465,
    tls: 'ssl',
  },
  { label: '自定义 SMTP', value: 'custom', host: '', port: 587, tls: 'starttls' },
]

const TLS_OPTIONS: Array<{ label: string; value: MailSettingsInput['smtpTls'] }> = [
  { label: 'SSL（常用 465）', value: 'ssl' },
  { label: 'STARTTLS（常用 587）', value: 'starttls' },
  { label: '不加密（仅测试）', value: 'none' },
]

const toast = useToast()

const loading = ref(false)
const error = ref('')
const saving = ref(false)
const testing = ref(false)
const testTo = ref('')
const view = ref<AdminMailView | null>(null)

const settings = ref({
  provider: 'qq' as ProviderValue,
  smtpHost: 'smtp.qq.com',
  smtpPort: 465,
  smtpTls: 'ssl' as MailSettingsInput['smtpTls'],
  smtpUsername: '',
  fromEmail: '',
  fromName: 'kapOJ',
  replyTo: '',
  publicBaseUrl: '',
  sendingEnabled: false,
  requireEmailVerification: true,
})

const password = ref('')
const passwordState = ref<PasswordState>('none')
const ready = ref(false)

const passwordLabel = computed(() => {
  switch (passwordState.value) {
    case 'ready':
      return '已就绪'
    case 'key_error':
      return '密钥不匹配'
    default:
      return '未配置'
  }
})

const passwordSeverity = computed<'success' | 'danger' | 'secondary'>(() => {
  if (passwordState.value === 'ready') return 'success'
  if (passwordState.value === 'key_error') return 'danger'
  return 'secondary'
})

const statusLabel = computed(() => (ready.value ? '可发送' : '未就绪'))
const statusSeverity = computed<'success' | 'warn'>(() => (ready.value ? 'success' : 'warn'))

function applyView(next: AdminMailView) {
  view.value = next
  settings.value.provider = next.provider
  settings.value.smtpHost = next.smtpHost
  settings.value.smtpPort = next.smtpPort
  settings.value.smtpTls = next.smtpTls
  settings.value.smtpUsername = next.smtpUsername
  settings.value.fromEmail = next.fromEmail
  settings.value.fromName = next.fromName
  settings.value.replyTo = next.replyTo
  settings.value.publicBaseUrl = next.publicBaseUrl
  settings.value.sendingEnabled = next.sendingEnabled
  settings.value.requireEmailVerification = next.requireEmailVerification
  passwordState.value = next.passwordState
  ready.value = next.ready
  password.value = ''
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    applyView(await getAdminMail())
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '加载邮件设置失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

function applyProvider(value: ProviderValue) {
  const preset = PROVIDERS.find((item) => item.value === value)
  if (!preset) return
  settings.value.smtpHost = preset.host
  settings.value.smtpPort = preset.port
  settings.value.smtpTls = preset.tls
}

function validBaseUrl(value: string): boolean {
  try {
    const url = new URL(value)
    return (
      (url.protocol === 'http:' || url.protocol === 'https:') &&
      url.host !== '' &&
      (url.pathname === '' || url.pathname === '/') &&
      url.search === '' &&
      url.hash === ''
    )
  } catch {
    return false
  }
}

function validate(): boolean {
  error.value = ''
  if (settings.value.requireEmailVerification && !settings.value.sendingEnabled) {
    error.value = '请先启用邮件发送，再开启强制邮箱验证'
    return false
  }
  if (settings.value.publicBaseUrl !== '' && !validBaseUrl(settings.value.publicBaseUrl)) {
    error.value = '站点公开地址需为 http(s) 根地址，不能包含路径、查询或片段'
    return false
  }
  if (!settings.value.sendingEnabled && !settings.value.requireEmailVerification) return true

  if (settings.value.smtpHost.trim() === '') {
    error.value = 'SMTP 主机不能为空'
    return false
  }
  if (validateEmail(settings.value.fromEmail) !== null) {
    error.value = '发件邮箱格式不正确'
    return false
  }
  if (!validBaseUrl(settings.value.publicBaseUrl)) {
    error.value = '启用邮件前必须填写站点公开地址'
    return false
  }
  if (settings.value.provider !== 'custom' && settings.value.smtpUsername.trim() === '') {
    error.value = '当前邮件服务商需要填写 SMTP 用户名'
    return false
  }
  if (
    settings.value.provider !== 'custom' &&
    password.value === '' &&
    passwordState.value !== 'ready'
  ) {
    error.value = '请填写 SMTP 密码/授权码'
    return false
  }
  return true
}

async function save() {
  if (!validate()) return
  saving.value = true
  try {
    const input: MailSettingsInput = {
      ...settings.value,
      smtpPort: Number(settings.value.smtpPort),
    }
    if (password.value !== '') input.smtpPassword = password.value
    applyView(await updateAdminMail(input))
    toast.add({ severity: 'success', summary: '邮件设置已保存', life: 3000 })
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '保存失败，请稍后再试'
  } finally {
    saving.value = false
  }
}

async function sendTest() {
  if (validateEmail(testTo.value) !== null) {
    error.value = '请输入正确的测试收件人邮箱'
    return
  }
  testing.value = true
  error.value = ''
  try {
    await testAdminMail(testTo.value)
    toast.add({ severity: 'success', summary: '测试邮件已发送', life: 3000 })
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '测试邮件发送失败，请稍后再试'
  } finally {
    testing.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="admin-mail">
    <AppPageHeader
      eyebrow="ADMIN // MAIL"
      title="邮件设置"
      description="配置注册验证邮件与密码找回邮件的发送通道；密码只保存加密后的密文。"
    />

    <AppLoadingState v-if="loading" />
    <AppErrorState v-else-if="error && !view" :description="error" @retry="load" />

    <template v-else>
      <Message v-if="error" severity="error" :closable="false" class="settings-banner" role="alert">
        {{ error }}
      </Message>

      <AppSurface class="status-card">
        <div class="status-row">
          <span class="status-label">发送状态</span>
          <Tag :value="statusLabel" :severity="statusSeverity" />
        </div>
        <div class="status-row">
          <span class="status-label">SMTP 密码</span>
          <Tag :value="passwordLabel" :severity="passwordSeverity" />
        </div>
        <p class="status-hint">
          {{
            ready
              ? '当前配置可以发送邮件。'
              : '填写完整配置、保存并测试通过后，才能开启强制邮箱验证。'
          }}
        </p>
      </AppSurface>

      <AppSurface>
        <h2 class="section-title">发件配置</h2>
        <div class="settings-grid">
          <AppFormField label="邮件服务商" required>
            <Select
              :model-value="settings.provider"
              :options="PROVIDERS"
              option-label="label"
              option-value="value"
              fluid
              @update:model-value="applyProvider"
            />
          </AppFormField>

          <AppFormField label="SMTP 主机" required>
            <InputText v-model="settings.smtpHost" fluid placeholder="smtp.qq.com" />
          </AppFormField>

          <AppFormField label="SMTP 端口" required>
            <InputText
              :model-value="String(settings.smtpPort)"
              type="number"
              fluid
              @update:model-value="settings.smtpPort = Number($event)"
            />
          </AppFormField>

          <AppFormField label="TLS 模式" required>
            <Select
              v-model="settings.smtpTls"
              :options="TLS_OPTIONS"
              option-label="label"
              option-value="value"
              fluid
            />
          </AppFormField>

          <AppFormField label="SMTP 用户名" required>
            <InputText
              v-model="settings.smtpUsername"
              fluid
              autocomplete="off"
              placeholder="bot@qq.com"
            />
          </AppFormField>

          <AppFormField
            label="SMTP 密码 / 授权码"
            :hint="passwordState === 'ready' ? '已配置，留空不修改' : 'QQ 邮箱请填写授权码'"
          >
            <Password
              v-model="password"
              fluid
              toggle-mask
              :feedback="false"
              autocomplete="new-password"
              placeholder="留空不修改"
            />
          </AppFormField>

          <AppFormField label="发件邮箱" required>
            <InputText v-model="settings.fromEmail" type="email" fluid placeholder="noreply@example.com" />
          </AppFormField>

          <AppFormField label="发件人名称">
            <InputText v-model="settings.fromName" fluid placeholder="kapOJ" />
          </AppFormField>

          <AppFormField label="Reply-To">
            <InputText v-model="settings.replyTo" type="email" fluid placeholder="support@example.com" />
          </AppFormField>

          <AppFormField label="站点公开地址" hint="用于拼接验证/重置链接，只填根地址" required>
            <InputText
              v-model="settings.publicBaseUrl"
              fluid
              placeholder="https://oj.example.com"
            />
          </AppFormField>
        </div>
      </AppSurface>

      <AppSurface>
        <h2 class="section-title">开关与测试</h2>
        <div class="switch-row">
          <div>
            <div class="switch-title">启用邮件发送</div>
            <p class="switch-hint">关闭后不会发送验证或重置邮件。</p>
          </div>
          <ToggleSwitch v-model="settings.sendingEnabled" />
        </div>

        <div class="switch-row">
          <div>
            <div class="switch-title">强制全站邮箱验证</div>
            <p class="switch-hint">
              开启后未验证用户无法登录；必须先把邮件服务配置到“可发送”。
            </p>
          </div>
          <ToggleSwitch v-model="settings.requireEmailVerification" />
        </div>

        <div class="actions">
          <Button label="保存" icon="pi pi-check" :loading="saving" @click="save" />
        </div>

        <div class="test-row">
          <InputText
            v-model="testTo"
            type="email"
            placeholder="收件人邮箱"
            aria-label="测试收件人邮箱"
          />
          <Button
            label="发送测试"
            icon="pi pi-send"
            :loading="testing"
            :disabled="!ready"
            @click="sendTest"
          />
        </div>
        <p class="switch-hint">测试发送使用已保存的配置，修改后请先保存。</p>
      </AppSurface>
    </template>
  </section>
</template>

<style scoped>
.admin-mail {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
}

.settings-banner {
  margin-bottom: 0;
}

.status-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
}

.status-label {
  color: var(--kapoj-text-secondary);
  font-size: 13px;
}

.status-hint,
.switch-hint {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 13px;
}

.section-title {
  margin: 0 0 var(--kapoj-space-4);
  font-size: 16px;
}

.settings-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--kapoj-space-4);
}

.switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-4);
  padding: var(--kapoj-space-3) 0;
  border-bottom: 1px solid var(--kapoj-border);
}

.switch-row:first-of-type {
  padding-top: 0;
}

.switch-title {
  font-size: 14px;
  font-weight: 500;
}

.actions {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--kapoj-space-4);
}

.test-row {
  display: flex;
  gap: var(--kapoj-space-2);
  margin-top: var(--kapoj-space-4);
}

.test-row :deep(input) {
  min-width: 240px;
}

@media (max-width: 720px) {
  .settings-grid {
    grid-template-columns: 1fr;
  }

  .test-row {
    flex-direction: column;
  }
}
</style>
