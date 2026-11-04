<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'

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
import type { AnnouncementItem, ContestDetail, ContestParticipant } from '@/api/types'
import ContestRanklist from '@/components/ContestRanklist.vue'
import MarkdownView from '@/components/MarkdownView.vue'
import { useAuthStore } from '@/stores/auth'
import {
  contestCountdown,
  contestModeLabel,
  contestStatusLabel,
  contestStatusTagType,
  registerTypeLabel,
} from '@/utils/contest'
import { formatDateTime } from '@/utils/format'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const contestId = computed(() => Number(route.params.id))

const contest = ref<ContestDetail | null>(null)
const announcements = ref<AnnouncementItem[]>([])
const countdown = ref<ReturnType<typeof contestCountdown>>(null)
const loading = ref(false)
const error = ref('')
const tab = ref('problems')

const registerDialog = ref(false)
const registerForm = reactive({ password: '', inviteCode: '' })
const registering = ref(false)

const participantsDialog = ref(false)
const participants = ref<ContestParticipant[]>([])

const announcementDialog = ref(false)
const editingAnnouncement = ref<AnnouncementItem | null>(null)
const announcementForm = reactive({ title: '', contentMd: '' })
const savingAnnouncement = ref(false)

const refreshTick = ref(0)
let latest = 0
let ticker: number | undefined
let source: EventSource | null = null
let reloadTimer: number | undefined
let syncTimer: number | undefined

const canManage = computed(() => contest.value?.canManage ?? false)
const canSubmit = computed(
  () => contest.value?.status === 'running' && (contest.value.registered || canManage.value),
)
const canRegister = computed(
  () =>
    Boolean(contest.value) &&
    auth.isLoggedIn &&
    !contest.value!.registered &&
    !canManage.value &&
    contest.value!.status !== 'ended',
)

async function loadContest() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const data = await getContest(contestId.value)
    if (seq !== latest) return
    contest.value = data
    updateCountdown()
    const items = await listAnnouncements(contestId.value).catch(() => [])
    if (seq === latest) announcements.value = items
  } catch (e) {
    if (seq !== latest) return
    error.value =
      e instanceof ApiError && e.code === 40400 ? '比赛不存在或无权访问' : '加载比赛失败'
  } finally {
    if (seq === latest) loading.value = false
  }
}

function updateCountdown() {
  if (!contest.value) {
    countdown.value = null
    return
  }
  const next = contestCountdown(
    contest.value.startTime,
    contest.value.endTime,
    contest.value.serverTime,
  )
  if (next && countdown.value && next.phase !== countdown.value.phase) {
    void loadContest()
  }
  countdown.value = next
}

function scheduleRefresh() {
  if (reloadTimer !== undefined) return
  reloadTimer = window.setTimeout(() => {
    reloadTimer = undefined
    refreshTick.value++
    void loadContest()
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
  syncTimer = window.setInterval(() => void loadContest(), 30000)
  connectStream()
})
onBeforeUnmount(() => {
  source?.close()
  if (ticker !== undefined) window.clearInterval(ticker)
  if (reloadTimer !== undefined) window.clearTimeout(reloadTimer)
  if (syncTimer !== undefined) window.clearInterval(syncTimer)
})

async function doRegister() {
  if (!contest.value) return
  if (contest.value.registerType === 'open') {
    await sendRegister({})
    return
  }
  registerForm.password = ''
  registerForm.inviteCode = ''
  registerDialog.value = true
}

async function sendRegister(input: { password?: string; inviteCode?: string }) {
  if (!contest.value) return
  registering.value = true
  try {
    await registerContest(contest.value.id, input)
    ElMessage.success('报名成功')
    registerDialog.value = false
    await loadContest()
  } catch (e) {
    ElMessage.error(e instanceof ApiError ? e.message : '报名失败')
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

async function openParticipants() {
  participantsDialog.value = true
  participants.value = await listContestParticipants(contestId.value).catch(() => [])
}

async function cloneCurrent() {
  if (!contest.value) return
  try {
    const { value } = await ElMessageBox.prompt('新比赛标题', '克隆比赛', {
      inputValue: `${contest.value.title}（副本）`,
    })
    const created = await cloneContest(contest.value.id, { title: value })
    ElMessage.success('已克隆')
    await router.push({ name: 'contest-edit', params: { id: created.id } })
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
    ElMessage.error(e instanceof ApiError ? e.message : '克隆失败')
  }
}

async function removeContest() {
  if (!contest.value) return
  try {
    await ElMessageBox.confirm('确定删除这场比赛吗？', '删除比赛', { type: 'warning' })
  } catch {
    return
  }
  try {
    await deleteContest(contest.value.id)
    ElMessage.success('已删除')
    await router.push({ name: 'contests' })
  } catch (e) {
    ElMessage.error(e instanceof ApiError ? e.message : '删除失败')
  }
}

function openAnnouncement(item?: AnnouncementItem) {
  editingAnnouncement.value = item ?? null
  announcementForm.title = item?.title ?? ''
  announcementForm.contentMd = item?.contentMd ?? ''
  announcementDialog.value = true
}

async function saveAnnouncement() {
  if (!contest.value) return
  savingAnnouncement.value = true
  try {
    const input = { title: announcementForm.title, contentMd: announcementForm.contentMd }
    if (editingAnnouncement.value) {
      await updateAnnouncement(contest.value.id, editingAnnouncement.value.id, input)
    } else {
      await createAnnouncement(contest.value.id, input)
    }
    ElMessage.success('公告已保存')
    announcementDialog.value = false
    announcements.value = await listAnnouncements(contest.value.id)
  } catch (e) {
    ElMessage.error(e instanceof ApiError ? e.message : '保存公告失败')
  } finally {
    savingAnnouncement.value = false
  }
}

async function removeAnnouncement(item: AnnouncementItem) {
  if (!contest.value) return
  try {
    await ElMessageBox.confirm(`删除公告「${item.title}」？`, '删除公告', { type: 'warning' })
  } catch {
    return
  }
  try {
    await deleteAnnouncement(contest.value.id, item.id)
    announcements.value = await listAnnouncements(contest.value.id)
  } catch (e) {
    ElMessage.error(e instanceof ApiError ? e.message : '删除公告失败')
  }
}
</script>

<template>
  <div v-loading="loading" class="page">
    <el-result v-if="error" icon="warning" :title="error">
      <template #extra>
        <RouterLink :to="{ name: 'contests' }">
          <el-button type="primary">返回比赛列表</el-button>
        </RouterLink>
      </template>
    </el-result>

    <template v-else-if="contest">
      <el-card shadow="never" class="head-card">
        <div class="head">
          <div>
            <h1>{{ contest.title }}</h1>
            <div class="meta">
              <el-tag :type="contestStatusTagType(contest.status)" size="small">
                {{ contestStatusLabel(contest.status) }}
              </el-tag>
              <el-tag size="small" effect="plain">{{ contestModeLabel(contest.mode) }}</el-tag>
              <el-tag size="small" effect="plain">{{
                registerTypeLabel(contest.registerType)
              }}</el-tag>
              <el-tag v-if="contest.registered" type="success" size="small" effect="plain">
                已报名
              </el-tag>
              <span v-if="countdown">{{ countdown.text }}</span>
            </div>
            <div class="times">
              <span>开始：{{ formatDateTime(contest.startTime) }}</span>
              <span>结束：{{ formatDateTime(contest.endTime) }}</span>
              <span>{{ contest.participantCount }} 人报名</span>
            </div>
          </div>
          <div class="actions">
            <el-button v-if="canRegister" type="primary" @click="doRegister">报名</el-button>
            <RouterLink
              v-else-if="!auth.isLoggedIn"
              :to="{ name: 'login', query: { redirect: route.fullPath } }"
            >
              <el-button type="primary">登录后报名</el-button>
            </RouterLink>
            <el-button v-if="canManage" @click="openParticipants">报名用户</el-button>
            <el-button
              v-if="canManage"
              @click="router.push({ name: 'contest-edit', params: { id: contest.id } })"
            >
              编辑
            </el-button>
            <el-button v-if="canManage" @click="cloneCurrent">克隆</el-button>
            <el-button v-if="canManage" type="danger" plain @click="removeContest">删除</el-button>
          </div>
        </div>
      </el-card>

      <el-card v-if="contest.descriptionMd" shadow="never" class="description-card">
        <MarkdownView :source="contest.descriptionMd" />
      </el-card>

      <el-card shadow="never">
        <el-tabs v-model="tab">
          <el-tab-pane label="题目" name="problems">
            <el-alert
              v-if="!contest.problems.length"
              title="题目将在比赛开始后公布。"
              type="info"
              show-icon
              :closable="false"
              class="banner"
            />
            <el-table v-else :data="contest.problems" row-key="problemId">
              <el-table-column prop="displayId" label="编号" width="70" />
              <el-table-column label="题目" min-width="220">
                <template #default="{ row }">
                  <RouterLink :to="{ name: 'problem-detail', params: { id: row.problemId } }">
                    {{ row.title }}
                  </RouterLink>
                </template>
              </el-table-column>
              <el-table-column prop="score" label="分数" width="80" />
              <el-table-column label="操作" width="140">
                <template #default="{ row }">
                  <el-button
                    v-if="canSubmit"
                    text
                    type="primary"
                    @click="openSubmit(row.problemId)"
                  >
                    提交
                  </el-button>
                </template>
              </el-table-column>
            </el-table>
          </el-tab-pane>

          <el-tab-pane label="榜单" name="ranklist" lazy>
            <ContestRanklist :contest-id="contest.id" :refresh-tick="refreshTick" />
          </el-tab-pane>

          <el-tab-pane label="公告" name="announcements">
            <div class="announcement-head">
              <span>比赛公告</span>
              <el-button v-if="canManage" size="small" @click="openAnnouncement()"
                >发布公告</el-button
              >
            </div>
            <el-empty v-if="!announcements.length" description="暂无公告" :image-size="60" />
            <div v-for="item in announcements" :key="item.id" class="announcement">
              <div class="announcement-title">
                <strong>{{ item.title }}</strong>
                <span class="time">{{ formatDateTime(item.createdAt) }}</span>
                <span v-if="canManage" class="ops">
                  <el-button text size="small" @click="openAnnouncement(item)">编辑</el-button>
                  <el-button text size="small" type="danger" @click="removeAnnouncement(item)">
                    删除
                  </el-button>
                </span>
              </div>
              <MarkdownView :source="item.contentMd" />
            </div>
          </el-tab-pane>
        </el-tabs>
      </el-card>
    </template>

    <el-dialog v-model="registerDialog" title="报名比赛" width="420px">
      <el-form label-width="90px">
        <el-form-item v-if="contest?.registerType === 'password'" label="比赛密码">
          <el-input v-model="registerForm.password" type="password" show-password />
        </el-form-item>
        <el-form-item v-if="contest?.registerType === 'invite'" label="邀请码">
          <el-input v-model="registerForm.inviteCode" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="registerDialog = false">取消</el-button>
        <el-button type="primary" :loading="registering" @click="sendRegister(registerForm)">
          报名
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="participantsDialog" title="报名用户" width="520px">
      <el-table :data="participants" empty-text="暂无报名用户">
        <el-table-column label="用户">
          <template #default="{ row }">{{ row.nickname || row.username }}</template>
        </el-table-column>
        <el-table-column label="报名时间">
          <template #default="{ row }">{{ formatDateTime(row.registerTime) }}</template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <el-dialog
      v-model="announcementDialog"
      :title="editingAnnouncement ? '编辑公告' : '发布公告'"
      width="620px"
    >
      <el-form label-width="70px">
        <el-form-item label="标题">
          <el-input v-model="announcementForm.title" maxlength="100" show-word-limit />
        </el-form-item>
        <el-form-item label="内容">
          <el-input
            v-model="announcementForm.contentMd"
            type="textarea"
            :rows="10"
            placeholder="支持 Markdown"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="announcementDialog = false">取消</el-button>
        <el-button type="primary" :loading="savingAnnouncement" @click="saveAnnouncement">
          保存
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page {
  min-height: 200px;
}
.head-card,
.description-card {
  margin-bottom: 16px;
}
.head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}
h1 {
  margin: 0;
  font-size: 24px;
}
.meta,
.times,
.actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
}
.meta {
  margin-top: 10px;
}
.times {
  margin-top: 8px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.banner {
  margin-bottom: 12px;
}
.announcement-head,
.announcement-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.announcement-head {
  margin-bottom: 12px;
}
.announcement {
  padding: 12px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
}
.announcement-title {
  margin-bottom: 8px;
}
.time,
.ops {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
</style>
