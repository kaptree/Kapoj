<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Textarea from 'primevue/textarea'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import {
  deleteMyEvaluation,
  deleteUserEvaluation,
  listUserEvaluations,
  saveMyEvaluation,
} from '@/api/social'
import type { EvaluationItem, EvaluationPage } from '@/api/types'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime } from '@/utils/format'

const MAX_CONTENT = 40
const PAGE_SIZE = 10
const CLOUD_COLORS = ['primary', 'success', 'warning', 'special', 're'] as const

const props = defineProps<{
  targetUserId: number
  targetActive?: boolean
}>()

const auth = useAuthStore()
const confirm = useConfirm()
const toast = useToast()

const pageData = ref<EvaluationPage | null>(null)
const loading = ref(false)
const error = ref('')
const page = ref(1)
const myContent = ref('')
const saving = ref(false)
const loadingMore = ref(false)
const deletingId = ref<number | null>(null)
const failedAvatars = ref<number[]>([])
let latestLoad = 0

const isSelf = computed(() => auth.user?.id != null && auth.user.id === props.targetUserId)
const isLoggedIn = computed(() => auth.isLoggedIn)
const canEdit = computed(() => pageData.value?.canEvaluate === true && !isSelf.value)
const cloud = computed(() => pageData.value?.cloud ?? [])
const items = computed(() => pageData.value?.items ?? [])
const myEvaluation = computed(() => pageData.value?.myEvaluation ?? null)
const hasMore = computed(
  () => pageData.value !== null && pageData.value.items.length < pageData.value.total,
)
const submitLabel = computed(() => (myEvaluation.value ? '更新评价' : '发布评价'))
const contentLength = computed(() => Array.from(myContent.value).length)
const canSubmit = computed(() => myContent.value.trim().length > 0 && !saving.value)

function message(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

function cloudFontSize(count: number): string {
  const size = 0.95 + Math.min(Math.max(count, 1) - 1, 4) * 0.18
  return `${Number(size.toFixed(2))}rem`
}

/** Stable hash keeps a word's colour consistent between renders and reloads. */
function cloudColor(text: string): string {
  let hash = 0
  for (let index = 0; index < text.length; index += 1) {
    hash = (hash * 31 + text.charCodeAt(index)) >>> 0
  }
  return `evaluation-cloud__tag--${CLOUD_COLORS[hash % CLOUD_COLORS.length]}`
}

function authorName(item: EvaluationItem): string {
  return item.author.nickname || item.author.username || '匿名用户'
}

function authorInitial(item: EvaluationItem): string {
  return authorName(item).slice(0, 1)
}

function avatarImage(item: EvaluationItem): string | undefined {
  if (!item.author.avatarUrl || failedAvatars.value.includes(item.author.id)) return undefined
  return item.author.avatarUrl
}

function onAvatarError(authorId: number) {
  if (!failedAvatars.value.includes(authorId)) failedAvatars.value.push(authorId)
}

async function load(targetPage = 1, append = false) {
  const seq = ++latestLoad
  if (append) {
    loadingMore.value = true
  } else {
    loading.value = true
    error.value = ''
  }
  try {
    const data = await listUserEvaluations(props.targetUserId, targetPage, PAGE_SIZE)
    if (seq !== latestLoad) return
    if (append && pageData.value) {
      pageData.value = {
        ...data,
        items: [...pageData.value.items, ...data.items],
      }
    } else {
      pageData.value = data
      myContent.value = data.myEvaluation?.content ?? ''
    }
    page.value = targetPage
  } catch (e) {
    if (seq !== latestLoad) return
    const text = message(e, '评价加载失败，请稍后再试')
    if (append) {
      toast.add({ severity: 'error', summary: '加载更多失败', detail: text, life: 4000 })
    } else {
      pageData.value = null
      error.value = text
    }
  } finally {
    if (seq === latestLoad) {
      loading.value = false
      loadingMore.value = false
    }
  }
}

function retry() {
  void load(1)
}

function loadMore() {
  if (loadingMore.value) return
  void load(page.value + 1, true)
}

async function submit() {
  const content = myContent.value.trim()
  if (!content || saving.value) return
  const updating = myEvaluation.value !== null
  saving.value = true
  try {
    await saveMyEvaluation(props.targetUserId, content)
    toast.add({
      severity: 'success',
      summary: updating ? '评价已更新' : '评价已发布',
      life: 3000,
    })
    await load(1)
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '评价保存失败',
      detail: message(e, '评价保存失败，请稍后再试'),
      life: 4000,
    })
  } finally {
    saving.value = false
  }
}

async function removeEvaluation(item: EvaluationItem, mine: boolean) {
  if (deletingId.value !== null) return
  deletingId.value = item.id
  try {
    if (mine) {
      await deleteMyEvaluation(props.targetUserId)
    } else {
      await deleteUserEvaluation(props.targetUserId, item.id)
    }
    toast.add({ severity: 'success', summary: '评价已删除', life: 3000 })
    await load(1)
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '删除失败',
      detail: message(e, '评价删除失败，请稍后再试'),
      life: 4000,
    })
  } finally {
    deletingId.value = null
  }
}

function requestDelete(item: EvaluationItem) {
  const mine = auth.user?.id != null && item.author.id === auth.user.id
  if (mine) {
    void removeEvaluation(item, true)
    return
  }
  if (!isSelf.value) {
    void removeEvaluation(item, false)
    return
  }
  confirm.require({
    header: '删除评价',
    message: `确定删除 ${authorName(item)} 的这条评价吗？`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => void removeEvaluation(item, false),
  })
}

watch(
  () => props.targetUserId,
  () => {
    pageData.value = null
    error.value = ''
    page.value = 1
    myContent.value = ''
    failedAvatars.value = []
    void load(1)
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  latestLoad += 1
})
</script>

<template>
  <AppSurface padding="lg" class="evaluation-card" data-test="evaluation-card">
    <header class="evaluation-card__head">
      <div>
        <h2 class="evaluation-card__title">词云评价</h2>
        <p class="evaluation-card__hint">同样的评价出现越多，词就越大。</p>
      </div>
      <span v-if="pageData && pageData.total > 0" class="evaluation-card__total">
        {{ pageData.total }} 条
      </span>
    </header>

    <AppLoadingState v-if="loading && !pageData" label="正在加载评价" />

    <AppErrorState v-else-if="error" title="评价加载失败" :description="error" @retry="retry" />

    <template v-else-if="pageData">
      <div class="evaluation-cloud" data-test="evaluation-cloud">
        <span
          v-for="item in cloud"
          :key="item.text"
          class="evaluation-cloud__tag"
          :class="cloudColor(item.text)"
          :style="{ fontSize: cloudFontSize(item.count) }"
          :title="`出现 ${item.count} 次`"
        >
          {{ item.text }}
        </span>
        <p v-if="cloud.length === 0" class="evaluation-cloud__empty">还没有评价</p>
      </div>

      <section class="evaluation-form" data-test="evaluation-form">
        <p v-if="!isLoggedIn" class="evaluation-form__hint">登录后可以给 TA 写一句评价。</p>
        <RouterLink v-if="!isLoggedIn" class="evaluation-form__login" :to="{ name: 'login' }">
          登录后评价
        </RouterLink>

        <p v-else-if="isSelf" class="evaluation-form__hint">这是你的主页，不能给自己评价。</p>

        <template v-else-if="canEdit">
          <Textarea
            v-model="myContent"
            class="evaluation-form__input"
            :maxlength="MAX_CONTENT"
            :rows="3"
            auto-resize
            aria-label="评价内容"
            placeholder="用一句话评价 TA（40 字以内）"
          />
          <div class="evaluation-form__footer">
            <span class="evaluation-form__counter">{{ contentLength }}/{{ MAX_CONTENT }}</span>
            <Button
              class="evaluation-form__submit"
              :label="submitLabel"
              icon="pi pi-send"
              size="small"
              :disabled="!canSubmit"
              :loading="saving"
              @click="submit"
            />
          </div>
        </template>

        <p v-else class="evaluation-form__hint">
          {{ targetActive === false ? '该用户当前不可评价。' : '当前不可评价。' }}
        </p>
      </section>

      <div v-if="items.length > 0" class="evaluation-list">
        <article
          v-for="item in items"
          :key="item.id"
          class="evaluation-item"
          data-test="evaluation-item"
        >
          <Avatar
            :image="avatarImage(item)"
            :label="avatarImage(item) ? undefined : authorInitial(item)"
            size="normal"
            shape="circle"
            class="evaluation-item__avatar"
            :aria-label="`${authorName(item)} 的头像`"
            @error="onAvatarError(item.author.id)"
          />
          <div class="evaluation-item__body">
            <div class="evaluation-item__meta">
              <RouterLink
                class="evaluation-item__author"
                :to="{ name: 'user-profile', params: { id: item.author.id } }"
              >
                {{ authorName(item) }}
              </RouterLink>
              <time class="evaluation-item__time" :datetime="item.updatedAt">
                {{ formatDateTime(item.updatedAt) }}
              </time>
            </div>
            <p class="evaluation-item__content">{{ item.content }}</p>
          </div>
          <Button
            v-if="item.canDelete"
            class="evaluation-item__delete"
            label="删除"
            icon="pi pi-trash"
            size="small"
            text
            severity="danger"
            :loading="deletingId === item.id"
            @click="requestDelete(item)"
          />
        </article>
      </div>

      <Button
        v-if="hasMore"
        class="evaluation-list__more"
        label="加载更多"
        icon="pi pi-angle-down"
        size="small"
        severity="secondary"
        outlined
        :loading="loadingMore"
        @click="loadMore"
      />
    </template>
  </AppSurface>
</template>

<style scoped>
.evaluation-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.evaluation-card__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
}

.evaluation-card__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.05rem;
  font-weight: 600;
  letter-spacing: -0.01em;
}

.evaluation-card__hint {
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text-muted);
  font-size: 0.75rem;
}

.evaluation-card__total {
  flex: 0 0 auto;
  padding: 1px var(--kapoj-space-2);
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 0.6875rem;
  border: 1px solid var(--kapoj-border);
  border-radius: 999px;
}

.evaluation-cloud {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 14px;
  align-items: center;
  justify-content: center;
  min-height: 120px;
  padding: var(--kapoj-space-3);
  background: var(--kapoj-bg-elevated);
  border: 1px dashed var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.evaluation-cloud__tag {
  font-weight: 600;
  line-height: 1.25;
  word-break: break-word;
}

.evaluation-cloud__tag--primary {
  color: var(--kapoj-primary);
}

.evaluation-cloud__tag--success {
  color: var(--kapoj-success);
}

.evaluation-cloud__tag--warning {
  color: var(--kapoj-warning);
}

.evaluation-cloud__tag--special {
  color: var(--kapoj-special);
}

.evaluation-cloud__tag--re {
  color: var(--kapoj-re);
}

.evaluation-cloud__empty {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 0.875rem;
}

.evaluation-form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  padding-top: var(--kapoj-space-3);
  border-top: 1px solid var(--kapoj-border);
}

.evaluation-form__hint {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 0.8125rem;
  line-height: 1.6;
}

.evaluation-form__login {
  align-self: flex-start;
  padding: 5px var(--kapoj-space-3);
  color: var(--kapoj-primary);
  font-size: 0.8125rem;
  font-weight: 600;
  text-decoration: none;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 35%, transparent);
  border-radius: var(--kapoj-radius-sm);
}

.evaluation-form__login:hover {
  border-color: var(--kapoj-primary);
}

.evaluation-form__input {
  width: 100%;
  font-size: 0.875rem;
}

.evaluation-form__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
}

.evaluation-form__counter {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 0.6875rem;
}

.evaluation-list {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  padding-top: var(--kapoj-space-3);
  border-top: 1px solid var(--kapoj-border);
}

.evaluation-item {
  display: flex;
  align-items: flex-start;
  gap: var(--kapoj-space-2);
  padding: var(--kapoj-space-2);
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.evaluation-item__avatar {
  flex: 0 0 auto;
  color: var(--kapoj-primary);
  font-size: 0.75rem;
  font-weight: 700;
  background: var(--kapoj-primary-soft);
}

.evaluation-item__body {
  flex: 1 1 auto;
  min-width: 0;
}

.evaluation-item__meta {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: var(--kapoj-space-1) var(--kapoj-space-2);
}

.evaluation-item__author {
  color: var(--kapoj-text);
  font-size: 0.8125rem;
  font-weight: 600;
  text-decoration: none;
}

.evaluation-item__author:hover {
  color: var(--kapoj-primary);
}

.evaluation-item__time {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 0.6875rem;
}

.evaluation-item__content {
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text);
  font-size: 0.875rem;
  line-height: 1.6;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}

.evaluation-item__delete {
  flex: 0 0 auto;
}

.evaluation-list__more {
  align-self: center;
}
</style>
