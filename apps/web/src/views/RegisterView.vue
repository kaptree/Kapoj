<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, type FormInstance } from 'element-plus'

import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { emailRules, nicknameRules, passwordRules, usernameRules } from '@/validation'

const auth = useAuthStore()
const router = useRouter()

const formRef = ref<FormInstance>()
const form = reactive({ username: '', email: '', password: '', nickname: '' })
const submitting = ref(false)
const error = ref('')

async function submit() {
  if (!(await formRef.value?.validate().catch(() => false))) return
  submitting.value = true
  error.value = ''
  try {
    await auth.register(form)
    ElMessage.success('注册成功')
    await router.replace({ name: 'home' })
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '注册失败，请稍后再试'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <el-card class="auth-card" shadow="never">
    <h2>注册</h2>
    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" class="alert" />
    <el-form ref="formRef" :model="form" label-position="top" @submit.prevent="submit">
      <el-form-item label="用户名" prop="username" :rules="usernameRules">
        <el-input v-model="form.username" autocomplete="username" />
      </el-form-item>
      <el-form-item label="邮箱" prop="email" :rules="emailRules">
        <el-input v-model="form.email" autocomplete="email" />
      </el-form-item>
      <el-form-item label="密码" prop="password" :rules="passwordRules">
        <el-input
          v-model="form.password"
          type="password"
          show-password
          autocomplete="new-password"
        />
      </el-form-item>
      <el-form-item label="昵称（可选，默认与用户名相同）" prop="nickname" :rules="nicknameRules">
        <el-input v-model="form.nickname" />
      </el-form-item>
      <el-button type="primary" native-type="submit" :loading="submitting" class="submit"
        >注册</el-button
      >
    </el-form>
    <p class="switch">已有账号？<RouterLink to="/login">登录</RouterLink></p>
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
