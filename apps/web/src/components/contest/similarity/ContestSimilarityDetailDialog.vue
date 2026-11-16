<script setup lang="ts">
import { computed } from 'vue'
import Dialog from 'primevue/dialog'
import Tag from 'primevue/tag'

import type { ContestSimilarityMatchDetail } from '@/api/types'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppStatusTag from '@/components/ui/AppStatusTag.vue'
import CodeViewer from '@/components/CodeViewer.vue'
import { useMediaQuery } from '@/composables/useMediaQuery'
import { formatDateTime } from '@/utils/format'
import { languageLabel } from '@/utils/submission'

const props = defineProps<{
  visible: boolean
  detail: ContestSimilarityMatchDetail | null
  loading: boolean
  error: string
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  retry: []
}>()

const dialogVisible = computed({
  get: () => props.visible,
  set: (value: boolean) => emit('update:visible', value),
})

const stacked = useMediaQuery('(max-width: 760px)')

function similaritySeverity(percent: number): 'danger' | 'warn' | 'secondary' {
  if (percent >= 90) return 'danger'
  if (percent >= 80) return 'warn'
  return 'secondary'
}

function displayName(nickname: string, username: string): string {
  return nickname.trim() || username || '?'
}
</script>

<template>
  <Dialog
    v-model:visible="dialogVisible"
    modal
    header="查重详情"
    :style="{ width: 'min(96vw, 1440px)' }"
    :draggable="false"
    class="similarity-detail-dialog"
  >
    <div data-test="similarity-detail" class="similarity-detail">
      <AppLoadingState v-if="loading" label="加载查重详情" :rows="4" />
      <AppErrorState v-else-if="error" :description="error" @retry="emit('retry')" />

      <template v-else-if="detail">
        <div class="similarity-detail__summary">
          <div class="similarity-detail__scores">
            <Tag
              :value="`最高相似度 ${detail.similarityPercent}%`"
              :severity="similaritySeverity(detail.similarityPercent)"
              data-test="similarity-detail-highest"
            />
            <Tag
              :value="`Dice ${detail.dicePercent}%`"
              :severity="similaritySeverity(detail.dicePercent)"
              data-test="similarity-detail-dice"
            />
          </div>
          <p class="similarity-detail__notice">相似度仅代表代码相似，需人工复核。</p>
        </div>

        <div class="similarity-detail__meta">
          <section class="meta-pane">
            <h3 class="meta-pane__title">原提交</h3>
            <dl class="meta-pane__list">
              <div>
                <dt>用户</dt>
                <dd>{{ displayName(detail.originalNickname, detail.originalUsername) }}</dd>
              </div>
              <div>
                <dt>时间</dt>
                <dd>{{ formatDateTime(detail.originalCreatedAt) }}</dd>
              </div>
              <div>
                <dt>语言</dt>
                <dd>{{ languageLabel(detail.language) }}</dd>
              </div>
              <div>
                <dt>状态</dt>
                <dd><AppStatusTag :status="detail.originalStatus" /></dd>
              </div>
              <div>
                <dt>得分</dt>
                <dd class="num">{{ detail.similarityPercent }}%</dd>
              </div>
            </dl>
          </section>

          <section class="meta-pane">
            <h3 class="meta-pane__title">疑似提交</h3>
            <dl class="meta-pane__list">
              <div>
                <dt>用户</dt>
                <dd>{{ displayName(detail.suspectNickname, detail.suspectUsername) }}</dd>
              </div>
              <div>
                <dt>时间</dt>
                <dd>{{ formatDateTime(detail.suspectCreatedAt) }}</dd>
              </div>
              <div>
                <dt>语言</dt>
                <dd>{{ languageLabel(detail.language) }}</dd>
              </div>
              <div>
                <dt>状态</dt>
                <dd><AppStatusTag :status="detail.suspectStatus" /></dd>
              </div>
              <div>
                <dt>得分</dt>
                <dd class="num">{{ detail.similarityPercent }}%</dd>
              </div>
            </dl>
          </section>
        </div>

        <div
          class="similarity-detail__codes"
          :class="{ 'similarity-detail__codes--stacked': stacked }"
          data-test="similarity-detail-codes"
        >
          <section class="code-pane" data-test="similarity-original-code">
            <h3 class="code-pane__title">原提交代码</h3>
            <CodeViewer :code="detail.originalCode" :language="detail.language" />
          </section>

          <section class="code-pane" data-test="similarity-suspect-code">
            <h3 class="code-pane__title">疑似代码</h3>
            <CodeViewer :code="detail.suspectCode" :language="detail.language" />
          </section>
        </div>
      </template>
    </div>
  </Dialog>
</template>

<style scoped>
.similarity-detail {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
}

.similarity-detail__summary {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
  padding-bottom: var(--kapoj-space-3);
  border-bottom: 1px solid var(--kapoj-border);
}

.similarity-detail__scores {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.similarity-detail__notice {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 0.8125rem;
}

.similarity-detail__meta {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--kapoj-space-3);
}

.meta-pane {
  min-width: 0;
  padding: var(--kapoj-space-3);
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.meta-pane__title,
.code-pane__title {
  margin: 0 0 var(--kapoj-space-2);
  color: var(--kapoj-text);
  font-size: 0.875rem;
  font-weight: 600;
}

.meta-pane__list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--kapoj-space-2) var(--kapoj-space-3);
  margin: 0;
}

.meta-pane__list div {
  min-width: 0;
}

.meta-pane__list dt {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.meta-pane__list dd {
  margin: 2px 0 0;
  overflow-wrap: anywhere;
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
}

.similarity-detail__codes {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--kapoj-space-3);
  align-items: start;
}

.similarity-detail__codes--stacked {
  grid-template-columns: minmax(0, 1fr);
}

.code-pane {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.code-pane :deep(.code-viewer__body) {
  max-height: min(58vh, 560px);
}

@media (max-width: 760px) {
  .similarity-detail__meta,
  .similarity-detail__codes {
    grid-template-columns: minmax(0, 1fr);
  }

  .meta-pane__list {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
