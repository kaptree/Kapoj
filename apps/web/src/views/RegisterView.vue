<script setup lang="ts">
import { reactive, ref } from 'vue'
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

async function submit() {
  if (!validateAll()) return

  submitting.value = true
  error.value = ''
  try {
    await auth.register(form)
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
</script>

<template>
  <AuthLayout title="注册" description="创建账号，开始记录你的每一次进步。">
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
</style>
