<script setup lang="ts">
import { ref, watch } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import {
  deleteProblemAttachment,
  listProblemAttachments,
  uploadProblemAttachment,
} from '@/api/problems'
import type { ProblemAttachment } from '@/api/types'
import { resolveAssetUrl } from '@/utils/asset'
import { formatBytes } from '@/utils/format'

const props = defineProps<{ problemId: number }>()

const toast = useToast()
const confirm = useConfirm()
const attachments = ref<ProblemAttachment[]>([])
const loading = ref(false)
const uploading = ref(false)
const error = ref('')
const dragActive = ref(false)
const fileInput = ref<HTMLInputElement>()

let latest = 0

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const list = await listProblemAttachments(props.problemId)
    if (seq === latest) attachments.value = list
  } catch (e) {
    if (seq === latest) {
      error.value = e instanceof ApiError ? e.message : '加载附件失败，请稍后再试'
    }
  } finally {
    if (seq === latest) loading.value = false
  }
}

watch(
  () => props.problemId,
  () => void load(),
  { immediate: true },
)

async function uploadFiles(files: File[]) {
  if (!files.length || uploading.value) return
  uploading.value = true
  error.value = ''
  try {
    for (const file of files) {
      await uploadProblemAttachment(props.problemId, file)
    }
    toast.add({ severity: 'success', summary: '附件已上传', life: 3000 })
    await load()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: e instanceof ApiError ? e.message : '上传附件失败，请稍后再试',
      life: 4000,
    })
  } finally {
    uploading.value = false
    if (fileInput.value) fileInput.value.value = ''
  }
}

function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  void uploadFiles([...(input.files ?? [])])
}

function onDrop(event: DragEvent) {
  dragActive.value = false
  void uploadFiles([...(event.dataTransfer?.files ?? [])])
}

async function copyLink(item: ProblemAttachment) {
  const absolute = new URL(resolveAssetUrl(item.url), window.location.origin).toString()
  try {
    await navigator.clipboard.writeText(absolute)
    toast.add({ severity: 'success', summary: '下载链接已复制', life: 3000 })
  } catch {
    toast.add({ severity: 'warn', summary: '复制失败，请手动复制链接', life: 3000 })
  }
}

function remove(item: ProblemAttachment) {
  confirm.require({
    message: `确定删除附件「${item.filename}」吗？`,
    header: '删除附件',
    icon: 'pi pi-exclamation-triangle',
    rejectProps: { label: '取消', severity: 'secondary', outlined: true },
    acceptProps: { label: '删除', severity: 'danger' },
    accept: async () => {
      try {
        await deleteProblemAttachment(props.problemId, item.id)
        attachments.value = attachments.value.filter((a) => a.id !== item.id)
        toast.add({ severity: 'success', summary: '附件已删除', life: 3000 })
      } catch (e) {
        toast.add({
          severity: 'error',
          summary: e instanceof ApiError ? e.message : '删除附件失败，请稍后再试',
          life: 4000,
        })
      }
    },
  })
}
</script>

<template>
  <div class="attachment-manager">
    <div class="attachment-manager__head">
      <div>
        <h2 class="attachment-manager__title">题目附件</h2>
        <p class="attachment-manager__hint">
          公开题附件对所有人可见；测试数据请使用下方“测试数据”区域。
        </p>
      </div>
      <span class="attachment-manager__count">{{ attachments.length }} 个附件</span>
    </div>

    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>

    <div
      class="attachment-drop"
      :class="{ 'attachment-drop--active': dragActive, 'attachment-drop--busy': uploading }"
      role="button"
      tabindex="0"
      @click="fileInput?.click()"
      @keydown.enter.prevent="fileInput?.click()"
      @keydown.space.prevent="fileInput?.click()"
      @dragover.prevent="dragActive = true"
      @dragleave.prevent="dragActive = false"
      @drop.prevent="onDrop"
    >
      <i class="pi pi-cloud-upload" aria-hidden="true" />
      <span>{{ uploading ? '正在上传…' : '点击或拖拽文件到这里上传' }}</span>
      <small>单个附件不超过 20 MiB</small>
      <input
        ref="fileInput"
        type="file"
        multiple
        class="visually-hidden"
        data-test="attachment-input"
        @click.stop
        @change="onFileChange"
      />
    </div>

    <div v-if="loading" class="attachment-manager__empty">正在加载附件…</div>
    <ul v-else-if="attachments.length" class="attachment-list">
      <li v-for="item in attachments" :key="item.id" class="attachment-item">
        <i class="pi pi-paperclip" aria-hidden="true" />
        <div class="attachment-item__body">
          <a :href="resolveAssetUrl(item.url)" class="attachment-item__name" download>
            {{ item.filename }}
          </a>
          <span class="attachment-item__meta">
            {{ formatBytes(item.size) }} · {{ item.contentType || '未知类型' }}
          </span>
        </div>
        <div class="attachment-item__actions">
          <Button label="复制链接" icon="pi pi-link" text size="small" @click="copyLink(item)" />
          <Button
            label="删除"
            icon="pi pi-trash"
            text
            size="small"
            severity="danger"
            @click="remove(item)"
          />
        </div>
      </li>
    </ul>
    <p v-else class="attachment-manager__empty">
      还没有附件。样例文件、PDF、图片等都可以放在这里。
    </p>
  </div>
</template>

<style scoped>
.attachment-manager {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.attachment-manager__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
}

.attachment-manager__title {
  margin: 0;
  font-size: 1rem;
}

.attachment-manager__hint {
  margin: 2px 0 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.attachment-manager__count {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.attachment-drop {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: var(--kapoj-space-4);
  color: var(--kapoj-text-secondary);
  text-align: center;
  cursor: pointer;
  border: 1px dashed var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.attachment-drop:hover,
.attachment-drop--active {
  color: var(--kapoj-primary);
  border-color: var(--kapoj-primary);
  background: var(--kapoj-primary-soft);
}

.attachment-drop--busy {
  cursor: wait;
  opacity: 0.7;
}

.attachment-drop i {
  font-size: 1.5rem;
}

.attachment-drop small,
.attachment-manager__empty {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.attachment-list {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  padding: 0;
  margin: 0;
  list-style: none;
}

.attachment-item {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  padding: var(--kapoj-space-2);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.attachment-item__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
}

.attachment-item__name {
  overflow: hidden;
  color: var(--kapoj-text);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.attachment-item__meta {
  color: var(--kapoj-text-muted);
  font-size: 11px;
}

.attachment-item__actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
}

@media (max-width: 640px) {
  .attachment-item {
    align-items: flex-start;
    flex-wrap: wrap;
  }

  .attachment-item__actions {
    width: 100%;
    justify-content: flex-start;
  }
}
</style>
