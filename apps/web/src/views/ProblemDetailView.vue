<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Button from 'primevue/button'
import Message from 'primevue/message'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import { getProblem, getStatistics } from '@/api/problems'
import type { ProblemDetail, ProblemStats } from '@/api/types'
import MarkdownView from '@/components/MarkdownView.vue'
import AppDifficultyTag from '@/components/ui/AppDifficultyTag.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'
import { useTheme } from '@/theme/theme'
import { formatCoins } from '@/utils/gamification'

const route = useRoute()
const auth = useAuthStore()
const toast = useToast()
const { isDark } = useTheme()

const canEdit = computed(() => auth.can(PERM.problem.update))
const contestId = computed(() => {
  const n = Number(route.query?.contestId)
  return Number.isInteger(n) && n > 0 ? n : 0
})

const problem = ref<ProblemDetail | null>(null)
const stats = ref<ProblemStats | null>(null)
const loading = ref(false)
const error = ref('')

const typeLabels = { traditional: '传统题', special: '特判题', interactive: '交互题' } as const

let latest = 0

async function load(id: string) {
  const seq = ++latest
  loading.value = true
  error.value = ''
  problem.value = null
  stats.value = null

  try {
    const detail = await getProblem(id)
    if (seq !== latest) return
    problem.value = detail

    // Statistics are a nice-to-have; the page works without them, but a late
    // response for a previous problem must not overwrite this one.
    const nextStats = await getStatistics(id).catch(() => null)
    if (seq !== latest) return
    stats.value = nextStats
  } catch (e) {
    if (seq !== latest) return
    error.value =
      e instanceof ApiError && e.code === 40400 ? '题目不存在' : '加载题目失败，请稍后再试'
  } finally {
    if (seq === latest) {
      loading.value = false
    }
  }
}

watch(
  () => route.params.id,
  (id) => {
    if (typeof id === 'string') void load(id)
  },
  { immediate: true },
)

function retry() {
  const id = route.params.id
  if (typeof id === 'string') void load(id)
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    toast.add({ severity: 'success', summary: '已复制', life: 2000 })
  } catch {
    toast.add({ severity: 'error', summary: '复制失败', detail: '请手动选择文本', life: 3000 })
  }
}

const rate = computed(() => (stats.value ? `${(stats.value.acceptRate * 100).toFixed(1)}%` : '-'))

const solutionPriceLabel = computed(() => {
  const price = problem.value?.solutionPriceCoins ?? 0
  return price > 0 ? `金币 ${formatCoins(price)}` : '免费'
})

const statsLabel = computed(() =>
  stats.value ? `通过率 ${rate.value} · ${stats.value.submitCount} 提交` : '查看提交统计',
)

const showSolutionResources = computed(
  () => problem.value?.isPublic !== false || auth.can(PERM.problem.viewHidden),
)
</script>

<template>
  <div class="problem-detail-page">
    <AppSurface v-if="loading && !problem" padding="lg">
      <AppLoadingState label="正在加载题目" />
    </AppSurface>

    <div v-else-if="error" class="error-panel">
      <AppErrorState :description="error" @retry="retry" />
      <Button as="router-link" to="/problems" label="返回题库" severity="secondary" outlined />
    </div>

    <div v-else-if="problem" class="problem-content">
      <AppPageHeader
        :eyebrow="`PROBLEM // ${problem?.slug.toUpperCase() ?? ''}`"
        :title="problem?.title ?? ''"
      >
        <template #actions>
          <Button
            v-if="contestId"
            as="router-link"
            :to="{ name: 'contest-detail', params: { id: contestId } }"
            :label="`返回比赛 #${contestId}`"
            icon="pi pi-arrow-left"
            severity="secondary"
            outlined
          />
          <Button
            as="router-link"
            :to="{
              name: 'problem-submit',
              params: { id: problem?.id ?? 0 },
              query: contestId ? { contestId: String(contestId) } : {},
            }"
            label="提交代码"
            icon="pi pi-send"
          />
          <Button
            v-if="canEdit"
            as="router-link"
            :to="{ name: 'problem-edit', params: { id: problem?.id ?? 0 } }"
            label="编辑"
            icon="pi pi-pencil"
            severity="secondary"
            outlined
          />
        </template>
      </AppPageHeader>

      <div class="meta-bar">
        <AppDifficultyTag :difficulty="problem?.difficulty ?? 0" />
        <span class="type-chip">{{ typeLabels[problem?.type ?? 'traditional'] }}</span>
        <span class="meta-item">
          <i class="pi pi-clock" aria-hidden="true" /> {{ problem?.timeLimitMs }} ms
        </span>
        <span class="meta-item">
          <i class="pi pi-database" aria-hidden="true" /> {{ problem?.memoryLimitMb }} MB
        </span>
        <span v-if="problem?.source" class="meta-item">来源：{{ problem?.source }}</span>
        <span v-if="problem?.tags.length" class="tag-list">
          <span v-for="t in problem?.tags ?? []" :key="t" class="tag-chip">{{ t }}</span>
        </span>
      </div>

      <Message
        v-if="problem?.isPublic === false"
        severity="warn"
        :closable="false"
        class="visibility-banner"
      >
        {{
          contestId
            ? '这道题目正在比赛中，比赛结束后将公开。'
            : '这道题目尚未公开，只有题目管理员能看到。'
        }}
      </Message>

      <div class="detail-grid">
        <div class="left-column">
          <AppSurface as="article" padding="lg" class="statement-card">
            <section aria-labelledby="problem-statement-title">
              <h2 id="problem-statement-title" class="section-title">题目描述</h2>
              <MarkdownView
                :source="problem?.statementMd ?? ''"
                :theme="isDark ? 'dark' : 'light'"
              />
            </section>

            <section v-if="problem?.inputFormatMd" aria-labelledby="problem-input-title">
              <h2 id="problem-input-title" class="section-title">输入格式</h2>
              <MarkdownView
                :source="problem?.inputFormatMd ?? ''"
                :theme="isDark ? 'dark' : 'light'"
              />
            </section>

            <section v-if="problem?.outputFormatMd" aria-labelledby="problem-output-title">
              <h2 id="problem-output-title" class="section-title">输出格式</h2>
              <MarkdownView
                :source="problem?.outputFormatMd ?? ''"
                :theme="isDark ? 'dark' : 'light'"
              />
            </section>

            <section v-if="problem?.samples.length" aria-labelledby="problem-samples-title">
              <h2 id="problem-samples-title" class="section-title">样例</h2>
              <div v-for="(s, i) in problem?.samples ?? []" :key="i" class="sample-grid">
                <div class="sample-panel">
                  <div class="sample-head">
                    <span>输入 {{ i + 1 }}</span>
                    <Button
                      type="button"
                      text
                      size="small"
                      icon="pi pi-copy"
                      label="复制"
                      :aria-label="`复制输入 ${i + 1}`"
                      @click="copy(s.input)"
                    />
                  </div>
                  <pre tabindex="0">{{ s.input }}</pre>
                </div>
                <div class="sample-panel">
                  <div class="sample-head">
                    <span>输出 {{ i + 1 }}</span>
                    <Button
                      type="button"
                      text
                      size="small"
                      icon="pi pi-copy"
                      label="复制"
                      :aria-label="`复制输出 ${i + 1}`"
                      @click="copy(s.output)"
                    />
                  </div>
                  <pre tabindex="0">{{ s.output }}</pre>
                </div>
              </div>
            </section>
          </AppSurface>

          <AppSurface as="section" padding="lg" class="hint-card">
            <h2 id="problem-hint-title" class="section-title">提示</h2>
            <MarkdownView
              v-if="problem?.hintMd"
              :source="problem?.hintMd ?? ''"
              :theme="isDark ? 'dark' : 'light'"
            />
            <AppEmptyState v-else icon="pi pi-lightbulb" title="暂无提示" />
          </AppSurface>
        </div>

        <aside class="info-column" aria-label="题目资源">
          <RouterLink
            v-if="showSolutionResources"
            class="resource-link"
            :to="`/problems/${problem?.id}/official-solution`"
          >
            <AppSurface as="div" padding="md" interactive class="resource-card">
              <span class="resource-card__icon" aria-hidden="true">
                <i class="pi pi-book" />
              </span>
              <span class="resource-card__body">
                <span class="resource-card__title">官方题解</span>
                <span class="resource-card__sub">{{ solutionPriceLabel }}</span>
              </span>
              <i class="pi pi-chevron-right resource-card__arrow" aria-hidden="true" />
            </AppSurface>
          </RouterLink>

          <RouterLink
            v-if="showSolutionResources"
            class="resource-link"
            :to="`/problems/${problem?.id}/solutions`"
          >
            <AppSurface as="div" padding="md" interactive class="resource-card">
              <span class="resource-card__icon" aria-hidden="true">
                <i class="pi pi-comments" />
              </span>
              <span class="resource-card__body">
                <span class="resource-card__title">解题思路</span>
                <span class="resource-card__sub">帖子 / 回复</span>
              </span>
              <i class="pi pi-chevron-right resource-card__arrow" aria-hidden="true" />
            </AppSurface>
          </RouterLink>

          <RouterLink class="resource-link" :to="`/problems/${problem?.id}/statistics`">
            <AppSurface as="div" padding="md" interactive class="resource-card">
              <span class="resource-card__icon" aria-hidden="true">
                <i class="pi pi-chart-bar" />
              </span>
              <span class="resource-card__body">
                <span class="resource-card__title">提交统计</span>
                <span class="resource-card__sub">{{ statsLabel }}</span>
              </span>
              <i class="pi pi-chevron-right resource-card__arrow" aria-hidden="true" />
            </AppSurface>
          </RouterLink>

          <RouterLink class="resource-link" :to="`/problems/${problem?.id}/contests`">
            <AppSurface as="div" padding="md" interactive class="resource-card">
              <span class="resource-card__icon" aria-hidden="true">
                <i class="pi pi-trophy" />
              </span>
              <span class="resource-card__body">
                <span class="resource-card__title">关联比赛</span>
                <span class="resource-card__sub">查看包含本题的比赛</span>
              </span>
              <i class="pi pi-chevron-right resource-card__arrow" aria-hidden="true" />
            </AppSurface>
          </RouterLink>
        </aside>
      </div>
    </div>
  </div>
</template>

<style scoped>
.problem-detail-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.problem-content {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.error-panel {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  align-items: flex-start;
}

.meta-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-size: 12px;
}

.meta-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.meta-item i {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.type-chip {
  padding: 1px 8px;
  color: var(--kapoj-info);
  font-size: 11px;
  border: 1px solid color-mix(in srgb, var(--kapoj-info) 32%, transparent);
  border-radius: var(--kapoj-radius-xs);
}

.tag-list {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-1);
}

.tag-chip {
  padding: 1px 7px;
  color: var(--kapoj-primary);
  font-size: 11px;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 28%, transparent);
  border-radius: var(--kapoj-radius-xs);
}

.visibility-banner {
  margin: 0;
}

.detail-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(320px, 380px);
  gap: var(--kapoj-space-5);
  align-items: start;
}

.left-column {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
}

.statement-card {
  min-width: 0;
}

.statement-card section + section {
  margin-top: var(--kapoj-space-6);
}

.section-title {
  margin: 0 0 var(--kapoj-space-3);
  color: var(--kapoj-text);
  font-size: 1.1rem;
  line-height: 1.3;
}

.sample-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--kapoj-space-3);
  margin-bottom: var(--kapoj-space-3);
}

.sample-panel {
  min-width: 0;
  overflow: hidden;
  background: var(--kapoj-bg);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.sample-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
  padding: 6px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-size: 12px;
  border-bottom: 1px solid var(--kapoj-border);
}

.sample-panel pre {
  padding: var(--kapoj-space-3);
  margin: 0;
  overflow: auto;
  color: var(--kapoj-text);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  line-height: 1.6;
}

.info-column {
  position: sticky;
  top: calc(var(--kapoj-header-height) + var(--kapoj-space-4));
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.resource-link {
  display: block;
  color: inherit;
  text-decoration: none;
}

.resource-card {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-3);
}

.resource-card__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 40px;
  height: 40px;
  color: var(--kapoj-primary);
  font-size: 1.1rem;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 28%, transparent);
  border-radius: var(--kapoj-radius-md);
}

.resource-card__body {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.resource-card__title {
  color: var(--kapoj-text);
  font-size: 0.95rem;
  font-weight: 600;
}

.resource-card__sub {
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
}

.resource-card__arrow {
  color: var(--kapoj-text-muted);
  font-size: 0.85rem;
}

@media (max-width: 1024px) {
  .detail-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .info-column {
    position: static;
  }
}

@media (max-width: 700px) {
  .sample-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .meta-bar {
    gap: var(--kapoj-space-2);
  }
}
</style>
