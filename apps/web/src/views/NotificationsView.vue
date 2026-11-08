<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Paginator from 'primevue/paginator'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import { listNotifications, markAllRead, markRead } from '@/api/social'
import type { NotificationItem } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { formatDateTime } from '@/utils/format'

const PAGE_SIZE = 20

const router = useRouter()
const toast = useToast()

const items = ref<NotificationItem[]>([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const error = ref('')
const markingAll = ref(false)

const first = computed(() => (page.value - 1) * PAGE_SIZE)
const hasUnread = computed(() => items.value.some((item) => item.readAt === null))

function typeLabel(type: NotificationItem['type']): string {
  switch (type) {
    case 'solution_reply':
      return '回复了你的解题思路'
    case 'solution_like':
      return '赞了你的解题思路'
    case 'follow':
      return '关注了你'
  }
}

function actorName(item: NotificationItem): string {
  return item.actor ? item.actor.nickname || item.actor.username : '未知用户'
}

let latest = 0

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const data = await listNotifications({ page: page.value, pageSize: PAGE_SIZE })
    if (seq !== latest) return
    items.value = data.items
    total.value = data.total
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载通知失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

onMounted(load)

function onPage(event: { page: number }) {
  page.value = event.page + 1
  void load()
}

async function openNotification(item: NotificationItem) {
  if (item.readAt === null) {
    try {
      await markRead(item.id)
      item.readAt = new Date().toISOString()
    } catch {
      // A failed single-mark shouldn't block navigation.
    }
  }
  void router.push(item.link)
}

async function markAll() {
  markingAll.value = true
  try {
    await markAllRead()
    items.value = items.value.map((item) => ({
      ...item,
      readAt: item.readAt ?? new Date().toISOString(),
    }))
    toast.add({ severity: 'success', summary: '已全部标记为已读', life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: e instanceof ApiError ? e.message : '操作失败，请稍后再试',
      life: 3000,
    })
  } finally {
    markingAll.value = false
  }
}
</script>

<template>
  <div class="notifications-page">
    <AppPageHeader
      eyebrow="USER // NOTIFICATIONS"
      title="通知"
      description="你收到的回复、点赞与关注。"
    />

    <AppSurface padding="lg" class="notifications-surface">
      <AppErrorState v-if="error && !items.length" :description="error" @retry="load" />

      <AppLoadingState v-if="loading && !items.length" label="正在加载通知" />

      <AppEmptyState
        v-else-if="!error && !items.length"
        icon="pi pi-bell"
        title="暂无通知"
        description="别人回复、点赞你的思路或关注你时，这里会有提醒。"
      />

      <template v-else>
        <div class="notifications-toolbar">
          <span class="notifications-count">共 {{ total }} 条</span>
          <Button
            v-if="hasUnread"
            label="全部已读"
            icon="pi pi-check"
            size="small"
            :loading="markingAll"
            @click="markAll"
          />
        </div>

        <ul class="notification-list">
          <li v-for="item in items" :key="item.id">
            <button
              type="button"
              class="notification-item"
              :class="{ 'notification-item--unread': item.readAt === null }"
              @click="openNotification(item)"
            >
              <span class="notification-item__dot" aria-hidden="true" />
              <div class="notification-item__body">
                <span class="notification-item__text">
                  <strong>{{ actorName(item) }}</strong>
                  {{ typeLabel(item.type) }}
                </span>
                <span class="notification-item__time">{{ formatDateTime(item.createdAt) }}</span>
              </div>
            </button>
          </li>
        </ul>

        <Paginator
          v-if="total > PAGE_SIZE"
          :rows="PAGE_SIZE"
          :total-records="total"
          :first="first"
          :page-link-size="5"
          template="PrevPageLink PageLinks NextPageLink"
          class="notifications-pager"
          @page="onPage"
        />
      </template>
    </AppSurface>
  </div>
</template>

<style scoped>
.notifications-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.notifications-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.notifications-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
}

.notifications-count {
  color: var(--kapoj-text-muted);
  font-size: 0.8125rem;
}

.notification-list {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  padding: 0;
  margin: 0;
  list-style: none;
}

.notification-item {
  display: flex;
  align-items: flex-start;
  gap: var(--kapoj-space-3);
  width: 100%;
  padding: var(--kapoj-space-3);
  color: inherit;
  text-align: left;
  cursor: pointer;
  background: var(--kapoj-bg);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.notification-item:hover {
  border-color: var(--kapoj-border-strong);
}

.notification-item--unread {
  background: color-mix(in srgb, var(--kapoj-primary) 5%, var(--kapoj-bg));
  border-color: color-mix(in srgb, var(--kapoj-primary) 28%, var(--kapoj-border));
}

.notification-item__dot {
  flex: none;
  width: 8px;
  height: 8px;
  margin-top: 5px;
  background: transparent;
  border-radius: 50%;
}

.notification-item--unread .notification-item__dot {
  background: var(--kapoj-primary);
}

.notification-item__body {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
  min-width: 0;
}

.notification-item__text {
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
  line-height: 1.5;
}

.notification-item__text strong {
  margin-right: 4px;
  color: var(--kapoj-text);
  font-weight: 600;
}

.notification-item__time {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.notifications-pager {
  align-self: flex-end;
}

@media (max-width: 640px) {
  .notifications-pager {
    align-self: stretch;
  }
}
</style>
