<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'

import * as authApi from '@/api/auth'
import { ApiError } from '@/api/http'
import AuthLayout from '@/components/auth/AuthLayout.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import { validateEmail } from '@/validation'

const email = ref('')
const emailError = ref('')
const error = ref('')
const submitting = ref(false)
const sent = ref(false)
const cooldown = ref(0)
let cooldownTimer: number | undefined

function startCooldown() {
  cooldown.value = 60
  if (cooldownTimer !== undefined) window.clearInterval(cooldownTimer)
  cooldownTimer = window.setInterval(() => {
    cooldown.value -= 1
    if (cooldown.value <= 0) {
      cooldown.value = 0
      window.clearInterval(cooldownTimer)
      cooldownTimer = undefined
    }
  }, 1000)
}

onBeforeUnmount(() => {
  if (cooldownTimer !== undefined) window.clearInterval(cooldownTimer)
})

async function submit() {
  emailError.value = validateEmail(email.value) ?? ''
  if (emailError.value) return
  submitting.value = true
  error.value = ''
  try {
    await authApi.forgotPassword(email.value)
    sent.value = true
    startCooldown()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '发送失败，请稍后再试'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <AuthLayout title="找回密码" description="输入注册邮箱，我们会发送重置链接。">
    <Message v-if="error" severity="error" :closable="false" class="form-banner" role="alert">
      {{ error }}
    </Message>

    <template v-if="sent">
      <Message severity="success" :closable="false" class="form-banner" role="status">
        如果该邮箱已注册，重置邮件已发送。请检查收件箱和垃圾邮件。
      </Message>
      <p class="hint">链接有效期 30 分钟，过期后可以重新申请。</p>
      <Button
        label="重新发送"
        icon="pi pi-envelope"
        :disabled="cooldown > 0"
        fluid
        @click="sent = false"
      />
      <p v-if="cooldown > 0" class="cooldown">{{ cooldown }} 秒后可重新发送</p>
    </template>

    <form v-else class="auth-form" novalidate @submit.prevent="submit">
      <AppFormField v-slot="{ id, invalid, describedby }" label="邮箱" :error="emailError" required>
        <InputText
          :id="id"
          v-model="email"
          type="email"
          fluid
          :invalid="invalid"
          :aria-describedby="describedby"
          autocomplete="email"
          placeholder="alice@example.com"
        />
      </AppFormField>

      <Button
        type="submit"
        label="发送重置邮件"
        icon="pi pi-envelope"
        :loading="submitting"
        fluid
      />
    </form>

    <template #footer>
      <RouterLink to="/login">返回登录</RouterLink>
    </template>
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

.hint,
.cooldown {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 13px;
  text-align: center;
}
</style>
