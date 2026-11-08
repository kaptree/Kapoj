<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Textarea from 'primevue/textarea'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import { deleteAvatar, uploadAvatar } from '@/api/users'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useAuthStore } from '@/stores/auth'
import { validateAvatarUrl, validateBio, validateProfileNickname } from '@/validation'

const MAX_AVATAR_BYTES = 2 * 1024 * 1024
const IMAGE_MIME_TYPES = new Set(['image/png', 'image/jpeg', 'image/webp', 'image/gif'])
const IMAGE_EXT_RE = /\.(png|jpe?g|webp|gif)$/i

const auth = useAuthStore()
const toast = useToast()

const fields = ref({ nickname: '', bio: '', avatarUrl: '' })
const errors = ref({ nickname: '', bio: '', avatarUrl: '' })
const loading = ref(true)
const saving = ref(false)
const saveError = ref('')
const refreshError = ref('')

const fileInput = ref<HTMLInputElement | null>(null)
const pendingFile = ref<File | null>(null)
const pendingPreview = ref('')
const uploading = ref(false)
const removing = ref(false)

const currentUser = computed(() => auth.user)

const avatarUploaded = computed(() => auth.user?.avatarUploaded === true)

const canRemove = computed(() => avatarUploaded.value || fields.value.avatarUrl.trim() !== '')

const avatarPlaceholder = '例如 https://example.com/avatar.png'

const avatarHint = computed(() =>
  avatarUploaded.value
    ? '填写外链会替换已上传头像；留空则保留上传头像。'
    : '填写图片外链，或在上方上传头像图片。',
)

const avatarPreview = computed(() => {
  if (pendingPreview.value) return pendingPreview.value
  const external = fields.value.avatarUrl.trim()
  if (external && validateAvatarUrl(external) === null) return external
  return auth.user?.avatarUrl || undefined
})

const bioCount = computed(() => Array.from(fields.value.bio).length)

const validators = {
  nickname: validateProfileNickname,
  bio: validateBio,
  avatarUrl: validateAvatarUrl,
}

function validateField(field: keyof typeof errors.value) {
  errors.value[field] = validators[field](fields.value[field]) ?? ''
}

function clearError(field: keyof typeof errors.value) {
  errors.value[field] = ''
  saveError.value = ''
}

function validateAll() {
  validateField('nickname')
  validateField('bio')
  validateField('avatarUrl')
  return !errors.value.nickname && !errors.value.bio && !errors.value.avatarUrl
}

function revokePendingPreview() {
  if (pendingPreview.value) {
    URL.revokeObjectURL(pendingPreview.value)
    pendingPreview.value = ''
  }
  pendingFile.value = null
}

onMounted(async () => {
  try {
    await auth.fetchMe()
  } catch {
    refreshError.value = '无法刷新最新资料，正在使用本地缓存。'
  } finally {
    if (auth.user) {
      fields.value.nickname = auth.user.nickname
      fields.value.bio = auth.user.bio
      // 已上传头像的内部 URL 不应填入外链输入，预览回落到 auth.user.avatarUrl。
      fields.value.avatarUrl = auth.user.avatarUploaded ? '' : auth.user.avatarUrl
    }
    loading.value = false
  }
})

onBeforeUnmount(revokePendingPreview)

function chooseImage() {
  fileInput.value?.click()
}

function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const picked = input.files?.[0] ?? null
  if (!picked) return
  if (picked.size > MAX_AVATAR_BYTES) {
    toast.add({ severity: 'warn', summary: '头像不能超过 2 MiB', life: 3000 })
    input.value = ''
    return
  }
  if (!IMAGE_MIME_TYPES.has(picked.type) && !IMAGE_EXT_RE.test(picked.name)) {
    toast.add({ severity: 'warn', summary: '仅支持 PNG、JPEG、WebP 或 GIF 图片', life: 3000 })
    input.value = ''
    return
  }
  revokePendingPreview()
  pendingFile.value = picked
  pendingPreview.value = URL.createObjectURL(picked)
  input.value = ''
}

async function upload() {
  const file = pendingFile.value
  if (!file) {
    toast.add({ severity: 'warn', summary: '请先选择头像图片', life: 3000 })
    return
  }
  uploading.value = true
  try {
    const me = await uploadAvatar(file)
    auth.setUser(me)
    revokePendingPreview()
    fields.value.avatarUrl = ''
    toast.add({ severity: 'success', summary: '头像已上传', life: 3000 })
  } catch (e) {
    const message = e instanceof ApiError ? e.message : '上传失败，请稍后再试'
    toast.add({ severity: 'error', summary: '上传失败', detail: message, life: 4000 })
  } finally {
    uploading.value = false
  }
}

async function removeAvatar() {
  removing.value = true
  try {
    const me = await deleteAvatar()
    auth.setUser(me)
    revokePendingPreview()
    fields.value.avatarUrl = ''
    toast.add({ severity: 'success', summary: '头像已移除', life: 3000 })
  } catch (e) {
    const message = e instanceof ApiError ? e.message : '移除失败，请稍后再试'
    toast.add({ severity: 'error', summary: '移除失败', detail: message, life: 4000 })
  } finally {
    removing.value = false
  }
}

async function save() {
  if (!validateAll()) return

  saving.value = true
  saveError.value = ''
  try {
    await auth.updateProfile(fields.value)
    toast.add({
      severity: 'success',
      summary: '资料已保存',
      detail: '公开资料已更新',
      life: 3000,
    })
  } catch (e) {
    const message = e instanceof ApiError ? e.message : '保存失败，请稍后再试'
    saveError.value = message
    toast.add({
      severity: 'error',
      summary: '保存失败',
      detail: message,
      life: 4000,
    })
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="profile-page">
    <AppPageHeader
      eyebrow="ACCOUNT // PROFILE"
      title="编辑资料"
      description="更新公开资料与头像。"
    />

    <AppSurface v-if="loading" padding="lg">
      <AppLoadingState label="正在加载资料" />
    </AppSurface>

    <div v-else class="profile-grid">
      <AppSurface as="section" tone="raised" padding="lg" class="profile-form-card">
        <Message
          v-if="refreshError"
          severity="warn"
          :closable="false"
          class="form-banner"
          role="status"
        >
          {{ refreshError }}
        </Message>
        <Message
          v-if="saveError"
          severity="error"
          :closable="false"
          class="form-banner"
          role="alert"
        >
          {{ saveError }}
        </Message>

        <form class="profile-form" novalidate @submit.prevent="save">
          <AppFormField
            v-slot="{ id, invalid, describedby }"
            label="昵称"
            :error="errors.nickname"
            required
          >
            <InputText
              :id="id"
              v-model="fields.nickname"
              fluid
              :invalid="invalid"
              :aria-describedby="describedby"
              @blur="validateField('nickname')"
              @update:model-value="clearError('nickname')"
            />
          </AppFormField>

          <AppFormField
            v-slot="{ id, invalid, describedby }"
            label="个人简介"
            :error="errors.bio"
            :hint="`${bioCount}/200 字符`"
          >
            <Textarea
              :id="id"
              v-model="fields.bio"
              fluid
              rows="5"
              maxlength="200"
              :invalid="invalid"
              :aria-describedby="describedby"
              placeholder="介绍一下你自己，最多 200 个字符。"
              @blur="validateField('bio')"
              @update:model-value="clearError('bio')"
            />
          </AppFormField>

          <AppFormField
            v-slot="{ id, invalid, describedby }"
            label="头像链接"
            :error="errors.avatarUrl"
            :hint="avatarHint"
          >
            <InputText
              :id="id"
              v-model="fields.avatarUrl"
              fluid
              :invalid="invalid"
              :aria-describedby="describedby"
              :placeholder="avatarPlaceholder"
              autocomplete="url"
              @blur="validateField('avatarUrl')"
              @update:model-value="clearError('avatarUrl')"
            />
          </AppFormField>

          <div class="avatar-upload">
            <input
              ref="fileInput"
              type="file"
              class="avatar-file-input"
              accept="image/png,image/jpeg,image/webp,image/gif"
              aria-label="选择头像图片"
              @change="onFileChange"
            />
            <Button
              type="button"
              label="选择图片"
              icon="pi pi-image"
              severity="secondary"
              outlined
              @click="chooseImage"
            />
            <Button
              type="button"
              label="上传头像"
              icon="pi pi-upload"
              :loading="uploading"
              @click="upload"
            />
            <Button
              v-if="canRemove"
              type="button"
              label="移除头像"
              icon="pi pi-trash"
              severity="danger"
              outlined
              :loading="removing"
              :disabled="uploading"
              @click="removeAvatar"
            />
          </div>

          <Button
            type="submit"
            label="保存资料"
            icon="pi pi-check"
            :loading="saving"
            class="profile-submit"
          />
        </form>
      </AppSurface>

      <AppSurface as="aside" padding="lg" class="profile-preview">
        <p class="preview-kicker">PROFILE PREVIEW</p>
        <Avatar
          :image="avatarPreview"
          :label="fields.nickname.slice(0, 1) || '?'"
          shape="square"
          size="xlarge"
          class="preview-avatar"
        />
        <h2 class="preview-name">{{ fields.nickname || '未命名' }}</h2>
        <p class="preview-username">@{{ currentUser?.username }}</p>
        <p class="preview-bio">{{ fields.bio || '这个人还没有写简介。' }}</p>
        <dl class="preview-meta">
          <div>
            <dt>Rating</dt>
            <dd class="num">{{ currentUser?.rating ?? '-' }}</dd>
          </div>
          <div>
            <dt>金币</dt>
            <dd class="num">{{ currentUser?.coins ?? '-' }}</dd>
          </div>
        </dl>
      </AppSurface>
    </div>
  </div>
</template>

<style scoped>
.profile-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
  max-width: 1040px;
}

.profile-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 320px;
  gap: var(--kapoj-space-5);
  align-items: start;
}

.profile-form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.form-banner {
  margin-bottom: var(--kapoj-space-4);
}

.avatar-upload {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
}

.avatar-file-input {
  display: none;
}

.profile-submit {
  align-self: flex-start;
  margin-top: var(--kapoj-space-1);
}

.profile-preview {
  position: sticky;
  top: calc(var(--kapoj-header-height) + var(--kapoj-space-4));
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--kapoj-space-2);
}

.preview-kicker {
  margin: 0;
  color: var(--kapoj-primary);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.14em;
}

.preview-avatar {
  width: 88px;
  height: 88px;
  margin: var(--kapoj-space-2) 0;
  color: var(--kapoj-primary);
  font-size: 28px;
  font-weight: 700;
  background: var(--kapoj-primary-soft);
  border: 1px solid var(--kapoj-border-strong);
  border-radius: var(--kapoj-radius-md);
}

.preview-name {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.25rem;
  line-height: 1.2;
}

.preview-username {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
}

.preview-bio {
  margin: var(--kapoj-space-2) 0 0;
  color: var(--kapoj-text-secondary);
  font-size: 13px;
}

.preview-meta {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--kapoj-space-3);
  width: 100%;
  padding-top: var(--kapoj-space-3);
  margin: var(--kapoj-space-3) 0 0;
  border-top: 1px solid var(--kapoj-border);
}

.preview-meta div {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.preview-meta dt {
  color: var(--kapoj-text-muted);
  font-size: 10px;
  text-transform: uppercase;
}

.preview-meta dd {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 14px;
  font-weight: 600;
}

@media (max-width: 900px) {
  .profile-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .profile-preview {
    position: static;
  }

  .profile-submit {
    width: 100%;
  }
}
</style>
