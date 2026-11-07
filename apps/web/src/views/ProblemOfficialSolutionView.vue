<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Button from 'primevue/button'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/http'
import {
  getOfficialSolution,
  purchaseOfficialSolution,
  updateOfficialSolution,
} from '@/api/problems'
import type { OfficialSolutionView } from '@/api/types'
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import MarkdownView from '@/components/MarkdownView.vue'
import AppEmptyState from '@/components/ui/AppEmptyState.vue'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppPageHeader from '@/components/ui/AppPageHeader.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useAuthStore } from '@/stores/auth'
import { useTheme } from '@/theme/theme'
import { formatCoins } from '@/utils/gamification'

const route = useRoute()
const auth = useAuthStore()
const toast = useToast()
const confirm = useConfirm()
const { isDark } = useTheme()

const problemId = computed(() => {
  const id = route.params.id
  return typeof id === 'string' ? Number(id) : null
})

const view = ref<OfficialSolutionView | null>(null)
const loading = ref(false)
const error = ref('')
const editing = ref(false)
const saving = ref(false)
const purchasing = ref(false)

const editMd = ref('')
const editCode = ref('')
const editPrice = ref(0)

const accessLabel = computed(() => {
  switch (view.value?.access) {
    case 'creator':
      return '创建者'
    case 'accepted':
      return '已通过'
    case 'purchased':
      return '已解锁'
    case 'free':
      return '免费'
    case 'locked':
      return '未解锁'
    case 'guest':
      return '游客'
    default:
      return ''
  }
})

const accessSeverity = computed(() => {
  if (view.value?.access === 'locked') return 'warn'
  if (view.value?.access === 'guest') return 'secondary'
  return 'success'
})

const priceLabel = computed(() => {
  const price = view.value?.priceCoins ?? 0
  return price > 0 ? `金币 ${formatCoins(price)}` : '免费'
})

const loginTarget = computed(() => ({ name: 'login', query: { redirect: route.fullPath } }))

let latest = 0

async function load() {
  const seq = ++latest
  loading.value = true
  error.value = ''
  view.value = null
  editing.value = false
  if (problemId.value === null) {
    loading.value = false
    error.value = '题目不存在'
    return
  }
  try {
    const data = await getOfficialSolution(problemId.value)
    if (seq !== latest) return
    view.value = data
  } catch (e) {
    if (seq !== latest) return
    error.value =
      e instanceof ApiError && e.code === 40400
        ? '题目不存在'
        : e instanceof ApiError
          ? e.message
          : '加载官方题解失败，请稍后再试'
  } finally {
    if (seq === latest) loading.value = false
  }
}

watch(() => route.params.id, load, { immediate: true })

function startEdit() {
  editMd.value = view.value?.solutionMd ?? ''
  editCode.value = view.value?.solutionCode ?? ''
  editPrice.value = view.value?.priceCoins ?? 0
  editing.value = true
}

function cancelEdit() {
  editing.value = false
}

async function save() {
  if (problemId.value === null) return
  saving.value = true
  error.value = ''
  try {
    const updated = await updateOfficialSolution(problemId.value, {
      solutionMd: editMd.value,
      solutionCode: editCode.value,
      priceCoins: editPrice.value,
    })
    view.value = updated
    editing.value = false
    toast.add({ severity: 'success', summary: '已保存', life: 3000 })
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '保存失败，请稍后再试'
  } finally {
    saving.value = false
  }
}

function purchase() {
  const price = view.value?.priceCoins ?? 0
  confirm.require({
    header: '解锁官方题解',
    message: `支付 ${formatCoins(price)} 金币查看官方题解？`,
    icon: 'pi pi-lock',
    acceptLabel: '支付',
    rejectLabel: '取消',
    accept: () => {
      void doPurchase()
    },
  })
}

async function doPurchase() {
  if (problemId.value === null) return
  purchasing.value = true
  error.value = ''
  try {
    view.value = await purchaseOfficialSolution(problemId.value)
    await auth.fetchMe().catch(() => undefined)
    toast.add({ severity: 'success', summary: '已解锁', life: 3000 })
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '购买失败，请稍后再试'
  } finally {
    purchasing.value = false
  }
}
</script>

<template>
  <div class="official-solution-page">
    <AppPageHeader
      eyebrow="PROBLEM // SOLUTION"
      title="官方题解"
      :description="`题目 #${problemId}`"
    >
      <template #actions>
        <Button
          as="router-link"
          :to="`/problems/${problemId}`"
          label="返回题目"
          icon="pi pi-arrow-left"
          severity="secondary"
          outlined
        />
      </template>
    </AppPageHeader>

    <AppSurface v-if="loading && !view" padding="lg">
      <AppLoadingState label="正在加载官方题解" />
    </AppSurface>

    <AppSurface v-else-if="error && !view" padding="lg">
      <AppErrorState :description="error" @retry="load" />
    </AppSurface>

    <template v-else-if="view">
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>

      <AppSurface padding="lg" class="solution-card">
        <div class="solution-meta">
          <Tag :value="accessLabel" :severity="accessSeverity" />
          <span class="solution-price">{{ priceLabel }}</span>
        </div>

        <template v-if="view.access === 'guest'">
          <AppEmptyState
            icon="pi pi-lock"
            title="登录后查看"
            description="登录后可购买，或通过该题后免费查看。"
          />
          <div class="cta-row">
            <RouterLink :to="loginTarget">
              <Button label="登录 / 注册" icon="pi pi-sign-in" />
            </RouterLink>
          </div>
        </template>

        <template v-else-if="view.access === 'locked'">
          <div class="locked-panel">
            <p class="locked-text">
              本题官方题解需支付 {{ formatCoins(view.priceCoins) }} 金币解锁，或通过该题后免费查看。
            </p>
            <Button
              v-if="view.canPurchase"
              :label="`支付 ${formatCoins(view.priceCoins)} 金币查看`"
              icon="pi pi-lock"
              :loading="purchasing"
              @click="purchase"
            />
          </div>
        </template>

        <template v-else>
          <div v-if="view.canEdit && !editing" class="cta-row">
            <Button label="编辑官方题解" icon="pi pi-pencil" @click="startEdit" />
          </div>

          <AppEmptyState
            v-if="!view.solutionMd && !view.solutionCode"
            icon="pi pi-file"
            title="暂无官方题解"
            description="题解发布后会显示在这里。"
          />
          <div v-else class="solution-body">
            <section v-if="view.solutionMd">
              <h2 class="section-title">官方说明</h2>
              <MarkdownView :source="view.solutionMd" :theme="isDark ? 'dark' : 'light'" />
            </section>
            <section v-if="view.solutionCode">
              <h2 class="section-title">官方代码</h2>
              <pre class="code-block" tabindex="0">{{ view.solutionCode }}</pre>
            </section>
          </div>
        </template>

        <div v-if="editing" class="edit-form">
          <label for="official-md" class="field-label">官方说明</label>
          <MarkdownEditor v-model="editMd" :theme="isDark ? 'dark' : 'light'" />
          <label for="official-code" class="field-label">官方代码</label>
          <Textarea id="official-code" v-model="editCode" rows="14" class="mono code-textarea" />
          <label for="official-price" class="field-label">价格（金币，0 表示免费）</label>
          <InputNumber id="official-price" v-model="editPrice" :min="0" :max="1000000" />
          <div class="edit-actions">
            <Button label="保存" icon="pi pi-check" :loading="saving" @click="save" />
            <Button label="取消" severity="secondary" outlined @click="cancelEdit" />
          </div>
        </div>
      </AppSurface>
    </template>
  </div>
</template>

<style scoped>
.official-solution-page {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  max-width: var(--kapoj-content-max);
}

.solution-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.solution-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2);
}

.solution-price {
  color: var(--kapoj-warning);
  font-size: 0.9rem;
  font-weight: 600;
}

.cta-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
}

.locked-panel {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--kapoj-space-3);
}

.locked-text {
  margin: 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.875rem;
}

.solution-body section + section {
  margin-top: var(--kapoj-space-5);
}

.section-title {
  margin: 0 0 var(--kapoj-space-3);
  color: var(--kapoj-text);
  font-size: 1.05rem;
  line-height: 1.3;
}

.code-block {
  max-height: 480px;
  padding: var(--kapoj-space-3);
  margin: 0;
  overflow: auto;
  color: var(--kapoj-text);
  font-family: var(--kapoj-font-mono);
  font-size: 12px;
  line-height: 1.6;
  background: var(--kapoj-bg);
  border: 1px solid var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.edit-form {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  padding-top: var(--kapoj-space-4);
  border-top: 1px solid var(--kapoj-border);
}

.field-label {
  color: var(--kapoj-text-secondary);
  font-size: 0.8125rem;
  font-weight: 600;
}

.edit-form :deep(.p-inputnumber) {
  width: 220px;
}

.edit-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-2);
  margin-top: var(--kapoj-space-2);
}

.mono :deep(textarea) {
  font-family: var(--kapoj-font-mono);
}
</style>
