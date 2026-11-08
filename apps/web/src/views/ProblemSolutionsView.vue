<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Paginator from 'primevue/paginator'
import { useToast } from 'primevue/usetoast'

import { createSolutionPost, listSolutionPosts } from '@/api/discussions'
import { ApiError } from '@/api/http'
import { likeSolutionPost, unlikeSolutionPost } from '@/api/social'
import type { SolutionAuthor, SolutionPostItem } from '@/api/types'
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useAuthStore } from '@/stores/auth'
import { useTheme } from '@/theme/theme'
import { formatDateTime } from '@/utils/format'

const PAGE_SIZE = 20
const MAX_TITLE = 100
const MAX_POST = 50000

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const toast = useToast()
const { isDark } = useTheme()

const problemId = computed(() => {
  const id = route.params.id
  return typeof id === 'string' ? Number(id) : null
})

const posts = ref<SolutionPostItem[]>([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const error = ref('')

const publishOpen = ref(false)
const publishTitle = ref('')
const publishMd = ref('')
const publishing = ref(false)
const publishError = ref('')

const isLoggedIn = computed(() => auth.isLoggedIn)
const loginTarget = computed(() => ({ name: 'login', query: { redirect: route.fullPath } }))
const first = computed(() => (page.value - 1) * PAGE_SIZE)

function charLen(s: string): number {
  return [...s].length
}

const publishTitleTrimmed = computed(() => publishTitle.value.trim())

const publishLengthError = computed(() => {
  const t = publishTitleTrimmed.value
  if (t.length > 0 && charLen(t) > MAX_TITLE) return '标题长度需为 1–100 个字符'
  if (publishMd.value.trim().length > 0 && charLen(publishMd.value) > MAX_POST)
    return '帖子正文不能超过 50000 个字符'
  return ''
})

const publishCanSubmit = computed(() => {
  if (publishTitleTrimmed.value.length === 0 || charLen(publishTitleTrimmed.value) > MAX_TITLE)
    return false
  const body = publishMd.value
  return body.trim().length > 0 && charLen(body) <= MAX_POST
})

function authorName(author: SolutionAuthor): string {
  return author.nickname || author.username
}

async function toggleLike(post: SolutionPostItem) {
  if (problemId.value === null) return
  if (!isLoggedIn.value) {
    void router.push(loginTarget.value)
    return
  }
  const prevLiked = post.likedByMe
  const prevCount = post.likeCount
  post.likedByMe = !prevLiked
  post.likeCount = Math.max(0, prevCount + (prevLiked ? -1 : 1))
  try {
    const result = prevLiked
      ? await unlikeSolutionPost(problemId.value, post.id)
      : await likeSolutionPost(problemId.value, post.id)
    post.likedByMe = result.liked
    post.likeCount = result.likeCount
  } catch (e) {
    post.likedByMe = prevLiked
    post.likeCount = prevCount
    toast.add({
      severity: 'error',
      summary: e instanceof ApiError ? e.message : '操作失败，请稍后再试',
      life: 3000,
    })
  }
}

let latest = 0
let loadedProblemId: number | null = null

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  if (problemId.value !== loadedProblemId) {
    posts.value = []
    total.value = 0
  }
  if (problemId.value === null) {
    loading.value = false
    error.value = '题目不存在'
    return
  }
  try {
    const data = await listSolutionPosts(problemId.value, { page: page.value, pageSize: PAGE_SIZE })
    if (seq !== latest) return
    loadedProblemId = problemId.value
    posts.value = data.items
    total.value = data.total
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载思路失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

watch(
  () => route.params.id,
  () => {
    page.value = 1
    void load()
  },
  { immediate: true },
)

function onPage(event: { page: number }) {
  page.value = event.page + 1
  void load()
}

function openPublish() {
  publishTitle.value = ''
  publishMd.value = ''
  publishError.value = ''
  publishOpen.value = true
}

async function submitPublish() {
  if (problemId.value === null) return
  if (!publishCanSubmit.value) return
  publishing.value = true
  publishError.value = ''
  try {
    await createSolutionPost(problemId.value, {
      title: publishTitleTrimmed.value,
      contentMd: publishMd.value,
    })
    publishOpen.value = false
    toast.add({ severity: 'success', summary: '已发布', life: 3000 })
    await load()
  } catch (e) {
    publishError.value = e instanceof ApiError ? e.message : '发布失败，请稍后再试'
  } finally {
    publishing.value = false
  }
}
</script>

<template>
  <div class="solutions-page">
    <AppPageHeader
      eyebrow="PROBLEM // SOLUTIONS"
      title="解题思路"
      :description="`题目 #${problemId} 的解题思路。`"
    >
      <template #actions>
        <Button
          as="router-link"
          :to="`/problems/${problemId}`"
          label="返回题目"
          icon="pi pi-arrow-left"
          severity="secondary"
          outlined
        />
        <Button v-if="isLoggedIn" label="发布思路" icon="pi pi-pencil" @click="openPublish" />
        <RouterLink v-else :to="loginTarget" class="login-link">
          <Button label="登录后发布" icon="pi pi-sign-in" />
        </RouterLink>
      </template>
    </AppPageHeader>

    <AppSurface padding="lg" class="solutions-surface">
      <AppErrorState v-if="error && !posts.length" :description="error" @retry="load" />

      <AppLoadingState v-if="loading && !posts.length" label="正在加载解题思路" />

      <AppEmptyState
        v-else-if="!error && !posts.length"
        icon="pi pi-comments"
        title="还没有人分享思路"
        description="成为第一个分享解题思路的人吧。"
      />

      <template v-else>
        <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>

        <div class="post-list">
          <RouterLink
            v-for="post in posts"
            :key="post.id"
            class="post-link"
            :to="`/problems/${problemId}/solutions/${post.id}`"
          >
            <AppSurface as="article" padding="md" interactive class="post-card">
              <div class="card-head">
                <h2 class="post-title">{{ post.title }}</h2>
                <div class="card-actions">
                  <Button
                    :icon="post.likedByMe ? 'pi pi-heart-fill' : 'pi pi-heart'"
                    :label="String(post.likeCount)"
                    :severity="post.likedByMe ? 'danger' : 'secondary'"
                    text
                    size="small"
                    :aria-pressed="post.likedByMe"
                    :aria-label="post.likedByMe ? '取消点赞' : '点赞'"
                    class="like-button"
                    @click.prevent.stop="toggleLike(post)"
                  />
                  <span class="reply-count">{{ post.replyCount }} 回复</span>
                </div>
              </div>
              <p class="post-excerpt">{{ post.excerpt }}</p>
              <p class="card-meta">
                {{ authorName(post.author) }} · {{ formatDateTime(post.createdAt) }}
              </p>
            </AppSurface>
          </RouterLink>
        </div>

        <Paginator
          v-if="total > 0"
          :rows="PAGE_SIZE"
          :total-records="total"
          :first="first"
          :page-link-size="5"
          template="PrevPageLink PageLinks NextPageLink"
          class="pager"
          @page="onPage"
        />
      </template>
    </AppSurface>

    <Dialog
      v-model:visible="publishOpen"
      header="发布思路"
      modal
      :draggable="false"
      :style="{ width: 'min(92vw, 640px)' }"
      @hide="publishError = ''"
    >
      <div class="publish-form">
        <label for="publish-title" class="field-label">标题</label>
        <InputText
          id="publish-title"
          v-model="publishTitle"
          fluid
          :maxlength="MAX_TITLE"
          @update:model-value="publishError = ''"
        />
        <label class="field-label">正文</label>
        <MarkdownEditor v-model="publishMd" :theme="isDark ? 'dark' : 'light'" />
        <Message v-if="publishLengthError" severity="error" :closable="false">
          {{ publishLengthError }}
        </Message>
        <Message v-if="publishError" severity="error" :closable="false">{{ publishError }}</Message>
      </div>
      <template #footer>
        <Button label="取消" severity="secondary" text @click="publishOpen = false" />
        <Button
          label="发布"
          icon="pi pi-check"
          :loading="publishing"
          :disabled="!publishCanSubmit"
          class="publish-submit"
          @click="submitPublish"
        />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.solutions-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.solutions-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.post-list {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.post-link {
  display: block;
  color: inherit;
  text-decoration: none;
}

.post-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
}

.card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
}

.card-actions {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.like-button {
  padding: 0 var(--kapoj-space-1);
}

.post-title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  line-height: 1.35;
  word-break: break-word;
}

.reply-count {
  flex-shrink: 0;
  padding: 1px 7px;
  color: var(--kapoj-text-secondary);
  font-size: 11px;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-xs);
}

.post-excerpt {
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
  line-height: 1.6;
  word-break: break-word;
}

.card-meta {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.publish-form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
}

.field-label {
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
  font-weight: 600;
}

.pager {
  align-self: flex-end;
}

@media (max-width: 640px) {
  .card-head {
    flex-direction: column;
  }

  .pager {
    align-self: stretch;
  }
}
</style>
