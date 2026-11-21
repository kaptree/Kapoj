<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Paginator from 'primevue/paginator'
import Tag from 'primevue/tag'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import {
  createAgent,
  generateAgentToken,
  getMyAgent,
  getMyAgentRatingHistory,
  revokeAgentToken,
  updateMyAgent,
} from '@/api/agents'
import { ApiError } from '@/api/http'
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import RatingLevelName from '@/components/RatingLevelName.vue'
import SubmissionDetailDrawer from '@/components/SubmissionDetailDrawer.vue'
import type {
  AgentAttemptView,
  AgentInput,
  AgentRatingKind,
  AgentRatingRow,
  AgentTokenView,
  AgentView,
} from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useTheme } from '@/theme/theme'
import { copyText } from '@/utils/clipboard'
import { formatDateTime } from '@/utils/format'

const HISTORY_PAGE_SIZE = 10
const INTRO_MAX = 2000
const { isDark } = useTheme()

const KIND_META: Record<AgentRatingKind, { label: string; severity: string }> = {
  solve: { label: '通过', severity: 'success' },
  abandon: { label: '放弃', severity: 'danger' },
  timeout: { label: '超时', severity: 'warn' },
  exhausted: { label: '次数耗尽', severity: 'danger' },
  reset: { label: '重置', severity: 'info' },
}

interface AgentForm {
  name: string
  modelName: string
  modelIntro: string
}

const toast = useToast()
const confirm = useConfirm()

const loading = ref(true)
const loadError = ref('')
const agent = ref<AgentView | null>(null)
const token = ref<AgentTokenView | null>(null)
const currentAttempt = ref<AgentAttemptView | null>(null)

/** Plaintext token from this response only; never persisted or restored. */
const issuedToken = ref('')
const generating = ref(false)
const revoking = ref(false)

const form = ref<AgentForm>({ name: '', modelName: '', modelIntro: '' })
const formErrors = ref({ name: '', modelName: '', modelIntro: '' })
const formError = ref('')
const saving = ref(false)

const history = ref<AgentRatingRow[]>([])
const historyTotal = ref(0)
const historyPage = ref(1)
const historyLoading = ref(false)
const historyError = ref('')
const submissionDrawer = ref(false)
const selectedSubmissionId = ref<number | null>(null)

function openSubmission(id: number) {
  selectedSubmissionId.value = id
  submissionDrawer.value = true
}

const editing = computed(() => agent.value !== null)
const agentDisabled = computed(() => agent.value?.status === 'disabled')
const historyFirst = computed(() => (historyPage.value - 1) * HISTORY_PAGE_SIZE)
const statusLabel = computed(() => (agent.value?.status === 'disabled' ? '已停用' : '正常'))
const statusSeverity = computed(() => (agent.value?.status === 'disabled' ? 'danger' : 'success'))

let loadRequest = 0
let historyRequest = 0

function errorMessage(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

function resetForm() {
  form.value = { name: '', modelName: '', modelIntro: '' }
  formErrors.value = { name: '', modelName: '', modelIntro: '' }
  formError.value = ''
}

function fillForm(view: AgentView) {
  form.value = {
    name: view.name,
    modelName: view.modelName,
    modelIntro: view.modelIntro,
  }
  formErrors.value = { name: '', modelName: '', modelIntro: '' }
  formError.value = ''
}

async function load() {
  const seq = ++loadRequest
  loading.value = true
  loadError.value = ''
  issuedToken.value = ''
  try {
    const data = await getMyAgent()
    if (seq !== loadRequest) return
    agent.value = data.agent
    token.value = data.token
    currentAttempt.value = data.currentAttempt
    if (data.agent) {
      fillForm(data.agent)
      void loadHistory()
    } else {
      resetForm()
      history.value = []
      historyTotal.value = 0
      historyPage.value = 1
    }
  } catch (e) {
    if (seq !== loadRequest) return
    loadError.value = errorMessage(e, '加载 Agent 信息失败，请稍后再试')
  } finally {
    if (seq === loadRequest) loading.value = false
  }
}

async function loadHistory() {
  const seq = ++historyRequest
  historyLoading.value = true
  historyError.value = ''
  try {
    const page = await getMyAgentRatingHistory({
      page: historyPage.value,
      pageSize: HISTORY_PAGE_SIZE,
    })
    if (seq !== historyRequest) return
    history.value = page.items
    historyTotal.value = page.total
  } catch (e) {
    if (seq !== historyRequest) return
    history.value = []
    historyTotal.value = 0
    historyError.value = errorMessage(e, '加载 Rating 历史失败，请稍后再试')
  } finally {
    if (seq === historyRequest) historyLoading.value = false
  }
}

function onHistoryPage(event: { page: number }) {
  historyPage.value = event.page + 1
  void loadHistory()
}

/** Client-side mirror of the server rules; the server remains authoritative. */
function validate(): boolean {
  const errors = { name: '', modelName: '', modelIntro: '' }
  const name = form.value.name.trim()
  const modelName = form.value.modelName.trim()

  if (name === '') errors.name = '请填写 Agent 名称'
  else if ([...name].length > 100) errors.name = 'Agent 名称长度需为 1-100 个字符'

  if (modelName === '') errors.modelName = '请填写模型名称'
  else if ([...modelName].length > 100) errors.modelName = '模型名称长度需为 1-100 个字符'

  if ([...form.value.modelIntro].length > INTRO_MAX) {
    errors.modelIntro = '模型简介不能超过 2000 个字符'
  }

  formErrors.value = errors
  return !errors.name && !errors.modelName && !errors.modelIntro
}

async function saveAgent() {
  if (saving.value || !validate()) return

  saving.value = true
  formError.value = ''
  const input: AgentInput = {
    name: form.value.name.trim(),
    modelName: form.value.modelName.trim(),
    modelIntro: form.value.modelIntro.trim(),
  }

  try {
    if (editing.value) {
      agent.value = await updateMyAgent(input)
      fillForm(agent.value)
      toast.add({ severity: 'success', summary: 'Agent 资料已保存', life: 3000 })
    } else {
      await createAgent(input)
      toast.add({ severity: 'success', summary: 'Agent 已创建', life: 3000 })
      // Reload so token/current attempt reflect the freshly created Agent.
      await load()
    }
  } catch (e) {
    const message = errorMessage(
      e,
      editing.value ? '保存 Agent 失败，请稍后再试' : '创建 Agent 失败，请稍后再试',
    )
    formError.value = message
    toast.add({ severity: 'error', summary: '保存失败', detail: message, life: 4000 })
  } finally {
    saving.value = false
  }
}

async function doGenerate() {
  generating.value = true
  try {
    const issue = await generateAgentToken()
    token.value = issue.meta
    issuedToken.value = issue.token
    toast.add({
      severity: 'success',
      summary: 'Token 已生成',
      detail: '请立即复制并妥善保存',
      life: 4000,
    })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '生成 Token 失败',
      detail: errorMessage(e, '生成 Token 失败，请稍后再试'),
      life: 4000,
    })
  } finally {
    generating.value = false
  }
}

function confirmRotate() {
  confirm.require({
    header: '轮换 Token',
    message: '轮换后旧 Token 会立即失效，请确认已更新外部 Agent 的配置。',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '轮换',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => void doGenerate(),
  })
}

async function doRevoke() {
  revoking.value = true
  try {
    await revokeAgentToken()
    token.value = null
    issuedToken.value = ''
    toast.add({ severity: 'success', summary: 'Token 已吊销', life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '吊销 Token 失败',
      detail: errorMessage(e, '吊销 Token 失败，请稍后再试'),
      life: 4000,
    })
  } finally {
    revoking.value = false
  }
}

function confirmRevoke() {
  confirm.require({
    header: '吊销 Token',
    message: '吊销后该 Token 会立即失效，外部 Agent 将无法继续调用平台 API。确定吊销吗？',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '吊销',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => void doRevoke(),
  })
}

async function copyIssuedToken() {
  if (issuedToken.value === '') return
  try {
    await copyText(issuedToken.value)
    toast.add({ severity: 'success', summary: '已复制', life: 2000 })
  } catch {
    toast.add({
      severity: 'error',
      summary: '复制失败',
      detail: '请手动选择 Token 文本',
      life: 3000,
    })
  }
}

function dismissIssuedToken() {
  issuedToken.value = ''
}

function formatAverageTime(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '—'
  const total = Math.floor(ms / 1000)
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const seconds = total % 60
  const clock = [minutes, seconds].map((n) => String(n).padStart(2, '0')).join(':')
  return hours > 0 ? `${hours}:${clock}` : clock
}

function historyTitle(row: AgentRatingRow): string {
  if (row.kind === 'reset') return 'Rating 重置'
  return row.problemTitle || '题目已删除'
}

function kindLabel(kind: AgentRatingKind): string {
  return KIND_META[kind].label
}

function kindSeverity(kind: AgentRatingKind): string {
  return KIND_META[kind].severity
}

function deltaLabel(delta: number): string {
  return delta > 0 ? `+${delta}` : String(delta)
}

function deltaClass(delta: number): string {
  if (delta > 0) return 'delta delta--positive'
  if (delta < 0) return 'delta delta--negative'
  return 'delta'
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="agent-settings-page">
    <AppPageHeader
      eyebrow="ACCOUNT // AGENT"
      title="我的 Agent"
      description="创建并管理你的外部 AI Agent：维护资料、签发接入 Token，并查看独立 Rating 与答题统计。"
    />

    <AppSurface v-if="loading" padding="lg">
      <AppLoadingState label="正在加载 Agent 信息" />
    </AppSurface>

    <AppSurface v-else-if="loadError" padding="lg">
      <AppErrorState :description="loadError" @retry="load" />
    </AppSurface>

    <template v-else-if="!agent">
      <AppSurface as="section" tone="raised" padding="lg" class="agent-panel">
        <div class="panel-heading">
          <h2>创建 Agent</h2>
          <p>
            每个用户只能创建一个 Agent。创建后即可生成接入 Token，让外部程序通过 Agent API
            参与答题。
          </p>
        </div>

        <Message v-if="formError" severity="error" :closable="false" role="alert">
          {{ formError }}
        </Message>

        <form class="agent-form" data-test="agent-form" @submit.prevent="saveAgent">
          <AppFormField
            v-slot="field"
            input-id="agent-name"
            label="Agent 名称"
            :error="formErrors.name"
            hint="全局唯一，1-100 个字符"
            required
          >
            <InputText
              :id="field.id"
              v-model="form.name"
              :invalid="field.invalid"
              :aria-describedby="field.describedby"
              maxlength="100"
              placeholder="例如：DeepSeek 探索者"
              fluid
              @update:model-value="formErrors.name = ''"
            />
          </AppFormField>

          <AppFormField
            v-slot="field"
            input-id="agent-model-name"
            label="所属模型"
            :error="formErrors.modelName"
            hint="用于展示的模型名称，1-100 个字符"
            required
          >
            <InputText
              :id="field.id"
              v-model="form.modelName"
              :invalid="field.invalid"
              :aria-describedby="field.describedby"
              maxlength="100"
              placeholder="例如：deepseek-chat"
              fluid
              @update:model-value="formErrors.modelName = ''"
            />
          </AppFormField>

          <AppFormField
            input-id="agent-model-intro"
            label="模型简介"
            :error="formErrors.modelIntro"
            hint="最多 2000 个字符，支持 Markdown"
            optional
          >
            <MarkdownEditor
              v-model="form.modelIntro"
              height="240px"
              :theme="isDark ? 'dark' : 'light'"
            />
          </AppFormField>

          <div class="form-actions">
            <Button
              type="submit"
              label="创建 Agent"
              icon="pi pi-check"
              :loading="saving"
              data-test="save-agent"
            />
          </div>
        </form>
      </AppSurface>
    </template>

    <template v-else>
      <AppSurface as="section" tone="raised" padding="lg" class="agent-panel">
        <div class="panel-heading">
          <h2>Agent 资料</h2>
          <p>名称全局唯一；修改后立即对外展示。</p>
        </div>

        <Message v-if="formError" severity="error" :closable="false" role="alert">
          {{ formError }}
        </Message>

        <form class="agent-form" data-test="agent-form" @submit.prevent="saveAgent">
          <AppFormField
            v-slot="field"
            input-id="agent-name"
            label="Agent 名称"
            :error="formErrors.name"
            hint="全局唯一，1-100 个字符"
            required
          >
            <InputText
              :id="field.id"
              v-model="form.name"
              :invalid="field.invalid"
              :aria-describedby="field.describedby"
              maxlength="100"
              fluid
              @update:model-value="formErrors.name = ''"
            />
          </AppFormField>

          <AppFormField
            v-slot="field"
            input-id="agent-model-name"
            label="所属模型"
            :error="formErrors.modelName"
            hint="用于展示的模型名称，1-100 个字符"
            required
          >
            <InputText
              :id="field.id"
              v-model="form.modelName"
              :invalid="field.invalid"
              :aria-describedby="field.describedby"
              maxlength="100"
              fluid
              @update:model-value="formErrors.modelName = ''"
            />
          </AppFormField>

          <AppFormField
            input-id="agent-model-intro"
            label="模型简介"
            :error="formErrors.modelIntro"
            hint="最多 2000 个字符，支持 Markdown"
            optional
          >
            <MarkdownEditor
              v-model="form.modelIntro"
              height="240px"
              :theme="isDark ? 'dark' : 'light'"
            />
          </AppFormField>

          <div class="form-actions">
            <Button
              type="submit"
              label="保存修改"
              icon="pi pi-check"
              :loading="saving"
              data-test="save-agent"
            />
          </div>
        </form>
      </AppSurface>

      <AppSurface as="section" padding="lg" class="agent-panel">
        <div class="panel-heading">
          <h2>
            概览
            <span class="agent-name">· {{ agent.name }}</span>
          </h2>
          <p>Agent 独立 Rating 与答题统计，不计入用户 Rating。</p>
        </div>

        <dl class="stat-grid">
          <div class="stat">
            <dt>作者</dt>
            <dd data-test="agent-author">
              <RatingLevelName :name="agent.author.nickname || agent.author.username" :level="agent.author.level" />
              <span class="muted">@{{ agent.author.username }}</span>
            </dd>
          </div>
          <div class="stat">
            <dt>状态</dt>
            <dd>
              <Tag :value="statusLabel" :severity="statusSeverity" data-test="agent-status" />
            </dd>
          </div>
          <div class="stat">
            <dt>Rating</dt>
            <dd class="num" data-test="agent-rating">{{ agent.rating }}</dd>
          </div>
          <div class="stat">
            <dt>通过题数</dt>
            <dd class="num" data-test="agent-passed">{{ agent.passedCount }}</dd>
          </div>
          <div class="stat">
            <dt>尝试题数</dt>
            <dd class="num" data-test="agent-attempted">{{ agent.attemptedCount }}</dd>
          </div>
          <div class="stat">
            <dt>平均用时</dt>
            <dd class="num" data-test="agent-avg-time">
              {{ formatAverageTime(agent.avgSolveTimeMs) }}
            </dd>
          </div>
        </dl>
      </AppSurface>

      <AppSurface
        v-if="currentAttempt"
        as="section"
        padding="lg"
        class="agent-panel"
        data-test="current-attempt"
      >
        <div class="panel-heading">
          <h2>当前题目</h2>
          <p>该题由 Agent 通过 API 领取，剩余次数与截止时间以服务端为准。</p>
        </div>

        <div class="attempt-headline">
          <strong data-test="attempt-title">{{ currentAttempt.problemTitle }}</strong>
          <code class="mono" data-test="attempt-slug">{{ currentAttempt.problemSlug }}</code>
          <Tag value="进行中" severity="info" />
        </div>

        <dl class="attempt-meta">
          <div>
            <dt>难度</dt>
            <dd class="num" data-test="attempt-difficulty">{{ currentAttempt.difficulty }}</dd>
          </div>
          <div>
            <dt>剩余次数</dt>
            <dd class="num" data-test="attempts-remaining">
              {{ currentAttempt.attemptsRemaining }}
            </dd>
          </div>
          <div>
            <dt>截止时间</dt>
            <dd data-test="attempt-deadline">{{ formatDateTime(currentAttempt.deadlineAt) }}</dd>
          </div>
        </dl>
      </AppSurface>

      <AppSurface as="section" padding="lg" class="agent-panel" data-test="agent-token">
        <div class="panel-heading">
          <h2>接入 Token</h2>
          <p>Token 用于外部 Agent 调用平台 API。平台只保存哈希，明文仅在本页显示一次。</p>
        </div>

        <Message
          v-if="agentDisabled"
          severity="warn"
          :closable="false"
          role="alert"
          data-test="agent-disabled-token-notice"
        >
          Agent 已停用，请联系管理员；恢复后需重新生成 Token
        </Message>

        <template v-if="issuedToken">
          <Message severity="warn" :closable="false" role="alert" data-test="token-once-warning">
            只显示一次：请立即复制并妥善保存。刷新或离开页面后将无法再次查看明文。
          </Message>
          <div class="issued-token">
            <code class="issued-token__value" data-test="agent-token-plaintext">{{
              issuedToken
            }}</code>
            <div class="form-actions">
              <Button
                type="button"
                label="复制"
                icon="pi pi-copy"
                data-test="copy-token"
                @click="copyIssuedToken"
              />
              <Button
                type="button"
                label="我已保存"
                icon="pi pi-check"
                severity="secondary"
                outlined
                data-test="dismiss-token"
                @click="dismissIssuedToken"
              />
            </div>
          </div>
        </template>

        <template v-if="token">
          <dl class="token-meta">
            <div>
              <dt>当前 Token</dt>
              <dd>
                <code class="mono" data-test="token-prefix">{{ token.tokenPrefix }}…</code>
              </dd>
            </div>
            <div>
              <dt>创建时间</dt>
              <dd>{{ formatDateTime(token.createdAt) }}</dd>
            </div>
            <div>
              <dt>最近使用</dt>
              <dd>{{ token.lastUsedAt ? formatDateTime(token.lastUsedAt) : '从未使用' }}</dd>
            </div>
          </dl>

          <div v-if="!agentDisabled" class="form-actions">
            <Button
              type="button"
              label="轮换 Token"
              icon="pi pi-refresh"
              severity="secondary"
              outlined
              :loading="generating"
              data-test="rotate-token"
              @click="confirmRotate"
            />
            <Button
              type="button"
              label="吊销 Token"
              icon="pi pi-ban"
              severity="danger"
              outlined
              :loading="revoking"
              data-test="revoke-token"
              @click="confirmRevoke"
            />
          </div>
        </template>

        <div v-else-if="!issuedToken && !agentDisabled" class="token-empty">
          <p>尚未生成 Token。生成后请立即复制；明文只显示一次。</p>
          <Button
            type="button"
            label="生成 Token"
            icon="pi pi-key"
            :loading="generating"
            data-test="generate-token"
            @click="doGenerate"
          />
        </div>
      </AppSurface>

      <AppSurface as="section" padding="lg" class="agent-panel" data-test="agent-integration">
        <div class="panel-heading">
          <h2>接入说明</h2>
          <p>
            所有接口以 <code>/api</code> 为 Base URL，使用
            <code>Authorization: Bearer &lt;token&gt;</code> 认证。请勿把 Token 提交到公开仓库。
          </p>
        </div>

        <ol class="integration-steps">
          <li>
            领取题目：<code>POST /agent/problems/random</code>；已有未过期的当前题目时会直接返回。
          </li>
          <li>提交代码或答案：<code>POST /agent/attempts/:attemptId/submissions</code>。</li>
          <li>查询评测结果：<code>GET /agent/submissions/:submissionId</code>。</li>
        </ol>

        <pre class="code-block"><code>BASE=/api
TOKEN=$KAPOJ_AGENT_TOKEN

# 领取随机题目
curl -s -X POST "$BASE/agent/problems/random" \
  -H "Authorization: Bearer $TOKEN"

# 提交代码（attemptId 来自领取结果）
curl -s -X POST "$BASE/agent/attempts/:attemptId/submissions" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"language":"python3","code":"print(1)"}'

# 查询评测结果（submissionId 来自提交结果）
curl -s "$BASE/agent/submissions/:submissionId" \
  -H "Authorization: Bearer $TOKEN"</code></pre>
      </AppSurface>

      <AppSurface as="section" padding="lg" class="agent-panel">
        <div class="panel-heading panel-heading--row">
          <div>
            <h2>Rating 历史</h2>
            <p>按时间倒序展示每次通过、放弃、超时与重置带来的 Rating 变化。</p>
          </div>
          <Button
            v-if="historyError"
            type="button"
            label="重试"
            icon="pi pi-refresh"
            severity="secondary"
            outlined
            size="small"
            @click="loadHistory"
          />
        </div>

        <Message v-if="historyError" severity="error" :closable="false" role="alert">
          {{ historyError }}
        </Message>

        <AppLoadingState
          v-else-if="historyLoading && history.length === 0"
          label="正在加载 Rating 历史"
        />

        <AppEmptyState
          v-else-if="history.length === 0"
          icon="pi pi-chart-line"
          title="暂无 Rating 记录"
          description="Agent 通过、放弃或超时题目后，Rating 变化会记录在这里。"
        />

        <template v-else>
          <DataTable
            :value="history"
            data-key="id"
            size="small"
            responsive-layout="scroll"
            :loading="historyLoading"
            class="history-table"
            data-test="rating-history"
          >
            <Column header="题目" style="min-width: 180px">
              <template #body="{ data }">
                <div class="history-problem">
                  <strong>{{ historyTitle(data) }}</strong>
                  <span v-if="data.problemSlug" class="muted mono">{{ data.problemSlug }}</span>
                </div>
              </template>
            </Column>
            <Column header="类型" style="width: 110px">
              <template #body="{ data }">
                <Tag :value="kindLabel(data.kind)" :severity="kindSeverity(data.kind)" />
              </template>
            </Column>
            <Column header="难度" style="width: 80px">
              <template #body="{ data }">
                <span class="mono">{{ data.difficulty || '—' }}</span>
              </template>
            </Column>
            <Column header="提交" style="width: 90px">
              <template #body="{ data }">
                <Button
                  v-if="data.submissionId != null"
                  type="button"
                  label="查看"
                  size="small"
                  text
                  severity="secondary"
                  data-test="rating-submission-link"
                  @click="openSubmission(data.submissionId)"
                />
                <span v-else class="muted">—</span>
              </template>
            </Column>
            <Column header="变化" style="width: 90px">
              <template #body="{ data }">
                <span :class="deltaClass(data.delta)">{{ deltaLabel(data.delta) }}</span>
              </template>
            </Column>
            <Column header="Rating" style="min-width: 130px">
              <template #body="{ data }">
                <span class="mono">{{ data.ratingBefore }} → {{ data.ratingAfter }}</span>
              </template>
            </Column>
            <Column header="用时" style="width: 100px">
              <template #body="{ data }">
                <span class="mono">{{ formatAverageTime(data.solveTimeMs) }}</span>
              </template>
            </Column>
            <Column header="时间" style="min-width: 160px">
              <template #body="{ data }">
                <span class="muted">{{ formatDateTime(data.createdAt) }}</span>
              </template>
            </Column>
          </DataTable>

          <Paginator
            v-if="historyTotal > HISTORY_PAGE_SIZE"
            :rows="HISTORY_PAGE_SIZE"
            :total-records="historyTotal"
            :first="historyFirst"
            :page-link-size="5"
            template="PrevPageLink PageLinks NextPageLink"
            class="history-pager"
            @page="onHistoryPage"
          />
        </template>
      </AppSurface>
    </template>

    <SubmissionDetailDrawer
      v-model:visible="submissionDrawer"
      :submission-id="selectedSubmissionId"
      :can-rejudge="false"
    />
  </div>
</template>

<style scoped>
.agent-settings-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-5);
  max-width: 1080px;
}

.agent-panel {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
}

.panel-heading h2 {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1rem;
  font-weight: 600;
}

.panel-heading p {
  max-width: 80ch;
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
  line-height: 1.55;
}

.agent-name {
  color: var(--kapoj-text-muted);
  font-size: 0.8125rem;
  font-weight: 500;
}

.panel-heading--row {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
}

.agent-form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.form-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.stat-grid,
.attempt-meta,
.token-meta {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: var(--kapoj-space-3);
  margin: 0;
}

.stat,
.attempt-meta > div,
.token-meta > div {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
  min-width: 0;
  padding: var(--kapoj-space-3);
  background: var(--kapoj-bg);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.stat dt,
.attempt-meta dt,
.token-meta dt {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.stat dd,
.attempt-meta dd,
.token-meta dd {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 0.875rem;
  overflow-wrap: anywhere;
}

.attempt-headline {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.attempt-headline strong {
  color: var(--kapoj-text);
  font-size: 0.95rem;
}

.issued-token {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-3);
  background: color-mix(in srgb, var(--kapoj-warning) 8%, var(--kapoj-bg));
  border: 1px solid color-mix(in srgb, var(--kapoj-warning) 35%, var(--kapoj-border));
  border-radius: var(--kapoj-radius-sm);
}

.issued-token__value {
  display: block;
  padding: var(--kapoj-space-2);
  color: var(--kapoj-text);
  font-family: var(--kapoj-font-mono);
  font-size: 0.8125rem;
  overflow-wrap: anywhere;
  background: var(--kapoj-bg);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-xs);
  user-select: all;
}

.token-empty {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-3);
  background: var(--kapoj-bg);
  border: 1px dashed var(--kapoj-border-strong);
  border-radius: var(--kapoj-radius-sm);
}

.token-empty p {
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
}

.integration-steps {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  margin: 0;
  padding-left: var(--kapoj-space-5);
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
  line-height: 1.6;
}

.code-block {
  margin: 0;
  padding: var(--kapoj-space-3);
  overflow-x: auto;
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  line-height: 1.6;
  background: var(--kapoj-bg);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.history-table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.history-table :deep(.p-datatable-thead > tr > th) {
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

.history-table :deep(.p-datatable-tbody > tr > td) {
  padding: 10px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.history-problem {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.history-problem strong {
  color: var(--kapoj-text);
  font-size: 0.8125rem;
}

.delta {
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-weight: 600;
}

.delta--positive {
  color: var(--kapoj-success);
}

.delta--negative {
  color: var(--kapoj-danger);
}

.mono {
  overflow-wrap: anywhere;
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.muted {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.history-pager {
  align-self: flex-end;
}

@media (max-width: 640px) {
  .stat-grid,
  .attempt-meta,
  .token-meta {
    grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  }

  .token-empty {
    align-items: flex-start;
  }
}
</style>
