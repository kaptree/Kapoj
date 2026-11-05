<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressBar from 'primevue/progressbar'
import Tag from 'primevue/tag'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import { equipTitle, listTitles, purchaseTitle, unequipTitle } from '@/api/gamification'
import { ApiError } from '@/api/http'
import type { TitleItem } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useAuthStore } from '@/stores/auth'
import { formatCoins, safeTitleColor, titleUnlockLabel } from '@/utils/gamification'

const route = useRoute()
const auth = useAuthStore()
const toast = useToast()
const confirm = useConfirm()

const titles = ref<TitleItem[]>([])
const loading = ref(false)
const error = ref('')
const actingId = ref<number | null>(null)
let latest = 0

const isLoggedIn = computed(() => auth.isLoggedIn)
const balance = computed(() => auth.user?.coins ?? 0)
const loginTarget = computed(() => ({ name: 'login', query: { redirect: route.fullPath } }))

function insufficient(item: TitleItem): boolean {
  return isLoggedIn.value && auth.user !== null && auth.user.coins < item.priceCoins
}

function shortfall(item: TitleItem): number {
  return Math.max(0, item.priceCoins - balance.value)
}

function conditionText(item: TitleItem): string {
  const label = titleUnlockLabel(item.progress.type)
  if (item.progress.type === 'achievement' && item.progress.achievementCode) {
    return `${label}：${item.progress.achievementCode}`
  }
  return label
}

function progressPercent(item: TitleItem): number {
  const { current, target } = item.progress
  if (target <= 0) return item.progress.done ? 100 : 0
  return Math.min(100, Math.round((current / target) * 100))
}

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const data = await listTitles()
    if (seq !== latest) return
    titles.value = data
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载称号商店失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

async function runAction(id: number, action: () => Promise<unknown>, successSummary: string) {
  actingId.value = id
  try {
    await action()
    toast.add({ severity: 'success', summary: successSummary, life: 3000 })
    await auth.fetchMe().catch(() => undefined)
    await load()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '操作失败',
      detail: e instanceof ApiError ? e.message : '操作失败，请稍后再试',
      life: 4000,
    })
  } finally {
    actingId.value = null
  }
}

function buy(item: TitleItem) {
  const price = item.priceCoins > 0 ? `花费 ${formatCoins(item.priceCoins)} 金币` : '免费'
  confirm.require({
    header: '购买称号',
    message: `确定${price}购买称号「${item.name}」吗？`,
    icon: 'pi pi-shopping-cart',
    acceptLabel: '购买',
    rejectLabel: '取消',
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => {
      void runAction(item.id, () => purchaseTitle(item.id), '购买成功，装备后即可展示')
    },
  })
}

function toggleEquip(item: TitleItem) {
  if (item.equipped) {
    void runAction(item.id, () => unequipTitle(item.id), '已卸下称号')
  } else {
    void runAction(item.id, () => equipTitle(item.id), '已装备称号，原称号已自动卸下')
  }
}

onMounted(load)
// 登录状态变化后重新拉取，以刷新解锁/购买/装备状态与余额。
watch(
  () => auth.isLoggedIn,
  () => void load(),
)
</script>

<template>
  <div class="title-shop-page">
    <AppPageHeader
      eyebrow="SHOP // TITLES"
      title="称号商店"
      description="达成解锁条件后，使用金币购买并装备专属称号。"
    >
      <template #actions>
        <Tag v-if="isLoggedIn" severity="warn" :value="`余额 ${formatCoins(balance)} 金币`" />
      </template>
    </AppPageHeader>

    <div v-if="!isLoggedIn" class="login-cta">
      <span>登录后可查看解锁进度、购买并装备称号。</span>
      <RouterLink :to="loginTarget">
        <Button label="登录 / 注册" icon="pi pi-sign-in" size="small" outlined />
      </RouterLink>
    </div>

    <AppErrorState v-if="error && !titles.length" :description="error" @retry="load" />

    <AppLoadingState v-else-if="loading && !titles.length" label="正在加载称号商店" />

    <template v-else>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>

      <AppEmptyState
        v-if="!error && titles.length === 0"
        icon="pi pi-id-card"
        title="暂无称号"
        description="新的称号上架后会显示在这里。"
      />

      <div v-else class="title-grid">
        <AppSurface
          v-for="item in titles"
          :key="item.id"
          as="article"
          padding="md"
          class="title-card"
          :class="{ 'is-equipped': item.equipped }"
        >
          <div class="card-head">
            <div class="title-wrap">
              <h2
                class="title-name"
                :style="{ color: safeTitleColor(item.color, 'var(--kapoj-text)') }"
              >
                {{ item.name }}
              </h2>
              <div class="title-tags">
                <Tag v-if="item.equipped" severity="success" value="装备中" />
                <Tag v-else-if="item.purchased" severity="info" value="已购买" />
                <Tag v-else-if="item.unlocked" severity="warn" value="已解锁" />
                <Tag v-else severity="secondary" value="未解锁" />
                <Tag v-if="!item.isActive" severity="secondary" value="已下线" />
              </div>
            </div>
            <span class="price">
              {{ item.priceCoins > 0 ? `${formatCoins(item.priceCoins)} 金币` : '免费' }}
            </span>
          </div>

          <div class="condition">
            <div class="condition-head">
              <span>{{ conditionText(item) }}</span>
              <span class="num">{{ item.progress.current }} / {{ item.progress.target }}</span>
            </div>
            <ProgressBar
              :value="progressPercent(item)"
              :show-value="false"
              :class="{ 'is-done': item.progress.done }"
            />
          </div>

          <div class="card-foot">
            <template v-if="!isLoggedIn">
              <RouterLink :to="loginTarget">
                <Button label="登录后查看" icon="pi pi-sign-in" size="small" outlined />
              </RouterLink>
            </template>
            <template v-else-if="item.equipped">
              <Button
                label="卸下"
                size="small"
                outlined
                :loading="actingId === item.id"
                :disabled="actingId !== null && actingId !== item.id"
                @click="toggleEquip(item)"
              />
            </template>
            <template v-else-if="item.purchased">
              <Button
                label="装备"
                size="small"
                outlined
                :loading="actingId === item.id"
                :disabled="actingId !== null && actingId !== item.id"
                @click="toggleEquip(item)"
              />
            </template>
            <template v-else-if="item.unlocked">
              <Button v-if="insufficient(item)" label="余额不足" size="small" disabled />
              <Button
                v-else
                label="购买"
                icon="pi pi-shopping-cart"
                size="small"
                :loading="actingId === item.id"
                :disabled="actingId !== null && actingId !== item.id"
                @click="buy(item)"
              />
            </template>
            <template v-else>
              <Button label="未解锁" size="small" disabled />
            </template>
          </div>

          <div v-if="!isLoggedIn" class="hint">登录后可查看自己的解锁进度。</div>
          <div v-else-if="item.equipped" class="hint">该称号正在公开展示。</div>
          <div v-else-if="item.purchased" class="hint">装备后会自动替换当前称号。</div>
          <div v-else-if="item.unlocked && insufficient(item)" class="hint">
            余额不足，还差 {{ formatCoins(shortfall(item)) }} 金币。
          </div>
          <div v-else-if="item.unlocked" class="hint">已达成条件，购买后即可装备。</div>
          <div v-else class="hint">达成解锁条件后即可购买。</div>
          <div v-if="item.unlocked && !item.isActive" class="hint">
            该称号已下线，已解锁或已购买的用户仍可继续兑换与使用。
          </div>
        </AppSurface>
      </div>
    </template>
  </div>
</template>

<style scoped>
.title-shop-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.login-cta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-3) var(--kapoj-space-4);
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 26%, var(--kapoj-border));
  border-radius: var(--kapoj-radius-md);
}

.title-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: var(--kapoj-space-4);
}

.title-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
}

.title-card.is-equipped {
  border-color: color-mix(in srgb, var(--kapoj-success) 45%, var(--kapoj-border));
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--kapoj-success) 18%, transparent);
}

.card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
}

.title-wrap {
  flex: 1;
  min-width: 0;
}

.title-name {
  margin: 0;
  font-size: 1.25rem;
  font-weight: 700;
  line-height: 1.3;
  word-break: break-word;
}

.title-tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-1);
  margin-top: var(--kapoj-space-2);
}

.price {
  flex: 0 0 auto;
  color: var(--kapoj-warning);
  font-size: 0.8125rem;
  font-weight: 600;
}

.condition {
  padding-top: var(--kapoj-space-3);
  border-top: 1px solid var(--kapoj-border);
}

.condition-head {
  display: flex;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
  margin-bottom: var(--kapoj-space-1);
  color: var(--kapoj-text-secondary);
  font-size: 12px;
}

.condition :deep(.p-progressbar) {
  height: 6px;
  background: var(--kapoj-bg);
  border-radius: var(--kapoj-radius-xs);
}

.condition :deep(.p-progressbar-value) {
  background: var(--kapoj-primary);
}

.condition :deep(.p-progressbar.is-done .p-progressbar-value) {
  background: var(--kapoj-success);
}

.card-foot {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: var(--kapoj-space-2);
  min-height: 40px;
  padding-top: var(--kapoj-space-3);
  margin-top: auto;
}

.hint {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

@media (max-width: 640px) {
  .title-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .login-cta :deep(.p-button) {
    width: 100%;
  }
}
</style>
