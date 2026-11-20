<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Drawer from 'primevue/drawer'
import Message from 'primevue/message'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import { getSubmission, rejudgeSubmission } from '@/api/submissions'
import type { SubmissionDetail } from '@/api/types'
import CodeViewer from '@/components/CodeViewer.vue'
import UserIdentityName from '@/components/UserIdentityName.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppStatusTag from '@/components/ui/AppStatusTag.vue'
import { formatDateTime } from '@/utils/format'
import { languageLabel } from '@/utils/submission'

const props = withDefaults(
  defineProps<{
    visible: boolean
    submissionId: number | null
    canRejudge?: boolean
  }>(),
  { canRejudge: false },
)

const emit = defineEmits<{
  'update:visible': [value: boolean]
  rejudged: []
}>()

const toast = useToast()

const detail = ref<SubmissionDetail | null>(null)
const loading = ref(false)
const error = ref('')
const rejudging = ref(false)

let requestSeq = 0

const testcaseTotal = computed<number | null>(() => {
  const cases = detail.value?.judgeResult ?? []
  if (!cases.length || cases.some((item) => item.score == null)) return null
  return cases.reduce((sum, item) => sum + (item.score ?? 0), 0)
})

async function load() {
  const id = props.submissionId
  if (!props.visible || id == null) {
    detail.value = null
    error.value = ''
    return
  }

  const seq = ++requestSeq
  loading.value = true
  error.value = ''
  detail.value = null
  try {
    const next = await getSubmission(id)
    if (seq === requestSeq) detail.value = next
  } catch (e) {
    if (seq !== requestSeq) return
    const message = e instanceof ApiError ? e.message : '加载记录详情失败'
    error.value = message
    toast.add({
      severity: 'error',
      summary: '加载失败',
      detail: message,
      life: 4000,
    })
  } finally {
    if (seq === requestSeq) loading.value = false
  }
}

async function rejudge() {
  if (!detail.value || rejudging.value) return
  rejudging.value = true
  try {
    await rejudgeSubmission(detail.value.id)
    toast.add({ severity: 'success', summary: '已提交重测', life: 3000 })
    const next = await getSubmission(detail.value.id)
    detail.value = next
    emit('rejudged')
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: e instanceof ApiError ? e.message : '重测失败，请稍后再试',
      life: 4000,
    })
  } finally {
    rejudging.value = false
  }
}

function setVisible(value: boolean) {
  emit('update:visible', value)
}

watch(
  () => [props.visible, props.submissionId] as const,
  () => void load(),
  { immediate: true },
)
</script>

<template>
  <Drawer
    :visible="visible"
    position="right"
    header="测评详情"
    class="submissions-drawer"
    :style="{ width: 'min(92vw, 760px)' }"
    :close-button-props="{ 'aria-label': '关闭测评详情' }"
    :dismissable="true"
    @update:visible="setVisible"
  >
    <AppLoadingState v-if="loading" label="正在加载测评详情" />
    <AppErrorState v-else-if="error" :description="error" @retry="load" />
    <template v-else-if="detail">
      <div v-if="canRejudge" class="drawer-actions">
        <Button
          label="重测"
          icon="pi pi-refresh"
          size="small"
          outlined
          :loading="rejudging"
          :disabled="detail.status === 'pending' || detail.status === 'judging'"
          data-test="rejudge"
          @click="rejudge"
        />
      </div>

      <dl class="detail-meta">
        <div>
          <dt>提交</dt>
          <dd>#{{ detail.id }}</dd>
        </div>
        <div>
          <dt>状态</dt>
          <dd><AppStatusTag :status="detail.status" /></dd>
        </div>
        <div class="detail-user">
          <dt>用户</dt>
          <dd>
            <UserIdentityName
              :nickname="detail.nickname"
              :username="detail.username"
              :title="detail.equippedTitle"
              :badges="detail.equippedBadges"
              size="small"
              :link-to="{ name: 'user-profile', params: { id: detail.userId } }"
            />
          </dd>
        </div>
        <div>
          <dt>题目</dt>
          <dd>
            <RouterLink
              :to="{ name: 'problem-detail', params: { id: detail.problemId } }"
              class="problem-link"
              @click.stop
            >
              {{ detail.problemTitle }}
            </RouterLink>
          </dd>
        </div>
        <div>
          <dt>语言</dt>
          <dd>{{ languageLabel(detail.language) }}</dd>
        </div>
        <div>
          <dt>得分</dt>
          <dd class="num">{{ detail.score }}</dd>
        </div>
        <div>
          <dt>耗时</dt>
          <dd class="num">
            {{ detail.language === 'answer' ? '—' : `${detail.timeUsedMs} ms` }}
          </dd>
        </div>
        <div>
          <dt>CPU 时间</dt>
          <dd class="num">
            {{ detail.language === 'answer' ? '—' : `${detail.cpuUsedMs ?? 0} ms` }}
          </dd>
        </div>
        <div>
          <dt>内存</dt>
          <dd class="num">
            {{ detail.language === 'answer' ? '—' : `${detail.memoryUsedKb} KB` }}
          </dd>
        </div>
        <div>
          <dt>提交时间</dt>
          <dd>{{ formatDateTime(detail.createdAt) }}</dd>
        </div>
        <div>
          <dt>评测时间</dt>
          <dd>{{ formatDateTime(detail.judgedAt) }}</dd>
        </div>
      </dl>

      <template v-if="detail.language === 'answer'">
        <Message
          v-if="detail.answerHidden"
          data-test="answer-hidden"
          severity="warn"
          :closable="false"
          class="hidden-banner"
        >
          该提交的答案暂不可见。仅提交者本人、管理员或已通过该题的用户可以查看。
        </Message>
        <template v-else-if="detail.answer">
          <h3>答案</h3>
          <pre class="code-block" data-test="submission-answer" tabindex="0">{{
            detail.answer
          }}</pre>
        </template>
      </template>

      <Message
        v-else-if="detail.codeHidden"
        severity="warn"
        :closable="false"
        class="hidden-banner"
      >
        该提交的代码暂不可见。仅提交者本人、管理员或已通过该题的用户可以查看。
      </Message>

      <template v-else>
        <h3>代码</h3>
        <CodeViewer :code="detail.code ?? ''" :language="detail.language" />

        <template v-if="detail.errorMessage">
          <h3>错误信息</h3>
          <pre class="code-block" tabindex="0">{{ detail.errorMessage }}</pre>
        </template>

        <template v-if="detail.judgeResult.length">
          <h3>测试点</h3>
          <DataTable
            :value="detail.judgeResult"
            size="small"
            responsive-layout="scroll"
            class="judge-table"
          >
            <Column header="测试点" style="width: 80px">
              <template #body="{ data }">
                <span class="num">{{ data.testcaseId }}</span>
              </template>
            </Column>
            <Column header="状态" style="width: 110px">
              <template #body="{ data }"><AppStatusTag :status="data.status" /></template>
            </Column>
            <Column header="耗时 (ms)" style="width: 110px">
              <template #body="{ data }">
                <span class="num">{{ data.timeMs }}</span>
              </template>
            </Column>
            <Column header="CPU (ms)" style="width: 100px">
              <template #body="{ data }">
                <span class="num">{{ data.cpuMs ?? 0 }}</span>
              </template>
            </Column>
            <Column header="内存 (KB)" style="width: 110px">
              <template #body="{ data }">
                <span class="num">{{ data.memoryKb }}</span>
              </template>
            </Column>
            <Column header="分数" style="width: 80px">
              <template #body="{ data }">
                <span class="num">{{ data.score ?? '—' }}</span>
              </template>
            </Column>
            <Column header="信息" style="min-width: 160px">
              <template #body="{ data }">{{ data.message }}</template>
            </Column>
          </DataTable>
          <p v-if="testcaseTotal !== null" class="testcase-total">
            测试点总分：{{ testcaseTotal }}
          </p>
        </template>
      </template>
    </template>
    <AppEmptyState v-else title="暂无详情" />
  </Drawer>
</template>

<style scoped>
.problem-link {
  color: var(--kapoj-text);
  font-weight: 600;
}

.problem-link:hover {
  color: var(--kapoj-primary);
}

.detail-meta {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--kapoj-space-3);
  margin: 0;
}

.detail-meta > div {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.detail-meta dt {
  color: var(--kapoj-text-muted);
  font-size: 10px;
  text-transform: uppercase;
}

.detail-meta dd {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 13px;
}

.detail-user dd {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.hidden-banner {
  margin-top: var(--kapoj-space-4);
}

h3 {
  margin: var(--kapoj-space-5) 0 var(--kapoj-space-2);
  color: var(--kapoj-text);
  font-size: 1rem;
}

.drawer-actions {
  display: flex;
  justify-content: flex-end;
  margin-bottom: var(--kapoj-space-2);
}

.testcase-total {
  margin: var(--kapoj-space-2) 0 0;
  color: var(--kapoj-text-muted);
  font-size: 12px;
  text-align: right;
}

.code-block {
  max-height: 420px;
  padding: var(--kapoj-space-3);
  margin: 0;
  overflow: auto;
  color: var(--kapoj-text);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  background: var(--kapoj-bg);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.judge-table {
  margin-top: var(--kapoj-space-2);
}

@media (max-width: 700px) {
  .detail-meta {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
