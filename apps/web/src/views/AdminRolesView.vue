<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import {
  createAdminRole,
  deleteAdminRole,
  listAdminPermissions,
  listAdminRoles,
  replaceAdminRolePermissions,
  updateAdminRole,
} from '@/api/admin'
import { ApiError } from '@/api/http'
import type {
  AdminPermissionItem,
  AdminRoleInput,
  AdminRoleItem,
  AdminRoleUpdateInput,
} from '@/api/types'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppFormField from '@/components/ui/AppFormField.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { PERM } from '@/permissions'
import { useAuthStore } from '@/stores/auth'

const ROLE_CODE_PATTERN = /^[a-z][a-z0-9_]{2,63}$/

const auth = useAuthStore()
const toast = useToast()
const confirm = useConfirm()

const canCreate = computed(() => auth.can(PERM.role.create))
const canUpdate = computed(() => auth.can(PERM.role.update))
const canDelete = computed(() => auth.can(PERM.role.delete))
const canPermissionsUpdate = computed(() => auth.can(PERM.role.permissions.update))
const hasActions = computed(() => canUpdate.value || canDelete.value || canPermissionsUpdate.value)

const roles = ref<AdminRoleItem[]>([])
const permissions = ref<AdminPermissionItem[]>([])
const loading = ref(false)
const error = ref('')

/** Permission directory grouped by groupName, preserving API order. */
const permissionGroups = computed(() => {
  const map = new Map<string, AdminPermissionItem[]>()
  for (const permission of permissions.value) {
    const list = map.get(permission.groupName)
    if (list) list.push(permission)
    else map.set(permission.groupName, [permission])
  }
  return Array.from(map, ([groupName, items]) => ({ groupName, items }))
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [roleList, permissionList] = await Promise.all([listAdminRoles(), listAdminPermissions()])
    roles.value = roleList
    permissions.value = permissionList
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '加载角色失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

function fail(e: unknown, fallback: string) {
  toast.add({
    severity: 'error',
    summary: '操作失败',
    detail: e instanceof ApiError ? e.message : fallback,
    life: 4000,
  })
}

// --- Editor dialog (create + edit) ------------------------------------------

type EditorMode = 'create' | 'edit'

const editorVisible = ref(false)
const editorMode = ref<EditorMode>('create')
const editingId = ref<number | null>(null)
const editingRole = ref<AdminRoleItem | null>(null)
const form = ref({ code: '', name: '', description: '' })
const permissionCodes = ref<string[]>([])
const saving = ref(false)
const formErrors = ref({ code: '', name: '' })

const isEdit = computed(() => editorMode.value === 'edit')
const editingIsSystem = computed(() => editingRole.value?.isSystem === true)
const editingIsSuper = computed(() => editingRole.value?.code === 'super_admin')

const codeDisabled = computed(() => isEdit.value)
const nameDisabled = computed(() => isEdit.value && (editingIsSystem.value || !canUpdate.value))
const descDisabled = computed(() => isEdit.value && !canUpdate.value)
const matrixDisabled = computed(
  () => isEdit.value && (editingIsSuper.value || !canPermissionsUpdate.value),
)

function openCreate() {
  editorMode.value = 'create'
  editingId.value = null
  editingRole.value = null
  form.value = { code: '', name: '', description: '' }
  permissionCodes.value = []
  formErrors.value = { code: '', name: '' }
  editorVisible.value = true
}

function openEdit(role: AdminRoleItem) {
  editorMode.value = 'edit'
  editingId.value = role.id
  editingRole.value = role
  form.value = { code: role.code, name: role.name, description: role.description }
  permissionCodes.value = [...role.permissions]
  formErrors.value = { code: '', name: '' }
  editorVisible.value = true
}

function validateCreate(): boolean {
  const code = form.value.code.trim()
  const name = form.value.name.trim()
  const codeError = ROLE_CODE_PATTERN.test(code)
    ? ''
    : 'code 需为 3–64 位小写字母、数字或下划线，且以字母开头'
  const nameError = name === '' ? '请输入角色名称' : ''
  formErrors.value = { code: codeError, name: nameError }
  return !codeError && !nameError
}

function validateEdit(): boolean {
  const name = form.value.name.trim()
  const nameError = name === '' ? '请输入角色名称' : ''
  formErrors.value = { code: '', name: nameError }
  return !nameError
}

function permissionsChanged(): boolean {
  const original = editingRole.value?.permissions ?? []
  const next = permissionCodes.value
  if (original.length !== next.length) return true
  const set = new Set(original)
  return next.some((code) => !set.has(code))
}

function metadataChanged(): boolean {
  const role = editingRole.value
  if (!role) return false
  const name = form.value.name.trim()
  const description = form.value.description.trim() || undefined
  return name !== role.name || description !== (role.description || undefined)
}

async function saveCreate() {
  if (!validateCreate()) return
  saving.value = true
  try {
    const input: AdminRoleInput = {
      code: form.value.code.trim(),
      name: form.value.name.trim(),
      description: form.value.description.trim() || undefined,
      permissionCodes: permissionCodes.value,
    }
    await createAdminRole(input)
    toast.add({ severity: 'success', summary: '角色已创建', life: 3000 })
    editorVisible.value = false
    await load()
  } catch (e) {
    fail(e, '创建角色失败')
  } finally {
    saving.value = false
  }
}

async function saveEdit() {
  if (editingId.value === null) return
  if (!validateEdit()) return

  const id = editingId.value
  const updateMeta = canUpdate.value && metadataChanged()
  const updatePerms = canPermissionsUpdate.value && !editingIsSuper.value && permissionsChanged()

  // Nothing actually changed (or we lack the capability to change anything):
  // do not fire a false "updated" toast.
  if (!updateMeta && !updatePerms) {
    toast.add({ severity: 'info', summary: '无变更', life: 3000 })
    return
  }

  saving.value = true
  try {
    if (updateMeta) {
      const input: AdminRoleUpdateInput = {
        name: form.value.name.trim(),
        description: form.value.description.trim() || undefined,
      }
      await updateAdminRole(id, input)
    }

    if (updatePerms) {
      try {
        await replaceAdminRolePermissions(id, permissionCodes.value)
      } catch (e) {
        if (updateMeta) {
          // Metadata already saved; make the partial failure explicit and refresh
          // so the list reflects the new metadata. Keep the dialog open.
          toast.add({
            severity: 'error',
            summary: '角色信息已保存，权限保存失败',
            detail: e instanceof ApiError ? e.message : '权限保存失败',
            life: 5000,
          })
          await load()
        } else {
          fail(e, '权限保存失败')
        }
        return
      }
    }

    toast.add({ severity: 'success', summary: '角色已更新', life: 3000 })
    editorVisible.value = false
    await load()
  } catch (e) {
    fail(e, '保存角色信息失败')
  } finally {
    saving.value = false
  }
}

function save() {
  if (isEdit.value) void saveEdit()
  else void saveCreate()
}

// --- Permission matrix helpers ----------------------------------------------

function groupCodes(groupName: string): string[] {
  return (
    permissionGroups.value
      .find((group) => group.groupName === groupName)
      ?.items.map((permission) => permission.code) ?? []
  )
}

function selectGroup(groupName: string) {
  const set = new Set(permissionCodes.value)
  for (const code of groupCodes(groupName)) set.add(code)
  permissionCodes.value = Array.from(set)
}

function clearGroup(groupName: string) {
  const set = new Set(permissionCodes.value)
  for (const code of groupCodes(groupName)) set.delete(code)
  permissionCodes.value = Array.from(set)
}

// --- Delete -----------------------------------------------------------------

function confirmDelete(role: AdminRoleItem) {
  confirm.require({
    header: '删除角色',
    message: `确定删除自定义角色「${role.name}」吗？该角色的用户关联将被移除。`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', outlined: true },
    accept: () => void doDelete(role),
  })
}

async function doDelete(role: AdminRoleItem) {
  try {
    await deleteAdminRole(role.id)
    toast.add({ severity: 'success', summary: '角色已删除', life: 3000 })
    await load()
  } catch (e) {
    fail(e, '删除角色失败')
  }
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="admin-roles">
    <AppPageHeader eyebrow="ADMIN // ROLES" title="角色权限" description="管理角色及其权限矩阵。">
      <template #actions>
        <Button v-if="canCreate" label="新建角色" icon="pi pi-plus" @click="openCreate" />
      </template>
    </AppPageHeader>

    <AppSurface padding="lg" class="admin-roles__surface">
      <AppErrorState v-if="error && roles.length === 0" :description="error" @retry="load" />

      <AppLoadingState v-if="loading && roles.length === 0" label="正在加载角色" />
      <DataTable
        v-else-if="!error || roles.length > 0"
        :value="roles"
        data-key="id"
        size="small"
        responsive-layout="scroll"
        class="admin-roles__table"
      >
        <Column header="名称" style="min-width: 160px">
          <template #body="{ data }">
            <div class="role-name">
              <span class="role-name__text">{{ data.name }}</span>
              <Tag v-if="data.isSystem" value="系统" severity="secondary" />
            </div>
          </template>
        </Column>
        <Column header="Code" style="min-width: 150px">
          <template #body="{ data }">
            <code class="role-code">{{ data.code }}</code>
          </template>
        </Column>
        <Column header="权限数" style="width: 90px">
          <template #body="{ data }">
            <span v-if="data.code === 'super_admin'" class="muted">全部</span>
            <span v-else class="num">{{ data.permissions.length }}</span>
          </template>
        </Column>
        <Column header="用户数" style="width: 90px">
          <template #body="{ data }">
            <span class="num">{{ data.userCount }}</span>
          </template>
        </Column>
        <Column header="描述" style="min-width: 200px">
          <template #body="{ data }">
            <span class="muted">{{ data.description || '—' }}</span>
          </template>
        </Column>
        <Column v-if="hasActions" header="操作" style="width: 170px">
          <template #body="{ data }">
            <div class="row-actions">
              <Button v-if="canUpdate" label="编辑" text size="small" @click="openEdit(data)" />
              <Button
                v-if="canPermissionsUpdate && data.code !== 'super_admin'"
                label="权限"
                text
                size="small"
                @click="openEdit(data)"
              />
              <Button
                v-if="canDelete && !data.isSystem"
                label="删除"
                text
                size="small"
                severity="danger"
                @click="confirmDelete(data)"
              />
            </div>
          </template>
        </Column>
        <template #empty>
          <AppEmptyState
            icon="pi pi-shield"
            title="暂无角色"
            description="创建自定义角色以开始配置权限。"
          />
        </template>
      </DataTable>
    </AppSurface>

    <Dialog
      v-model:visible="editorVisible"
      :header="isEdit ? '编辑角色' : '新建角色'"
      modal
      :draggable="false"
      :style="{ width: 'min(94vw, 680px)' }"
    >
      <div class="admin-form">
        <div class="form-grid">
          <AppFormField label="Code" required :error="formErrors.code">
            <InputText
              id="role-code"
              v-model="form.code"
              class="code-input"
              :disabled="codeDisabled"
              maxlength="64"
              placeholder="小写字母、数字、下划线"
              fluid
            />
          </AppFormField>
          <AppFormField label="名称" required :error="formErrors.name">
            <InputText
              id="role-name"
              v-model="form.name"
              class="name-input"
              :disabled="nameDisabled"
              maxlength="32"
              fluid
            />
          </AppFormField>
        </div>
        <AppFormField label="描述" optional>
          <Textarea
            id="role-desc"
            v-model="form.description"
            class="desc-input"
            :disabled="descDisabled"
            rows="2"
            maxlength="200"
            fluid
          />
        </AppFormField>

        <div v-if="editingIsSuper" class="permission-bypass">
          <i class="pi pi-lock" aria-hidden="true" />
          <span>绕过全部权限检查</span>
        </div>
        <div v-else class="permission-matrix">
          <p class="permission-matrix__title">权限</p>
          <div v-for="group in permissionGroups" :key="group.groupName" class="permission-group">
            <div class="permission-group__header">
              <h3 class="permission-group__name">{{ group.groupName }}</h3>
              <div class="permission-group__actions">
                <Button
                  label="全选"
                  size="small"
                  text
                  :data-select-all="group.groupName"
                  :disabled="matrixDisabled"
                  @click="selectGroup(group.groupName)"
                />
                <Button
                  label="清空"
                  size="small"
                  text
                  :data-clear="group.groupName"
                  :disabled="matrixDisabled"
                  @click="clearGroup(group.groupName)"
                />
              </div>
            </div>
            <div class="permission-group__items">
              <label
                v-for="permission in group.items"
                :key="permission.code"
                class="permission-option"
                :for="`perm-${permission.code}`"
              >
                <Checkbox
                  v-model="permissionCodes"
                  :value="permission.code"
                  :input-id="`perm-${permission.code}`"
                  :disabled="matrixDisabled"
                />
                <span class="permission-option__body">
                  <span class="permission-option__name">{{ permission.name }}</span>
                  <span class="permission-option__code">{{ permission.code }}</span>
                </span>
              </label>
            </div>
          </div>
        </div>
      </div>

      <template #footer>
        <Button label="取消" severity="secondary" text @click="editorVisible = false" />
        <Button
          :label="isEdit ? '保存' : '创建'"
          icon="pi pi-check"
          :loading="saving"
          @click="save"
        />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.admin-roles {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.admin-roles__surface {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
}

.admin-roles__table {
  overflow: hidden;
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.admin-roles__table :deep(.p-datatable-thead > tr > th) {
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

.admin-roles__table :deep(.p-datatable-tbody > tr > td) {
  padding: 8px var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  border-color: var(--kapoj-border);
}

.role-name,
.row-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.role-name__text {
  color: var(--kapoj-text);
  font-weight: 600;
}

.role-code {
  padding: 2px 6px;
  color: var(--kapoj-text-secondary);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-xs);
}

.muted {
  color: var(--kapoj-text-muted);
  font-size: 12px;
}

.admin-form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--kapoj-space-3);
}

.permission-bypass {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  padding: var(--kapoj-space-3);
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
  background: var(--kapoj-surface-hover);
  border: 1px dashed var(--kapoj-border-strong);
  border-radius: var(--kapoj-radius-md);
}

.permission-matrix {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
  min-width: 0;
}

.permission-matrix__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 0.9rem;
  font-weight: 600;
}

.permission-group {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
  padding: var(--kapoj-space-2);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-md);
}

.permission-group__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
}

.permission-group__name {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 0.8rem;
  font-weight: 600;
}

.permission-group__actions {
  display: flex;
  gap: var(--kapoj-space-1);
}

.permission-group__items {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-1);
}

.permission-option {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  padding: var(--kapoj-space-1);
  cursor: pointer;
  border-radius: var(--kapoj-radius-sm);
}

.permission-option:hover {
  background: var(--kapoj-surface-hover);
}

.permission-option__body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.permission-option__name {
  color: var(--kapoj-text);
  font-size: 0.8125rem;
}

.permission-option__code {
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 11px;
}
</style>
