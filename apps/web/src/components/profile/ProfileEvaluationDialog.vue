<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Textarea from 'primevue/textarea'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import { deleteMyEvaluation, saveMyEvaluation } from '@/api/social'
import type { EvaluationItem } from '@/api/types'

const MAX_CONTENT = 40

const props = withDefaults(
  defineProps<{
    visible: boolean
    targetUserId: number
    targetActive?: boolean
    myEvaluation: EvaluationItem | null
    canEvaluate: boolean
    isSelf: boolean
    isLoggedIn: boolean
  }>(),
  {
    targetActive: true,
  },
)

const emit = defineEmits<{
  'update:visible': [value: boolean]
  saved: []
  deleted: []
}>()

const toast = useToast()
const confirm = useConfirm()

const content = ref('')
const saving = ref(false)
const deleting = ref(false)
const error = ref('')

const dialogVisible = computed({
  get: () => props.visible,
  set: (value: boolean) => emit('update:visible', value),
})

const normalizedContent = computed(() => content.value.replace(/[\r\n\t]+/g, ' ').trim())
const contentLength = computed(() => Array.from(normalizedContent.value).length)
const canSubmit = computed(() => normalizedContent.value.length > 0 && !saving.value)
const title = computed(() => (props.myEvaluation ? '更新评价' : '写评价'))
const submitLabel = computed(() => (props.myEvaluation ? '更新评价' : '发布评价'))

function message(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

watch(
  [() => props.visible, () => props.myEvaluation],
  ([visible, myEvaluation]) => {
    if (!visible) return
    content.value = myEvaluation?.content ?? ''
    error.value = ''
  },
  { immediate: true },
)

async function submit() {
  if (!canSubmit.value || saving.value) return
  saving.value = true
  error.value = ''
  try {
    await saveMyEvaluation(props.targetUserId, normalizedContent.value)
    toast.add({
      severity: 'success',
      summary: props.myEvaluation ? '评价已更新' : '评价已发布',
      life: 3000,
    })
    emit('update:visible', false)
    emit('saved')
  } catch (e) {
    error.value = message(e, '评价保存失败，请稍后再试')
  } finally {
    saving.value = false
  }
}

function requestDelete() {
  if (!props.myEvaluation) return
  confirm.require({
    header: '删除评价',
    message: '确定删除你对 TA 的评价吗？',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => void remove(),
  })
}

async function remove() {
  if (deleting.value) return
  deleting.value = true
  error.value = ''
  try {
    await deleteMyEvaluation(props.targetUserId)
    toast.add({ severity: 'success', summary: '评价已删除', life: 3000 })
    emit('update:visible', false)
    emit('deleted')
  } catch (e) {
    error.value = message(e, '评价删除失败，请稍后再试')
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <Dialog
    v-model:visible="dialogVisible"
    modal
    :header="title"
    :style="{ width: 'min(92vw, 30rem)' }"
    :draggable="false"
    class="profile-evaluation-dialog"
  >
    <div class="profile-evaluation-dialog__body">
      <template v-if="!isLoggedIn">
        <p class="profile-evaluation-dialog__hint">登录后可以给 TA 写评价。</p>
        <RouterLink class="profile-evaluation-dialog__login" :to="{ name: 'login' }">
          登录后评价
        </RouterLink>
      </template>

      <p v-else-if="isSelf" class="profile-evaluation-dialog__hint">
        这是你的主页，不能给自己评价。
      </p>

      <p v-else-if="!canEvaluate" class="profile-evaluation-dialog__hint">
        {{ targetActive === false ? '该用户当前不可评价。' : '当前不可评价。' }}
      </p>

      <template v-else>
        <Textarea
          v-model="content"
          class="profile-evaluation-dialog__input"
          :maxlength="MAX_CONTENT"
          :rows="3"
          auto-resize
          autofocus
          aria-label="评价内容"
          placeholder="用一句话评价 TA（40 字以内）"
        />
        <div class="profile-evaluation-dialog__footer">
          <span class="profile-evaluation-dialog__counter"
            >{{ contentLength }}/{{ MAX_CONTENT }}</span
          >
          <Button
            class="profile-evaluation-dialog__submit"
            :label="submitLabel"
            icon="pi pi-send"
            size="small"
            :disabled="!canSubmit"
            :loading="saving"
            @click="submit"
          />
        </div>
        <Button
          v-if="myEvaluation"
          class="profile-evaluation-dialog__delete"
          label="删除我的评价"
          icon="pi pi-trash"
          size="small"
          text
          severity="danger"
          :loading="deleting"
          @click="requestDelete"
        />
      </template>

      <p v-if="error" class="profile-evaluation-dialog__error">{{ error }}</p>
    </div>
  </Dialog>
</template>

<style scoped>
.profile-evaluation-dialog__body {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.profile-evaluation-dialog__hint {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 0.875rem;
  line-height: 1.6;
}

.profile-evaluation-dialog__login {
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

.profile-evaluation-dialog__login:hover {
  border-color: var(--kapoj-primary);
}

.profile-evaluation-dialog__input {
  width: 100%;
  font-size: 0.875rem;
}

.profile-evaluation-dialog__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
}

.profile-evaluation-dialog__counter {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 0.6875rem;
}

.profile-evaluation-dialog__delete {
  align-self: flex-start;
}

.profile-evaluation-dialog__error {
  margin: 0;
  color: var(--kapoj-danger);
  font-size: 0.8125rem;
}
</style>
