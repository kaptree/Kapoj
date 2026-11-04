<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, type FormInstance, type FormItemRule } from 'element-plus'

import { ApiError } from '@/api/http'
import { updateMe } from '@/api/users'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const formRef = ref<FormInstance>()
const fields = reactive({ nickname: '', bio: '', avatarUrl: '' })
const saving = ref(false)

// Bound from script: a literal "https://" placeholder trips vue-tsc 3.3.
const avatarPlaceholder = '例如 https://example.com/avatar.png'

const avatarRules: FormItemRule[] = [
  { type: 'url', message: '请输入 http 或 https 链接', trigger: 'blur' },
]

onMounted(async () => {
  await auth.fetchMe().catch(() => undefined)
  if (auth.user) {
    fields.nickname = auth.user.nickname
    fields.bio = auth.user.bio
    fields.avatarUrl = auth.user.avatarUrl
  }
})

async function save() {
  if (!(await formRef.value?.validate().catch(() => false))) return
  saving.value = true
  try {
    auth.setUser(await updateMe(fields))
    ElMessage.success('资料已保存')
  } catch (e) {
    ElMessage.error(e instanceof ApiError ? e.message : '保存失败，请稍后再试')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <el-card shadow="never" class="card">
    <h2>编辑资料</h2>
    <el-form ref="formRef" :model="fields" label-position="top" @submit.prevent="save">
      <el-form-item
        label="昵称"
        prop="nickname"
        :rules="[
          { required: true, whitespace: true, message: '请输入昵称', trigger: 'blur' },
          { max: 32, message: '昵称不能超过 32 个字符', trigger: 'blur' },
        ]"
      >
        <el-input v-model="fields.nickname" />
      </el-form-item>
      <el-form-item label="个人简介" prop="bio">
        <el-input v-model="fields.bio" type="textarea" :rows="4" maxlength="200" show-word-limit />
      </el-form-item>
      <el-form-item label="头像链接" prop="avatarUrl" :rules="avatarRules">
        <el-input v-model="fields.avatarUrl" :placeholder="avatarPlaceholder" />
        <div class="hint">头像上传功能将在后续版本提供，目前请填写图片链接。</div>
      </el-form-item>
      <el-button type="primary" native-type="submit" :loading="saving">保存资料</el-button>
    </el-form>
  </el-card>
</template>

<style scoped>
.card {
  max-width: 560px;
}
.hint {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
