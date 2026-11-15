<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
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
const detailVisible = ref(false)
const selectedCertificate = ref<ProfileCertificate | null>(null)
let latestLoad = 0

const certificates = computed(() => pageData.value?.items ?? [])
const isSelf = computed(() => auth.user?.id != null && auth.user.id === props.targetUserId)

function approvedDate(certificate: ProfileCertificate): string {
  return certificate.approvedAt ? formatDateTime(certificate.approvedAt) : '—'
}

function openDetail(certificate: ProfileCertificate) {
  selectedCertificate.value = certificate
  detailVisible.value = true
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
    detailVisible.value = false
    selectedCertificate.value = null
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
      <div class="certificates-card__heading">
        <span class="certificates-card__mark" aria-hidden="true">
          <i class="pi pi-verified" />
        </span>
        <div>
          <h2 class="certificates-card__title">荣誉证书</h2>
          <p class="certificates-card__hint">仅展示已通过审核的证书</p>
        </div>
      </div>
      <span v-if="pageData && pageData.total > 0" class="certificates-card__total">
        {{ pageData.total }} 张
      </span>
    </header>

    <AppLoadingState v-if="loading && !pageData" label="正在加载证书" />

    <AppErrorState v-else-if="error" title="证书加载失败" :description="error" @retry="retry" />

    <template v-else-if="pageData">
      <div v-if="certificates.length === 0" class="certificates-card__empty">
        <span class="certificates-card__empty-icon" aria-hidden="true">
          <i class="pi pi-verified" />
        </span>
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
          <button
            type="button"
            class="certificate-item__main"
            :aria-label="`查看证书详情：${certificate.type} ${certificate.level}`"
            @click="openDetail(certificate)"
          >
            <span class="certificate-item__icon" aria-hidden="true">
              <i class="pi pi-verified" />
            </span>

            <span class="certificate-item__content">
              <span class="certificate-item__title-row">
                <strong class="certificate-item__type">{{ certificate.type }}</strong>
                <Tag value="已认证" severity="success" class="certificate-item__tag" />
              </span>

              <span class="certificate-item__level">
                <span class="certificate-item__level-label">荣誉等级</span>
                <strong>{{ certificate.level }}</strong>
              </span>

              <span class="certificate-item__meta">
                <span class="certificate-item__meta-item">
                  <i class="pi pi-calendar" aria-hidden="true" />
                  {{ approvedDate(certificate) }}
                </span>
                <code class="mono certificate-item__code">{{ certificate.verifyCode }}</code>
              </span>
            </span>

            <i class="pi pi-chevron-right certificate-item__chevron" aria-hidden="true" />
          </button>

          <div class="certificate-item__verify">
            <RouterLink
              class="certificate-item__link"
              :to="{ name: 'certificate-verify', query: { code: certificate.verifyCode } }"
            >
              核验证书
              <i class="pi pi-arrow-right" aria-hidden="true" />
            </RouterLink>
          </div>
        </li>
      </ul>
    </template>

    <Dialog
      v-model:visible="detailVisible"
      modal
      :draggable="false"
      :style="{ width: 'min(92vw, 32rem)' }"
      header="证书详情"
      class="certificate-dialog"
    >
      <div v-if="selectedCertificate" class="certificate-detail" data-test="certificate-detail">
        <div class="certificate-detail__status">
          <i class="pi pi-verified" aria-hidden="true" />
          已认证证书
        </div>

        <dl class="certificate-detail__list">
          <div class="certificate-detail__row">
            <dt>证书 / 赛事</dt>
            <dd>{{ selectedCertificate.type }}</dd>
          </div>
          <div class="certificate-detail__row">
            <dt>荣誉等级</dt>
            <dd class="certificate-detail__level">{{ selectedCertificate.level }}</dd>
          </div>
          <div class="certificate-detail__row">
            <dt>认证日期</dt>
            <dd>{{ approvedDate(selectedCertificate) }}</dd>
          </div>
          <div class="certificate-detail__row">
            <dt>证书编号</dt>
            <dd>
              <code class="mono certificate-detail__code">
                {{ selectedCertificate.verifyCode }}
              </code>
            </dd>
          </div>
        </dl>

        <div class="certificate-detail__actions">
          <RouterLink
            class="certificate-detail__link"
            :to="{
              name: 'certificate-verify',
              query: { code: selectedCertificate.verifyCode },
            }"
            @click="detailVisible = false"
          >
            前往核验
            <i class="pi pi-arrow-right" aria-hidden="true" />
          </RouterLink>
        </div>
      </div>
    </Dialog>
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

.certificates-card__heading {
  display: flex;
  align-items: flex-start;
  gap: var(--kapoj-space-2);
  min-width: 0;
}

.certificates-card__mark {
  display: grid;
  flex: 0 0 auto;
  width: 34px;
  height: 34px;
  color: var(--profile-success, var(--kapoj-success));
  font-size: 15px;
  background: var(
    --profile-success-soft,
    color-mix(in srgb, var(--kapoj-success) 10%, transparent)
  );
  border: 1px solid
    color-mix(
      in srgb,
      var(--profile-success, var(--kapoj-success)) 22%,
      var(--profile-border, var(--kapoj-border))
    );
  border-radius: 10px;
  place-items: center;
}

.certificates-card__title {
  margin: 0;
  color: var(--profile-text, var(--kapoj-text));
  font-size: 1.05rem;
  font-weight: 600;
  line-height: 1.3;
  letter-spacing: -0.01em;
}

.certificates-card__hint {
  margin: 3px 0 0;
  color: var(--profile-text-muted, var(--kapoj-text-muted));
  font-size: 11px;
}

.certificates-card__total {
  flex: 0 0 auto;
  padding: 2px 8px;
  color: var(--profile-text-muted, var(--kapoj-text-muted));
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  border: 1px solid var(--profile-border, var(--kapoj-border));
  border-radius: 999px;
}

.certificates-card__empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--kapoj-space-3);
  padding: var(--kapoj-space-5) var(--kapoj-space-3);
  text-align: center;
  background: var(--profile-surface-muted, var(--kapoj-bg-elevated));
  border: 1px dashed var(--profile-border, var(--kapoj-border));
  border-radius: 12px;
}

.certificates-card__empty-icon {
  display: grid;
  width: 42px;
  height: 42px;
  color: var(--profile-success, var(--kapoj-success));
  font-size: 18px;
  background: var(
    --profile-success-soft,
    color-mix(in srgb, var(--kapoj-success) 10%, transparent)
  );
  border-radius: 50%;
  place-items: center;
}

.certificates-card__empty-text {
  margin: 0;
  color: var(--profile-text-muted, var(--kapoj-text-muted));
  font-size: 0.875rem;
}

.certificates-card__cta {
  padding: 5px var(--kapoj-space-3);
  color: var(--profile-primary-text, var(--kapoj-primary));
  font-size: 0.8125rem;
  font-weight: 600;
  text-decoration: none;
  background: var(--profile-primary-soft, var(--kapoj-primary-soft));
  border: 1px solid
    color-mix(
      in srgb,
      var(--profile-primary, var(--kapoj-primary)) 35%,
      var(--profile-border, var(--kapoj-border))
    );
  border-radius: var(--kapoj-radius-sm);
}

.certificates-card__cta:hover {
  border-color: var(--profile-primary, var(--kapoj-primary));
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
  background: var(--profile-surface-muted, var(--kapoj-bg-elevated));
  border: 1px solid var(--profile-border, var(--kapoj-border));
  border-radius: 12px;
  transition:
    transform var(--kapoj-transition),
    border-color var(--kapoj-transition),
    box-shadow var(--kapoj-transition);
}

.certificate-item:hover {
  border-color: color-mix(
    in srgb,
    var(--profile-success, var(--kapoj-success)) 38%,
    var(--profile-border, var(--kapoj-border))
  );
  box-shadow: var(--profile-shadow, var(--kapoj-shadow-sm));
  transform: translateY(-1px);
}

.certificate-item__main {
  display: flex;
  align-items: flex-start;
  gap: var(--kapoj-space-3);
  width: 100%;
  padding: 14px;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
  background: none;
  border: 0;
  border-radius: 12px;
}

.certificate-item__main:focus-visible {
  outline-offset: -3px;
}

.certificate-item__icon {
  display: grid;
  flex: 0 0 auto;
  width: 34px;
  height: 34px;
  color: var(--profile-success, var(--kapoj-success));
  font-size: 16px;
  background: var(
    --profile-success-soft,
    color-mix(in srgb, var(--kapoj-success) 10%, transparent)
  );
  border-radius: 9px;
  place-items: center;
}

.certificate-item__content {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 5px;
  min-width: 0;
}

.certificate-item__title-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--kapoj-space-2);
  min-width: 0;
}

.certificate-item__type {
  flex: 1 1 auto;
  min-width: 0;
  color: var(--profile-text, var(--kapoj-text));
  font-size: 0.875rem;
  font-weight: 700;
  line-height: 1.35;
  overflow-wrap: anywhere;
}

.certificate-item__tag {
  flex: 0 0 auto;
  padding: 1px 7px !important;
  color: var(--profile-success-text, var(--kapoj-success)) !important;
  font-size: 9px !important;
  font-weight: 600;
  background: var(--profile-success-soft, rgb(25 164 99 / 10%)) !important;
  border: 1px solid
    color-mix(in srgb, var(--profile-success, var(--kapoj-success)) 24%, transparent);
  border-radius: 999px !important;
}

.certificate-item__level {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  width: fit-content;
  padding: 2px 8px;
  color: var(--profile-success-text, var(--kapoj-success));
  font-size: 12px;
  background: var(
    --profile-success-soft,
    color-mix(in srgb, var(--kapoj-success) 10%, transparent)
  );
  border-radius: 999px;
}

.certificate-item__level-label {
  font-size: 9px;
  font-weight: 500;
}

.certificate-item__meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px var(--kapoj-space-2);
  color: var(--profile-text-muted, var(--kapoj-text-muted));
  font-size: 10px;
}

.certificate-item__meta-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.certificate-item__meta-item i {
  font-size: 9px;
}

.certificate-item__code {
  min-width: 0;
  color: var(--profile-text-secondary, var(--kapoj-text-secondary));
  font-size: 9px;
  letter-spacing: 0.02em;
  overflow-wrap: anywhere;
}

.certificate-item__chevron {
  flex: 0 0 auto;
  margin-top: 8px;
  color: var(--profile-text-muted, var(--kapoj-text-muted));
  font-size: 10px;
  transition: transform var(--kapoj-transition);
}

.certificate-item:hover .certificate-item__chevron {
  color: var(--profile-success, var(--kapoj-success));
  transform: translateX(2px);
}

.certificate-item__verify {
  display: flex;
  justify-content: flex-end;
  padding: 8px 14px 10px;
  border-top: 1px solid var(--profile-border, var(--kapoj-border));
}

.certificate-item__link {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: var(--profile-primary-text, var(--kapoj-primary));
  font-size: 11px;
  font-weight: 600;
  text-decoration: none;
}

.certificate-item__link i {
  font-size: 9px;
}

.certificate-item__link:hover {
  text-decoration: underline;
}

.certificate-detail {
  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
}

.certificate-detail__status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  width: fit-content;
  padding: 4px 10px;
  color: var(--profile-success-text, var(--kapoj-success));
  font-size: 12px;
  font-weight: 600;
  background: var(
    --profile-success-soft,
    color-mix(in srgb, var(--kapoj-success) 10%, transparent)
  );
  border-radius: 999px;
}

.certificate-detail__list {
  display: flex;
  flex-direction: column;
  gap: 0;
  padding: 0;
  margin: 0;
  border: 1px solid var(--profile-border, var(--kapoj-border));
  border-radius: 10px;
}

.certificate-detail__row {
  display: flex;
  justify-content: space-between;
  gap: var(--kapoj-space-3);
  padding: 10px 12px;
  border-bottom: 1px solid var(--profile-border, var(--kapoj-border));
}

.certificate-detail__row:last-child {
  border-bottom: 0;
}

.certificate-detail__row dt {
  flex: 0 0 auto;
  color: var(--kapoj-text-secondary);
  font-size: 11px;
}

.certificate-detail__row dd {
  min-width: 0;
  margin: 0;
  color: var(--profile-text, var(--kapoj-text));
  font-size: 12px;
  font-weight: 600;
  text-align: right;
  overflow-wrap: anywhere;
}

.certificate-detail__level {
  color: var(--profile-success-text, var(--kapoj-success)) !important;
}

.certificate-detail__code {
  font-size: 11px;
  font-weight: 500;
}

.certificate-detail__actions {
  display: flex;
  justify-content: flex-end;
}

.certificate-detail__link {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 7px 13px;
  color: var(--profile-primary-text, var(--kapoj-primary));
  font-size: 12px;
  font-weight: 600;
  text-decoration: none;
  background: var(--profile-primary-soft, var(--kapoj-primary-soft));
  border: 1px solid
    color-mix(
      in srgb,
      var(--profile-primary, var(--kapoj-primary)) 30%,
      var(--profile-border, var(--kapoj-border))
    );
  border-radius: var(--kapoj-radius-sm);
}

.certificate-detail__link:hover {
  text-decoration: none;
  border-color: var(--profile-primary, var(--kapoj-primary));
}

@media (prefers-reduced-motion: reduce) {
  .certificate-item,
  .certificate-item__chevron {
    transition: none;
  }

  .certificate-item:hover {
    transform: none;
  }

  .certificate-item:hover .certificate-item__chevron {
    transform: none;
  }
}
</style>
