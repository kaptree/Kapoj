<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import Tag from 'primevue/tag'

import { listUserCertificates } from '@/api/certificates'
import { ApiError } from '@/api/http'
import type { ProfileCertificate, ProfileCertificatePage } from '@/api/types'
import AppErrorState from '@/components/ui/AppErrorState.vue'
import AppLoadingState from '@/components/ui/AppLoadingState.vue'
import AppSurface from '@/components/ui/AppSurface.vue'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{
  targetUserId: number
}>()

const auth = useAuthStore()

const pageData = ref<ProfileCertificatePage | null>(null)
const loading = ref(false)
const error = ref('')
let latestLoad = 0

const certificates = computed(() => pageData.value?.items ?? [])
const isSelf = computed(() => auth.user?.id != null && auth.user.id === props.targetUserId)

function approvedDate(certificate: ProfileCertificate): string {
  return certificate.approvedAt ? formatDateTime(certificate.approvedAt) : '—'
}

async function load() {
  const seq = ++latestLoad
  loading.value = true
  error.value = ''
  try {
    const data = await listUserCertificates(props.targetUserId)
    if (seq !== latestLoad) return
    pageData.value = data
  } catch (e) {
    if (seq !== latestLoad) return
    pageData.value = null
    error.value = e instanceof ApiError ? e.message : '证书加载失败，请稍后再试'
  } finally {
    if (seq === latestLoad) loading.value = false
  }
}

function retry() {
  void load()
}

watch(
  () => props.targetUserId,
  () => {
    pageData.value = null
    error.value = ''
    void load()
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  latestLoad += 1
})
</script>

<template>
  <AppSurface padding="lg" class="certificates-card" data-test="certificates-card">
    <header class="certificates-card__head">
      <div>
        <h2 class="certificates-card__title">荣誉证书</h2>
        <p class="certificates-card__hint">仅展示已通过审核的证书。</p>
      </div>
      <span v-if="pageData && pageData.total > 0" class="certificates-card__total">
        {{ pageData.total }} 张
      </span>
    </header>

    <AppLoadingState v-if="loading && !pageData" label="正在加载证书" />

    <AppErrorState v-else-if="error" title="证书加载失败" :description="error" @retry="retry" />

    <template v-else-if="pageData">
      <div v-if="certificates.length === 0" class="certificates-card__empty">
        <p class="certificates-card__empty-text" data-test="certificates-empty">暂无已认证证书</p>
        <RouterLink
          v-if="isSelf"
          class="certificates-card__cta"
          :to="{ name: 'certificate-settings' }"
        >
          去提交证书
        </RouterLink>
      </div>

      <ul v-else class="certificate-list">
        <li
          v-for="certificate in certificates"
          :key="certificate.id"
          class="certificate-item"
          data-test="certificate-item"
        >
          <div class="certificate-item__head">
            <i class="pi pi-verified certificate-item__icon" aria-hidden="true" />
            <strong class="certificate-item__type">{{ certificate.type }}</strong>
            <Tag value="已认证" severity="success" class="certificate-item__tag" />
          </div>

          <dl class="certificate-item__meta">
            <div class="certificate-item__row">
              <dt>等级</dt>
              <dd class="mono">{{ certificate.level }}</dd>
            </div>
            <div class="certificate-item__row">
              <dt>通过日期</dt>
              <dd>{{ approvedDate(certificate) }}</dd>
            </div>
          </dl>

          <div class="certificate-item__verify">
            <code class="mono certificate-item__code">{{ certificate.verifyCode }}</code>
            <RouterLink
              class="certificate-item__link"
              :to="{ name: 'certificate-verify', query: { code: certificate.verifyCode } }"
            >
              核验证书
            </RouterLink>
          </div>
        </li>
      </ul>
    </template>
  </AppSurface>
</template>

<style scoped>
.certificates-card {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-3);
}

.certificates-card__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
}

.certificates-card__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.05rem;
  font-weight: 600;
  letter-spacing: -0.01em;
}

.certificates-card__hint {
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text-muted);
  font-size: 0.75rem;
}

.certificates-card__total {
  flex: 0 0 auto;
  padding: 1px var(--kapoj-space-2);
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 0.6875rem;
  border: 1px solid var(--kapoj-border);
  border-radius: 999px;
}

.certificates-card__empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-5) var(--kapoj-space-3);
  text-align: center;
  background: var(--kapoj-bg-elevated);
  border: 1px dashed var(--kapoj-border);
  border-radius: var(--kapoj-radius-sm);
}

.certificates-card__empty-text {
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 0.875rem;
}

.certificates-card__cta {
  padding: 5px var(--kapoj-space-3);
  color: var(--kapoj-primary);
  font-size: 0.8125rem;
  font-weight: 600;
  text-decoration: none;
  background: var(--kapoj-primary-soft);
  border: 1px solid color-mix(in srgb, var(--kapoj-primary) 35%, transparent);
  border-radius: var(--kapoj-radius-sm);
}

.certificates-card__cta:hover {
  border-color: var(--kapoj-primary);
}

.certificate-list {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  padding: 0;
  margin: 0;
  list-style: none;
}

.certificate-item {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-2);
  padding: var(--kapoj-space-3);
  background: var(--kapoj-bg-elevated);
  border: 1px solid var(--kapoj-border);
  border-left: 3px solid var(--kapoj-success);
  border-radius: var(--kapoj-radius-sm);
}

.certificate-item__head {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-2);
  min-width: 0;
}

.certificate-item__icon {
  flex: 0 0 auto;
  color: var(--kapoj-success);
  font-size: 1rem;
}

.certificate-item__type {
  flex: 1 1 auto;
  min-width: 0;
  color: var(--kapoj-text);
  font-size: 0.875rem;
  font-weight: 600;
  overflow-wrap: anywhere;
}

.certificate-item__tag {
  flex: 0 0 auto;
}

.certificate-item__meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kapoj-space-1) var(--kapoj-space-4);
  margin: 0;
}

.certificate-item__row {
  display: flex;
  align-items: baseline;
  gap: var(--kapoj-space-2);
  min-width: 0;
}

.certificate-item__row dt {
  color: var(--kapoj-text-muted);
  font-size: 0.6875rem;
}

.certificate-item__row dd {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 0.8125rem;
}

.certificate-item__verify {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-1) var(--kapoj-space-2);
  padding-top: var(--kapoj-space-2);
  border-top: 1px solid var(--kapoj-border);
}

.certificate-item__code {
  min-width: 0;
  color: var(--kapoj-text-secondary);
  font-size: 0.6875rem;
  letter-spacing: 0.02em;
  overflow-wrap: anywhere;
}

.certificate-item__link {
  flex: 0 0 auto;
  color: var(--kapoj-primary);
  font-size: 0.75rem;
  font-weight: 600;
  text-decoration: none;
}

.certificate-item__link:hover {
  text-decoration: underline;
}
</style>
