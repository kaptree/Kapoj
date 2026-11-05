<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Message from 'primevue/message'
import Paginator from 'primevue/paginator'
import Tag from 'primevue/tag'
import { useToast } from 'primevue/usetoast'

import {
  equipBadge,
  equipTitle,
  getGamificationConfig,
  listCoinTransactions,
  listTitles,
  listUserBadges,
  unequipBadge,
  unequipTitle,
} from '@/api/gamification'
import { ApiError } from '@/api/http'
import type {
  BadgeItem,
  CoinTransactionItem,
  EquippedBadge,
  EquippedTitle,
  GamificationConfig,
  TitleItem,
} from '@/api/types'
import IdentityShowcase from '@/components/IdentityShowcase.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime } from '@/utils/format'
import {
  badgeImageSrc,
  badgeInitial,
  coinReasonLabel,
  formatCoins,
  safeTitleColor,
} from '@/utils/gamification'

const PAGE_SIZE = 20
const auth = useAuthStore()
const toast = useToast()

const ownedBadges = ref<BadgeItem[]>([])
const ownedTitles = ref<TitleItem[]>([])
const transactions = ref<CoinTransactionItem[]>([])
const config = ref<GamificationConfig | null>(null)
const txTotal = ref(0)
const txPage = ref(1)
const loading = ref(false)
const txLoading = ref(false)
const error = ref('')
const txError = ref('')
const actingKey = ref('')
const failedImages = ref<number[]>([])
let latest = 0

const balance = computed(() => auth.user?.coins ?? 0)
const equippedBadges = computed<EquippedBadge[]>(() =>
  ownedBadges.value
    .filter((badge) => badge.equipped)
    .map((badge) => ({ id: badge.id, name: badge.name, imageUrl: badge.imageUrl })),
)
const equippedTitle = computed<EquippedTitle | null>(() => {
  const title = ownedTitles.value.find((item) => item.equipped)
  return title ? { id: title.id, name: title.name, color: title.color } : null
})
const equippedBadgeCount = computed(
  () => ownedBadges.value.filter((badge) => badge.equipped).length,
)
const badgeLimitReached = computed(
  () => config.value !== null && equippedBadgeCount.value >= config.value.maxEquippedBadges,
)
const badgeEquipSummary = computed(() =>
  config.value
    ? `已装备 ${equippedBadgeCount.value} / ${config.value.maxEquippedBadges} 个`
    : `已装备 ${equippedBadgeCount.value} 个`,
)
const txFirst = computed(() => (txPage.value - 1) * PAGE_SIZE)

function amountText(amount: number): string {
  const sign = amount > 0 ? '+' : amount < 0 ? '-' : ''
  return `${sign}${formatCoins(Math.abs(amount))}`
}

function amountClass(amount: number): string {
  if (amount > 0) return 'income'
  if (amount < 0) return 'expense'
  return ''
}

function badgeLabel(item: BadgeItem): string | undefined {
  return badgeImage(item) ? undefined : badgeInitial(item.name)
}

function badgeImage(item: BadgeItem): string | undefined {
  return failedImages.value.includes(item.id) ? undefined : badgeImageSrc(item.imageUrl)
}

function onBadgeImageError(id: number) {
  if (!failedImages.value.includes(id)) failedImages.value.push(id)
}

async function loadTransactions() {
  txLoading.value = true
  txError.value = ''
  try {
    const data = await listCoinTransactions({ page: txPage.value, pageSize: PAGE_SIZE })
    transactions.value = data.items
    txTotal.value = data.total
  } catch (e) {
    txError.value = e instanceof ApiError ? e.message : '加载金币流水失败，请稍后再试'
  } finally {
    txLoading.value = false
  }
}

async function loadPage() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    await auth.fetchMe()
    const userId = auth.user?.id
    if (!userId) throw new Error('missing user')
    const [badgeData, titleData, configData] = await Promise.all([
      listUserBadges(userId),
      listTitles(),
      getGamificationConfig().catch(() => null),
    ])
    if (seq !== latest) return
    ownedBadges.value = badgeData
    failedImages.value = []
    ownedTitles.value = titleData.filter((title) => title.purchased)
    // 配置加载失败时不做前端上限拦截，装备仍由服务端校验。
    config.value = configData
    await loadTransactions()
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载装扮信息失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

async function refreshBadges() {
  await auth.fetchMe().catch(() => undefined)
  const userId = auth.user?.id
  if (!userId) return
  ownedBadges.value = await listUserBadges(userId)
  failedImages.value = []
}

async function refreshTitles() {
  await auth.fetchMe().catch(() => undefined)
  const data = await listTitles()
  ownedTitles.value = data.filter((title) => title.purchased)
}

async function runBadgeAction(badge: BadgeItem) {
  const key = `badge-${badge.id}`
  actingKey.value = key
  try {
    if (badge.equipped) {
      await unequipBadge(badge.id)
    } else {
      await equipBadge(badge.id)
    }
    toast.add({
      severity: 'success',
      summary: badge.equipped ? '已卸下徽章' : '已装备徽章',
      life: 3000,
    })
    await refreshBadges()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '操作失败',
      detail: e instanceof ApiError ? e.message : '操作失败，请稍后再试',
      life: 4000,
    })
  } finally {
    actingKey.value = ''
  }
}

async function runTitleAction(title: TitleItem) {
  const key = `title-${title.id}`
  actingKey.value = key
  try {
    if (title.equipped) {
      await unequipTitle(title.id)
    } else {
      await equipTitle(title.id)
    }
    toast.add({
      severity: 'success',
      summary: title.equipped ? '已卸下称号' : '已装备称号，原称号已自动卸下',
      life: 3000,
    })
    await refreshTitles()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '操作失败',
      detail: e instanceof ApiError ? e.message : '操作失败，请稍后再试',
      life: 4000,
    })
  } finally {
    actingKey.value = ''
  }
}

function onTxPage(event: { page: number }) {
  txPage.value = event.page + 1
  void loadTransactions()
}

onMounted(loadPage)
// 同一个用户刷新资料时不重复加载；切换账号（如另一标签页登录）时重新加载。
watch(
  () => auth.user?.id,
  (id, previous) => {
    if (id !== previous) void loadPage()
  },
)
</script>

<template>
  <div class="appearance-page">
    <AppPageHeader
      eyebrow="APPEARANCE // MANAGER"
      title="装扮管理"
      description="管理已拥有的徽章与称号，并查看金币流水。"
    />

    <AppErrorState
      v-if="error && !ownedBadges.length && !ownedTitles.length"
      :description="error"
      @retry="loadPage"
    />

    <AppLoadingState v-else-if="loading" label="正在加载装扮信息" />

    <template v-else>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>

      <AppSurface padding="lg" class="overview-surface">
        <div class="overview-grid">
          <div class="overview-panel">
            <span class="overview-label">金币余额</span>
            <strong class="balance-value"> {{ formatCoins(balance) }} <span>金币</span> </strong>
            <span class="muted">余额以服务端返回为准</span>
          </div>
          <div class="overview-panel">
            <span class="overview-label">当前装扮</span>
            <IdentityShowcase
              v-if="equippedBadges.length || equippedTitle"
              :badges="equippedBadges"
              :title="equippedTitle"
              class="overview-identity"
            />
            <span v-else class="muted">尚未装备徽章或称号</span>
          </div>
        </div>
      </AppSurface>

      <AppSurface padding="lg" class="appearance-section">
        <div class="section-head">
          <div>
            <span class="section-title">我的徽章</span>
            <span class="section-sub">{{ badgeEquipSummary }}</span>
          </div>
          <RouterLink to="/shop/badges">
            <Button label="前往徽章商城" text size="small" icon="pi pi-arrow-right" />
          </RouterLink>
        </div>

        <div v-if="badgeLimitReached" class="limit-hint">装备已满，请先卸下一个徽章。</div>

        <AppEmptyState
          v-if="ownedBadges.length === 0"
          icon="pi pi-box"
          title="暂无徽章，去商城看看吧"
          description="购买徽章后可以在这里装备展示。"
        />
        <div v-else class="badge-grid">
          <div
            v-for="item in ownedBadges"
            :key="item.id"
            class="owned-card"
            :class="{ 'is-equipped': item.equipped }"
          >
            <Avatar
              :image="badgeImage(item)"
              :label="badgeLabel(item)"
              size="large"
              shape="square"
              class="owned-icon"
              :aria-label="`徽章图标：${item.name}`"
              @error="onBadgeImageError(item.id)"
            />
            <div class="owned-main">
              <span class="owned-name">{{ item.name }}</span>
              <div class="owned-tags">
                <Tag v-if="item.equipped" severity="success" value="装备中" />
                <Tag v-if="!item.isActive" severity="secondary" value="已下线" />
              </div>
            </div>
            <Button
              :label="item.equipped ? '卸下' : '装备'"
              size="small"
              outlined
              :loading="actingKey === `badge-${item.id}`"
              :disabled="
                (actingKey !== '' && actingKey !== `badge-${item.id}`) ||
                (!item.equipped && badgeLimitReached)
              "
              @click="runBadgeAction(item)"
            />
          </div>
        </div>
      </AppSurface>

      <AppSurface padding="lg" class="appearance-section">
        <div class="section-head">
          <div>
            <span class="section-title">我的称号</span>
            <span class="section-sub">已购买 {{ ownedTitles.length }} 个</span>
          </div>
          <RouterLink to="/shop/titles">
            <Button label="前往称号商店" text size="small" icon="pi pi-arrow-right" />
          </RouterLink>
        </div>

        <AppEmptyState
          v-if="ownedTitles.length === 0"
          icon="pi pi-id-card"
          title="暂无称号，去商店看看吧"
          description="购买称号后可以在这里装备展示。"
        />
        <div v-else class="title-list">
          <div
            v-for="item in ownedTitles"
            :key="item.id"
            class="owned-title"
            :class="{ 'is-equipped': item.equipped }"
          >
            <div class="title-main">
              <span
                class="title-name"
                :style="{ color: safeTitleColor(item.color, 'var(--kapoj-text)') }"
              >
                {{ item.name }}
              </span>
              <Tag v-if="item.equipped" severity="success" value="装备中" />
              <Tag v-if="!item.isActive" severity="secondary" value="已下线" />
            </div>
            <Button
              :label="item.equipped ? '卸下' : '装备'"
              size="small"
              outlined
              :loading="actingKey === `title-${item.id}`"
              :disabled="actingKey !== '' && actingKey !== `title-${item.id}`"
              @click="runTitleAction(item)"
            />
          </div>
        </div>
      </AppSurface>

      <AppSurface padding="lg" class="appearance-section">
        <div class="section-head">
          <div>
            <span class="section-title">金币流水</span>
            <span class="section-sub">共 {{ txTotal }} 条</span>
          </div>
        </div>

        <AppErrorState
          v-if="txError && !transactions.length"
          :description="txError"
          @retry="loadTransactions"
        />
        <template v-else>
          <Message v-if="txError" severity="error" :closable="false">
            {{ txError }}
          </Message>
          <AppLoadingState v-if="txLoading && !transactions.length" label="正在加载金币流水" />
          <AppEmptyState
            v-else-if="transactions.length === 0"
            icon="pi pi-coins"
            title="暂无金币流水"
            description="获得或消费金币后会显示在这里。"
          />
          <template v-else>
            <DataTable
              :value="transactions"
              :loading="txLoading"
              data-key="id"
              size="small"
              responsive-layout="scroll"
              class="transactions-table"
            >
              <Column header="时间" style="min-width: 180px">
                <template #body="{ data }">{{ formatDateTime(data.createdAt) }}</template>
              </Column>
              <Column header="原因" style="min-width: 180px">
                <template #body="{ data }">
                  <div>{{ coinReasonLabel(data.reason) }}</div>
                  <div v-if="data.note" class="note">{{ data.note }}</div>
                </template>
              </Column>
              <Column header="金额" style="width: 120px">
                <template #body="{ data }">
                  <span class="amount" :class="amountClass(data.amount)">
                    {{ amountText(data.amount) }}
                  </span>
                </template>
              </Column>
            </DataTable>
            <Paginator
              v-if="txTotal > PAGE_SIZE"
              :rows="PAGE_SIZE"
              :total-records="txTotal"
              :first="txFirst"
              :page-link-size="5"
              template="PrevPageLink PageLinks NextPageLink"
              class="pager"
              @page="onTxPage"
            />
          </template>
        </template>
      </AppSurface>
    </template>
  </div>
</template>

<style scoped>
.appearance-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.overview-grid {
  display: grid;
  grid-template-columns: minmax(200px, 260px) minmax(240px, 1fr);
  gap: var(--kapoj-space-4);
}

.overview-panel {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
  min-width: 0;
  padding: var(--kapoj-space-4);
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.overview-label {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.balance-value {
  color: var(--kapoj-warning);
  font-size: 1.75rem;
  line-height: 1.2;
}

.balance-value span {
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
  font-weight: 400;
}

.overview-identity {
  margin-top: var(--kapoj-space-1);
}

.overview-identity :deep(.identity-title) {
  font-size: 18px;
}

.muted {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.appearance-section {
  min-width: 0;
}

.section-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
  margin-bottom: var(--kapoj-space-3);
}

.section-title {
  color: var(--kapoj-text);
  font-size: 1rem;
  font-weight: 600;
}

.section-sub {
  margin-left: var(--kapoj-space-2);
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.limit-hint {
  margin-bottom: var(--kapoj-space-2);
  color: var(--kapoj-warning);
  font-size: 12px;
}

.badge-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: var(--kapoj-space-3);
}

.owned-card {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-3);
  min-width: 0;
  padding: var(--kapoj-space-3);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.owned-card.is-equipped,
.owned-title.is-equipped {
  border-color: color-mix(in srgb, var(--kapoj-success) 45%, var(--kapoj-border));
}

.owned-icon {
  flex: 0 0 auto;
  color: var(--kapoj-primary);
  font-size: 1.1rem;
  font-weight: 700;
  background: var(--kapoj-primary-soft);
}

.owned-main {
  flex: 1;
  min-width: 0;
}

.owned-name {
  color: var(--kapoj-text);
  font-weight: 600;
  word-break: break-word;
}

.owned-tags,
.title-main {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.owned-tags {
  gap: var(--kapoj-space-1);
  margin-top: var(--kapoj-space-1);
}

.title-list {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
}

.owned-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-3);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.title-name {
  font-size: 1rem;
  font-weight: 600;
  word-break: break-word;
}

.transactions-table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.transactions-table :deep(.p-datatable-thead > tr > th) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  background: var(--kapoj-bg-elevated);
  border-color: var(--kapoj-border);
}

.transactions-table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.note {
  margin-top: 2px;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.amount {
  font-family: var(--kapoj-font-mono);
  font-weight: 600;
}

.amount.income {
  color: var(--kapoj-success);
}

.amount.expense {
  color: var(--kapoj-danger);
}

.pager {
  align-self: flex-end;
  margin-top: var(--kapoj-space-2);
}

@media (max-width: 640px) {
  .overview-grid,
  .badge-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .owned-title {
    align-items: flex-start;
    flex-direction: column;
  }

  .pager {
    align-self: stretch;
  }
}
</style>
