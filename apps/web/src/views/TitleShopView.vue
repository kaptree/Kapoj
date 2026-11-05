<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'

import { equipTitle, listTitles, purchaseTitle, unequipTitle } from '@/api/gamification'
import { ApiError } from '@/api/http'
import type { TitleItem } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { formatCoins, safeTitleColor, titleUnlockLabel } from '@/utils/gamification'

const route = useRoute()
const auth = useAuthStore()

const titles = ref<TitleItem[]>([])
const loading = ref(false)
const error = ref('')
const actingId = ref<number | null>(null)
let latest = 0

const balance = computed(() => auth.user?.coins ?? 0)
const loginTarget = computed(() => ({ name: 'login', query: { redirect: route.fullPath } }))

function insufficient(item: TitleItem): boolean {
  return auth.isLoggedIn && auth.user !== null && auth.user.coins < item.priceCoins
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

async function buy(item: TitleItem) {
  const price = item.priceCoins > 0 ? `花费 ${formatCoins(item.priceCoins)} 金币` : '免费'
  try {
    await ElMessageBox.confirm(`确定${price}购买称号「${item.name}」吗？`, '购买称号', {
      type: 'info',
      confirmButtonText: '购买',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  await runAction(item.id, () => purchaseTitle(item.id), '购买成功，装备后即可展示')
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
  <el-card shadow="never">
    <template #header>
      <div class="head">
        <div>
          <span class="page-title">称号商店</span>
          <span class="sub">达成解锁条件后，使用金币购买并装备专属称号</span>
        </div>
        <el-tag v-if="auth.isLoggedIn" type="warning" effect="plain">
          余额 {{ formatCoins(balance) }} 金币
        </el-tag>
      </div>
    </template>

    <div v-if="!auth.isLoggedIn" class="cta">
      <span>登录后可查看解锁进度、购买并装备称号。</span>
      <RouterLink :to="loginTarget">
        <el-button type="primary" plain size="small">登录 / 注册</el-button>
      </RouterLink>
    </div>

    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" class="banner" />

    <div v-loading="loading" class="grid-wrap">
      <el-empty v-if="!loading && !error && titles.length === 0" description="暂无称号" />
      <div v-else class="grid">
        <el-card
          v-for="item in titles"
          :key="item.id"
          shadow="hover"
          class="title-card"
          :class="{ equipped: item.equipped }"
        >
          <div class="card-head">
            <div class="title-wrap">
              <div class="title-name" :style="{ color: safeTitleColor(item.color) }">
                {{ item.name }}
              </div>
              <div class="tags">
                <el-tag v-if="item.equipped" type="success" size="small">装备中</el-tag>
                <el-tag v-else-if="item.purchased" type="primary" size="small">已购买</el-tag>
                <el-tag v-else-if="item.unlocked" type="warning" size="small">已解锁</el-tag>
                <el-tag v-else type="info" size="small">未解锁</el-tag>
                <el-tag v-if="!item.isActive" type="info" size="small">已下线</el-tag>
              </div>
            </div>
            <span class="price">
              {{ item.priceCoins > 0 ? `${formatCoins(item.priceCoins)} 金币` : '免费' }}
            </span>
          </div>

          <div class="condition">
            <div class="condition-head">
              <span>{{ conditionText(item) }}</span>
              <span>{{ item.progress.current }} / {{ item.progress.target }}</span>
            </div>
            <el-progress
              :percentage="progressPercent(item)"
              :status="item.progress.done ? 'success' : undefined"
              :stroke-width="8"
              :show-text="false"
            />
          </div>

          <div class="card-foot">
            <template v-if="!auth.isLoggedIn">
              <RouterLink :to="loginTarget">
                <el-button type="primary" plain size="small">登录后查看</el-button>
              </RouterLink>
            </template>
            <template v-else-if="item.equipped">
              <el-button
                size="small"
                :loading="actingId === item.id"
                :disabled="actingId !== null && actingId !== item.id"
                @click="toggleEquip(item)"
              >
                卸下
              </el-button>
            </template>
            <template v-else-if="item.purchased">
              <el-button
                type="primary"
                plain
                size="small"
                :loading="actingId === item.id"
                :disabled="actingId !== null && actingId !== item.id"
                @click="toggleEquip(item)"
              >
                装备
              </el-button>
            </template>
            <template v-else-if="item.unlocked">
              <el-button v-if="insufficient(item)" disabled size="small">余额不足</el-button>
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
            <template v-else>
              <el-button disabled size="small">未解锁</el-button>
            </template>
          </div>

          <div v-if="!auth.isLoggedIn" class="hint">登录后可查看自己的解锁进度。</div>
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
.title-card {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.title-card.equipped {
  border-color: var(--el-color-success);
}
.card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}
.title-wrap {
  flex: 1;
  min-width: 0;
}
.title-name {
  font-size: 20px;
  font-weight: 700;
  word-break: break-word;
}
.tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}
.price {
  flex: none;
  font-size: 13px;
  font-weight: 600;
  color: var(--el-color-warning);
}
.condition {
  padding-top: 12px;
  margin-top: 14px;
  border-top: 1px solid var(--el-border-color-lighter);
}
.condition-head {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
  color: var(--el-text-color-regular);
  font-size: 12px;
}
.card-foot {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: auto;
  padding-top: 14px;
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
