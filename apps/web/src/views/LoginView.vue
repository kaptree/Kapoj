<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'

import { ApiError } from '@/api/http'
import AuthLayout from '@/components/auth/AuthLayout.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import { safeRedirect } from '@/router/guard'
import { useAuthStore } from '@/stores/auth'
import { validateLoginAccount, validateLoginPassword } from '@/validation'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const form = reactive({ account: '', password: '' })
const errors = reactive({ account: '', password: '' })
const submitting = ref(false)
const error = ref('')
const unverified = ref(false)
const unverifiedEmail = ref('')

const validators = {
  account: validateLoginAccount,
  password: validateLoginPassword,
}

function validateField(field: keyof typeof errors) {
  errors[field] = validators[field](form[field]) ?? ''
}

function clearError(field: keyof typeof errors) {
  errors[field] = ''
}

function validateAll() {
  validateField('account')
  validateField('password')
  return !errors.account && !errors.password
}

async function submit() {
  if (!validateAll()) return

  submitting.value = true
  error.value = ''
  unverified.value = false
  unverifiedEmail.value = ''
  try {
    await auth.login(form)
    await router.replace(safeRedirect(route.query.redirect))
  } catch (e) {
    if (e instanceof ApiError && e.code === 40303) {
      unverified.value = true
      unverifiedEmail.value = form.account.includes('@') ? form.account.trim() : ''
    }
    error.value = e instanceof ApiError ? e.message : '登录失败，请稍后再试'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <AuthLayout title="登录" description="使用用户名或邮箱继续你的训练。">
    <Message v-if="error" severity="error" :closable="false" class="form-banner" role="alert">
      {{ error }}
    </Message>

    <Message v-if="unverified" severity="warn" :closable="false" class="form-banner" role="status">
      邮箱尚未验证。
      <RouterLink
        :to="{ name: 'verify-email', query: unverifiedEmail ? { email: unverifiedEmail } : {} }"
      >
        去验证邮箱
      </RouterLink>
    </Message>

    <form class="auth-form" novalidate @submit.prevent="submit">
      <AppFormField
        v-slot="{ id, invalid, describedby }"
        label="用户名或邮箱"
        :error="errors.account"
        required
      >
        <InputText
          :id="id"
          v-model="form.account"
          fluid
          :invalid="invalid"
          :aria-describedby="describedby"
          autocomplete="username"
          placeholder="alice 或 alice@example.com"
          @blur="validateField('account')"
          @update:model-value="clearError('account')"
        />
      </AppFormField>

      <AppFormField
        v-slot="{ id, invalid, describedby }"
        label="密码"
        :error="errors.password"
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
            autocomplete: 'current-password',
            'aria-describedby': describedby,
          }"
          placeholder="输入密码"
          @blur="validateField('password')"
          @update:model-value="clearError('password')"
        />
      </AppFormField>

      <Button
        type="submit"
        label="登录"
        icon="pi pi-arrow-right"
        icon-pos="right"
        :loading="submitting"
        fluid
        class="auth-submit"
      />

      <div class="auth-links">
        <RouterLink to="/forgot-password">忘记密码？</RouterLink>
      </div>
    </form>

    <template #footer> 还没有账号？<RouterLink to="/register">创建账号</RouterLink> </template>
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

.auth-links {
  display: flex;
  justify-content: flex-end;
  font-size: 13px;
}
</style>
