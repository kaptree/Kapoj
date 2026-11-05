<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'

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

function amountText(amount: number): string {
  const sign = amount > 0 ? '+' : amount < 0 ? '-' : ''
  return `${sign}${formatCoins(Math.abs(amount))}`
}

function amountClass(amount: number): string {
  if (amount > 0) return 'income'
  if (amount < 0) return 'expense'
  return ''
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
    ElMessage.success(badge.equipped ? '已卸下徽章' : '已装备徽章')
    await refreshBadges()
  } catch (e) {
    ElMessage.error(e instanceof ApiError ? e.message : '操作失败，请稍后再试')
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
    ElMessage.success(title.equipped ? '已卸下称号' : '已装备称号，原称号已自动卸下')
    await refreshTitles()
  } catch (e) {
    ElMessage.error(e instanceof ApiError ? e.message : '操作失败，请稍后再试')
  } finally {
    actingKey.value = ''
  }
}

function onTxPageChange(page: number) {
  txPage.value = page
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
  <el-card shadow="never">
    <template #header>
      <div class="head">
        <div>
          <span class="page-title">装扮管理</span>
          <span class="sub">管理已拥有的徽章与称号，并查看金币流水</span>
        </div>
      </div>
    </template>

    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" class="banner" />

    <div v-loading="loading" class="content">
      <div class="overview">
        <div class="overview-panel">
          <div class="overview-label">金币余额</div>
          <div class="balance-value">{{ formatCoins(balance) }} <span>金币</span></div>
          <div class="muted">余额以服务端返回为准</div>
        </div>
        <div class="overview-panel">
          <div class="overview-label">当前装扮</div>
          <IdentityShowcase
            v-if="equippedBadges.length || equippedTitle"
            :badges="equippedBadges"
            :title="equippedTitle"
          />
          <span v-else class="muted">尚未装备徽章或称号</span>
        </div>
      </div>

      <section class="section">
        <div class="section-head">
          <div>
            <span class="section-title">我的徽章</span>
            <span class="section-sub">{{ badgeEquipSummary }}</span>
          </div>
          <RouterLink to="/shop/badges">
            <el-button text type="primary" size="small">前往徽章商城</el-button>
          </RouterLink>
        </div>

        <div v-if="badgeLimitReached" class="limit-hint">装备已满，请先卸下一个徽章。</div>

        <el-empty v-if="ownedBadges.length === 0" description="暂无徽章，去商城看看吧" />
        <div v-else class="badge-grid">
          <div
            v-for="badge in ownedBadges"
            :key="badge.id"
            class="owned-card"
            :class="{ equipped: badge.equipped }"
          >
            <el-avatar :size="48" shape="square" :src="badgeImageSrc(badge.imageUrl)" class="icon">
              {{ badgeInitial(badge.name) }}
            </el-avatar>
            <div class="owned-main">
              <div class="owned-name">{{ badge.name }}</div>
              <div class="tags">
                <el-tag v-if="badge.equipped" type="success" size="small">装备中</el-tag>
                <el-tag v-if="!badge.isActive" type="info" size="small">已下线</el-tag>
              </div>
            </div>
            <el-button
              size="small"
              :type="badge.equipped ? 'info' : 'primary'"
              :loading="actingKey === `badge-${badge.id}`"
              :disabled="
                (actingKey !== '' && actingKey !== `badge-${badge.id}`) ||
                (!badge.equipped && badgeLimitReached)
              "
              @click="runBadgeAction(badge)"
            >
              {{ badge.equipped ? '卸下' : '装备' }}
            </el-button>
          </div>
        </div>
      </section>

      <section class="section">
        <div class="section-head">
          <div>
            <span class="section-title">我的称号</span>
            <span class="section-sub">已购买 {{ ownedTitles.length }} 个</span>
          </div>
          <RouterLink to="/shop/titles">
            <el-button text type="primary" size="small">前往称号商店</el-button>
          </RouterLink>
        </div>

        <el-empty v-if="ownedTitles.length === 0" description="暂无称号，去商店看看吧" />
        <div v-else class="title-list">
          <div
            v-for="title in ownedTitles"
            :key="title.id"
            class="owned-title"
            :class="{ equipped: title.equipped }"
          >
            <div class="title-main">
              <span class="title-name" :style="{ color: safeTitleColor(title.color) }">
                {{ title.name }}
              </span>
              <el-tag v-if="title.equipped" type="success" size="small">装备中</el-tag>
              <el-tag v-if="!title.isActive" type="info" size="small">已下线</el-tag>
            </div>
            <el-button
              size="small"
              :type="title.equipped ? 'info' : 'primary'"
              :loading="actingKey === `title-${title.id}`"
              :disabled="actingKey !== '' && actingKey !== `title-${title.id}`"
              @click="runTitleAction(title)"
            >
              {{ title.equipped ? '卸下' : '装备' }}
            </el-button>
          </div>
        </div>
      </section>

      <section class="section">
        <div class="section-head">
          <div>
            <span class="section-title">金币流水</span>
            <span class="section-sub">共 {{ txTotal }} 条</span>
          </div>
        </div>

        <el-alert
          v-if="txError"
          :title="txError"
          type="error"
          show-icon
          :closable="false"
          class="banner"
        />

        <div v-loading="txLoading">
          <el-empty v-if="!txLoading && transactions.length === 0" description="暂无金币流水" />
          <template v-else>
            <el-table :data="transactions" stripe>
              <el-table-column label="时间" min-width="180">
                <template #default="{ row }">{{ formatDateTime(row.createdAt) }}</template>
              </el-table-column>
              <el-table-column label="原因" min-width="180">
                <template #default="{ row }">
                  <div>{{ coinReasonLabel(row.reason) }}</div>
                  <div v-if="row.note" class="note">{{ row.note }}</div>
                </template>
              </el-table-column>
              <el-table-column label="金额" width="120" align="right">
                <template #default="{ row }">
                  <span class="amount" :class="amountClass(row.amount)">
                    {{ amountText(row.amount) }}
                  </span>
                </template>
              </el-table-column>
            </el-table>
            <div v-if="txTotal > PAGE_SIZE" class="pager">
              <el-pagination
                layout="prev, pager, next, total"
                :current-page="txPage"
                :page-size="PAGE_SIZE"
                :total="txTotal"
                @current-change="onTxPageChange"
              />
            </div>
          </template>
        </div>
      </section>
    </div>
  </el-card>
</template>

<style scoped>
.head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.page-title {
  font-size: 18px;
  font-weight: 600;
}
.sub {
  margin-left: 10px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.banner {
  margin-bottom: 12px;
}
.content {
  min-height: 240px;
}
.overview {
  display: grid;
  grid-template-columns: minmax(200px, 260px) minmax(240px, 1fr);
  gap: 16px;
}
.overview-panel {
  padding: 16px;
  background: var(--el-fill-color-light);
  border-radius: 8px;
}
.overview-label {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.balance-value {
  margin-top: 6px;
  font-size: 28px;
  font-weight: 700;
  color: var(--el-color-warning);
  line-height: 1.2;
}
.balance-value span {
  font-size: 13px;
  font-weight: 400;
  color: var(--el-text-color-secondary);
}
.overview-panel :deep(.identity-title) {
  font-size: 18px;
}
.muted {
  margin-top: 6px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.section {
  padding-top: 18px;
  margin-top: 18px;
  border-top: 1px solid var(--el-border-color-lighter);
}
.section-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
}
.section-title {
  font-size: 16px;
  font-weight: 600;
}
.section-sub {
  margin-left: 10px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.limit-hint {
  margin-bottom: 10px;
  color: var(--el-color-warning);
  font-size: 12px;
}
.badge-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 12px;
}
.owned-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 8px;
}
.owned-card.equipped,
.owned-title.equipped {
  border-color: var(--el-color-success);
}
.icon {
  flex: none;
  font-size: 18px;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}
.owned-main {
  flex: 1;
  min-width: 0;
}
.owned-name {
  font-weight: 600;
  word-break: break-word;
}
.tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 4px;
}
.title-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.owned-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 8px;
}
.title-main {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.title-name {
  font-size: 16px;
  font-weight: 600;
  word-break: break-word;
}
.note {
  margin-top: 2px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.amount {
  font-weight: 600;
}
.amount.income {
  color: var(--el-color-success);
}
.amount.expense {
  color: var(--el-color-danger);
}
.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}
@media (max-width: 640px) {
  .overview {
    grid-template-columns: 1fr;
  }
  .badge-grid {
    grid-template-columns: 1fr;
  }
  .sub {
    display: block;
    margin: 4px 0 0;
  }
}
</style>
