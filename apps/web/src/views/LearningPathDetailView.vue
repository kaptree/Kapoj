<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Button from 'primevue/button'
import Column from 'primevue/column'
import ProgressBar from 'primevue/progressbar'
import Tag from 'primevue/tag'
import TreeTable from 'primevue/treetable'
import type { TreeNode } from 'primevue/treenode'

import { ApiError } from '@/api/http'
import { getLearningTree } from '@/api/learning'
import type { LearningPathNodeItem, LearningTree } from '@/api/types'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import MarkdownView from '@/components/MarkdownView.vue'
import { useTheme } from '@/theme/theme'
import {
  flattenLearningNodes,
  learningPathVisibilityLabel,
  learningStatusLabel,
  learningStatusSeverity,
  nodeProgressPercent,
} from '@/utils/learning'

const route = useRoute()
const { isDark } = useTheme()

const id = computed(() => {
  const raw = route.params.id
  return typeof raw === 'string' ? raw : ''
})

const path = ref<LearningTree['path'] | null>(null)
const nodes = ref<LearningPathNodeItem[]>([])
const loading = ref(false)
const error = ref('')
const selectedNode = ref<LearningPathNodeItem | null>(null)
const expandedKeys = ref<Record<string, boolean>>({})
const selectionKeys = ref<Record<string, boolean>>({})

let latest = 0

const treeNodes = computed<TreeNode[]>(() => nodes.value.map(toTreeNode))

function toTreeNode(node: LearningPathNodeItem): TreeNode {
  return {
    key: String(node.id),
    id: String(node.id),
    data: node,
    leaf: node.children.length === 0,
    children: node.children.map(toTreeNode),
  }
}

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const data = await getLearningTree(id.value)
    if (seq !== latest) return
    path.value = data.path
    nodes.value = data.nodes
    expandedKeys.value = Object.fromEntries(
      flattenLearningNodes(data.nodes).map((node) => [String(node.id), true]),
    )
    selectNode(flattenLearningNodes(data.nodes).find((node) => node.status === 'active') ?? null)
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
    selectedNode.value = null
    selectionKeys.value = {}
    void load()
  },
  { immediate: true },
)

function selectNode(node: LearningPathNodeItem | null) {
  selectedNode.value = node
  selectionKeys.value = node ? { [String(node.id)]: true } : {}
}

function onNodeSelect(treeNode: TreeNode) {
  selectedNode.value = treeNode.data
}

const authorLabel = computed(() => {
  if (!path.value) return ''
  return path.value.author.nickname || path.value.author.username
})

const progressLabel = computed(() => {
  if (!path.value) return ''
  return `已完成 ${path.value.myCompletedCount} / ${path.value.nodeCount} 节点`
})

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

    <AppSurface padding="lg" class="detail-surface">
      <AppErrorState v-if="error" :description="error" @retry="load" />

      <AppLoadingState v-else-if="loading && !path" label="正在加载学习路线" />

      <template v-else-if="path">
        <header class="path-head">
          <div class="path-meta">
            <Tag :value="learningPathVisibilityLabel(path)" :severity="visibilitySeverity()" />
            <span class="path-author">作者 {{ authorLabel }}</span>
            <span class="path-progress">{{ progressLabel }}</span>
          </div>
          <MarkdownView
            v-if="path.descriptionMd"
            :source="path.descriptionMd"
            :theme="isDark ? 'dark' : 'light'"
            class="path-desc"
          />
        </header>

        <div class="detail-layout">
          <div class="tree-panel">
            <div class="tree-scroll">
              <TreeTable
                v-model:expanded-keys="expandedKeys"
                v-model:selection-keys="selectionKeys"
                :value="treeNodes"
                data-key="id"
                selection-mode="single"
                table-style="min-width: 40rem"
                class="tree-table"
                @node-select="onNodeSelect"
              >
                <Column field="title" header="节点" expander>
                  <template #body="{ node }">
                    <span class="node-title">{{ node.data.title }}</span>
                    <Tag
                      :value="learningStatusLabel(node.data.status)"
                      :severity="learningStatusSeverity(node.data.status)"
                      :icon="node.data.status === 'locked' ? 'pi pi-lock' : undefined"
                      class="node-status"
                    />
                  </template>
                </Column>
                <Column field="progress" header="进度" style="min-width: 12rem">
                  <template #body="{ node }">
                    <div class="progress-cell">
                      <span class="progress-num"
                        >{{ node.data.solvedCount }} / {{ node.data.requiredCount }}</span
                      >
                      <ProgressBar
                        :value="nodeProgressPercent(node.data.solvedCount, node.data.requiredCount)"
                        class="progress-bar"
                      />
                    </div>
                  </template>
                </Column>
                <Column field="count" header="题目数" style="width: 6rem">
                  <template #body="{ node }">
                    <span class="count-num">{{ node.data.problems.length }}</span>
                  </template>
                </Column>
                <Column header="操作" style="width: 6rem">
                  <template #body="{ node }">
                    <Button
                      label="查看"
                      size="small"
                      text
                      class="view-button"
                      @click="selectNode(node.data)"
                    />
                  </template>
                </Column>
                <template #empty>
                  <p class="tree-empty">该路线还没有节点。</p>
                </template>
              </TreeTable>
            </div>
          </div>

          <aside class="detail-panel">
            <template v-if="selectedNode">
              <h2 class="detail-title">{{ selectedNode.title }}</h2>
              <MarkdownView
                v-if="selectedNode.descriptionMd"
                :source="selectedNode.descriptionMd"
                :theme="isDark ? 'dark' : 'light'"
                class="detail-desc"
              />
              <p v-else class="detail-empty">该节点暂无描述。</p>

              <ul v-if="selectedNode.problems.length" class="problem-list">
                <li v-for="p in selectedNode.problems" :key="p.problemId" class="problem-item">
                  <RouterLink :to="`/problems/${p.problemId}`" class="problem-link">
                    {{ p.title }}
                  </RouterLink>
                  <span class="problem-difficulty">难度 {{ p.difficulty }}</span>
                </li>
              </ul>
              <p v-else class="detail-empty">该节点暂无题目。</p>
            </template>
            <p v-else class="detail-empty">选择左侧节点查看详情。</p>
          </aside>
        </div>
      </template>
    </AppSurface>
  </div>
</template>

<style scoped>
.detail-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.detail-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.path-head {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.path-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
}

.path-author {
  color: var(--kapoj-text-secondary);
}

.path-progress {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.path-desc {
  color: var(--kapoj-text-secondary);
}

.detail-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 20rem);
  gap: var(--kapoj-space-4);
  align-items: start;
}

.tree-panel {
  min-width: 0;
}

.tree-scroll {
  overflow-x: auto;
}

.node-title {
  margin-right: var(--kapoj-space-2);
  font-weight: 500;
}

.progress-cell {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
}

.progress-num {
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.count-num {
  color: var(--kapoj-text-secondary);
}

.tree-empty {
  margin: var(--kapoj-space-4);
  color: var(--kapoj-text-muted);
}

.detail-panel {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-4);
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.detail-title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.05rem;
  font-weight: 600;
}

.detail-desc {
  color: var(--kapoj-text-secondary);
}

.detail-empty {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 0.875rem;
}

.problem-list {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.problem-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
  padding: var(--kapoj-space-2) var(--kapoj-space-3);
  background: var(--kapoj-surface);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.problem-link {
  min-width: 0;
  color: var(--kapoj-primary);
  font-size: 0.875rem;
  text-decoration: none;
  word-break: break-word;
}

.problem-link:hover {
  text-decoration: underline;
}

.problem-difficulty {
  flex-shrink: 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

@media (max-width: 900px) {
  .detail-layout {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
