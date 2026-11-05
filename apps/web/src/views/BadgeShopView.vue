<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'

import {
  equipBadge,
  getGamificationConfig,
  listBadges,
  purchaseBadge,
  unequipBadge,
} from '@/api/gamification'
import { ApiError } from '@/api/http'
import type { BadgeItem, GamificationConfig } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { badgeImageSrc, badgeInitial, formatCoins } from '@/utils/gamification'

const route = useRoute()
const auth = useAuthStore()

const badges = ref<BadgeItem[]>([])
const config = ref<GamificationConfig | null>(null)
const loading = ref(false)
const error = ref('')
const category = ref('')
const actingId = ref<number | null>(null)
let latest = 0

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

function ownedHint(item: BadgeItem): string {
  if (item.equipped) return '该徽章正在公开展示。'
  if (limitReached.value) return '装备已满，请先卸下一个徽章。'
  return '可装备此徽章用于公开展示。'
}

function insufficient(item: BadgeItem): boolean {
  return auth.isLoggedIn && auth.user !== null && auth.user.coins < item.priceCoins
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

async function runAction(id: number, action: () => Promise<unknown>, successText: string) {
  actingId.value = id
  try {
    await action()
    ElMessage.success(successText)
    await auth.fetchMe().catch(() => undefined)
    await load()
  } catch (e) {
    ElMessage.error(e instanceof ApiError ? e.message : '操作失败，请稍后再试')
  } finally {
    actingId.value = null
  }
}

async function buy(item: BadgeItem) {
  const price = item.priceCoins > 0 ? `花费 ${formatCoins(item.priceCoins)} 金币` : '免费'
  try {
    await ElMessageBox.confirm(`确定${price}购买徽章「${item.name}」吗？`, '购买徽章', {
      type: 'info',
      confirmButtonText: '购买',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  await runAction(item.id, () => purchaseBadge(item.id), '购买成功，可在装扮管理中装备')
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
  <el-card shadow="never">
    <template #header>
      <div class="head">
        <div>
          <span class="page-title">徽章商城</span>
          <span class="sub">使用金币购买徽章，装备后可在公开页面展示</span>
        </div>
        <div v-if="auth.isLoggedIn" class="head-tags">
          <el-tag :type="limitReached ? 'warning' : 'success'" effect="plain">
            {{ equipSummary }}
          </el-tag>
          <el-tag type="warning" effect="plain"> 余额 {{ formatCoins(balance) }} 金币 </el-tag>
        </div>
      </div>
    </template>

    <div v-if="!auth.isLoggedIn" class="cta">
      <span>登录后可购买徽章并装备展示。</span>
      <RouterLink :to="loginTarget">
        <el-button type="primary" plain size="small">登录 / 注册</el-button>
      </RouterLink>
    </div>

    <div v-if="categories.length" class="toolbar">
      <span class="filter-label">分类</span>
      <el-radio-group v-model="category" class="category-filter">
        <el-radio-button value="">全部</el-radio-button>
        <el-radio-button v-for="item in categories" :key="item" :value="item">
          {{ item }}
        </el-radio-button>
      </el-radio-group>
    </div>

    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" class="banner" />

    <div v-loading="loading" class="grid-wrap">
      <el-empty
        v-if="!loading && !error && filtered.length === 0"
        :description="category ? '该分类暂无徽章' : '暂无徽章'"
      />
      <div v-else class="grid">
        <el-card
          v-for="item in filtered"
          :key="item.id"
          shadow="hover"
          class="badge-card"
          :class="{ equipped: item.equipped }"
        >
          <div class="card-head">
            <el-avatar :size="56" shape="square" :src="badgeImageSrc(item.imageUrl)" class="icon">
              {{ badgeInitial(item.name) }}
            </el-avatar>
            <div class="name-wrap">
              <div class="name">{{ item.name }}</div>
              <div class="tags">
                <el-tag v-if="item.category" size="small" effect="plain">{{
                  item.category
                }}</el-tag>
                <el-tag v-if="item.equipped" type="success" size="small">装备中</el-tag>
                <el-tag v-else-if="item.owned" type="info" size="small">已拥有</el-tag>
                <el-tag v-if="!item.isActive" type="info" size="small">已下线</el-tag>
              </div>
            </div>
          </div>

          <div class="desc">{{ item.description || '暂无描述' }}</div>

          <div class="meta">
            <span class="price">
              {{ item.priceCoins > 0 ? `${formatCoins(item.priceCoins)} 金币` : '免费' }}
            </span>
            <span v-if="item.isLimited" class="stock" :class="{ empty: item.stock <= 0 }">
              {{ item.stock > 0 ? `限量 · 剩余 ${item.stock}` : '限量 · 已售罄' }}
            </span>
            <span v-else class="stock">不限量</span>
          </div>

          <div class="card-foot">
            <template v-if="!auth.isLoggedIn">
              <RouterLink :to="loginTarget">
                <el-button type="primary" plain size="small">登录后购买</el-button>
              </RouterLink>
            </template>
            <template v-else-if="item.owned">
              <el-button
                type="primary"
                plain
                size="small"
                :loading="actingId === item.id"
                :disabled="
                  (actingId !== null && actingId !== item.id) || (!item.equipped && limitReached)
                "
                @click="toggleEquip(item)"
              >
                {{ item.equipped ? '卸下' : '装备' }}
              </el-button>
              <el-button disabled size="small">已拥有</el-button>
            </template>
            <template v-else>
              <el-button v-if="item.isLimited && item.stock <= 0" disabled size="small">
                已售罄
              </el-button>
              <el-button v-else-if="insufficient(item)" disabled size="small">余额不足</el-button>
              <el-button
                v-else
                type="primary"
                size="small"
                :loading="actingId === item.id"
                :disabled="actingId !== null && actingId !== item.id"
                @click="buy(item)"
              >
                购买
              </el-button>
            </template>
          </div>

          <div v-if="!auth.isLoggedIn" class="hint">登录后才能购买与装备徽章。</div>
          <div v-else-if="item.owned" class="hint">{{ ownedHint(item) }}</div>
          <div v-else-if="item.isLimited && item.stock <= 0" class="hint">
            限量库存已售罄，无法购买。
          </div>
          <div v-else-if="insufficient(item)" class="hint">
            余额不足，还差 {{ formatCoins(shortfall(item)) }} 金币。
          </div>
        </el-card>
      </div>
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
.head-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.sub {
  margin-left: 10px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.cta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px 14px;
  margin-bottom: 16px;
  color: var(--el-text-color-regular);
  background: var(--el-fill-color-light);
  border-radius: 6px;
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin-bottom: 16px;
}
.filter-label {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.category-filter {
  display: flex;
  flex-wrap: wrap;
}
.banner {
  margin-bottom: 12px;
}
.grid-wrap {
  min-height: 180px;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}
.badge-card {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.badge-card.equipped {
  border-color: var(--el-color-success);
}
.card-head {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}
.icon {
  flex: none;
  font-size: 20px;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}
.name-wrap {
  flex: 1;
  min-width: 0;
}
.name {
  font-size: 16px;
  font-weight: 600;
  word-break: break-word;
}
.tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 6px;
}
.desc {
  margin-top: 12px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.5;
  display: -webkit-box;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  line-clamp: 2;
}
.meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  padding-top: 12px;
  margin-top: 12px;
  font-size: 13px;
  border-top: 1px solid var(--el-border-color-lighter);
}
.price {
  font-weight: 600;
  color: var(--el-color-warning);
}
.stock {
  color: var(--el-text-color-secondary);
}
.stock.empty {
  color: var(--el-color-danger);
}
.card-foot {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  margin-top: auto;
  padding-top: 12px;
}
.hint {
  margin-top: 8px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
@media (max-width: 640px) {
  .grid {
    grid-template-columns: 1fr;
  }
  .sub {
    display: block;
    margin: 4px 0 0;
  }
}
</style>
