<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Avatar from 'primevue/avatar'
import Dialog from 'primevue/dialog'
import Paginator from 'primevue/paginator'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import Tabs from 'primevue/tabs'

import { ApiError } from '@/api/http'
import { listFollowers, listFollowing } from '@/api/social'
import type { SocialUserSummary } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'

const PAGE_SIZE = 20

const props = withDefaults(
  defineProps<{
    userId: number
    initialTab?: 'followers' | 'following'
  }>(),
  { initialTab: 'followers' },
)

const visible = defineModel<boolean>('visible', { default: false })

const activeTab = ref<'followers' | 'following'>(props.initialTab)
const items = ref<SocialUserSummary[]>([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const error = ref('')

const first = computed(() => (page.value - 1) * PAGE_SIZE)
const emptyTitle = computed(() => (activeTab.value === 'followers' ? '暂无粉丝' : '暂无关注'))

function displayName(user: SocialUserSummary): string {
  return user.nickname || user.username
}

let latest = 0

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const data =
      activeTab.value === 'followers'
        ? await listFollowers(props.userId, { page: page.value, pageSize: PAGE_SIZE })
        : await listFollowing(props.userId, { page: page.value, pageSize: PAGE_SIZE })
    if (seq !== latest) return
    items.value = data.items
    total.value = data.total
  } catch (e) {
    if (seq !== latest) return
    items.value = []
    total.value = 0
    error.value = e instanceof ApiError ? e.message : '加载失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

watch(
  visible,
  (value) => {
    if (!value) return
    activeTab.value = props.initialTab
    page.value = 1
    void load()
  },
  { immediate: true },
)

watch(activeTab, () => {
  page.value = 1
  if (visible.value) void load()
})

function onPage(event: { page: number }) {
  page.value = event.page + 1
  void load()
}
</script>

<template>
  <Dialog
    v-model:visible="visible"
    header="粉丝与关注"
    modal
    :draggable="false"
    append-to="self"
    :style="{ width: 'min(92vw, 480px)' }"
  >
    <div class="follow-dialog">
      <Tabs v-model:value="activeTab">
        <TabList>
          <Tab value="followers">粉丝</Tab>
          <Tab value="following">关注</Tab>
        </TabList>
      </Tabs>

      <AppLoadingState v-if="loading" label="正在加载" />

      <AppErrorState v-else-if="error" :description="error" @retry="load" />

      <AppEmptyState
        v-else-if="items.length === 0"
        icon="pi pi-users"
        :title="emptyTitle"
        :description="activeTab === 'followers' ? '还没有人关注 Ta。' : 'Ta 还没有关注任何人。'"
      />

      <template v-else>
        <ul class="follow-list">
          <li v-for="user in items" :key="user.id">
            <RouterLink :to="`/users/${user.id}`" class="follow-user">
              <Avatar
                :image="user.avatarUrl || undefined"
                :label="displayName(user).slice(0, 1)"
                shape="circle"
                class="follow-user__avatar"
              />
              <div class="follow-user__info">
                <span class="follow-user__name">{{ displayName(user) }}</span>
                <span class="follow-user__username">@{{ user.username }}</span>
              </div>
              <span class="follow-user__rating">Rating {{ user.rating }}</span>
            </RouterLink>
          </li>
        </ul>

        <Paginator
          v-if="total > PAGE_SIZE"
          :rows="PAGE_SIZE"
          :total-records="total"
          :first="first"
          :page-link-size="5"
          template="PrevPageLink PageLinks NextPageLink"
          class="follow-pager"
          @page="onPage"
        />
      </template>
    </div>
  </Dialog>
</template>

<style scoped>
.follow-dialog {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
}

.follow-list {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
  padding: 0;
  margin: 0;
  list-style: none;
}

.follow-user {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-2) var(--kapoj-space-3);
  color: inherit;
  text-decoration: none;
  border-radius: var(--kapoj-radius-sm);
}

.follow-user:hover {
  background: var(--kapoj-surface-hover);
}

.follow-user__avatar {
  flex: none;
  color: var(--kapoj-primary);
  background: var(--kapoj-primary-soft);
}

.follow-user__info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.follow-user__name {
  color: var(--kapoj-text);
  font-size: 0.875rem;
  font-weight: 600;
}

.follow-user__username {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
}

.follow-user__rating {
  margin-left: auto;
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  white-space: nowrap;
}

.follow-pager {
  align-self: flex-end;
}

@media (max-width: 640px) {
  .follow-pager {
    align-self: stretch;
  }
}
</style>
