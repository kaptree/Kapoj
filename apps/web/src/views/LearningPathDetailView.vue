<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Button from 'primevue/button'
import Tag from 'primevue/tag'

import { ApiError } from '@/api/http'
import { getLearningTree } from '@/api/learning'
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
const { isDark } = useTheme()
const isMobile = useMediaQuery('(max-width: 899px)')
const showSolved = computed(() => auth.isLoggedIn)

const id = computed(() => {
  const raw = route.params.id
  return typeof raw === 'string' ? raw : ''
})

const path = ref<LearningTree['path'] | null>(null)
const nodes = ref<LearningPathNodeItem[]>([])
const loading = ref(false)
const error = ref('')
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
        </div>
        <LearningPathSummary
          :path="path"
          :nodes="nodes"
          :show-solved="showSolved"
          :compact="isMobile"
        />
        <section v-if="path.descriptionMd" class="path-description">
          <div
            class="path-description__content"
            :class="{ 'path-description--expanded': descriptionExpanded }"
          >
            <MarkdownView :source="path.descriptionMd" :theme="isDark ? 'dark' : 'light'" />
          </div>
          <button
            type="button"
            class="path-description__toggle"
            :aria-expanded="descriptionExpanded"
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
        <AppEmptyState v-if="nodes.length === 0" title="该路线还没有节点" />

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
  align-items: center;
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
