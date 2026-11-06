<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import MultiSelect from 'primevue/multiselect'
import Paginator from 'primevue/paginator'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import {
  adjustAdminUserCoins,
  banAdminUser,
  getAdminUser,
  listAdminRoles,
  listAdminUsers,
  replaceAdminUserRoles,
  unbanAdminUser,
  updateAdminUser,
} from '@/api/admin'
import { ApiError } from '@/api/http'
import type { AdminRoleItem, AdminUserItem, AdminUserUpdateInput, Status } from '@/api/types'
import { getProfile } from '@/api/users'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime } from '@/utils/format'
import { formatCoins } from '@/utils/gamification'
import { validateAvatarUrl, validateBio, validateProfileNickname } from '@/validation'

const PAGE_SIZE = 20

const STATUS_OPTIONS: { value: Status; label: string }[] = [
  { value: 'active', label: '正常' },
  { value: 'banned', label: '已封禁' },
]

const auth = useAuthStore()
const toast = useToast()
const confirm = useConfirm()

const canEdit = computed(() => auth.can(PERM.user.update))
const canBan = computed(() => auth.can(PERM.user.ban))
const canAdjustCoins = computed(() => auth.can(PERM.user.coins.adjust))
const canRoleRead = computed(() => auth.can(PERM.role.read))
const canAssignRoles = computed(() => auth.can(PERM.user.roles.assign) && canRoleRead.value)
const hasActions = computed(
  () => canEdit.value || canBan.value || canAdjustCoins.value || canAssignRoles.value,
)

const filters = ref({ keyword: '', status: null as Status | null, roleId: null as number | null })
const items = ref<AdminUserItem[]>([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const error = ref('')
const roles = ref<AdminRoleItem[]>([])

const roleFilterOptions = computed(() =>
  roles.value.map((role) => ({ value: role.id, label: role.name })),
)

const roleAssignOptions = computed(() =>
  roles.value.map((role) => ({
    value: role.id,
    label: role.name,
    disabled: role.code === 'super_admin' && !auth.isSuperAdmin,
  })),
)

// Only the newest request may update the page; slow older ones are ignored.
let latest = 0

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  try {
    const result = await listAdminUsers({
      keyword: filters.value.keyword.trim() || undefined,
      status: filters.value.status ?? undefined,
      roleId: filters.value.roleId ?? undefined,
      page: page.value,
      pageSize: PAGE_SIZE,
    })
    if (seq !== latest) return
    items.value = result.items
    total.value = result.total
  } catch (e) {
    if (seq !== latest) return
    error.value = e instanceof ApiError ? e.message : '加载用户失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

async function loadRoles() {
  try {
    roles.value = await listAdminRoles()
  } catch {
    roles.value = []
  }
}

function search() {
  page.value = 1
  void load()
}

function reset() {
  filters.value = { keyword: '', status: null, roleId: null }
  page.value = 1
  void load()
}

function onPage(event: { page: number }) {
  page.value = event.page + 1
  void load()
}

function fail(e: unknown, fallback: string) {
  toast.add({
    severity: 'error',
    summary: '操作失败',
    detail: e instanceof ApiError ? e.message : fallback,
    life: 4000,
  })
}

function isSelf(item: AdminUserItem): boolean {
  return auth.user?.id === item.id
}

function isSuperTarget(item: AdminUserItem): boolean {
  return item.roles.some((role) => role.code === 'super_admin')
}

/** Ban/unban of self or of a super target by a non-super actor is forbidden. */
function canOperateBan(item: AdminUserItem): boolean {
  if (isSelf(item)) return false
  if (isSuperTarget(item) && !auth.isSuperAdmin) return false
  return true
}

// --- Edit dialog -----------------------------------------------------------

const editDialogVisible = ref(false)
const editLoading = ref(false)
const editSaving = ref(false)
const editingId = ref<number | null>(null)
const editForm = ref({ nickname: '', bio: '', avatarUrl: '' })
const editErrors = ref({ nickname: '', bio: '', avatarUrl: '' })

async function openEdit(item: AdminUserItem) {
  editingId.value = item.id
  editForm.value = { nickname: '', bio: '', avatarUrl: '' }
  editErrors.value = { nickname: '', bio: '', avatarUrl: '' }
  editDialogVisible.value = true
  editLoading.value = true
  try {
    const [detail, profile] = await Promise.all([getAdminUser(item.id), getProfile(item.id)])
    if (editingId.value !== item.id) return
    // Nickname comes from the admin detail; bio/avatarUrl from the public profile.
    editForm.value = {
      nickname: detail.nickname,
      bio: profile.bio,
      avatarUrl: profile.avatarUrl,
    }
  } catch (e) {
    if (editingId.value !== item.id) return
    fail(e, '加载用户详情失败')
    editDialogVisible.value = false
  } finally {
    // Only the newest openEdit may settle the dialog/loading state.
    if (editingId.value === item.id) editLoading.value = false
  }
}

function validateEdit(): boolean {
  const nickname = validateProfileNickname(editForm.value.nickname)
  const bio = validateBio(editForm.value.bio)
  const avatarUrl = validateAvatarUrl(editForm.value.avatarUrl)
  editErrors.value = {
    nickname: nickname ?? '',
    bio: bio ?? '',
    avatarUrl: avatarUrl ?? '',
  }
  return !nickname && !bio && !avatarUrl
}

async function saveEdit() {
  if (!validateEdit()) return
  if (editingId.value === null) return
  editSaving.value = true
  try {
    const input: AdminUserUpdateInput = {
      nickname: editForm.value.nickname.trim(),
      bio: editForm.value.bio.trim(),
      avatarUrl: editForm.value.avatarUrl.trim(),
    }
    await updateAdminUser(editingId.value, input)
    toast.add({ severity: 'success', summary: '用户资料已更新', life: 3000 })
    editDialogVisible.value = false
    void load()
  } catch (e) {
    fail(e, '保存用户资料失败')
  } finally {
    editSaving.value = false
  }
}

// --- Role assignment dialog ------------------------------------------------

const roleDialogVisible = ref(false)
const roleSaving = ref(false)
const roleEditingId = ref<number | null>(null)
const roleForm = ref<number[]>([])

function openRoles(item: AdminUserItem) {
  roleEditingId.value = item.id
  roleForm.value = item.roles.map((role) => role.id)
  roleDialogVisible.value = true
}

async function saveRoles() {
  if (roleEditingId.value === null) return
  roleSaving.value = true
  try {
    await replaceAdminUserRoles(roleEditingId.value, roleForm.value)
    toast.add({ severity: 'success', summary: '角色已更新', life: 3000 })
    roleDialogVisible.value = false
    void load()
  } catch (e) {
    fail(e, '分配角色失败')
  } finally {
    roleSaving.value = false
  }
}

// --- Coin adjust dialog ----------------------------------------------------

const coinDialogVisible = ref(false)
const coinSaving = ref(false)
const coinEditingId = ref<number | null>(null)
const coinForm = ref({ amount: 0, note: '' })
const coinErrors = ref({ amount: '', note: '' })

function openCoins(item: AdminUserItem) {
  coinEditingId.value = item.id
  coinForm.value = { amount: 0, note: '' }
  coinErrors.value = { amount: '', note: '' }
  coinDialogVisible.value = true
}

function validateCoins(): boolean {
  const amount = coinForm.value.amount
  const note = coinForm.value.note.trim()
  let amountError = ''
  if (!Number.isInteger(amount)) amountError = '调整金额必须为整数'
  else if (amount === 0) amountError = '调整金额不能为 0'
  const noteError = note.length < 1 || note.length > 200 ? '备注需为 1–200 个字符' : ''
  coinErrors.value = { amount: amountError, note: noteError }
  return !amountError && !noteError
}

async function saveCoins() {
  if (!validateCoins()) return
  if (coinEditingId.value === null) return
  coinSaving.value = true
  try {
    await adjustAdminUserCoins(coinEditingId.value, {
      amount: coinForm.value.amount,
      note: coinForm.value.note.trim(),
    })
    toast.add({ severity: 'success', summary: '金币已调整', life: 3000 })
    coinDialogVisible.value = false
    void load()
  } catch (e) {
    fail(e, '调整金币失败')
  } finally {
    coinSaving.value = false
  }
}

// --- Ban / unban -----------------------------------------------------------

function confirmBan(item: AdminUserItem) {
  confirm.require({
    header: '封禁用户',
    message: `确定封禁用户「${item.nickname || item.username}」吗？封禁后该用户将无法登录管理后台。`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '封禁',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => void doBan(item),
  })
}

async function doBan(item: AdminUserItem) {
  try {
    await banAdminUser(item.id)
    toast.add({ severity: 'success', summary: '用户已封禁', life: 3000 })
    void load()
  } catch (e) {
    fail(e, '封禁失败')
  }
}

function confirmUnban(item: AdminUserItem) {
  confirm.require({
    header: '解封用户',
    message: `确定解封用户「${item.nickname || item.username}」吗？`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '解封',
    rejectLabel: '取消',
    acceptProps: { severity: 'warn' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => void doUnban(item),
  })
}

async function doUnban(item: AdminUserItem) {
  try {
    await unbanAdminUser(item.id)
    toast.add({ severity: 'success', summary: '用户已解封', life: 3000 })
    void load()
  } catch (e) {
    fail(e, '解封失败')
  }
}

onMounted(() => {
  void load()
  if (canRoleRead.value) void loadRoles()
})
</script>

<template>
  <div class="admin-users">
    <AppPageHeader
      eyebrow="ADMIN // USERS"
      title="用户管理"
      description="搜索、查看用户，并维护其角色、金币与封禁状态。"
    />

    <AppSurface padding="lg" class="admin-users__surface">
      <form class="filter-bar" role="search" @submit.prevent="search">
        <div class="filter-fields">
          <InputText
            v-model="filters.keyword"
            class="keyword-input"
            fluid
            placeholder="搜索用户名、昵称或邮箱"
            aria-label="搜索用户名、昵称或邮箱"
          />
          <Select
            v-model="filters.status"
            :options="STATUS_OPTIONS"
            option-label="label"
            option-value="value"
            placeholder="状态"
            show-clear
            aria-label="按状态筛选"
            fluid
          />
          <Select
            v-if="canRoleRead"
            v-model="filters.roleId"
            :options="roleFilterOptions"
            option-label="label"
            option-value="value"
            placeholder="角色"
            show-clear
            aria-label="按角色筛选"
            fluid
          />
        </div>
        <div class="filter-actions">
          <Button type="submit" label="搜索" icon="pi pi-search" />
          <Button type="button" label="重置" severity="secondary" text @click="reset" />
        </div>
      </form>

      <AppErrorState v-if="error && items.length === 0" :description="error" @retry="load" />

      <AppLoadingState v-if="loading && items.length === 0" label="正在加载用户" />
      <DataTable
        v-else-if="!error || items.length > 0"
        :value="items"
        :loading="loading"
        data-key="id"
        size="small"
        responsive-layout="scroll"
        class="admin-users__table"
      >
        <Column header="用户" style="min-width: 180px">
          <template #body="{ data }">
            <div class="identity">
              <span class="identity__nickname">{{ data.nickname || data.username }}</span>
              <span class="identity__handle">@{{ data.username }} · {{ data.email }}</span>
            </div>
          </template>
        </Column>
        <Column header="角色" style="min-width: 160px">
          <template #body="{ data }">
            <div v-if="data.roles.length > 0" class="role-tags">
              <Tag v-for="role in data.roles" :key="role.id" :value="role.name" severity="info" />
            </div>
            <span v-else class="muted">无</span>
          </template>
        </Column>
        <Column header="状态" style="width: 90px">
          <template #body="{ data }">
            <Tag
              :value="data.status === 'active' ? '正常' : '已封禁'"
              :severity="data.status === 'active' ? 'success' : 'danger'"
            />
          </template>
        </Column>
        <Column header="Rating / 金币" style="width: 150px">
          <template #body="{ data }">
            <span class="num">{{ data.rating }}</span>
            <span class="muted"> · {{ formatCoins(data.coins) }} 金币</span>
          </template>
        </Column>
        <Column header="注册时间" style="width: 170px">
          <template #body="{ data }">
            <span class="muted">{{ formatDateTime(data.createdAt) }}</span>
          </template>
        </Column>
        <Column v-if="hasActions" header="操作" style="width: 220px">
          <template #body="{ data }">
            <div class="row-actions">
              <Button v-if="canEdit" label="编辑" text size="small" @click="openEdit(data)" />
              <Button
                v-if="canAssignRoles"
                label="角色"
                text
                size="small"
                @click="openRoles(data)"
              />
              <Button
                v-if="canAdjustCoins"
                label="金币"
                text
                size="small"
                @click="openCoins(data)"
              />
              <template v-if="canBan">
                <Button
                  v-if="data.status === 'active'"
                  label="封禁"
                  text
                  size="small"
                  severity="danger"
                  :disabled="!canOperateBan(data)"
                  @click="confirmBan(data)"
                />
                <Button
                  v-else
                  label="解封"
                  text
                  size="small"
                  severity="warn"
                  :disabled="!canOperateBan(data)"
                  @click="confirmUnban(data)"
                />
              </template>
            </div>
          </template>
        </Column>
        <template #empty>
          <AppEmptyState
            icon="pi pi-users"
            title="没有找到用户"
            description="调整筛选条件，或清空搜索后重试。"
          />
        </template>
      </DataTable>

      <Paginator
        v-if="total > PAGE_SIZE"
        :rows="PAGE_SIZE"
        :total-records="total"
        :first="(page - 1) * PAGE_SIZE"
        :page-link-size="5"
        template="PrevPageLink PageLinks NextPageLink"
        class="admin-users__pager"
        @page="onPage"
      />
    </AppSurface>

    <!-- Edit dialog -->
    <Dialog
      v-model:visible="editDialogVisible"
      header="编辑用户"
      modal
      :draggable="false"
      :style="{ width: 'min(94vw, 560px)' }"
    >
      <AppLoadingState v-if="editLoading" label="正在加载用户详情" />
      <div v-else class="admin-form">
        <AppFormField label="昵称" required :error="editErrors.nickname">
          <InputText
            id="edit-nickname"
            v-model="editForm.nickname"
            class="nickname-input"
            maxlength="32"
            fluid
          />
        </AppFormField>
        <AppFormField label="个人简介" :error="editErrors.bio">
          <Textarea
            id="edit-bio"
            v-model="editForm.bio"
            class="bio-input"
            rows="3"
            maxlength="200"
            fluid
          />
        </AppFormField>
        <AppFormField label="头像链接" :error="editErrors.avatarUrl">
          <InputText
            id="edit-avatar"
            v-model="editForm.avatarUrl"
            class="avatar-input"
            maxlength="512"
            placeholder="http(s) 图片地址"
            fluid
          />
        </AppFormField>
      </div>

      <template #footer>
        <Button label="取消" severity="secondary" text @click="editDialogVisible = false" />
        <Button label="保存" icon="pi pi-check" :loading="editSaving" @click="saveEdit" />
      </template>
    </Dialog>

    <!-- Role assignment dialog -->
    <Dialog
      v-model:visible="roleDialogVisible"
      header="分配角色"
      modal
      :draggable="false"
      :style="{ width: 'min(94vw, 520px)' }"
    >
      <div class="admin-form">
        <AppFormField label="角色" hint="提交后将整体替换该用户的角色。">
          <MultiSelect
            v-model="roleForm"
            input-id="assign-roles"
            class="roles-select"
            :options="roleAssignOptions"
            option-label="label"
            option-value="value"
            option-disabled="disabled"
            display="chip"
            filter
            placeholder="选择角色"
            fluid
          />
        </AppFormField>
      </div>

      <template #footer>
        <Button label="取消" severity="secondary" text @click="roleDialogVisible = false" />
        <Button label="保存" icon="pi pi-check" :loading="roleSaving" @click="saveRoles" />
      </template>
    </Dialog>

    <!-- Coin adjust dialog -->
    <Dialog
      v-model:visible="coinDialogVisible"
      header="调整金币"
      modal
      :draggable="false"
      :style="{ width: 'min(94vw, 480px)' }"
    >
      <div class="admin-form">
        <AppFormField label="调整金额" required :error="coinErrors.amount">
          <InputNumber
            v-model="coinForm.amount"
            input-id="coin-amount"
            class="amount-input"
            :step="1"
            :min-fraction-digits="0"
            fluid
          />
        </AppFormField>
        <AppFormField label="备注" required :error="coinErrors.note">
          <Textarea
            id="coin-note"
            v-model="coinForm.note"
            class="note-input"
            rows="3"
            maxlength="200"
            placeholder="调整原因，写入流水"
            fluid
          />
        </AppFormField>
      </div>

      <template #footer>
        <Button label="取消" severity="secondary" text @click="coinDialogVisible = false" />
        <Button label="保存" icon="pi pi-check" :loading="coinSaving" @click="saveCoins" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.admin-users {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.admin-users__surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
}

.filter-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
}

.filter-fields {
  display: flex;
  flex: 1 1 420px;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
}

.filter-fields > * {
  flex: 1 1 180px;
  min-width: 0;
}

.filter-actions {
  display: flex;
  gap: var(--kapoj-space-2);
}

.admin-users__table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.admin-users__table :deep(.p-datatable-thead > tr > th) {
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

.admin-users__table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.identity {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.identity__nickname {
  color: var(--kapoj-text);
  font-weight: 600;
}

.identity__handle {
  overflow: hidden;
  color: var(--kapoj-text-muted);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.role-tags,
.row-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-1);
}

.muted {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.admin-users__pager {
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.admin-form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
}
</style>
