<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import {
  createSolutionReply,
  deleteSolutionPost,
  deleteSolutionReply,
  getSolutionPost,
  updateSolutionPost,
  updateSolutionReply,
} from '@/api/discussions'
import { ApiError } from '@/api/http'
import type { SolutionAuthor, SolutionPostDetail, SolutionReplyItem } from '@/api/types'
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import MarkdownView from '@/components/MarkdownView.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useAuthStore } from '@/stores/auth'
import { useTheme } from '@/theme/theme'
import { formatDateTime } from '@/utils/format'

const MAX_TITLE = 100
const MAX_POST = 50000
const MAX_REPLY = 10000

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const toast = useToast()
const confirm = useConfirm()
const { isDark } = useTheme()

const problemId = computed(() => {
  const id = route.params.id
  return typeof id === 'string' ? Number(id) : null
})
const postId = computed(() => {
  const id = route.params.postId
  return typeof id === 'string' ? Number(id) : null
})

const detail = ref<SolutionPostDetail | null>(null)
const loading = ref(false)
const error = ref('')

const replyMd = ref('')
const replyError = ref('')
const replying = ref(false)

const editOpen = ref(false)
const editTitle = ref('')
const editMd = ref('')
const editError = ref('')
const savingPost = ref(false)

const editReplyOpen = ref(false)
const editingReplyId = ref<number | null>(null)
const editReplyMd = ref('')
const editReplyError = ref('')
const savingReply = ref(false)

const isLoggedIn = computed(() => auth.isLoggedIn)
const loginTarget = computed(() => ({ name: 'login', query: { redirect: route.fullPath } }))

function charLen(s: string): number {
  return [...s].length
}

function authorName(author: SolutionAuthor): string {
  return author.nickname || author.username
}

const replyCanSubmit = computed(() => {
  const body = replyMd.value
  return body.trim().length > 0 && charLen(body) <= MAX_REPLY
})

const replyLengthError = computed(() => {
  const body = replyMd.value
  return body.trim().length > 0 && charLen(body) > MAX_REPLY ? '回复不能超过 10000 个字符' : ''
})

const editTitleTrimmed = computed(() => editTitle.value.trim())
const editCanSubmit = computed(() => {
  if (editTitleTrimmed.value.length === 0 || charLen(editTitleTrimmed.value) > MAX_TITLE)
    return false
  const body = editMd.value
  return body.trim().length > 0 && charLen(body) <= MAX_POST
})

const editReplyCanSubmit = computed(() => {
  const body = editReplyMd.value
  return body.trim().length > 0 && charLen(body) <= MAX_REPLY
})

let latest = 0

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  detail.value = null
  if (problemId.value === null || postId.value === null) {
    loading.value = false
    error.value = '帖子不存在'
    return
  }
  try {
    const data = await getSolutionPost(problemId.value, postId.value)
    if (seq !== latest) return
    detail.value = data
  } catch (e) {
    if (seq !== latest) return
    error.value =
      e instanceof ApiError && e.code === 40400
        ? '帖子不存在'
        : e instanceof ApiError
          ? e.message
          : '加载思路详情失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

watch(() => [route.params.id, route.params.postId], load, { immediate: true })

async function submitReply() {
  if (problemId.value === null || postId.value === null) return
  if (!replyCanSubmit.value) return
  replying.value = true
  replyError.value = ''
  try {
    await createSolutionReply(problemId.value, postId.value, { contentMd: replyMd.value })
    replyMd.value = ''
    toast.add({ severity: 'success', summary: '已回复', life: 3000 })
    await load()
  } catch (e) {
    replyError.value = e instanceof ApiError ? e.message : '回复失败，请稍后再试'
  } finally {
    replying.value = false
  }
}

function openEditPost() {
  editTitle.value = detail.value?.title ?? ''
  editMd.value = detail.value?.contentMd ?? ''
  editError.value = ''
  editOpen.value = true
}

async function submitEditPost() {
  if (problemId.value === null || postId.value === null) return
  if (!editCanSubmit.value) return
  savingPost.value = true
  editError.value = ''
  try {
    detail.value = await updateSolutionPost(problemId.value, postId.value, {
      title: editTitleTrimmed.value,
      contentMd: editMd.value,
    })
    editOpen.value = false
    toast.add({ severity: 'success', summary: '已保存', life: 3000 })
  } catch (e) {
    editError.value = e instanceof ApiError ? e.message : '保存失败，请稍后再试'
  } finally {
    savingPost.value = false
  }
}

function deletePost() {
  confirm.require({
    header: '删除思路',
    message: `确定删除「${detail.value?.title ?? ''}」？`,
    icon: 'pi pi-trash',
    acceptLabel: '删除',
    rejectLabel: '取消',
    accept: () => {
      void doDeletePost()
    },
  })
}

async function doDeletePost() {
  if (problemId.value === null || postId.value === null) return
  try {
    await deleteSolutionPost(problemId.value, postId.value)
    toast.add({ severity: 'success', summary: '已删除', life: 3000 })
    await router.push(`/problems/${problemId.value}/solutions`)
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: e instanceof ApiError ? e.message : '删除失败，请稍后再试',
      life: 3000,
    })
  }
}

function openEditReply(reply: SolutionReplyItem) {
  editingReplyId.value = reply.id
  editReplyMd.value = reply.contentMd
  editReplyError.value = ''
  editReplyOpen.value = true
}

async function submitEditReply() {
  if (problemId.value === null || postId.value === null || editingReplyId.value === null) return
  if (!editReplyCanSubmit.value) return
  savingReply.value = true
  editReplyError.value = ''
  try {
    await updateSolutionReply(problemId.value, postId.value, editingReplyId.value, {
      contentMd: editReplyMd.value,
    })
    editReplyOpen.value = false
    toast.add({ severity: 'success', summary: '已保存', life: 3000 })
    await load()
  } catch (e) {
    editReplyError.value = e instanceof ApiError ? e.message : '保存失败，请稍后再试'
  } finally {
    savingReply.value = false
  }
}

function deleteReply(reply: SolutionReplyItem) {
  confirm.require({
    header: '删除回复',
    message: '确定删除这条回复？',
    icon: 'pi pi-trash',
    acceptLabel: '删除',
    rejectLabel: '取消',
    accept: () => {
      void doDeleteReply(reply.id)
    },
  })
}

async function doDeleteReply(replyId: number) {
  if (problemId.value === null || postId.value === null) return
  try {
    await deleteSolutionReply(problemId.value, postId.value, replyId)
    toast.add({ severity: 'success', summary: '已删除', life: 3000 })
    await load()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: e instanceof ApiError ? e.message : '删除失败，请稍后再试',
      life: 3000,
    })
  }
}
</script>

<template>
  <div class="solution-detail-page">
    <AppPageHeader
      eyebrow="PROBLEM // SOLUTIONS"
      title="解题思路"
      :description="`题目 #${problemId} 的解题思路。`"
    >
      <template #actions>
        <Button
          as="router-link"
          :to="`/problems/${problemId}/solutions`"
          label="返回列表"
          icon="pi pi-arrow-left"
          severity="secondary"
          outlined
        />
        <Button v-if="detail?.canEdit" label="编辑思路" icon="pi pi-pencil" @click="openEditPost" />
        <Button
          v-if="detail?.canDelete"
          label="删除思路"
          icon="pi pi-trash"
          severity="danger"
          outlined
          @click="deletePost"
        />
      </template>
    </AppPageHeader>

    <AppSurface v-if="loading && !detail" padding="lg">
      <AppLoadingState label="正在加载思路详情" />
    </AppSurface>

    <AppSurface v-else-if="error && !detail" padding="lg">
      <AppErrorState :description="error" @retry="load" />
    </AppSurface>

    <template v-else-if="detail">
      <AppSurface padding="lg" class="post-card">
        <h1 class="post-title">{{ detail.title }}</h1>
        <p class="post-meta">
          {{ authorName(detail.author) }} · {{ formatDateTime(detail.createdAt) }}
        </p>
        <div class="post-body">
          <MarkdownView :source="detail.contentMd" :theme="isDark ? 'dark' : 'light'" />
        </div>
      </AppSurface>

      <AppSurface padding="lg" class="replies-card">
        <h2 class="section-title">回复（{{ detail.replies.length }}）</h2>
        <AppEmptyState
          v-if="!detail.replies.length"
          icon="pi pi-comments"
          title="暂无回复"
          description="来发布第一条回复吧。"
        />
        <div v-else class="replies-list">
          <div v-for="reply in detail.replies" :key="reply.id" class="reply">
            <div class="reply-head">
              <span class="reply-author">{{ authorName(reply.author) }}</span>
              <span class="reply-time">{{ formatDateTime(reply.createdAt) }}</span>
              <div class="reply-actions">
                <Button
                  v-if="reply.canEdit"
                  label="编辑"
                  size="small"
                  text
                  severity="secondary"
                  @click="openEditReply(reply)"
                />
                <Button
                  v-if="reply.canDelete"
                  label="删除"
                  size="small"
                  text
                  severity="danger"
                  @click="deleteReply(reply)"
                />
              </div>
            </div>
            <div class="reply-body">
              <MarkdownView :source="reply.contentMd" :theme="isDark ? 'dark' : 'light'" />
            </div>
          </div>
        </div>
      </AppSurface>

      <AppSurface padding="lg" class="reply-composer">
        <h2 class="section-title">回复</h2>
        <template v-if="isLoggedIn">
          <MarkdownEditor v-model="replyMd" :theme="isDark ? 'dark' : 'light'" />
          <Message v-if="replyLengthError" severity="error" :closable="false">
            {{ replyLengthError }}
          </Message>
          <Message v-if="replyError" severity="error" :closable="false">{{ replyError }}</Message>
          <div class="composer-actions">
            <Button
              label="发布回复"
              icon="pi pi-send"
              :loading="replying"
              :disabled="!replyCanSubmit"
              class="reply-submit"
              @click="submitReply"
            />
          </div>
        </template>
        <template v-else>
          <AppEmptyState
            icon="pi pi-lock"
            title="登录后回复"
            description="登录后即可分享你的思路。"
          />
          <div class="cta-row">
            <RouterLink :to="loginTarget" class="login-link">
              <Button label="登录 / 注册" icon="pi pi-sign-in" />
            </RouterLink>
          </div>
        </template>
      </AppSurface>
    </template>

    <Dialog
      v-model:visible="editOpen"
      header="编辑思路"
      modal
      :draggable="false"
      :style="{ width: 'min(92vw, 640px)' }"
      @hide="editError = ''"
    >
      <div class="edit-form">
        <label for="edit-title" class="field-label">标题</label>
        <InputText
          id="edit-title"
          v-model="editTitle"
          fluid
          :maxlength="MAX_TITLE"
          @update:model-value="editError = ''"
        />
        <label class="field-label">正文</label>
        <MarkdownEditor v-model="editMd" :theme="isDark ? 'dark' : 'light'" />
        <Message v-if="editError" severity="error" :closable="false">{{ editError }}</Message>
      </div>
      <template #footer>
        <Button label="取消" severity="secondary" text @click="editOpen = false" />
        <Button
          label="保存"
          icon="pi pi-check"
          :loading="savingPost"
          :disabled="!editCanSubmit"
          class="edit-post-submit"
          @click="submitEditPost"
        />
      </template>
    </Dialog>

    <Dialog
      v-model:visible="editReplyOpen"
      header="编辑回复"
      modal
      :draggable="false"
      :style="{ width: 'min(92vw, 640px)' }"
      @hide="editReplyError = ''"
    >
      <div class="edit-form">
        <label class="field-label">回复</label>
        <MarkdownEditor v-model="editReplyMd" :theme="isDark ? 'dark' : 'light'" />
        <Message v-if="editReplyError" severity="error" :closable="false">
          {{ editReplyError }}
        </Message>
      </div>
      <template #footer>
        <Button label="取消" severity="secondary" text @click="editReplyOpen = false" />
        <Button
          label="保存"
          icon="pi pi-check"
          :loading="savingReply"
          :disabled="!editReplyCanSubmit"
          class="edit-reply-submit"
          @click="submitEditReply"
        />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.solution-detail-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.post-card,
.replies-card,
.reply-composer {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.post-title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.25rem;
  line-height: 1.3;
  word-break: break-word;
}

.post-meta {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.post-body {
  padding-top: var(--kapoj-space-3);
  border-top: 1px solid var(--kapoj-border);
}

.section-title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  font-weight: 600;
}

.replies-list {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.reply {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  padding: var(--kapoj-space-3);
  background: var(--kapoj-bg);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.reply-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.reply-author {
  color: var(--kapoj-text);
  font-size: 0.875rem;
  font-weight: 600;
}

.reply-time {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.reply-actions {
  display: flex;
  gap: var(--kapoj-space-1);
  margin-left: auto;
}

.reply-body {
  min-width: 0;
}

.composer-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--kapoj-space-2);
  margin-top: var(--kapoj-space-1);
}

.cta-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
}

.edit-form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
}

.field-label {
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
  font-weight: 600;
}

@media (max-width: 640px) {
  .reply-actions {
    width: 100%;
    margin-left: 0;
  }
}
</style>
