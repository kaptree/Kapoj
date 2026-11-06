<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import TabPanel from 'primevue/tabpanel'
import TabPanels from 'primevue/tabpanels'
import Tabs from 'primevue/tabs'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import {
  cloneContest,
  createAnnouncement,
  deleteAnnouncement,
  deleteContest,
  getContest,
  listAnnouncements,
  listContestParticipants,
  registerContest,
  updateAnnouncement,
} from '@/api/contests'
import { ApiError } from '@/api/http'
import type {
  AnnouncementItem,
  ContestDetail,
  ContestParticipant,
  ContestStatus,
} from '@/api/types'
import ContestRanklist from '@/components/ContestRanklist.vue'
import MarkdownView from '@/components/MarkdownView.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'
import { useTheme } from '@/theme/theme'
import {
  contestCountdown,
  contestModeLabel,
  contestStatusLabel,
  registerTypeLabel,
} from '@/utils/contest'
import { formatDateTime } from '@/utils/format'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const toast = useToast()
const confirm = useConfirm()
const { isDark } = useTheme()
const contestId = computed(() => Number(route.params.id))

const contest = ref<ContestDetail | null>(null)
const announcements = ref<AnnouncementItem[]>([])
const countdown = ref<ReturnType<typeof contestCountdown>>(null)
const loading = ref(false)
const error = ref('')
const tab = ref('problems')

const registerDialog = ref(false)
const registerForm = ref({ password: '', inviteCode: '' })
const registering = ref(false)

const participantsDialog = ref(false)
const participants = ref<ContestParticipant[]>([])
const participantsLoading = ref(false)

const announcementDialog = ref(false)
const editingAnnouncement = ref<AnnouncementItem | null>(null)
const announcementForm = ref({ title: '', contentMd: '' })
const savingAnnouncement = ref(false)

const cloneDialog = ref(false)
const cloneTitle = ref('')
const cloneError = ref('')
const cloning = ref(false)

const refreshTick = ref(0)
const serverSkew = ref(0)
const refreshError = ref('')
let latest = 0
let ticker: number | undefined
let source: EventSource | null = null
let reloadTimer: number | undefined
let syncTimer: number | undefined

const canReadParticipants = computed(() => auth.can(PERM.contest.participant.read))
const canEdit = computed(() => auth.can(PERM.contest.update))
const canClone = computed(() => auth.can(PERM.contest.create))
const canDelete = computed(() => auth.can(PERM.contest.delete))
const canManageAnnouncements = computed(() => auth.can(PERM.contest.announcement.manage))
const canTestSubmit = computed(() => auth.can(PERM.contest.update))
const isLoggedIn = computed(() => auth.isLoggedIn)
const loginTo = computed(() => ({
  name: 'login',
  query: { redirect: route.fullPath },
}))
const canSubmit = computed(
  () => contest.value?.status === 'running' && (contest.value.registered || canTestSubmit.value),
)
const canRegister = computed(() => {
  const current = contest.value
  return (
    Boolean(current) &&
    isLoggedIn.value &&
    !current!.registered &&
    !canTestSubmit.value &&
    current!.status !== 'ended'
  )
})
const problemsEmptyText = computed(() => {
  const status = contest.value?.status
  if (status === 'upcoming') return '题目将在比赛开始后公布。'
  if (status === 'ended') return '本场比赛没有公开题目。'
  return '暂无题目。'
})

async function loadAnnouncements() {
  announcements.value = await listAnnouncements(contestId.value).catch(() => [])
}

async function loadContest(background = false) {
  const seq = ++latest
  if (!background) {
    loading.value = true
    error.value = ''
  } else {
    refreshError.value = ''
  }
  try {
    const data = await getContest(contestId.value)
    if (seq !== latest) return
    contest.value = data
    serverSkew.value = new Date(data.serverTime).getTime() - Date.now()
    updateCountdown()
    if (!background) void loadAnnouncements()
  } catch (e) {
    if (seq !== latest) return
    const message =
      e instanceof ApiError && e.code === 40400 ? '比赛不存在或无权访问' : '加载比赛失败'
    if (contest.value) refreshError.value = '刷新失败，稍后将自动重试'
    else error.value = message
  } finally {
    if (seq === latest && !background) loading.value = false
  }
}

function updateCountdown() {
  if (!contest.value) {
    countdown.value = null
    return
  }
  const next = contestCountdown(contest.value.startTime, contest.value.endTime, serverSkew.value)
  if (next && countdown.value && next.phase !== countdown.value.phase) {
    refreshTick.value++
    void loadContest(true)
  }
  countdown.value = next
}

function scheduleRefresh() {
  if (reloadTimer !== undefined) return
  reloadTimer = window.setTimeout(() => {
    reloadTimer = undefined
    refreshTick.value++
    void loadContest(true)
  }, 300)
}

function connectStream() {
  if (typeof EventSource === 'undefined') return
  source = new EventSource('/api/submissions/stream')
  source.onopen = scheduleRefresh
  source.onmessage = scheduleRefresh
}

onMounted(() => {
  void loadContest()
  ticker = window.setInterval(updateCountdown, 1000)
  syncTimer = window.setInterval(scheduleRefresh, 30000)
  connectStream()
})
onBeforeUnmount(() => {
  source?.close()
  if (ticker !== undefined) window.clearInterval(ticker)
  if (reloadTimer !== undefined) window.clearTimeout(reloadTimer)
  if (syncTimer !== undefined) window.clearInterval(syncTimer)
})

function contestTagSeverity(status: ContestStatus): 'success' | 'warn' | 'info' {
  if (status === 'running') return 'success'
  if (status === 'upcoming') return 'warn'
  return 'info'
}

function doRegister() {
  if (!contest.value) return
  if (contest.value.registerType === 'open') {
    void sendRegister({})
    return
  }
  registerForm.value = { password: '', inviteCode: '' }
  registerDialog.value = true
}

function resetRegister() {
  registerForm.value = { password: '', inviteCode: '' }
}

async function sendRegister(input: { password?: string; inviteCode?: string }) {
  if (!contest.value) return
  registering.value = true
  try {
    await registerContest(contest.value.id, input)
    toast.add({ severity: 'success', summary: '报名成功', life: 3000 })
    registerDialog.value = false
    refreshTick.value++
    await loadContest(true)
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '报名失败',
      detail: e instanceof ApiError ? e.message : '报名失败',
      life: 4000,
    })
  } finally {
    registering.value = false
  }
}

function openSubmit(problemId: number) {
  void router.push({
    name: 'problem-submit',
    params: { id: problemId },
    query: { contestId: String(contestId.value) },
  })
}

function openEdit() {
  if (!contest.value) return
  void router.push({ name: 'contest-edit', params: { id: contest.value.id } })
}

async function openParticipants() {
  participantsDialog.value = true
  participantsLoading.value = true
  try {
    participants.value = await listContestParticipants(contestId.value)
  } catch {
    participants.value = []
  } finally {
    participantsLoading.value = false
  }
}

function openClone() {
  if (!contest.value) return
  cloneTitle.value = `${contest.value.title}（副本）`
  cloneError.value = ''
  cloneDialog.value = true
}

function resetClone() {
  cloneTitle.value = ''
  cloneError.value = ''
}

async function submitClone() {
  if (!contest.value) return
  const title = cloneTitle.value.trim()
  if (!title) {
    cloneError.value = '请输入比赛标题'
    return
  }

  cloning.value = true
  cloneError.value = ''
  try {
    const created = await cloneContest(contest.value.id, { title })
    toast.add({ severity: 'success', summary: '比赛已克隆', life: 3000 })
    cloneDialog.value = false
    await router.push({ name: 'contest-edit', params: { id: created.id } })
  } catch (e) {
    cloneError.value = e instanceof ApiError ? e.message : '克隆失败'
    toast.add({
      severity: 'error',
      summary: '克隆失败',
      detail: cloneError.value,
      life: 4000,
    })
  } finally {
    cloning.value = false
  }
}

function removeContest() {
  confirm.require({
    header: '删除比赛',
    message: '确定删除这场比赛吗？',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => {
      void doRemoveContest()
    },
  })
}

async function doRemoveContest() {
  if (!contest.value) return
  try {
    await deleteContest(contest.value.id)
    toast.add({ severity: 'success', summary: '已删除', life: 3000 })
    await router.push({ name: 'contests' })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '删除失败',
      detail: e instanceof ApiError ? e.message : '删除失败',
      life: 4000,
    })
  }
}

function openAnnouncement(item?: AnnouncementItem) {
  editingAnnouncement.value = item ?? null
  announcementForm.value = {
    title: item?.title ?? '',
    contentMd: item?.contentMd ?? '',
  }
  announcementDialog.value = true
}

async function saveAnnouncement() {
  if (!contest.value) return
  savingAnnouncement.value = true
  try {
    const input = {
      title: announcementForm.value.title,
      contentMd: announcementForm.value.contentMd,
    }
    if (editingAnnouncement.value) {
      await updateAnnouncement(contest.value.id, editingAnnouncement.value.id, input)
    } else {
      await createAnnouncement(contest.value.id, input)
    }
    toast.add({ severity: 'success', summary: '公告已保存', life: 3000 })
    announcementDialog.value = false
    await loadAnnouncements()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '保存公告失败',
      detail: e instanceof ApiError ? e.message : '保存公告失败',
      life: 4000,
    })
  } finally {
    savingAnnouncement.value = false
  }
}

function removeAnnouncement(item: AnnouncementItem) {
  confirm.require({
    header: '删除公告',
    message: `删除公告「${item.title}」？`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => {
      void doRemoveAnnouncement(item)
    },
  })
}

async function doRemoveAnnouncement(item: AnnouncementItem) {
  if (!contest.value) return
  try {
    await deleteAnnouncement(contest.value.id, item.id)
    await loadAnnouncements()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: '删除公告失败',
      detail: e instanceof ApiError ? e.message : '删除公告失败',
      life: 4000,
    })
  }
}
</script>

<template>
  <div class="contest-detail">
    <AppSurface v-if="error && !contest" padding="lg" class="state-surface">
      <AppErrorState :description="error" retry-label="重新加载" @retry="loadContest" />
      <RouterLink :to="{ name: 'contests' }" class="back-link">
        <Button label="返回比赛列表" icon="pi pi-arrow-left" text />
      </RouterLink>
    </AppSurface>

    <AppLoadingState v-else-if="loading && !contest" label="正在加载比赛" />

    <template v-else-if="contest">
      <Message v-if="refreshError" severity="warn" :closable="false" class="refresh-banner">
        {{ refreshError }}
      </Message>

      <AppPageHeader
        eyebrow="CONTEST // ARENA"
        :title="contest?.title ?? ''"
        description="查看赛程、题目、榜单与比赛公告。"
      >
        <template #actions>
          <Button v-if="canRegister" label="报名" icon="pi pi-user-plus" @click="doRegister" />
          <RouterLink v-else-if="!isLoggedIn" :to="loginTo" class="login-link">
            <Button label="登录后报名" icon="pi pi-sign-in" />
          </RouterLink>
          <Button
            v-if="canReadParticipants"
            label="报名用户"
            icon="pi pi-users"
            severity="secondary"
            outlined
            @click="openParticipants"
          />
          <Button
            v-if="canEdit"
            label="编辑"
            icon="pi pi-pencil"
            severity="secondary"
            text
            @click="openEdit"
          />
          <Button
            v-if="canClone"
            label="克隆"
            icon="pi pi-copy"
            severity="secondary"
            text
            @click="openClone"
          />
          <Button
            v-if="canDelete"
            label="删除"
            icon="pi pi-trash"
            severity="danger"
            outlined
            @click="removeContest"
          />
        </template>
      </AppPageHeader>

      <AppSurface padding="md" class="meta-surface">
        <div class="meta-row">
          <Tag
            :value="contestStatusLabel(contest?.status ?? 'upcoming')"
            :severity="contestTagSeverity(contest?.status ?? 'upcoming')"
          />
          <Tag :value="contestModeLabel(contest?.mode ?? 'icpc')" severity="secondary" />
          <Tag :value="registerTypeLabel(contest?.registerType ?? 'open')" severity="secondary" />
          <Tag v-if="contest?.registered" value="已报名" severity="success" />
          <span v-if="countdown" class="countdown">{{ countdown.text }}</span>
        </div>
        <div class="meta-times">
          <span>开始：{{ formatDateTime(contest?.startTime ?? '') }}</span>
          <span>结束：{{ formatDateTime(contest?.endTime ?? '') }}</span>
          <span>{{ contest?.participantCount ?? 0 }} 人报名</span>
        </div>
      </AppSurface>

      <AppSurface v-if="contest?.descriptionMd" padding="lg" class="description-surface">
        <MarkdownView :source="contest?.descriptionMd ?? ''" :theme="isDark ? 'dark' : 'light'" />
      </AppSurface>

      <AppSurface padding="lg" class="content-surface">
        <Tabs v-model:value="tab" lazy>
          <TabList>
            <Tab value="problems">题目</Tab>
            <Tab value="ranklist">榜单</Tab>
            <Tab value="announcements">公告</Tab>
          </TabList>
          <TabPanels>
            <TabPanel value="problems">
              <Message
                v-if="!contest?.problems.length"
                severity="info"
                :closable="false"
                class="problems-empty"
              >
                {{ problemsEmptyText }}
              </Message>
              <DataTable
                v-else
                :value="contest?.problems ?? []"
                data-key="problemId"
                size="small"
                responsive-layout="scroll"
                class="problems-table"
              >
                <Column header="编号" style="width: 70px">
                  <template #body="{ data }">
                    <span class="num">{{ data.displayId }}</span>
                  </template>
                </Column>
                <Column header="题目" style="min-width: 220px">
                  <template #body="{ data }">
                    <RouterLink
                      :to="{
                        name: 'problem-detail',
                        params: { id: data.problemId },
                        query: { contestId: String(contestId) },
                      }"
                      class="problem-link"
                    >
                      {{ data.title }}
                    </RouterLink>
                  </template>
                </Column>
                <Column header="分数" style="width: 80px">
                  <template #body="{ data }">
                    <span class="num">{{ data.score }}</span>
                  </template>
                </Column>
                <Column header="操作" style="width: 120px">
                  <template #body="{ data }">
                    <Button
                      v-if="canSubmit"
                      label="提交"
                      size="small"
                      text
                      icon="pi pi-send"
                      @click="openSubmit(data.problemId)"
                    />
                  </template>
                </Column>
                <template #empty>
                  <AppEmptyState icon="pi pi-list" title="暂无题目" />
                </template>
              </DataTable>
            </TabPanel>

            <TabPanel value="ranklist">
              <ContestRanklist :contest-id="contest?.id ?? contestId" :refresh-tick="refreshTick" />
            </TabPanel>

            <TabPanel value="announcements">
              <div class="announcement-head">
                <span class="announcement-heading">比赛公告</span>
                <Button
                  v-if="canManageAnnouncements"
                  label="发布公告"
                  icon="pi pi-megaphone"
                  size="small"
                  @click="openAnnouncement()"
                />
              </div>
              <AppEmptyState
                v-if="!announcements.length"
                icon="pi pi-megaphone"
                title="暂无公告"
                description="比赛相关通知会显示在这里。"
              />
              <div v-for="item in announcements" :key="item.id" class="announcement">
                <div class="announcement-title">
                  <strong>{{ item.title }}</strong>
                  <span class="announcement-time">{{ formatDateTime(item.createdAt) }}</span>
                  <span v-if="canManageAnnouncements" class="announcement-ops">
                    <Button label="编辑" text size="small" @click="openAnnouncement(item)" />
                    <Button
                      label="删除"
                      text
                      size="small"
                      severity="danger"
                      @click="removeAnnouncement(item)"
                    />
                  </span>
                </div>
                <MarkdownView :source="item.contentMd" :theme="isDark ? 'dark' : 'light'" />
              </div>
            </TabPanel>
          </TabPanels>
        </Tabs>
      </AppSurface>
    </template>

    <Dialog
      v-model:visible="registerDialog"
      header="报名比赛"
      modal
      :draggable="false"
      :style="{ width: 'min(92vw, 420px)' }"
      @hide="resetRegister"
    >
      <div class="dialog-form">
        <AppFormField v-if="contest?.registerType === 'password'" label="比赛密码" required>
          <Password
            v-model="registerForm.password"
            :feedback="false"
            toggle-mask
            fluid
            input-id="register-password"
            placeholder="请输入比赛密码"
          />
        </AppFormField>
        <AppFormField v-if="contest?.registerType === 'invite'" label="邀请码" required>
          <InputText v-model="registerForm.inviteCode" fluid input-id="register-invite" />
        </AppFormField>
      </div>
      <template #footer>
        <Button label="取消" severity="secondary" text @click="registerDialog = false" />
        <Button
          label="报名"
          icon="pi pi-user-plus"
          :loading="registering"
          class="register-submit"
          @click="sendRegister(registerForm)"
        />
      </template>
    </Dialog>

    <Dialog
      v-model:visible="participantsDialog"
      header="报名用户"
      modal
      :draggable="false"
      :style="{ width: 'min(92vw, 520px)' }"
    >
      <AppLoadingState v-if="participantsLoading" label="正在加载报名用户" />
      <DataTable
        v-else
        :value="participants"
        data-key="userId"
        size="small"
        responsive-layout="scroll"
        class="participants-table"
      >
        <Column header="用户" style="min-width: 140px">
          <template #body="{ data }">
            {{ data.nickname || data.username }}
          </template>
        </Column>
        <Column header="报名时间" style="min-width: 160px">
          <template #body="{ data }">{{ formatDateTime(data.registerTime) }}</template>
        </Column>
        <template #empty>
          <AppEmptyState icon="pi pi-users" title="暂无报名用户" />
        </template>
      </DataTable>
      <template #footer>
        <Button label="关闭" severity="secondary" text @click="participantsDialog = false" />
      </template>
    </Dialog>

    <Dialog
      v-model:visible="announcementDialog"
      :header="editingAnnouncement ? '编辑公告' : '发布公告'"
      modal
      :draggable="false"
      :style="{ width: 'min(92vw, 620px)' }"
    >
      <div class="dialog-form">
        <AppFormField label="标题" required :hint="`${announcementForm.title.length}/100`">
          <InputText
            v-model="announcementForm.title"
            maxlength="100"
            fluid
            input-id="announcement-title"
          />
        </AppFormField>
        <AppFormField label="内容" required>
          <Textarea
            v-model="announcementForm.contentMd"
            rows="10"
            fluid
            placeholder="支持 Markdown"
            input-id="announcement-content"
          />
        </AppFormField>
      </div>
      <template #footer>
        <Button label="取消" severity="secondary" text @click="announcementDialog = false" />
        <Button
          label="保存"
          icon="pi pi-check"
          :loading="savingAnnouncement"
          class="announcement-submit"
          @click="saveAnnouncement"
        />
      </template>
    </Dialog>

    <Dialog
      v-model:visible="cloneDialog"
      header="克隆比赛"
      modal
      :draggable="false"
      :style="{ width: 'min(92vw, 420px)' }"
      @hide="resetClone"
    >
      <AppFormField label="新比赛标题" :error="cloneError" required>
        <InputText
          v-model="cloneTitle"
          fluid
          input-id="clone-title"
          :invalid="Boolean(cloneError)"
          @update:model-value="cloneError = ''"
        />
      </AppFormField>
      <template #footer>
        <Button label="取消" severity="secondary" text @click="cloneDialog = false" />
        <Button
          label="克隆"
          icon="pi pi-copy"
          :loading="cloning"
          class="clone-submit"
          @click="submitClone"
        />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.contest-detail {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.state-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  align-items: flex-start;
}

.back-link {
  color: var(--kapoj-primary);
  font-size: 0.875rem;
}

.refresh-banner {
  margin: 0;
}

.meta-surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.meta-row,
.meta-times {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-3);
}

.meta-times {
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
}

.countdown {
  padding: 2px var(--kapoj-space-2);
  color: var(--kapoj-primary);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  font-weight: 600;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 32%, transparent);
  border-radius: var(--kapoj-radius-xs);
}

.description-surface {
  line-height: 1.7;
}

.content-surface {
  min-width: 0;
}

.problems-empty,
.problems-table {
  margin-top: var(--kapoj-space-2);
}

.problems-table,
.participants-table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.problems-table :deep(.p-datatable-thead > tr > th),
.participants-table :deep(.p-datatable-thead > tr > th) {
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

.problems-table :deep(.p-datatable-tbody > tr > td),
.participants-table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.problem-link {
  color: var(--kapoj-text);
  font-weight: 600;
}

.problem-link:hover {
  color: var(--kapoj-primary);
}

.announcement-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
  margin-bottom: var(--kapoj-space-3);
}

.announcement-heading {
  color: var(--kapoj-text);
  font-size: 1rem;
  font-weight: 600;
}

.announcement {
  padding: var(--kapoj-space-3) 0;
  border-bottom: 1px solid var(--kapoj-border);
}

.announcement:last-child {
  border-bottom: 0;
}

.announcement-title {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-3);
  margin-bottom: var(--kapoj-space-2);
}

.announcement-title strong {
  color: var(--kapoj-text);
}

.announcement-time {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
}

.announcement-ops {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-1);
  margin-left: auto;
}

.dialog-form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

@media (max-width: 700px) {
  .meta-times {
    gap: var(--kapoj-space-2);
  }
}
</style>
