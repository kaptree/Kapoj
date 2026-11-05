<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Message from 'primevue/message'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import {
  equipBadge,
  getGamificationConfig,
  listBadges,
  purchaseBadge,
  unequipBadge,
} from '@/api/gamification'
import { ApiError } from '@/api/http'
import type { BadgeItem, GamificationConfig } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useAuthStore } from '@/stores/auth'
import { badgeImageSrc, badgeInitial, formatCoins } from '@/utils/gamification'

const route = useRoute()
const auth = useAuthStore()
const toast = useToast()
const confirm = useConfirm()

const badges = ref<BadgeItem[]>([])
const config = ref<GamificationConfig | null>(null)
const loading = ref(false)
const error = ref('')
const category = ref('')
const actingId = ref<number | null>(null)
const failedImages = ref<number[]>([])
let latest = 0

const isLoggedIn = computed(() => auth.isLoggedIn)
const categories = computed(() => {
  const seen = new Set<string>()
  const result: string[] = []
  for (const badge of badges.value) {
    if (badge.category && !seen.has(badge.category)) {
      seen.add(badge.category)
      result.push(badge.category)
    }
  }
  return result
})
const categoryOptions = computed(() => [
  { label: '全部', value: '' },
  ...categories.value.map((item) => ({ label: item, value: item })),
])
const filtered = computed(() =>
  category.value ? badges.value.filter((badge) => badge.category === category.value) : badges.value,
)
const balance = computed(() => auth.user?.coins ?? 0)
const loginTarget = computed(() => ({ name: 'login', query: { redirect: route.fullPath } }))
const equippedCount = computed(() => badges.value.filter((badge) => badge.equipped).length)
const limitReached = computed(
  () => config.value !== null && equippedCount.value >= config.value.maxEquippedBadges,
)
const equipSummary = computed(() =>
  config.value
    ? `已装备 ${equippedCount.value} / ${config.value.maxEquippedBadges} 个`
    : `已装备 ${equippedCount.value} 个`,
)

function imageSrc(item: BadgeItem): string | undefined {
  return failedImages.value.includes(item.id) ? undefined : badgeImageSrc(item.imageUrl)
}

function onImageError(id: number) {
  if (!failedImages.value.includes(id)) failedImages.value.push(id)
}

function ownedHint(item: BadgeItem): string {
  if (item.equipped) return '该徽章正在公开展示。'
  if (limitReached.value) return '装备已满，请先卸下一个徽章。'
  return '可装备此徽章用于公开展示。'
}

function insufficient(item: BadgeItem): boolean {
  return isLoggedIn.value && auth.user !== null && auth.user.coins < item.priceCoins
}

function shortfall(item: BadgeItem): number {
  return Math.max(0, item.priceCoins - balance.value)
}

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const data = await listBadges()
    if (seq !== latest) return
    badges.value = data
    failedImages.value = []
    if (category.value && !data.some((badge) => badge.category === category.value)) {
      category.value = ''
    }
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载徽章商城失败，请稍后再试'
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

function buy(item: BadgeItem) {
  const price = item.priceCoins > 0 ? `花费 ${formatCoins(item.priceCoins)} 金币` : '免费'
  confirm.require({
    header: '购买徽章',
    message: `确定${price}购买徽章「${item.name}」吗？`,
    icon: 'pi pi-shopping-cart',
    acceptLabel: '购买',
    rejectLabel: '取消',
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => {
      void runAction(item.id, () => purchaseBadge(item.id), '购买成功，可在装扮管理中装备')
    },
  })
}

function toggleEquip(item: BadgeItem) {
  if (item.equipped) {
    void runAction(item.id, () => unequipBadge(item.id), '已卸下徽章')
  } else {
    void runAction(item.id, () => equipBadge(item.id), '已装备徽章')
  }
}

async function loadConfig() {
  try {
    config.value = await getGamificationConfig()
  } catch {
    // 配置加载失败时不做前端上限拦截，仍由服务端在装备时校验。
    config.value = null
  }
}

onMounted(() => {
  void loadConfig()
  void load()
})
// 登录状态变化后重新拉取，以刷新拥有/装备状态与余额。
watch(
  () => auth.isLoggedIn,
  () => void load(),
)
</script>

<template>
  <div class="shop-page">
    <AppPageHeader
      eyebrow="SHOP // BADGES"
      title="徽章商城"
      description="使用金币购买徽章，装备后可在公开页面展示。"
    >
      <template #actions>
        <template v-if="isLoggedIn">
          <Tag :severity="limitReached ? 'warn' : 'success'" :value="equipSummary" />
          <Tag severity="warn" :value="`余额 ${formatCoins(balance)} 金币`" />
        </template>
      </template>
    </AppPageHeader>

    <div v-if="!isLoggedIn" class="login-cta">
      <span>登录后可购买徽章并装备展示。</span>
      <RouterLink :to="loginTarget">
        <Button label="登录 / 注册" icon="pi pi-sign-in" size="small" outlined />
      </RouterLink>
    </div>

    <div v-if="categories.length" class="toolbar">
      <span class="filter-label">分类</span>
      <SelectButton
        v-model="category"
        :options="categoryOptions"
        option-label="label"
        option-value="value"
        :allow-empty="false"
      />
    </div>

    <AppErrorState v-if="error && !badges.length" :description="error" @retry="load" />

    <AppLoadingState v-else-if="loading && !badges.length" label="正在加载徽章商城" />

    <template v-else>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>

      <AppEmptyState
        v-if="!error && filtered.length === 0"
        icon="pi pi-box"
        :title="category ? '该分类暂无徽章' : '暂无徽章'"
        description="新的徽章上架后会显示在这里。"
      />

      <div v-else class="badge-grid">
        <AppSurface
          v-for="item in filtered"
          :key="item.id"
          as="article"
          padding="md"
          class="badge-card"
          :class="{ 'is-equipped': item.equipped }"
        >
          <div class="card-head">
            <Avatar
              :image="imageSrc(item)"
              :label="imageSrc(item) ? undefined : badgeInitial(item.name)"
              size="large"
              shape="square"
              class="badge-icon"
              :aria-label="`徽章图标：${item.name}`"
              @error="onImageError(item.id)"
            />
            <div class="name-wrap">
              <h2 class="badge-name">{{ item.name }}</h2>
              <div class="badge-tags">
                <Tag v-if="item.category" severity="secondary" :value="item.category" />
                <Tag v-if="item.equipped" severity="success" value="装备中" />
                <Tag v-else-if="item.owned" severity="info" value="已拥有" />
                <Tag v-if="!item.isActive" severity="secondary" value="已下线" />
              </div>
            </div>
          </div>

          <p class="badge-desc">{{ item.description || '暂无描述' }}</p>

          <div class="badge-meta">
            <span class="price">
              {{ item.priceCoins > 0 ? `${formatCoins(item.priceCoins)} 金币` : '免费' }}
            </span>
            <span v-if="item.isLimited" class="stock" :class="{ empty: item.stock <= 0 }">
              {{ item.stock > 0 ? `限量 · 剩余 ${item.stock}` : '限量 · 已售罄' }}
            </span>
            <span v-else class="stock">不限量</span>
          </div>

          <div class="card-foot">
            <template v-if="!isLoggedIn">
              <RouterLink :to="loginTarget">
                <Button label="登录后购买" icon="pi pi-sign-in" size="small" outlined />
              </RouterLink>
            </template>
            <template v-else-if="item.owned">
              <Button
                :label="item.equipped ? '卸下' : '装备'"
                size="small"
                outlined
                :loading="actingId === item.id"
                :disabled="
                  (actingId !== null && actingId !== item.id) || (!item.equipped && limitReached)
                "
                @click="toggleEquip(item)"
              />
              <Button label="已拥有" size="small" disabled />
            </template>
            <template v-else>
              <Button
                v-if="item.isLimited && item.stock <= 0"
                label="已售罄"
                size="small"
                disabled
              />
              <Button v-else-if="insufficient(item)" label="余额不足" size="small" disabled />
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
          </div>

          <div v-if="!isLoggedIn" class="hint">登录后才能购买与装备徽章。</div>
          <div v-else-if="item.owned" class="hint">{{ ownedHint(item) }}</div>
          <div v-else-if="item.isLimited && item.stock <= 0" class="hint">
            限量库存已售罄，无法购买。
          </div>
          <div v-else-if="insufficient(item)" class="hint">
            余额不足，还差 {{ formatCoins(shortfall(item)) }} 金币。
          </div>
        </AppSurface>
      </div>
    </template>
  </div>
</template>

<style scoped>
.shop-page {
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

.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-3);
}

.filter-label {
  color: var(--kapoj-text-secondary);
  font-size: 13px;
}

.badge-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: var(--kapoj-space-4);
}

.badge-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
}

.badge-card.is-equipped {
  border-color: color-mix(in srgb, var(--kapoj-success) 45%, var(--kapoj-border));
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--kapoj-success) 18%, transparent);
}

.card-head {
  display: flex;
  align-items: flex-start;
  gap: var(--kapoj-space-3);
}

.badge-icon {
  flex: 0 0 auto;
  width: 56px;
  height: 56px;
  color: var(--kapoj-primary);
  font-size: 1.3rem;
  font-weight: 700;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 26%, transparent);
}

.name-wrap {
  flex: 1;
  min-width: 0;
}

.badge-name {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  line-height: 1.35;
  word-break: break-word;
}

.badge-tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-1);
  margin-top: var(--kapoj-space-1);
}

.badge-desc {
  display: -webkit-box;
  margin: 0;
  overflow: hidden;
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
  line-height: 1.5;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  line-clamp: 2;
}

.badge-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-3);
  padding-top: var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
  border-top: 1px solid var(--kapoj-border);
}

.price {
  color: var(--kapoj-warning);
  font-weight: 600;
}

.stock.empty {
  color: var(--kapoj-danger);
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
  .badge-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .login-cta :deep(.p-button) {
    width: 100%;
  }
}
</style>
