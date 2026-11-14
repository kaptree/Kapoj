<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Button from 'primevue/button'
import Tag from 'primevue/tag'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import {
  getLearningTree,
  purchaseLearningPath,
  recommendLearningPath,
  unrecommendLearningPath,
} from '@/api/learning'
import type { LearningPathNodeItem, LearningTree } from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import LearningMobileTree from '@/components/learning/LearningMobileTree.vue'
import LearningNodePanel from '@/components/learning/LearningNodePanel.vue'
import LearningPathSummary from '@/components/learning/LearningPathSummary.vue'
import LearningTreeCanvas from '@/components/learning/LearningTreeCanvas.vue'
import MarkdownView from '@/components/MarkdownView.vue'
import { useAuthStore } from '@/stores/auth'
import { useMediaQuery } from '@/composables/useMediaQuery'
import { useTheme } from '@/theme/theme'
import {
  findLearningNode,
  flattenLearningNodes,
  learningPathVisibilityLabel,
} from '@/utils/learning'

const route = useRoute()
const auth = useAuthStore()
const toast = useToast()
const { isDark } = useTheme()
const isMobile = useMediaQuery('(max-width: 899px)')
const showSolved = computed(() => auth.isLoggedIn)
const isLoggedIn = computed(() => auth.isLoggedIn)
const loginRedirect = computed(() => route.fullPath)

const id = computed(() => {
  const raw = route.params.id
  return typeof raw === 'string' ? raw : ''
})

const path = ref<LearningTree['path'] | null>(null)
const nodes = ref<LearningPathNodeItem[]>([])
const locked = ref(false)
const loading = ref(false)
const error = ref('')
const purchasing = ref(false)
const recommending = ref(false)
const selectedId = ref<number | null>(null)
const descriptionExpanded = ref(false)
const panelRef = ref<{ scrollToProblems: () => void } | null>(null)

const selectedNode = computed(() =>
  selectedId.value === null ? null : (findLearningNode(nodes.value, selectedId.value) ?? null),
)

const selectedIndex = computed(() => {
  if (selectedId.value === null) return null
  const index = flattenLearningNodes(nodes.value).findIndex((node) => node.id === selectedId.value)
  return index < 0 ? null : index + 1
})

let latest = 0

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const data = await getLearningTree(id.value)
    if (seq !== latest) return
    path.value = data.path
    nodes.value = data.nodes
    locked.value = Boolean(data.locked)
    const flatNodes = flattenLearningNodes(data.nodes)
    selectedId.value =
      (flatNodes.find((node) => node.status === 'active') ?? flatNodes[0])?.id ?? null
  } catch (e) {
    if (seq !== latest) return
    if (e instanceof ApiError && e.code === 40400) {
      error.value = '学习路线不存在或无权查看'
    } else {
      error.value = '加载学习路线失败，请稍后再试'
    }
  } finally {
    if (seq === latest) loading.value = false
  }
}

watch(
  () => route.params.id,
  () => {
    path.value = null
    nodes.value = []
    locked.value = false
    selectedId.value = null
    descriptionExpanded.value = false
    error.value = ''
    void load()
  },
  { immediate: true },
)

function selectNode(node: LearningPathNodeItem) {
  selectedId.value = node.id
}

function onMore(node: LearningPathNodeItem) {
  selectNode(node)
  void nextTick(() => panelRef.value?.scrollToProblems())
}

function visibilitySeverity(): 'info' | 'warn' {
  return path.value?.isPublic ? 'info' : 'warn'
}

const priceLabel = computed(() =>
  path.value && path.value.priceCoins > 0 ? `${path.value.priceCoins} 金币` : '免费',
)

const canRecommend = computed(
  () =>
    auth.isLoggedIn &&
    !!path.value &&
    path.value.author.id !== auth.user?.id &&
    (!locked.value || path.value.purchased),
)

async function buyPath() {
  if (!path.value || purchasing.value) return
  purchasing.value = true
  try {
    const result = await purchaseLearningPath(path.value.id)
    path.value = result.path
    try {
      await auth.fetchMe()
    } catch {
      // The purchase already succeeded; a stale coin badge is recoverable.
    }
    toast.add({ severity: 'success', summary: '购买成功', life: 3000 })
    await load()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '购买失败',
      detail: e instanceof ApiError ? e.message : '购买失败，请稍后再试',
      life: 4000,
    })
  } finally {
    purchasing.value = false
  }
}

async function toggleRecommend() {
  if (!path.value || recommending.value) return
  recommending.value = true
  try {
    path.value = path.value.recommendedByMe
      ? await unrecommendLearningPath(path.value.id)
      : await recommendLearningPath(path.value.id)
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '操作失败',
      detail: e instanceof ApiError ? e.message : '推荐操作失败，请稍后再试',
      life: 4000,
    })
  } finally {
    recommending.value = false
  }
}
</script>

<template>
  <div class="detail-page">
    <AppPageHeader eyebrow="LEARNING // PATH" :title="path?.title ?? '学习路线'">
      <template #actions>
        <Button
          as="router-link"
          to="/learning-paths"
          label="返回列表"
          icon="pi pi-arrow-left"
          severity="secondary"
          outlined
        />
      </template>
    </AppPageHeader>

    <template v-if="path">
      <div class="path-overview">
        <div class="path-visibility">
          <Tag :value="learningPathVisibilityLabel(path)" :severity="visibilitySeverity()" />
          <Tag
            :value="priceLabel"
            :severity="path.priceCoins > 0 ? 'warn' : 'success'"
            data-test="path-price"
          />
          <span class="path-count" data-test="path-participants"
            >参与 {{ path.participantCount }}</span
          >
          <span class="path-count" data-test="path-recommends">推荐 {{ path.recommendCount }}</span>
          <Button
            type="button"
            size="small"
            :label="path.recommendedByMe ? '取消推荐' : '推荐'"
            :icon="path.recommendedByMe ? 'pi pi-thumbs-down' : 'pi pi-thumbs-up'"
            :severity="path.recommendedByMe ? 'secondary' : 'primary'"
            :outlined="path.recommendedByMe"
            :disabled="!canRecommend"
            :loading="recommending"
            data-test="path-recommend"
            @click="toggleRecommend"
          />
        </div>
        <LearningPathSummary
          :path="path"
          :nodes="nodes"
          :show-solved="showSolved"
          :compact="isMobile"
        />
        <section v-if="path.descriptionMd" class="path-description">
          <div
            id="path-description-content"
            class="path-description__content"
            :class="{ 'path-description--expanded': descriptionExpanded }"
          >
            <MarkdownView :source="path.descriptionMd" :theme="isDark ? 'dark' : 'light'" />
          </div>
          <button
            type="button"
            class="path-description__toggle"
            :aria-expanded="descriptionExpanded"
            aria-controls="path-description-content"
            @click="descriptionExpanded = !descriptionExpanded"
          >
            {{ descriptionExpanded ? '收起' : '展开' }}
          </button>
        </section>
      </div>
    </template>

    <AppSurface padding="lg" class="detail-surface">
      <AppErrorState v-if="error" :description="error" @retry="load" />

      <AppLoadingState v-else-if="loading && !path" label="正在加载学习路线" />

      <template v-else-if="path">
        <section v-if="locked" class="locked-panel" data-test="path-locked">
          <div>
            <p class="section-eyebrow">PAID PATH</p>
            <h2>购买后解锁完整路线</h2>
            <p>
              该路线售价 <strong>{{ priceLabel }}</strong
              >。购买后可永久访问全部节点与题目；作者和管理员可直接访问。
            </p>
          </div>
          <Button
            v-if="isLoggedIn"
            label="购买并解锁"
            icon="pi pi-lock-open"
            :loading="purchasing"
            data-test="path-purchase"
            @click="buyPath"
          />
          <Button
            v-else
            as="router-link"
            :to="{ name: 'login', query: { redirect: loginRedirect } }"
            label="登录后购买"
            icon="pi pi-sign-in"
          />
        </section>

        <AppEmptyState v-else-if="nodes.length === 0" title="该路线还没有节点" />

        <LearningMobileTree
          v-else-if="isMobile"
          :nodes="nodes"
          :selected-id="selectedId"
          @select="selectNode"
        />

        <div v-else class="desktop-layout">
          <LearningTreeCanvas
            :nodes="nodes"
            :path-id="path.id"
            :selected-id="selectedId"
            @select="selectNode"
            @more="onMore"
          />
          <LearningNodePanel
            ref="panelRef"
            :node="selectedNode"
            :index="selectedIndex"
            empty-label="选择树上的节点查看详情。"
          />
        </div>
      </template>
    </AppSurface>

    <p class="sr-only" aria-live="polite">
      {{ selectedNode ? `已选择 ${selectedNode.title}` : '' }}
    </p>
  </div>
</template>

<style scoped>
.detail-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.path-overview {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.path-visibility {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
  align-items: center;
}

.path-count {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.locked-panel {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-4);
  padding: var(--kapoj-space-5);
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.locked-panel h2 {
  margin: 0 0 var(--kapoj-space-1);
  font-size: 1.1rem;
}

.locked-panel p {
  margin: 0;
  color: var(--kapoj-text-secondary);
}

.path-description {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--kapoj-space-1);
  color: var(--kapoj-text-secondary);
}

.path-description__content {
  display: -webkit-box;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.path-description__content.path-description--expanded {
  display: block;
  overflow: visible;
}

.path-description__toggle {
  padding: 0;
  color: var(--kapoj-primary);
  font: inherit;
  cursor: pointer;
  background: transparent;
  border: 0;
}

.path-description__toggle:hover {
  text-decoration: underline;
}

.detail-surface {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.desktop-layout {
  display: grid;
  min-width: 0;
  grid-template-columns: minmax(0, 1fr) 300px;
  gap: var(--kapoj-space-4);
  align-items: stretch;
}

.desktop-layout :deep(.tree-viewport) {
  height: clamp(560px, 68vh, 680px);
  min-height: 0;
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

@media (max-width: 1099px) {
  .desktop-layout {
    grid-template-columns: minmax(0, 1fr) 280px;
  }
}
</style>
