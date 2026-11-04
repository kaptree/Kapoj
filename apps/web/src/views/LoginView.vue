<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { FormInstance } from 'element-plus'

import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { safeRedirect } from '@/router/guard'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const formRef = ref<FormInstance>()
const form = reactive({ account: '', password: '' })
const submitting = ref(false)
const error = ref('')

async function submit() {
  if (!(await formRef.value?.validate().catch(() => false))) return
  submitting.value = true
  error.value = ''
  try {
    await auth.login(form)
    await router.replace(safeRedirect(route.query.redirect))
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '登录失败，请稍后再试'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <el-card class="auth-card" shadow="never">
    <h2>登录</h2>
    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" class="alert" />
    <el-form ref="formRef" :model="form" label-position="top" @submit.prevent="submit">
      <el-form-item
        label="用户名或邮箱"
        prop="account"
        :rules="[{ required: true, message: '请输入用户名或邮箱', trigger: 'blur' }]"
      >
        <el-input v-model="form.account" autocomplete="username" />
      </el-form-item>
      <el-form-item
        label="密码"
        prop="password"
        :rules="[{ required: true, message: '请输入密码', trigger: 'blur' }]"
      >
        <el-input v-model="form.password" type="password" show-password autocomplete="current-password" />
      </el-form-item>
      <el-button type="primary" native-type="submit" :loading="submitting" class="submit">登录</el-button>
    </el-form>
    <p class="switch">还没有账号？<RouterLink to="/register">注册</RouterLink></p>
  </el-card>
</template>

<style scoped>
.auth-card {
  max-width: 400px;
  margin: 40px auto;
}
.alert {
  margin-bottom: 16px;
}
.submit {
  width: 100%;
}
.switch {
  text-align: center;
  color: var(--el-text-color-secondary);
}
</style>
