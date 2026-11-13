<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import { useToast } from 'primevue/usetoast'

import * as authApi from '@/api/auth'
import { ApiError } from '@/api/http'
import AuthLayout from '@/components/auth/AuthLayout.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import { useAuthStore } from '@/stores/auth'
import { validateEmail } from '@/validation'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const toast = useToast()

const token = computed(() => (typeof route.query.token === 'string' ? route.query.token : ''))
const email = ref(typeof route.query.email === 'string' ? route.query.email : '')
const emailError = ref('')
const error = ref('')
const verifying = ref(false)
const resending = ref(false)
const resent = ref(false)
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

async function verify() {
  if (!token.value) return
  verifying.value = true
  error.value = ''
  try {
    const session = await authApi.verifyEmail(token.value)
    auth.setSession(session)
    toast.add({ severity: 'success', summary: '邮箱验证成功', life: 3000 })
    await router.replace({ name: 'home' })
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '验证链接无效或已过期'
  } finally {
    verifying.value = false
  }
}

async function resend() {
  emailError.value = validateEmail(email.value) ?? ''
  if (emailError.value) return
  resending.value = true
  error.value = ''
  try {
    await authApi.resendVerification(email.value)
    resent.value = true
    startCooldown()
    toast.add({ severity: 'success', summary: '验证邮件已发送', life: 3000 })
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '发送失败，请稍后再试'
  } finally {
    resending.value = false
  }
}

onMounted(() => {
  if (token.value) void verify()
})
</script>

<template>
  <AuthLayout title="验证邮箱" description="完成邮箱验证后即可登录 kapOJ。">
    <Message v-if="error" severity="error" :closable="false" class="form-banner" role="alert">
      {{ error }}
    </Message>

    <div v-if="verifying" class="verify-status" role="status">正在验证邮箱，请稍候…</div>

    <template v-else>
      <Message v-if="resent" severity="success" :closable="false" class="form-banner" role="status">
        如果该邮箱已注册且尚未验证，验证邮件已发送。
      </Message>

      <form class="auth-form" novalidate @submit.prevent="resend">
        <AppFormField
          v-slot="{ id, invalid, describedby }"
          label="邮箱"
          :error="emailError"
          required
        >
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
          label="重新发送验证邮件"
          icon="pi pi-envelope"
          :loading="resending"
          :disabled="cooldown > 0"
          fluid
        />
        <p v-if="cooldown > 0" class="cooldown">{{ cooldown }} 秒后可重新发送</p>
      </form>
    </template>

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

.verify-status {
  padding: var(--kapoj-space-6) 0;
  color: var(--kapoj-text-secondary);
  text-align: center;
}

.cooldown {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 13px;
  text-align: center;
}
</style>
