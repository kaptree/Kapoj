<script setup lang="ts">
import { onBeforeUnmount, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import AuthLayout from '@/components/auth/AuthLayout.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import { useAuthStore } from '@/stores/auth'
import { validateEmail, validateNickname, validatePassword, validateUsername } from '@/validation'

const auth = useAuthStore()
const router = useRouter()
const toast = useToast()

const form = reactive({ username: '', email: '', password: '', nickname: '' })
const errors = reactive({ username: '', email: '', password: '', nickname: '' })
const submitting = ref(false)
const error = ref('')

const verification = ref<{ email: string; sent: boolean } | null>(null)
const resendCooldown = ref(0)
const resending = ref(false)
let cooldownTimer: number | undefined

const validators = {
  username: validateUsername,
  email: validateEmail,
  password: validatePassword,
  nickname: validateNickname,
}

function validateField(field: keyof typeof errors) {
  errors[field] = validators[field](form[field]) ?? ''
}

function clearError(field: keyof typeof errors) {
  errors[field] = ''
}

function validateAll() {
  for (const field of Object.keys(errors) as (keyof typeof errors)[]) {
    validateField(field)
  }
  return !errors.username && !errors.email && !errors.password && !errors.nickname
}

function startCooldown() {
  resendCooldown.value = 60
  if (cooldownTimer !== undefined) window.clearInterval(cooldownTimer)
  cooldownTimer = window.setInterval(() => {
    resendCooldown.value -= 1
    if (resendCooldown.value <= 0) {
      resendCooldown.value = 0
      window.clearInterval(cooldownTimer)
      cooldownTimer = undefined
    }
  }, 1000)
}

onBeforeUnmount(() => {
  if (cooldownTimer !== undefined) window.clearInterval(cooldownTimer)
})

async function submit() {
  if (!validateAll()) return

  submitting.value = true
  error.value = ''
  try {
    const result = await auth.register(form)
    if (result.verificationRequired) {
      verification.value = { email: result.email, sent: result.verificationEmailSent }
      startCooldown()
      return
    }
    toast.add({
      severity: 'success',
      summary: '注册成功',
      detail: '欢迎加入 kapOJ',
      life: 3000,
    })
    await router.replace({ name: 'home' })
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '注册失败，请稍后再试'
  } finally {
    submitting.value = false
  }
}

async function resend() {
  if (!verification.value || resendCooldown.value > 0) return
  resending.value = true
  try {
    await auth.resendVerification(verification.value.email)
    verification.value.sent = true
    startCooldown()
    toast.add({
      severity: 'success',
      summary: '验证邮件已重新发送',
      life: 3000,
    })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '发送失败',
      detail: e instanceof ApiError ? e.message : '请稍后再试',
      life: 4000,
    })
  } finally {
    resending.value = false
  }
}
</script>

<template>
  <AuthLayout title="注册" description="创建账号，开始记录你的每一次进步。">
    <template v-if="verification">
      <Message
        :severity="verification.sent ? 'success' : 'warn'"
        :closable="false"
        class="form-banner"
        role="status"
      >
        {{ verification.sent ? '验证邮件已发送' : '验证邮件发送失败' }}
      </Message>
      <div class="verify-panel">
        <p>
          我们已向 <strong>{{ verification.email }}</strong> 发送验证链接。
          请点击邮件中的链接完成验证后再登录。
        </p>
        <p class="verify-hint">如果没有收到，请检查垃圾邮件，或稍后重新发送。</p>
        <Button
          label="重新发送验证邮件"
          icon="pi pi-envelope"
          :loading="resending"
          :disabled="resendCooldown > 0"
          fluid
          @click="resend"
        />
        <p v-if="resendCooldown > 0" class="verify-countdown">
          {{ resendCooldown }} 秒后可重新发送
        </p>
        <RouterLink to="/login" class="verify-login">返回登录</RouterLink>
      </div>
    </template>

    <template v-else>
      <Message v-if="error" severity="error" :closable="false" class="form-banner" role="alert">
        {{ error }}
      </Message>

      <form class="auth-form" novalidate @submit.prevent="submit">
        <AppFormField
          v-slot="{ id, invalid, describedby }"
          label="用户名"
          :error="errors.username"
          hint="3–32 位字母、数字或下划线"
          required
        >
          <InputText
            :id="id"
            v-model="form.username"
            fluid
            :invalid="invalid"
            :aria-describedby="describedby"
            autocomplete="username"
            placeholder="alice_01"
            @blur="validateField('username')"
            @update:model-value="clearError('username')"
          />
        </AppFormField>

        <AppFormField
          v-slot="{ id, invalid, describedby }"
          label="邮箱"
          :error="errors.email"
          required
        >
          <InputText
            :id="id"
            v-model="form.email"
            fluid
            :invalid="invalid"
            :aria-describedby="describedby"
            autocomplete="email"
            placeholder="alice@example.com"
            @blur="validateField('email')"
            @update:model-value="clearError('email')"
          />
        </AppFormField>

        <AppFormField
          v-slot="{ id, invalid, describedby }"
          label="密码"
          :error="errors.password"
          hint="8–72 个字节"
          required
        >
          <Password
            v-model="form.password"
            :input-id="id"
            fluid
            toggle-mask
            :feedback="false"
            :invalid="invalid"
            :input-props="{
              autocomplete: 'new-password',
              'aria-describedby': describedby,
            }"
            placeholder="至少 8 个字节"
            @blur="validateField('password')"
            @update:model-value="clearError('password')"
          />
        </AppFormField>

        <AppFormField
          v-slot="{ id, invalid, describedby }"
          label="昵称"
          :error="errors.nickname"
          hint="可选，默认与用户名相同"
          optional
        >
          <InputText
            :id="id"
            v-model="form.nickname"
            fluid
            :invalid="invalid"
            :aria-describedby="describedby"
            placeholder="你的展示名称"
            @blur="validateField('nickname')"
            @update:model-value="clearError('nickname')"
          />
        </AppFormField>

        <Button
          type="submit"
          label="注册"
          icon="pi pi-arrow-right"
          icon-pos="right"
          :loading="submitting"
          fluid
          class="auth-submit"
        />
      </form>
    </template>

    <template #footer> 已有账号？<RouterLink to="/login">登录</RouterLink> </template>
  </AuthLayout>
</template>

<style scoped>
.form-banner {
  margin-bottom: var(--kapoj-space-4);
}

.auth-form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.auth-submit {
  margin-top: var(--kapoj-space-1);
}

.verify-panel {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-size: 14px;
  line-height: 1.7;
}

.verify-panel p {
  margin: 0;
}

.verify-hint,
.verify-countdown {
  color: var(--kapoj-text-muted);
  font-size: 13px;
}

.verify-login {
  align-self: flex-start;
}
</style>
