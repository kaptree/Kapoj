<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Password from 'primevue/password'

import * as authApi from '@/api/auth'
import { ApiError } from '@/api/http'
import AuthLayout from '@/components/auth/AuthLayout.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import { validatePassword } from '@/validation'

const route = useRoute()

const token = computed(() => (typeof route.query.token === 'string' ? route.query.token : ''))
const form = reactive({ newPassword: '', confirmPassword: '' })
const errors = reactive({ newPassword: '', confirmPassword: '' })
const error = ref('')
const submitting = ref(false)
const done = ref(false)
const invalidToken = ref(false)

function validate(): boolean {
  errors.newPassword = validatePassword(form.newPassword) ?? ''
  errors.confirmPassword =
    form.confirmPassword === ''
      ? '请再次输入密码'
      : form.confirmPassword === form.newPassword
        ? ''
        : '两次输入的密码不一致'
  return !errors.newPassword && !errors.confirmPassword
}

async function submit() {
  if (!token.value || !validate()) return
  submitting.value = true
  error.value = ''
  invalidToken.value = false
  try {
    await authApi.resetPassword({ token: token.value, newPassword: form.newPassword })
    done.value = true
  } catch (e) {
    if (e instanceof ApiError && e.code === 40003) {
      invalidToken.value = true
    }
    error.value = e instanceof ApiError ? e.message : '重置失败，请稍后再试'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <AuthLayout title="重置密码" description="设置一个新密码，完成后使用新密码登录。">
    <Message v-if="error" severity="error" :closable="false" class="form-banner" role="alert">
      {{ error }}
    </Message>

    <div v-if="invalidToken || !token" class="invalid-panel">
      <p>链接无效或已过期。</p>
      <p class="hint">请重新申请密码重置邮件。</p>
      <RouterLink to="/forgot-password">重新申请重置链接</RouterLink>
    </div>

    <div v-else-if="done" class="done-panel">
      <Message severity="success" :closable="false" class="form-banner" role="status">
        密码已重置，请使用新密码登录。
      </Message>
      <RouterLink to="/login">去登录</RouterLink>
    </div>

    <form v-else class="auth-form" novalidate @submit.prevent="submit">
      <AppFormField
        v-slot="{ id, invalid, describedby }"
        label="新密码"
        :error="errors.newPassword"
        hint="8–72 个字节"
        required
      >
        <Password
          v-model="form.newPassword"
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
        />
      </AppFormField>

      <AppFormField
        v-slot="{ id, invalid, describedby }"
        label="确认新密码"
        :error="errors.confirmPassword"
        required
      >
        <Password
          v-model="form.confirmPassword"
          :input-id="id"
          fluid
          toggle-mask
          :feedback="false"
          :invalid="invalid"
          :input-props="{
            autocomplete: 'new-password',
            'aria-describedby': describedby,
          }"
          placeholder="再次输入新密码"
        />
      </AppFormField>

      <Button
        type="submit"
        label="重置密码"
        icon="pi pi-check"
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

.invalid-panel,
.done-panel {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-size: 14px;
}

.invalid-panel p,
.done-panel p {
  margin: 0;
}

.hint {
  color: var(--kapoj-text-muted);
  font-size: 13px;
}
</style>
